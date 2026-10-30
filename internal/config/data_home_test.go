package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveDataHomePrefersExplicitHome(t *testing.T) {
	explicit := t.TempDir()
	values := map[string]string{"SHELLINK_HOME": explicit, "APPDATA": t.TempDir()}
	got, err := resolveDataHomeWith(env(values), "windows", t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got != explicit {
		t.Fatalf("explicit SHELLINK_HOME ignored: got %q want %q", got, explicit)
	}
}

// A "shellink-data" folder left beside the executable must not take over the
// database: doing so silently shows an empty session list while the real
// history stays in the per-user directory.
func TestResolveDataHomeIgnoresPortableFolderWhenDefaultIsWritable(t *testing.T) {
	appData := t.TempDir()
	portable := filepath.Join(t.TempDir(), "shellink-data")
	if err := os.MkdirAll(portable, 0o700); err != nil {
		t.Fatal(err)
	}
	values := map[string]string{"APPDATA": appData}
	got, err := resolveDataHomeWith(env(values), "windows", t.TempDir(), portable)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(appData, "Shellink")
	if got != want {
		t.Fatalf("portable folder took precedence: got %q want %q", got, want)
	}
}

func TestResolveDataHomeFallsBackWhenDefaultIsNotWritable(t *testing.T) {
	// A path whose parent is a regular file can never be created, which is how
	// a locked-down or read-only profile directory behaves.
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	portable := filepath.Join(t.TempDir(), "shellink-data")
	values := map[string]string{"APPDATA": blocker}
	got, err := resolveDataHomeWith(env(values), "windows", t.TempDir(), portable)
	if err != nil {
		t.Fatal(err)
	}
	if got != portable {
		t.Fatalf("expected portable fallback %q, got %q", portable, got)
	}
	if !writableDirectory(portable) {
		t.Fatalf("fallback directory was not usable: %q", portable)
	}
}

func TestResolveDataHomeUnixDefault(t *testing.T) {
	home := t.TempDir()
	got, err := resolveDataHomeWith(env(map[string]string{}), "darwin", home, "")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".Shellink"); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}
