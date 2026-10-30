package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareLogWithFallbackUsesConfiguredPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "shellink.log")
	file, err := prepareLogWithFallback(path)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	defer file.Close()
	if filepath.Clean(file.Name()) != filepath.Clean(path) {
		t.Fatalf("expected %s, got %s", path, file.Name())
	}
}

// A log file that cannot be created (directory in the way, locked by another
// process, read-only data directory) must not abort the daemon start: logging
// moves to the temp directory instead of returning "Access is denied".
func TestPrepareLogWithFallbackFallsBackToTemp(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "shellink.log")
	if err := os.Mkdir(blocked, 0o700); err != nil {
		t.Fatalf("prepare blocked path: %v", err)
	}
	file, err := prepareLogWithFallback(blocked)
	if err != nil {
		t.Fatalf("expected fallback log, got error: %v", err)
	}
	defer file.Close()
	t.Cleanup(func() { _ = os.Remove(file.Name()) })
	if filepath.Clean(file.Name()) == filepath.Clean(blocked) {
		t.Fatalf("expected fallback file, still logging to %s", file.Name())
	}
	if filepath.Dir(file.Name()) != filepath.Clean(os.TempDir()) {
		t.Fatalf("expected temp directory, got %s", file.Name())
	}
}
