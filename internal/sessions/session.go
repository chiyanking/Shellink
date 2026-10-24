package sessions

import (
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"

	"shellink/internal/protocol"
)

type State string
type Mode string

const (
	Connecting   State = "CONNECTING"
	Outputting   State = "OUTPUTTING"
	Idle         State = "IDLE"
	WaitingInput State = "WAITING_INPUT"
	Disconnected State = "DISCONNECTED"
	Auto         Mode  = "AUTO"
	Manual       Mode  = "MANUAL"
)

const defaultPromptPattern = `[$#>%](\s|\x{00a0})*$`

type SessionOptions struct {
	ID          string
	ProfileID   string
	ProfileName string
	Term        string
	Cols        int
	Rows        int
	PromptRegex *string
	SilenceMS   int
}

type Session struct {
	ID          string
	ProfileID   string
	ProfileName string
	Term        string
	CreatedAt   int64
	Cols        int
	Rows        int

	mu               sync.Mutex
	state            State
	mode             Mode
	lastSeq          int64
	closed           bool
	internalDepth    int
	loginPhase       bool
	recentPlain      string
	recentPublic     string
	prompt           *regexp.Regexp
	silenceThreshold time.Duration
	silenceTimer     *time.Timer
	stateChanged     chan struct{}
	transport        Transport
	bus              *Bus
}

func NewSession(opts SessionOptions, bus *Bus) (*Session, error) {
	pattern := defaultPromptPattern
	if opts.PromptRegex != nil && *opts.PromptRegex != "" {
		pattern = *opts.PromptRegex
	}
	prompt, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	return &Session{
		ID: opts.ID, ProfileID: opts.ProfileID, ProfileName: opts.ProfileName, Term: opts.Term,
		Cols: opts.Cols, Rows: opts.Rows, CreatedAt: time.Now().UnixMilli(),
		state: Connecting, mode: Auto, loginPhase: true, prompt: prompt,
		silenceThreshold: time.Duration(opts.SilenceMS) * time.Millisecond,
		stateChanged:     make(chan struct{}), bus: bus,
	}, nil
}

func (s *Session) Attach(transport Transport) {
	s.mu.Lock()
	s.transport = transport
	s.mu.Unlock()
	go func() {
		for output := range transport.Output() {
			s.handleOutput(output)
		}
		waitErr := <-transport.Done()
		var exitCode *int
		var processErr interface{ ExitCode() int }
		if errors.As(waitErr, &processErr) {
			code := processErr.ExitCode()
			exitCode = &code
		}
		reason := "Shell channel closed"
		if waitErr != nil && exitCode == nil {
			reason = "Connection error: " + waitErr.Error()
		}
		s.handleClose(reason, exitCode)
	}()
}

func (s *Session) Snapshot() (State, Mode, int64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state, s.mode, s.lastSeq, !s.closed
}

func (s *Session) LastSequence() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastSeq
}

func (s *Session) RecentOutput(maxLines int) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if maxLines <= 0 {
		maxLines = 15
	}
	lines := strings.Split(s.recentPublic, "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, "\n")
}

func (s *Session) Write(data string, record bool) error {
	s.mu.Lock()
	if s.closed || s.transport == nil {
		s.mu.Unlock()
		return errors.New("session is disconnected")
	}
	transport := s.transport
	var event *Event
	if record {
		event = s.recordLocked("input", data, data)
	}
	s.mu.Unlock()
	if event != nil {
		s.bus.Publish(*event)
	}
	return transport.Write(data)
}

func (s *Session) Resize(cols, rows int) error {
	s.mu.Lock()
	if s.closed || s.transport == nil {
		s.mu.Unlock()
		return errors.New("session is disconnected")
	}
	s.Cols, s.Rows = cols, rows
	transport := s.transport
	s.mu.Unlock()
	return transport.Resize(cols, rows)
}

func (s *Session) SetMode(mode Mode) {
	s.mu.Lock()
	if s.mode == mode {
		s.mu.Unlock()
		return
	}
	s.mode = mode
	s.mu.Unlock()
	s.bus.Publish(Event{Name: "session.mode", Data: protocol.Object("sessionId", s.ID, "mode", string(mode))})
}

func (s *Session) BeginInternal() {
	s.mu.Lock()
	s.internalDepth++
	s.mu.Unlock()
}

func (s *Session) EndInternal() {
	s.mu.Lock()
	if s.internalDepth > 0 {
		s.internalDepth--
	}
	s.mu.Unlock()
}

