package otel

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	otelapi "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type RouteInfo struct {
	Route     string
	Category  string
	Operation string
}

type routeInfoKey struct{}

type APIErrorInfo struct {
	Status  int
	Code    string
	Message string
}

type apiErrorKey struct{}

type AgentResolver func(*http.Request, string) AgentAttributes

type APIMetrics struct {
	requestCounter  metric.Int64Counter
	requestDuration metric.Float64Histogram
	requestSize     metric.Int64Histogram
	responseSize    metric.Int64Histogram
	activeRequests  metric.Int64UpDownCounter
	errorCounter    metric.Int64Counter
	summary         *APISummaryStore
}

type apiMiddleware struct {
	metrics  *APIMetrics
	tracer   trace.Tracer
	resolver AgentResolver
}

type apiMiddlewareOptions struct {
	tracer   trace.Tracer
	resolver AgentResolver
	summary  *APISummaryStore
}

type APIMiddlewareOption func(*apiMiddlewareOptions)

func WithAPITracer(tracer trace.Tracer) APIMiddlewareOption {
	return func(options *apiMiddlewareOptions) {
		options.tracer = tracer
	}
}

func WithAPIResolver(resolver AgentResolver) APIMiddlewareOption {
	return func(options *apiMiddlewareOptions) {
		options.resolver = resolver
	}
}

func WithSummaryStore(store *APISummaryStore) APIMiddlewareOption {
	return func(options *apiMiddlewareOptions) {
		options.summary = store
	}
}

func NewAPIInstrumentationMiddleware(meter metric.Meter, opts ...APIMiddlewareOption) (func(http.Handler) http.Handler, error) {
	if meter == nil {
		meter = otelapi.GetMeterProvider().Meter("gestalt/api")
	}
	options := apiMiddlewareOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	if options.tracer == nil {
		options.tracer = otelapi.Tracer("gestalt/api")
	}
	metrics, err := newAPIMetrics(meter, options.summary)
	if err != nil {
		return nil, err
	}
	middleware := &apiMiddleware{
		metrics:  metrics,
		tracer:   options.tracer,
		resolver: options.resolver,
	}
	return middleware.wrap, nil
}

func WithRouteInfo(next http.Handler, info RouteInfo) http.Handler {
	if next == nil {
		return http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), routeInfoKey{}, info)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func RecordAPIError(ctx context.Context, info APIErrorInfo) {
	if ctx == nil {
		return
	}
	tracker, ok := ctx.Value(apiErrorKey{}).(*APIErrorInfo)
	if !ok || tracker == nil {
		return
	}
	*tracker = info
}

func apiErrorFromContext(ctx context.Context) (APIErrorInfo, bool) {
	if ctx == nil {
		return APIErrorInfo{}, false
	}
	tracker, ok := ctx.Value(apiErrorKey{}).(*APIErrorInfo)
	if !ok || tracker == nil {
		return APIErrorInfo{}, false
	}
	if tracker.Status == 0 && tracker.Code == "" && tracker.Message == "" {
		return APIErrorInfo{}, false
	}
	return *tracker, true
}

