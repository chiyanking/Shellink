package sessions

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"shellink/internal/protocol"
	"shellink/internal/service"
)

const transferChunkSize = 384

type transferCodec string

const (
	codecBase64  transferCodec = "base64"
	codecOpenSSL transferCodec = "openssl"
	codecPython  transferCodec = "python3"
	codecXXD     transferCodec = "xxd"
)

type TextEdit struct {
	OldText string `json:"oldText"`
	NewText string `json:"newText"`
}

type transferResult struct {
	value protocol.Value
	data  []byte
}

func (m *Manager) Upload(id, remotePath string, data []byte, timeoutMS int, expectedSHA string) (protocol.Value, error) {
	path, err := validateRemotePath(remotePath)
	if err != nil {
		return nil, err
	}
	if len(data) > m.cfg.TransferMaxBytes {
		return nil, &service.AppError{Code: "PAYLOAD_TOO_LARGE", Message: fmt.Sprintf("File is too large (%d bytes); limit is %d bytes", len(data), m.cfg.TransferMaxBytes), Status: 413}
	}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	if expectedSHA != "" && !strings.EqualFold(strings.TrimSpace(expectedSHA), digest) {
		return nil, service.Invalid("Upload content does not match the sha256 parameter")
	}
	if timeoutMS <= 0 {
		timeoutMS = m.cfg.TransferTimeoutMS
	}
	result, err := m.runTransfer(id, "upload", path, timeoutMS, func(ctx context.Context, session *Session) (transferResult, error) {
		return m.performUpload(ctx, session, path, data, digest)
	})
	if err != nil {
		return nil, err
	}
	return result.value, nil
}

func (m *Manager) Download(id, remotePath string, timeoutMS int) (protocol.Value, error) {
	path, err := validateRemotePath(remotePath)
	if err != nil {
		return nil, err
	}
	if timeoutMS <= 0 {
		timeoutMS = m.cfg.TransferTimeoutMS
	}
	result, err := m.runTransfer(id, "download", path, timeoutMS, func(ctx context.Context, session *Session) (transferResult, error) {
		return m.performDownload(ctx, session, path)
	})
	if err != nil {
		return nil, err
	}
	return result.value, nil
}

func (m *Manager) Edit(id, remotePath string, edits []TextEdit, timeoutMS int) (protocol.Value, error) {
	path, err := validateRemotePath(remotePath)
	if err != nil {
		return nil, err
	}
	if len(edits) == 0 {
		return nil, service.Invalid("edits must be a non-empty array")
	}
	for i, edit := range edits {
		if edit.OldText == "" {
			return nil, service.Invalid(fmt.Sprintf("edits[%d].oldText must not be empty", i))
		}
	}
	if timeoutMS <= 0 {
		timeoutMS = m.cfg.EditTimeoutMS
	}
	result, err := m.runTransfer(id, "edit", path, timeoutMS, func(ctx context.Context, session *Session) (transferResult, error) {
		start := time.Now()
		original, _, err := m.downloadBytes(ctx, session, path)
		if err != nil {
			return transferResult{}, err
		}
		updated, replaced, err := replaceText(original, edits)
		if err != nil {
			return transferResult{}, err
		}
		sum := sha256.Sum256(updated)
		meta, err := m.uploadBytes(ctx, session, path, updated, hex.EncodeToString(sum[:]))
		if err != nil {
			return transferResult{}, err
		}
		return transferResult{value: protocol.Object("ok", true, "path", path, "engine", "go", "replaced", replaced,
			"durationMs", time.Since(start).Milliseconds(), "size", meta.size, "sha256", meta.sha256)}, nil
	})
	if err != nil {
		return nil, err
	}
	return result.value, nil
}

