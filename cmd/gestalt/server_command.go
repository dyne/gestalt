package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"gestalt"
	"gestalt/internal/api"
	"gestalt/internal/app"
	"gestalt/internal/event"
	"gestalt/internal/logging"
	"gestalt/internal/prompt"
	"gestalt/internal/temporal"
	temporalworker "gestalt/internal/temporal/worker"
	"gestalt/internal/version"
	"gestalt/internal/watcher"
)

func runServer(args []string) int {
	cfg, err := loadConfig(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if cfg.ShowVersion {
		if version.Version == "" || version.Version == "dev" {
			fmt.Fprintln(os.Stdout, "gestalt dev")
		} else {
			fmt.Fprintf(os.Stdout, "gestalt version %s\n", version.Version)
		}
		return 0
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
		logStartupFlags(logger, cfg)
	}
	logVersionInfo(logger)
	ensureStateDir(cfg, logger)

	temporalDevServer, devServerError := startTemporalDevServer(&cfg, logger)
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
			waitForTemporalServer(cfg.TemporalHost, temporalDevServerStartTimeout, temporalDevServer.Done(), logger)
		} else {
			logTemporalServerHealth(logger, cfg.TemporalHost)
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

	configPaths, err := prepareConfig(cfg, logger)
	if err != nil {
		logger.Error("config extraction failed", map[string]string{
			"error": err.Error(),
		})
		return 1
	}

	if err := prepareScipAssets(logger); err != nil {
		logger.Warn("scip asset extraction failed", map[string]string{
			"error": err.Error(),
		})
	}

	if !cfg.DevMode {
		prompt.ValidatePromptFiles(configPaths.ConfigDir, logger)
	}

	configFS := buildConfigFS(configPaths.Root)
	configOverlay := configFS
	if shouldPreferLocalConfig(configPaths) {
		configOverlay = overlayFS{
			primary:  os.DirFS("."),
			fallback: configFS,
		}
	}

	buildResult, err := app.Build(app.BuildOptions{
		Logger:               logger,
		Shell:                cfg.Shell,
		ConfigFS:             configFS,
		ConfigOverlay:        configOverlay,
		ConfigRoot:           configPaths.SubDir,
		AgentsDir:            filepath.Join(configPaths.ConfigDir, "agents"),
		TemporalClient:       temporalClient,
		TemporalEnabled:      temporalEnabled,
		SessionLogDir:        cfg.SessionLogDir,
		InputHistoryDir:      cfg.InputHistoryDir,
		SessionRetentionDays: cfg.SessionRetentionDays,
		BufferLines:          cfg.SessionBufferLines,
	})
	if err != nil {
		var buildErr app.BuildError
		if errors.As(err, &buildErr) {
			switch buildErr.Stage {
			case app.StageLoadSkills:
				logger.Error("load skills failed", map[string]string{
					"error": buildErr.Err.Error(),
				})
				return 1
			case app.StageLoadAgents:
				logger.Error("load agents failed", map[string]string{
					"error": buildErr.Err.Error(),
				})
				return 1
			}
		}
		logger.Error("app build failed", map[string]string{
			"error": err.Error(),
		})
		return 1
	}
	logger.Info("skills loaded", map[string]string{
		"count": strconv.Itoa(len(buildResult.Skills)),
	})
	logger.Info("agents loaded", map[string]string{
		"count": strconv.Itoa(len(buildResult.Agents)),
	})
	manager := buildResult.Manager

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

	planPath := preparePlanFile(logger)

	fsWatcher, err := watcher.NewWithOptions(watcher.Options{
		Logger:     logger,
		MaxWatches: cfg.MaxWatches,
		WatchDir:   cfg.SCIPAutoReindex,
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
		watchPlanFile(eventBus, fsWatcher, logger, planPath)
		if workDir, err := os.Getwd(); err == nil {
			if _, err := watcher.StartGitWatcher(eventBus, fsWatcher, workDir); err != nil && logger != nil {
				logger.Warn("git watcher unavailable", map[string]string{
					"error": err.Error(),
				})
			}
			if cfg.SCIPAutoReindex {
				if _, err := watcher.WatchFile(eventBus, fsWatcher, workDir); err != nil && logger != nil {
					logger.Warn("scip watcher unavailable", map[string]string{
						"path":  workDir,
						"error": err.Error(),
					})
				}
			}
		} else if logger != nil {
			logger.Warn("git watcher unavailable", map[string]string{
				"error": err.Error(),
			})
		}
	}

	staticDir := findStaticDir()
	frontendFS := fs.FS(nil)
	if sub, err := fs.Sub(gestalt.EmbeddedFrontendFS, path.Join("frontend", "dist")); err == nil {
		frontendFS = sub
	} else if logger != nil {
		logger.Warn("embedded frontend unavailable", map[string]string{
			"error": err.Error(),
		})
	}

	backendMux := http.NewServeMux()
	api.RegisterRoutes(backendMux, manager, cfg.AuthToken, api.StatusConfig{
		TemporalUIPort: cfg.TemporalUIPort,
	}, cfg.SCIPIndexPath, cfg.SCIPAutoReindex, "", nil, logger, eventBus)
	backendListener, backendPort, err := listenOnPort(cfg.BackendPort)
	if err != nil {
		logger.Error("backend listen failed", map[string]string{
			"error": err.Error(),
		})
		return 1
	}
	cfg.BackendPort = backendPort
	backendServer := &http.Server{
		Handler:           backendMux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	backendAddress := backendListener.Addr().String()
	logger.Info("gestalt backend listening", map[string]string{
		"addr":    backendAddress,
		"version": version.Version,
	})

	backendURL := &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("localhost:%d", backendPort),
	}
	frontendHandler := buildFrontendHandler(staticDir, frontendFS, backendURL, cfg.AuthToken, logger)
	frontendServer := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.FrontendPort),
		Handler:           frontendHandler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	logger.Info("gestalt frontend listening", map[string]string{
		"addr":    frontendServer.Addr,
		"version": version.Version,
	})

	stopSignals := make(chan os.Signal, 1)
	signal.Notify(stopSignals, os.Interrupt, syscall.SIGTERM)

	runner := &ServerRunner{
		Logger:          logger,
		ShutdownTimeout: httpServerShutdownTimeout,
	}
	runner.Run(stopSignals,
		ManagedServer{
			Name: "backend",
			Serve: func() error {
				return backendServer.Serve(backendListener)
			},
			Shutdown: backendServer.Shutdown,
		},
		ManagedServer{
			Name:     "frontend",
			Serve:    frontendServer.ListenAndServe,
			Shutdown: frontendServer.Shutdown,
		},
	)
	return 0
}
