package scip

import (
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"strings"

	"github.com/klauspost/compress/zstd"
	_ "github.com/mattn/go-sqlite3"
	scip "github.com/sourcegraph/scip/bindings/go/scip"
	"google.golang.org/protobuf/proto"
)

const (
	defaultFindLimit = 20
	defaultCacheSize = 256
)

// Index represents a SCIP SQLite database.
type Index struct {
	db          *sql.DB
	path        string
	symbolCache *lruCache
}

// OpenIndex opens a SCIP SQLite database.
func OpenIndex(path string) (*Index, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)

	return &Index{
		db:          db,
		path:        path,
		symbolCache: newLRUCache(defaultCacheSize),
	}, nil
}

// Close closes the underlying database connection.
func (idx *Index) Close() error {
	return idx.db.Close()
}

// Symbol represents a code symbol with metadata.
type Symbol struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Kind          string   `json:"kind"`
	Signature     string   `json:"signature"`
	Documentation []string `json:"documentation"`
	FilePath      string   `json:"file_path"`
	Line          int      `json:"line"`
	Language      string   `json:"language"`
}

// Occurrence represents a symbol reference.
type Occurrence struct {
	Symbol   string `json:"symbol"`
	FilePath string `json:"file_path"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Role     string `json:"role"`
}

// IndexStats represents basic counts from an index.
type IndexStats struct {
	Documents   int `json:"documents"`
	Symbols     int `json:"symbols"`
	Occurrences int `json:"occurrences"`
}

// FindSymbols searches for symbols by name (fuzzy match).
func (idx *Index) FindSymbols(query string, limit int) ([]Symbol, error) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return nil, fmt.Errorf("query must not be empty")
	}
	if limit <= 0 {
		limit = defaultFindLimit
	}

	needle := strings.ToLower(trimmed)
	pattern := "%" + escapeLike(needle) + "%"

	rows, err := idx.db.Query(
		`SELECT gs.symbol, gs.display_name, gs.kind, gs.documentation,
		        MIN(d.relative_path) AS file_path,
		        MIN(d.language) AS language,
		        MIN(der.start_line) AS line
		 FROM global_symbols gs
		 LEFT JOIN defn_enclosing_ranges der ON der.symbol_id = gs.id
		 LEFT JOIN documents d ON d.id = der.document_id
		 WHERE LOWER(gs.symbol) LIKE ? ESCAPE '!'
		    OR LOWER(COALESCE(gs.display_name, '')) LIKE ? ESCAPE '!'
		 GROUP BY gs.id
		 ORDER BY CASE
		          WHEN LOWER(COALESCE(gs.display_name, '')) = ? THEN 0
		          WHEN LOWER(gs.symbol) = ? THEN 1
		          ELSE 2
		         END,
		         gs.symbol
		 LIMIT ?`,
		pattern,
		pattern,
		needle,
		needle,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var symbols []Symbol
	for rows.Next() {
		var symbolID string
		var displayName sql.NullString
		var kind sql.NullInt64
		var docText sql.NullString
		var filePath sql.NullString
		var language sql.NullString
		var line sql.NullInt64

		if err := rows.Scan(&symbolID, &displayName, &kind, &docText, &filePath, &language, &line); err != nil {
			return nil, err
		}

		symbols = append(symbols, Symbol{
			ID:            symbolID,
			Name:          selectSymbolName(displayName, symbolID),
			Kind:          kindToString(kind),
			Documentation: splitDocumentation(docText),
			FilePath:      nullStringValue(filePath),
			Line:          int(line.Int64),
			Language:      nullStringValue(language),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return symbols, nil
}

// GetDefinition retrieves symbol definition with documentation.
func (idx *Index) GetDefinition(symbolID string) (*Symbol, error) {
	if cached, ok := idx.symbolCache.Get(symbolID); ok {
		return cached, nil
	}

	var dbID int64
	var displayName sql.NullString
	var kind sql.NullInt64
	var docText sql.NullString

	row := idx.db.QueryRow(
		`SELECT id, display_name, kind, documentation
		 FROM global_symbols
		 WHERE symbol = ?`,
		symbolID,
	)
	if err := row.Scan(&dbID, &displayName, &kind, &docText); err != nil {
		if err == sql.ErrNoRows {
			return nil, SymbolNotFoundError{Symbol: symbolID}
		}
		return nil, err
	}

	defRow := idx.db.QueryRow(
		`SELECT d.relative_path, d.language, der.start_line
		 FROM defn_enclosing_ranges der
		 JOIN documents d ON d.id = der.document_id
		 WHERE der.symbol_id = ?
		 ORDER BY der.start_line
		 LIMIT 1`,
		dbID,
	)

	var filePath sql.NullString
	var language sql.NullString
	var line sql.NullInt64
	if err := defRow.Scan(&filePath, &language, &line); err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	symbol := &Symbol{
		ID:            symbolID,
		Name:          selectSymbolName(displayName, symbolID),
		Kind:          kindToString(kind),
		Documentation: splitDocumentation(docText),
		FilePath:      nullStringValue(filePath),
		Line:          int(line.Int64),
		Language:      nullStringValue(language),
	}

	idx.symbolCache.Add(symbolID, symbol)
	return symbol, nil
}

// GetReferences finds all references to a symbol.
func (idx *Index) GetReferences(symbolID string) ([]Occurrence, error) {
	dbID, err := idx.lookupSymbolID(symbolID)
	if err != nil {
		return nil, err
	}

	rows, err := idx.db.Query(
		`SELECT DISTINCT c.occurrences, d.relative_path
		 FROM mentions m
		 JOIN chunks c ON c.id = m.chunk_id
		 JOIN documents d ON d.id = c.document_id
		 WHERE m.symbol_id = ?`,
		dbID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var occurrences []Occurrence
	for rows.Next() {
		var blob []byte
		var filePath string
		if err := rows.Scan(&blob, &filePath); err != nil {
			return nil, err
		}

		chunkOccurrences, err := decodeOccurrences(blob)
		if err != nil {
			return nil, IndexCorruptedError{Err: err}
		}

		for _, occ := range chunkOccurrences {
			if occ.Symbol != symbolID {
				continue
			}
			if scip.SymbolRole_Definition.Matches(occ) {
				continue
			}

			rng, err := scip.NewRange(occ.Range)
			if err != nil {
				return nil, IndexCorruptedError{Err: err}
			}

			occurrences = append(occurrences, Occurrence{
				Symbol:   symbolID,
				FilePath: filePath,
				Line:     int(rng.Start.Line),
				Column:   int(rng.Start.Character),
				Role:     roleFromSymbolRoles(occ.SymbolRoles),
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return occurrences, nil
}

// GetSymbolsInFile lists all symbols defined in a file.
func (idx *Index) GetSymbolsInFile(filePath string) ([]Symbol, error) {
	rows, err := idx.db.Query(
		`SELECT d.id, d.language
		 FROM documents d
		 WHERE d.relative_path = ?`,
		filePath,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Symbol
	for rows.Next() {
		var docID int64
		var language sql.NullString
		if err := rows.Scan(&docID, &language); err != nil {
			return nil, err
		}

		symbolRows, err := idx.db.Query(
			`SELECT gs.symbol, gs.display_name, gs.kind, gs.documentation, der.start_line
			 FROM defn_enclosing_ranges der
			 JOIN global_symbols gs ON gs.id = der.symbol_id
			 WHERE der.document_id = ?
			 ORDER BY der.start_line`,
			docID,
		)
		if err != nil {
			return nil, err
		}

		for symbolRows.Next() {
			var symbolID string
			var displayName sql.NullString
			var kind sql.NullInt64
			var docText sql.NullString
			var line sql.NullInt64
			if err := symbolRows.Scan(&symbolID, &displayName, &kind, &docText, &line); err != nil {
				symbolRows.Close()
				return nil, err
			}

			results = append(results, Symbol{
				ID:            symbolID,
				Name:          selectSymbolName(displayName, symbolID),
				Kind:          kindToString(kind),
				Documentation: splitDocumentation(docText),
				FilePath:      filePath,
				Line:          int(line.Int64),
				Language:      nullStringValue(language),
			})
		}
		if err := symbolRows.Err(); err != nil {
			symbolRows.Close()
			return nil, err
		}
		symbolRows.Close()
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

// GetStats retrieves basic counts from the index.
func (idx *Index) GetStats() (IndexStats, error) {
	var stats IndexStats

	docRow := idx.db.QueryRow(`SELECT COUNT(*) FROM documents`)
	if err := docRow.Scan(&stats.Documents); err != nil {
		return IndexStats{}, err
	}

	symbolRow := idx.db.QueryRow(`SELECT COUNT(*) FROM global_symbols`)
	if err := symbolRow.Scan(&stats.Symbols); err != nil {
		return IndexStats{}, err
	}

	occurrenceRow := idx.db.QueryRow(`SELECT COUNT(*) FROM mentions`)
	if err := occurrenceRow.Scan(&stats.Occurrences); err != nil {
		return IndexStats{}, err
	}

	return stats, nil
}

// GetTypeInfo retrieves type information for a symbol.
func (idx *Index) GetTypeInfo(symbolID string) (string, error) {
	row := idx.db.QueryRow(`SELECT signature FROM global_symbols WHERE symbol = ?`, symbolID)
	var signature []byte
	if err := row.Scan(&signature); err != nil {
		if err == sql.ErrNoRows {
			return "", SymbolNotFoundError{Symbol: symbolID}
		}
		return "", err
	}
	if len(signature) == 0 {
		return "", nil
	}

	var doc scip.Document
	if err := proto.Unmarshal(signature, &doc); err != nil {
		return "", IndexCorruptedError{Err: err}
	}
	return strings.TrimSpace(doc.Text), nil
}

func (idx *Index) lookupSymbolID(symbolID string) (int64, error) {
	row := idx.db.QueryRow(`SELECT id FROM global_symbols WHERE symbol = ?`, symbolID)
	var dbID int64
	if err := row.Scan(&dbID); err != nil {
		if err == sql.ErrNoRows {
			return 0, SymbolNotFoundError{Symbol: symbolID}
		}
		return 0, err
	}
	return dbID, nil
}

func decodeOccurrences(blob []byte) ([]*scip.Occurrence, error) {
	if len(blob) == 0 {
		return nil, nil
	}
	decoder, err := zstd.NewReader(nil)
	if err != nil {
		return nil, err
	}
	defer decoder.Close()

	reader := bytes.NewReader(blob)
	if err := decoder.Reset(reader); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(decoder)
	if err != nil {
		return nil, err
	}

	var doc scip.Document
	if err := proto.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	return doc.Occurrences, nil
}

func selectSymbolName(displayName sql.NullString, symbolID string) string {
	if displayName.Valid && displayName.String != "" {
		return displayName.String
	}
	return symbolID
}

func kindToString(kind sql.NullInt64) string {
	if !kind.Valid {
		return ""
	}
	return scip.SymbolInformation_Kind(kind.Int64).String()
}

func splitDocumentation(docText sql.NullString) []string {
	if !docText.Valid || docText.String == "" {
		return nil
	}
	return strings.Split(docText.String, "\n")
}

func nullStringValue(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func roleFromSymbolRoles(symbolRoles int32) string {
	if symbolRoles&int32(scip.SymbolRole_Definition) != 0 {
		return "definition"
	}
	return "reference"
}

func escapeLike(input string) string {
	replacer := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_")
	return replacer.Replace(input)
}

func cloneSymbol(symbol *Symbol) *Symbol {
	if symbol == nil {
		return nil
	}
	clone := *symbol
	if symbol.Documentation != nil {
		clone.Documentation = append([]string(nil), symbol.Documentation...)
	}
	return &clone
}
