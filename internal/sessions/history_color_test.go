package sessions

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"shellink/internal/config"
	"shellink/internal/protocol"
	"shellink/internal/store"
)

// historyOf builds a manager over a throwaway database carrying one recorded
// chunk, then asks for the transcript with and without colour.
//
// `since` is exclusive, so a chunk has to carry a sequence number above zero to
// be picked up at all.
func historyOf(t *testing.T, raw string) (colored string, plain string) {
	t.Helper()
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
	manager := NewManager(db, cfg, NewBus())
	t.Cleanup(func() {
		manager.CloseAll("test complete")
		_ = db.Close()
	})

	const sessionID = "history-color"
	if err := db.InsertSession(store.SessionRecord{
		ID: sessionID, ProfileID: "p", ProfileName: "p", Target: "local",
		State: string(Connecting), Mode: string(Auto), Cols: 80, Rows: 24,
		CreatedAt: time.Now().UnixMilli(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.AddHistory([]store.HistoryChunk{{
		SessionID: sessionID, Direction: "output", Seq: 1, DataRaw: raw, DataPlain: StripANSI(raw),
	}}); err != nil {
		t.Fatal(err)
	}

	asText := func(value any) string {
		fields, ok := value.([]protocol.KV)
		if !ok {
			t.Fatalf("expected key/value fields, got %T", value)
		}
		for _, field := range fields {
			if field.Key != "text" {
				continue
			}
			text, ok := field.Val.(string)
			if !ok {
				t.Fatalf("text field is %T, not a string", field.Val)
			}
			return text
		}
		return ""
	}

	withColor, err := manager.History(sessionID, 0, 10, false, true)
	if err != nil {
		t.Fatal(err)
	}
	withoutColor, err := manager.History(sessionID, 0, 10, false, false)
	if err != nil {
		t.Fatal(err)
	}
	return asText(withColor), asText(withoutColor)
}

// The daemon keeps whatever the remote shell painted. A caller looking at the
// transcript gets it; a caller parsing it does not.
func TestHistoryKeepsColourOnlyWhenAsked(t *testing.T) {
	colored, plain := historyOf(t, "root@VM-0-7-ubuntu:/# ls\r\n\x1b[32ma.txt\x1b[0m\r\n")

	if !strings.Contains(colored, "\x1b[") {
		t.Fatalf("coloured transcript lost its escapes: %q", colored)
	}
	if strings.Contains(plain, "\x1b[") {
		t.Fatalf("plain transcript leaked escapes: %q", plain)
	}
	// Both branches must carry the same words, only the paint differs.
	if !strings.Contains(plain, "root@VM-0-7-ubuntu:/# ls") || !strings.Contains(plain, "a.txt") {
		t.Fatalf("plain transcript lost content: %q", plain)
	}
}

// A prompt that arrives with the shell's own bracketed-paste and window-title
// noise must still be recognised and tinted.
func TestHistoryTintsPromptBehindShellNoise(t *testing.T) {
	colored, plain := historyOf(t, "\x1b[?2004h\x1b]0;root@VM-0-7-ubuntu: ~\x07root@VM-0-7-ubuntu:~# ls\r\n")

	if !strings.Contains(colored, "\x1b[1;32mroot@VM-0-7-ubuntu\x1b[0m") {
		t.Fatalf("prompt was not tinted: %q", colored)
	}
	if !strings.Contains(colored, "\x1b[1;33m#\x1b[0m") {
		t.Fatalf("root sigil was not tinted: %q", colored)
	}
	// The screen noise must never reach a review pane.
	if strings.Contains(plain, "?2004") || strings.Contains(colored, "?2004") {
		t.Fatalf("bracketed-paste switch leaked into the transcript: %q", colored)
	}
	if strings.Contains(colored, "\x1b]0;") || strings.Contains(plain, "\x1b]0;") {
		t.Fatalf("window-title sequence leaked into the transcript: %q", colored)
	}
}
