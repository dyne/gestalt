package api

import "testing"

func FuzzParseTerminalPath(f *testing.F) {
	seeds := []string{
		"",
		"/api/sessions/1",
		"/api/sessions/1/output",
		"/api/sessions/1/history",
		"/api/sessions/",
		"/api/sessions//output",
		"/api/sessions//history",
		"/api/sessions/abc/extra",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, path string) {
		_, _, _ = parseTerminalPath(path)
	})
}
