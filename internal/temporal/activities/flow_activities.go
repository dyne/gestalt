package activities

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
	"gestalt/internal/notification"
	"gestalt/internal/terminal"

	"go.temporal.io/sdk/activity"
)

const (
	SendToTerminalActivityName = "SendToTerminalActivity"
	PostWebhookActivityName    = "PostWebhookActivity"
	PublishToastActivityName   = "PublishToastActivity"
)

const defaultWebhookTimeout = 10 * time.Second

type FlowActivities struct {
	Manager *terminal.Manager
	Logger  *logging.Logger
}

func NewFlowActivities(manager *terminal.Manager, logger *logging.Logger) *FlowActivities {
	return &FlowActivities{
		Manager: manager,
		Logger:  logger,
	}
}

func (activities *FlowActivities) SendToTerminalActivity(activityContext context.Context, request flow.ActivityRequest) (activityErr error) {
	start := time.Now()
	attempt := activityAttempt(activityContext)
	defer func() {
		metrics.Default.RecordActivity(SendToTerminalActivityName, time.Since(start), activityErr, attempt)
	}()

	if activityContext != nil {
		if contextError := activityContext.Err(); contextError != nil {
			activityErr = contextError
			return contextError
		}
	}
	manager, managerError := activities.ensureManager()
	if managerError != nil {
		activityErr = managerError
		return managerError
	}

	if heartbeat, ok := heartbeatDetails(activityContext); ok && flow.ShouldSkipSend(&heartbeat) {
		return nil
	}

	targetSessionID := strings.TrimSpace(configString(request.Config, "target_session_id"))
	targetName := strings.TrimSpace(configString(request.Config, "target_agent_name"))
	if targetSessionID == "" && targetName == "" {
		activityErr = errors.New("target session id or agent name is required")
		return activityErr
	}

	message := buildMessage(configString(request.Config, "message_template"), request.OutputTail)
	if strings.TrimSpace(message) == "" {
		activityErr = errors.New("message is required")
		return activityErr
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
		activities.logWarn("flow terminal send failed", map[string]string{
			"agent_name": targetName,
			"session_id": targetSessionID,
			"error":      writeErr.Error(),
		})
		activityErr = writeErr
		return writeErr
	}

	recordHeartbeat(activityContext, flow.ActivityHeartbeat{Sent: true})
	activities.logInfo("flow terminal message sent", map[string]string{
		"agent_name": targetName,
		"session_id": targetSessionID,
	})
	return nil
}

func (activities *FlowActivities) PostWebhookActivity(activityContext context.Context, request flow.ActivityRequest) (activityErr error) {
	start := time.Now()
	attempt := activityAttempt(activityContext)
	defer func() {
		metrics.Default.RecordActivity(PostWebhookActivityName, time.Since(start), activityErr, attempt)
	}()

	if activityContext != nil {
		if contextError := activityContext.Err(); contextError != nil {
			activityErr = contextError
			return contextError
		}
	}

	if heartbeat, ok := heartbeatDetails(activityContext); ok && flow.ShouldSkipWebhook(&heartbeat) {
		return nil
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

	bodyTemplate := configString(request.Config, "body_template")
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
	httpRequest, requestErr := http.NewRequestWithContext(activityContext, http.MethodPost, urlValue, bytes.NewReader(body))
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

	recordHeartbeat(activityContext, flow.ActivityHeartbeat{Posted: true, StatusCode: response.StatusCode})
	activities.logInfo("flow webhook posted", map[string]string{
		"status": strconv.Itoa(response.StatusCode),
	})
	return nil
}

func (activities *FlowActivities) PublishToastActivity(activityContext context.Context, request flow.ActivityRequest) (activityErr error) {
	start := time.Now()
	attempt := activityAttempt(activityContext)
	defer func() {
		metrics.Default.RecordActivity(PublishToastActivityName, time.Since(start), activityErr, attempt)
	}()

	if activityContext != nil {
		if contextError := activityContext.Err(); contextError != nil {
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

	message := strings.TrimSpace(configString(request.Config, "message_template"))
	if message == "" {
		activityErr = errors.New("toast message is required")
		return activityErr
	}

	notification.PublishToast(level, message)
	activities.logInfo("flow toast published", map[string]string{
		"level": level,
	})
	return nil
}

func (activities *FlowActivities) ensureManager() (*terminal.Manager, error) {
	if activities == nil || activities.Manager == nil {
		return nil, errors.New("terminal manager unavailable")
	}
	return activities.Manager, nil
}

func (activities *FlowActivities) logInfo(message string, fields map[string]string) {
	if activities == nil || activities.Logger == nil {
		return
	}
	activities.Logger.Info(message, fields)
}

func (activities *FlowActivities) logWarn(message string, fields map[string]string) {
	if activities == nil || activities.Logger == nil {
		return
	}
	activities.Logger.Warn(message, fields)
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

func isToastLevel(level string) bool {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "info", "warning", "error":
		return true
	default:
		return false
	}
}

func heartbeatDetails(activityContext context.Context) (flow.ActivityHeartbeat, bool) {
	if activityContext == nil || !activity.IsActivity(activityContext) {
		return flow.ActivityHeartbeat{}, false
	}
	var heartbeat flow.ActivityHeartbeat
	if err := activity.GetHeartbeatDetails(activityContext, &heartbeat); err != nil {
		return flow.ActivityHeartbeat{}, false
	}
	return heartbeat, true
}

func recordHeartbeat(activityContext context.Context, heartbeat flow.ActivityHeartbeat) {
	if activityContext == nil || !activity.IsActivity(activityContext) {
		return
	}
	activity.RecordHeartbeat(activityContext, heartbeat)
}
