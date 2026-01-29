package api

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gestalt/internal/git"
	"gestalt/internal/otel"
	"gestalt/internal/temporal"
	"gestalt/internal/version"
)

func (h *RestHandler) handleStatus(w http.ResponseWriter, r *http.Request) *apiError {
	if err := h.requireManager(); err != nil {
		return err
	}

	workDir, err := os.Getwd()
	if err != nil {
		workDir = "unknown"
		if h.Logger != nil {
			h.Logger.Warn("failed to get working directory", map[string]string{
				"error": err.Error(),
			})
		}
	}

	terminals := h.Manager.List()
	gitOrigin, gitBranch := h.gitInfo()
	versionInfo := version.GetVersionInfo()
	response := statusResponse{
		SessionCount:   len(terminals),
		ServerTime:     time.Now().UTC(),
		SessionPersist: h.Manager.SessionPersistenceEnabled(),
		WorkingDir:     workDir,
		GitOrigin:      gitOrigin,
		GitBranch:      gitBranch,
		Version:        versionInfo.Version,
		Major:          versionInfo.Major,
		Minor:          versionInfo.Minor,
		Patch:          versionInfo.Patch,
		Built:          versionInfo.Built,
		GitCommit:      versionInfo.GitCommit,
		TemporalUIURL:  buildTemporalUIURL(r, h.TemporalUIPort),
		TemporalHost:   strings.TrimSpace(h.TemporalHost),
	}
	temporalStatus := temporal.DevServerStatusSnapshot()
	response.TemporalDevServerRunning = temporalStatus.Running
	response.TemporalDevServerPID = temporalStatus.PID
	collectorStatus := otel.CollectorStatusSnapshot()
	response.OTelCollectorRunning = collectorStatus.Running
	response.OTelCollectorPID = collectorStatus.PID
	response.OTelCollectorHTTPEndpoint = collectorStatus.HTTPEndpoint
	response.OTelCollectorRestartCount = collectorStatus.RestartCount
	if !collectorStatus.LastExitTime.IsZero() {
		lastExit := collectorStatus.LastExitTime.Format(time.RFC3339)
		if strings.TrimSpace(collectorStatus.LastExitErr) != "" {
			lastExit = fmt.Sprintf("%s: %s", lastExit, strings.TrimSpace(collectorStatus.LastExitErr))
		}
		response.OTelCollectorLastExit = lastExit
	}

	writeJSON(w, http.StatusOK, response)
	return nil
}

func buildTemporalUIURL(r *http.Request, uiPort int) string {
	if uiPort <= 0 || r == nil {
		return ""
	}
	host := forwardedHeaderValue(r, "X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if idx := strings.Index(host, ","); idx >= 0 {
		host = strings.TrimSpace(host[:idx])
	}
	hostname := host
	if splitHost, _, err := net.SplitHostPort(host); err == nil {
		hostname = splitHost
	}
	if strings.TrimSpace(hostname) == "" {
		return ""
	}
	scheme := forwardedHeaderValue(r, "X-Forwarded-Proto")
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	return fmt.Sprintf("%s://%s", scheme, net.JoinHostPort(hostname, strconv.Itoa(uiPort)))
}

func forwardedHeaderValue(r *http.Request, header string) string {
	if r == nil {
		return ""
	}
	value := strings.TrimSpace(r.Header.Get(header))
	if value == "" {
		return ""
	}
	if idx := strings.Index(value, ","); idx >= 0 {
		value = strings.TrimSpace(value[:idx])
	}
	return value
}

func (h *RestHandler) setGitBranch(branch string) {
	if h == nil {
		return
	}
	h.gitMutex.Lock()
	h.GitBranch = branch
	h.gitMutex.Unlock()
}

func (h *RestHandler) gitInfo() (string, string) {
	if h == nil {
		return "", ""
	}

	origin := h.GitOrigin
	branch := h.GitBranch
	if origin != "" || branch != "" {
		return origin, branch
	}

	if origin == "" && branch == "" {
		origin, branch = loadGitInfo()
		if origin != "" || branch != "" {
			h.gitMutex.Lock()
			h.GitOrigin = origin
			h.GitBranch = branch
			h.gitMutex.Unlock()
		}
	}

	return origin, branch
}

func loadGitInfo() (string, string) {
	workDir, err := os.Getwd()
	if err != nil {
		return "", ""
	}
	gitDir := git.ResolveGitDir(workDir)
	if gitDir == "" {
		return "", ""
	}
	origin := git.ReadGitOrigin(filepath.Join(gitDir, "config"))
	branch := git.ReadGitBranch(filepath.Join(gitDir, "HEAD"))
	return origin, branch
}