func (m *Manager) UploadStart(id, remotePath string, data []byte, timeoutMS int, expectedSHA string) (protocol.Value, error) {
	path, err := validateRemotePath(remotePath)
	if err != nil {
		return nil, err
	}
	if len(data) > m.cfg.TransferMaxBytes {
		return nil, &service.AppError{Code: "PAYLOAD_TOO_LARGE", Message: fmt.Sprintf("File is too large (%d bytes); limit is %d bytes", len(data), m.cfg.TransferMaxBytes), Status: 413}
	}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	if expectedSHA != "" && !strings.EqualFold(strings.TrimSpace(expectedSHA), digest) {
		return nil, service.Invalid("Upload content does not match the sha256 parameter")
	}
	if timeoutMS <= 0 {
		timeoutMS = m.cfg.TransferTimeoutMS
	}
	return m.startTransferJob(id, "upload", path, timeoutMS, func(ctx context.Context, session *Session) (transferResult, error) {
		return m.performUpload(ctx, session, path, data, digest)
	})
}

func (m *Manager) DownloadStart(id, remotePath, output string, timeoutMS int) (protocol.Value, error) {
	path, err := validateRemotePath(remotePath)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(output) == "" {
		return nil, service.Invalid("output must not be empty")
	}
	if timeoutMS <= 0 {
		timeoutMS = m.cfg.TransferTimeoutMS
	}
	return m.startTransferJob(id, "download", path, timeoutMS, func(ctx context.Context, session *Session) (transferResult, error) {
		result, err := m.performDownload(ctx, session, path)
		if err != nil {
			return transferResult{}, err
		}
		if err := os.MkdirAll(filepath.Dir(output), 0700); err != nil {
			return transferResult{}, transferAppError(err)
		}
		if err := os.WriteFile(output, result.data, 0600); err != nil {
			return transferResult{}, transferAppError(err)
		}
		fields := protocol.MapValue(result.value)
		fields["data"] = nil
		fields["output"] = output
		return transferResult{value: mapProtocolValue(fields)}, nil
	})
}

func (m *Manager) EditStart(id, remotePath string, edits []TextEdit, timeoutMS int) (protocol.Value, error) {
	path, err := validateRemotePath(remotePath)
	if err != nil {
		return nil, err
	}
	if len(edits) == 0 {
		return nil, service.Invalid("edits must be a non-empty array")
	}
	for i, edit := range edits {
		if edit.OldText == "" {
			return nil, service.Invalid(fmt.Sprintf("edits[%d].oldText must not be empty", i))
		}
	}
	if timeoutMS <= 0 {
		timeoutMS = m.cfg.EditTimeoutMS
	}
	return m.startTransferJob(id, "edit", path, timeoutMS, func(ctx context.Context, session *Session) (transferResult, error) {
		start := time.Now()
		original, _, err := m.downloadBytes(ctx, session, path)
		if err != nil {
			return transferResult{}, err
		}
		updated, replaced, err := replaceText(original, edits)
		if err != nil {
			return transferResult{}, err
		}
		sum := sha256.Sum256(updated)
		meta, err := m.uploadBytes(ctx, session, path, updated, hex.EncodeToString(sum[:]))
		if err != nil {
			return transferResult{}, err
		}
		return transferResult{value: protocol.Object("ok", true, "path", path, "engine", "go", "replaced", replaced,
			"durationMs", time.Since(start).Milliseconds(), "size", meta.size, "sha256", meta.sha256)}, nil
	})
}

func (m *Manager) runTransfer(id, kind, remotePath string, timeoutMS int, operation func(context.Context, *Session) (transferResult, error)) (transferResult, error) {
	session := m.Get(id)
	if session == nil {
		return transferResult{}, service.NotFound("Session not found or disconnected")
	}
	if err := m.claimSessionOperation(id, kind); err != nil {
		return transferResult{}, err
	}
	defer m.releaseLock(id)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMS)*time.Millisecond)
	defer cancel()
	return m.performTransfer(ctx, session, operation)
}

