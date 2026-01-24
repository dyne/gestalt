//go:build !noscip

package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"gestalt/internal/scip"

	"github.com/klauspost/compress/zstd"
	_ "github.com/mattn/go-sqlite3"
	scipproto "github.com/sourcegraph/scip/bindings/go/scip"
	"golang.org/x/time/rate"
	"google.golang.org/protobuf/proto"
)

const (
	scipTestFilePath     = "internal/scip/index.go"
	scipTestSymbolPrefix = "scip-go gomod gestalt v1 `gestalt/internal/scip`/"
)

type scipTestSymbol struct {
	ID            string
	Name          string
	Line          int
	Documentation []string
	ReferenceLine int
}

type scipTestFixture struct {
	handler           *SCIPHandler
	indexPath         string
	filePath          string
	openIndex         scipTestSymbol
	getSymbolsInFile  scipTestSymbol
	decodeOccurrences scipTestSymbol
}

func TestSCIPFindSymbols(t *testing.T) {
	fixture := newSCIPFixture(t, true)

	req := httptest.NewRequest(http.MethodGet, "/api/scip/symbols?q=OpenIndex&limit=5", nil)
	res := httptest.NewRecorder()
	restHandler("", fixture.handler.FindSymbols)(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.Code)
	}

	var payload struct {
		Query   string        `json:"query"`
		Symbols []scip.Symbol `json:"symbols"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Query != "OpenIndex" {
		t.Fatalf("unexpected query: %s", payload.Query)
	}
	if len(payload.Symbols) != 1 {
		t.Fatalf("expected 1 symbol, got %d", len(payload.Symbols))
	}
	assertSymbol(t, payload.Symbols[0], fixture.openIndex, fixture.filePath)
}

func TestSCIPFindSymbolsLimit(t *testing.T) {
	fixture := newSCIPFixture(t, true)

	req := httptest.NewRequest(http.MethodGet, "/api/scip/symbols?q=Index&limit=1", nil)
	res := httptest.NewRecorder()
	restHandler("", fixture.handler.FindSymbols)(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.Code)
	}

	var payload struct {
		Symbols []scip.Symbol `json:"symbols"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Symbols) != 1 {
		t.Fatalf("expected 1 symbol, got %d", len(payload.Symbols))
	}

	expected := []string{fixture.openIndex.ID, fixture.getSymbolsInFile.ID}
	sort.Strings(expected)
	if payload.Symbols[0].ID != expected[0] {
		t.Fatalf("expected symbol %s, got %s", expected[0], payload.Symbols[0].ID)
	}
}

