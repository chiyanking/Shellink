package cli

import (
	"fmt"
	"os"
	"time"

	"shellink/internal/config"
	"shellink/internal/daemon"
	"shellink/internal/mcpserver"
	"shellink/internal/protocol"
)

// handleMCP starts an MCP (Model Context Protocol) server over stdio. It ensures
// the Shellink daemon is running, then uses the same local RPC connection as
// the CLI. It works even when the daemon's HTTP service is disabled.
func handleMCP(cfg config.Config, _ map[string]string) error {
	conn, err := daemon.Ensure(cfg, 15*time.Second)
	if err != nil {
		return fmt.Errorf("failed to start Shellink daemon: %w", err)
	}
	defer conn.Close()
	return mcpserver.New(func(method string, params protocol.Value) (protocol.Value, error) {
		return conn.Request(method, params, 30*time.Second)
	}).Serve(os.Stdin, os.Stderr, os.Stdout)
}