func (m *Manager) startTransferJob(id, kind, remotePath string, timeoutMS int, operation func(context.Context, *Session) (transferResult, error)) (protocol.Value, error) {
	session := m.Get(id)
	if session == nil {
		return nil, service.NotFound("Session not found or disconnected")
	}
	m.jobsMu.Lock()
	if m.locks[id] != "" {
		message := m.lockBusyMessageLocked(id)
		m.jobsMu.Unlock()
		return nil, service.Conflict(message)
	}
	state, _, seq, active := session.Snapshot()
	if !active || state != WaitingInput {
		m.jobsMu.Unlock()
		return nil, service.Conflict("Files can be transferred only while WAITING_INPUT")
	}
	jobID, err := randomJobID(m.jobs)
	if err != nil {
		m.jobsMu.Unlock()
		return nil, service.Internal(err)
	}
	pathCopy := remotePath
	job := &Job{ID: jobID, SessionID: id, Kind: kind, Remote: &pathCopy, StartSeq: seq,
		StartedAt: time.Now().UnixMilli(), TimeoutMS: timeoutMS, Status: JobRunning, changed: make(chan struct{})}
	m.jobs[jobID] = job
	m.locks[id] = kind
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMS)*time.Millisecond)
	m.jobCancels[jobID] = cancel
	m.jobsMu.Unlock()
	go func() {
		result, runErr := m.performTransfer(ctx, session, operation)
		cancel()
		state, _, _, active := session.Snapshot()
		var statePtr *State
		if active {
			statePtr = &state
		}
		if runErr != nil {
			message := runErr.Error()
			status := JobFailed
			if errors.Is(ctx.Err(), context.Canceled) {
				status = JobCanceled
			} else if !active {
				status = JobDisconnected
			}
			m.settleJob(jobID, status, statePtr, &message)
		} else {
			m.jobsMu.Lock()
			if current := m.jobs[jobID]; current != nil && !terminalJobStatus(current.Status) {
				current.Result = result.value
			}
			m.jobsMu.Unlock()
			m.settleJob(jobID, JobDone, statePtr, nil)
		}
		m.jobsMu.Lock()
		delete(m.jobCancels, jobID)
		m.jobsMu.Unlock()
		m.releaseLock(id)
	}()
	return job.value(), nil
}

func (m *Manager) claimSessionOperation(id, kind string) error {
	session := m.Get(id)
	if session == nil {
		return service.NotFound("Session not found or disconnected")
	}
	state, mode, _, active := session.Snapshot()
	if !active || state == Disconnected {
		return service.NotFound("Session not found or disconnected")
	}
	if mode == Manual {
		return service.Conflict("Session is in MANUAL mode; file operation was rejected")
	}
	if state != WaitingInput {
		return service.Conflict("Session state is " + string(state) + "; files can be transferred only while WAITING_INPUT")
	}
	m.jobsMu.Lock()
	defer m.jobsMu.Unlock()
	if m.locks[id] != "" {
		return service.Conflict(m.lockBusyMessageLocked(id))
	}
	m.locks[id] = kind
	return nil
}

func (m *Manager) performTransfer(ctx context.Context, session *Session, operation func(context.Context, *Session) (transferResult, error)) (transferResult, error) {
	session.BeginInternal()
	result, err := operation(ctx, session)
	if err != nil {
		m.recoverTransfer(session)
	}
	session.EndInternal()
	if session.IsInternal() {
		return result, err
	}
	_, _, _, active := session.Snapshot()
	if active && !session.IsClosed() {
		if err == nil {
			_ = session.Write("\x15", false)
			m.finishTransferPrompt(session, true)
		} else {
			m.finishTransferPrompt(session, true)
		}
		session.EmitDisplayOutput("\r\n")
	}
	return result, err
}

func (m *Manager) finishTransferPrompt(session *Session, restoreEcho bool) {
	marker := "SP_END_" + randomToken(8)
	command := ":"
	if restoreEcho {
		command = "stty echo 2>/dev/null || true"
	}
	command += "; printf '\\n%s\\n' '" + marker + "'"
	seq := session.LastSequence()
	_ = session.Write(command+"\n", true)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _ = m.waitOutputMarker(ctx, session, seq, regexp.MustCompile(`(?m)^`+regexp.QuoteMeta(marker)+`$`))
	_ = m.waitPrompt(ctx, session)
}

