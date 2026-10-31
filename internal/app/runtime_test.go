package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"shellink/internal/client"
	"shellink/internal/config"
	"shellink/internal/mcpserver"
	"shellink/internal/protocol"
)

func TestRuntimeStartupReportsHTTPPortConflictBeforeOpeningRPC(t *testing.T) {
	reserved, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "operation not permitted") {
			t.Skip("sandbox does not permit binding a local TCP port")
		}
		t.Fatal(err)
	}
	defer reserved.Close()
	port := reserved.Addr().(*net.TCPAddr).Port

	root := t.TempDir()
	cfg := config.Config{
		DataHome: root, DBPath: filepath.Join(root, "shellink.db"),
		SocketPath: filepath.Join(root, "shellink.sock"), LogPath: filepath.Join(root, "shellink.log"),
		PIDPath: filepath.Join(root, "shellink.pid"), Host: "127.0.0.1", Port: port,
		HTTPEnabled: true, MaxFrameBytes: 16 << 20, SocketMaxQueueBytes: 4 << 20,
		TransferMaxBytes: 1 << 20,
	}

	err = New(cfg).Start()
	if err == nil || !strings.Contains(err.Error(), "listen for HTTP") {
		t.Fatalf("Start() error = %v, want HTTP listen error", err)
	}
	if _, statErr := os.Stat(cfg.SocketPath); !os.IsNotExist(statErr) {
		t.Fatalf("RPC socket exists after HTTP startup failure: %v", statErr)
	}
	if !strings.Contains(err.Error(), fmt.Sprint(port)) {
		t.Fatalf("startup error %q does not identify the occupied port %d", err, port)
	}
}

