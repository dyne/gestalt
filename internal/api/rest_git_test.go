package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gestalt/internal/gitlog"
)

type stubGitLogReader struct {
	result gitlog.Result
	err    error
	opts   gitlog.Options
}

func (reader *stubGitLogReader) Recent(_ context.Context, _ string, opts gitlog.Options) (gitlog.Result, error) {
	reader.opts = opts
	if reader.err != nil {
		return gitlog.Result{}, reader.err
	}
	return reader.result, nil
}

func TestGitLogEndpointReturnsPayload(t *testing.T) {
	added := 7
	deleted := 2
	reader := &stubGitLogReader{
		result: gitlog.Result{
			Branch: "feature/demo",
			Commits: []gitlog.Commit{
				{
					SHA:         "1111111111111111111111111111111111111111",
					ShortSHA:    "111111111111",
					CommittedAt: "2026-02-18T00:00:00Z",
					Subject:     "feat(ui): add git log",
					Stats: gitlog.CommitStats{
						FilesChanged: 1,
						LinesAdded:   7,
						LinesDeleted: 2,
					},
					Files: []gitlog.FileStat{
						{
							Path:    "frontend/src/views/Dashboard.svelte",
							Added:   &added,
							Deleted: &deleted,
							Binary:  false,
						},
					},
				},
			},
		},
	}
	handler := &RestHandler{GitLogReader: reader}

	req := httptest.NewRequest(http.MethodGet, "/api/git/log?limit=12", nil)
	rec := httptest.NewRecorder()
	restHandler("", nil, handler.handleGitLog)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	var payload gitLogResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Branch != "feature/demo" {
		t.Fatalf("unexpected branch %q", payload.Branch)
	}
	if len(payload.Commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(payload.Commits))
	}
	if reader.opts.Limit != 12 {
		t.Fatalf("expected reader limit 12, got %d", reader.opts.Limit)
	}
}

func TestGitLogEndpointInvalidLimit(t *testing.T) {
	handler := &RestHandler{GitLogReader: &stubGitLogReader{}}
	req := httptest.NewRequest(http.MethodGet, "/api/git/log?limit=nope", nil)
	rec := httptest.NewRecorder()
	restHandler("", nil, handler.handleGitLog)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestGitLogEndpointNotGitRepo(t *testing.T) {
	handler := &RestHandler{GitLogReader: &stubGitLogReader{err: gitlog.ErrNotGitRepo}}
	req := httptest.NewRequest(http.MethodGet, "/api/git/log", nil)
	rec := httptest.NewRecorder()
	restHandler("", nil, handler.handleGitLog)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	var payload gitLogResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Branch != "" || len(payload.Commits) != 0 {
		t.Fatalf("expected empty payload, got %#v", payload)
	}
}

func TestGitLogEndpointTimeout(t *testing.T) {
	handler := &RestHandler{GitLogReader: &stubGitLogReader{err: context.DeadlineExceeded}}
	req := httptest.NewRequest(http.MethodGet, "/api/git/log", nil)
	rec := httptest.NewRecorder()
	restHandler("", nil, handler.handleGitLog)(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", rec.Code)
	}
}

func TestGitLogEndpointMethodNotAllowed(t *testing.T) {
	handler := &RestHandler{GitLogReader: &stubGitLogReader{}}
	req := httptest.NewRequest(http.MethodPost, "/api/git/log", nil)
	rec := httptest.NewRecorder()
	restHandler("", nil, handler.handleGitLog)(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", rec.Code)
	}
	if allow := rec.Header().Get("Allow"); allow != "GET" {
		t.Fatalf("expected Allow GET, got %q", allow)
	}
}

func TestGitLogEndpointUnavailable(t *testing.T) {
	handler := &RestHandler{GitLogReader: &stubGitLogReader{err: errors.New("git failed")}}
	req := httptest.NewRequest(http.MethodGet, "/api/git/log", nil)
	rec := httptest.NewRecorder()
	restHandler("", nil, handler.handleGitLog)(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", rec.Code)
	}
}

func TestGitLogEndpointUsesDefaultLimit(t *testing.T) {
	reader := &stubGitLogReader{result: gitlog.Result{}}
	handler := &RestHandler{GitLogReader: reader}
	req := httptest.NewRequest(http.MethodGet, "/api/git/log", nil)
	rec := httptest.NewRecorder()
	restHandler("", nil, handler.handleGitLog)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if reader.opts.Limit != gitlog.DefaultLimit {
		t.Fatalf("expected default limit %d, got %d", gitlog.DefaultLimit, reader.opts.Limit)
	}
}

func TestGitLogEndpointRejectsOverMaxLimit(t *testing.T) {
	handler := &RestHandler{GitLogReader: &stubGitLogReader{}}
	req := httptest.NewRequest(http.MethodGet, "/api/git/log?limit=999", nil)
	rec := httptest.NewRecorder()
	restHandler("", nil, handler.handleGitLog)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestGitLogEndpointTimeoutContextCanceled(t *testing.T) {
	handler := &RestHandler{GitLogReader: &stubGitLogReader{err: context.Canceled}}
	req := httptest.NewRequest(http.MethodGet, "/api/git/log", nil)
	rec := httptest.NewRecorder()
	restHandler("", nil, handler.handleGitLog)(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", rec.Code)
	}
}

func TestGitLogTimeoutConstant(t *testing.T) {
	if gitLogTimeout != 2*time.Second {
		t.Fatalf("expected git log timeout 2s, got %s", gitLogTimeout)
	}
}
