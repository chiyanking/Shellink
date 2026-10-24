package sessions

import (
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"shellink/internal/protocol"
	"shellink/internal/service"
	"shellink/internal/store"
)

type JobStatus string

const (
	JobRunning      JobStatus = "RUNNING"
	JobDone         JobStatus = "DONE"
	JobTimedOut     JobStatus = "TIMED_OUT"
	JobCanceled     JobStatus = "CANCELED"
	JobDisconnected JobStatus = "DISCONNECTED"
	JobFailed       JobStatus = "FAILED"
)

type Job struct {
	ID        string
	SessionID string
	Kind      string
	Command   *string
	Remote    *string
	StartSeq  int64
	StartedAt int64
	EndedAt   *int64
	TimeoutMS int
	Status    JobStatus
	State     *string
	Error     *string
	Result    protocol.Value
	changed   chan struct{}
}

func (j *Job) value() protocol.Value {
	return protocol.Object("id", j.ID, "sessionId", j.SessionID, "kind", j.Kind,
		"command", j.Command, "remotePath", j.Remote, "startSeq", j.StartSeq,
		"startedAt", j.StartedAt, "endedAt", j.EndedAt, "timeoutMs", j.TimeoutMS,
		"status", string(j.Status), "state", j.State, "error", j.Error, "result", j.Result)
}

func (m *Manager) Exec(sessionID, command string, timeoutMS int) (protocol.Value, error) {
	if timeoutMS <= 0 {
		timeoutMS = m.cfg.ExecDefaultTimeoutMS
	}
	started, err := m.ExecStart(sessionID, command, timeoutMS)
	if err != nil {
		return nil, err
	}
	fields := protocol.MapValue(started)
	jobID, _ := protocol.StringValue(fields["jobId"])
	job, err := m.waitJob(jobID, time.Duration(timeoutMS+1000)*time.Millisecond, false)
	if err != nil {
		return nil, err
	}
	if job.Status == JobFailed && job.Error != nil {
		return nil, service.Conflict(*job.Error)
	}
	text, cursor := m.historyText(job.SessionID, job.StartSeq, 10_000, false)
	return protocol.Object("jobId", job.ID, "status", string(job.Status),
		"state", nullableString(job.State, string(m.currentState(sessionID))),
		"output", text, "startSeq", job.StartSeq, "cursor", cursor,
		"durationMs", time.Now().UnixMilli()-job.StartedAt, "timedOut", job.Status == JobTimedOut), nil
}

func (m *Manager) ExecStart(sessionID, command string, timeoutMS int) (protocol.Value, error) {
	if command == "" {
		return nil, service.Invalid("command must be a non-empty string")
	}
	if timeoutMS <= 0 {
		timeoutMS = m.cfg.ExecDefaultTimeoutMS
	}
	if timeoutMS < 1000 || timeoutMS > 600_000 {
		return nil, service.Invalid("timeoutMs must be between 1000 and 600000")
	}
	session := m.Get(sessionID)
	if session == nil {
		return nil, service.NotFound("Session not found or disconnected")
	}
	state, mode, startSeq, _ := session.Snapshot()
	if mode == Manual {
		return nil, service.Conflict("Session is in MANUAL mode; AI input was rejected")
	}
	m.jobsMu.Lock()
	if m.locks[sessionID] != "" {
		message := m.lockBusyMessageLocked(sessionID)
		m.jobsMu.Unlock()
		return nil, service.Conflict(message)
	}
	if state != WaitingInput {
		m.jobsMu.Unlock()
		return nil, service.Conflict("Session state is " + string(state) + "; commands can run only while WAITING_INPUT")
	}
	jobID, err := randomJobID(m.jobs)
	if err != nil {
		m.jobsMu.Unlock()
		return nil, service.Internal(err)
	}
	cmd := command
	job := &Job{ID: jobID, SessionID: sessionID, Kind: "exec", Command: &cmd,
		StartSeq: startSeq, StartedAt: time.Now().UnixMilli(), TimeoutMS: timeoutMS,
		Status: JobRunning, changed: make(chan struct{})}
	m.jobs[jobID] = job
	m.locks[sessionID] = "exec"
	m.jobsMu.Unlock()
	go m.runExec(jobID, session, command, time.Duration(timeoutMS)*time.Millisecond)
	return protocol.Object("jobId", jobID, "status", string(JobRunning), "startSeq", startSeq, "state", string(state)), nil
}

