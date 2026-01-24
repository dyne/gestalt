package otel

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	Enabled        bool
	BinaryPath     string
	ConfigPath     string
	DataDir        string
	GRPCEndpoint   string
	HTTPEndpoint   string
	RemoteEndpoint string
	RemoteInsecure bool
	Logger         *logging.Logger
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
	cmd        *exec.Cmd
	done       chan error
	stderr     *bytes.Buffer
	configPath string
	info       CollectorInfo
	logger     *logging.Logger
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
	if err := WriteCollectorConfig(options.ConfigPath, dataPath, options.GRPCEndpoint, options.HTTPEndpoint, options.RemoteEndpoint, options.RemoteInsecure); err != nil {
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
		logger: options.Logger,
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

	go collector.waitForExit()
	return collector, nil
}

func (collector *Collector) Stop(ctx context.Context) error {
	if collector == nil || collector.cmd == nil || collector.cmd.Process == nil {
		return nil
	}
	collector.logInfo("otel collector stopping", map[string]string{
		"config": collector.configPath,
	})

	if err := signalProcess(collector.cmd.Process); err != nil {
		collector.logWarn("otel collector signal failed", map[string]string{
			"error": err.Error(),
		})
	}

	select {
	case err := <-collector.done:
		ClearActiveCollector()
		return err
	case <-ctx.Done():
		_ = collector.cmd.Process.Kill()
		<-collector.done
		ClearActiveCollector()
		return ctx.Err()
	}
}

func (collector *Collector) waitForExit() {
	err := collector.cmd.Wait()
	collector.done <- err
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
