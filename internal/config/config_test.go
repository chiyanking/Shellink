package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromUsesWindowsEndpointAndUserConfigHome(t *testing.T) {
	root := t.TempDir()
	values := map[string]string{"APPDATA": root}
	getenv := func(key string) string { return values[key] }
	cfg, err := LoadFrom(getenv, "windows")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataHome != filepath.Join(root, "Shellink") || cfg.PipeName == "" || cfg.SocketPath != "" {
		t.Fatalf("unexpected Windows paths: %#v", cfg)
	}
	if info, err := os.Stat(filepath.Join(cfg.DataHome, "master.key")); err != nil || info.Size() != 65 {
		t.Fatalf("master key was not initialized: %v", err)
	}
}

func TestLoadFromRejectsInvalidFrameAndKeySettings(t *testing.T) {
	for _, values := range []map[string]string{
		{"SHELLINK_MAX_FRAME_BYTES": "999"},
		{"SHELLINK_TRANSFER_MAX_BYTES": "20000000"},
		{"SHELLINK_MASTER_KEY": "12"},
		{"SHELLINK_REQUIRE_TOKEN_FOR_SENSITIVE_OPS": "maybe"},
	} {
		t.Run(valuesKey(values), func(t *testing.T) {
			root := t.TempDir()
			values["SHELLINK_HOME"] = root
			cfg, err := LoadFrom(func(key string) string { return values[key] }, "darwin")
			if err == nil {
				t.Fatalf("LoadFrom(%#v) unexpectedly succeeded: %#v", values, cfg)
			}
		})
	}
}

func valuesKey(values map[string]string) string {
	for key := range values {
		return key
	}
	return "case"
}