func (m *Manager) runExec(jobID string, session *Session, command string, timeout time.Duration) {
	state, _, _, active := session.Snapshot()
	if !active || state == Disconnected {
		m.settleJob(jobID, JobDisconnected, nil, nil)
		m.releaseLock(session.ID)
		return
	}
	if !strings.HasSuffix(command, "\n") && !strings.HasSuffix(command, "\r") {
		command += "\n"
	}
	if err := session.Write(command, true); err != nil {
		message := err.Error()
		m.settleJob(jobID, JobFailed, nil, &message)
		m.releaseLock(session.ID)
		return
	}
	state, timedOut := session.WaitForStable(timeout, false)
	m.touchJobState(jobID, string(state))
	if !timedOut {
		m.settleJob(jobID, JobDone, &state, nil)
		m.releaseLock(session.ID)
		return
	}
	m.settleJob(jobID, JobTimedOut, &state, nil)
	state, _ = session.WaitForStable(24*time.Hour, false)
	m.touchJobState(jobID, string(state))
	if state == Disconnected {
		m.settleJob(jobID, JobDisconnected, &state, nil)
	} else {
		m.settleJob(jobID, JobDone, &state, nil)
	}
	m.releaseLock(session.ID)
}

func (m *Manager) ExecStatus(jobID string, since int64, waitMS int) (protocol.Value, error) {
	if waitMS < 0 || waitMS > 120_000 {
		return nil, service.Invalid("waitMs must be between 0 and 120000")
	}
	job, err := m.waitJob(jobID, time.Duration(waitMS)*time.Millisecond, true)
	if err != nil {
		return nil, err
	}
	text, cursor := m.historyText(job.SessionID, since, 10_000, false)
	return protocol.Object("job", job.value(), "output", text, "cursor", cursor, "done", terminalJobStatus(job.Status)), nil
}

func (m *Manager) ExecCancel(jobID string) (protocol.Value, error) {
	m.jobsMu.Lock()
	job := m.jobs[jobID]
	if job == nil {
		m.jobsMu.Unlock()
		return nil, service.NotFound("Job not found")
	}
	if terminalJobStatus(job.Status) {
		value := job.value()
		m.jobsMu.Unlock()
		return value, nil
	}
	session := m.Get(job.SessionID)
	cancel := m.jobCancels[jobID]
	m.jobsMu.Unlock()
	if cancel != nil {
		cancel()
		if session != nil {
			_ = session.Write("\x03", true)
		}
		return protocol.Object("ok", true, "jobId", jobID, "status", string(JobCanceled)), nil
	}
	if session != nil {
		_ = session.Write("\x03", true)
	}
	m.settleJob(jobID, JobCanceled, nil, nil)
	m.jobsMu.Lock()
	value := m.jobs[jobID].value()
	m.jobsMu.Unlock()
	return value, nil
}

func (m *Manager) waitJob(jobID string, wait time.Duration, terminal bool) (*Job, error) {
	var timer *time.Timer
	var timeout <-chan time.Time
	if wait > 0 {
		timer = time.NewTimer(wait)
		timeout = timer.C
		defer timer.Stop()
	}
	for {
		m.jobsMu.Lock()
		job := m.jobs[jobID]
		if job == nil {
			m.jobsMu.Unlock()
			return nil, service.NotFound("Job not found")
		}
		if jobPredicate(job.Status, terminal) || wait <= 0 {
			copy := cloneJob(job)
			m.jobsMu.Unlock()
			return copy, nil
		}
		changed := job.changed
		m.jobsMu.Unlock()
		select {
		case <-changed:
		case <-timeout:
			m.jobsMu.Lock()
			copy := cloneJob(m.jobs[jobID])
			m.jobsMu.Unlock()
			return copy, nil
		}
	}
}

