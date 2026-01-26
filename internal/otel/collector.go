package otel

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gestalt/internal/logging"
)

const (
	defaultCollectorBinary = "otelcol-gestalt"
	defaultStateDir        = ".gestalt"
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

type Collector struct {
	mu         sync.Mutex
	cmd        *exec.Cmd
	done       chan error
	stderr     *bytes.Buffer
	configPath string
	info       CollectorInfo
	logger     *logging.Logger
	options    Options
	binaryPath string
	rotateStop chan struct{}
	rotating   bool
}

var activeCollector struct {
	mu   sync.RWMutex
	info CollectorInfo
	ok   bool
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

	binaryPath, err := resolveCollectorBinary(options.BinaryPath)
	if err != nil {
		return nil, err
	}

	dataPath := filepath.Join(options.DataDir, "otel.json")
	if err := WriteCollectorConfig(options.ConfigPath, dataPath, options.GRPCEndpoint, options.HTTPEndpoint, options.RemoteEndpoint, options.RemoteInsecure, options.SelfMetricsEnabled); err != nil {
		return nil, err
	}

	cmd := exec.Command(binaryPath, "--config", options.ConfigPath)
	stderr := &bytes.Buffer{}
	cmd.Stdout = io.Discard
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	collector := &Collector{
		cmd:        cmd,
		done:       make(chan error, 1),
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
		logger:     options.Logger,
		options:    options,
		binaryPath: binaryPath,
		rotateStop: make(chan struct{}),
	}
	SetActiveCollector(collector.info)

	startFields := map[string]string{
		"path":   binaryPath,
		"config": options.ConfigPath,
	}
	if options.RemoteEndpoint != "" {
		startFields["remote_endpoint"] = options.RemoteEndpoint
		startFields["remote_insecure"] = strconv.FormatBool(options.RemoteInsecure)
	}
	collector.logInfo("otel collector started", startFields)

	go collector.waitForExit(cmd, collector.done)
	go collector.monitorRotation()
	return collector, nil
}

func (collector *Collector) Stop(ctx context.Context) error {
	if collector == nil {
		return nil
	}
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

func (collector *Collector) waitForExit(cmd *exec.Cmd, done chan error) {
	err := cmd.Wait()
	if done != nil {
		done <- err
	}
	collector.logExit(err)
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
	if err := collector.stopProcess(ctx, false); err != nil {
		collector.logWarn("otel collector stop failed for rotation", map[string]string{
			"error": err.Error(),
		})
	}

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
	options := collector.options
	binaryPath := collector.binaryPath
	dataPath := collector.info.DataPath
	collector.mu.Unlock()

	if err := WriteCollectorConfig(options.ConfigPath, dataPath, options.GRPCEndpoint, options.HTTPEndpoint, options.RemoteEndpoint, options.RemoteInsecure, options.SelfMetricsEnabled); err != nil {
		return err
	}

	cmd := exec.Command(binaryPath, "--config", options.ConfigPath)
	stderr := &bytes.Buffer{}
	cmd.Stdout = io.Discard
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return err
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
	collector.logInfo("otel collector restarted", map[string]string{
		"config": options.ConfigPath,
	})

	go collector.waitForExit(cmd, done)
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

func signalProcess(process *os.Process) error {
	if process == nil {
		return nil
	}
	if runtime.GOOS == "windows" {
		return process.Kill()
	}
	return process.Signal(os.Interrupt)
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
