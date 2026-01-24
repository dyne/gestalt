package otel

var RequestDurationBuckets = []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0, 5.0, 10.0}

const (
	MetricRequestCount     = "http.server.request.count"
	MetricRequestDuration  = "http.server.request.duration"
	MetricRequestSize      = "http.server.request.size"
	MetricResponseSize     = "http.server.response.size"
	MetricActiveRequests   = "http.server.active_requests"
	MetricAPIErrorCount    = "api.errors.count"
	spanNameHTTPRequest    = "http.server.request"
	spanNameTerminalCreate = "terminal.create"
	spanNameTerminalDelete = "terminal.delete"
	spanNameAgentInput     = "agent.input"
	spanNameTerminalOutput = "terminal.output"
)
