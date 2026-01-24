package otel

import (
	"sort"
	"sync"
	"time"
)

const defaultSummaryTTL = 60 * time.Second

type EndpointSummary struct {
	Route string `json:"route"`
	Count int64  `json:"count"`
}

type EndpointLatencySummary struct {
	Route   string  `json:"route"`
	P99Secs float64 `json:"p99_seconds"`
	Count   int64   `json:"count"`
}

type AgentSummary struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

type CategoryErrorRate struct {
	Category     string  `json:"category"`
	Total        int64   `json:"total"`
	Errors       int64   `json:"errors"`
	ErrorRatePct float64 `json:"error_rate_pct"`
}

type MetricsSummary struct {
	UpdatedAt        time.Time                `json:"updated_at"`
	TopEndpoints     []EndpointSummary        `json:"top_endpoints"`
	SlowestEndpoints []EndpointLatencySummary `json:"slowest_endpoints"`
	TopAgents        []AgentSummary           `json:"top_agents"`
	ErrorRates       []CategoryErrorRate      `json:"error_rates"`
}

type APISummaryStore struct {
	mu            sync.RWMutex
	routes        map[string]*routeStats
	agents        map[string]int64
	categories    map[string]*categoryStats
	lastSummaryAt time.Time
	cachedSummary MetricsSummary
	cacheTTL      time.Duration
}

type routeStats struct {
	count    int64
	buckets  []int64
	lastSeen time.Time
}

type categoryStats struct {
	total  int64
	errors int64
}

type APISample struct {
	Route           string
	Category        string
	AgentName       string
	DurationSeconds float64
	HasError        bool
}

func NewAPISummaryStore() *APISummaryStore {
	return &APISummaryStore{
		routes:     make(map[string]*routeStats),
		agents:     make(map[string]int64),
		categories: make(map[string]*categoryStats),
		cacheTTL:   defaultSummaryTTL,
	}
}

func (store *APISummaryStore) Record(sample APISample) {
	if store == nil {
		return
	}
	store.mu.Lock()
	defer store.mu.Unlock()

	if sample.Route != "" {
		stats := store.routes[sample.Route]
		if stats == nil {
			stats = &routeStats{buckets: make([]int64, len(RequestDurationBuckets)+1)}
			store.routes[sample.Route] = stats
		}
		stats.count++
		stats.lastSeen = time.Now().UTC()
		bucketIndex := durationBucketIndex(sample.DurationSeconds)
		stats.buckets[bucketIndex]++
	}

	if sample.AgentName != "" {
		store.agents[sample.AgentName]++
	}

	category := sample.Category
	if category == "" {
		category = "unknown"
	}
	stats := store.categories[category]
	if stats == nil {
		stats = &categoryStats{}
		store.categories[category] = stats
	}
	stats.total++
	if sample.HasError {
		stats.errors++
	}
}

func (store *APISummaryStore) Summary(now time.Time) MetricsSummary {
	if store == nil {
		return MetricsSummary{UpdatedAt: now}
	}
	store.mu.RLock()
	if !store.lastSummaryAt.IsZero() && now.Sub(store.lastSummaryAt) < store.cacheTTL {
		summary := store.cachedSummary
		store.mu.RUnlock()
		return summary
	}
	store.mu.RUnlock()

	store.mu.Lock()
	defer store.mu.Unlock()
	if !store.lastSummaryAt.IsZero() && now.Sub(store.lastSummaryAt) < store.cacheTTL {
		return store.cachedSummary
	}

	endpoints := make([]EndpointSummary, 0, len(store.routes))
	slowest := make([]EndpointLatencySummary, 0, len(store.routes))
	for route, stats := range store.routes {
		endpoints = append(endpoints, EndpointSummary{Route: route, Count: stats.count})
		p99 := estimateP99(stats.buckets, stats.count)
		slowest = append(slowest, EndpointLatencySummary{Route: route, P99Secs: p99, Count: stats.count})
	}
	agents := make([]AgentSummary, 0, len(store.agents))
	for name, count := range store.agents {
		agents = append(agents, AgentSummary{Name: name, Count: count})
	}
	categories := make([]CategoryErrorRate, 0, len(store.categories))
	for category, stats := range store.categories {
		pct := 0.0
		if stats.total > 0 {
			pct = (float64(stats.errors) / float64(stats.total)) * 100
		}
		categories = append(categories, CategoryErrorRate{
			Category:     category,
			Total:        stats.total,
			Errors:       stats.errors,
			ErrorRatePct: pct,
		})
	}

	sort.Slice(endpoints, func(i, j int) bool { return endpoints[i].Count > endpoints[j].Count })
	sort.Slice(slowest, func(i, j int) bool { return slowest[i].P99Secs > slowest[j].P99Secs })
	sort.Slice(agents, func(i, j int) bool { return agents[i].Count > agents[j].Count })
	sort.Slice(categories, func(i, j int) bool { return categories[i].ErrorRatePct > categories[j].ErrorRatePct })

	summary := MetricsSummary{
		UpdatedAt:        now,
		TopEndpoints:     limitEndpoints(endpoints, 10),
		SlowestEndpoints: limitSlowest(slowest, 10),
		TopAgents:        limitAgents(agents, 10),
		ErrorRates:       categories,
	}
	store.cachedSummary = summary
	store.lastSummaryAt = now
	return summary
}

func durationBucketIndex(durationSeconds float64) int {
	for index, bound := range RequestDurationBuckets {
		if durationSeconds <= bound {
			return index
		}
	}
	return len(RequestDurationBuckets)
}

func estimateP99(buckets []int64, total int64) float64 {
	if total == 0 || len(buckets) == 0 {
		return 0
	}
	target := int64(float64(total) * 0.99)
	if target < 1 {
		target = 1
	}
	cumulative := int64(0)
	for index, count := range buckets {
		cumulative += count
		if cumulative >= target {
			if index < len(RequestDurationBuckets) {
				return RequestDurationBuckets[index]
			}
			return RequestDurationBuckets[len(RequestDurationBuckets)-1]
		}
	}
	return RequestDurationBuckets[len(RequestDurationBuckets)-1]
}

func limitEndpoints(values []EndpointSummary, limit int) []EndpointSummary {
	if len(values) <= limit {
		return values
	}
	return values[:limit]
}

func limitSlowest(values []EndpointLatencySummary, limit int) []EndpointLatencySummary {
	if len(values) <= limit {
		return values
	}
	return values[:limit]
}

func limitAgents(values []AgentSummary, limit int) []AgentSummary {
	if len(values) <= limit {
		return values
	}
	return values[:limit]
}
