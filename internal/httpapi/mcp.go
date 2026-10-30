package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"shellink/internal/mcpserver"
	"shellink/internal/protocol"
)

// mcpState is the persisted configuration of the built-in MCP server.
type mcpState struct {
	Enabled bool   `json:"enabled"`
	Token   string `json:"token"`
}

var (
	mcpMu     sync.Mutex
	mcpLoaded bool
	mcpCache  mcpState
)

func (s *Server) mcpStatePath() string {
	return filepath.Join(s.cfg.DataHome, "mcp.json")
}

// loadMCPState reads (and lazily initialises) the MCP configuration. A token is
// generated on first use so the endpoint is never accidentally open.
func (s *Server) loadMCPState() mcpState {
	mcpMu.Lock()
	defer mcpMu.Unlock()
	if mcpLoaded {
		return mcpCache
	}
	state := mcpState{Enabled: true}
	if data, err := os.ReadFile(s.mcpStatePath()); err == nil {
		_ = json.Unmarshal(data, &state)
	}
	if state.Token == "" {
		state.Token = newMCPToken()
		mcpCache, mcpLoaded = state, true
		data, err := json.MarshalIndent(state, "", "  ")
		if err == nil {
			_ = os.MkdirAll(s.cfg.DataHome, 0700)
			_ = os.WriteFile(s.mcpStatePath(), data, 0600)
		}
	}
	mcpCache, mcpLoaded = state, true
	return mcpCache
}

func (s *Server) saveMCPState(state mcpState) error {
	mcpMu.Lock()
	mcpCache, mcpLoaded = state, true
	mcpMu.Unlock()
	if err := os.MkdirAll(s.cfg.DataHome, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.mcpStatePath(), data, 0600)
}

func newMCPToken() string {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return hex.EncodeToString([]byte(fmt.Sprint(time.Now().UnixNano())))
	}
	return hex.EncodeToString(buf)
}

// advertisedHost reports the host an external MCP client should use. When the
// daemon binds to a wildcard address we substitute the machine's LAN address so
// the displayed URL is actually reachable.
func (s *Server) advertisedHost() string {
	host := strings.TrimSpace(s.cfg.Host)
	if host == "" || host == "0.0.0.0" || host == "::" {
		if lan := localIP(); lan != "" {
			return lan
		}
		return "127.0.0.1"
	}
	return host
}

// localIP returns the first private IPv4 address of this machine.
func localIP() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.To4() == nil {
				continue
			}
			if isPrivateIPv4(ip) {
				return ip.String()
			}
		}
	}
	return ""
}

func isPrivateIPv4(ip net.IP) bool {
	return ip[0] == 10 || (ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31) || (ip[0] == 192 && ip[1] == 168)
}

func (s *Server) mcpURL() string {
	return fmt.Sprintf("http://%s:%d/shellink/mcp", s.advertisedHost(), s.cfg.Port)
}

// mcpInfo describes the built-in MCP server so the Web UI can control it and
// show users exactly how to plug Shellink into an MCP client.
func (s *Server) mcpInfo(w http.ResponseWriter, _ *http.Request) {
	state := s.loadMCPState()
	command, err := os.Executable()
	if err != nil {
		command = "shellink"
	}
	url := s.mcpURL()
	host := s.advertisedHost()
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":      state.Enabled,
		"running":      state.Enabled,
		"rpcConnected": true,
		"transport":    "http",
		"host":         host,
		"bindHost":     s.cfg.Host,
		"port":         s.cfg.Port,
		"path":         "/shellink/mcp",
		"url":          url,
		"token":        state.Token,
		"tokenSet":     state.Token != "",
		"command":      command,
		"args":         []string{"mcp"},
		"clientConfig": map[string]any{
			"mcpServers": map[string]any{
				"shellink": map[string]any{
					"url": url,
					"headers": map[string]any{
						"Authorization": "Bearer " + state.Token,
					},
				},
			},
		},
		"clientConfigStdio": map[string]any{
			"mcpServers": map[string]any{
				"shellink": map[string]any{
					"command": command,
					"args":    []string{"mcp"},
				},
			},
		},
		"tools": mcpserver.Tools,
	})
}

// mcpControl turns the MCP endpoint on or off and rotates its access token.
func (s *Server) mcpControl(w http.ResponseWriter, r *http.Request) {
	state := s.loadMCPState()
	var body struct {
		Enabled         *bool `json:"enabled"`
		RegenerateToken bool  `json:"regenerateToken"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body)
	if body.Enabled != nil {
		state.Enabled = *body.Enabled
	}
	if body.RegenerateToken {
		state.Token = newMCPToken()
	}
	if err := s.saveMCPState(state); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": state.Enabled, "running": state.Enabled, "token": state.Token})
}

func (s *Server) mcpStart(w http.ResponseWriter, _ *http.Request) {
	state := s.loadMCPState()
	state.Enabled = true
	if err := s.saveMCPState(state); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": true, "running": true, "token": state.Token})
}

func (s *Server) mcpStop(w http.ResponseWriter, _ *http.Request) {
	state := s.loadMCPState()
	state.Enabled = false
	if err := s.saveMCPState(state); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": false, "running": false})
}

func (s *Server) mcpRestart(w http.ResponseWriter, _ *http.Request) {
	state := s.loadMCPState()
	state.Enabled = true
	if err := s.saveMCPState(state); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": true, "running": true, "token": state.Token})
}

// mcpHTTP serves MCP over plain HTTP (JSON-RPC 2.0 POST). This is the address
// style shown in the Web UI: http://<host>:<port>/shellink/mcp
func (s *Server) mcpHTTP(w http.ResponseWriter, r *http.Request) {
	state := s.loadMCPState()
	if !state.Enabled {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "MCP service is disabled"})
		return
	}
	if state.Token != "" {
		provided := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if !strings.EqualFold(provided, state.Token) {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "Unauthorized: invalid MCP token"})
			return
		}
	}
	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
			"error": "Use POST with a JSON-RPC 2.0 body (streamable HTTP transport)",
		})
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	srv := mcpserver.New(fmt.Sprintf("http://%s:%d/shellink/api", s.cfg.Host, s.cfg.Port))
	out := srv.HandleJSON(body)
	if out == nil {
		// Notification: no response body expected.
		w.WriteHeader(http.StatusAccepted)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(out)
}

// serverStop shuts the whole daemon down, so the UI can stop the service without
// running "shellink.exe server stop" in a terminal.
func (s *Server) serverStop(w http.ResponseWriter, _ *http.Request) {
	value, err := s.rpc.Dispatch("system.stop", protocol.Object(), nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"stopping": true, "detail": protocol.ToJSONValue(value)})
}
