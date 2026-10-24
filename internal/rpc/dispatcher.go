package rpc

import (
	"errors"
	"fmt"
	"os"
	"time"

	"shellink/internal/config"
	"shellink/internal/protocol"
	"shellink/internal/service"
	"shellink/internal/sessions"
	"shellink/internal/store"
	"shellink/internal/webhooks"
)

const ProtocolVersion = 1

var Version = "0.2.4"
var Commit = "unknown"

var Methods = []string{
	"system.hello", "system.ping", "system.status", "system.stop",
	"profiles.list", "profiles.get", "profiles.create", "profiles.update", "profiles.delete",
	"sessions.list", "sessions.create", "sessions.state", "sessions.history", "sessions.input",
	"sessions.exec", "sessions.execStart", "sessions.execStatus", "sessions.execCancel",
	"sessions.uploadStart", "sessions.downloadStart", "sessions.editStart", "sessions.download",
	"sessions.upload", "sessions.edit", "sessions.mode", "sessions.resize", "sessions.close",
	"sessions.removeRecord", "webhooks.list", "webhooks.create", "webhooks.delete",
	"events.subscribe", "events.unsubscribe",
}

type EventSink interface {
	AddSubscription(sessionID *string, replay bool) (protocol.Value, error)
	RemoveSubscription(id string) bool
}

type Dispatcher struct {
	cfg      config.Config
	db       *store.Store
	profiles *service.Profiles
	sessions *sessions.Manager
	webhooks *webhooks.Service
	started  time.Time
	stop     func()
}

func NewDispatcher(cfg config.Config, db *store.Store, profiles *service.Profiles, manager *sessions.Manager, hooks *webhooks.Service, stop func()) *Dispatcher {
	return &Dispatcher{cfg: cfg, db: db, profiles: profiles, sessions: manager, webhooks: hooks, started: time.Now(), stop: stop}
}

