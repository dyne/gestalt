package server

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gestalt/internal/logging"
)

const temporalDefaultHost = "localhost:7233"
const temporalHealthCheckTimeout = 500 * time.Millisecond
const temporalDevServerStartTimeout = 10 * time.Second
const temporalDevServerStopTimeout = 5 * time.Second

type TemporalDevServer struct {
	cmd     *exec.Cmd
	logFile *os.File
	done    chan error
}

func StartTemporalDevServer(cfg *Config, logger *logging.Logger) (*TemporalDevServer, error) {
	if cfg == nil || !cfg.TemporalDevServer {
		return nil, nil
	}
	temporalPath, err := exec.LookPath("temporal")
	if err != nil {
		return nil, fmt.Errorf("temporal CLI not found")
	}

	dataDir := filepath.Join(".gestalt", "temporal")
	absDataDir, err := filepath.Abs(dataDir)
	if err != nil {
		absDataDir = dataDir
	}
	cacheDir := filepath.Join(absDataDir, "cache")
	configDir := filepath.Join(absDataDir, "config")
	stateDir := filepath.Join(absDataDir, "state")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("create temporal cache dir: %w", err)
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return nil, fmt.Errorf("create temporal config dir: %w", err)
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return nil, fmt.Errorf("create temporal state dir: %w", err)
	}

	logPath := filepath.Join(absDataDir, "temporal.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open temporal log: %w", err)
	}

	temporalPort, uiPort, err := resolveTemporalDevPorts(cfg, logger)
	if err != nil {
		_ = logFile.Close()
		return nil, err
	}

	cmd := exec.Command(temporalPath, "server", "start-dev",
		"--ip", "0.0.0.0",
		"--port", strconv.Itoa(temporalPort),
		"--ui-port", strconv.Itoa(uiPort),
	)
	cfg.TemporalUIPort = uiPort
	cmd.Dir = absDataDir
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = append(os.Environ(),
		"HOME="+absDataDir,
		"XDG_CACHE_HOME="+cacheDir,
		"XDG_CONFIG_HOME="+configDir,
		"XDG_STATE_HOME="+stateDir,
	)
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("start temporal dev server: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	if logger != nil {
		logger.Info("temporal dev server started", map[string]string{
			"dir":     absDataDir,
			"log":     logPath,
			"host":    normalizeTemporalHost(cfg.TemporalHost),
			"ui_port": strconv.Itoa(uiPort),
		})
	}

	return &TemporalDevServer{
		cmd:     cmd,
		logFile: logFile,
		done:    done,
	}, nil
}

func resolveTemporalDevPorts(cfg *Config, logger *logging.Logger) (int, int, error) {
	if cfg == nil {
		return 0, 0, fmt.Errorf("missing temporal config")
	}
	temporalPort := 0
	temporalHostSource := sourceDefault
	if cfg.Sources != nil {
		temporalHostSource = cfg.Sources["temporal-host"]
	}
	if temporalHostSource != sourceDefault && strings.TrimSpace(cfg.TemporalHost) != "" {
		if _, port, err := net.SplitHostPort(cfg.TemporalHost); err == nil {
			if parsed, err := strconv.Atoi(port); err == nil && parsed > 0 {
				temporalPort = parsed
			}
		} else if logger != nil {
			logger.Warn("temporal host missing port; using random port", map[string]string{
				"host": cfg.TemporalHost,
			})
		}
	}

	if temporalPort == 0 {
		port, err := pickRandomPort()
		if err != nil {
			return 0, 0, fmt.Errorf("select temporal port: %w", err)
		}
		temporalPort = port
		cfg.TemporalHost = fmt.Sprintf("localhost:%d", temporalPort)
	}

	uiPort, err := pickRandomPortExcluding(temporalPort)
	if err != nil {
		return 0, 0, fmt.Errorf("select temporal UI port: %w", err)
	}
	return temporalPort, uiPort, nil
}

func pickRandomPort() (int, error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()

	tcpAddress, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected listener address: %T", listener.Addr())
	}
	return tcpAddress.Port, nil
}

func pickRandomPortExcluding(excluded int) (int, error) {
	for attempt := 0; attempt < 10; attempt++ {
		port, err := pickRandomPort()
		if err != nil {
			return 0, err
		}
		if port != excluded {
			return port, nil
		}
	}
	return 0, fmt.Errorf("failed to select distinct port")
}

func (server *TemporalDevServer) Done() <-chan error {
	if server == nil {
		return nil
	}
	return server.done
}

func (server *TemporalDevServer) Stop(logger *logging.Logger) {
	if server == nil {
		return
	}
	if server.cmd == nil || server.cmd.Process == nil {
		if server.logFile != nil {
			_ = server.logFile.Close()
		}
		return
	}

	select {
	case err := <-server.done:
		if logger != nil && err != nil {
			logger.Warn("temporal dev server exited", map[string]string{
				"error": err.Error(),
			})
		}
	default:
		if err := server.cmd.Process.Signal(os.Interrupt); err != nil && logger != nil {
			logger.Warn("temporal dev server signal failed", map[string]string{
				"error": err.Error(),
			})
		}
		select {
		case err := <-server.done:
			if logger != nil && err != nil {
				logger.Warn("temporal dev server stopped", map[string]string{
					"error": err.Error(),
				})
			}
		case <-time.After(temporalDevServerStopTimeout):
			if killErr := server.cmd.Process.Kill(); killErr != nil && logger != nil {
				logger.Warn("temporal dev server kill failed", map[string]string{
					"error": killErr.Error(),
				})
			}
		}
	}

	if server.logFile != nil {
		_ = server.logFile.Close()
	}
}

func LogTemporalServerHealth(logger *logging.Logger, host string) {
	if logger == nil {
		return
	}
	if err := temporalServerReachable(host); err != nil {
		logger.Warn("temporal server unavailable", map[string]string{
			"host":  normalizeTemporalHost(host),
			"error": err.Error(),
		})
		return
	}
	logger.Info("temporal server reachable", map[string]string{
		"host": normalizeTemporalHost(host),
	})
}

func WaitForTemporalServer(host string, timeout time.Duration, done <-chan error, logger *logging.Logger) bool {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		if err := temporalServerReachable(host); err == nil {
			if logger != nil {
				logger.Info("temporal server ready", map[string]string{
					"host": normalizeTemporalHost(host),
				})
			}
			return true
		}

		if time.Now().After(deadline) {
			if logger != nil {
				logger.Warn("temporal server wait timed out", map[string]string{
					"host": normalizeTemporalHost(host),
				})
			}
			return false
		}

		select {
		case err := <-done:
			if logger != nil {
				message := "temporal dev server exited"
				fields := map[string]string{}
				if err != nil {
					fields["error"] = err.Error()
				}
				logger.Warn(message, fields)
			}
			return false
		case <-ticker.C:
		}
	}
}

func temporalServerReachable(host string) error {
	address := normalizeTemporalHost(host)
	dialer := net.Dialer{Timeout: temporalHealthCheckTimeout}
	connection, dialError := dialer.Dial("tcp", address)
	if dialError != nil {
		return dialError
	}
	if err := connection.Close(); err != nil {
		return err
	}
	return nil
}

func normalizeTemporalHost(host string) string {
	address := strings.TrimSpace(host)
	if address == "" {
		return temporalDefaultHost
	}
	return address
}
