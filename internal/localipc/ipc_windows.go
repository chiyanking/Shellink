//go:build windows

package localipc

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"shellink/internal/config"
)

type endpoint struct {
	Address string `json:"address"`
	Token   string `json:"token"`
}

func endpointPath(cfg config.Config) string { return filepath.Join(cfg.DataHome, "shellink.endpoint") }

func listenPlatform(cfg config.Config) (*Listener, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		listener.Close()
		return nil, err
	}
	value := endpoint{Address: listener.Addr().String(), Token: hex.EncodeToString(secret[:])}
	data, err := json.Marshal(value)
	if err == nil {
		err = os.WriteFile(endpointPath(cfg), data, 0600)
	}
	if err != nil {
		listener.Close()
		return nil, err
	}
	return &Listener{NetListener: listener, Token: value.Token, cleanup: func() { _ = os.Remove(endpointPath(cfg)) }}, nil
}

func dialPlatform(cfg config.Config) (net.Conn, string, error) {
	data, err := os.ReadFile(endpointPath(cfg))
	if err != nil {
		return nil, "", err
	}
	var value endpoint
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, "", fmt.Errorf("invalid Shellink endpoint file: %w", err)
	}
	conn, err := net.DialTimeout("tcp", value.Address, time.Second)
	return conn, value.Token, err
}
