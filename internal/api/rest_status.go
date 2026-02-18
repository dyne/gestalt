package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gestalt/internal/git"
	"gestalt/internal/otel"
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
	agentsSessionID, agentsTmuxSession := h.Manager.AgentsHubStatus()
	gitOrigin, gitBranch := h.gitInfo()
	versionInfo := version.GetVersionInfo()
	response := statusResponse{
		SessionCount:           len(terminals),
		ServerTime:             time.Now().UTC(),
		SessionPersist:         h.Manager.SessionPersistenceEnabled(),
		SessionScrollbackLines: h.SessionScrollbackLines,
		SessionFontFamily:      h.SessionFontFamily,
		SessionFontSize:        h.SessionFontSize,
		SessionInputFontFamily: h.SessionInputFontFamily,
		SessionInputFontSize:   h.SessionInputFontSize,
		AgentsSessionID:        agentsSessionID,
		AgentsTmuxSession:      agentsTmuxSession,
		WorkingDir:             workDir,
		GitOrigin:              gitOrigin,
		GitBranch:              gitBranch,
		Version:                versionInfo.Version,
		Major:                  versionInfo.Major,
		Minor:                  versionInfo.Minor,
		Patch:                  versionInfo.Patch,
		Built:                  versionInfo.Built,
		GitCommit:              versionInfo.GitCommit,
	}
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