func (m *Manager) recoverTransfer(session *Session) {
	state, _, _, active := session.Snapshot()
	if !active || state == Disconnected {
		return
	}
	_ = session.Write("\x03", false)
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	_ = m.waitPrompt(ctx, session)
	marker := "SP_REC_" + randomToken(6)
	seq := session.LastSequence()
	_ = session.Write("stty echo 2>/dev/null || true; printf '\\n%s\\n' '"+marker+"'\n", false)
	_, _ = m.waitOutputMarker(ctx, session, seq, regexp.MustCompile(`(?m)^`+regexp.QuoteMeta(marker)+`$`))
}

type uploadMeta struct {
	size   int
	sha256 string
	codec  string
}

func (m *Manager) performUpload(ctx context.Context, session *Session, remotePath string, data []byte, digest string) (transferResult, error) {
	started := time.Now()
	meta, err := m.uploadBytes(ctx, session, remotePath, data, digest)
	if err != nil {
		return transferResult{}, err
	}
	return transferResult{value: protocol.Object("ok", true, "remotePath", remotePath, "size", meta.size,
		"sha256", meta.sha256, "codec", meta.codec, "durationMs", time.Since(started).Milliseconds())}, nil
}

func (m *Manager) uploadBytes(ctx context.Context, session *Session, remotePath string, data []byte, digest string) (uploadMeta, error) {
	if err := checkTransferReady(session); err != nil {
		return uploadMeta{}, err
	}
	codec, err := m.probeTransferCodec(ctx, session)
	if err != nil {
		return uploadMeta{}, err
	}
	token := randomToken(8)
	encoded := encodeTransferData(codec, data)
	staged := "/tmp/.shellink-" + token + ".b64"
	targetTmp := "/tmp/.shellink-" + token + ".tmp"
	qStage, qTargetTmp, qPath := shellQuote(staged), shellQuote(targetTmp), shellQuote(remotePath)
	if _, err := m.commandMarker(ctx, session, ": > "+qStage+"; stty -echo 2>/dev/null || true", regexp.MustCompile(`(?m)^SP_READY$`), "; printf '\\nSP_READY\\n'"); err != nil {
		return uploadMeta{}, err
	}
	for offset, seq := 0, 0; offset < len(encoded); seq++ {
		end := min(offset+transferChunkSize, len(encoded))
		chunk := encoded[offset:end]
		offset = end
		marker := fmt.Sprintf("SP_U_%s_%d", token, seq)
		cmd := "printf '%s' '" + chunk + "' >> " + qStage + "; printf '\\n%s%s\\n' '" + marker[:len(marker)/2] + "' '" + marker[len(marker)/2:] + "'"
		if _, err := m.writeAndWaitMarker(ctx, session, cmd, regexp.MustCompile(`(?m)^`+regexp.QuoteMeta(marker)+`$`)); err != nil {
			return uploadMeta{}, transferFailure("Upload staging timed out", err)
		}
	}
	checkMarker := "SP_U_SIZE_" + token
	checkCommand := "printf '\\n%s%s:%s\\n' '" + checkMarker[:len(checkMarker)/2] + "' '" + checkMarker[len(checkMarker)/2:] + "' \"$(wc -c < " + qStage + " | tr -d ' ')\""
	checkOutput, err := m.commandMarker(ctx, session, checkCommand, regexp.MustCompile(`(?m)^`+regexp.QuoteMeta(checkMarker)+`:(\d+)$`), "")
	if err != nil {
		return uploadMeta{}, err
	}
	count := lastCapture(checkOutput, regexp.MustCompile(`(?m)^`+regexp.QuoteMeta(checkMarker)+`:(\d+)$`), 1)
	if count != fmt.Sprint(len(encoded)) {
		return uploadMeta{}, &service.AppError{Code: "TRANSFER_ERROR", Message: "Upload staging is corrupted; retry the transfer", Status: 502}
	}
	finalMarker := "SP_UP_" + token
	final := decodeTransferFile(codec, staged, targetTmp) + "; rm -f " + qStage + "; mv -f " + qTargetTmp + " " + qPath + " 2>/dev/null || { cp " + qTargetTmp + " " + qPath + " && rm -f " + qTargetTmp + "; }; printf '\\n%s%s:%s\\n' '" + finalMarker[:len(finalMarker)/2] + "' '" + finalMarker[len(finalMarker)/2:] + "' \"$(wc -c < " + qPath + " | tr -d ' ')\""
	finalOutput, err := m.commandMarker(ctx, session, final, regexp.MustCompile(`(?m)^`+regexp.QuoteMeta(finalMarker)+`:(\d+)$`), "")
	if err != nil {
		return uploadMeta{}, transferFailure("Upload decoding timed out", err)
	}
	remoteSize := lastCapture(finalOutput, regexp.MustCompile(`(?m)^`+regexp.QuoteMeta(finalMarker)+`:(\d+)$`), 1)
	if remoteSize != fmt.Sprint(len(data)) {
		return uploadMeta{}, &service.AppError{Code: "TRANSFER_ERROR", Message: fmt.Sprintf("Uploaded size mismatch: expected %d, remote size is %s", len(data), remoteSize), Status: 502}
	}
	return uploadMeta{size: len(data), sha256: digest, codec: string(codec)}, nil
}

