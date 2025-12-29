package main

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"gestalt/internal/agent"
	"gestalt/internal/api"
	"gestalt/internal/logging"
	"gestalt/internal/terminal"
)

type Config struct {
	Port      int
	Shell     string
	AuthToken string
}

func main() {
	cfg := loadConfig()
	logBuffer := logging.NewLogBuffer(logging.DefaultBufferSize)
	logger := logging.NewLogger(logBuffer, logging.LevelInfo)

	agents, err := loadAgents(logger)
	if err != nil {
		logger.Error("load agents failed", map[string]string{
			"error": err.Error(),
		})
		os.Exit(1)
	}
	logger.Info("agents loaded", map[string]string{
		"count": strconv.Itoa(len(agents)),
	})

	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell:  cfg.Shell,
		Agents: agents,
		Logger: logger,
	})

	staticDir := findStaticDir()

	mux := http.NewServeMux()
	api.RegisterRoutes(mux, manager, cfg.AuthToken, staticDir, logger)

	server := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.Port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	logger.Info("gestalt listening", map[string]string{
		"addr": server.Addr,
	})
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("http server stopped", map[string]string{
			"error": err.Error(),
		})
	}
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

func loadAgents(logger *logging.Logger) (map[string]agent.Agent, error) {
	loader := agent.Loader{Logger: logger}
	return loader.Load(filepath.Join("config", "agents"), filepath.Join("config", "prompts"))
}
