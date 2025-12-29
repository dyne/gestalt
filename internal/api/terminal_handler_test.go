package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidateToken(t *testing.T) {
	tests := []struct {
		name       string
		token      string
		authHeader string
		queryToken string
		want       bool
	}{
		{name: "no-token-required", token: "", want: true},
		{name: "bearer-ok", token: "secret", authHeader: "Bearer secret", want: true},
		{name: "bearer-wrong", token: "secret", authHeader: "Bearer nope", want: false},
		{name: "query-ok", token: "secret", queryToken: "secret", want: true},
		{name: "query-wrong", token: "secret", queryToken: "nope", want: false},
		{name: "header-overrides-query", token: "secret", authHeader: "Bearer secret", queryToken: "nope", want: true},
		{name: "missing-auth", token: "secret", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			url := "/"
			if test.queryToken != "" {
				url = "/?token=" + test.queryToken
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			if test.authHeader != "" {
				req.Header.Set("Authorization", test.authHeader)
			}
			if got := validateToken(req, test.token); got != test.want {
				t.Fatalf("expected %v, got %v", test.want, got)
			}
		})
	}
}
