package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gestalt/internal/flow"
	"gestalt/internal/logging"
	"gestalt/internal/metrics"
	"gestalt/internal/notify"
	"gestalt/internal/terminal"
)

const (
	sendToTerminalActivityName    = "SendToTerminalActivity"
	postWebhookActivityName       = "PostWebhookActivity"
	publishToastActivityName      = "PublishToastActivity"
	defaultWebhookTimeout         = 10 * time.Second
	defaultOutputTailLines        = 50
)

// Dispatcher executes flow activities locally.
type Dispatcher struct {
	Manager          *terminal.Manager
	Logger           *logging.Logger
	NotificationSink notify.Sink
	MaxOutputBytes   int64
}

func NewDispatcher(manager *terminal.Manager, logger *logging.Logger, sink notify.Sink, maxOutputBytes int64) *Dispatcher {
	if maxOutputBytes < 0 {
		maxOutputBytes = 0
	}
	if sink == nil {
		sink = notify.NewOTelSink(nil)
	}
	return &Dispatcher{
		Manager:          manager,
		Logger:           logger,
		NotificationSink: sink,
		MaxOutputBytes:   maxOutputBytes,
	}
}

func (d *Dispatcher) Dispatch(ctx context.Context, request flow.ActivityRequest) error {
	switch request.ActivityID {
	case "send_to_terminal":
		return d.sendToTerminal(ctx, request)
	case "post_webhook":
		return d.postWebhook(ctx, request)
	case "toast_notification":
		return d.publishToast(ctx, request)
	default:
		d.logWarn("unknown flow activity", map[string]string{
			"activity_id": request.ActivityID,
		})
		return nil
	}
}

func (d *Dispatcher) sendToTerminal(ctx context.Context, request flow.ActivityRequest) (activityErr error) {
	start := time.Now()
	defer func() {
		metrics.Default.RecordActivity(sendToTerminalActivityName, time.Since(start), activityErr, 1)
	}()

	if ctx != nil {
		if contextError := ctx.Err(); contextError != nil {
			activityErr = contextError
			return contextError
		}
	}
	manager, managerErr := d.ensureManager()
	if managerErr != nil {
		activityErr = managerErr
		return managerErr
	}

	targetSessionID := strings.TrimSpace(configString(request.Config, "target_session_id"))
	targetName := strings.TrimSpace(configString(request.Config, "target_agent_name"))
	if targetSessionID == "" && targetName == "" {
		activityErr = errors.New("target session id or agent name is required")
		return activityErr
	}

	messageTemplate := flow.RenderTemplate(configString(request.Config, "message_template"), request)
	if request.OutputTail == "" && !strings.EqualFold(targetSessionID, terminal.ChatSessionID) {
		request.OutputTail = d.buildOutputTail(request)
	}
	message := buildMessage(messageTemplate, request.OutputTail)
	if strings.TrimSpace(message) == "" {
		activityErr = errors.New("message is required")
		return activityErr
	}

	if strings.EqualFold(targetSessionID, terminal.ChatSessionID) {
		if !manager.PublishChatMessage(message, "flow", "user") {
			activityErr = errors.New("failed to publish chat message")
			return activityErr
		}
		d.logInfo("flow chat message sent", map[string]string{
			"session_id": terminal.ChatSessionID,
		})
		return nil
	}

	var session *terminal.Session
	if targetSessionID != "" {
		session, activityErr = lookupSession(manager, targetSessionID)
	} else {
		session, activityErr = lookupAgentSession(manager, targetName)
	}
	if activityErr != nil {
		return activityErr
	}

	if !strings.HasSuffix(message, "\n") {
		message += "\n"
	}
	if writeErr := session.Write([]byte(message)); writeErr != nil {
		d.logWarn("flow terminal send failed", map[string]string{
			"agent_name": targetName,
			"session_id": targetSessionID,
			"error":      writeErr.Error(),
		})
		activityErr = writeErr
		return writeErr
	}

	d.logInfo("flow terminal message sent", map[string]string{
		"agent_name": targetName,
		"session_id": targetSessionID,
	})
	return nil
}

