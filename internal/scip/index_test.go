package scip

import (
	"bytes"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/klauspost/compress/zstd"
	_ "github.com/mattn/go-sqlite3"
	scip "github.com/sourcegraph/scip/bindings/go/scip"
	"google.golang.org/protobuf/proto"
)

const (
	testSymbolFoo = "scip-go gomod test v1 `test/pkg`/Foo#"
	testSymbolBar = "scip-go gomod test v1 `test/pkg`/Bar#"
)

func TestFindSymbols(t *testing.T) {
	path := createTestDB(t)
	index, err := OpenIndex(path)
	if err != nil {
		t.Fatalf("OpenIndex failed: %v", err)
	}
	defer index.Close()

	results, err := index.FindSymbols("Foo", 10)
	if err != nil {
		t.Fatalf("FindSymbols failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != testSymbolFoo {
		t.Fatalf("unexpected symbol ID: %s", results[0].ID)
	}
	if results[0].FilePath != "foo.go" {
		t.Fatalf("unexpected file path: %s", results[0].FilePath)
	}
}

func TestGetDefinition(t *testing.T) {
	path := createTestDB(t)
	index, err := OpenIndex(path)
	if err != nil {
		t.Fatalf("OpenIndex failed: %v", err)
	}
	defer index.Close()

	definition, err := index.GetDefinition(testSymbolFoo)
	if err != nil {
		t.Fatalf("GetDefinition failed: %v", err)
	}
	if definition.FilePath != "foo.go" {
		t.Fatalf("unexpected definition file: %s", definition.FilePath)
	}
	if definition.Line != 1 {
		t.Fatalf("unexpected definition line: %d", definition.Line)
	}
}

func TestGetDefinitionMissing(t *testing.T) {
	path := createTestDB(t)
	index, err := OpenIndex(path)
	if err != nil {
		t.Fatalf("OpenIndex failed: %v", err)
	}
	defer index.Close()

	_, err = index.GetDefinition("missing")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	var notFound SymbolNotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("expected SymbolNotFoundError, got %T", err)
	}
}

func TestGetReferences(t *testing.T) {
	path := createTestDB(t)
	index, err := OpenIndex(path)
	if err != nil {
		t.Fatalf("OpenIndex failed: %v", err)
	}
	defer index.Close()

	refs, err := index.GetReferences(testSymbolFoo)
	if err != nil {
		t.Fatalf("GetReferences failed: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1 reference, got %d", len(refs))
	}
	if refs[0].Line != 5 {
		t.Fatalf("unexpected reference line: %d", refs[0].Line)
	}
	if refs[0].Role != "reference" {
		t.Fatalf("unexpected reference role: %s", refs[0].Role)
	}
}

func TestGetSymbolsInFile(t *testing.T) {
	path := createTestDB(t)
	index, err := OpenIndex(path)
	if err != nil {
		t.Fatalf("OpenIndex failed: %v", err)
	}
	defer index.Close()

	symbols, err := index.GetSymbolsInFile("foo.go")
	if err != nil {
		t.Fatalf("GetSymbolsInFile failed: %v", err)
	}
	if len(symbols) != 1 {
		t.Fatalf("expected 1 symbol, got %d", len(symbols))
	}
	if symbols[0].ID != testSymbolFoo {
		t.Fatalf("unexpected symbol ID: %s", symbols[0].ID)
	}
}

func TestGetStats(t *testing.T) {
	path := createTestDB(t)
	index, err := OpenIndex(path)
	if err != nil {
		t.Fatalf("OpenIndex failed: %v", err)
	}
	defer index.Close()

	stats, err := index.GetStats()
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}
	if stats.Documents != 2 {
		t.Fatalf("unexpected documents count: %d", stats.Documents)
	}
	if stats.Symbols != 2 {
		t.Fatalf("unexpected symbols count: %d", stats.Symbols)
	}
	if stats.Occurrences != 3 {
		t.Fatalf("unexpected occurrences count: %d", stats.Occurrences)
	}
}

