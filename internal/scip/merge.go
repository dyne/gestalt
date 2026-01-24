//go:build !noscip

package scip

import (
	"fmt"
	"os"
	"path/filepath"

	scip "github.com/sourcegraph/scip/bindings/go/scip"
	"google.golang.org/protobuf/proto"
)

// MergeIndexes combines multiple .scip files into a single .scip file.
func MergeIndexes(inputs []string, output string) error {
	if len(inputs) == 0 {
		return fmt.Errorf("no input indexes provided")
	}
	if output == "" {
		return fmt.Errorf("output path is required")
	}
	for _, input := range inputs {
		if input == output {
			return fmt.Errorf("output path must differ from input: %s", input)
		}
	}

	merged := &scip.Index{}
	documentPaths := make(map[string]struct{})
	externalSymbols := make(map[string]struct{})

	for _, input := range inputs {
		payload, err := os.ReadFile(input)
		if err != nil {
			return fmt.Errorf("read scip index %s: %w", input, err)
		}

		var index scip.Index
		if err := proto.Unmarshal(payload, &index); err != nil {
			return fmt.Errorf("decode scip index %s: %w", input, err)
		}

		if merged.Metadata == nil && index.Metadata != nil {
			merged.Metadata = proto.Clone(index.Metadata).(*scip.Metadata)
		}

		for _, doc := range index.Documents {
			if doc == nil {
				continue
			}
			relativePath := doc.RelativePath
			if relativePath == "" {
				return fmt.Errorf("document with empty relative path in %s", input)
			}
			if _, exists := documentPaths[relativePath]; exists {
				return fmt.Errorf("duplicate document path %s in %s", relativePath, input)
			}
			documentPaths[relativePath] = struct{}{}
			merged.Documents = append(merged.Documents, doc)
		}

		for _, symbol := range index.ExternalSymbols {
			if symbol == nil || symbol.Symbol == "" {
				continue
			}
			if _, exists := externalSymbols[symbol.Symbol]; exists {
				continue
			}
			externalSymbols[symbol.Symbol] = struct{}{}
			merged.ExternalSymbols = append(merged.ExternalSymbols, symbol)
		}
	}

	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	payload, err := proto.Marshal(merged)
	if err != nil {
		return fmt.Errorf("encode scip index: %w", err)
	}
	if err := os.WriteFile(output, payload, 0o644); err != nil {
		return fmt.Errorf("write merged scip index: %w", err)
	}
	return nil
}
