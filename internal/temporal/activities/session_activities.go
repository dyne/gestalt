package activities

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"gestalt/internal/logging"
	"gestalt/internal/terminal"
)

const (
	SpawnTerminalActivityName     = "SpawnTerminalActivity"
	TerminateTerminalActivityName = "TerminateTerminalActivity"
	RecordBellActivityName        = "RecordBellActivity"
	UpdateTaskActivityName        = "UpdateTaskActivity"
	GetOutputActivityName         = "GetOutputActivity"
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

func (activities *SessionActivities) SpawnTerminalActivity(activityContext context.Context, sessionID, shell string) error {
	if activityContext != nil {
		if activityError := activityContext.Err(); activityError != nil {
			return activityError
		}
	}
	manager, managerError := activities.ensureManager()
	if managerError != nil {
		return managerError
	}
	trimmedID := strings.TrimSpace(sessionID)
	if trimmedID == "" {
		return errors.New("session id is required")
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
		return createError
	}
	if createdSession != nil {
		activities.logInfo("temporal terminal created", map[string]string{
			"terminal_id": createdSession.ID,
		})
	}
	return nil
}

func (activities *SessionActivities) TerminateTerminalActivity(activityContext context.Context, sessionID string) error {
	if activityContext != nil {
		if activityError := activityContext.Err(); activityError != nil {
			return activityError
		}
	}
	manager, managerError := activities.ensureManager()
	if managerError != nil {
		return managerError
	}
	trimmedID := strings.TrimSpace(sessionID)
	if trimmedID == "" {
		return errors.New("session id is required")
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
		return deleteError
	}
	activities.logInfo("temporal terminal deleted", map[string]string{
		"terminal_id": trimmedID,
	})
	return nil
}

func (activities *SessionActivities) RecordBellActivity(activityContext context.Context, sessionID string, timestamp time.Time, contextText string) error {
	if activityContext != nil {
		if activityError := activityContext.Err(); activityError != nil {
			return activityError
		}
	}
	manager, managerError := activities.ensureManager()
	if managerError != nil {
		return managerError
	}
	trimmedID := strings.TrimSpace(sessionID)
	if trimmedID == "" {
		return errors.New("session id is required")
	}
	if _, ok := manager.Get(trimmedID); !ok {
		return terminal.ErrSessionNotFound
	}
	activities.logInfo("temporal bell recorded", map[string]string{
		"terminal_id": trimmedID,
		"timestamp":   timestamp.UTC().Format(time.RFC3339),
		"context":     contextText,
	})
	return nil
}

func (activities *SessionActivities) UpdateTaskActivity(activityContext context.Context, sessionID, l1Task, l2Task string) error {
	if activityContext != nil {
		if activityError := activityContext.Err(); activityError != nil {
			return activityError
		}
	}
	manager, managerError := activities.ensureManager()
	if managerError != nil {
		return managerError
	}
	trimmedID := strings.TrimSpace(sessionID)
	if trimmedID == "" {
		return errors.New("session id is required")
	}
	if _, ok := manager.Get(trimmedID); !ok {
		return terminal.ErrSessionNotFound
	}
	activities.logInfo("temporal task update recorded", map[string]string{
		"terminal_id": trimmedID,
		"l1_task":     l1Task,
		"l2_task":     l2Task,
	})
	return nil
}

func (activities *SessionActivities) GetOutputActivity(activityContext context.Context, sessionID string) (string, error) {
	if activityContext != nil {
		if activityError := activityContext.Err(); activityError != nil {
			return "", activityError
		}
	}
	manager, managerError := activities.ensureManager()
	if managerError != nil {
		return "", managerError
	}
	trimmedID := strings.TrimSpace(sessionID)
	if trimmedID == "" {
		return "", errors.New("session id is required")
	}
	lines, historyError := manager.HistoryLines(trimmedID, terminal.DefaultHistoryLines)
	if historyError != nil {
		return "", historyError
	}
	output := strings.Join(lines, "\n")
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
