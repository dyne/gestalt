package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gestalt/internal/scip"

	"github.com/klauspost/compress/zstd"
	_ "github.com/mattn/go-sqlite3"
	scipproto "github.com/sourcegraph/scip/bindings/go/scip"
	"google.golang.org/protobuf/proto"
)

const (
	apiSymbolFoo = "scip-go gomod test v1 `test/pkg`/Foo#"
)

func TestSCIPFindSymbols(t *testing.T) {
	handler := newTestSCIPHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/scip/symbols?q=Foo&limit=5", nil)
	res := httptest.NewRecorder()
	restHandler("", handler.FindSymbols)(res, req)

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
	if payload.Symbols[0].ID != apiSymbolFoo {
		t.Fatalf("unexpected symbol id: %s", payload.Symbols[0].ID)
	}
}

func TestSCIPGetSymbol(t *testing.T) {
	handler := newTestSCIPHandler(t)

	path := "/api/scip/symbols/" + urlEscape(apiSymbolFoo)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	res := httptest.NewRecorder()
	restHandler("", handler.HandleSymbol)(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.Code)
	}

	var payload scip.Symbol
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.ID != apiSymbolFoo {
		t.Fatalf("unexpected symbol id: %s", payload.ID)
	}
}

func TestSCIPGetReferences(t *testing.T) {
	handler := newTestSCIPHandler(t)

	path := "/api/scip/symbols/" + urlEscape(apiSymbolFoo) + "/references"
	req := httptest.NewRequest(http.MethodGet, path, nil)
	res := httptest.NewRecorder()
	restHandler("", handler.HandleSymbol)(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.Code)
	}

	var payload struct {
		References []scip.Occurrence `json:"references"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.References) != 1 {
		t.Fatalf("expected 1 reference, got %d", len(payload.References))
	}
}

func TestSCIPGetFileSymbols(t *testing.T) {
	handler := newTestSCIPHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/scip/files/foo.go", nil)
	res := httptest.NewRecorder()
	restHandler("", handler.GetFileSymbols)(res, req)

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
}

func TestSCIPReIndex(t *testing.T) {
	handler := newTestSCIPHandler(t)

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
		t.Fatalf("reindex did not complete")
	}
}

func newTestSCIPHandler(t *testing.T) *SCIPHandler {
	t.Helper()

	path := createTestSCIPDB(t)
	handler, err := NewSCIPHandler(path, nil)
	if err != nil {
		t.Fatalf("NewSCIPHandler failed: %v", err)
	}
	return handler
}

func createTestSCIPDB(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "index.db")
	if err := buildTestSCIPDB(path); err != nil {
		t.Fatalf("build test db: %v", err)
	}
	return path
}

func buildTestSCIPDB(path string) error {
	db, err := sqlOpenSQLite(path)
	if err != nil {
		return err
	}
	defer db.Close()

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
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}

	docID, err := insertDocument(db, "foo.go", "go")
	if err != nil {
		return err
	}
	symbolID, err := insertSymbol(db, apiSymbolFoo, "Foo", scipproto.SymbolInformation_Function, "Foo docs")
	if err != nil {
		return err
	}

	occurrences := []*scipproto.Occurrence{
		{
			Range:          []int32{1, 0, 1, 3},
			Symbol:         apiSymbolFoo,
			SymbolRoles:    int32(scipproto.SymbolRole_Definition),
			EnclosingRange: []int32{1, 0, 1, 3},
		},
		{
			Range:       []int32{5, 4, 5, 7},
			Symbol:      apiSymbolFoo,
			SymbolRoles: int32(scipproto.SymbolRole_ReadAccess),
		},
	}
	chunkID, err := insertChunk(db, docID, 0, 1, 5, occurrences)
	if err != nil {
		return err
	}
	if err := insertMention(db, chunkID, symbolID, int32(scipproto.SymbolRole_Definition)); err != nil {
		return err
	}
	if err := insertMention(db, chunkID, symbolID, int32(scipproto.SymbolRole_ReadAccess)); err != nil {
		return err
	}
	if err := insertEnclosingRange(db, docID, symbolID, 1, 0, 1, 3); err != nil {
		return err
	}
	return nil
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

func urlEscape(value string) string {
	return strings.ReplaceAll(url.PathEscape(value), "+", "%20")
}
