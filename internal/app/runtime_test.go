package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"shellink/internal/client"
	"shellink/internal/config"
	"shellink/internal/protocol"
)

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
	if _, err := connection.Request("system.ping", protocol.Object(), time.Second); err != nil {
		t.Fatal(err)
	}
	profile, err := connection.Request("profiles.create", protocol.JSONValue(map[string]any{
		"name": "local-test", "connectType": "command",
		"command": `printf 'shell$ '; while IFS= read -r line; do eval "$line"; printf 'shell$ '; done`,
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
	result, err := connection.Request("sessions.exec", protocol.JSONValue(map[string]any{
		"id": sessionID, "command": "printf 'go-runtime-ok'", "timeoutMs": 2000,
	}), 4*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	output, _ := protocol.StringValue(protocol.MapValue(result)["output"])
	if !strings.Contains(output, "go-runtime-ok") {
		t.Fatalf("command output did not contain marker: %q", output)
	}
	state, err := connection.Request("sessions.state", protocol.JSONValue(map[string]any{"id": sessionID}), time.Second)
	if err != nil || protocol.MapValue(state)["state"] != "WAITING_INPUT" {
		t.Fatalf("unexpected session state after exec: %#v %v", state, err)
	}
	_, err = connection.Request("sessions.close", protocol.JSONValue(map[string]any{"id": sessionID}), time.Second)
	if err != nil {
		t.Fatal(err)
	}
}
