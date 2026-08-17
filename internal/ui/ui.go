package ui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed dist/*
var staticFS embed.FS

// Handler serves the embedded Svelte SPA. Requests that look like asset files
// (they contain a dot, e.g. /assets/index-abc.js) are served from the embedded
// filesystem; everything else returns index.html so client-side routing works.
func Handler() http.Handler {
	distFS, err := fs.Sub(staticFS, "dist")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(distFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && strings.Contains(r.URL.Path, ".") {
			fileServer.ServeHTTP(w, r)
			return
		}
		index, err := fs.ReadFile(distFS, "index.html")
		if err != nil {
			http.Error(w, "failed to load index.html", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(index)
	})
}
