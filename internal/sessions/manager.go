package sessions

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"shellink/internal/config"
	"shellink/internal/protocol"
	"shellink/internal/service"
	"shellink/internal/store"
)

type Manager struct {
	db  *store.Store
	cfg config.Config
	bus *Bus
	box store.SecretBox

	mu   sync.RWMutex
	live map[string]*Session

	historyMu sync.Mutex
	pending   []store.HistoryChunk

	jobsMu     sync.Mutex
	jobs       map[string]*Job
	locks      map[string]string
	jobCancels map[string]context.CancelFunc
	started    time.Time
	stopCh     chan struct{}
	stopOnce   sync.Once
}

func NewManager(db *store.Store, cfg config.Config, bus *Bus) *Manager {
	m := &Manager{
		db: db, cfg: cfg, bus: bus, box: store.NewSecretBox(cfg.MasterKey), live: make(map[string]*Session),
		jobs: make(map[string]*Job), locks: make(map[string]string), jobCancels: make(map[string]context.CancelFunc),
		started: time.Now(), stopCh: make(chan struct{}),
	}
	bus.Subscribe(m.onEvent)
	go m.flushHistoryPeriodically()
	return m
}

func (m *Manager) MarkStale() error { return m.db.MarkStaleSessions() }

func (m *Manager) CloseAll(reason string) {
	m.stopOnce.Do(func() { close(m.stopCh) })
	m.jobsMu.Lock()
	cancellations := make([]context.CancelFunc, 0, len(m.jobCancels))
	for _, cancel := range m.jobCancels {
		cancellations = append(cancellations, cancel)
	}
	m.jobsMu.Unlock()
	for _, cancel := range cancellations {
		cancel()
	}
	m.mu.RLock()
	items := make([]*Session, 0, len(m.live))
	for _, session := range m.live {
		items = append(items, session)
	}
	m.mu.RUnlock()
	for _, session := range items {
		session.Close(reason)
	}
	m.flushHistory()
}

func (m *Manager) Create(profileID string, cols, rows int) (protocol.Value, error) {
	profile, err := m.db.Profile(profileID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, service.NotFound("Profile not found")
	}
	if err != nil {
		return nil, service.Internal(err)
	}
	if cols == 0 {
		cols = profile.Cols
	}
	if rows == 0 {
		rows = profile.Rows
	}
	if cols < 20 || cols > 500 || rows < 5 || rows > 200 {
		return nil, service.Invalid("Terminal dimensions are invalid")
	}
	id, err := m.allocateID()
	if err != nil {
		return nil, service.Internal(err)
	}
	// Command sessions run a local program: there is no login handshake to wait
	// for, so they must not start in the login phase (otherwise they would be
	// pinned to CONNECTING because no shell prompt ever appears).
	loginPhase := profile.ConnectType != "command"
	session, err := NewSession(SessionOptions{
		ID: id, ProfileID: profile.ID, ProfileName: profile.Name, Term: profile.Term,
		Cols: cols, Rows: rows, PromptRegex: profile.PromptRegex, SilenceMS: m.cfg.SilenceThresholdMS,
		LoginPhase: &loginPhase, LoginTimeoutMS: m.cfg.SSHReadyTimeoutMS,
	}, m.bus)
	if err != nil {
		return nil, service.Invalid("Invalid promptRegex: " + err.Error())
	}
	program, args, env, keyPath, password, err := m.sessionCommand(profile, id)
	if err != nil {
		return nil, service.Internal(err)
	}
	if password != "" {
		session.autoPassword = password
	}
	transport, err := NewProcessTransport(program, args, env, profile.Term, cols, rows)
	if err != nil {
		if keyPath != "" {
			_ = os.Remove(keyPath)
		}
		return nil, service.Internal(err)
	}
	target := profileTarget(profile)
	if profile.ConnectType == "command" && len([]rune(target)) > 120 {
		target = string([]rune(target)[:117]) + "..."
	}
	record := store.SessionRecord{
		ID: id, ProfileID: profile.ID, ProfileName: profile.Name, Target: target,
		State: string(Connecting), Mode: string(Auto), Cols: cols, Rows: rows, CreatedAt: session.CreatedAt,
	}
	if err := m.db.InsertSession(record); err != nil {
		_ = transport.Close()
		if keyPath != "" {
			_ = os.Remove(keyPath)
		}
		return nil, service.Internal(err)
	}
	log.Printf("[Shellink] session=%s event=created profile_id=%s connect_type=%s auth_type=%s password_configured=%t key_configured=%t", id, profile.ID, profile.ConnectType, profile.AuthType, profile.EncryptedPassword != nil, profile.EncryptedPrivateKey != nil)
	m.mu.Lock()
	m.live[id] = session
	m.mu.Unlock()
	if keyPath != "" {
		var unsubscribe func()
		unsubscribe = m.bus.Subscribe(func(event Event) {
			if event.Name == "session.closed" && protocol.MapValue(event.Data)["sessionId"] == id {
				_ = os.Remove(keyPath)
				unsubscribe()
			}
		})
	}
	session.Attach(transport)
	m.bus.Publish(Event{Name: "session.created", Data: protocol.Object("sessionId", id)})
	return protocol.Object("id", id, "state", string(Connecting), "mode", string(Auto), "target", profileTarget(profile)), nil
}