func (m *Manager) probeTransferCodec(ctx context.Context, session *Session) (transferCodec, error) {
	marker := "SP_CODEC_" + randomToken(6)
	command := "if command -v base64 >/dev/null 2>&1; then v=base64; elif command -v openssl >/dev/null 2>&1; then v=openssl; elif command -v python3 >/dev/null 2>&1; then v=python3; elif command -v xxd >/dev/null 2>&1; then v=xxd; else v=none; fi; printf '\\n%s%s:%s\\n' '" + marker[:len(marker)/2] + "' '" + marker[len(marker)/2:] + "' \"$v\""
	output, err := m.commandMarker(ctx, session, command, regexp.MustCompile(`(?m)^`+regexp.QuoteMeta(marker)+`:(base64|openssl|python3|xxd|none)$`), "")
	if err != nil {
		return "", err
	}
	name := lastCapture(output, regexp.MustCompile(`(?m)^`+regexp.QuoteMeta(marker)+`:(base64|openssl|python3|xxd|none)$`), 1)
	if name == "none" || name == "" {
		return "", &service.AppError{Code: "TRANSFER_ERROR", Message: "No remote file codec is available (requires base64, openssl, python3, or xxd)", Status: 502}
	}
	return transferCodec(name), nil
}

func encodeTransferData(codec transferCodec, data []byte) string {
	if codec == codecXXD {
		return hex.EncodeToString(data)
	}
	return base64.StdEncoding.EncodeToString(data)
}

func encodeTransferFile(codec transferCodec, quotedPath, rawPath string) string {
	switch codec {
	case codecOpenSSL:
		return "openssl base64 -A -in " + quotedPath
	case codecPython:
		pathJSON, _ := json.Marshal(rawPath)
		script := "import sys,base64;sys.stdout.buffer.write(base64.b64encode(open(" + string(pathJSON) + ", 'rb').read()))"
		return "python3 -c " + shellQuote(script)
	case codecXXD:
		return "xxd -p " + quotedPath
	default:
		return "base64 < " + quotedPath
	}
}

func decodeTransferFile(codec transferCodec, quotedSource, quotedDestination string) string {
	quotedSource, quotedDestination = shellQuote(quotedSource), shellQuote(quotedDestination)
	switch codec {
	case codecOpenSSL:
		return "openssl base64 -d -A -in " + quotedSource + " -out " + quotedDestination
	case codecPython:
		pathSource, _ := json.Marshal(quotedSource)
		pathDestination, _ := json.Marshal(quotedDestination)
		script := "import base64;open(" + string(pathDestination) + ", 'wb').write(base64.b64decode(open(" + string(pathSource) + ", 'rb').read()))"
		return "python3 -c " + shellQuote(script)
	case codecXXD:
		return "xxd -r -p < " + quotedSource + " > " + quotedDestination
	default:
		return "base64 -d < " + quotedSource + " > " + quotedDestination + " 2>/dev/null || base64 -D < " + quotedSource + " > " + quotedDestination
	}
}

