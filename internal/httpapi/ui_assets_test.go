package httpapi

import (
	"bytes"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"testing"
)

func TestEmbeddedUIServesAllBuildAssets(t *testing.T) {
	handler, ok := embeddedUIAssets()
	if !ok {
		t.Fatal("built Web UI assets are not embedded")
	}
	handler = http.StripPrefix("/shellink/ui/", handler)
	build := os.DirFS("assets")
	err := fs.WalkDir(build, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || name == "complete.txt" {
			return nil
		}
		t.Run(name, func(t *testing.T) {
			want, err := fs.ReadFile(build, name)
			if err != nil {
				t.Fatal(err)
			}
			url := "/shellink/ui/" + name
			if name == "index.html" {
				url = "/shellink/ui/"
			}
			request := httptest.NewRequest(http.MethodGet, url, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("built asset is not served: status=%d body=%q", response.Code, response.Body.String())
			}
			if !bytes.Equal(response.Body.Bytes(), want) {
				t.Fatal("served asset differs from build output")
			}
			if path.Ext(name) == ".js" {
				contentType := response.Header().Get("Content-Type")
				if !strings.Contains(contentType, "javascript") {
					t.Fatalf("module script has an invalid Content-Type: %q", contentType)
				}
			}
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