func (m *Manager) sessionCommand(profile store.Profile, sessionID string) (string, []string, map[string]string, string, string, error) {
	if profile.ConnectType == "command" {
		command := ""
		if profile.Command != nil {
			command = *profile.Command
		}
		if runtime.GOOS == "windows" {
			shell := os.Getenv("COMSPEC")
			if shell == "" {
				shell = "cmd.exe"
			}
			return shell, []string{"/D", "/S", "/C", command}, nil, "", "", nil
		}
		return "/bin/sh", []string{"-lc", command}, nil, "", "", nil
	}
	ssh, err := exec.LookPath("ssh")
	if err != nil && runtime.GOOS == "windows" {
		ssh, err = exec.LookPath("ssh.exe")
	}
	if err != nil {
		return "", nil, nil, "", "", fmt.Errorf("OpenSSH client was not found in PATH")
	}
	args := []string{
		"-tt", "-p", fmt.Sprint(profile.Port),
		"-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=40",
		"-o", fmt.Sprintf("ConnectTimeout=%d", max(1, m.cfg.SSHReadyTimeoutMS/1000)),
		// Accept new host keys automatically so a first connection does not hang
		// on the "Are you sure you want to continue connecting?" prompt.
		"-o", "StrictHostKeyChecking=accept-new",
	}
	env := map[string]string{}
	keyPath := ""
	password := ""
	if profile.EncryptedPrivateKey != nil {
		privateKey, decryptErr := m.box.Decrypt(*profile.EncryptedPrivateKey)
		if decryptErr != nil {
			return "", nil, nil, "", "", fmt.Errorf("decrypt profile key: %w", decryptErr)
		}
		keyPath = filepath.Join(m.cfg.DataHome, ".shellink-key-"+sessionID)
		if err := os.WriteFile(keyPath, []byte(privateKey), 0600); err != nil {
			return "", nil, nil, "", "", fmt.Errorf("write temporary SSH key: %w", err)
		}
		args = append(args, "-i", keyPath, "-o", "IdentitiesOnly=yes")
	}
	if profile.EncryptedPassword != nil {
		decrypted, decryptErr := m.box.Decrypt(*profile.EncryptedPassword)
		if decryptErr != nil {
			if keyPath != "" {
				_ = os.Remove(keyPath)
			}
			return "", nil, nil, "", "", fmt.Errorf("decrypt profile password: %w", decryptErr)
		}
		password = decrypted
		// Hand the password to ssh through SSH_ASKPASS. This is the mechanism
		// OpenSSH uses when stdin is not a terminal, which is exactly the case
		// for Shellink's pipe based transport on Windows. (Under ConPTY the
		// askpass child inherits the pseudo console and its output never reaches
		// ssh, which is why ConPTY is no longer the default on Windows.)
		executable, _ := os.Executable()
		env["SHELLINK_ASKPASS_SECRET"] = password
		env["SHELLINK_SESSION_ID"] = sessionID
		env["SHELLINK_LOG"] = m.cfg.LogPath
		env["SSH_ASKPASS"] = executable
		env["SSH_ASKPASS_REQUIRE"] = "force"
		if os.Getenv("DISPLAY") == "" {
			env["DISPLAY"] = "shellink:0"
		}
	}
	host := profile.Host
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	args = append(args, profile.Username+"@"+host)
	return ssh, args, env, keyPath, password, nil
}

func (m *Manager) Get(id string) *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.live[id]
}

