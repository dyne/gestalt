package api

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
)

type SPAHandler struct {
	fs         fs.FS
	fileServer http.Handler
	indexPath  string
}

func NewSPAHandler(staticDir string) *SPAHandler {
	return NewSPAHandlerFS(os.DirFS(staticDir))
}

func NewSPAHandlerFS(staticFS fs.FS) *SPAHandler {
	httpFS := http.FS(staticFS)
	return &SPAHandler{
		fs:         staticFS,
		fileServer: http.FileServer(httpFS),
		indexPath:  "index.html",
	}
}

func (h *SPAHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requested := path.Clean(r.URL.Path)
	if requested == "." || requested == "/" {
		r.URL.Path = "/"
		h.fileServer.ServeHTTP(w, r)
		return
	}

	requested = strings.TrimPrefix(requested, "/")
	if requested == "" {
		r.URL.Path = "/"
		h.fileServer.ServeHTTP(w, r)
		return
	}

	if info, err := fs.Stat(h.fs, requested); err == nil && !info.IsDir() {
		r.URL.Path = "/" + requested
		h.fileServer.ServeHTTP(w, r)
		return
	}

	r.URL.Path = "/"
	h.fileServer.ServeHTTP(w, r)
}