func TestSCIPFindSymbolsEmpty(t *testing.T) {
	fixture := newSCIPFixture(t, true)

	req := httptest.NewRequest(http.MethodGet, "/api/scip/symbols?q=MissingSymbol", nil)
	res := httptest.NewRecorder()
	restHandler("", fixture.handler.FindSymbols)(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.Code)
	}

	var payload struct {
		Symbols []scip.Symbol `json:"symbols"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Symbols) != 0 {
		t.Fatalf("expected 0 symbols, got %d", len(payload.Symbols))
	}
}

func TestSCIPFindSymbolsMissingQuery(t *testing.T) {
	fixture := newSCIPFixture(t, true)

	req := httptest.NewRequest(http.MethodGet, "/api/scip/symbols?limit=2", nil)
	res := httptest.NewRecorder()
	restHandler("", fixture.handler.FindSymbols)(res, req)

	assertAPIError(t, res, http.StatusBadRequest, "missing query")
}

func TestSCIPFindSymbolsInvalidLimit(t *testing.T) {
	fixture := newSCIPFixture(t, true)

	req := httptest.NewRequest(http.MethodGet, "/api/scip/symbols?q=OpenIndex&limit=0", nil)
	res := httptest.NewRecorder()
	restHandler("", fixture.handler.FindSymbols)(res, req)

	assertAPIError(t, res, http.StatusBadRequest, "invalid limit")
}

func TestSCIPFindSymbolsRateLimited(t *testing.T) {
	fixture := newSCIPFixture(t, true)
	fixture.handler.rateLimiter = rate.NewLimiter(0, 0)

	req := httptest.NewRequest(http.MethodGet, "/api/scip/symbols?q=OpenIndex", nil)
	res := httptest.NewRecorder()
	restHandler("", fixture.handler.FindSymbols)(res, req)

	assertAPIError(t, res, http.StatusTooManyRequests, "rate limit exceeded")
}

func TestSCIPFindSymbolsIndexUnavailable(t *testing.T) {
	indexPath := filepath.Join(t.TempDir(), "missing.db")
	handler, err := NewSCIPHandler(indexPath, nil, SCIPHandlerOptions{})
	if err != nil {
		t.Fatalf("NewSCIPHandler failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/scip/symbols?q=OpenIndex", nil)
	res := httptest.NewRecorder()
	restHandler("", handler.FindSymbols)(res, req)

	assertAPIError(t, res, http.StatusServiceUnavailable, "scip index unavailable")
}

func TestSCIPHandleSymbolMissingSuffix(t *testing.T) {
	fixture := newSCIPFixture(t, true)

	req := httptest.NewRequest(http.MethodGet, "/api/scip/symbols/", nil)
	res := httptest.NewRecorder()
	restHandler("", fixture.handler.HandleSymbol)(res, req)

	assertAPIError(t, res, http.StatusNotFound, "symbol not found")
}

func TestSCIPGetSymbol(t *testing.T) {
	fixture := newSCIPFixture(t, true)

	path := "/api/scip/symbols/" + urlEscape(fixture.openIndex.ID)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	res := httptest.NewRecorder()
	restHandler("", fixture.handler.HandleSymbol)(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.Code)
	}

	var payload scip.Symbol
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	assertSymbol(t, payload, fixture.openIndex, fixture.filePath)
}

func TestSCIPGetSymbolFallback(t *testing.T) {
	fixture := newSCIPFixture(t, false)

	path := "/api/scip/symbols/" + urlEscape(fixture.openIndex.ID)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	res := httptest.NewRecorder()
	restHandler("", fixture.handler.HandleSymbol)(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.Code)
	}

	var payload scip.Symbol
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	assertSymbol(t, payload, fixture.openIndex, fixture.filePath)
}

func TestSCIPGetSymbolNotFound(t *testing.T) {
	fixture := newSCIPFixture(t, true)

	path := "/api/scip/symbols/" + urlEscape("missing")
	req := httptest.NewRequest(http.MethodGet, path, nil)
	res := httptest.NewRecorder()
	restHandler("", fixture.handler.HandleSymbol)(res, req)

	assertAPIError(t, res, http.StatusNotFound, "symbol not found")
}

func TestSCIPGetSymbolBadEscape(t *testing.T) {
	fixture := newSCIPFixture(t, true)

	req := newRequestWithPath(http.MethodGet, "/api/scip/symbols/%zz")
	res := httptest.NewRecorder()
	restHandler("", fixture.handler.HandleSymbol)(res, req)

	assertAPIError(t, res, http.StatusBadRequest, "invalid symbol id")
}

func TestSCIPGetSymbolMethodNotAllowed(t *testing.T) {
	fixture := newSCIPFixture(t, true)

	path := "/api/scip/symbols/" + urlEscape(fixture.openIndex.ID)
	req := httptest.NewRequest(http.MethodPost, path, nil)
	res := httptest.NewRecorder()
	restHandler("", fixture.handler.HandleSymbol)(res, req)

	assertAPIError(t, res, http.StatusMethodNotAllowed, "method not allowed")
	if allow := res.Result().Header.Get("Allow"); allow != http.MethodGet {
		t.Fatalf("expected Allow header GET, got %s", allow)
	}
}

func TestSCIPGetReferences(t *testing.T) {
	fixture := newSCIPFixture(t, true)

	path := "/api/scip/symbols/" + urlEscape(fixture.decodeOccurrences.ID) + "/references"
	req := httptest.NewRequest(http.MethodGet, path, nil)
	res := httptest.NewRecorder()
	restHandler("", fixture.handler.HandleSymbol)(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.Code)
	}

	var payload struct {
		Symbol     string            `json:"symbol"`
		References []scip.Occurrence `json:"references"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Symbol != fixture.decodeOccurrences.ID {
		t.Fatalf("unexpected symbol: %s", payload.Symbol)
	}
	if len(payload.References) != 1 {
		t.Fatalf("expected 1 reference, got %d", len(payload.References))
	}
	assertOccurrence(t, payload.References[0], fixture.decodeOccurrences, fixture.filePath)
}

