//go:build !noscip

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gestalt/internal/scip"

	scipproto "github.com/sourcegraph/scip/bindings/go/scip"
	"google.golang.org/protobuf/proto"
)

func TestSCIPReIndexMissingPath(t *testing.T) {
	handler := newTestSCIPHandler(t)

	body := bytes.NewBufferString(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/api/scip/index", body)
	res := httptest.NewRecorder()
	restHandler("", handler.ReIndex)(res, req)

	assertAPIError(t, res, http.StatusBadRequest, "path is required")
}

func TestSCIPReIndexSkipsRecent(t *testing.T) {
	handler := newTestSCIPHandler(t)

	if err := os.WriteFile(handler.indexPath, []byte(""), 0o644); err != nil {
		t.Fatalf("write scip index: %v", err)
	}

	meta := scip.IndexMetadata{
		CreatedAt:   time.Now().UTC(),
		ProjectRoot: "/tmp/repo",
		Languages:   []string{"go"},
		FilesHashed: "hash",
	}
	if err := scip.SaveMetadata(handler.indexPath, meta); err != nil {
		t.Fatalf("SaveMetadata failed: %v", err)
	}

	handler.detectLangs = func(string) ([]string, error) {
		t.Fatal("expected recent index check to skip indexing")
		return nil, nil
	}

	body := bytes.NewBufferString(`{"path":"/tmp/repo"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/scip/index", body)
	res := httptest.NewRecorder()
	restHandler("", handler.ReIndex)(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.Code)
	}

	var payload struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Status != "recent" {
		t.Fatalf("expected status recent, got %s", payload.Status)
	}
	if !strings.Contains(payload.Message, "Use force") {
		t.Fatalf("expected warning message, got %s", payload.Message)
	}
}

func TestSCIPReIndexStarts(t *testing.T) {
	handler := newTestSCIPHandler(t)

	done := make(chan struct{})
	handler.asyncIndexer.SetOnSuccess(func(scip.IndexStatus) error {
		close(done)
		return nil
	})
	handler.detectLangs = func(path string) ([]string, error) {
		if path != "/tmp/repo" {
			t.Fatalf("unexpected path: %s", path)
		}
		return []string{"go"}, nil
	}
	handler.findIndexerPath = func(lang, dir string) (string, error) {
		if lang != "go" || dir != "/tmp/repo" {
			t.Fatalf("unexpected indexer lookup args: %s %s", lang, dir)
		}
		return "/bin/echo", nil
	}
	handler.runIndexer = func(lang, dir, output string) error {
		if lang != "go" || dir != "/tmp/repo" {
			t.Fatalf("unexpected indexer args: %s %s", lang, dir)
		}
		if !strings.HasSuffix(output, "index-go.scip") {
			t.Fatalf("unexpected index output: %s", output)
		}
		writeTestSCIPFile(t, output, "go", "main.go")
		return nil
	}

	body := bytes.NewBufferString(`{"path":"/tmp/repo"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/scip/index", body)
	res := httptest.NewRecorder()
	restHandler("", handler.ReIndex)(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.Code)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("expected reindex to finish")
	}
}

func TestSCIPReindexDefaultsToProjectRoot(t *testing.T) {
	handler := newTestSCIPHandler(t)

	projectRoot := t.TempDir()
	handler.projectRoot = projectRoot

	done := make(chan struct{})
	handler.asyncIndexer.SetOnSuccess(func(scip.IndexStatus) error {
		close(done)
		return nil
	})
	handler.detectLangs = func(path string) ([]string, error) {
		if path != projectRoot {
			t.Fatalf("unexpected project root: %s", path)
		}
		return []string{"go"}, nil
	}
	handler.findIndexerPath = func(string, string) (string, error) {
		return "/bin/echo", nil
	}
	handler.runIndexer = func(_ string, _ string, output string) error {
		writeTestSCIPFile(t, output, "go", "main.go")
		return nil
	}

	body := bytes.NewBufferString(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/api/scip/reindex", body)
	res := httptest.NewRecorder()
	restHandler("", handler.Reindex)(res, req)

	if res.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d", res.Code)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("expected reindex to finish")
	}
}

func TestSCIPReIndexConflict(t *testing.T) {
	handler := newTestSCIPHandler(t)

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})

	handler.asyncIndexer.SetOnSuccess(func(scip.IndexStatus) error {
		close(done)
		return nil
	})
	handler.detectLangs = func(string) ([]string, error) {
		return []string{"go"}, nil
	}
	handler.findIndexerPath = func(string, string) (string, error) {
		return "/bin/echo", nil
	}
	handler.runIndexer = func(_ string, _ string, output string) error {
		writeTestSCIPFile(t, output, "go", "main.go")
		close(started)
		<-release
		return nil
	}
	handler.configureIndexer()

	if !handler.asyncIndexer.StartAsync(scip.IndexRequest{
		ProjectRoot: "/tmp/repo",
		ScipDir:     handler.scipDir,
		IndexPath:   handler.indexPath,
		Languages:   []string{"go"},
		Merge:       true,
	}) {
		t.Fatal("expected indexing to start")
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("expected indexing to start")
	}

	body := bytes.NewBufferString(`{"path":"/tmp/repo"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/scip/index", body)
	res := httptest.NewRecorder()
	restHandler("", handler.ReIndex)(res, req)

	assertAPIError(t, res, http.StatusConflict, "indexing already in progress")

	close(release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("expected indexing to finish")
	}
}

func newTestSCIPHandler(t *testing.T) *SCIPHandler {
	t.Helper()
	indexPath := filepath.Join(t.TempDir(), "index.scip")
	handler, err := NewSCIPHandler(indexPath, nil, SCIPHandlerOptions{})
	if err != nil {
		t.Fatalf("NewSCIPHandler failed: %v", err)
	}
	return handler
}

func writeTestSCIPFile(t *testing.T, path, language, relativePath string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create scip dir: %v", err)
	}
	index := &scipproto.Index{
		Documents: []*scipproto.Document{
			{RelativePath: relativePath, Language: language},
		},
	}
	payload, err := proto.Marshal(index)
	if err != nil {
		t.Fatalf("marshal scip index: %v", err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("write scip index: %v", err)
	}
}

func assertAPIError(t *testing.T, res *httptest.ResponseRecorder, expectedStatus int, expectedMessage string) {
	t.Helper()
	if res.Code != expectedStatus {
		t.Fatalf("expected status %d, got %d", expectedStatus, res.Code)
	}
	var payload errorResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Message != expectedMessage {
		t.Fatalf("expected message %q, got %q", expectedMessage, payload.Message)
	}
}
