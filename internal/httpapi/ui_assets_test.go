//go:build embedui

package httpapi

import (
	"bytes"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestEmbeddedUIServesBuildOutput(t *testing.T) {
	handler, ok := embeddedUIAssets()
	if !ok {
		t.Fatal("embedded UI is unavailable")
	}
	build := os.DirFS("../../web/dist")
	if _, err := fs.Stat(build, "index.html"); err != nil {
		t.Fatal(err)
	}
	err := fs.WalkDir(build, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		t.Run(name, func(t *testing.T) {
			want, err := fs.ReadFile(build, name)
			if err != nil {
				t.Fatal(err)
			}
			url := "/" + name
			if name == "index.html" {
				url = "/"
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, url, nil))
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
			}
			if !bytes.Equal(response.Body.Bytes(), want) {
				t.Fatal("served asset differs from frontend build output")
			}
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