func TestSCIPGetReferencesOnlyDefinition(t *testing.T) {
	fixture := newSCIPFixture(t, true)

	path := "/api/scip/symbols/" + urlEscape(fixture.openIndex.ID) + "/references"
	req := httptest.NewRequest(http.MethodGet, path, nil)
	res := httptest.NewRecorder()
	restHandler("", fixture.handler.HandleSymbol)(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.Code)
	}

	var payload struct {
		References []scip.Occurrence `json:"references"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.References) != 0 {
		t.Fatalf("expected 0 references, got %d", len(payload.References))
	}
}

func TestSCIPGetReferencesNotFound(t *testing.T) {
	fixture := newSCIPFixture(t, true)

	path := "/api/scip/symbols/" + urlEscape("missing") + "/references"
	req := httptest.NewRequest(http.MethodGet, path, nil)
	res := httptest.NewRecorder()
	restHandler("", fixture.handler.HandleSymbol)(res, req)

	assertAPIError(t, res, http.StatusNotFound, "symbol not found")
}

func TestSCIPGetReferencesBadEscape(t *testing.T) {
	fixture := newSCIPFixture(t, true)

	req := newRequestWithPath(http.MethodGet, "/api/scip/symbols/%zz/references")
	res := httptest.NewRecorder()
	restHandler("", fixture.handler.HandleSymbol)(res, req)

	assertAPIError(t, res, http.StatusBadRequest, "invalid symbol id")
}

func TestSCIPGetReferencesMethodNotAllowed(t *testing.T) {
	fixture := newSCIPFixture(t, true)

	path := "/api/scip/symbols/" + urlEscape(fixture.decodeOccurrences.ID) + "/references"
	req := httptest.NewRequest(http.MethodPost, path, nil)
	res := httptest.NewRecorder()
	restHandler("", fixture.handler.HandleSymbol)(res, req)

	assertAPIError(t, res, http.StatusMethodNotAllowed, "method not allowed")
	if allow := res.Result().Header.Get("Allow"); allow != http.MethodGet {
		t.Fatalf("expected Allow header GET, got %s", allow)
	}
}

func TestSCIPGetFileSymbols(t *testing.T) {
	fixture := newSCIPFixture(t, true)

	path := "/api/scip/files/" + urlEscape(fixture.filePath)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	res := httptest.NewRecorder()
	restHandler("", fixture.handler.GetFileSymbols)(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.Code)
	}

	var payload struct {
		File    string        `json:"file"`
		Symbols []scip.Symbol `json:"symbols"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.File != fixture.filePath {
		t.Fatalf("unexpected file: %s", payload.File)
	}

	expected := fixture.allSymbols()
	sort.Slice(expected, func(left, right int) bool {
		return expected[left].Line < expected[right].Line
	})
	if len(payload.Symbols) != len(expected) {
		t.Fatalf("expected %d symbols, got %d", len(expected), len(payload.Symbols))
	}
	for index, symbol := range payload.Symbols {
		assertSymbol(t, symbol, expected[index], fixture.filePath)
	}
}

func TestSCIPGetFileSymbolsFallback(t *testing.T) {
	fixture := newSCIPFixture(t, false)

	path := "/api/scip/files/" + urlEscape(fixture.filePath)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	res := httptest.NewRecorder()
	restHandler("", fixture.handler.GetFileSymbols)(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.Code)
	}

	var payload struct {
		Symbols []scip.Symbol `json:"symbols"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	expected := fixture.allSymbols()
	sort.Slice(expected, func(left, right int) bool {
		return expected[left].Line < expected[right].Line
	})
	if len(payload.Symbols) != len(expected) {
		t.Fatalf("expected %d symbols, got %d", len(expected), len(payload.Symbols))
	}
	for index, symbol := range payload.Symbols {
		assertSymbol(t, symbol, expected[index], fixture.filePath)
	}
}

func TestSCIPGetFileSymbolsMissingPath(t *testing.T) {
	fixture := newSCIPFixture(t, true)

	req := httptest.NewRequest(http.MethodGet, "/api/scip/files/", nil)
	res := httptest.NewRecorder()
	restHandler("", fixture.handler.GetFileSymbols)(res, req)

	assertAPIError(t, res, http.StatusBadRequest, "missing file path")
}

func TestSCIPGetFileSymbolsBadEscape(t *testing.T) {
	fixture := newSCIPFixture(t, true)

	req := newRequestWithPath(http.MethodGet, "/api/scip/files/%zz")
	res := httptest.NewRecorder()
	restHandler("", fixture.handler.GetFileSymbols)(res, req)

	assertAPIError(t, res, http.StatusBadRequest, "invalid file path")
}

func TestSCIPGetFileSymbolsMissingDoc(t *testing.T) {
	fixture := newSCIPFixture(t, true)

	path := "/api/scip/files/" + urlEscape("internal/scip/missing.go")
	req := httptest.NewRequest(http.MethodGet, path, nil)
	res := httptest.NewRecorder()
	restHandler("", fixture.handler.GetFileSymbols)(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.Code)
	}

	var payload struct {
		Symbols []scip.Symbol `json:"symbols"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Symbols) != 0 {
		t.Fatalf("expected 0 symbols, got %d", len(payload.Symbols))
	}
}

func TestSCIPGetFileSymbolsMethodNotAllowed(t *testing.T) {
	fixture := newSCIPFixture(t, true)

	path := "/api/scip/files/" + urlEscape(fixture.filePath)
	req := httptest.NewRequest(http.MethodPost, path, nil)
	res := httptest.NewRecorder()
	restHandler("", fixture.handler.GetFileSymbols)(res, req)

	assertAPIError(t, res, http.StatusMethodNotAllowed, "method not allowed")
	if allow := res.Result().Header.Get("Allow"); allow != http.MethodGet {
		t.Fatalf("expected Allow header GET, got %s", allow)
	}
}