func (d *Dispatcher) postWebhook(ctx context.Context, request flow.ActivityRequest) (activityErr error) {
	start := time.Now()
	defer func() {
		metrics.Default.RecordActivity(postWebhookActivityName, time.Since(start), activityErr, 1)
	}()

	if ctx != nil {
		if contextError := ctx.Err(); contextError != nil {
			activityErr = contextError
			return contextError
		}
	}

	urlValue := configString(request.Config, "url")
	if strings.TrimSpace(urlValue) == "" {
		activityErr = errors.New("webhook url is required")
		return activityErr
	}

	headers, headersErr := parseHeaders(configString(request.Config, "headers_json"))
	if headersErr != nil {
		activityErr = headersErr
		return headersErr
	}

	bodyTemplate := flow.RenderTemplate(configString(request.Config, "body_template"), request)
	body, defaultContentType, bodyErr := buildWebhookBody(request, bodyTemplate)
	if bodyErr != nil {
		activityErr = bodyErr
		return bodyErr
	}
	if headers.Get("Content-Type") == "" && defaultContentType != "" {
		headers.Set("Content-Type", defaultContentType)
	}

	idempotencyKey := flow.BuildIdempotencyKey(request.EventID, request.TriggerID, request.ActivityID)
	if idempotencyKey != "" && headers.Get("Idempotency-Key") == "" {
		headers.Set("Idempotency-Key", idempotencyKey)
	}

	httpClient := &http.Client{
		Timeout: defaultWebhookTimeout,
	}
	httpRequest, requestErr := http.NewRequestWithContext(ctx, http.MethodPost, urlValue, bytes.NewReader(body))
	if requestErr != nil {
		activityErr = requestErr
		return requestErr
	}
	httpRequest.Header = headers

	response, responseErr := httpClient.Do(httpRequest)
	if responseErr != nil {
		activityErr = responseErr
		return responseErr
	}
	defer func() {
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
	}()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		activityErr = fmt.Errorf("webhook returned %d", response.StatusCode)
		return activityErr
	}

	d.logInfo("flow webhook posted", map[string]string{
		"status": strconv.Itoa(response.StatusCode),
	})
	return nil
}

func (d *Dispatcher) publishToast(ctx context.Context, request flow.ActivityRequest) (activityErr error) {
	start := time.Now()
	defer func() {
		metrics.Default.RecordActivity(publishToastActivityName, time.Since(start), activityErr, 1)
	}()

	if ctx != nil {
		if contextError := ctx.Err(); contextError != nil {
			activityErr = contextError
			return contextError
		}
	}

	level := strings.TrimSpace(configString(request.Config, "level"))
	if level == "" {
		activityErr = errors.New("toast level is required")
		return activityErr
	}
	if !isToastLevel(level) {
		activityErr = fmt.Errorf("unsupported toast level %q", level)
		return activityErr
	}

	message := strings.TrimSpace(flow.RenderTemplate(configString(request.Config, "message_template"), request))
	if message == "" {
		activityErr = errors.New("toast message is required")
		return activityErr
	}

	if d.NotificationSink == nil {
		activityErr = errors.New("notification sink unavailable")
		return activityErr
	}
	occurredAt := time.Now().UTC()
	fields := map[string]string{
		"notify.type":  "toast",
		"type":         flow.CanonicalNotifyEventType("toast"),
		"notify.level": level,
	}
	event := notify.Event{
		Fields:     fields,
		OccurredAt: occurredAt,
		Level:      level,
		Message:    message,
	}
	if err := d.NotificationSink.Emit(ctx, event); err != nil {
		activityErr = err
		return err
	}
	d.logInfo("flow toast published", map[string]string{
		"level": level,
	})
	return nil
}

func (d *Dispatcher) buildOutputTail(request flow.ActivityRequest) string {
	if !configBoolDefault(request.Config, "include_terminal_output", false) {
		return ""
	}
	sessionID := strings.TrimSpace(request.Event["session_id"])
	if sessionID == "" {
		sessionID = strings.TrimSpace(request.Event["terminal_id"])
	}
	if sessionID == "" {
		return ""
	}
	lines := configInt(request.Config, "output_tail_lines", defaultOutputTailLines)
	if lines <= 0 {
		return ""
	}
	manager, managerErr := d.ensureManager()
	if managerErr != nil {
		return ""
	}
	maxLines := lines + 1
	history, err := manager.HistoryLines(sessionID, maxLines)
	if err != nil {
		d.logWarn("flow output tail failed", map[string]string{
			"error":      err.Error(),
			"session_id": sessionID,
		})
		return ""
	}
	for len(history) > 0 && strings.TrimSpace(history[len(history)-1]) == "" {
		history = history[:len(history)-1]
	}
	if len(history) > lines {
		history = history[len(history)-lines:]
	}
	output := strings.Join(history, "\n")
	return d.capOutput(output)
}