func newAPIMetrics(meter metric.Meter, summary *APISummaryStore) (*APIMetrics, error) {
	requestCounter, err := meter.Int64Counter(MetricRequestCount,
		metric.WithDescription("Total HTTP requests"),
	)
	if err != nil {
		return nil, err
	}
	requestDuration, err := meter.Float64Histogram(MetricRequestDuration,
		metric.WithDescription("HTTP request duration"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}
	requestSize, err := meter.Int64Histogram(MetricRequestSize,
		metric.WithDescription("HTTP request size"),
		metric.WithUnit("By"),
	)
	if err != nil {
		return nil, err
	}
	responseSize, err := meter.Int64Histogram(MetricResponseSize,
		metric.WithDescription("HTTP response size"),
		metric.WithUnit("By"),
	)
	if err != nil {
		return nil, err
	}
	activeRequests, err := meter.Int64UpDownCounter(MetricActiveRequests,
		metric.WithDescription("Active HTTP requests"),
	)
	if err != nil {
		return nil, err
	}
	errorCounter, err := meter.Int64Counter(MetricAPIErrorCount,
		metric.WithDescription("HTTP error count"),
	)
	if err != nil {
		return nil, err
	}
	if summary == nil {
		summary = NewAPISummaryStore()
	}
	return &APIMetrics{
		requestCounter:  requestCounter,
		requestDuration: requestDuration,
		requestSize:     requestSize,
		responseSize:    responseSize,
		activeRequests:  activeRequests,
		errorCounter:    errorCounter,
		summary:         summary,
	}, nil
}

func (middleware *apiMiddleware) wrap(next http.Handler) http.Handler {
	if next == nil {
		return http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if middleware == nil || middleware.metrics == nil {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		routeInfo := resolveRouteInfo(r)
		requestSize, bodyAgentID, counter := prepareRequest(r, routeInfo)

		agentAttributes := resolveAgentAttributes(middleware.resolver, r, bodyAgentID)
		attributes := buildAttributes(r, routeInfo, agentAttributes, 0)
		statusAttributes := attributes

		ctx := otelapi.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		ctx = context.WithValue(ctx, apiErrorKey{}, &APIErrorInfo{})
		ctx, span := middleware.tracer.Start(ctx, spanNameHTTPRequest,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(attributes...),
		)
		defer span.End()
		r = r.WithContext(ctx)

		activeAttrs := buildActiveAttributes(r, routeInfo, agentAttributes)
		middleware.metrics.activeRequests.Add(ctx, 1, metric.WithAttributes(activeAttrs...))
		span.AddEvent("request.received")

		if agentAttributes.Name != "" || agentAttributes.ID != "" {
			span.AddEvent("agent.resolved", trace.WithAttributes(agentAttributes.AsSpanAttributes()...))
		}
		if agentAttributes.TerminalID != "" {
			span.AddEvent("terminal.session_found", trace.WithAttributes(attribute.String("terminal.id", agentAttributes.TerminalID)))
		}

		responseRecorder := &statusRecorder{ResponseWriter: w}
		childSpanName := criticalSpanName(routeInfo, r.Method)
		var childSpan trace.Span
		if childSpanName != "" {
			ctx, childSpan = middleware.tracer.Start(ctx, childSpanName)
			r = r.WithContext(ctx)
		}

		next.ServeHTTP(responseRecorder, r)

		if childSpan != nil {
			if requestSize > 0 {
				childSpan.SetAttributes(attribute.Int64("request.size", requestSize))
			}
			if responseRecorder.bytes > 0 {
				childSpan.SetAttributes(attribute.Int64("response.size", responseRecorder.bytes))
			}
			if agentAttributes.ID != "" {
				childSpan.SetAttributes(attribute.String("agent.id", agentAttributes.ID))
			}
			if agentAttributes.TerminalID != "" {
				childSpan.SetAttributes(attribute.String("terminal.id", agentAttributes.TerminalID))
			}
			childSpan.End()
		}

		status := responseRecorder.status
		if status == 0 {
			status = http.StatusOK
		}

		if requestSize <= 0 && counter != nil {
			requestSize = counter.count
		}

		durationSeconds := time.Since(start).Seconds()
		statusAttributes = buildAttributes(r, routeInfo, agentAttributes, status)
		span.SetAttributes(attribute.Int("http.status_code", status))
		span.AddEvent("response.prepared", trace.WithAttributes(
			attribute.Int("http.status_code", status),
		))

		middleware.metrics.requestCounter.Add(ctx, 1, metric.WithAttributes(statusAttributes...))
		middleware.metrics.requestDuration.Record(ctx, durationSeconds, metric.WithAttributes(durationAttributes(routeInfo, agentAttributes)...))
		if requestSize > 0 {
			middleware.metrics.requestSize.Record(ctx, requestSize, metric.WithAttributes(statusAttributes...))
			span.AddEvent("request.body_read", trace.WithAttributes(attribute.Int64("request.size", requestSize)))
		}
		if responseRecorder.bytes > 0 {
			middleware.metrics.responseSize.Record(ctx, responseRecorder.bytes, metric.WithAttributes(statusAttributes...))
		}

		errorInfo, hasErrorInfo := apiErrorFromContext(ctx)
		hasError := status >= http.StatusBadRequest
		if hasErrorInfo {
			hasError = true
		}
		if hasError {
			errorType := errorTypeForStatus(status)
			if hasErrorInfo && errorInfo.Code != "" {
				errorType = errorInfo.Code
			}
			errorAttrs := append([]attribute.KeyValue{}, statusAttributes...)
			errorAttrs = append(errorAttrs, attribute.String("error_type", errorType))
			middleware.metrics.errorCounter.Add(ctx, 1, metric.WithAttributes(errorAttrs...))
			span.SetStatus(codes.Error, errorInfo.Message)
			if errorInfo.Message != "" {
				span.RecordError(&apiErrorWrapper{message: errorInfo.Message})
				span.AddEvent("error", trace.WithAttributes(
					attribute.String("exception.type", errorType),
					attribute.String("exception.message", errorInfo.Message),
				))
			}
		}

		middleware.metrics.activeRequests.Add(ctx, -1, metric.WithAttributes(activeAttrs...))
		span.AddEvent("response.sent", trace.WithAttributes(
			attribute.Int64("response.size", responseRecorder.bytes),
		))

		if middleware.metrics.summary != nil {
			middleware.metrics.summary.Record(APISample{
				Route:           routeInfo.Route,
				Category:        routeInfo.Category,
				AgentName:       agentAttributes.Name,
				DurationSeconds: durationSeconds,
				HasError:        hasError,
			})
		}
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (recorder *statusRecorder) WriteHeader(statusCode int) {
	recorder.status = statusCode
	recorder.ResponseWriter.WriteHeader(statusCode)
}

func (recorder *statusRecorder) Write(data []byte) (int, error) {
	if recorder.status == 0 {
		recorder.status = http.StatusOK
	}
	n, err := recorder.ResponseWriter.Write(data)
	recorder.bytes += int64(n)
	return n, err
}

type countingReadCloser struct {
	io.ReadCloser
	count int64
}

func (reader *countingReadCloser) Read(data []byte) (int, error) {
	n, err := reader.ReadCloser.Read(data)
	reader.count += int64(n)
	return n, err
}

type apiErrorWrapper struct {
	message string
}

func (err *apiErrorWrapper) Error() string {
	return err.message
}

func resolveRouteInfo(r *http.Request) RouteInfo {
	info := RouteInfo{}
	if r != nil {
		if ctxInfo, ok := r.Context().Value(routeInfoKey{}).(RouteInfo); ok {
			info = ctxInfo
		}
		info.Route = normalizeRoute(info.Route, r.URL.Path)
		if info.Category == "" {
			info.Category = categoryForPath(r.URL.Path)
		}
		if info.Operation == "" || info.Operation == "auto" {
			info.Operation = operationForMethod(r.Method)
		}
	}
	if info.Route == "" && r != nil {
		info.Route = r.URL.Path
	}
	return info
}

func normalizeRoute(route, path string) string {
	if path == "" {
		return route
	}
	if strings.HasPrefix(path, "/api/terminals") {
		return terminalRoute(path)
	}
	if strings.HasPrefix(path, "/api/agents") {
		return agentsRoute(path)
	}
	if strings.HasPrefix(path, "/api/skills") {
		return skillsRoute(path)
	}
	if route != "" {
		return route
	}
	return path
}

func categoryForPath(path string) string {
	switch {
	case strings.HasPrefix(path, "/api/terminals"):
		return "terminals"
	case strings.HasPrefix(path, "/api/agents"):
		return "agents"
	case strings.HasPrefix(path, "/api/workflows"):
		return "workflows"
	case strings.HasPrefix(path, "/api/config"):
		return "config"
	case strings.HasPrefix(path, "/api/otel"):
		return "otel"
	case strings.HasPrefix(path, "/api/skills"):
		return "skills"
	case strings.HasPrefix(path, "/api/plans"):
		return "plan"
	case strings.HasPrefix(path, "/api/status"):
		return "status"
	case strings.HasPrefix(path, "/api/metrics"):
		return "status"
	case strings.HasPrefix(path, "/api/events"):
		return "status"
	default:
		return "status"
	}
}

func operationForMethod(method string) string {
	switch strings.ToUpper(method) {
	case http.MethodPost:
		return "create"
	case http.MethodPut, http.MethodPatch:
		return "update"
	case http.MethodDelete:
		return "delete"
	case http.MethodGet:
		return "read"
	default:
		return "query"
	}
}

func criticalSpanName(routeInfo RouteInfo, method string) string {
	if method == http.MethodPost && routeInfo.Route == "/api/terminals" {
		return spanNameTerminalCreate
	}
	if method == http.MethodDelete && routeInfo.Route == "/api/terminals/:id" {
		return spanNameTerminalDelete
	}
	if method == http.MethodPost && routeInfo.Route == "/api/agents/:name/input" {
		return spanNameAgentInput
	}
	if method == http.MethodGet && routeInfo.Route == "/api/terminals/:id/output" {
		return spanNameTerminalOutput
	}
	return ""
}

func terminalRoute(path string) string {
	trimmed := strings.TrimPrefix(path, "/api/terminals")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return "/api/terminals"
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) == 1 {
		return "/api/terminals/:id"
	}
	switch parts[1] {
	case "output":
		return "/api/terminals/:id/output"
	case "history":
		return "/api/terminals/:id/history"
	case "input-history":
		return "/api/terminals/:id/input-history"
	case "bell":
		return "/api/terminals/:id/bell"
	case "workflow":
		if len(parts) >= 3 {
			switch parts[2] {
			case "resume":
				return "/api/terminals/:id/workflow/resume"
			case "history":
				return "/api/terminals/:id/workflow/history"
			}
		}
	}
	return "/api/terminals/:id"
}

func agentsRoute(path string) string {
	trimmed := strings.TrimPrefix(path, "/api/agents")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return "/api/agents"
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) == 2 && parts[1] == "input" {
		return "/api/agents/:name/input"
	}
	return "/api/agents"
}

func skillsRoute(path string) string {
	trimmed := strings.TrimPrefix(path, "/api/skills")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return "/api/skills"
	}
	return "/api/skills/:name"
}

func prepareRequest(r *http.Request, routeInfo RouteInfo) (int64, string, *countingReadCloser) {
	if r == nil {
		return 0, "", nil
	}
	if r.Body == nil {
		r.Body = http.NoBody
	}
	var bodyAgentID string
	var bodyBytes []byte
	if routeInfo.Route == "/api/terminals" && r.Method == http.MethodPost {
		payload, err := io.ReadAll(r.Body)
		if err == nil {
			bodyBytes = payload
			bodyAgentID = parseAgentIDFromBody(payload)
		}
		_ = r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}
	counter := &countingReadCloser{ReadCloser: r.Body}
	r.Body = counter
	if len(bodyBytes) > 0 {
		return int64(len(bodyBytes)), bodyAgentID, counter
	}
	if r.ContentLength > 0 {
		return r.ContentLength, bodyAgentID, counter
	}
	return 0, bodyAgentID, counter
}

func parseAgentIDFromBody(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	var request struct {
		Agent string `json:"agent"`
	}
	if err := json.Unmarshal(payload, &request); err != nil {
		return ""
	}
	return strings.TrimSpace(request.Agent)
}

type AgentAttributes struct {
	ID         string
	Name       string
	Type       string
	TerminalID string
}

func (attrs AgentAttributes) AsSpanAttributes() []attribute.KeyValue {
	return buildAgentAttributes(attrs)
}

func resolveAgentAttributes(resolver AgentResolver, r *http.Request, bodyAgentID string) AgentAttributes {
	info := AgentAttributes{Type: "unknown"}
	if resolver != nil {
		info = resolver(r, bodyAgentID)
	}
	if info.Type == "" {
		info.Type = "unknown"
	}
	if r == nil {
		return info
	}
	if info.TerminalID == "" {
		info.TerminalID = terminalIDFromPath(r.URL.Path)
	}
	if info.Name == "" {
		info.Name = agentNameFromPath(r.URL.Path)
	}
	if info.ID == "" {
		info.ID = bodyAgentID
	}
	return info
}

func terminalIDFromPath(path string) string {
	trimmed := strings.TrimPrefix(path, "/api/terminals/")
	if trimmed == path {
		return ""
	}
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return ""
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func agentNameFromPath(path string) string {
	trimmed := strings.TrimSuffix(path, "/")
	const prefix = "/api/agents/"
	if !strings.HasPrefix(trimmed, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(trimmed, prefix)
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[1] != "input" {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

func buildAttributes(r *http.Request, routeInfo RouteInfo, agentAttributes AgentAttributes, statusCode int) []attribute.KeyValue {
	attributes := make([]attribute.KeyValue, 0, 14)
	if r != nil {
		attributes = append(attributes,
			attribute.String("http.method", r.Method),
			attribute.String("http.target", sanitizeTarget(r)),
			attribute.String("http.scheme", requestScheme(r)),
			attribute.String("user_agent", r.UserAgent()),
		)
	}
	if routeInfo.Route != "" {
		attributes = append(attributes, attribute.String("http.route", routeInfo.Route))
	}
	if statusCode > 0 {
		attributes = append(attributes, attribute.Int("http.status_code", statusCode))
	}
	if routeInfo.Category != "" {
		attributes = append(attributes, attribute.String("api.category", routeInfo.Category))
	}
	if routeInfo.Operation != "" {
		attributes = append(attributes, attribute.String("api.operation", routeInfo.Operation))
	}
	attributes = append(attributes, buildAgentAttributes(agentAttributes)...)
	if agentAttributes.TerminalID != "" {
		attributes = append(attributes, attribute.String("terminal.id", agentAttributes.TerminalID))
	}
	return attributes
}

func buildAgentAttributes(agentAttributes AgentAttributes) []attribute.KeyValue {
	attributes := make([]attribute.KeyValue, 0, 3)
	if agentAttributes.ID != "" {
		attributes = append(attributes, attribute.String("agent.id", agentAttributes.ID))
	}
	if agentAttributes.Name != "" {
		attributes = append(attributes, attribute.String("agent.name", agentAttributes.Name))
	}
	if agentAttributes.Type != "" {
		attributes = append(attributes, attribute.String("agent.type", agentAttributes.Type))
	}
	return attributes
}

func buildActiveAttributes(r *http.Request, routeInfo RouteInfo, agentAttributes AgentAttributes) []attribute.KeyValue {
	attributes := make([]attribute.KeyValue, 0, 10)
	if r != nil {
		attributes = append(attributes,
			attribute.String("http.method", r.Method),
			attribute.String("http.scheme", requestScheme(r)),
		)
	}
	if routeInfo.Route != "" {
		attributes = append(attributes, attribute.String("http.route", routeInfo.Route))
	}
	if routeInfo.Category != "" {
		attributes = append(attributes, attribute.String("api.category", routeInfo.Category))
	}
	attributes = append(attributes, buildAgentAttributes(agentAttributes)...)
	if agentAttributes.TerminalID != "" {
		attributes = append(attributes, attribute.String("terminal.id", agentAttributes.TerminalID))
	}
	return attributes
}

func durationAttributes(routeInfo RouteInfo, agentAttributes AgentAttributes) []attribute.KeyValue {
	attributes := make([]attribute.KeyValue, 0, 6)
	if routeInfo.Route != "" {
		attributes = append(attributes, attribute.String("http.route", routeInfo.Route))
	}
	if routeInfo.Category != "" {
		attributes = append(attributes, attribute.String("api.category", routeInfo.Category))
	}
	attributes = append(attributes, buildAgentAttributes(agentAttributes)...)
	return attributes
}

func sanitizeTarget(r *http.Request) string {
	if r == nil || r.URL == nil {
		return ""
	}
	copyURL := *r.URL
	query := copyURL.Query()
	query.Del("token")
	copyURL.RawQuery = query.Encode()
	return copyURL.RequestURI()
}

func requestScheme(r *http.Request) string {
	if r == nil {
		return "http"
	}
	if r.URL != nil && r.URL.Scheme != "" {
		return r.URL.Scheme
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func errorTypeForStatus(status int) string {
	switch {
	case status >= http.StatusInternalServerError:
		return "internal_error"
	case status >= http.StatusBadRequest:
		return "client_error"
	default:
		return ""
	}
}