func createTestDB(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "index.db")
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
	})

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
			t.Fatalf("schema exec failed: %v", err)
		}
	}

	fooDocID := insertDocument(t, db, "foo.go", "go")
	barDocID := insertDocument(t, db, "bar.go", "go")

	fooSymbolID := insertSymbol(t, db, testSymbolFoo, "Foo", scip.SymbolInformation_Function, "Foo docs")
	barSymbolID := insertSymbol(t, db, testSymbolBar, "Bar", scip.SymbolInformation_Function, "Bar docs")

	fooOccurrences := []*scip.Occurrence{
		{
			Range:          []int32{1, 0, 1, 3},
			Symbol:         testSymbolFoo,
			SymbolRoles:    int32(scip.SymbolRole_Definition),
			EnclosingRange: []int32{1, 0, 1, 3},
		},
		{
			Range:       []int32{5, 4, 5, 7},
			Symbol:      testSymbolFoo,
			SymbolRoles: int32(scip.SymbolRole_ReadAccess),
		},
	}
	fooChunkID := insertChunk(t, db, fooDocID, 0, 1, 5, fooOccurrences)
	insertMention(t, db, fooChunkID, fooSymbolID, int32(scip.SymbolRole_Definition))
	insertMention(t, db, fooChunkID, fooSymbolID, int32(scip.SymbolRole_ReadAccess))
	insertEnclosingRange(t, db, fooDocID, fooSymbolID, 1, 0, 1, 3)

	barOccurrences := []*scip.Occurrence{
		{
			Range:          []int32{2, 0, 2, 3},
			Symbol:         testSymbolBar,
			SymbolRoles:    int32(scip.SymbolRole_Definition),
			EnclosingRange: []int32{2, 0, 2, 3},
		},
	}
	barChunkID := insertChunk(t, db, barDocID, 0, 2, 2, barOccurrences)
	insertMention(t, db, barChunkID, barSymbolID, int32(scip.SymbolRole_Definition))
	insertEnclosingRange(t, db, barDocID, barSymbolID, 2, 0, 2, 3)

	return path
}

func insertDocument(t *testing.T, db *sql.DB, path, language string) int64 {
	t.Helper()
	result, err := db.Exec(
		`INSERT INTO documents (language, relative_path, position_encoding, text)
		 VALUES (?, ?, ?, ?)`,
		language,
		path,
		"UTF-8",
		nil,
	)
	if err != nil {
		t.Fatalf("insert document: %v", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("document id: %v", err)
	}
	return id
}

func insertSymbol(t *testing.T, db *sql.DB, symbol, displayName string, kind scip.SymbolInformation_Kind, documentation string) int64 {
	t.Helper()
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
		t.Fatalf("insert symbol: %v", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("symbol id: %v", err)
	}
	return id
}

func insertChunk(t *testing.T, db *sql.DB, docID int64, chunkIndex int, startLine int32, endLine int32, occurrences []*scip.Occurrence) int64 {
	t.Helper()
	blob, err := compressOccurrences(occurrences)
	if err != nil {
		t.Fatalf("compress occurrences: %v", err)
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
		t.Fatalf("insert chunk: %v", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("chunk id: %v", err)
	}
	return id
}

func insertMention(t *testing.T, db *sql.DB, chunkID, symbolID int64, role int32) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO mentions (chunk_id, symbol_id, role) VALUES (?, ?, ?)`,
		chunkID,
		symbolID,
		role,
	); err != nil {
		t.Fatalf("insert mention: %v", err)
	}
}

func insertEnclosingRange(t *testing.T, db *sql.DB, docID, symbolID int64, startLine, startChar, endLine, endChar int32) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO defn_enclosing_ranges (document_id, symbol_id, start_line, start_char, end_line, end_char)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		docID,
		symbolID,
		startLine,
		startChar,
		endLine,
		endChar,
	); err != nil {
		t.Fatalf("insert defn range: %v", err)
	}
}

func compressOccurrences(occurrences []*scip.Occurrence) ([]byte, error) {
	data, err := proto.Marshal(&scip.Document{Occurrences: occurrences})
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