func (d *Dispatcher) ensureManager() (*terminal.Manager, error) {
	if d == nil || d.Manager == nil {
		return nil, errors.New("terminal manager unavailable")
	}
	return d.Manager, nil
}

func (d *Dispatcher) capOutput(value string) string {
	if d == nil || d.MaxOutputBytes <= 0 {
		return value
	}
	if int64(len(value)) <= d.MaxOutputBytes {
		return value
	}
	return string([]byte(value)[:d.MaxOutputBytes])
}

func (d *Dispatcher) logInfo(message string, fields map[string]string) {
	if d == nil || d.Logger == nil {
		return
	}
	d.Logger.Info(message, fields)
}

func (d *Dispatcher) logWarn(message string, fields map[string]string) {
	if d == nil || d.Logger == nil {
		return
	}
	d.Logger.Warn(message, fields)
}

func parseHeaders(raw string) (http.Header, error) {
	headers := http.Header{}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return headers, nil
	}
	decoded := map[string]string{}
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return nil, err
	}
	for key, value := range decoded {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		headers.Set(key, value)
	}
	return headers, nil
}

func buildWebhookBody(request flow.ActivityRequest, bodyTemplate string) ([]byte, string, error) {
	if strings.TrimSpace(bodyTemplate) == "" {
		payload := map[string]any{
			"event_id":    request.EventID,
			"trigger_id":  request.TriggerID,
			"activity_id": request.ActivityID,
			"event":       request.Event,
		}
		data, err := json.Marshal(payload)
		return data, "application/json", err
	}
	return []byte(bodyTemplate), "text/plain; charset=utf-8", nil
}

func buildMessage(template string, outputTail string) string {
	message := template
	if outputTail != "" {
		if strings.TrimSpace(message) != "" {
			message += "\n\n" + outputTail
		} else {
			message = outputTail
		}
	}
	return message
}

func lookupAgentSession(manager *terminal.Manager, agentName string) (*terminal.Session, error) {
	if manager == nil {
		return nil, errors.New("terminal manager unavailable")
	}
	name := strings.TrimSpace(agentName)
	if name == "" {
		return nil, errors.New("target agent name is required")
	}
	if session, ok := manager.GetSessionByAgent(name); ok {
		return session, nil
	}
	for _, info := range manager.ListAgents() {
		if strings.EqualFold(info.Name, name) {
			if session, ok := manager.GetSessionByAgent(info.Name); ok {
				return session, nil
			}
		}
	}
	if terminalID, ok := manager.GetAgentTerminal(name); ok {
		if session, ok := manager.Get(terminalID); ok {
			return session, nil
		}
	}
	for _, info := range manager.List() {
		if strings.EqualFold(info.Role, name) {
			if session, ok := manager.Get(info.ID); ok {
				return session, nil
			}
		}
	}
	return nil, terminal.ErrSessionNotFound
}

func lookupSession(manager *terminal.Manager, sessionID string) (*terminal.Session, error) {
	if manager == nil {
		return nil, errors.New("terminal manager unavailable")
	}
	id := strings.TrimSpace(sessionID)
	if id == "" {
		return nil, errors.New("target session id is required")
	}
	if session, ok := manager.Get(id); ok {
		return session, nil
	}
	return nil, terminal.ErrSessionNotFound
}

func writeSessionMessage(session *terminal.Session, message string) error {
	if session == nil {
		return errors.New("session unavailable")
	}
	if !strings.HasSuffix(message, "\n") {
		message += "\n"
	}
	return session.Write([]byte(message))
}

func configString(config map[string]any, key string) string {
	if config == nil {
		return ""
	}
	value, ok := config[key]
	if !ok || value == nil {
		return ""
	}
	parsed, ok := value.(string)
	if !ok {
		return ""
	}
	return parsed
}

func configBoolDefault(config map[string]any, key string, fallback bool) bool {
	if config == nil {
		return fallback
	}
	value, ok := config[key]
	if !ok || value == nil {
		return fallback
	}
	parsed, ok := value.(bool)
	if !ok {
		return fallback
	}
	return parsed
}

func configInt(config map[string]any, key string, fallback int) int {
	if config == nil {
		return fallback
	}
	value, ok := config[key]
	if !ok || value == nil {
		return fallback
	}
	switch parsed := value.(type) {
	case int:
		return parsed
	case int32:
		return int(parsed)
	case int64:
		return int(parsed)
	case float32:
		return int(parsed)
	case float64:
		return int(parsed)
	default:
		return fallback
	}
}

func isToastLevel(level string) bool {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "info", "warning", "error":
		return true
	default:
		return false
	}
}
