package api

import "testing"

func FuzzParseTerminalPath(f *testing.F) {
	seeds := []string{
		"",
		"/api/terminals/1",
		"/api/terminals/1/output",
		"/api/terminals/1/history",
		"/api/terminals/",
		"/api/terminals//output",
		"/api/terminals//history",
		"/api/terminals/abc/extra",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, path string) {
		_, _ = parseTerminalPath(path)
	})
}
