package api

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gestalt/internal/metrics"
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
		TerminalCount:  len(terminals),
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

func (h *RestHandler) handleMetrics(w http.ResponseWriter, r *http.Request) *apiError {
	if r.Method != http.MethodGet {
		return methodNotAllowed(w, "GET")
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	if err := metrics.Default.WritePrometheus(w); err != nil {
		return &apiError{Status: http.StatusInternalServerError, Message: "failed to write metrics"}
	}
	return nil
}

func (h *RestHandler) handleEventDebug(w http.ResponseWriter, r *http.Request) *apiError {
	if r.Method != http.MethodGet {
		return methodNotAllowed(w, "GET")
	}
	snapshots := metrics.Default.EventBusSnapshots()
	response := make([]eventBusDebug, 0, len(snapshots))
	for _, snapshot := range snapshots {
		response = append(response, eventBusDebug{
			Name:                  snapshot.Name,
			FilteredSubscribers:   snapshot.FilteredSubscribers,
			UnfilteredSubscribers: snapshot.UnfilteredSubscribers,
		})
	}
	writeJSON(w, http.StatusOK, response)
	return nil
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
	gitDir := resolveGitDir(workDir)
	if gitDir == "" {
		return "", ""
	}
	origin := readGitOrigin(filepath.Join(gitDir, "config"))
	branch := readGitBranch(filepath.Join(gitDir, "HEAD"))
	return origin, branch
}

func resolveGitDir(workDir string) string {
	gitPath := filepath.Join(workDir, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return ""
	}
	if info.IsDir() {
		return gitPath
	}
	if !info.Mode().IsRegular() {
		return ""
	}
	contents, err := os.ReadFile(gitPath)
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(contents))
	const prefix = "gitdir:"
	if !strings.HasPrefix(line, prefix) {
		return ""
	}
	gitDir := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	if gitDir == "" {
		return ""
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(workDir, gitDir)
	}
	return gitDir
}

func readGitOrigin(configPath string) string {
	file, err := os.Open(configPath)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	section := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		if section != `remote "origin"` {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		if key != "url" {
			continue
		}
		return strings.TrimSpace(parts[1])
	}
	return ""
}

func readGitBranch(headPath string) string {
	contents, err := os.ReadFile(headPath)
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(contents))
	if line == "" {
		return ""
	}
	const prefix = "ref: "
	if strings.HasPrefix(line, prefix) {
		ref := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		return strings.TrimPrefix(ref, "refs/heads/")
	}
	short := line
	if len(short) > 12 {
		short = short[:12]
	}
	return fmt.Sprintf("detached@%s", short)
}
