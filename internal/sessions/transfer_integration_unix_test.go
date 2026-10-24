//go:build !windows

package sessions

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"shellink/internal/config"
	"shellink/internal/protocol"
	"shellink/internal/store"
)

func TestTransferOperationsThroughInteractiveShell(t *testing.T) {
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "shellink.db"))
	if err != nil {
		t.Fatal(err)
	}
	var masterKey [32]byte
	cfg := config.Config{
		DataHome: root, MasterKey: masterKey, TransferMaxBytes: 1 << 20,
		TransferTimeoutMS: 15_000, EditTimeoutMS: 15_000, ExecDefaultTimeoutMS: 2_000,
		SilenceThresholdMS: 100,
	}
	bus := NewBus()
	manager := NewManager(db, cfg, bus)
	t.Cleanup(func() {
		manager.CloseAll("test complete")
		_ = db.Close()
	})
	const sessionID = "transfer-test"
	if err := db.InsertSession(store.SessionRecord{
		ID: sessionID, ProfileID: "test-profile", ProfileName: "test", Target: "local",
		State: string(Connecting), Mode: string(Auto), Cols: 80, Rows: 24, CreatedAt: time.Now().UnixMilli(),
	}); err != nil {
		t.Fatal(err)
	}
	session, err := NewSession(SessionOptions{
		ID: sessionID, ProfileID: "test-profile", ProfileName: "test", Term: "xterm-256color",
		Cols: 80, Rows: 24, SilenceMS: cfg.SilenceThresholdMS,
	}, bus)
	if err != nil {
		t.Fatal(err)
	}
	transport, err := NewProcessTransport("/bin/sh", []string{"-lc", `printf 'shell$ '; while IFS= read -r line; do eval "$line"; printf 'shell$ '; done`}, nil, "xterm-256color", 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	manager.mu.Lock()
	manager.live[sessionID] = session
	manager.mu.Unlock()
	session.Attach(transport)
	waitUntil := time.Now().Add(5 * time.Second)
	for {
		state, _, _, active := session.Snapshot()
		if state == WaitingInput && active {
			break
		}
		if time.Now().After(waitUntil) {
			t.Fatalf("shell did not become ready: state=%s active=%t output=%q", state, active, session.RecentOutput(10))
		}
		time.Sleep(20 * time.Millisecond)
	}

	remotePath := filepath.Join(root, "uploaded file.bin")
	want := bytes.Repeat([]byte{0, 1, 2, 0xff, '\n'}, 1024)
	if _, err := manager.Upload(sessionID, remotePath, want, 15_000, ""); err != nil {
		t.Fatalf("upload failed: %v; recent output %q", err, session.RecentOutput(10))
	}
	downloadValue, err := manager.Download(sessionID, remotePath, 15_000)
	if err != nil {
		t.Fatalf("download failed: %v; recent output %q", err, session.RecentOutput(10))
	}
	got, ok := protocol.MapValue(downloadValue)["data"].([]byte)
	if !ok || !bytes.Equal(got, want) {
		t.Fatalf("download data = %v, want %v", got, want)
	}
	editPath := filepath.Join(root, "editable file.txt")
	if _, err := manager.Upload(sessionID, editPath, []byte("alpha\r\nShellink\r\n"), 15_000, ""); err != nil {
		t.Fatalf("edit fixture upload failed: %v", err)
	}
	if _, err := manager.Edit(sessionID, editPath, []TextEdit{{OldText: "Shellink", NewText: "Go Shellink"}}, 15_000); err != nil {
		t.Fatalf("edit failed: %v; recent output %q", err, session.RecentOutput(10))
	}
	updated, err := os.ReadFile(editPath)
	if err != nil {
		t.Fatal(err)
	}
	editWant := []byte("alpha\r\nGo Shellink\r\n")
	if !bytes.Equal(updated, editWant) {
		t.Fatalf("edited file = %q, want %q", updated, editWant)
	}
}
