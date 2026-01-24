//go:build !noscip

package scip

import "fmt"

// SymbolNotFoundError indicates a missing symbol in the SCIP index.
type SymbolNotFoundError struct {
	Symbol string
}

func (err SymbolNotFoundError) Error() string {
	return fmt.Sprintf("symbol not found: %s", err.Symbol)
}

// IndexCorruptedError indicates the SCIP index data could not be decoded.
type IndexCorruptedError struct {
	Err error
}

func (err IndexCorruptedError) Error() string {
	return fmt.Sprintf("scip index corrupted: %v", err.Err)
}

func (err IndexCorruptedError) Unwrap() error {
	return err.Err
}