func (m *Manager) performDownload(ctx context.Context, session *Session, remotePath string) (transferResult, error) {
	started := time.Now()
	data, codec, err := m.downloadBytes(ctx, session, remotePath)
	if err != nil {
		return transferResult{}, err
	}
	sum := sha256.Sum256(data)
	return transferResult{data: data, value: protocol.Object("remotePath", remotePath, "data", data, "size", len(data),
		"sha256", hex.EncodeToString(sum[:]), "codec", string(codec), "durationMs", time.Since(started).Milliseconds())}, nil
}

func (m *Manager) downloadBytes(ctx context.Context, session *Session, remotePath string) ([]byte, transferCodec, error) {
	if err := checkTransferReady(session); err != nil {
		return nil, "", err
	}
	codec, err := m.probeTransferCodec(ctx, session)
	if err != nil {
		return nil, "", err
	}
	token := randomToken(8)
	qPath := shellQuote(remotePath)
	statMarker := "SP_STAT_" + token
	statCommand := "if [ ! -f " + qPath + " ]; then v=missing; elif [ ! -r " + qPath + " ]; then v=unreadable; else v=ok:$(wc -c < " + qPath + " | tr -d ' '); fi; printf '\\n%s%s:%s\\n' '" + statMarker[:len(statMarker)/2] + "' '" + statMarker[len(statMarker)/2:] + "' \"$v\""
	statOutput, err := m.commandMarker(ctx, session, statCommand, regexp.MustCompile(`(?m)^`+regexp.QuoteMeta(statMarker)+`:(missing|unreadable|ok:(\d+))$`), "")
	if err != nil {
		return nil, "", err
	}
	status := lastCapture(statOutput, regexp.MustCompile(`(?m)^`+regexp.QuoteMeta(statMarker)+`:(missing|unreadable|ok:(\d+))$`), 1)
	if status == "missing" {
		return nil, "", &service.AppError{Code: "NOT_FOUND", Message: "Remote file not found", Status: 404}
	}
	if status == "unreadable" {
		return nil, "", &service.AppError{Code: "TRANSFER_ERROR", Message: "Remote file is unreadable", Status: 502}
	}
	var remoteSize int
	if _, err := fmt.Sscanf(strings.TrimPrefix(status, "ok:"), "%d", &remoteSize); err != nil || remoteSize < 0 {
		return nil, "", &service.AppError{Code: "TRANSFER_ERROR", Message: "Unable to parse the remote file size", Status: 502}
	}
	if remoteSize > m.cfg.TransferMaxBytes {
		return nil, "", &service.AppError{Code: "PAYLOAD_TOO_LARGE", Message: fmt.Sprintf("File is too large (%d bytes); limit is %d bytes", remoteSize, m.cfg.TransferMaxBytes), Status: 413}
	}
	begin, end := "SPB_"+token, "SPE_"+token
	command := "printf '%s\\n' '" + begin + "'; " + encodeTransferFile(codec, qPath, remotePath) + "; printf '\\n%s\\n' '" + end + "'"
	output, err := m.commandFrame(ctx, session, command, begin, end)
	if err != nil {
		return nil, "", err
	}
	payload, err := extractMarkedPayload(output, begin, end)
	if err != nil {
		return nil, "", err
	}
	compact := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\r' || r == '\n' {
			return -1
		}
		return r
	}, payload)
	var data []byte
	if codec == codecXXD {
		data, err = hex.DecodeString(compact)
	} else {
		data, err = base64.StdEncoding.DecodeString(compact)
	}
	if err != nil {
		return nil, "", &service.AppError{Code: "TRANSFER_ERROR", Message: "Unable to decode remote data", Status: 502}
	}
	if len(data) != remoteSize {
		return nil, "", &service.AppError{Code: "TRANSFER_ERROR", Message: fmt.Sprintf("Downloaded size mismatch: expected %d, got %d", remoteSize, len(data)), Status: 502}
	}
	return data, codec, nil
}

