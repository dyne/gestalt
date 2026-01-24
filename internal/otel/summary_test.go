package otel

import (
	"testing"
	"time"
)

func TestAPISummaryStore(t *testing.T) {
	store := NewAPISummaryStore()
	store.Record(APISample{Route: "/api/terminals", Category: "terminals", AgentName: "alpha", DurationSeconds: 0.2, HasError: false})
	store.Record(APISample{Route: "/api/terminals", Category: "terminals", AgentName: "alpha", DurationSeconds: 0.4, HasError: true})
	store.Record(APISample{Route: "/api/status", Category: "status", AgentName: "beta", DurationSeconds: 0.1, HasError: false})

	summary := store.Summary(time.Now().UTC())
	if len(summary.TopEndpoints) == 0 || summary.TopEndpoints[0].Route != "/api/terminals" {
		t.Fatalf("expected /api/terminals top endpoint, got %#v", summary.TopEndpoints)
	}
	if len(summary.TopAgents) == 0 || summary.TopAgents[0].Name != "alpha" {
		t.Fatalf("expected alpha top agent, got %#v", summary.TopAgents)
	}
	found := false
	for _, entry := range summary.ErrorRates {
		if entry.Category == "terminals" {
			found = true
			if entry.Total != 2 || entry.Errors != 1 {
				t.Fatalf("expected terminals error stats 2/1, got %+v", entry)
			}
		}
	}
	if !found {
		t.Fatalf("expected terminals error rate entry")
	}
}
