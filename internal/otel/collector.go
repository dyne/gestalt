package otel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"gestalt/internal/logging"
)

const (
	defaultCollectorBinary  = "otelcol-gestalt"
	defaultStateDir         = ".gestalt"
	collectorPIDFileName    = "collector.pid"
	collectorReadyTimeout   = 3 * time.Second
	collectorReadyInterval  = 100 * time.Millisecond
	collectorReadyDialWait  = 200 * time.Millisecond
	collectorStderrTailSize = 8192
	collectorRestartBase    = 500 * time.Millisecond
	collectorRestartMax     = 8 * time.Second
	collectorRestartLimit   = 5
)

var ErrCollectorNotFound = errors.New("otel collector binary not found")

type Options struct {
	Enabled            bool
	BinaryPath         string
	ConfigPath         string
	DataDir            string
	GRPCEndpoint       string
	HTTPEndpoint       string
	RemoteEndpoint     string
	RemoteInsecure     bool
	SelfMetricsEnabled bool
	Logger             *logging.Logger
}

type CollectorInfo struct {
	StartTime      time.Time
	ConfigPath     string
	DataPath       string
	GRPCEndpoint   string
	HTTPEndpoint   string
	RemoteEndpoint string
	RemoteInsecure bool
}

type CollectorStatus struct {
	PID          int
	Running      bool
	StartTime    time.Time
	LastExitTime time.Time
	LastExitErr  string
	StderrTail   string
	RestartCount int
	HTTPEndpoint string
}

type Collector struct {
	mu              sync.Mutex
	cmd             *exec.Cmd
	done            chan error
	exit            chan collectorExit
	stderr          *tailBuffer
	configPath      string
	info            CollectorInfo
	logger          *logging.Logger
	options         Options
	binaryPath      string
	pidPath         string
	rotateStop      chan struct{}
	rotating        bool
	intentionalStop bool
	supervising     bool
	restartBase     time.Duration
	restartMax      time.Duration
	restartLimit    int
	restartHook     func() error
}

var activeCollector struct {
	mu   sync.RWMutex
	info CollectorInfo
	ok   bool
}

var activeCollectorStatus struct {
	mu     sync.RWMutex
	status CollectorStatus
}

type collectorExit struct {
	err         error
	pid         int
	stderrTail  string
	intentional bool
}

func OptionsFromEnv(stateDir string) Options {
	if strings.TrimSpace(stateDir) == "" {
		stateDir = defaultStateDir
	}
	opts := Options{
		Enabled:        true,
		BinaryPath:     strings.TrimSpace(os.Getenv("GESTALT_OTEL_COLLECTOR")),
		ConfigPath:     strings.TrimSpace(os.Getenv("GESTALT_OTEL_CONFIG")),
		DataDir:        strings.TrimSpace(os.Getenv("GESTALT_OTEL_DATA_DIR")),
		GRPCEndpoint:   strings.TrimSpace(os.Getenv("GESTALT_OTEL_GRPC_ENDPOINT")),
		HTTPEndpoint:   strings.TrimSpace(os.Getenv("GESTALT_OTEL_HTTP_ENDPOINT")),
		RemoteEndpoint: strings.TrimSpace(os.Getenv("GESTALT_OTEL_REMOTE_ENDPOINT")),
	}
	if rawEnabled, ok := os.LookupEnv("GESTALT_OTEL_ENABLED"); ok {
		if parsed, err := strconv.ParseBool(strings.TrimSpace(rawEnabled)); err == nil {
			opts.Enabled = parsed
		}
	}
	if rawSelfMetrics, ok := os.LookupEnv("GESTALT_OTEL_SELF_METRICS"); ok {
		if parsed, err := strconv.ParseBool(strings.TrimSpace(rawSelfMetrics)); err == nil {
			opts.SelfMetricsEnabled = parsed
		}
	}
	if rawInsecure, ok := os.LookupEnv("GESTALT_OTEL_REMOTE_INSECURE"); ok {
		if parsed, err := strconv.ParseBool(strings.TrimSpace(rawInsecure)); err == nil {
			opts.RemoteInsecure = parsed
		}
	}
	if opts.DataDir == "" {
		opts.DataDir = filepath.Join(stateDir, "otel")
	}
	if opts.ConfigPath == "" {
		opts.ConfigPath = filepath.Join(opts.DataDir, "collector.yaml")
	}
	if opts.GRPCEndpoint == "" {
		opts.GRPCEndpoint = defaultGRPCEndpoint
	}
	if opts.HTTPEndpoint == "" {
		opts.HTTPEndpoint = defaultHTTPEndpoint
	}
	return opts
}