func (s *Session) IsInternal() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.internalDepth > 0
}

func (s *Session) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *Session) EmitDisplayOutput(text string) {
	if text == "" {
		return
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	plain := StripANSI(text)
	s.appendRecentLocked(plain)
	event := s.recordLocked("output", text, plain)
	s.mu.Unlock()
	s.bus.Publish(*event)
}

func (s *Session) WaitForStable(timeout time.Duration, acceptIdle bool) (State, bool) {
	if timeout <= 0 {
		timeout = time.Millisecond
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		s.mu.Lock()
		state := s.state
		changed := s.stateChanged
		s.mu.Unlock()
		if state == Disconnected || state == WaitingInput || (acceptIdle && state == Idle) {
			return state, false
		}
		select {
		case <-timer.C:
			s.mu.Lock()
			state = s.state
			s.mu.Unlock()
			return state, true
		case <-changed:
		}
	}
}

func (s *Session) Close(reason string) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	transport := s.transport
	s.mu.Unlock()
	if transport != nil {
		_ = transport.Close()
	}
	s.handleClose(reason, nil)
}

func (s *Session) handleOutput(raw string) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	plain := StripANSI(raw)
	s.appendRecentLocked(plain)
	event := s.recordLocked("output", raw, plain)
	stateEvent := s.transitionLocked(Outputting)
	if s.silenceTimer != nil {
		s.silenceTimer.Stop()
	}
	s.silenceTimer = time.AfterFunc(s.silenceThreshold, s.evaluateSilence)
	s.mu.Unlock()
	s.bus.Publish(*event)
	if stateEvent != nil {
		s.bus.Publish(*stateEvent)
	}
}

func (s *Session) appendRecentLocked(plain string) {
	s.recentPlain = tailRunes(s.recentPlain+plain, 16384)
	if s.internalDepth == 0 {
		s.recentPublic = tailRunes(s.recentPublic+plain, 16384)
	}
}

func (s *Session) recordLocked(direction, raw, plain string) *Event {
	s.lastSeq++
	return &Event{Name: "session.data", Data: protocol.Object(
		"sessionId", s.ID, "seq", s.lastSeq, "direction", direction,
		"raw", raw, "plain", plain, "internal", s.internalDepth > 0,
	)}
}

func (s *Session) evaluateSilence() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	lines := strings.Split(s.recentPlain, "\n")
	lastLine := lines[len(lines)-1]
	atPrompt := s.prompt.MatchString(lastLine)
	next := Idle
	if s.loginPhase {
		if atPrompt {
			s.loginPhase = false
			next = WaitingInput
		} else {
			next = Connecting
		}
	} else if atPrompt {
		next = WaitingInput
	}
	event := s.transitionLocked(next)
	s.mu.Unlock()
	if event != nil {
		s.bus.Publish(*event)
	}
}

func (s *Session) transitionLocked(next State) *Event {
	if s.loginPhase && next == Outputting {
		next = Connecting
	}
	if s.state == next {
		return nil
	}
	previous := s.state
	s.state = next
	close(s.stateChanged)
	s.stateChanged = make(chan struct{})
	return &Event{Name: "session.state", Data: protocol.Object(
		"sessionId", s.ID, "state", string(next), "prevState", string(previous), "at", time.Now().UnixMilli(),
	)}
}

func (s *Session) handleClose(reason string, exitCode *int) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	if s.silenceTimer != nil {
		s.silenceTimer.Stop()
	}
	stateEvent := s.transitionLocked(Disconnected)
	s.mu.Unlock()
	if stateEvent != nil {
		s.bus.Publish(*stateEvent)
	}
	s.bus.Publish(Event{Name: "session.closed", Data: protocol.Object(
		"sessionId", s.ID, "reason", reason, "exitCode", exitCode,
	)})
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[@-Z\\-_]|\x1b\([AB012]`)
var controlsPattern = regexp.MustCompile(`[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]`)

func StripANSI(input string) string {
	input = ansiPattern.ReplaceAllString(input, "")
	input = strings.ReplaceAll(input, "\r\n", "\n")
	input = strings.ReplaceAll(input, "\r", "")
	return controlsPattern.ReplaceAllString(input, "")
}

func tailRunes(input string, max int) string {
	runes := []rune(input)
	if len(runes) > max {
		runes = runes[len(runes)-max:]
	}
	return string(runes)
}
