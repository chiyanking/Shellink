package daemon

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"shellink/internal/client"
	"shellink/internal/config"
)

func Connect(cfg config.Config, timeout time.Duration) (*client.Client, error) {
	return client.Connect(cfg, timeout)
}

func Ensure(cfg config.Config, timeout time.Duration) (*client.Client, error) {
	if current, err := Connect(cfg, time.Second); err == nil {
		return current, nil
	}
	if err := startDetached(cfg); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		if current, err := Connect(cfg, time.Second); err == nil {
			return current, nil
		}
	}
	if tail := LogTail(cfg.LogPath, 40); tail != "" {
		return nil, fmt.Errorf("daemon did not start within %s\nRecent log:\n%s", timeout, tail)
	}
	return nil, fmt.Errorf("daemon did not start within %s", timeout)
}

func LogTail(path string, lines int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return ""
	}
	all := strings.Split(content, "\n")
	if lines > 0 && len(all) > lines {
		all = all[len(all)-lines:]
	}
	return strings.Join(all, "\n")
}

func startDetached(cfg config.Config) error { return startDetachedPlatform(cfg) }

func prepareLog(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	return file, nil
}

func tailScanner(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > 1000 {
			lines = lines[1:]
		}
	}
	return lines, scanner.Err()
}
