package cli

import (
	"fmt"
	"os"
	"time"

	"shellink/internal/config"
	"shellink/internal/daemon"
	"shellink/internal/mcpserver"
)

// handleMCP starts an MCP (Model Context Protocol) server over stdio. It ensures
// the Shellink daemon is running, then translates MCP tool calls into requests
// against the daemon's local HTTP API. This lets any MCP-capable client (e.g. an
// AI assistant) drive Shellink: list/create profiles and sessions, send input,
// and read output.
func handleMCP(cfg config.Config, _ map[string]string) error {
	conn, err := daemon.Ensure(cfg, 15*time.Second)
	if err != nil {
		return fmt.Errorf("failed to start Shellink daemon: %w", err)
	}
	defer conn.Close()
	base := fmt.Sprintf("http://%s:%d/shellink/api", cfg.Host, cfg.Port)
	return mcpserver.New(base).Serve(os.Stdin, os.Stderr, os.Stdout)
}
