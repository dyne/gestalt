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
		setSecurityHeaders(w, cacheControlNoCache)
		r.URL.Path = "/"
		h.fileServer.ServeHTTP(w, r)
		return
	}

	requested = strings.TrimPrefix(requested, "/")
	if requested == "" {
		setSecurityHeaders(w, cacheControlNoCache)
		r.URL.Path = "/"
		h.fileServer.ServeHTTP(w, r)
		return
	}

	if info, err := fs.Stat(h.fs, requested); err == nil && !info.IsDir() {
		if strings.HasSuffix(requested, ".html") {
			setSecurityHeaders(w, cacheControlNoCache)
		} else if isHashedAsset(requested) {
			setSecurityHeaders(w, cacheControlImmutable)
		} else {
			setSecurityHeaders(w, cacheControlNoCache)
		}
		r.URL.Path = "/" + requested
		h.fileServer.ServeHTTP(w, r)
		return
	}

	setSecurityHeaders(w, cacheControlNoCache)
	r.URL.Path = "/"
	h.fileServer.ServeHTTP(w, r)
}

func isHashedAsset(filePath string) bool {
	name := path.Base(filePath)
	extIndex := strings.LastIndex(name, ".")
	if extIndex <= 0 {
		return false
	}
	base := name[:extIndex]
	dashIndex := strings.LastIndex(base, "-")
	if dashIndex < 0 || dashIndex == len(base)-1 {
		return false
	}
	hash := base[dashIndex+1:]
	if len(hash) < 8 {
		return false
	}
	for _, char := range hash {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') && (char < 'A' || char > 'F') {
			return false
		}
	}
	return true
}