func (m *Manager) commandMarker(ctx context.Context, session *Session, command string, marker *regexp.Regexp, suffix string) (string, error) {
	seq := session.LastSequence()
	if err := session.Write(command+suffix+"\n", true); err != nil {
		return "", service.Conflict(err.Error())
	}
	output, err := m.waitOutputMarker(ctx, session, seq, marker)
	if err != nil {
		return output, err
	}
	if err := m.waitPrompt(ctx, session); err != nil {
		return output, err
	}
	return output, nil
}

func (m *Manager) writeAndWaitMarker(ctx context.Context, session *Session, command string, marker *regexp.Regexp) (string, error) {
	seq := session.LastSequence()
	if err := session.Write(command+"\n", true); err != nil {
		return "", service.Conflict(err.Error())
	}
	return m.waitOutputMarker(ctx, session, seq, marker)
}

func (m *Manager) commandFrame(ctx context.Context, session *Session, command, begin, end string) (string, error) {
	seq := session.LastSequence()
	if err := session.Write(command+"\n", true); err != nil {
		return "", service.Conflict(err.Error())
	}
	frame := regexp.MustCompile(`(?s)(?:^|\n)` + regexp.QuoteMeta(begin) + `\r?\n.*?\r?\n` + regexp.QuoteMeta(end) + `(?:\r?\n|$)`)
	output, err := m.waitOutputMarker(ctx, session, seq, frame)
	if err != nil {
		return output, err
	}
	if err := m.waitPrompt(ctx, session); err != nil {
		return output, err
	}
	return output, nil
}

func (m *Manager) waitOutputMarker(ctx context.Context, session *Session, since int64, marker *regexp.Regexp) (string, error) {
	ticker := time.NewTicker(15 * time.Millisecond)
	defer ticker.Stop()
	for {
		output, _ := m.historyText(session.ID, since, 100_000, true)
		if marker.MatchString(output) {
			return output, nil
		}
		state, _, _, active := session.Snapshot()
		if !active || state == Disconnected {
			return output, &service.AppError{Code: "TRANSFER_ERROR", Message: "Session disconnected during file operation", Status: 502}
		}
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.Canceled) {
				return output, &service.AppError{Code: "CANCELED", Message: "File operation canceled", Status: 499}
			}
			return output, &service.AppError{Code: "TIMEOUT", Message: "File operation timed out", Status: 504}
		case <-ticker.C:
		}
	}
}

func (m *Manager) waitPrompt(ctx context.Context, session *Session) error {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		state, _, _, active := session.Snapshot()
		if !active || state == Disconnected {
			return &service.AppError{Code: "TRANSFER_ERROR", Message: "Session disconnected during file operation", Status: 502}
		}
		if state == WaitingInput {
			return nil
		}
		select {
		case <-ctx.Done():
			return &service.AppError{Code: "TIMEOUT", Message: "File operation timed out", Status: 504}
		case <-ticker.C:
		}
	}
}

func checkTransferReady(session *Session) error {
	state, mode, _, active := session.Snapshot()
	if !active || state == Disconnected {
		return service.NotFound("Session not found or disconnected")
	}
	if mode == Manual {
		return service.Conflict("Session is in MANUAL mode; file operation was rejected")
	}
	if state != WaitingInput {
		return service.Conflict("Session state is " + string(state) + "; files can be transferred only while WAITING_INPUT")
	}
	return nil
}

func validateRemotePath(value string) (string, error) {
	path := strings.TrimSpace(value)
	if path == "" || strings.ContainsRune(path, 0) {
		return "", service.Invalid("path must be a non-empty string without NUL")
	}
	if len(path) > 4096 {
		return "", service.Invalid("path is too long")
	}
	return path, nil
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }

func extractMarkedPayload(output, begin, end string) (string, error) {
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	lastBegin, lastEnd := -1, -1
	for i, line := range lines {
		if line == begin {
			lastBegin = i
		}
		if line == end && lastBegin >= 0 && i > lastBegin {
			lastEnd = i
		}
	}
	if lastBegin < 0 || lastEnd <= lastBegin {
		return "", &service.AppError{Code: "TRANSFER_ERROR", Message: "Transfer marker frame was not found (remote output may be truncated)", Status: 502}
	}
	return strings.Join(lines[lastBegin+1:lastEnd], "\n"), nil
}

