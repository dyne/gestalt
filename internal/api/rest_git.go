package api

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"gestalt/internal/gitlog"
)

const gitLogTimeout = 2 * time.Second

func (h *RestHandler) handleGitLog(w http.ResponseWriter, r *http.Request) *apiError {
	if r.Method != http.MethodGet {
		return methodNotAllowed(w, "GET")
	}
	if h.GitLogReader == nil {
		return &apiError{Status: http.StatusServiceUnavailable, Message: "git log unavailable"}
	}

	limit := gitlog.DefaultLimit
	limitParam := r.URL.Query().Get("limit")
	if limitParam != "" {
		parsed, err := strconv.Atoi(limitParam)
		if err != nil {
			return &apiError{Status: http.StatusBadRequest, Message: "invalid limit"}
		}
		if parsed <= 0 || parsed > gitlog.MaxLimit {
			return &apiError{Status: http.StatusBadRequest, Message: "invalid limit"}
		}
		limit = parsed
	}

	workDir, err := os.Getwd()
	if err != nil {
		if h.Logger != nil {
			h.Logger.Warn("failed to get working directory for git log", map[string]string{
				"error": err.Error(),
			})
		}
		return &apiError{Status: http.StatusServiceUnavailable, Message: "git log unavailable"}
	}

	ctx, cancel := context.WithTimeout(r.Context(), gitLogTimeout)
	defer cancel()

	result, err := h.GitLogReader.Recent(ctx, workDir, gitlog.Options{
		Limit:             limit,
		MaxFilesPerCommit: gitlog.DefaultMaxFilesPerCommit,
	})
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
			return &apiError{Status: http.StatusServiceUnavailable, Message: "git log timed out"}
		case errors.Is(err, gitlog.ErrNotGitRepo), errors.Is(err, gitlog.ErrEmptyRepo):
			writeJSON(w, http.StatusOK, gitLogResponse{Branch: "", Commits: []gitLogCommit{}})
			return nil
		default:
			return &apiError{Status: http.StatusServiceUnavailable, Message: "git log unavailable"}
		}
	}

	if h.Logger != nil && len(result.Warnings) > 0 {
		h.Logger.Warn("git log parse warnings", map[string]string{
			"warnings": strings.Join(result.Warnings, "; "),
		})
	}
	writeJSON(w, http.StatusOK, convertGitLogResponse(result))
	return nil
}

func convertGitLogResponse(result gitlog.Result) gitLogResponse {
	commits := make([]gitLogCommit, 0, len(result.Commits))
	for _, commit := range result.Commits {
		files := make([]gitLogFile, 0, len(commit.Files))
		for _, file := range commit.Files {
			files = append(files, gitLogFile{
				Path:    file.Path,
				Added:   file.Added,
				Deleted: file.Deleted,
				Binary:  file.Binary,
			})
		}
		commits = append(commits, gitLogCommit{
			SHA:         commit.SHA,
			ShortSHA:    commit.ShortSHA,
			CommittedAt: commit.CommittedAt,
			Subject:     commit.Subject,
			Stats: gitLogCommitStats{
				FilesChanged: commit.Stats.FilesChanged,
				LinesAdded:   commit.Stats.LinesAdded,
				LinesDeleted: commit.Stats.LinesDeleted,
				HasBinary:    commit.Stats.HasBinary,
			},
			Files:          files,
			FilesTruncated: commit.FilesTruncated,
		})
	}
	return gitLogResponse{
		Branch:  result.Branch,
		Commits: commits,
	}
}
