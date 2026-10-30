// Package mcpserver implements the Model Context Protocol server that is
// shared by the CLI (shellink mcp, stdio transport) and the daemon HTTP API
// (POST /shellink/mcp, HTTP transport).
package mcpserver

// Tools is the list of MCP tools exposed by Shellink.
var Tools = []map[string]any{
	{
		"name":        "shellink_list_profiles",
		"description": "List all configured Shellink connection profiles (SSH or command).",
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
	},
	{
		"name":        "shellink_create_profile",
		"description": "Create a Shellink connection profile. For SSH use connectType=ssh with host/port/username and authType=password plus password, or authType=key with privateKey.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":        map[string]any{"type": "string", "description": "Profile display name"},
				"connectType": map[string]any{"type": "string", "enum": []string{"ssh", "command"}},
				"host":        map[string]any{"type": "string"},
				"port":        map[string]any{"type": "integer"},
				"username":    map[string]any{"type": "string"},
				"authType":    map[string]any{"type": "string", "enum": []string{"password", "key"}},
				"password":    map[string]any{"type": "string", "description": "Plaintext password (server encrypts it)"},
				"command":     map[string]any{"type": "string", "description": "Command for connectType=command"},
				"term":        map[string]any{"type": "string"},
			},
			"required": []string{"name", "connectType"},
		},
	},
	{
		"name":        "shellink_list_sessions",
		"description": "List all Shellink sessions (active and historical).",
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
	},
	{
		"name":        "shellink_create_session",
		"description": "Open a new session for a profile and start connecting.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"profileId": map[string]any{"type": "string", "description": "ID of the profile to connect with"},
			},
			"required": []string{"profileId"},
		},
	},
	{
		"name":        "shellink_session_state",
		"description": "Get the current state (CONNECTING/OUTPUTTING/WAITING_INPUT/IDLE/DISCONNECTED), mode and recent output of a session.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{"type": "string"},
			},
			"required": []string{"id"},
		},
	},
	{
		"name":        "shellink_read_output",
		"description": "Read the terminal output (plain text) of a session.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":    map[string]any{"type": "string"},
				"limit": map[string]any{"type": "integer", "description": "Max number of history chunks"},
			},
			"required": []string{"id"},
		},
	},
	{
		"name":        "shellink_send_input",
		"description": "Send text input to a session (as if typed into the terminal). A trailing newline is added by the caller if desired.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":   map[string]any{"type": "string"},
				"text": map[string]any{"type": "string"},
			},
			"required": []string{"id", "text"},
		},
	},
	{
		"name":        "shellink_close_session",
		"description": "Close/terminate a session.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{"type": "string"},
			},
			"required": []string{"id"},
		},
	},
}