func TestSCIPGetFileSymbolsRateLimited(t *testing.T) {
	fixture := newSCIPFixture(t, true)
	fixture.handler.rateLimiter = rate.NewLimiter(0, 0)

	path := "/api/scip/files/" + urlEscape(fixture.filePath)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	res := httptest.NewRecorder()
	restHandler("", fixture.handler.GetFileSymbols)(res, req)

	assertAPIError(t, res, http.StatusTooManyRequests, "rate limit exceeded")
}

func TestSCIPStatus(t *testing.T) {
	fixture := newSCIPFixture(t, true)
	projectRoot := createProjectRootWithIndexFile(t)

	meta, err := scip.BuildMetadata(projectRoot, []string{"go"})
	if err != nil {
		t.Fatalf("BuildMetadata failed: %v", err)
	}
	if err := scip.SaveMetadata(fixture.indexPath, meta); err != nil {
		t.Fatalf("SaveMetadata failed: %v", err)
	}

	handler, err := NewSCIPHandler(fixture.indexPath, nil, SCIPHandlerOptions{ProjectRoot: projectRoot})
	if err != nil {
		t.Fatalf("NewSCIPHandler failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/scip/status", nil)
	res := httptest.NewRecorder()
	restHandler("", handler.Status)(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.Code)
	}

	var payload scipStatusResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Indexed {
		t.Fatalf("expected indexed true")
	}
	if !payload.Fresh {
		t.Fatalf("expected fresh true")
	}
	if payload.Documents != 1 {
		t.Fatalf("expected documents 1, got %d", payload.Documents)
	}
	if payload.Symbols != len(fixture.allSymbols()) {
		t.Fatalf("expected symbols %d, got %d", len(fixture.allSymbols()), payload.Symbols)
	}
	if payload.CreatedAt == "" {
		t.Fatalf("expected created_at to be set")
	}

	sourcePath := filepath.Join(projectRoot, scipTestFilePath)
	if err := os.WriteFile(sourcePath, []byte("package scip\n// change\n"), 0o644); err != nil {
		t.Fatalf("rewrite source: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/scip/status", nil)
	res = httptest.NewRecorder()
	restHandler("", handler.Status)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.Code)
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Fresh {
		t.Fatalf("expected fresh false after change")
	}
}

func TestSCIPStatusMissingIndex(t *testing.T) {
	indexPath := filepath.Join(t.TempDir(), "missing.db")
	handler, err := NewSCIPHandler(indexPath, nil, SCIPHandlerOptions{})
	if err != nil {
		t.Fatalf("NewSCIPHandler failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/scip/status", nil)
	res := httptest.NewRecorder()
	restHandler("", handler.Status)(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.Code)
	}

	var payload scipStatusResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Indexed {
		t.Fatalf("expected indexed false")
	}
	if payload.Fresh {
		t.Fatalf("expected fresh false")
	}
	if payload.Documents != 0 {
		t.Fatalf("expected documents 0, got %d", payload.Documents)
	}
	if payload.Symbols != 0 {
		t.Fatalf("expected symbols 0, got %d", payload.Symbols)
	}
	if payload.CreatedAt != "" {
		t.Fatalf("expected empty created_at")
	}
}

func TestSCIPStatusRateLimited(t *testing.T) {
	fixture := newSCIPFixture(t, true)
	fixture.handler.rateLimiter = rate.NewLimiter(0, 0)

	req := httptest.NewRequest(http.MethodGet, "/api/scip/status", nil)
	res := httptest.NewRecorder()
	restHandler("", fixture.handler.Status)(res, req)

	assertAPIError(t, res, http.StatusTooManyRequests, "rate limit exceeded")
}

