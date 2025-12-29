package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"gestalt/internal/api"
	"gestalt/internal/terminal"
)

type Config struct {
	Port      int
	Shell     string
	AuthToken string
}

func main() {
	cfg := loadConfig()

	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell: cfg.Shell,
	})

	staticDir := findStaticDir()

	mux := http.NewServeMux()
	api.RegisterRoutes(mux, manager, cfg.AuthToken, staticDir)

	server := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.Port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("gestalt listening on %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}

func loadConfig() Config {
	port := 8080
	if rawPort := os.Getenv("GESTALT_PORT"); rawPort != "" {
		if parsed, err := strconv.Atoi(rawPort); err == nil {
			port = parsed
		}
	}

	shell := os.Getenv("GESTALT_SHELL")
	if shell == "" {
		shell = terminal.DefaultShell()
	}

	return Config{
		Port:      port,
		Shell:     shell,
		AuthToken: os.Getenv("GESTALT_TOKEN"),
	}
}

func findStaticDir() string {
	distPath := filepath.Join("frontend", "dist")
	if info, err := os.Stat(distPath); err == nil && info.IsDir() {
		return distPath
	}

	return ""
}
