package api

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"time"

	"gestalt/internal/logging"
	"gestalt/internal/temporal"
	"gestalt/internal/temporal/workflows"

	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/converter"
)

const workflowQueryTimeout = 3 * time.Second
const workflowStatusUnknown = "unknown"
const workflowHistoryTimeout = 5 * time.Second

func (h *RestHandler) handleWorkflows(w http.ResponseWriter, r *http.Request) *apiError {
	if err := h.requireManager(); err != nil {
		return err
	}
	if r.Method != http.MethodGet {
		return methodNotAllowed(w, "GET")
	}
	if !h.Manager.TemporalEnabled() || h.Manager.TemporalClient() == nil {
		writeJSON(w, http.StatusOK, []workflowSummary{})
		return nil
	}

	summaries := h.listWorkflowSummaries(r.Context())
	writeJSON(w, http.StatusOK, summaries)
	return nil
}

func (h *RestHandler) listWorkflowSummaries(ctx context.Context) []workflowSummary {
	temporalClient := h.Manager.TemporalClient()
	if temporalClient == nil {
		return []workflowSummary{}
	}

	infos := h.Manager.List()
	summaries := make([]workflowSummary, 0, len(infos))
	for _, info := range infos {
		session, ok := h.Manager.Get(info.ID)
		if !ok {
			continue
		}
		workflowID, workflowRunID, ok := session.WorkflowIdentifiers()
		if !ok {
			continue
		}

		summary := workflowSummary{
			SessionID:     info.ID,
			WorkflowID:    workflowID,
			WorkflowRunID: workflowRunID,
			Title:         info.Title,
			Role:          info.Role,
		}

		state, err := queryWorkflowState(ctx, temporalClient, workflowID, workflowRunID)
		if err != nil {
			var notFound *serviceerror.NotFound
			if errors.As(err, &notFound) {
				summary.Status = workflows.SessionStatusStopped
			} else {
				summary.Status = workflowStatusUnknown
			}
			summary.StartTime = info.CreatedAt
			if h.Logger != nil {
				h.Logger.Warn("workflow status query failed", map[string]string{
					"workflow_id": workflowID,
					"run_id":      workflowRunID,
					"error":       err.Error(),
				})
			}
		} else {
			summary.AgentID = state.AgentID
			summary.AgentName = state.AgentName
			if summary.AgentName == "" {
				summary.AgentName = state.AgentID
			}
			summary.CurrentL1 = state.CurrentL1
			summary.CurrentL2 = state.CurrentL2
			summary.Status = state.Status
			if summary.Status == "" {
				summary.Status = workflowStatusUnknown
			}
			summary.StartTime = state.StartTime
			if summary.StartTime.IsZero() {
				summary.StartTime = info.CreatedAt
			}
			if len(state.BellEvents) > 0 {
				summary.BellEvents = make([]workflowBellEvent, 0, len(state.BellEvents))
				for _, event := range state.BellEvents {
					summary.BellEvents = append(summary.BellEvents, workflowBellEvent{
						Timestamp: event.Timestamp,
						Context:   event.Context,
					})
				}
			}
			if len(state.TaskEvents) > 0 {
				summary.TaskEvents = make([]workflowTaskEvent, 0, len(state.TaskEvents))
				for _, event := range state.TaskEvents {
					summary.TaskEvents = append(summary.TaskEvents, workflowTaskEvent{
						Timestamp: event.Timestamp,
						L1:        event.L1,
						L2:        event.L2,
					})
				}
			}
		}
		summaries = append(summaries, summary)
	}

	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].StartTime.Equal(summaries[j].StartTime) {
			return summaries[i].SessionID < summaries[j].SessionID
		}
		return summaries[i].StartTime.After(summaries[j].StartTime)
	})

	return summaries
}

func queryWorkflowState(ctx context.Context, temporalClient temporal.WorkflowClient, workflowID, workflowRunID string) (workflows.SessionWorkflowState, error) {
	var state workflows.SessionWorkflowState
	if temporalClient == nil {
		return state, errors.New("temporal client unavailable")
	}
	if workflowID == "" {
		return state, errors.New("workflow id required")
	}

	queryContext, cancel := context.WithTimeout(ctx, workflowQueryTimeout)
	defer cancel()

	encodedValue, err := temporalClient.QueryWorkflow(queryContext, workflowID, workflowRunID, workflows.StatusQueryName)
	if err != nil {
		return state, err
	}
	if encodedValue == nil || !encodedValue.HasValue() {
		return state, errors.New("workflow status unavailable")
	}
	if err := encodedValue.Get(&state); err != nil {
		return state, err
	}
	return state, nil
}