func StartCollector(options Options) (*Collector, error) {
	if !options.Enabled {
		return nil, nil
	}

	pidPath := collectorPIDPath(options.DataDir)
	stopCollectorFromPID(pidPath, options.Logger)

	binaryPath, err := resolveCollectorBinary(options.BinaryPath)
	if err != nil {
		return nil, err
	}

	dataPath := filepath.Join(options.DataDir, "otel.json")
	if err := WriteCollectorConfig(options.ConfigPath, dataPath, options.GRPCEndpoint, options.HTTPEndpoint, options.RemoteEndpoint, options.RemoteInsecure, options.SelfMetricsEnabled); err != nil {
		return nil, err
	}

	cmd := exec.Command(binaryPath, "--config", options.ConfigPath)
	stderr := newTailBuffer(collectorStderrTailSize)
	cmd.Stdout = io.Discard
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	if err := writeCollectorPID(pidPath, cmd.Process.Pid); err != nil && options.Logger != nil {
		options.Logger.Warn("otel collector pid write failed", map[string]string{
			"error": err.Error(),
			"path":  pidPath,
		})
	}

	collector := &Collector{
		cmd:        cmd,
		done:       make(chan error, 1),
		exit:       make(chan collectorExit, 1),
		stderr:     stderr,
		configPath: options.ConfigPath,
		info: CollectorInfo{
			StartTime:      time.Now().UTC(),
			ConfigPath:     options.ConfigPath,
			DataPath:       dataPath,
			GRPCEndpoint:   options.GRPCEndpoint,
			HTTPEndpoint:   options.HTTPEndpoint,
			RemoteEndpoint: options.RemoteEndpoint,
			RemoteInsecure: options.RemoteInsecure,
		},
		logger:       options.Logger,
		options:      options,
		binaryPath:   binaryPath,
		pidPath:      pidPath,
		rotateStop:   make(chan struct{}),
		restartBase:  collectorRestartBase,
		restartMax:   collectorRestartMax,
		restartLimit: collectorRestartLimit,
	}
	readyExit := make(chan error, 1)

	go collector.waitForExit(cmd, collector.done, readyExit)
	if err := waitForCollectorReady(collector.info.HTTPEndpoint, readyExit, collectorReadyTimeout); err != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		collector.setIntentionalStop(true)
		_ = collector.stopProcess(ctx, false)
		collector.setIntentionalStop(false)
		stderrTail := collectorStderrTail(collector.stderr, collectorStderrTailSize)
		if stderrTail != "" {
			return nil, fmt.Errorf("otel collector not ready: %w: %s", err, stderrTail)
		}
		return nil, fmt.Errorf("otel collector not ready: %w", err)
	}
	SetActiveCollector(collector.info)
	updateCollectorStatus(func(status *CollectorStatus) {
		status.PID = cmd.Process.Pid
		status.Running = true
		status.StartTime = collector.info.StartTime
		status.HTTPEndpoint = collector.info.HTTPEndpoint
		status.RestartCount = 0
		status.LastExitTime = time.Time{}
		status.LastExitErr = ""
		status.StderrTail = ""
	})

	startFields := map[string]string{
		"path":   binaryPath,
		"config": options.ConfigPath,
	}
	if options.RemoteEndpoint != "" {
		startFields["remote_endpoint"] = options.RemoteEndpoint
		startFields["remote_insecure"] = strconv.FormatBool(options.RemoteInsecure)
	}
	collector.logInfo("otel collector started", startFields)
	go collector.monitorRotation()
	return collector, nil
}

func (collector *Collector) Stop(ctx context.Context) error {
	if collector == nil {
		return nil
	}
	collector.setIntentionalStop(true)
	defer collector.setIntentionalStop(false)
	closeRotation := func() {
		collector.mu.Lock()
		if collector.rotateStop != nil {
			close(collector.rotateStop)
			collector.rotateStop = nil
		}
		collector.mu.Unlock()
	}
	closeRotation()

	return collector.stopProcess(ctx, true)
}

