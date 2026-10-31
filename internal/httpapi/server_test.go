package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"shellink/internal/config"
	"shellink/internal/mcpconfig"
	"shellink/internal/rpc"
	"shellink/internal/service"
	"shellink/internal/sessions"
	"shellink/internal/store"
	"shellink/internal/webhooks"
)

func TestSessionCloseHTTPRoute(t *testing.T) {
	handler := testServer(t).Handler()
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

func testServer(t *testing.T) *Server {
	t.Helper()
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
	return New(cfg, dispatcher, manager, hooks)
}

func TestBrowserRoutesRemoved(t *testing.T) {
	handler := testServer(t).Handler()
	for _, path := range []string{"/shellink/ui", "/shellink/ui/", "/shellink/ui/mcp", "/shellink/ws/events", "/shellink/ws/sessions/missing"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNotFound {
			t.Errorf("removed route %s returned %d", path, response.Code)
		}
	}
}

func TestMCPHTTPDispatchesWithoutLoopbackAPI(t *testing.T) {
	server := testServer(t)
	// No listener is started, so an HTTP request back into the daemon would
	// fail. MCP must dispatch directly and retain its token gate.
	handler := server.Handler()
	state, err := mcpconfig.Load(server.cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"invalid-token", state.Token} {
		request := httptest.NewRequest(http.MethodPost, "/shellink/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"shellink_list_profiles","arguments":{}}}`))
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if token != state.Token {
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("unauthorized MCP returned %d", response.Code)
			}
			continue
		}
		var body struct {
			Result struct {
				Content []struct{ Text string } `json:"content"`
				IsError bool                    `json:"isError"`
			} `json:"result"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if response.Code != http.StatusOK || body.Result.IsError || len(body.Result.Content) != 1 || body.Result.Content[0].Text != "[]" {
			t.Fatalf("MCP required a loopback HTTP API: status=%d body=%s", response.Code, response.Body)
		}
	}
}

func TestWebhookCallbackStillReceivesMessages(t *testing.T) {
	server := testServer(t)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/shellink/webhook/callback", strings.NewReader(`{"event":"build.completed","sessionId":"sess-test"}`)))
	if response.Code != http.StatusAccepted || len(server.webhooks.Messages()) != 1 {
		t.Fatalf("callback failed: status=%d body=%s", response.Code, response.Body)
	}
}
