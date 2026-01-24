package main

import (
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"gestalt/internal/api"
	"gestalt/internal/logging"
)

const httpServerShutdownTimeout = 5 * time.Second

func listenOnPort(port int) (net.Listener, int, error) {
	address := ":" + strconv.Itoa(port)
	if port == 0 {
		address = ":0"
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, 0, err
	}
	tcpAddress, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		_ = listener.Close()
		return nil, 0, fmt.Errorf("unexpected listener address: %T", listener.Addr())
	}
	return listener, tcpAddress.Port, nil
}

func buildFrontendHandler(staticDir string, frontendFS fs.FS, backendURL *url.URL, authToken string, logger *logging.Logger) http.Handler {
	mux := http.NewServeMux()
	proxy := httputil.NewSingleHostReverseProxy(backendURL)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		if logger != nil {
			logger.Warn("frontend proxy error", map[string]string{
				"error": err.Error(),
			})
		}
		http.Error(w, "backend unavailable", http.StatusBadGateway)
	}

	mux.Handle("/api", proxy)
	mux.Handle("/api/", proxy)
	mux.Handle("/ws", proxy)
	mux.Handle("/ws/", proxy)

	if staticDir != "" {
		mux.Handle("/", api.NewSPAHandler(staticDir))
		return mux
	}

	if frontendFS != nil {
		mux.Handle("/", api.NewSPAHandlerFS(frontendFS))
		return mux
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if authToken != "" {
			w.Header().Set("X-Gestalt-Auth", "required")
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("gestalt ok\n"))
	})
	return mux
}

func findStaticDir() string {
	distPath := filepath.Join("gestalt", "dist")
	if info, err := os.Stat(distPath); err == nil && info.IsDir() {
		return distPath
	}

	return ""
}