func (m *Manager) List() (protocol.Value, error) {
	records, err := m.db.Sessions()
	if err != nil {
		return nil, service.Internal(err)
	}
	type item struct {
		value  protocol.Value
		active bool
		at     int64
	}
	items := make([]item, 0, len(records))
	for _, record := range records {
		state, mode, _, active := m.getSnapshot(record.ID)
		if !active {
			state, mode = State(record.State), Mode(record.Mode)
		}
		at := record.CreatedAt
		if !active && record.ClosedAt != nil {
			at = *record.ClosedAt
		}
		items = append(items, item{value: protocol.Object(
			"id", record.ID, "profileId", record.ProfileID, "profileName", record.ProfileName,
			"target", record.Target, "state", string(state), "mode", string(mode),
			"cols", record.Cols, "rows", record.Rows, "createdAt", record.CreatedAt,
			"closedAt", record.ClosedAt, "closeReason", record.CloseReason, "active", active,
		), active: active, at: at})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].active != items[j].active {
			return items[i].active
		}
		return items[i].at > items[j].at
	})
	values := make([]protocol.Value, 0, len(items))
	for _, item := range items {
		values = append(values, item.value)
	}
	return values, nil
}

func (m *Manager) State(id string) (protocol.Value, error) {
	if session := m.Get(id); session != nil {
		state, mode, seq, active := session.Snapshot()
		return protocol.Object("id", id, "state", string(state), "mode", string(mode), "lastSeq", seq, "recentOutput", session.RecentOutput(15), "active", active), nil
	}
	record, err := m.db.Session(id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, service.NotFound("Session not found")
	}
	if err != nil {
		return nil, service.Internal(err)
	}
	// Once a session ends it leaves the live map, but the UI still reads its
	// state. Return the tail of the transcript so the terminal view does not go
	// blank the moment a session disconnects.
	recent := m.recentOutputFromHistory(id)
	return protocol.Object("id", id, "state", string(Disconnected), "mode", record.Mode,
		"closeReason", record.CloseReason, "exitCode", record.ExitCode, "active", false,
		"recentOutput", recent), nil
}

// recentOutputFromHistory rebuilds the last lines of output for a session that
// is no longer live.
func (m *Manager) recentOutputFromHistory(id string) string {
	m.flushHistory()
	chunks, err := m.db.History(id, 0, 200)
	if err != nil {
		return ""
	}
	var sb strings.Builder
	for _, chunk := range chunks {
		if chunk.Direction == "output" {
			sb.WriteString(chunk.DataPlain)
		}
	}
	lines := strings.Split(sb.String(), "\n")
	if len(lines) > 15 {
		lines = lines[len(lines)-15:]
	}
	return strings.Join(lines, "\n")
}

func (m *Manager) History(id string, since int64, limit int, includeInternal bool) (protocol.Value, error) {
	if _, err := m.db.Session(id); errors.Is(err, store.ErrNotFound) {
		return nil, service.NotFound("Session not found")
	} else if err != nil {
		return nil, service.Internal(err)
	}
	m.flushHistory()
	chunks, err := m.db.History(id, since, limit)
	if err != nil {
		return nil, service.Internal(err)
	}
	cursor := since
	if len(chunks) > 0 {
		cursor = chunks[len(chunks)-1].Seq
	}
	// Build an auditable transcript that keeps BOTH program output and the
	// commands the user typed (direction == "input"), so a session's history is
	// reviewable after the session ends — not only visible live in the terminal.
	var text string
	if includeInternal {
		var sb strings.Builder
		for _, chunk := range chunks {
			sb.WriteString(chunk.DataRaw)
		}
		text = sb.String()
	} else {
		text = renderHistoryWithoutInternal(chunks, "\n")
	}
	return protocol.Object("cursor", cursor, "text", StripANSI(text)), nil
}

func (m *Manager) RawHistory(id string, maxBytes int, includeInternal bool) (string, error) {
	if _, err := m.db.Session(id); errors.Is(err, store.ErrNotFound) {
		return "", service.NotFound("Session not found")
	} else if err != nil {
		return "", service.Internal(err)
	}
	m.flushHistory()
	chunks, err := m.db.AllHistory(id, true)
	if err != nil {
		return "", err
	}
	if !includeInternal {
		return limitTail(renderHistoryWithoutInternal(chunks, "\r\n"), maxBytes), nil
	}
	var parts []string
	total := 0
	for i := len(chunks) - 1; i >= 0; i-- {
		if total+len(chunks[i].DataRaw) > maxBytes {
			break
		}
		total += len(chunks[i].DataRaw)
		parts = append(parts, chunks[i].DataRaw)
	}
	var result strings.Builder
	for i := len(parts) - 1; i >= 0; i-- {
		result.WriteString(parts[i])
	}
	return result.String(), nil
}

