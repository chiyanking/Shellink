//go:build !windows

package localipc

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"shellink/internal/config"
)

func listenPlatform(cfg config.Config) (*Listener, error) {
	dir := filepath.Dir(cfg.SocketPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return nil, err
	}
	if info, err := os.Lstat(cfg.SocketPath); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("refusing to remove a non-socket path: %s", cfg.SocketPath)
		}
		conn, dialErr := net.DialTimeout("unix", cfg.SocketPath, 300*time.Millisecond)
		if dialErr == nil {
			conn.Close()
			return nil, fmt.Errorf("Shellink daemon is already running")
		}
		if err := os.Remove(cfg.SocketPath); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	listener, err := net.Listen("unix", cfg.SocketPath)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(cfg.SocketPath, 0600); err != nil {
		listener.Close()
		os.Remove(cfg.SocketPath)
		return nil, err
	}
	return &Listener{NetListener: listener, cleanup: func() { _ = os.Remove(cfg.SocketPath) }}, nil
}

func dialPlatform(cfg config.Config) (net.Conn, string, error) {
	conn, err := net.DialTimeout("unix", cfg.SocketPath, time.Second)
	return conn, "", err
}