func lastCapture(value string, pattern *regexp.Regexp, index int) string {
	matches := pattern.FindAllStringSubmatch(value, -1)
	if len(matches) == 0 || index >= len(matches[len(matches)-1]) {
		return ""
	}
	return matches[len(matches)-1][index]
}

func randomToken(size int) string {
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(data)
}

func replaceText(data []byte, edits []TextEdit) ([]byte, int, error) {
	if !utf8.Valid(data) {
		return nil, 0, &service.AppError{Code: "TRANSFER_ERROR", Message: "File is not valid UTF-8", Status: 502}
	}
	bom := []byte(nil)
	if strings.HasPrefix(string(data), "\xef\xbb\xbf") {
		bom, data = data[:3], data[3:]
	}
	text := string(data)
	ending := "\n"
	if strings.Contains(text, "\r\n") {
		ending = "\r\n"
	} else if strings.Contains(text, "\r") && !strings.Contains(text, "\n") {
		ending = "\r"
	}
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	type replacement struct {
		start, end int
		text       string
		index      int
	}
	items := make([]replacement, 0, len(edits))
	for i, edit := range edits {
		old := strings.ReplaceAll(strings.ReplaceAll(edit.OldText, "\r\n", "\n"), "\r", "\n")
		newText := strings.ReplaceAll(strings.ReplaceAll(edit.NewText, "\r\n", "\n"), "\r", "\n")
		count := strings.Count(text, old)
		if count == 0 {
			return nil, 0, service.Invalid(fmt.Sprintf("Could not find edits[%d]; oldText must match exactly including whitespace", i))
		}
		if count > 1 {
			return nil, 0, service.Invalid(fmt.Sprintf("Found %d occurrences of edits[%d]; each oldText must be unique", count, i))
		}
		start := strings.Index(text, old)
		items = append(items, replacement{start: start, end: start + len(old), text: newText, index: i})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].start < items[j].start })
	for i := 1; i < len(items); i++ {
		if items[i-1].end > items[i].start {
			return nil, 0, service.Invalid(fmt.Sprintf("edits[%d] and edits[%d] overlap", items[i-1].index, items[i].index))
		}
	}
	for i := len(items) - 1; i >= 0; i-- {
		item := items[i]
		text = text[:item.start] + item.text + text[item.end:]
	}
	if text == strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", "\n"), "\r", "\n") {
		return nil, 0, service.Invalid("No changes made; replacement produced identical content")
	}
	if ending != "\n" {
		text = strings.ReplaceAll(text, "\n", ending)
	}
	return append(bom, []byte(text)...), len(items), nil
}

func transferFailure(message string, err error) error {
	var app *service.AppError
	if errors.As(err, &app) {
		return app
	}
	return &service.AppError{Code: "TRANSFER_ERROR", Message: message + ": " + err.Error(), Status: 502}
}

func transferAppError(err error) error {
	return &service.AppError{Code: "TRANSFER_ERROR", Message: err.Error(), Status: 500}
}

func mapProtocolValue(fields map[string]protocol.Value) protocol.Value {
	values := make([]protocol.KV, 0, len(fields))
	for key, value := range fields {
		values = append(values, protocol.KV{Key: key, Val: value})
	}
	return values
}

func DecodeEdits(value protocol.Value) ([]TextEdit, error) {
	items, ok := value.([]protocol.Value)
	if !ok || len(items) == 0 {
		return nil, service.Invalid("edits must be a non-empty array")
	}
	edits := make([]TextEdit, 0, len(items))
	for i, item := range items {
		fields := protocol.MapValue(item)
		oldText, oldOK := protocol.StringValue(fields["oldText"])
		newText, newOK := protocol.StringValue(fields["newText"])
		if !oldOK || !newOK {
			return nil, service.Invalid(fmt.Sprintf("edits[%d] oldText and newText must be strings", i))
		}
		edits = append(edits, TextEdit{OldText: oldText, NewText: newText})
	}
	return edits, nil
}
