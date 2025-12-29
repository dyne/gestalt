package api

import (
	"net/http"
	"os"
	"path/filepath"
)

type SPAHandler struct {
	staticDir string
	indexPath string
}

func NewSPAHandler(staticDir string) *SPAHandler {
	return &SPAHandler{
		staticDir: staticDir,
		indexPath: "index.html",
	}
}

func (h *SPAHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := filepath.Clean(r.URL.Path)
	if path == "." || path == "/" {
		path = "/" + h.indexPath
	}

	requested := filepath.Join(h.staticDir, path)
	info, err := os.Stat(requested)
	if err == nil && !info.IsDir() {
		http.ServeFile(w, r, requested)
		return
	}

	index := filepath.Join(h.staticDir, h.indexPath)
	http.ServeFile(w, r, index)
}