func (collector *Collector) StartSupervision(ctx context.Context) {
	if collector == nil {
		return
	}
	collector.mu.Lock()
	if collector.supervising {
		collector.mu.Unlock()
		return
	}
	collector.supervising = true
	exit := collector.exit
	collector.mu.Unlock()

	go collector.supervise(ctx, exit)
}

func (collector *Collector) waitForExit(cmd *exec.Cmd, done chan error, notify chan<- error) {
	err := cmd.Wait()
	if done != nil {
		done <- err
	}
	if notify != nil {
		select {
		case notify <- err:
		default:
		}
	}
	exitEvent := collectorExit{
		err:         err,
		pid:         processID(cmd),
		stderrTail:  collectorStderrTail(collector.stderr, collectorStderrTailSize),
		intentional: collector.isIntentionalStop(),
	}
	if collector.exit != nil {
		select {
		case collector.exit <- exitEvent:
		default:
		}
	}
	stderrTail := exitEvent.stderrTail
	updateCollectorStatus(func(status *CollectorStatus) {
		status.Running = false
		status.PID = 0
		status.LastExitTime = time.Now().UTC()
		if err != nil {
			status.LastExitErr = err.Error()
			if stderrTail != "" {
				status.StderrTail = stderrTail
			}
		} else {
			status.LastExitErr = ""
			status.StderrTail = ""
		}
	})
	collector.logExit(err)
	collector.clearActiveIfCurrent(cmd)
	if cmd != nil && cmd.Process != nil {
		removeCollectorPID(collector.pidPath, cmd.Process.Pid)
	}
}

func (collector *Collector) logExit(err error) {
	if collector == nil || collector.logger == nil {
		return
	}
	fields := map[string]string{
		"config": collector.configPath,
	}
	if err != nil {
		fields["error"] = err.Error()
		if collector.stderr != nil {
			stderr := strings.TrimSpace(collector.stderr.String())
			if stderr != "" {
				fields["stderr"] = stderr
			}
		}
		collector.logger.Warn("otel collector exited", fields)
		return
	}
	collector.logger.Info("otel collector exited", fields)
}

func (collector *Collector) clearActiveIfCurrent(cmd *exec.Cmd) {
	if collector == nil {
		return
	}
	collector.mu.Lock()
	isCurrent := collector.cmd == cmd
	collector.mu.Unlock()
	if !isCurrent {
		return
	}
	ClearActiveCollector()
}

func (collector *Collector) logInfo(message string, fields map[string]string) {
	if collector == nil || collector.logger == nil {
		return
	}
	collector.logger.Info(message, fields)
}

func (collector *Collector) logWarn(message string, fields map[string]string) {
	if collector == nil || collector.logger == nil {
		return
	}
	collector.logger.Warn(message, fields)
}

func (collector *Collector) setIntentionalStop(value bool) {
	if collector == nil {
		return
	}
	collector.mu.Lock()
	collector.intentionalStop = value
	collector.mu.Unlock()
}

func (collector *Collector) isIntentionalStop() bool {
	if collector == nil {
		return false
	}
	collector.mu.Lock()
	value := collector.intentionalStop
	collector.mu.Unlock()
	return value
}

func (collector *Collector) stopProcess(ctx context.Context, clearActive bool) error {
	collector.mu.Lock()
	cmd := collector.cmd
	done := collector.done
	configPath := collector.configPath
	collector.mu.Unlock()

	if cmd == nil || cmd.Process == nil {
		return nil
	}
	collector.logInfo("otel collector stopping", map[string]string{
		"config": configPath,
	})

	if err := signalProcess(cmd.Process); err != nil {
		collector.logWarn("otel collector signal failed", map[string]string{
			"error": err.Error(),
		})
	}

	select {
	case err := <-done:
		if clearActive {
			ClearActiveCollector()
		}
		return err
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		if done != nil {
			<-done
		}
		if clearActive {
			ClearActiveCollector()
		}
		return ctx.Err()
	}
}

