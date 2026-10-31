// Package mcpconfig owns MCP HTTP settings shared by local RPC and HTTP.
package mcpconfig

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"

	"shellink/internal/config"
)

type State struct {
	Enabled bool   `json:"enabled"`
	Token   string `json:"token"`
}

var mu sync.Mutex

func load(cfg config.Config) (State, error) {
	state := State{Enabled: true}
	data, err := os.ReadFile(filepath.Join(cfg.DataHome, "mcp.json"))
	if err == nil {
		if err := json.Unmarshal(data, &state); err != nil {
			return state, err
		}
	} else if !os.IsNotExist(err) {
		return state, err
	}
	if state.Token == "" {
		token := make([]byte, 24)
		if _, err := rand.Read(token); err != nil {
			return state, err
		}
		state.Token = hex.EncodeToString(token)
		if err := save(cfg, state); err != nil {
			return state, err
		}
	}
	return state, nil
}
func save(cfg config.Config, state State) error {
	if err := os.MkdirAll(cfg.DataHome, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(cfg.DataHome, ".mcp-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), filepath.Join(cfg.DataHome, "mcp.json"))
}
func Load(cfg config.Config) (State, error) { mu.Lock(); defer mu.Unlock(); return load(cfg) }
func Update(cfg config.Config, enabled *bool, rotate bool) (State, error) {
	mu.Lock()
	defer mu.Unlock()
	state, err := load(cfg)
	if err != nil {
		return state, err
	}
	if enabled != nil {
		state.Enabled = *enabled
	}
	if rotate {
		token := make([]byte, 24)
		if _, err := rand.Read(token); err != nil {
			return state, err
		}
		state.Token = hex.EncodeToString(token)
	}
	return state, save(cfg, state)
}
func Info(cfg config.Config) (map[string]any, error) {
	state, err := Load(cfg)
	if err != nil {
		return nil, err
	}
	host := cfg.Host
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	url := fmt.Sprintf("http://%s/shellink/mcp", net.JoinHostPort(host, fmt.Sprint(cfg.Port)))
	command, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return map[string]any{"enabled": state.Enabled, "running": state.Enabled && cfg.HTTPEnabled, "httpEnabled": cfg.HTTPEnabled, "bindHost": cfg.Host, "url": url, "token": state.Token, "command": command, "args": []string{"mcp"}, "clientConfig": map[string]any{"mcpServers": map[string]any{"shellink": map[string]any{"url": url, "headers": map[string]any{"Authorization": "Bearer " + state.Token}}}}, "clientConfigStdio": map[string]any{"mcpServers": map[string]any{"shellink": map[string]any{"command": command, "args": []string{"mcp"}}}}}, nil
}