func TestSCIPReIndex(t *testing.T) {
	fixture := newSCIPFixture(t, true)
	handler := fixture.handler

	done := make(chan struct{})
	handler.detectLangs = func(path string) ([]string, error) {
		if path != "/tmp/repo" {
			t.Fatalf("unexpected path: %s", path)
		}
		return []string{"go"}, nil
	}
	handler.runIndexer = func(lang, dir, output string) error {
		if lang != "go" || dir != "/tmp/repo" {
			t.Fatalf("unexpected indexer args: %s %s", lang, dir)
		}
		return nil
	}
	handler.convert = func(scipPath, dbPath string) error {
		if !strings.HasSuffix(scipPath, "index.scip") {
			t.Fatalf("unexpected scip path: %s", scipPath)
		}
		if dbPath != handler.indexPath {
			t.Fatalf("unexpected db path: %s", dbPath)
		}
		return nil
	}
	handler.openIndex = func(path string) (*scip.Index, error) {
		defer close(done)
		return scip.OpenIndex(path)
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

func TestSCIPReIndexSkipsRecent(t *testing.T) {
	fixture := newSCIPFixture(t, true)

	meta := scip.IndexMetadata{
		CreatedAt:   time.Now().UTC(),
		ProjectRoot: "/tmp/repo",
		Languages:   []string{"go"},
		FilesHashed: "hash",
	}
	if err := scip.SaveMetadata(fixture.indexPath, meta); err != nil {
		t.Fatalf("SaveMetadata failed: %v", err)
	}

	body := bytes.NewBufferString(`{"path":"/tmp/repo"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/scip/index", body)
	res := httptest.NewRecorder()
	restHandler("", fixture.handler.ReIndex)(res, req)

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

	fixture.handler.reindexMu.Lock()
	reindexing := fixture.handler.reindexing
	fixture.handler.reindexMu.Unlock()
	if reindexing {
		t.Fatalf("expected reindexing to be false after skip")
	}
}

func TestSCIPReIndexInvalidBody(t *testing.T) {
	fixture := newSCIPFixture(t, true)

	body := bytes.NewBufferString("not-json")
	req := httptest.NewRequest(http.MethodPost, "/api/scip/index", body)
	res := httptest.NewRecorder()
	restHandler("", fixture.handler.ReIndex)(res, req)

	assertAPIError(t, res, http.StatusBadRequest, "invalid request body")
}

func TestSCIPReIndexMissingPath(t *testing.T) {
	fixture := newSCIPFixture(t, true)

	body := bytes.NewBufferString(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/api/scip/index", body)
	res := httptest.NewRecorder()
	restHandler("", fixture.handler.ReIndex)(res, req)

	assertAPIError(t, res, http.StatusBadRequest, "path is required")
}

func TestSCIPReIndexConflict(t *testing.T) {
	fixture := newSCIPFixture(t, true)
	fixture.handler.reindexMu.Lock()
	fixture.handler.reindexing = true
	fixture.handler.reindexMu.Unlock()
	defer func() {
		fixture.handler.reindexMu.Lock()
		fixture.handler.reindexing = false
		fixture.handler.reindexMu.Unlock()
	}()

	body := bytes.NewBufferString(`{"path":"/tmp/repo"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/scip/index", body)
	res := httptest.NewRecorder()
	restHandler("", fixture.handler.ReIndex)(res, req)

	assertAPIError(t, res, http.StatusConflict, "indexing already in progress")
}

func TestSCIPReIndexMethodNotAllowed(t *testing.T) {
	fixture := newSCIPFixture(t, true)

	req := httptest.NewRequest(http.MethodGet, "/api/scip/index", nil)
	res := httptest.NewRecorder()
	restHandler("", fixture.handler.ReIndex)(res, req)

	assertAPIError(t, res, http.StatusMethodNotAllowed, "method not allowed")
	if allow := res.Result().Header.Get("Allow"); allow != http.MethodPost {
		t.Fatalf("expected Allow header POST, got %s", allow)
	}
}

func TestSCIPReIndexRateLimited(t *testing.T) {
	fixture := newSCIPFixture(t, true)
	fixture.handler.rateLimiter = rate.NewLimiter(0, 0)

	body := bytes.NewBufferString(`{"path":"/tmp/repo"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/scip/index", body)
	res := httptest.NewRecorder()
	restHandler("", fixture.handler.ReIndex)(res, req)

	assertAPIError(t, res, http.StatusTooManyRequests, "rate limit exceeded")
}

func newSCIPFixture(t *testing.T, includeDefnRanges bool) scipTestFixture {
	fixture, err := buildSCIPDBFromIndexFile(filepath.Join(t.TempDir(), "index.db"), includeDefnRanges)
	if err != nil {
		t.Fatalf("build test db: %v", err)
	}

	handler, err := NewSCIPHandler(fixture.indexPath, nil, SCIPHandlerOptions{})
	if err != nil {
		t.Fatalf("NewSCIPHandler failed: %v", err)
	}
	fixture.handler = handler
	return fixture
}

func (fixture scipTestFixture) allSymbols() []scipTestSymbol {
	return []scipTestSymbol{fixture.openIndex, fixture.getSymbolsInFile, fixture.decodeOccurrences}
}

func createProjectRootWithIndexFile(t *testing.T) string {
	_, content, err := readIndexFileContent()
	if err != nil {
		t.Fatalf("read index.go: %v", err)
	}
	projectRoot := t.TempDir()
	destPath := filepath.Join(projectRoot, scipTestFilePath)
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		t.Fatalf("create project dir: %v", err)
	}
	if err := os.WriteFile(destPath, content, 0o644); err != nil {
		t.Fatalf("write project file: %v", err)
	}
	return projectRoot
}

func buildTestSCIPDB(path string) error {
	_, err := buildSCIPDBFromIndexFile(path, true)
	return err
}

func buildSCIPDBFromIndexFile(path string, includeDefnRanges bool) (scipTestFixture, error) {
	fixture := scipTestFixture{
		indexPath: path,
		filePath:  scipTestFilePath,
	}

	_, content, err := readIndexFileContent()
	if err != nil {
		return scipTestFixture{}, err
	}
	lines := splitLines(string(content))

	openIndexLine, err := findLineIndex(lines, "func OpenIndex(")
	if err != nil {
		return scipTestFixture{}, err
	}
	getSymbolsLine, err := findLineIndex(lines, "func (idx *Index) GetSymbolsInFile(")
	if err != nil {
		return scipTestFixture{}, err
	}
	decodeDefinitionLine, err := findLineIndex(lines, "func decodeOccurrences(")
	if err != nil {
		return scipTestFixture{}, err
	}
	decodeReferenceLine, err := findLineIndexExcluding(lines, "decodeOccurrences(", "func decodeOccurrences(")
	if err != nil {
		return scipTestFixture{}, err
	}

	fixture.openIndex = scipTestSymbol{
		ID:            makeSymbolID("OpenIndex#"),
		Name:          "OpenIndex",
		Line:          openIndexLine,
		Documentation: []string{"OpenIndex docs", "Second line"},
	}
	fixture.getSymbolsInFile = scipTestSymbol{
		ID:            makeSymbolID("Index#GetSymbolsInFile"),
		Name:          "GetSymbolsInFile",
		Line:          getSymbolsLine,
		Documentation: []string{"GetSymbolsInFile docs"},
	}
	fixture.decodeOccurrences = scipTestSymbol{
		ID:            makeSymbolID("decodeOccurrences#"),
		Name:          "decodeOccurrences",
		Line:          decodeDefinitionLine,
		Documentation: []string{"decodeOccurrences docs"},
		ReferenceLine: decodeReferenceLine,
	}

	database, err := sqlOpenSQLite(path)
	if err != nil {
		return scipTestFixture{}, err
	}
	defer database.Close()

	schemaStatements := []string{
		`CREATE TABLE documents (
			id INTEGER PRIMARY KEY,
			language TEXT,
			relative_path TEXT NOT NULL UNIQUE,
			position_encoding TEXT,
			text TEXT
		);`,
		`CREATE TABLE chunks (
			id INTEGER PRIMARY KEY,
			document_id INTEGER NOT NULL,
			chunk_index INTEGER NOT NULL,
			start_line INTEGER NOT NULL,
			end_line INTEGER NOT NULL,
			occurrences BLOB NOT NULL
		);`,
		`CREATE TABLE global_symbols (
			id INTEGER PRIMARY KEY,
			symbol TEXT NOT NULL UNIQUE,
			display_name TEXT,
			kind INTEGER,
			documentation TEXT,
			signature BLOB,
			enclosing_symbol TEXT,
			relationships BLOB
		);`,
		`CREATE TABLE mentions (
			chunk_id INTEGER NOT NULL,
			symbol_id INTEGER NOT NULL,
			role INTEGER NOT NULL,
			PRIMARY KEY (chunk_id, symbol_id, role)
		);`,
		`CREATE TABLE defn_enclosing_ranges (
			id INTEGER PRIMARY KEY,
			document_id INTEGER NOT NULL,
			symbol_id INTEGER NOT NULL,
			start_line INTEGER NOT NULL,
			start_char INTEGER NOT NULL,
			end_line INTEGER NOT NULL,
			end_char INTEGER NOT NULL
		);`,
	}
	for _, stmt := range schemaStatements {
		if _, err := database.Exec(stmt); err != nil {
			return scipTestFixture{}, err
		}
	}

	docID, err := insertDocument(database, scipTestFilePath, "go")
	if err != nil {
		return scipTestFixture{}, err
	}
	openIndexID, err := insertSymbol(database, fixture.openIndex.ID, fixture.openIndex.Name, scipproto.SymbolInformation_Function, strings.Join(fixture.openIndex.Documentation, "\n"))
	if err != nil {
		return scipTestFixture{}, err
	}
	getSymbolsID, err := insertSymbol(database, fixture.getSymbolsInFile.ID, fixture.getSymbolsInFile.Name, scipproto.SymbolInformation_Function, strings.Join(fixture.getSymbolsInFile.Documentation, "\n"))
	if err != nil {
		return scipTestFixture{}, err
	}
	decodeID, err := insertSymbol(database, fixture.decodeOccurrences.ID, fixture.decodeOccurrences.Name, scipproto.SymbolInformation_Function, strings.Join(fixture.decodeOccurrences.Documentation, "\n"))
	if err != nil {
		return scipTestFixture{}, err
	}

	occurrences := []*scipproto.Occurrence{
		{
			Range:          buildRange(fixture.openIndex.Line, 0, fixture.openIndex.Line, 3),
			Symbol:         fixture.openIndex.ID,
			SymbolRoles:    int32(scipproto.SymbolRole_Definition),
			EnclosingRange: buildRange(fixture.openIndex.Line, 0, fixture.openIndex.Line, 3),
		},
		{
			Range:          buildRange(fixture.getSymbolsInFile.Line, 0, fixture.getSymbolsInFile.Line, 3),
			Symbol:         fixture.getSymbolsInFile.ID,
			SymbolRoles:    int32(scipproto.SymbolRole_Definition),
			EnclosingRange: buildRange(fixture.getSymbolsInFile.Line, 0, fixture.getSymbolsInFile.Line, 3),
		},
		{
			Range:          buildRange(fixture.decodeOccurrences.Line, 0, fixture.decodeOccurrences.Line, 3),
			Symbol:         fixture.decodeOccurrences.ID,
			SymbolRoles:    int32(scipproto.SymbolRole_Definition),
			EnclosingRange: buildRange(fixture.decodeOccurrences.Line, 0, fixture.decodeOccurrences.Line, 3),
		},
		{
			Range:       buildRange(fixture.decodeOccurrences.ReferenceLine, 0, fixture.decodeOccurrences.ReferenceLine, 3),
			Symbol:      fixture.decodeOccurrences.ID,
			SymbolRoles: int32(scipproto.SymbolRole_ReadAccess),
		},
	}
	chunkStart := occurrences[0].Range[0]
	chunkEnd := occurrences[0].Range[2]
	for _, occ := range occurrences[1:] {
		if occ.Range[0] < chunkStart {
			chunkStart = occ.Range[0]
		}
		if occ.Range[2] > chunkEnd {
			chunkEnd = occ.Range[2]
		}
	}

	chunkID, err := insertChunk(database, docID, 0, chunkStart, chunkEnd, occurrences)
	if err != nil {
		return scipTestFixture{}, err
	}
	if err := insertMention(database, chunkID, openIndexID, int32(scipproto.SymbolRole_Definition)); err != nil {
		return scipTestFixture{}, err
	}
	if err := insertMention(database, chunkID, getSymbolsID, int32(scipproto.SymbolRole_Definition)); err != nil {
		return scipTestFixture{}, err
	}
	if err := insertMention(database, chunkID, decodeID, int32(scipproto.SymbolRole_Definition)); err != nil {
		return scipTestFixture{}, err
	}
	if err := insertMention(database, chunkID, decodeID, int32(scipproto.SymbolRole_ReadAccess)); err != nil {
		return scipTestFixture{}, err
	}

	if includeDefnRanges {
		if err := insertEnclosingRange(database, docID, openIndexID, int32(fixture.openIndex.Line), 0, int32(fixture.openIndex.Line), 3); err != nil {
			return scipTestFixture{}, err
		}
		if err := insertEnclosingRange(database, docID, getSymbolsID, int32(fixture.getSymbolsInFile.Line), 0, int32(fixture.getSymbolsInFile.Line), 3); err != nil {
			return scipTestFixture{}, err
		}
		if err := insertEnclosingRange(database, docID, decodeID, int32(fixture.decodeOccurrences.Line), 0, int32(fixture.decodeOccurrences.Line), 3); err != nil {
			return scipTestFixture{}, err
		}
	}

	return fixture, nil
}

func readIndexFileContent() (string, []byte, error) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		return "", nil, err
	}
	fullPath := filepath.Join(repoRoot, scipTestFilePath)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return "", nil, err
	}
	return fullPath, content, nil
}

func findRepoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	current := cwd
	for {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.New("go.mod not found")
		}
		current = parent
	}
}

func splitLines(content string) []string {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	return strings.Split(normalized, "\n")
}

func findLineIndex(lines []string, needle string) (int, error) {
	for index, line := range lines {
		if strings.Contains(line, needle) {
			return index, nil
		}
	}
	return 0, fmt.Errorf("line not found: %s", needle)
}

func findLineIndexExcluding(lines []string, needle string, exclude string) (int, error) {
	for index, line := range lines {
		if strings.Contains(line, needle) && (exclude == "" || !strings.Contains(line, exclude)) {
			return index, nil
		}
	}
	return 0, fmt.Errorf("line not found for %s", needle)
}

func buildRange(startLine, startChar, endLine, endChar int) []int32 {
	return []int32{int32(startLine), int32(startChar), int32(endLine), int32(endChar)}
}

func makeSymbolID(suffix string) string {
	return scipTestSymbolPrefix + suffix
}

func sqlOpenSQLite(path string) (*sql.DB, error) {
	return sql.Open("sqlite3", path)
}

func insertDocument(db *sql.DB, path, language string) (int64, error) {
	result, err := db.Exec(
		`INSERT INTO documents (language, relative_path, position_encoding, text)
		 VALUES (?, ?, ?, ?)`,
		language,
		path,
		"UTF-8",
		nil,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func insertSymbol(db *sql.DB, symbol, displayName string, kind scipproto.SymbolInformation_Kind, documentation string) (int64, error) {
	result, err := db.Exec(
		`INSERT INTO global_symbols (symbol, display_name, kind, documentation, enclosing_symbol)
		 VALUES (?, ?, ?, ?, ?)`,
		symbol,
		displayName,
		int64(kind),
		documentation,
		nil,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func insertChunk(db *sql.DB, docID int64, chunkIndex int, startLine int32, endLine int32, occurrences []*scipproto.Occurrence) (int64, error) {
	blob, err := compressOccurrences(occurrences)
	if err != nil {
		return 0, err
	}
	result, err := db.Exec(
		`INSERT INTO chunks (document_id, chunk_index, start_line, end_line, occurrences)
		 VALUES (?, ?, ?, ?, ?)`,
		docID,
		chunkIndex,
		startLine,
		endLine,
		blob,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func insertMention(db *sql.DB, chunkID, symbolID int64, role int32) error {
	_, err := db.Exec(
		`INSERT INTO mentions (chunk_id, symbol_id, role) VALUES (?, ?, ?)`,
		chunkID,
		symbolID,
		role,
	)
	return err
}

func insertEnclosingRange(db *sql.DB, docID, symbolID int64, startLine, startChar, endLine, endChar int32) error {
	_, err := db.Exec(
		`INSERT INTO defn_enclosing_ranges (document_id, symbol_id, start_line, start_char, end_line, end_char)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		docID,
		symbolID,
		startLine,
		startChar,
		endLine,
		endChar,
	)
	return err
}

func compressOccurrences(occurrences []*scipproto.Occurrence) ([]byte, error) {
	data, err := proto.Marshal(&scipproto.Document{Occurrences: occurrences})
	if err != nil {
		return nil, err
	}
	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		return nil, err
	}
	var buffer bytes.Buffer
	encoder.Reset(&buffer)
	if _, err := encoder.Write(data); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func assertSymbol(t *testing.T, got scip.Symbol, expected scipTestSymbol, filePath string) {
	t.Helper()
	if got.ID != expected.ID {
		t.Fatalf("unexpected symbol id: %s", got.ID)
	}
	if got.Name != expected.Name {
		t.Fatalf("unexpected symbol name: %s", got.Name)
	}
	if got.Kind != scipproto.SymbolInformation_Function.String() {
		t.Fatalf("unexpected symbol kind: %s", got.Kind)
	}
	if got.FilePath != filePath {
		t.Fatalf("unexpected symbol file path: %s", got.FilePath)
	}
	if got.Line != expected.Line {
		t.Fatalf("unexpected symbol line: %d", got.Line)
	}
	if got.Language != "go" {
		t.Fatalf("unexpected symbol language: %s", got.Language)
	}
	if !equalStringSlices(got.Documentation, expected.Documentation) {
		t.Fatalf("unexpected documentation: %v", got.Documentation)
	}
	if got.Signature != "" {
		t.Fatalf("expected empty signature, got %q", got.Signature)
	}
}

func assertOccurrence(t *testing.T, got scip.Occurrence, expected scipTestSymbol, filePath string) {
	t.Helper()
	if got.Symbol != expected.ID {
		t.Fatalf("unexpected occurrence symbol: %s", got.Symbol)
	}
	if got.FilePath != filePath {
		t.Fatalf("unexpected occurrence file path: %s", got.FilePath)
	}
	if got.Line != expected.ReferenceLine {
		t.Fatalf("unexpected occurrence line: %d", got.Line)
	}
	if got.Column != 0 {
		t.Fatalf("unexpected occurrence column: %d", got.Column)
	}
	if got.Role != "reference" {
		t.Fatalf("unexpected occurrence role: %s", got.Role)
	}
}

func assertAPIError(t *testing.T, res *httptest.ResponseRecorder, expectedStatus int, expectedMessage string) {
	t.Helper()
	if res.Code != expectedStatus {
		t.Fatalf("expected status %d, got %d", expectedStatus, res.Code)
	}
	var payload struct {
		Message string `json:"message"`
		Error   string `json:"error"`
		Code    string `json:"code"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	message := payload.Message
	if message == "" {
		message = payload.Error
	}
	if message != expectedMessage {
		t.Fatalf("expected error %q, got %q", expectedMessage, message)
	}
	expectedCode := errorCodeForStatus(expectedStatus)
	if expectedCode != "" && payload.Code != expectedCode {
		t.Fatalf("expected code %q, got %q", expectedCode, payload.Code)
	}
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index, value := range left {
		if right[index] != value {
			return false
		}
	}
	return true
}

func urlEscape(value string) string {
	return strings.ReplaceAll(url.PathEscape(value), "+", "%20")
}

func newRequestWithPath(method, path string) *http.Request {
	return &http.Request{
		Method: method,
		URL: &url.URL{
			Path:    path,
			RawPath: path,
		},
		Header: make(http.Header),
	}
}