func (collector *Collector) supervise(ctx context.Context, exit <-chan collectorExit) {
	if collector == nil || exit == nil {
		return
	}
	collector.mu.Lock()
	base := collector.restartBase
	max := collector.restartMax
	limit := collector.restartLimit
	collector.mu.Unlock()
	if base <= 0 {
		base = collectorRestartBase
	}
	if max <= 0 {
		max = collectorRestartMax
	}
	if limit <= 0 {
		limit = collectorRestartLimit
	}

	backoff := base
	restarts := 0

	for {
		select {
		case <-ctx.Done():
			return
		case evt := <-exit:
			if evt.intentional {
				restarts = 0
				backoff = base
				continue
			}
			for {
				if ctx.Err() != nil {
					return
				}
				if restarts >= limit {
					collector.logWarn("otel collector supervision stopped", map[string]string{
						"reason":        "restart_limit",
						"restart_limit": strconv.Itoa(limit),
					})
					return
				}
				restarts++
				fields := map[string]string{
					"attempt": strconv.Itoa(restarts),
					"backoff": backoff.String(),
				}
				if evt.pid > 0 {
					fields["pid"] = strconv.Itoa(evt.pid)
				}
				if evt.err != nil {
					fields["error"] = evt.err.Error()
				}
				if evt.stderrTail != "" {
					fields["stderr_tail"] = evt.stderrTail
				}
				collector.logWarn("otel collector exited unexpectedly, restarting", fields)

				timer := time.NewTimer(backoff)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
				if err := collector.restartProcess(); err != nil {
					collector.logWarn("otel collector restart failed", map[string]string{
						"error":   err.Error(),
						"attempt": strconv.Itoa(restarts),
					})
					backoff = nextBackoff(backoff, max)
					continue
				}
				restarts = 0
				backoff = base
				break
			}
		}
	}
}

func (collector *Collector) monitorRotation() {
	config := rotationConfigFromEnv()
	if !config.Enabled() {
		return
	}
	ticker := time.NewTicker(config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			collector.rotateIfNeeded(config)
		case <-collector.rotateStop:
			return
		}
	}
}

func (collector *Collector) rotateIfNeeded(config rotationConfig) {
	if collector == nil {
		return
	}
	collector.mu.Lock()
	if collector.rotating || collector.rotateStop == nil {
		collector.mu.Unlock()
		return
	}
	dataPath := collector.info.DataPath
	collector.rotating = true
	collector.mu.Unlock()

	defer func() {
		collector.mu.Lock()
		collector.rotating = false
		collector.mu.Unlock()
	}()

	if strings.TrimSpace(dataPath) == "" {
		return
	}
	info, err := os.Stat(dataPath)
	if err != nil {
		return
	}
	if !shouldRotate(info, config) {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	collector.setIntentionalStop(true)
	if err := collector.stopProcess(ctx, false); err != nil {
		collector.logWarn("otel collector stop failed for rotation", map[string]string{
			"error": err.Error(),
		})
	}
	collector.setIntentionalStop(false)

	rotatedPath, err := rotateCollectorFile(dataPath)
	if err != nil {
		collector.logWarn("otel collector rotation failed", map[string]string{
			"error": err.Error(),
		})
	} else if rotatedPath != "" {
		collector.logInfo("otel collector rotated file", map[string]string{
			"path": rotatedPath,
		})
	}

	pruneRotatedFiles(filepath.Dir(dataPath), config)

	if err := collector.restartProcess(); err != nil {
		collector.logWarn("otel collector restart failed", map[string]string{
			"error": err.Error(),
		})
	}
}

func (collector *Collector) restartProcess() error {
	collector.mu.Lock()
	restartHook := collector.restartHook
	options := collector.options
	binaryPath := collector.binaryPath
	dataPath := collector.info.DataPath
	pidPath := collector.pidPath
	collector.mu.Unlock()

	if restartHook != nil {
		return restartHook()
	}

	if err := WriteCollectorConfig(options.ConfigPath, dataPath, options.GRPCEndpoint, options.HTTPEndpoint, options.RemoteEndpoint, options.RemoteInsecure, options.SelfMetricsEnabled); err != nil {
		return err
	}

	cmd := exec.Command(binaryPath, "--config", options.ConfigPath)
	stderr := newTailBuffer(collectorStderrTailSize)
	cmd.Stdout = io.Discard
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	if err := writeCollectorPID(pidPath, cmd.Process.Pid); err != nil && options.Logger != nil {
		options.Logger.Warn("otel collector pid write failed", map[string]string{
			"error": err.Error(),
			"path":  pidPath,
		})
	}
	done := make(chan error, 1)

	collector.mu.Lock()
	collector.cmd = cmd
	collector.done = done
	collector.stderr = stderr
	collector.configPath = options.ConfigPath
	collector.info = CollectorInfo{
		StartTime:      time.Now().UTC(),
		ConfigPath:     options.ConfigPath,
		DataPath:       dataPath,
		GRPCEndpoint:   options.GRPCEndpoint,
		HTTPEndpoint:   options.HTTPEndpoint,
		RemoteEndpoint: options.RemoteEndpoint,
		RemoteInsecure: options.RemoteInsecure,
	}
	collector.logger = options.Logger
	collector.mu.Unlock()

	SetActiveCollector(collector.info)
	updateCollectorStatus(func(status *CollectorStatus) {
		status.PID = cmd.Process.Pid
		status.Running = true
		status.StartTime = collector.info.StartTime
		status.HTTPEndpoint = collector.info.HTTPEndpoint
		status.RestartCount++
	})
	collector.logInfo("otel collector restarted", map[string]string{
		"config": options.ConfigPath,
	})

	go collector.waitForExit(cmd, done, nil)
	return nil
}

type rotationConfig struct {
	MaxBytes      int64
	MaxFiles      int
	MaxAge        time.Duration
	CheckInterval time.Duration
}

func (config rotationConfig) Enabled() bool {
	return config.MaxBytes > 0 || config.MaxAge > 0
}

func rotationConfigFromEnv() rotationConfig {
	return rotationConfig{
		MaxBytes:      parseRotationBytes(os.Getenv("GESTALT_OTEL_FILE_MAX_BYTES"), 512*1024*1024),
		MaxFiles:      parseRotationInt(os.Getenv("GESTALT_OTEL_FILE_MAX_FILES"), 10),
		MaxAge:        parseRotationAge(os.Getenv("GESTALT_OTEL_FILE_MAX_AGE"), 7*24*time.Hour),
		CheckInterval: 1 * time.Minute,
	}
}

func parseRotationBytes(raw string, fallback int64) int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func parseRotationInt(raw string, fallback int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func parseRotationAge(raw string, fallback time.Duration) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	if strings.HasSuffix(raw, "d") {
		days := strings.TrimSuffix(raw, "d")
		if parsed, err := strconv.Atoi(strings.TrimSpace(days)); err == nil && parsed > 0 {
			return time.Duration(parsed) * 24 * time.Hour
		}
	}
	if parsed, err := time.ParseDuration(raw); err == nil && parsed > 0 {
		return parsed
	}
	if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
		return time.Duration(parsed) * 24 * time.Hour
	}
	return fallback
}

