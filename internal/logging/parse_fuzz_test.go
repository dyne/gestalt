package logging

import "testing"

func FuzzParseLevel(f *testing.F) {
	seeds := []string{"info", "warn", "warning", "error", "debug", "", "???", "INFO"}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		_, _ = ParseLevel(raw)
	})
}