func (d *Dispatcher) Dispatch(method string, params protocol.Value, sink EventSink) (protocol.Value, error) {
	fields := protocol.MapValue(params)
	switch method {
	case "system.hello":
		version, ok := integer(fields, "protocolVersion", 0)
		if !ok {
			return nil, service.Invalid("protocolVersion must be an integer")
		}
		if version != ProtocolVersion {
			return nil, &service.AppError{Code: "PROTOCOL_ERROR", Message: fmt.Sprintf("Unsupported protocol version: %d", version), Status: 400}
		}
		caps := make([]protocol.Value, len(Methods))
		for i, name := range Methods {
			caps[i] = name
		}
		return protocol.Object("serviceVersion", Version, "serviceCommit", Commit,
			"protocolVersion", ProtocolVersion, "capabilities", caps), nil
	case "system.ping":
		return protocol.Object("pong", true, "at", time.Now().UnixMilli()), nil
	case "system.status":
		list, err := d.sessions.List()
		if err != nil {
			return nil, err
		}
		active := 0
		if values, ok := list.([]protocol.Value); ok {
			for _, item := range values {
				if value, yes := protocol.MapValue(item)["active"].(bool); yes && value {
					active++
				}
			}
		}
		host := d.cfg.Host
		url := fmt.Sprintf("http://%s:%d", host, d.cfg.Port)
		var httpURL, ui protocol.Value
		if d.cfg.HTTPEnabled {
			httpURL, ui = url, protocol.Object("url", webUIURL(host, d.cfg.Port))
		}
		socketPath := d.cfg.SocketPath
		if d.cfg.PipeName != "" {
			socketPath = d.cfg.PipeName
		}
		return protocol.Object("version", Version, "commit", Commit, "protocolVersion", ProtocolVersion,
			"pid", os.Getpid(), "uptimeSeconds", int64(time.Since(d.started).Seconds()),
			"socketPath", socketPath, "httpEnabled", d.cfg.HTTPEnabled, "httpUrl", httpURL,
			"webui", ui, "activeSessions", active), nil
	case "system.stop":
		go d.stop()
		return protocol.Object("stopping", true), nil
	case "profiles.list":
		return d.profiles.List(params)
	case "profiles.get":
		id, err := requiredString(fields, "id")
		if err != nil {
			return nil, err
		}
		return d.profiles.Get(id)
	case "profiles.create":
		return d.profiles.Create(params)
	case "profiles.update":
		id, err := requiredString(fields, "id")
		if err != nil {
			return nil, err
		}
		profile, ok := fields["profile"]
		if !ok {
			return nil, service.Invalid("profile is required")
		}
		return d.profiles.Update(id, profile)
	case "profiles.delete":
		id, err := requiredString(fields, "id")
		if err != nil {
			return nil, err
		}
		if err := d.profiles.Delete(id); err != nil {
			return nil, err
		}
		return protocol.Object("ok", true), nil
	case "sessions.list":
		return d.sessions.List()
	case "sessions.create":
		profileID, err := requiredString(fields, "profileId")
		if err != nil {
			return nil, err
		}
		cols, ok := optionalInteger(fields, "cols", 0)
		if !ok {
			return nil, service.Invalid("cols must be an integer")
		}
		rows, ok := optionalInteger(fields, "rows", 0)
		if !ok {
			return nil, service.Invalid("rows must be an integer")
		}
		return d.sessions.Create(profileID, int(cols), int(rows))
	case "sessions.state":
		id, err := requiredString(fields, "id")
		if err != nil {
			return nil, err
		}
		return d.sessions.State(id)
	case "sessions.history":
		id, err := requiredString(fields, "id")
		if err != nil {
			return nil, err
		}
		since, ok := optionalInteger(fields, "since", 0)
		if !ok || since < 0 {
			return nil, service.Invalid("since must be a non-negative integer")
		}
		limit, ok := optionalInteger(fields, "limit", 2000)
		if !ok || limit < 1 || limit > 10_000 {
			return nil, service.Invalid("limit must be between 1 and 10000")
		}
		include := false
		if raw, present := fields["includeInternal"]; present {
			var valid bool
			include, valid = protocol.BoolValue(raw)
			if !valid {
				return nil, service.Invalid("includeInternal must be a boolean")
			}
		}
		return d.sessions.History(id, since, int(limit), include)
	case "sessions.input":
		id, err := requiredString(fields, "id")
		if err != nil {
			return nil, err
		}
		text, ok := protocol.StringValue(fields["text"])
		if !ok {
			return nil, service.Invalid("text must be a string")
		}
		appendNewline := true
		if raw, present := fields["appendNewline"]; present {
			appendNewline, ok = protocol.BoolValue(raw)
			if !ok {
				return nil, service.Invalid("appendNewline must be a boolean")
			}
		}
		manual := false
		if raw, present := fields["manual"]; present {
			manual, ok = protocol.BoolValue(raw)
			if !ok {
				return nil, service.Invalid("manual must be a boolean")
			}
		}
		return d.sessions.Input(id, text, appendNewline, manual)
	case "sessions.exec":
		id, err := requiredString(fields, "id")
		if err != nil {
			return nil, err
		}
		command, err := requiredString(fields, "command")
		if err != nil {
			return nil, err
		}
		timeout, ok := optionalInteger(fields, "timeoutMs", int64(d.cfg.ExecDefaultTimeoutMS))
		if !ok {
			return nil, service.Invalid("timeoutMs must be an integer")
		}
		return d.sessions.Exec(id, command, int(timeout))
	case "sessions.execStart":
		id, err := requiredString(fields, "id")
		if err != nil {
			return nil, err
		}
		command, err := requiredString(fields, "command")
		if err != nil {
			return nil, err
		}
		timeout, ok := optionalInteger(fields, "timeoutMs", int64(d.cfg.ExecDefaultTimeoutMS))
		if !ok {
			return nil, service.Invalid("timeoutMs must be an integer")
		}
		return d.sessions.ExecStart(id, command, int(timeout))
	case "sessions.execStatus":
		id, err := requiredString(fields, "jobId")
		if err != nil {
			return nil, err
		}
		since, ok := optionalInteger(fields, "since", 0)
		if !ok {
			return nil, service.Invalid("since must be an integer")
		}
		wait, ok := optionalInteger(fields, "waitMs", 0)
		if !ok {
			return nil, service.Invalid("waitMs must be an integer")
		}
		return d.sessions.ExecStatus(id, since, int(wait))
	case "sessions.execCancel":
		id, err := requiredString(fields, "jobId")
		if err != nil {
			return nil, err
		}
		return d.sessions.ExecCancel(id)
	case "sessions.mode":
		id, err := requiredString(fields, "id")
		if err != nil {
			return nil, err
		}
		mode, err := requiredString(fields, "mode")
		if err != nil {
			return nil, err
		}
		return d.sessions.Mode(id, mode)
	case "sessions.resize":
		id, err := requiredString(fields, "id")
		if err != nil {
			return nil, err
		}
		cols, ok := optionalInteger(fields, "cols", 0)
		if !ok {
			return nil, service.Invalid("cols must be an integer")
		}
		rows, ok := optionalInteger(fields, "rows", 0)
		if !ok {
			return nil, service.Invalid("rows must be an integer")
		}
		if err := d.sessions.Resize(id, int(cols), int(rows)); err != nil {
			return nil, normalizeError(err)
		}
		return protocol.Object("ok", true), nil
	case "sessions.close":
		id, err := requiredString(fields, "id")
		if err != nil {
			return nil, err
		}
		return d.sessions.Close(id)
	case "sessions.removeRecord":
		id, err := requiredString(fields, "id")
		if err != nil {
			return nil, err
		}
		return d.sessions.Remove(id)
	case "sessions.upload", "sessions.uploadStart":
		id, err := requiredString(fields, "id")
		if err != nil {
			return nil, err
		}
		path, err := requiredString(fields, "path")
		if err != nil {
			return nil, err
		}
		data, ok := fields["data"].([]byte)
		if !ok {
			return nil, service.Invalid("data must be a byte array")
		}
		timeout, ok := optionalInteger(fields, "timeoutMs", int64(d.cfg.TransferTimeoutMS))
		if !ok {
			return nil, service.Invalid("timeoutMs must be an integer")
		}
		sha, _ := protocol.StringValue(fields["sha256"])
		if method == "sessions.uploadStart" {
			return d.sessions.UploadStart(id, path, data, int(timeout), sha)
		}
		return d.sessions.Upload(id, path, data, int(timeout), sha)
	case "sessions.download", "sessions.downloadStart":
		id, err := requiredString(fields, "id")
		if err != nil {
			return nil, err
		}
		path, err := requiredString(fields, "path")
		if err != nil {
			return nil, err
		}
		timeout, ok := optionalInteger(fields, "timeoutMs", int64(d.cfg.TransferTimeoutMS))
		if !ok {
			return nil, service.Invalid("timeoutMs must be an integer")
		}
		if method == "sessions.downloadStart" {
			output, err := requiredString(fields, "output")
			if err != nil {
				return nil, err
			}
			return d.sessions.DownloadStart(id, path, output, int(timeout))
		}
		return d.sessions.Download(id, path, int(timeout))
	case "sessions.edit", "sessions.editStart":
		id, err := requiredString(fields, "id")
		if err != nil {
			return nil, err
		}
		path, err := requiredString(fields, "path")
		if err != nil {
			return nil, err
		}
		edits, editErr := sessions.DecodeEdits(fields["edits"])
		if editErr != nil {
			return nil, editErr
		}
		timeout, ok := optionalInteger(fields, "timeoutMs", int64(d.cfg.EditTimeoutMS))
		if !ok {
			return nil, service.Invalid("timeoutMs must be an integer")
		}
		if method == "sessions.editStart" {
			return d.sessions.EditStart(id, path, edits, int(timeout))
		}
		return d.sessions.Edit(id, path, edits, int(timeout))
	case "webhooks.list":
		return d.webhooks.List()
	case "webhooks.create":
		return d.webhooks.Create(params)
	case "webhooks.delete":
		id, err := requiredString(fields, "id")
		if err != nil {
			return nil, err
		}
		if err := d.webhooks.Delete(id); err != nil {
			return nil, err
		}
		return protocol.Object("ok", true), nil
	case "events.subscribe":
		if sink == nil {
			return nil, service.Invalid("events are not available on this connection")
		}
		var sessionID *string
		if raw, present := fields["sessionId"]; present && raw != nil {
			value, valid := protocol.StringValue(raw)
			if !valid {
				return nil, service.Invalid("sessionId must be a string")
			}
			sessionID = &value
		}
		replay := false
		if raw, present := fields["replay"]; present {
			var valid bool
			replay, valid = protocol.BoolValue(raw)
			if !valid {
				return nil, service.Invalid("replay must be a boolean")
			}
		}
		return sink.AddSubscription(sessionID, replay)
	case "events.unsubscribe":
		if sink == nil {
			return nil, service.Invalid("events are not available on this connection")
		}
		id, err := requiredString(fields, "subscriptionId")
		if err != nil {
			return nil, err
		}
		return protocol.Object("removed", sink.RemoveSubscription(id)), nil
	default:
		return nil, &service.AppError{Code: "METHOD_NOT_FOUND", Message: "Unknown method: " + method, Status: 404}
	}
}

func requiredString(fields map[string]protocol.Value, key string) (string, *service.AppError) {
	value, ok := protocol.StringValue(fields[key])
	if !ok || value == "" {
		return "", service.Invalid(key + " must be a non-empty string")
	}
	return value, nil
}

func integer(fields map[string]protocol.Value, key string, fallback int64) (int64, bool) {
	value, exists := fields[key]
	if !exists {
		return fallback, false
	}
	n, ok := protocol.IntValue(value)
	return n, ok
}

func optionalInteger(fields map[string]protocol.Value, key string, fallback int64) (int64, bool) {
	value, exists := fields[key]
	if !exists || value == nil {
		return fallback, true
	}
	n, ok := protocol.IntValue(value)
	return n, ok
}

func normalizeError(err error) error {
	var app *service.AppError
	if errors.As(err, &app) {
		return app
	}
	return service.Internal(err)
}

func webUIURL(host string, port int) string {
	if host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("http://%s:%d/shellink/ui/", host, port)
}
