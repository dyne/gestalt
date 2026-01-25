package activities

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"gestalt/internal/event"
	"gestalt/internal/logging"
	"gestalt/internal/metrics"
	"gestalt/internal/terminal"

	"go.temporal.io/sdk/activity"
)

const (
	SpawnTerminalActivityName     = "SpawnTerminalActivity"
	TerminateTerminalActivityName = "TerminateTerminalActivity"
	RecordBellActivityName        = "RecordBellActivity"
	UpdateTaskActivityName        = "UpdateTaskActivity"
	GetOutputActivityName         = "GetOutputActivity"
	EmitWorkflowEventActivityName = "EmitWorkflowEventActivity"
)

type SessionActivities struct {
	Manager *terminal.Manager
	Logger  *logging.Logger
}

func NewSessionActivities(manager *terminal.Manager, logger *logging.Logger) *SessionActivities {
	return &SessionActivities{
		Manager: manager,
		Logger:  logger,
	}
}

func (activities *SessionActivities) SpawnTerminalActivity(activityContext context.Context, sessionID, shell string) (activityErr error) {
	start := time.Now()
	attempt := activityAttempt(activityContext)
	defer func() {
		metrics.Default.RecordActivity(SpawnTerminalActivityName, time.Since(start), activityErr, attempt)
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
	trimmedID := strings.TrimSpace(sessionID)
	if trimmedID == "" {
		activityErr = errors.New("session id is required")
		return activityErr
	}
	if _, ok := manager.Get(trimmedID); ok {
		activities.logInfo("temporal terminal already exists", map[string]string{
			"terminal_id": trimmedID,
		})
		return nil
	}
	createdSession, createError := manager.CreateWithID(trimmedID, "", "", "", shell)
	if createError != nil {
		activities.logWarn("temporal terminal create failed", map[string]string{
			"terminal_id": trimmedID,
			"error":       createError.Error(),
		})
		activityErr = createError
		return createError
	}
	if createdSession != nil {
		activities.logInfo("temporal terminal created", map[string]string{
			"terminal_id": createdSession.ID,
		})
	}
	return nil
}

func (activities *SessionActivities) TerminateTerminalActivity(activityContext context.Context, sessionID string) (activityErr error) {
	start := time.Now()
	attempt := activityAttempt(activityContext)
	defer func() {
		metrics.Default.RecordActivity(TerminateTerminalActivityName, time.Since(start), activityErr, attempt)
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
	trimmedID := strings.TrimSpace(sessionID)
	if trimmedID == "" {
		activityErr = errors.New("session id is required")
		return activityErr
	}
	deleteError := manager.Delete(trimmedID)
	if deleteError != nil {
		if errors.Is(deleteError, terminal.ErrSessionNotFound) {
			activities.logInfo("temporal terminal already deleted", map[string]string{
				"terminal_id": trimmedID,
			})
			return nil
		}
		activities.logWarn("temporal terminal delete failed", map[string]string{
			"terminal_id": trimmedID,
			"error":       deleteError.Error(),
		})
		activityErr = deleteError
		return deleteError
	}
	activities.logInfo("temporal terminal deleted", map[string]string{
		"terminal_id": trimmedID,
	})
	return nil
}

func (activities *SessionActivities) RecordBellActivity(activityContext context.Context, sessionID string, timestamp time.Time, contextText string) (activityErr error) {
	start := time.Now()
	attempt := activityAttempt(activityContext)
	defer func() {
		metrics.Default.RecordActivity(RecordBellActivityName, time.Since(start), activityErr, attempt)
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
	trimmedID := strings.TrimSpace(sessionID)
	if trimmedID == "" {
		activityErr = errors.New("session id is required")
		return activityErr
	}
	if _, ok := manager.Get(trimmedID); !ok {
		activityErr = terminal.ErrSessionNotFound
		return activityErr
	}
	contextText = terminal.FilterTerminalOutput(contextText)
	activities.logInfo("temporal bell recorded", map[string]string{
		"terminal_id": trimmedID,
		"timestamp":   timestamp.UTC().Format(time.RFC3339),
		"context":     contextText,
	})
	return nil
}

func (activities *SessionActivities) UpdateTaskActivity(activityContext context.Context, sessionID, l1Task, l2Task string) (activityErr error) {
	start := time.Now()
	attempt := activityAttempt(activityContext)
	defer func() {
		metrics.Default.RecordActivity(UpdateTaskActivityName, time.Since(start), activityErr, attempt)
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
	trimmedID := strings.TrimSpace(sessionID)
	if trimmedID == "" {
		activityErr = errors.New("session id is required")
		return activityErr
	}
	if _, ok := manager.Get(trimmedID); !ok {
		activityErr = terminal.ErrSessionNotFound
		return activityErr
	}
	activities.logInfo("temporal task update recorded", map[string]string{
		"terminal_id": trimmedID,
		"l1_task":     l1Task,
		"l2_task":     l2Task,
	})
	return nil
}

func (activities *SessionActivities) EmitWorkflowEventActivity(activityContext context.Context, payload event.WorkflowEvent) (activityErr error) {
	start := time.Now()
	attempt := activityAttempt(activityContext)
	defer func() {
		metrics.Default.RecordActivity(EmitWorkflowEventActivityName, time.Since(start), activityErr, attempt)
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
	if strings.TrimSpace(payload.EventType) == "" {
		activityErr = errors.New("workflow event type is required")
		return activityErr
	}
	if strings.TrimSpace(payload.WorkflowID) == "" {
		activityErr = errors.New("workflow id is required")
		return activityErr
	}
	if strings.TrimSpace(payload.SessionID) == "" {
		activityErr = errors.New("session id is required")
		return activityErr
	}

	bus := manager.WorkflowBus()
	if bus == nil {
		activityErr = errors.New("workflow event bus unavailable")
		return activityErr
	}

	if payload.OccurredAt.IsZero() {
		payload.OccurredAt = time.Now().UTC()
	}
	bus.Publish(payload)
	return nil
}

func (activities *SessionActivities) GetOutputActivity(activityContext context.Context, sessionID string) (output string, activityErr error) {
	start := time.Now()
	attempt := activityAttempt(activityContext)
	defer func() {
		metrics.Default.RecordActivity(GetOutputActivityName, time.Since(start), activityErr, attempt)
	}()

	if activityContext != nil {
		if contextError := activityContext.Err(); contextError != nil {
			activityErr = contextError
			return "", contextError
		}
	}
	manager, managerError := activities.ensureManager()
	if managerError != nil {
		activityErr = managerError
		return "", managerError
	}
	trimmedID := strings.TrimSpace(sessionID)
	if trimmedID == "" {
		activityErr = errors.New("session id is required")
		return "", activityErr
	}
	lines, historyError := manager.HistoryLines(trimmedID, terminal.DefaultHistoryLines)
	if historyError != nil {
		activityErr = historyError
		return "", historyError
	}
	output = strings.Join(lines, "\n")
	activities.logInfo("temporal output retrieved", map[string]string{
		"terminal_id": trimmedID,
		"line_count":  strconv.Itoa(len(lines)),
	})
	return output, nil
}

func (activities *SessionActivities) ensureManager() (*terminal.Manager, error) {
	if activities == nil || activities.Manager == nil {
		return nil, errors.New("terminal manager unavailable")
	}
	return activities.Manager, nil
}

func (activities *SessionActivities) logInfo(message string, fields map[string]string) {
	if activities == nil || activities.Logger == nil {
		return
	}
	activities.Logger.Info(message, fields)
}

func (activities *SessionActivities) logWarn(message string, fields map[string]string) {
	if activities == nil || activities.Logger == nil {
		return
	}
	activities.Logger.Warn(message, fields)
}

func activityAttempt(activityContext context.Context) int32 {
	if activityContext == nil || !activity.IsActivity(activityContext) {
		return 1
	}
	info := activity.GetInfo(activityContext)
	if info.Attempt <= 0 {
		return 1
	}
	return info.Attempt
}
