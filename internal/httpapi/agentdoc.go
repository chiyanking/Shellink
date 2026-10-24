package httpapi

import _ "embed"

//go:embed agentdoc.md
var AgentDoc string

var AgentDocJSON = map[string]any{
	"name":        "shellink",
	"defaultMode": "cli",
	"output": map[string]any{
		"jsonFlag":  "--json",
		"exitCodes": map[string]int{"success": 0, "runtimeError": 1, "usageError": 2},
	},
	"states":              []string{"CONNECTING", "OUTPUTTING", "WAITING_INPUT", "IDLE", "DISCONNECTED"},
	"modes":               []string{"AUTO", "MANUAL"},
	"jobStatuses":         []string{"RUNNING", "DONE", "TIMED_OUT", "CANCELED", "DISCONNECTED", "FAILED"},
	"profileConnectTypes": []string{"ssh", "command"},
	"defaults": map[string]int{
		"execTimeoutMs": 20_000, "editTimeoutMs": 25_000, "transferTimeoutMs": 120_000,
	},
	"longRunning": map[string]any{
		"preferDetachAboveMs": 25_000,
		"flow": []string{
			"shellink session exec '<id>' --command '...' --detach --json",
			"shellink session exec-status '<job-id>' --since <cursor> --wait 20000 --json",
			"shellink session exec-cancel '<job-id>' --json",
		},
		"onConflict409":    "poll exec-status for the named job, or exec-cancel",
		"onMethodNotFound": "shellink server restart",
	},
	"remoteEdit": map[string]any{
		"command": "shellink session edit ID --input <FILE|-> --json",
		"input": map[string]any{
			"path":  "non-empty remote file path",
			"edits": []map[string]string{{"oldText": "non-empty exact text, unique in the file", "newText": "replacement text"}},
		},
		"requirements": map[string]any{"mode": "AUTO", "state": "WAITING_INPUT", "editsMustNotOverlap": true},
	},
	"commands": map[string][]string{
		"profile": {"list", "get", "create", "update", "delete"},
		"session": {"list", "create", "state", "history", "input", "exec", "exec-status", "exec-cancel", "mode", "close", "remove-record", "download", "upload", "edit"},
		"webhook": {"list", "create", "delete"},
		"server":  {"start", "status", "stop", "restart", "logs", "run"},
		"upgrade": {"--check", "--version", "--yes"},
	},
}
