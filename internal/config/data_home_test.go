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

// A failure of the writability probe must not move the database away from a
// directory that already holds one: that is what made a fresh portable folder
// come up empty while the real session history was still in the old place.
func TestResolveDataHomeKeepsDefaultWhenItAlreadyHoldsData(t *testing.T) {
	appData := t.TempDir()
	existing := filepath.Join(appData, "Shellink")
	if err := os.MkdirAll(existing, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(existing, "shellink.db"), []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}
	original := writableProbe
	writableProbe = func(string) bool { return false }
	defer func() { writableProbe = original }()

	portable := filepath.Join(t.TempDir(), "shellink-data")
	got, err := resolveDataHomeWith(env(map[string]string{"APPDATA": appData}), "windows", t.TempDir(), portable)
	if err != nil {
		t.Fatal(err)
	}
	if got != existing {
		t.Fatalf("left the existing data directory: got %q want %q", got, existing)
	}
	if _, err := os.Stat(filepath.Join(portable)); err == nil {
		t.Fatalf("portable directory should not have been created")
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