func shouldRotate(info os.FileInfo, config rotationConfig) bool {
	if info == nil {
		return false
	}
	if config.MaxBytes > 0 && info.Size() >= config.MaxBytes {
		return true
	}
	if config.MaxAge > 0 && time.Since(info.ModTime()) >= config.MaxAge {
		return true
	}
	return false
}

func rotateCollectorFile(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", nil
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	dir := filepath.Dir(path)
	rotated := filepath.Join(dir, fmt.Sprintf("otel-%s.json", time.Now().UTC().Format("20060102-150405")))
	if err := os.Rename(path, rotated); err != nil {
		return "", err
	}
	return rotated, nil
}

func pruneRotatedFiles(dir string, config rotationConfig) {
	if dir == "" {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	now := time.Now()
	files := make([]fileInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "otel-") || !strings.HasSuffix(name, ".json") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		path := filepath.Join(dir, name)
		if config.MaxAge > 0 && now.Sub(info.ModTime()) > config.MaxAge {
			_ = os.Remove(path)
			continue
		}
		files = append(files, fileInfo{path: path, modTime: info.ModTime()})
	}

	if config.MaxFiles > 0 && len(files) > config.MaxFiles {
		sort.Slice(files, func(i, j int) bool {
			return files[i].modTime.After(files[j].modTime)
		})
		for _, entry := range files[config.MaxFiles:] {
			_ = os.Remove(entry.path)
		}
	}
}

type fileInfo struct {
	path    string
	modTime time.Time
}

func resolveCollectorBinary(explicitPath string) (string, error) {
	candidate := strings.TrimSpace(explicitPath)
	if candidate != "" {
		if hasBinary(candidate) {
			return candidate, nil
		}
		return "", ErrCollectorNotFound
	}

	if path, err := exec.LookPath(defaultCollectorBinary); err == nil && hasBinary(path) {
		return path, nil
	}

	if exe, err := os.Executable(); err == nil {
		path := filepath.Join(filepath.Dir(exe), defaultCollectorBinary)
		if hasBinary(path) {
			return path, nil
		}
	}

	if hasBinary(filepath.Join("otel", defaultCollectorBinary)) {
		return filepath.Join("otel", defaultCollectorBinary), nil
	}

	return "", ErrCollectorNotFound
}

