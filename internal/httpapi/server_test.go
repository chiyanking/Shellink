package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"shellink/internal/config"
	"shellink/internal/rpc"
	"shellink/internal/service"
	"shellink/internal/sessions"
	"shellink/internal/store"
	"shellink/internal/webhooks"
)

func TestSessionCloseHTTPRoute(t *testing.T) {
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "shellink.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var key [32]byte
	key[0] = 1
	cfg := config.Config{DataHome: root, DBPath: filepath.Join(root, "shellink.db"), MasterKey: key}
	bus := sessions.NewBus()
	manager := sessions.NewManager(db, cfg, bus)
	t.Cleanup(func() { manager.CloseAll("test complete") })
	hooks := webhooks.New(db, bus)
	dispatcher := rpc.NewDispatcher(cfg, db, service.NewProfiles(db, key), manager, hooks, nil)
	handler := New(cfg, dispatcher, manager, hooks, bus).Handler()
	request := httptest.NewRequest("POST", "/shellink/api/sessions/missing/close", nil)
	request.Host = "localhost:7070"
	request.RemoteAddr = "127.0.0.1:1234"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected JSON response, got %q: %v", response.Body.String(), err)
	}
	if response.Code != 404 || body["error"] != "Session not found" {
		t.Fatalf("unexpected close response: status=%d body=%v", response.Code, body)
	}
}