func (m *Manager) Input(id, text string, appendNewline, manual bool) (protocol.Value, error) {
	session := m.Get(id)
	if session == nil {
		return nil, service.NotFound("Session not found or disconnected")
	}
	state, mode, seq, _ := session.Snapshot()
	if manual {
		if mode != Manual {
			return nil, service.Conflict("Session is not in MANUAL mode")
		}
		if err := session.Write(text, true); err != nil {
			return nil, service.Conflict(err.Error())
		}
		_, _, seq, _ = session.Snapshot()
		return protocol.Object("ok", true, "state", string(state), "lastSeq", seq), nil
	}
	if mode == Manual {
		return nil, service.Conflict("Session is in MANUAL mode; AI input was rejected")
	}
	if state == Disconnected {
		return nil, service.NotFound("Session is disconnected")
	}
	payload := text
	if appendNewline {
		payload += "\n"
	}
	m.jobsMu.Lock()
	lock := m.locks[id]
	m.jobsMu.Unlock()
	if lock == "transfer" && text != "\x03" && payload != "\x03" {
		return nil, service.Conflict("This session is performing a file operation; try again later")
	}
	if state != Connecting && state != WaitingInput && state != Outputting && state != Idle && lock != "exec" {
		return nil, service.Conflict("Session state is " + string(state) + "; input is not currently allowed")
	}
	if err := session.Write(payload, true); err != nil {
		return nil, service.Conflict(err.Error())
	}
	_, _, seq, _ = session.Snapshot()
	return protocol.Object("ok", true, "state", string(state), "lastSeq", seq), nil
}

func (m *Manager) TerminalInput(id, text string) error {
	session := m.Get(id)
	if session == nil {
		return service.NotFound("Session not found or disconnected")
	}
	state, mode, _, _ := session.Snapshot()
	if mode != Manual && state != Connecting && !(mode == Auto && state == WaitingInput) {
		return service.Conflict("The current session state does not accept input")
	}
	if mode == Manual || state == Connecting {
		return session.Write(text, true)
	}
	_, err := m.Input(id, text, false, false)
	return err
}

func (m *Manager) Mode(id, mode string) (protocol.Value, error) {
	session := m.Get(id)
	if session == nil {
		return nil, service.NotFound("Session not found or disconnected")
	}
	if mode != string(Auto) && mode != string(Manual) {
		return nil, service.Invalid("mode must be AUTO or MANUAL")
	}
	session.SetMode(Mode(mode))
	return protocol.Object("ok", true, "mode", mode), nil
}

func (m *Manager) Resize(id string, cols, rows int) error {
	session := m.Get(id)
	if session == nil {
		return service.NotFound("Session not found or disconnected")
	}
	if cols < 20 || cols > 500 || rows < 5 || rows > 200 {
		return service.Invalid("Terminal dimensions are invalid")
	}
	return session.Resize(cols, rows)
}

func (m *Manager) Close(id string) (protocol.Value, error) {
	if session := m.Get(id); session != nil {
		session.Close("Close requested")
		return protocol.Object("ok", true), nil
	}
	if _, err := m.db.Session(id); errors.Is(err, store.ErrNotFound) {
		return nil, service.NotFound("Session not found")
	} else if err != nil {
		return nil, service.Internal(err)
	}
	return protocol.Object("ok", true, "note", "Session is already disconnected"), nil
}

func (m *Manager) Remove(id string) (protocol.Value, error) {
	if session := m.Get(id); session != nil {
		session.Close("Deleted from the Web UI")
	}
	if _, err := m.db.Session(id); errors.Is(err, store.ErrNotFound) {
		if m.Get(id) == nil {
			return nil, service.NotFound("Session not found")
		}
	} else if err != nil {
		return nil, service.Internal(err)
	}
	m.flushHistory()
	if err := m.db.DeleteSession(id); err != nil {
		return nil, service.Internal(err)
	}
	m.mu.Lock()
	delete(m.live, id)
	m.mu.Unlock()
	return protocol.Object("ok", true), nil
}

func (m *Manager) Purge(olderThan string) (protocol.Value, error) {
	if olderThan != "24h" && olderThan != "7d" && olderThan != "all" {
		return nil, service.Invalid("olderThan must be 24h, 7d, or all")
	}
	records, err := m.db.Sessions()
	if err != nil {
		return nil, service.Internal(err)
	}
	var cutoff int64
	if olderThan == "24h" {
		cutoff = time.Now().Add(-24 * time.Hour).UnixMilli()
	} else if olderThan == "7d" {
		cutoff = time.Now().Add(-7 * 24 * time.Hour).UnixMilli()
	}
	deleted := 0
	for _, record := range records {
		if m.Get(record.ID) != nil {
			continue
		}
		endedAt := record.CreatedAt
		if record.ClosedAt != nil {
			endedAt = *record.ClosedAt
		}
		if olderThan != "all" && endedAt >= cutoff {
			continue
		}
		if _, err := m.Remove(record.ID); err == nil {
			deleted++
		}
	}
	return protocol.Object("ok", true, "deleted", deleted), nil
}

