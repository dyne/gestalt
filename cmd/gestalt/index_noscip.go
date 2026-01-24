//go:build noscip

package main

import (
	"fmt"
	"io"
)

func runIndexCommand(_ []string, out io.Writer, errOut io.Writer) int {
	message := "SCIP support disabled at build time. Rebuild without -tags noscip to enable `gestalt index`."
	if errOut != nil {
		fmt.Fprintln(errOut, message)
	} else if out != nil {
		fmt.Fprintln(out, message)
	}
	return 1
}