func hasBinary(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return strings.HasSuffix(strings.ToLower(path), ".exe") || hasBinary(path+".exe")
	}
	return info.Mode()&0o111 != 0
}

func collectorPIDPath(dataDir string) string {
	if strings.TrimSpace(dataDir) == "" {
		return ""
	}
	return filepath.Join(dataDir, collectorPIDFileName)
}

func readCollectorPID(pidPath string) (int, error) {
	if strings.TrimSpace(pidPath) == "" {
		return 0, os.ErrNotExist
	}
	raw, err := os.ReadFile(pidPath)
	if err != nil {
		return 0, err
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return 0, fmt.Errorf("empty pid file")
	}
	pid, err := strconv.Atoi(trimmed)
	if err != nil || pid <= 0 {
		return 0, fmt.Errorf("invalid pid")
	}
	return pid, nil
}

func writeCollectorPID(pidPath string, pid int) error {
	if strings.TrimSpace(pidPath) == "" || pid <= 0 {
		return fmt.Errorf("invalid pid path")
	}
	if err := os.MkdirAll(filepath.Dir(pidPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(pidPath, []byte(strconv.Itoa(pid)), 0o644)
}

func removeCollectorPID(pidPath string, pid int) {
	if strings.TrimSpace(pidPath) == "" || pid <= 0 {
		return
	}
	currentPID, err := readCollectorPID(pidPath)
	if err != nil || currentPID != pid {
		return
	}
	_ = os.Remove(pidPath)
}

func stopCollectorFromPID(pidPath string, logger *logging.Logger) {
	pid, err := readCollectorPID(pidPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return
		}
		_ = os.Remove(pidPath)
		return
	}
	if !isProcessAlive(pid) {
		_ = os.Remove(pidPath)
		return
	}
	if logger != nil {
		logger.Warn("otel collector pid found, stopping existing process", map[string]string{
			"path": pidPath,
			"pid":  strconv.Itoa(pid),
		})
	}
	process, err := os.FindProcess(pid)
	if err == nil {
		if err := signalProcess(process); err != nil && logger != nil {
			logger.Warn("otel collector pid signal failed", map[string]string{
				"error": err.Error(),
				"pid":   strconv.Itoa(pid),
			})
		}
		if !waitForProcessExit(pid, 5*time.Second) && logger != nil {
			logger.Warn("otel collector pid did not exit after signal", map[string]string{
				"pid": strconv.Itoa(pid),
			})
		}
		if isProcessAlive(pid) {
			if err := process.Kill(); err != nil && logger != nil {
				logger.Warn("otel collector pid kill failed", map[string]string{
					"error": err.Error(),
					"pid":   strconv.Itoa(pid),
				})
			} else {
				_ = waitForProcessExit(pid, 2*time.Second)
			}
		}
	}
	if isProcessAlive(pid) && logger != nil {
		logger.Warn("otel collector pid still running after stop attempt", map[string]string{
			"pid": strconv.Itoa(pid),
		})
	}
	_ = os.Remove(pidPath)
}

func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return process.Signal(syscall.Signal(0)) == nil
}

