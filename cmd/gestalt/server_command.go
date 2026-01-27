package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/http/pprof"
	"net/url"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"gestalt"
	"gestalt/internal/api"
	"gestalt/internal/app"
	"gestalt/internal/event"
	"gestalt/internal/logging"
	"gestalt/internal/otel"
	"gestalt/internal/ports"
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

	portRegistry := ports.NewPortRegistry()
	collectorOptions := otel.OptionsFromEnv(".gestalt")
	collectorOptions.Logger = logger
	grpcEndpointSet := false
	httpEndpointSet := false
	if _, ok := os.LookupEnv("GESTALT_OTEL_GRPC_ENDPOINT"); ok {
		grpcEndpointSet = true
	}
	if _, ok := os.LookupEnv("GESTALT_OTEL_HTTP_ENDPOINT"); ok {
		httpEndpointSet = true
	}
	if collectorOptions.Enabled && !grpcEndpointSet && !httpEndpointSet {
		grpcPort, httpPort, err := resolveOTelPorts(4317, 4318)
		if err != nil {
			if logger != nil {
				logger.Warn("otel port selection failed", map[string]string{
					"error": err.Error(),
				})
			}
		} else {
			collectorOptions.GRPCEndpoint = net.JoinHostPort("127.0.0.1", strconv.Itoa(grpcPort))
			collectorOptions.HTTPEndpoint = net.JoinHostPort("127.0.0.1", strconv.Itoa(httpPort))
			if logger != nil && (grpcPort != 4317 || httpPort != 4318) {
				logger.Info("otel collector ports selected", map[string]string{
					"grpc_port": strconv.Itoa(grpcPort),
					"http_port": strconv.Itoa(httpPort),
				})
			}
		}
	}
	collector, collectorErr := otel.StartCollector(collectorOptions)
	if collectorErr != nil {
		fields := map[string]string{
			"error": collectorErr.Error(),
		}
		if errors.Is(collectorErr, otel.ErrCollectorNotFound) {
			fields["path"] = collectorOptions.BinaryPath
			logger.Warn("otel collector unavailable", fields)
		} else {
			logger.Warn("otel collector start failed", fields)
		}
	}
	if collector != nil {
		if port, ok := parseEndpointPort(collectorOptions.HTTPEndpoint); ok {
			portRegistry.Set("otel", port)
		}
		defer func() {
			if err := otel.StopCollectorWithTimeout(collector, httpServerShutdownTimeout); err != nil && logger != nil {
				logger.Warn("otel collector shutdown failed", map[string]string{
					"error": err.Error(),
				})
			}
		}()
	}
	sdkOptions := otel.SDKOptionsFromEnv(".gestalt")
	sdkOptions.ServiceVersion = strings.TrimSpace(version.Version)
	if sdkOptions.ServiceVersion == "" {
		sdkOptions.ServiceVersion = "dev"
	}
	if collectorOptions.Enabled && strings.TrimSpace(collectorOptions.HTTPEndpoint) != "" {
		sdkOptions.HTTPEndpoint = collectorOptions.HTTPEndpoint
	}
	if logger != nil {
		logger.Info("otel endpoints configured", map[string]string{
			"otel collector http endpoint": strings.TrimSpace(collectorOptions.HTTPEndpoint),
			"otel sdk http endpoint":       strings.TrimSpace(sdkOptions.HTTPEndpoint),
		})
	}
	sdkShutdown, sdkErr := otel.SetupSDK(context.Background(), sdkOptions)
	if sdkErr != nil {
		logger.Warn("otel sdk init failed", map[string]string{
			"error": sdkErr.Error(),
		})
	} else if !sdkOptions.Enabled {
		sdkShutdown = nil
	} else if sdkShutdown != nil {
		defer func() {
			if err := sdkShutdown(context.Background()); err != nil && logger != nil {
				logger.Warn("otel sdk shutdown failed", map[string]string{
					"error": err.Error(),
				})
			}
		}()
	}
	if !sdkOptions.Enabled || sdkErr != nil {
		stopFallback := otel.StartLogHubFallback(logger, sdkOptions)
		defer stopFallback()
	}

	temporalDevServer, devServerError := startTemporalDevServer(&cfg, logger)
	if devServerError != nil {
		logger.Warn("temporal dev server start failed", map[string]string{
			"error": devServerError.Error(),
		})
	}
	if cfg.TemporalUIPort > 0 {
		portRegistry.Set("temporal", cfg.TemporalUIPort)
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
		PortResolver:         portRegistry,
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

	plansDir := preparePlanFile(logger)

	fsWatcher, err := watcher.NewWithOptions(watcher.Options{
		Logger:     logger,
		MaxWatches: cfg.MaxWatches,
		WatchDir:   true,
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
		planWatchPath := plansDir
		if workDir, err := os.Getwd(); err == nil {
			planWatchPath = filepath.Join(workDir, plansDir)
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
		watchPlanFile(eventBus, fsWatcher, logger, planWatchPath)
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
	if cfg.PprofEnabled {
		registerPprofHandlers(backendMux, logger)
	}
	api.RegisterRoutes(backendMux, manager, cfg.AuthToken, api.StatusConfig{
		TemporalUIPort: cfg.TemporalUIPort,
	}, "", nil, logger, eventBus)
	backendListener, backendPort, err := listenOnPort(cfg.BackendPort)
	if err != nil {
		logger.Error("backend listen failed", map[string]string{
			"error": err.Error(),
		})
		return 1
	}
	cfg.BackendPort = backendPort
	portRegistry.Set("backend", backendPort)
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
	portRegistry.Set("frontend", cfg.FrontendPort)
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

func registerPprofHandlers(mux *http.ServeMux, logger *logging.Logger) {
	if mux == nil {
		return
	}
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	mux.Handle("/debug/pprof/allocs", pprof.Handler("allocs"))
	mux.Handle("/debug/pprof/block", pprof.Handler("block"))
	mux.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	mux.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	mux.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))
	mux.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
	if logger != nil {
		logger.Info("pprof endpoints enabled", map[string]string{
			"path": "/debug/pprof/",
		})
	}
}

func parseEndpointPort(endpoint string) (int, bool) {
	trimmed := strings.TrimSpace(endpoint)
	if trimmed == "" {
		return 0, false
	}
	if port, err := strconv.Atoi(trimmed); err == nil && port > 0 {
		return port, true
	}
	if strings.HasPrefix(trimmed, ":") {
		trimmed = "localhost" + trimmed
	}
	_, portText, err := net.SplitHostPort(trimmed)
	if err != nil {
		return 0, false
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 {
		return 0, false
	}
	return port, true
}

func isPortAvailable(port int) bool {
	if port <= 0 || port > 65535 {
		return false
	}
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return false
	}
	_ = listener.Close()
	return true
}

func resolveOTelPorts(defaultGRPC, defaultHTTP int) (int, int, error) {
	if defaultGRPC <= 0 || defaultHTTP <= 0 {
		return 0, 0, fmt.Errorf("default ports must be positive")
	}
	if isPortAvailable(defaultGRPC) && isPortAvailable(defaultHTTP) {
		return defaultGRPC, defaultHTTP, nil
	}

	for attempt := 0; attempt < 10; attempt++ {
		grpcPort, err := pickRandomPort()
		if err != nil {
			return 0, 0, err
		}
		if grpcPort <= 0 || grpcPort >= 65535 {
			continue
		}
		httpPort := grpcPort + 1
		if !isPortAvailable(grpcPort) || !isPortAvailable(httpPort) {
			continue
		}
		return grpcPort, httpPort, nil
	}
	return 0, 0, fmt.Errorf("failed to select available OTel ports")
}