func TestRuntimeRPCProfileAndCommandSession(t *testing.T) {
	root := t.TempDir()
	socketDir, err := os.MkdirTemp("/tmp", "sh-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	var key [32]byte
	key[0] = 41
	cfg := config.Config{
		DataHome: root, DBPath: filepath.Join(root, "shellink.db"),
		SocketPath: filepath.Join(socketDir, "shellink.sock"), LogPath: filepath.Join(root, "shellink.log"),
		PIDPath: filepath.Join(root, "shellink.pid"), Host: "127.0.0.1", Port: 7070,
		Token: "test-token", MasterKey: key, HTTPEnabled: false,
		MaxFrameBytes: 16 << 20, SocketMaxQueueBytes: 4 << 20, SilenceThresholdMS: 80,
		ExecDefaultTimeoutMS: 2000, TransferMaxBytes: 1 << 20, TransferTimeoutMS: 5000,
		EditTimeoutMS: 2000, SSHReadyTimeoutMS: 5000,
	}
	runtime := New(cfg)
	if err := runtime.Start(); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "operation not permitted") {
			t.Skip("sandbox does not permit binding a local Unix socket")
		}
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	connection, err := client.Connect(cfg, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	status, err := connection.Request("system.status", protocol.Object(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	statusFields := protocol.MapValue(status)
	if _, exists := statusFields["webui"]; exists || statusFields["httpEnabled"] != false || statusFields["httpUrl"] != nil {
		t.Fatalf("HTTP-disabled status advertises removed or disabled endpoints: %#v", status)
	}
	if _, err := connection.Request("system.ping", protocol.Object(), time.Second); err != nil {
		t.Fatal(err)
	}
	profile, err := connection.Request("profiles.create", protocol.JSONValue(map[string]any{
		"name": "local-test", "connectType": "command",
		"command": `stty -echo; printf 'shell$ '; while IFS= read -r line; do eval "$line"; printf 'shell$ '; done`,
	}), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	profileID, ok := protocol.StringValue(protocol.MapValue(profile)["id"])
	if !ok {
		t.Fatalf("profile response had no ID: %#v", profile)
	}
	session, err := connection.Request("sessions.create", protocol.JSONValue(map[string]any{"profileId": profileID}), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	sessionID, ok := protocol.StringValue(protocol.MapValue(session)["id"])
	if !ok {
		t.Fatalf("session response had no ID: %#v", session)
	}
	waitFor := func(want string) protocol.Value {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			value, requestErr := connection.Request("sessions.state", protocol.JSONValue(map[string]any{"id": sessionID}), time.Second)
			if requestErr == nil && protocol.MapValue(value)["state"] == want {
				return value
			}
			time.Sleep(25 * time.Millisecond)
		}
		value, _ := connection.Request("sessions.state", protocol.JSONValue(map[string]any{"id": sessionID}), time.Second)
		t.Fatalf("session did not reach %s: %#v", want, value)
		return nil
	}
	waitFor("WAITING_INPUT")
	for _, command := range []struct {
		name   string
		text   string
		output string
	}{
		{"immediate output", "printf 'go-runtime-ok'", "go-runtime-okshell$ "},
		{"delayed output", "sleep 0.2; printf 'delayed-result'", "delayed-resultshell$ "},
		{"no body output", "true", "shell$ "},
	} {
		t.Run(command.name, func(t *testing.T) {
			result, err := connection.Request("sessions.exec", protocol.JSONValue(map[string]any{
				"id": sessionID, "command": command.text, "timeoutMs": 2000,
			}), 4*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			fields := protocol.MapValue(result)
			output, _ := protocol.StringValue(fields["output"])
			if output != command.output {
				t.Fatalf("command output = %q, want %q", output, command.output)
			}
			if fields["status"] != "DONE" || fields["timedOut"] != false {
				t.Fatalf("unexpected exec result: %#v", result)
			}
			state, err := connection.Request("sessions.state", protocol.JSONValue(map[string]any{"id": sessionID}), time.Second)
			if err != nil || protocol.MapValue(state)["state"] != "WAITING_INPUT" {
				t.Fatalf("unexpected session state after exec: %#v %v", state, err)
			}
		})
	}
	// MCP stdio shares this daemon even though no HTTP listener exists. Check
	// actual history, lifecycle and control errors through its JSON-RPC stream.
	_, err = connection.Request("sessions.mode", protocol.Object("id", sessionID, "mode", "MANUAL"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	mcp := mcpserver.New(func(method string, params protocol.Value) (protocol.Value, error) {
		return connection.Request(method, params, time.Second)
	})
	input := strings.NewReader(fmt.Sprintf(`{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"shellink_read_output","arguments":{"id":%q}}}
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"shellink_send_input","arguments":{"id":%q,"text":"echo forbidden","manual":true}}}
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"shellink_close_session","arguments":{"id":%q}}}
`, sessionID, sessionID, sessionID))
	var output, diagnostics bytes.Buffer
	if err := mcp.Serve(input, &diagnostics, &output); err != nil {
		t.Fatal(err)
	}
	if diagnostics.Len() != 0 {
		t.Fatalf("unexpected MCP diagnostics: %s", &diagnostics)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("stdout must contain exactly three MCP responses: %s", &output)
	}
	for i, line := range lines {
		var response struct {
			ID     int `json:"id"`
			Result struct {
				Content []struct{ Text string } `json:"content"`
				IsError bool                    `json:"isError"`
			} `json:"result"`
		}
		if err := json.Unmarshal([]byte(line), &response); err != nil {
			t.Fatal(err)
		}
		if response.ID != i+1 || len(response.Result.Content) != 1 {
			t.Fatalf("invalid MCP response: %s", line)
		}
		text := response.Result.Content[0].Text
		switch response.ID {
		case 1:
			if response.Result.IsError || !strings.Contains(text, "go-runtime-ok") || !strings.Contains(text, "delayed-result") {
				t.Fatalf("MCP history lost terminal output: %s", line)
			}
		case 2:
			if !response.Result.IsError || !strings.Contains(text, "MANUAL") {
				t.Fatalf("MCP input bypassed manual control: %s", line)
			}
		case 3:
			if response.Result.IsError || !strings.Contains(text, `"ok":true`) {
				t.Fatalf("MCP could not close the shared session: %s", line)
			}
		}
	}
}
