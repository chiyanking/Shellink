package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"shellink/internal/mcpconfig"
	"shellink/internal/mcpserver"
	"shellink/internal/protocol"
)

func (s *Server) mcpInfo(w http.ResponseWriter, _ *http.Request) {
	info, err := mcpconfig.Info(s.cfg)
	if err != nil {
		s.respondError(w, err)
		return
	}
	info["tools"] = mcpserver.Tools
	writeJSON(w, http.StatusOK, info)
}
func (s *Server) mcpControl(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled         *bool `json:"enabled"`
		RegenerateToken bool  `json:"regenerateToken"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Invalid MCP configuration"})
		return
	}
	state, err := mcpconfig.Update(s.cfg, body.Enabled, body.RegenerateToken)
	if err != nil {
		s.respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}
func (s *Server) setMCP(w http.ResponseWriter, enabled bool) {
	state, err := mcpconfig.Update(s.cfg, &enabled, false)
	if err != nil {
		s.respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}
func (s *Server) mcpStart(w http.ResponseWriter, _ *http.Request)   { s.setMCP(w, true) }
func (s *Server) mcpStop(w http.ResponseWriter, _ *http.Request)    { s.setMCP(w, false) }
func (s *Server) mcpRestart(w http.ResponseWriter, _ *http.Request) { s.setMCP(w, true) }
func (s *Server) mcpHTTP(w http.ResponseWriter, r *http.Request) {
	state, err := mcpconfig.Load(s.cfg)
	if err != nil {
		s.respondError(w, err)
		return
	}
	if !state.Enabled {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "MCP service is disabled"})
		return
	}
	if !subtleTokenEqual(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), state.Token) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "Unauthorized: invalid MCP token"})
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "Use POST with a JSON-RPC 2.0 body"})
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	srv := mcpserver.New(func(method string, params protocol.Value) (protocol.Value, error) {
		return s.rpc.Dispatch(method, params, nil)
	})
	out := srv.HandleJSON(body)
	if out == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(out)
}
func (s *Server) serverStop(w http.ResponseWriter, _ *http.Request) {
	value, err := s.rpc.Dispatch("system.stop", protocol.Object(), nil)
	s.respond(w, value, err)
}