func waitForProcessExit(pid int, timeout time.Duration) bool {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !isProcessAlive(pid) {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return !isProcessAlive(pid)
}

func signalProcess(process *os.Process) error {
	if process == nil {
		return nil
	}
	if runtime.GOOS == "windows" {
		return process.Kill()
	}
	return process.Signal(os.Interrupt)
}

func processID(cmd *exec.Cmd) int {
	if cmd == nil || cmd.Process == nil {
		return 0
	}
	return cmd.Process.Pid
}

func nextBackoff(current, max time.Duration) time.Duration {
	if current <= 0 {
		return collectorRestartBase
	}
	next := current * 2
	if max > 0 && next > max {
		return max
	}
	return next
}

func waitForCollectorReady(endpoint string, exit <-chan error, timeout time.Duration) error {
	address, err := normalizeDialAddress(endpoint)
	if err != nil {
		return err
	}
	if timeout <= 0 {
		timeout = collectorReadyTimeout
	}
	deadline := time.Now().Add(timeout)
	dialer := net.Dialer{Timeout: collectorReadyDialWait}

	for {
		select {
		case err := <-exit:
			if err != nil {
				return fmt.Errorf("collector exited before ready: %w", err)
			}
			return errors.New("collector exited before ready")
		default:
		}

		conn, err := dialer.Dial("tcp", address)
		if err == nil {
			_ = conn.Close()
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("collector not ready after %s", timeout)
		}
		select {
		case err := <-exit:
			if err != nil {
				return fmt.Errorf("collector exited before ready: %w", err)
			}
			return errors.New("collector exited before ready")
		case <-time.After(collectorReadyInterval):
		}
	}
}

func normalizeDialAddress(endpoint string) (string, error) {
	trimmed := strings.TrimSpace(endpoint)
	if trimmed == "" {
		trimmed = defaultHTTPEndpoint
	}
	if port, err := strconv.Atoi(trimmed); err == nil && port > 0 {
		return net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), nil
	}
	if strings.HasPrefix(trimmed, ":") {
		trimmed = "127.0.0.1" + trimmed
	}
	if _, _, err := net.SplitHostPort(trimmed); err != nil {
		return "", err
	}
	return trimmed, nil
}

type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func newTailBuffer(max int) *tailBuffer {
	if max <= 0 {
		max = collectorStderrTailSize
	}
	return &tailBuffer{max: max}
}

func (buffer *tailBuffer) Write(data []byte) (int, error) {
	if buffer == nil {
		return len(data), nil
	}
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	if buffer.max <= 0 {
		return len(data), nil
	}
	if len(data) >= buffer.max {
		buffer.buf = append(buffer.buf[:0], data[len(data)-buffer.max:]...)
		return len(data), nil
	}
	if len(buffer.buf)+len(data) > buffer.max {
		over := len(buffer.buf) + len(data) - buffer.max
		buffer.buf = append(buffer.buf[over:], data...)
		return len(data), nil
	}
	buffer.buf = append(buffer.buf, data...)
	return len(data), nil
}

func (buffer *tailBuffer) Bytes() []byte {
	if buffer == nil {
		return nil
	}
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	if len(buffer.buf) == 0 {
		return nil
	}
	out := make([]byte, len(buffer.buf))
	copy(out, buffer.buf)
	return out
}

func (buffer *tailBuffer) String() string {
	return string(buffer.Bytes())
}

func collectorStderrTail(buffer *tailBuffer, max int) string {
	if buffer == nil || max <= 0 {
		return ""
	}
	data := buffer.Bytes()
	if len(data) <= max {
		return strings.TrimSpace(string(data))
	}
	return strings.TrimSpace(string(data[len(data)-max:]))
}

func StopCollectorWithTimeout(collector *Collector, timeout time.Duration) error {
	if collector == nil {
		return nil
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return collector.Stop(ctx)
}

func ActiveCollector() (CollectorInfo, bool) {
	activeCollector.mu.RLock()
	defer activeCollector.mu.RUnlock()
	return activeCollector.info, activeCollector.ok
}

func CollectorStatusSnapshot() CollectorStatus {
	activeCollectorStatus.mu.RLock()
	defer activeCollectorStatus.mu.RUnlock()
	return activeCollectorStatus.status
}

func SetCollectorStatus(status CollectorStatus) {
	activeCollectorStatus.mu.Lock()
	activeCollectorStatus.status = status
	activeCollectorStatus.mu.Unlock()
}

func ClearCollectorStatus() {
	activeCollectorStatus.mu.Lock()
	activeCollectorStatus.status = CollectorStatus{}
	activeCollectorStatus.mu.Unlock()
}

func SetActiveCollector(info CollectorInfo) {
	activeCollector.mu.Lock()
	activeCollector.info = info
	activeCollector.ok = true
	activeCollector.mu.Unlock()
}

func ClearActiveCollector() {
	activeCollector.mu.Lock()
	activeCollector.info = CollectorInfo{}
	activeCollector.ok = false
	activeCollector.mu.Unlock()
}

func updateCollectorStatus(update func(*CollectorStatus)) {
	if update == nil {
		return
	}
	activeCollectorStatus.mu.Lock()
	update(&activeCollectorStatus.status)
	activeCollectorStatus.mu.Unlock()
}