func (m *Manager) onEvent(event Event) {
	fields := protocol.MapValue(event.Data)
	getString := func(key string) string {
		value, _ := protocol.StringValue(fields[key])
		return value
	}
	switch event.Name {
	case "session.data":
		seq, _ := protocol.IntValue(fields["seq"])
		internal, _ := protocol.BoolValue(fields["internal"])
		m.historyMu.Lock()
		m.pending = append(m.pending, store.HistoryChunk{
			SessionID: getString("sessionId"), Seq: seq, Direction: getString("direction"),
			DataRaw: getString("raw"), DataPlain: getString("plain"), Internal: internal,
			CreatedAt: time.Now().UnixMilli(),
		})
		if len(m.pending) >= 128 {
			m.flushHistoryLocked()
		}
		m.historyMu.Unlock()
	case "session.state":
		_ = m.db.UpdateSessionState(getString("sessionId"), getString("state"))
	case "session.mode":
		_ = m.db.UpdateSessionMode(getString("sessionId"), getString("mode"))
	case "session.closed":
		var exitCode *int
		if value, ok := protocol.IntValue(fields["exitCode"]); ok {
			n := int(value)
			exitCode = &n
		}
		_ = m.db.CloseSession(getString("sessionId"), getString("reason"), exitCode)
		m.mu.Lock()
		delete(m.live, getString("sessionId"))
		m.mu.Unlock()
	}
}

func (m *Manager) flushHistory() {
	m.historyMu.Lock()
	m.flushHistoryLocked()
	m.historyMu.Unlock()
}

func (m *Manager) flushHistoryLocked() {
	if len(m.pending) == 0 {
		return
	}
	batch := m.pending
	m.pending = nil
	if err := m.db.AddHistory(batch); err != nil {
		m.pending = append(batch, m.pending...)
	}
}

func (m *Manager) flushHistoryPeriodically() {
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.flushHistory()
		case <-m.stopCh:
			return
		}
	}
}

func (m *Manager) allocateID() (string, error) {
	const alphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz"
	for attempt := 0; attempt < 32; attempt++ {
		var bytes [8]byte
		if _, err := rand.Read(bytes[:]); err != nil {
			return "", err
		}
		var id strings.Builder
		for _, value := range bytes {
			id.WriteByte(alphabet[int(value)%len(alphabet)])
		}
		if m.Get(id.String()) != nil {
			continue
		}
		if _, err := m.db.Session(id.String()); errors.Is(err, store.ErrNotFound) {
			return id.String(), nil
		} else if err != nil {
			return "", err
		}
	}
	return "", errors.New("unable to allocate a unique session ID")
}

func (m *Manager) getSnapshot(id string) (State, Mode, int64, bool) {
	if session := m.Get(id); session != nil {
		state, mode, seq, active := session.Snapshot()
		return state, mode, seq, active
	}
	return Disconnected, Auto, 0, false
}

func profileTarget(profile store.Profile) string {
	if profile.ConnectType == "command" {
		if profile.Command == nil {
			return ""
		}
		return *profile.Command
	}
	return fmt.Sprintf("%s@%s:%d", profile.Username, profile.Host, profile.Port)
}

func renderHistoryWithoutInternal(chunks []store.HistoryChunk, newline string) string {
	var output strings.Builder
	hidden := 0
	flush := func() {
		if hidden == 0 {
			return
		}
		fmt.Fprintf(&output, "%s[shellink] hidden %d internal transfer output chunks%s", newline, hidden, newline)
		hidden = 0
	}
	for _, chunk := range chunks {
		if chunk.Internal {
			hidden++
			continue
		}
		flush()
		if chunk.Direction == "input" {
			// Mark user-typed commands so the audit trail is readable. The
			// trailing newline is restored so the following program output
			// starts on its own line instead of being glued to the command.
			output.WriteString(newline + "❯ " + cleanInput(chunk.DataPlain) + newline)
			continue
		}
		output.WriteString(chunk.DataRaw)
	}
	flush()
	return output.String()
}

// cleanInput normalises a recorded user input string for display in the
// auditable history: drop carriage returns and any trailing newlines.
func cleanInput(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "")
	return strings.TrimRight(text, "\n")
}

func limitTail(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	return value[len(value)-maxBytes:]
}
