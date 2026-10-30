package sessions

import (
	"errors"
	"log"
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

// maxAutoPasswordAttempts bounds how many times Shellink will automatically send
// the stored password in response to a password prompt before giving up. This
// prevents an infinite loop when the configured password is wrong.
const maxAutoPasswordAttempts = 3

// passwordPromptRe matches the SSH password prompt printed to the PTY so Shellink
// can auto-fill it (used instead of SSH_ASKPASS, which deadlocks under ConPTY on
// Windows).
var passwordPromptRe = regexp.MustCompile(`(?i)password\s*:`)

// authFailureRe matches output that means the connection or login attempt has
// definitively failed. When it appears the login phase is abandoned immediately
// so the session stops reporting CONNECTING and surfaces the error to the UI
// instead of hanging in "connecting" forever.
var authFailureRe = regexp.MustCompile(`(?i)(permission denied|authentication failed|access denied|connection refused|connection timed out|connection closed by|host key verification failed|could not resolve hostname|no route to host|network is unreachable|too many authentication failures)`)

// defaultLoginTimeout is how long Shellink waits for a login phase to complete
// before giving up on it. Without this a session whose prompt never matches the
// configured prompt regex (custom shells, non-interactive commands, unusual
// banners) would stay in CONNECTING forever, which looks like a hung connection.
const defaultLoginTimeout = 30 * time.Second

type SessionOptions struct {
	ID          string
	ProfileID   string
	ProfileName string
	Term        string
	Cols        int
	Rows        int
	PromptRegex *string
	SilenceMS   int
	// LoginPhase controls whether the session starts in the login phase. Command
	// sessions have nothing to log into, so they skip it.
	LoginPhase *bool
	// LoginTimeoutMS bounds how long the login phase may last. <=0 uses
	// defaultLoginTimeout.
	LoginTimeoutMS int
}

type Session struct {
	ID          string
	ProfileID   string
	ProfileName string
	Term        string
	CreatedAt   int64
	Cols        int
	Rows        int

	mu                   sync.Mutex
	state                State
	mode                 Mode
	lastSeq              int64
	lastOutputSeq        int64
	closed               bool
	internalDepth        int
	loginPhase           bool
	autoPassword         string
	autoPasswordAttempts int
	recentPlain          string
	recentPublic         string
	prompt               *regexp.Regexp
	silenceThreshold     time.Duration
	silenceTimer         *time.Timer
	loginDeadline        time.Time
	stateChanged         chan struct{}
	transport            Transport
	bus                  *Bus
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
	loginPhase := true
	if opts.LoginPhase != nil {
		loginPhase = *opts.LoginPhase
	}
	loginTimeout := defaultLoginTimeout
	if opts.LoginTimeoutMS > 0 {
		loginTimeout = time.Duration(opts.LoginTimeoutMS) * time.Millisecond
	}
	return &Session{
		ID: opts.ID, ProfileID: opts.ProfileID, ProfileName: opts.ProfileName, Term: opts.Term,
		Cols: opts.Cols, Rows: opts.Rows, CreatedAt: time.Now().UnixMilli(),
		state: Connecting, mode: Auto, loginPhase: loginPhase, prompt: prompt,
		silenceThreshold: time.Duration(opts.SilenceMS) * time.Millisecond,
		loginDeadline:    time.Now().Add(loginTimeout),
		stateChanged:     make(chan struct{}), bus: bus,
	}, nil
}

func (s *Session) Attach(transport Transport) {
	s.mu.Lock()
	s.transport = transport
	s.mu.Unlock()
	log.Printf("[Shellink] session=%s event=transport-attached", s.ID)
	go func() {
		// Safety net: the silence timer only fires after output arrives, so a
		// session that never produces output would stay in the login phase (and
		// therefore in CONNECTING) forever. End the login phase on a deadline.
		if deadline, ok := s.loginDeadlineOnce(); ok {
			timer := time.AfterFunc(time.Until(deadline), s.endLoginPhase)
			defer timer.Stop()
		}
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
		log.Printf("[Shellink] session=%s event=transport-ended reason=%q exit_code=%v", s.ID, reason, exitCode)
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
	state, mode := s.state, s.mode
	var event *Event
	if record {
		event = s.recordLocked("input", data, data)
	}
	s.mu.Unlock()
	log.Printf("[Shellink] session=%s event=input bytes=%d mode=%s state=%s", s.ID, len(data), mode, state)
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
	previous := s.mode
	s.mode = mode
	s.mu.Unlock()
	log.Printf("[Shellink] session=%s event=mode previous=%s mode=%s", s.ID, previous, mode)
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

// WaitForStableAfter ignores a prompt left over from before the command was sent.
// Output may arrive and settle before this method is called, so compare sequence
// numbers rather than requiring the caller to observe an intermediate state.
func (s *Session) WaitForStableAfter(afterSeq int64, timeout time.Duration, acceptIdle bool) (State, bool) {
	if timeout <= 0 {
		timeout = time.Millisecond
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		s.mu.Lock()
		state := s.state
		hasOutput := s.lastOutputSeq > afterSeq
		changed := s.stateChanged
		s.mu.Unlock()
		if state == Disconnected || (hasOutput && (state == WaitingInput || (acceptIdle && state == Idle))) {
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
	internal := s.internalDepth > 0
	plain := StripANSI(raw)
	s.appendRecentLocked(plain)
	event := s.recordLocked("output", raw, plain)
	s.lastOutputSeq = s.lastSeq
	stateEvent := s.transitionLocked(Outputting)
	if s.silenceTimer != nil {
		s.silenceTimer.Stop()
	}
	s.silenceTimer = time.AfterFunc(s.silenceThreshold, s.evaluateSilence)
	// Auto-fill the password prompt through the PTY when running in AUTO mode
	// during the login phase. This replaces the SSH_ASKPASS mechanism, which
	// deadlocks under ConPTY on Windows.
	autoPassword := ""
	shouldSend := false
	if s.mode == Auto && s.loginPhase && s.autoPassword != "" && s.autoPasswordAttempts < maxAutoPasswordAttempts {
		if passwordPromptRe.MatchString(plain) {
			autoPassword = s.autoPassword
			shouldSend = true
			s.autoPasswordAttempts++
		}
	}
	s.mu.Unlock()
	log.Printf("[Shellink] session=%s event=output bytes=%d internal=%t kind=%s", s.ID, len(raw), internal, outputDiagnosticKind(plain))
	s.bus.Publish(*event)
	if stateEvent != nil {
		s.bus.Publish(*stateEvent)
	}
	if shouldSend {
		if err := s.Write(autoPassword+"\n", false); err != nil {
			log.Printf("[Shellink] session=%s event=auto-password-write-error err=%v", s.ID, err)
		} else {
			log.Printf("[Shellink] session=%s event=auto-password-sent attempt=%d", s.ID, s.autoPasswordAttempts)
		}
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

// loginDeadlineOnce reports the login-phase deadline for sessions that start in
// the login phase.
func (s *Session) loginDeadlineOnce() (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.loginPhase {
		return time.Time{}, false
	}
	return s.loginDeadline, true
}

// endLoginPhase aborts the login phase so the session leaves CONNECTING even if
// no output ever arrived (for example a host that silently drops the connection).
func (s *Session) endLoginPhase() {
	s.mu.Lock()
	if !s.loginPhase {
		s.mu.Unlock()
		return
	}
	s.loginPhase = false
	next := Idle
	if s.prompt.MatchString(lastLine(s.recentPlain)) {
		next = WaitingInput
	}
	event := s.transitionLocked(next)
	s.mu.Unlock()
	log.Printf("[Shellink] session=%s event=login-phase-timeout next=%s", s.ID, next)
	if event != nil {
		s.bus.Publish(*event)
	}
}

func lastLine(text string) string {
	lines := strings.Split(text, "\n")
	return lines[len(lines)-1]
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
	authFailed := authFailureRe.MatchString(s.recentPlain)
	loginExpired := time.Now().After(s.loginDeadline)
	next := Idle
	if s.loginPhase {
		switch {
		case atPrompt:
			// Reached a shell prompt: login finished and the remote is waiting
			// for a command.
			s.loginPhase = false
			next = WaitingInput
		case authFailed:
			// The credentials were rejected (or the host refused us). Stop
			// pretending we are still connecting so the UI can show the error.
			s.loginPhase = false
			next = Idle
		case loginExpired:
			// Give up on the login phase: the prompt never matched. Fall back to
			// IDLE so the session becomes usable instead of hanging in CONNECTING.
			s.loginPhase = false
			next = Idle
		default:
			next = Connecting
		}
	} else if atPrompt {
		next = WaitingInput
	}
	loginPhase := s.loginPhase
	event := s.transitionLocked(next)
	s.mu.Unlock()
	log.Printf("[Shellink] session=%s event=silence login_phase=%t prompt_match=%t auth_failed=%t login_expired=%t next=%s", s.ID, loginPhase, atPrompt, authFailed, loginExpired, next)
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
	log.Printf("[Shellink] session=%s event=state previous=%s state=%s", s.ID, previous, next)
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
	// The channel is gone, so there is nothing left to log into.
	s.loginPhase = false
	if s.silenceTimer != nil {
		s.silenceTimer.Stop()
	}
	stateEvent := s.transitionLocked(Disconnected)
	s.mu.Unlock()
	log.Printf("[Shellink] session=%s event=closed reason=%q exit_code=%v", s.ID, reason, exitCode)
	if stateEvent != nil {
		s.bus.Publish(*stateEvent)
	}
	s.bus.Publish(Event{Name: "session.closed", Data: protocol.Object(
		"sessionId", s.ID, "reason", reason, "exitCode", exitCode,
	)})
}

func outputDiagnosticKind(text string) string {
	text = strings.ToLower(text)
	switch {
	case strings.Contains(text, "permission denied"), strings.Contains(text, "authentication failed"):
		return "auth-rejected"
	case strings.Contains(text, "password:"):
		return "password-prompt"
	case strings.Contains(text, "gateshell"):
		return "gate-shell"
	default:
		return "terminal-data"
	}
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
