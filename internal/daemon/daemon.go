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

// prepareLogWithFallback opens the configured log file and falls back to the OS
// temporary directory when that path cannot be written.
//
// A daemon that cannot write its log is still able to serve the UI, so failing
// the whole start just because the log file is locked (antivirus, a leftover
// handle, a read-only data directory) only produces an opaque "Access is
// denied" with no way forward. Logging to the temp directory keeps the app
// usable and `server logs` still has something to show.
func prepareLogWithFallback(path string) (*os.File, error) {
	file, err := prepareLog(path)
	if err == nil {
		return file, nil
	}
	fallback := filepath.Join(os.TempDir(), "shellink.log")
	if filepath.Clean(fallback) == filepath.Clean(path) {
		return nil, err
	}
	if alternative, altErr := prepareLog(fallback); altErr == nil {
		return alternative, nil
	}
	return nil, err
}

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
