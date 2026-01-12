package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path"
	"time"

	"gestalt"
	"gestalt/internal/api"
	"gestalt/internal/event"
	"gestalt/internal/logging"
	"gestalt/internal/server"
	"gestalt/internal/temporal"
	temporalworker "gestalt/internal/temporal/worker"
	"gestalt/internal/terminal"
	"gestalt/internal/version"
	"gestalt/internal/watcher"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

const httpServerShutdownTimeout = 5 * time.Second

func main() {
	cfg, err := server.LoadConfig(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if cfg.ShowVersion {
		if version.Version == "" || version.Version == "dev" {
			fmt.Fprintln(os.Stdout, "gestalt-desktop dev")
		} else {
			fmt.Fprintf(os.Stdout, "gestalt-desktop version %s\n", version.Version)
		}
		return
	}

	logBuffer := logging.NewLogBuffer(logging.DefaultBufferSize)
	logLevel := logging.LevelInfo
	if cfg.Verbose {
		logLevel = logging.LevelDebug
	} else if cfg.Quiet {
		logLevel = logging.LevelWarning
	}
	logger := logging.NewLogger(logBuffer, logLevel)
	if cfg.Verbose {
		server.LogStartupFlags(logger, cfg)
	}
	server.LogVersionInfo(logger)
	server.EnsureStateDir(cfg, logger)

	temporalDevServer, devServerError := server.StartTemporalDevServer(&cfg, logger)
	if devServerError != nil {
		logger.Warn("temporal dev server start failed", map[string]string{
			"error": devServerError.Error(),
		})
	}
	if temporalDevServer != nil {
		defer temporalDevServer.Stop(logger)
	}
	if cfg.TemporalDevServer && !cfg.TemporalEnabled {
		logger.Warn("temporal dev server running while workflows disabled", nil)
	}

	temporalEnabled := cfg.TemporalEnabled
	var temporalClient temporal.WorkflowClient
	if temporalEnabled {
		if temporalDevServer != nil {
			server.WaitForTemporalServer(cfg.TemporalHost, 10*time.Second, temporalDevServer.Done(), logger)
		} else {
			server.LogTemporalServerHealth(logger, cfg.TemporalHost)
		}
		var temporalClientError error
		temporalClient, temporalClientError = temporal.NewClient(temporal.ClientConfig{
			HostPort:  cfg.TemporalHost,
			Namespace: cfg.TemporalNamespace,
		})
		if temporalClientError != nil {
			temporalEnabled = false
			logger.Warn("temporal client unavailable", map[string]string{
				"host":      cfg.TemporalHost,
				"namespace": cfg.TemporalNamespace,
				"error":     temporalClientError.Error(),
			})
		} else if temporalClient != nil {
			defer temporalClient.Close()
			logger.Info("temporal client connected", map[string]string{
				"host":      cfg.TemporalHost,
				"namespace": cfg.TemporalNamespace,
			})
		}
	}

	configPaths, err := server.PrepareConfig(cfg, logger)
	if err != nil {
		logger.Error("config extraction failed", map[string]string{
			"error": err.Error(),
		})
		os.Exit(1)
	}

	if !cfg.DevMode {
		server.ValidatePromptFiles(configPaths.ConfigDir, logger)
	}

	configFS := server.BuildConfigFS(configPaths.Root)
	skills, err := server.LoadSkills(logger, configFS, configPaths.SubDir)
	if err != nil {
		logger.Error("load skills failed", map[string]string{
			"error": err.Error(),
		})
		os.Exit(1)
	}
	logger.Info("skills loaded", map[string]string{
		"count": fmt.Sprintf("%d", len(skills)),
	})

	agents, err := server.LoadAgents(logger, configFS, configPaths.SubDir, server.BuildSkillIndex(skills))
	if err != nil {
		logger.Error("load agents failed", map[string]string{
			"error": err.Error(),
		})
		os.Exit(1)
	}
	logger.Info("agents loaded", map[string]string{
		"count": fmt.Sprintf("%d", len(agents)),
	})

	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell:                cfg.Shell,
		Agents:               agents,
		Skills:               skills,
		Logger:               logger,
		TemporalClient:       temporalClient,
		TemporalEnabled:      temporalEnabled,
		SessionLogDir:        cfg.SessionLogDir,
		InputHistoryDir:      cfg.InputHistoryDir,
		SessionRetentionDays: cfg.SessionRetentionDays,
		BufferLines:          cfg.SessionBufferLines,
		PromptFS:             configFS,
		PromptDir:            path.Join(configPaths.SubDir, "prompts"),
	})

	workerStarted := false
	if temporalEnabled && temporalClient != nil {
		workerError := temporalworker.StartWorker(temporalClient, manager)
		if workerError != nil {
			logger.Warn("temporal worker start failed", map[string]string{
				"error": workerError.Error(),
			})
		} else {
			workerStarted = true
		}
	}
	if workerStarted {
		defer temporalworker.StopWorker()
	}

	planPath := server.PreparePlanFile(logger)

	fsWatcher, err := watcher.NewWithOptions(watcher.Options{
		Logger:     logger,
		MaxWatches: cfg.MaxWatches,
	})
	if err != nil && logger != nil {
		logger.Warn("filesystem watcher unavailable", map[string]string{
			"error": err.Error(),
		})
	}
	eventBus := event.NewBus[watcher.Event](context.Background(), event.BusOptions{
		Name: "watcher_events",
	})
	if fsWatcher != nil {
		fsWatcher.SetErrorHandler(func(err error) {
			eventBus.Publish(watcher.Event{
				Type:      watcher.EventTypeWatchError,
				Timestamp: time.Now().UTC(),
			})
		})
		server.WatchPlanFile(eventBus, fsWatcher, logger, planPath)
		if workDir, err := os.Getwd(); err == nil {
			if _, err := watcher.StartGitWatcher(eventBus, fsWatcher, workDir); err != nil && logger != nil {
				logger.Warn("git watcher unavailable", map[string]string{
					"error": err.Error(),
				})
			}
		} else if logger != nil {
			logger.Warn("git watcher unavailable", map[string]string{
				"error": err.Error(),
			})
		}
	}

	backendMux := http.NewServeMux()
	api.RegisterRoutes(backendMux, manager, cfg.AuthToken, api.StatusConfig{
		TemporalUIPort: cfg.TemporalUIPort,
	}, "", nil, logger, eventBus)

	backendListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		logger.Error("backend listen failed", map[string]string{
			"error": err.Error(),
		})
		os.Exit(1)
	}
	backendServer := &http.Server{
		Handler:           backendMux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	backendAddress := backendListener.Addr().String()
	logger.Info("gestalt backend listening", map[string]string{
		"addr":    backendAddress,
		"version": version.Version,
	})

	backendURL := fmt.Sprintf("http://%s", backendAddress)

	frontendFS := fs.FS(nil)
	if sub, err := fs.Sub(gestalt.EmbeddedDesktopFrontendFS, path.Join("frontend", "build")); err == nil {
		frontendFS = sub
	} else if logger != nil {
		logger.Warn("embedded desktop frontend unavailable", map[string]string{
			"error": err.Error(),
		})
	}
	if frontendFS == nil {
		logger.Error("embedded desktop frontend missing", nil)
		os.Exit(1)
	}

	go func() {
		if err := backendServer.Serve(backendListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("backend server stopped", map[string]string{
				"error": err.Error(),
			})
		}
	}()

	app := NewApp(backendURL, manager, backendServer, logger)

	err = wails.Run(&options.App{
		Title:  "Gestalt",
		Width:  1400,
		Height: 900,
		AssetServer: &assetserver.Options{
			Assets: frontendFS,
		},
		OnStartup:     app.startup,
		OnShutdown:    app.shutdown,
		OnBeforeClose: app.beforeClose,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		logger.Error("wails run failed", map[string]string{
			"error": err.Error(),
		})
		shutdownContext, cancel := context.WithTimeout(context.Background(), httpServerShutdownTimeout)
		defer cancel()
		_ = backendServer.Shutdown(shutdownContext)
		os.Exit(1)
	}
}
