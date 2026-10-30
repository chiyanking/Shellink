package httpapi

import (
	"io/fs"
	"net/http"
	"path"
	"strings"

	webui "shellink/web"
)

func embeddedUIAssets() (http.Handler, bool) {
	root, err := webui.Assets()
	if err != nil {
		return nil, false
	}
	return staticUI(root), true
}

func staticUI(root fs.FS) http.Handler {
	files := http.FileServer(http.FS(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "" || name == "." {
			name = "index.html"
		}
		if _, err := fs.Stat(root, name); err != nil {
			if path.Ext(name) != "" {
				http.NotFound(w, r)
				return
			}
			copy := r.Clone(r.Context())
			urlCopy := *r.URL
			urlCopy.Path = "/index.html"
			copy.URL = &urlCopy
			files.ServeHTTP(w, copy)
			return
		}
		files.ServeHTTP(w, r)
	})
}