func (m *Manager) settleJob(id string, status JobStatus, state *State, errorText *string) {
	m.jobsMu.Lock()
	defer m.jobsMu.Unlock()
	job := m.jobs[id]
	if job == nil || terminalJobStatus(job.Status) || job.Status == status {
		return
	}
	job.Status = status
	if state != nil {
		text := string(*state)
		job.State = &text
	}
	if errorText != nil {
		job.Error = errorText
	}
	if terminalJobStatus(status) {
		ended := time.Now().UnixMilli()
		job.EndedAt = &ended
	}
	close(job.changed)
	job.changed = make(chan struct{})
}

func (m *Manager) touchJobState(id, state string) {
	m.jobsMu.Lock()
	if job := m.jobs[id]; job != nil {
		job.State = &state
		close(job.changed)
		job.changed = make(chan struct{})
	}
	m.jobsMu.Unlock()
}

func (m *Manager) releaseLock(sessionID string) {
	m.jobsMu.Lock()
	delete(m.locks, sessionID)
	m.jobsMu.Unlock()
}

func (m *Manager) lockBusyMessageLocked(sessionID string) string {
	var current *Job
	for _, job := range m.jobs {
		if job.SessionID == sessionID && (job.Status == JobRunning || job.Status == JobTimedOut) &&
			(current == nil || job.StartedAt > current.StartedAt) {
			current = job
		}
	}
	if current == nil {
		return "This session is performing another operation; try again later"
	}
	what := current.Kind + " operation"
	if current.Command != nil {
		command := *current.Command
		if len(command) > 80 {
			command = command[:80]
		}
		what = "exec: " + command
	}
	return fmt.Sprintf("This session is running a %s (job %s, status %s); poll 'session exec-status %s' or cancel with 'session exec-cancel %s'", what, current.ID, current.Status, current.ID, current.ID)
}

func (m *Manager) historyText(sessionID string, since int64, limit int, includeInternal bool) (string, int64) {
	m.flushHistory()
	chunks, err := m.db.History(sessionID, since, limit)
	if err != nil {
		return "", since
	}
	var output strings.Builder
	var outputs []store.HistoryChunk
	cursor := since
	for _, chunk := range chunks {
		cursor = chunk.Seq
		if chunk.Direction == "output" {
			outputs = append(outputs, chunk)
		}
	}
	if includeInternal {
		for _, chunk := range outputs {
			output.WriteString(chunk.DataRaw)
		}
	} else {
		output.WriteString(renderHistoryWithoutInternal(outputs, "\n"))
	}
	return StripANSI(output.String()), cursor
}

func (m *Manager) currentState(sessionID string) State {
	if session := m.Get(sessionID); session != nil {
		state, _, _, _ := session.Snapshot()
		return state
	}
	return Disconnected
}

func randomJobID(existing map[string]*Job) (string, error) {
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
		if existing[id.String()] == nil {
			return id.String(), nil
		}
	}
	return "", errors.New("unable to allocate a unique job ID")
}

func jobPredicate(status JobStatus, terminal bool) bool {
	if terminal {
		return terminalJobStatus(status)
	}
	return status != JobRunning
}

func terminalJobStatus(status JobStatus) bool {
	return status == JobDone || status == JobCanceled || status == JobDisconnected || status == JobFailed
}

func cloneJob(job *Job) *Job {
	copy := *job
	return &copy
}

func nullableString(value *string, fallback string) string {
	if value == nil {
		return fallback
	}
	return *value
}