func fetchWorkflowHistoryEntries(ctx context.Context, temporalClient temporal.WorkflowClient, workflowID, workflowRunID string, logger *logging.Logger) ([]workflowHistoryEntry, error) {
	if temporalClient == nil {
		return nil, errors.New("temporal client unavailable")
	}
	if workflowID == "" {
		return nil, errors.New("workflow id required")
	}

	historyContext, cancel := context.WithTimeout(ctx, workflowHistoryTimeout)
	defer cancel()

	iterator := temporalClient.GetWorkflowHistory(historyContext, workflowID, workflowRunID, false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)
	if iterator == nil {
		return nil, errors.New("workflow history unavailable")
	}

	entries := []workflowHistoryEntry{}
	dataConverter := converter.GetDefaultDataConverter()

	for iterator.HasNext() {
		event, err := iterator.Next()
		if err != nil {
			return nil, err
		}
		if event == nil {
			continue
		}

		eventTime := time.Time{}
		if timestamp := event.GetEventTime(); timestamp != nil {
			eventTime = timestamp.AsTime()
		}

		if event.GetEventType() != enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_SIGNALED {
			continue
		}

		attributes := event.GetWorkflowExecutionSignaledEventAttributes()
		if attributes == nil {
			continue
		}

		signalName := attributes.GetSignalName()
		entry := workflowHistoryEntry{
			EventID:    event.GetEventId(),
			Timestamp:  eventTime,
			SignalName: signalName,
		}

		switch signalName {
		case workflows.UpdateTaskSignalName:
			var payload workflows.UpdateTaskSignal
			if decodeSignalPayload(dataConverter, attributes.GetInput(), &payload, logger, signalName) {
				entry.Type = "task_update"
				entry.L1 = payload.L1
				entry.L2 = payload.L2
			} else {
				entry.Type = "signal"
			}
		case workflows.BellSignalName:
			var payload workflows.BellSignal
			if decodeSignalPayload(dataConverter, attributes.GetInput(), &payload, logger, signalName) {
				entry.Type = "bell"
				entry.Context = payload.Context
				if !payload.Timestamp.IsZero() {
					entry.Timestamp = payload.Timestamp
				}
			} else {
				entry.Type = "signal"
			}
		case workflows.NotifySignalName:
			var payload workflows.NotifySignal
			if decodeSignalPayload(dataConverter, attributes.GetInput(), &payload, logger, signalName) {
				entry.Type = "notify"
				entry.Context = payload.EventType
				if entry.Context == "" {
					entry.Context = payload.Source
				}
				if !payload.Timestamp.IsZero() {
					entry.Timestamp = payload.Timestamp
				}
			} else {
				entry.Type = "signal"
			}
		case workflows.ResumeSignalName:
			var payload workflows.ResumeSignal
			if decodeSignalPayload(dataConverter, attributes.GetInput(), &payload, logger, signalName) {
				entry.Type = "resume"
				entry.Action = payload.Action
			} else {
				entry.Type = "signal"
			}
		case workflows.TerminateSignalName:
			var payload workflows.TerminateSignal
			if decodeSignalPayload(dataConverter, attributes.GetInput(), &payload, logger, signalName) {
				entry.Type = "terminate"
				entry.Reason = payload.Reason
			} else {
				entry.Type = "signal"
			}
		default:
			entry.Type = "signal"
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

func decodeSignalPayload(dataConverter converter.DataConverter, payloads *commonpb.Payloads, destination interface{}, logger *logging.Logger, signalName string) bool {
	if payloads == nil {
		return false
	}
	if dataConverter == nil {
		dataConverter = converter.GetDefaultDataConverter()
	}
	if err := dataConverter.FromPayloads(payloads, destination); err != nil {
		if logger != nil {
			logger.Warn("failed to decode workflow signal", map[string]string{
				"signal": signalName,
				"error":  err.Error(),
			})
		}
		return false
	}
	return true
}
