package rpc

import (
	"encoding/json"
	"os"
	"runtime"
	"strings"
	"time"

	"shellink/internal/mcpconfig"
	"shellink/internal/protocol"
	"shellink/internal/service"
)

func (d *Dispatcher) desktopDispatch(method string, params protocol.Value) (protocol.Value, error) {
	fields := protocol.MapValue(params)
	switch method {
	case "desktop.info":
		executable, _ := os.Executable()
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/sh"
		}
		if runtime.GOOS == "windows" {
			shell = "powershell.exe"
		}
		status, err := d.Dispatch("system.status", protocol.Object(), nil)
		if err != nil {
			return nil, err
		}
		return protocol.Object("status", status, "dataHome", d.cfg.DataHome, "logPath", d.cfg.LogPath, "command", executable, "defaultShell", shell, "platform", runtime.GOOS), nil
	case "mcp.info":
		info, err := mcpconfig.Info(d.cfg)
		return protocol.JSONValue(info), err
	case "mcp.config":
		var enabled *bool
		if raw, ok := fields["enabled"]; ok {
			value, valid := protocol.BoolValue(raw)
			if !valid {
				return nil, service.Invalid("enabled must be a boolean")
			}
			enabled = &value
		}
		rotate := false
		if raw, ok := fields["regenerateToken"]; ok {
			var valid bool
			rotate, valid = protocol.BoolValue(raw)
			if !valid {
				return nil, service.Invalid("regenerateToken must be a boolean")
			}
		}
		if _, err := mcpconfig.Update(d.cfg, enabled, rotate); err != nil {
			return nil, err
		}
		info, err := mcpconfig.Info(d.cfg)
		return protocol.JSONValue(info), err
	case "webhooks.messages":
		data, err := json.Marshal(d.webhooks.Messages())
		if err != nil {
			return nil, err
		}
		var value any
		err = json.Unmarshal(data, &value)
		return protocol.JSONValue(value), err
	case "webhooks.clearMessages":
		d.webhooks.ClearMessages()
		return protocol.Object("ok", true), nil
	}
	id, err := requiredString(fields, "id")
	if err != nil {
		return nil, err
	}
	switch method {
	case "sessions.rename":
		name, err := requiredString(fields, "name")
		if err != nil {
			return nil, err
		}
		name = strings.TrimSpace(name)
		if name == "" || len([]rune(name)) > 80 {
			return nil, service.Invalid("name must be between 1 and 80 characters")
		}
		record, dbErr := d.db.Session(id)
		if dbErr != nil {
			return nil, service.NotFound("Session not found")
		}
		if err := d.db.UpdateSessionMetadata(id, name, record.Source); err != nil {
			return nil, err
		}
		return protocol.Object("ok", true), nil
	case "sessions.replay":
		return d.sessions.DesktopReplay(id)
	case "sessions.historyChunks":
		since, ok := optionalInteger(fields, "since", 0)
		if !ok || since < 0 {
			return nil, service.Invalid("since must be non-negative")
		}
		limit, ok := optionalInteger(fields, "limit", 500)
		if !ok || limit < 1 || limit > 2000 {
			return nil, service.Invalid("limit must be between 1 and 2000")
		}
		if _, err := d.sessions.History(id, 0, 1, false, false); err != nil {
			return nil, err
		}
		chunks, err := d.db.History(id, since, int(limit))
		if err != nil {
			return nil, err
		}
		items := make([]protocol.Value, 0, len(chunks))
		cursor := since
		for _, chunk := range chunks {
			cursor = chunk.Seq
			if chunk.Internal {
				continue
			}
			items = append(items, protocol.Object("id", chunk.ID, "seq", chunk.Seq, "sessionId", id, "direction", chunk.Direction, "plain", chunk.DataPlain, "createdAt", chunk.CreatedAt))
		}
		return protocol.Object("items", items, "cursor", cursor, "hasMore", len(chunks) == int(limit), "at", time.Now().UnixMilli()), nil
	}
	return nil, service.Invalid("Unknown desktop method")
}
