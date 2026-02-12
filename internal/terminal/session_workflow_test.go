package terminal

import (
	"context"
	"strings"
	"testing"
	"time"

	"gestalt/internal/agent"
	"gestalt/internal/otel"
	"gestalt/internal/temporal"
	"gestalt/internal/temporal/workflows"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/converter"
)

type fakeWorkflowRun struct {
	workflowID string
	runID      string
}

func (run *fakeWorkflowRun) GetID() string {
	return run.workflowID
}

func (run *fakeWorkflowRun) GetRunID() string {
	return run.runID
}

func (run *fakeWorkflowRun) Get(ctx context.Context, valuePtr interface{}) error {
	return nil
}

func (run *fakeWorkflowRun) GetWithOptions(ctx context.Context, valuePtr interface{}, options client.WorkflowRunGetOptions) error {
	return nil
}

type signalRecord struct {
	workflowID string
	runID      string
	signalName string
	payload    interface{}
}

type fakeWorkflowClient struct {
	executeCalls int
	startOptions client.StartWorkflowOptions
	lastRequest  workflows.SessionWorkflowRequest
	runID        string
	signals      []signalRecord
}

func (client *fakeWorkflowClient) ExecuteWorkflow(ctx context.Context, options client.StartWorkflowOptions, workflow interface{}, args ...interface{}) (client.WorkflowRun, error) {
	client.executeCalls++
	client.startOptions = options
	if len(args) > 0 {
		if request, ok := args[0].(workflows.SessionWorkflowRequest); ok {
			client.lastRequest = request
		}
	}
	return &fakeWorkflowRun{workflowID: options.ID, runID: client.runID}, nil
}

func (client *fakeWorkflowClient) SignalWithStartWorkflow(ctx context.Context, workflowID, signalName string, signalArg interface{}, options client.StartWorkflowOptions, workflow interface{}, args ...interface{}) (client.WorkflowRun, error) {
	return &fakeWorkflowRun{workflowID: workflowID, runID: client.runID}, nil
}

func (client *fakeWorkflowClient) SignalWorkflow(ctx context.Context, workflowID, runID, signalName string, arg interface{}) error {
	client.signals = append(client.signals, signalRecord{
		workflowID: workflowID,
		runID:      runID,
		signalName: signalName,
		payload:    arg,
	})
	return nil
}

func (client *fakeWorkflowClient) QueryWorkflow(ctx context.Context, workflowID, runID, queryType string, args ...interface{}) (converter.EncodedValue, error) {
	return nil, nil
}

func (client *fakeWorkflowClient) GetWorkflowHistory(ctx context.Context, workflowID, runID string, isLongPoll bool, filterType enumspb.HistoryEventFilterType) client.HistoryEventIterator {
	return nil
}

func (client *fakeWorkflowClient) Close() {
}

var _ temporal.WorkflowClient = (*fakeWorkflowClient)(nil)

func TestSessionStartWorkflowAndSignals(testingContext *testing.T) {
	workflowClient := &fakeWorkflowClient{runID: "run-123"}
	pty := newScriptedPty()
	profile := &agent.Agent{
		Name:    "Codex",
		CLIType: "codex",
		CLIConfig: map[string]interface{}{
			"model": "o3",
		},
		Shell: "codex -c model=o3",
	}
	profile.ConfigHash = agent.ComputeConfigHash(profile)
	session := newSession("7", pty, nil, nil, "title", "role", time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), 10, 0, OutputBackpressureBlock, 0, profile, nil, nil)
	session.Command = "codex -c model=o3"
	session.ConfigHash = profile.ConfigHash
	session.AgentID = "codex"

	startError := session.StartWorkflow(workflowClient, "L1", "L2")
	if startError != nil {
		testingContext.Fatalf("start workflow error: %v", startError)
	}
	if workflowClient.executeCalls != 1 {
		testingContext.Fatalf("expected 1 workflow start, got %d", workflowClient.executeCalls)
	}
	if session.WorkflowID == nil || *session.WorkflowID != "7" {
		testingContext.Fatalf("unexpected workflow id: %v", session.WorkflowID)
	}
	if session.WorkflowRunID == nil || *session.WorkflowRunID != "run-123" {
		testingContext.Fatalf("unexpected workflow run id: %v", session.WorkflowRunID)
	}
	if workflowClient.lastRequest.L1Task != "L1" || workflowClient.lastRequest.L2Task != "L2" {
		testingContext.Fatalf("unexpected request tasks: %+v", workflowClient.lastRequest)
	}
	if workflowClient.lastRequest.AgentID != "codex" {
		testingContext.Fatalf("unexpected request agent id: %q", workflowClient.lastRequest.AgentID)
	}
	if workflowClient.lastRequest.AgentName != "Codex" {
		testingContext.Fatalf("unexpected request agent name: %q", workflowClient.lastRequest.AgentName)
	}
	if workflowClient.lastRequest.Shell != session.Command {
		testingContext.Fatalf("unexpected request shell: %q", workflowClient.lastRequest.Shell)
	}
	if workflowClient.lastRequest.ConfigHash != profile.ConfigHash {
		testingContext.Fatalf("unexpected request config hash: %q", workflowClient.lastRequest.ConfigHash)
	}
	if workflowClient.lastRequest.AgentConfig == "" || !strings.Contains(workflowClient.lastRequest.AgentConfig, "name = \"Codex\"") {
		testingContext.Fatalf("unexpected request agent config: %q", workflowClient.lastRequest.AgentConfig)
	}
	if !workflowClient.lastRequest.StartTime.Equal(session.CreatedAt) {
		testingContext.Fatalf("unexpected request start time: %v", workflowClient.lastRequest.StartTime)
	}
	if workflowClient.startOptions.WorkflowExecutionTimeout != temporalWorkflowExecutionTimeout {
		testingContext.Fatalf("unexpected workflow execution timeout: %v", workflowClient.startOptions.WorkflowExecutionTimeout)
	}
	if workflowClient.startOptions.WorkflowRunTimeout != temporalWorkflowRunTimeout {
		testingContext.Fatalf("unexpected workflow run timeout: %v", workflowClient.startOptions.WorkflowRunTimeout)
	}
	if workflowClient.startOptions.WorkflowTaskTimeout != temporalWorkflowTaskTimeout {
		testingContext.Fatalf("unexpected workflow task timeout: %v", workflowClient.startOptions.WorkflowTaskTimeout)
	}
	if workflowClient.startOptions.RetryPolicy == nil || workflowClient.startOptions.RetryPolicy.MaximumAttempts != 5 {
		testingContext.Fatalf("unexpected workflow retry policy: %#v", workflowClient.startOptions.RetryPolicy)
	}
	memo := workflowClient.startOptions.Memo
	if memo == nil {
		testingContext.Fatalf("expected workflow memo")
	}
	if memo["config_hash"] != profile.ConfigHash {
		testingContext.Fatalf("unexpected memo config hash: %v", memo["config_hash"])
	}
	if memo["cli_type"] != "codex" {
		testingContext.Fatalf("unexpected memo cli_type: %v", memo["cli_type"])
	}
	if memo["agent_id"] != "codex" {
		testingContext.Fatalf("unexpected memo agent_id: %v", memo["agent_id"])
	}
	if memo["agent_name"] != "Codex" {
		testingContext.Fatalf("unexpected memo agent_name: %v", memo["agent_name"])
	}
	if memo["agent_config"] == nil {
		testingContext.Fatalf("expected memo agent_config")
	}

	bellError := session.SendBellSignal("bell context")
	if bellError != nil {
		testingContext.Fatalf("bell signal error: %v", bellError)
	}
	updateError := session.UpdateTask("Next L1", "Next L2")
	if updateError != nil {
		testingContext.Fatalf("task update error: %v", updateError)
	}

	closeError := session.Close()
	if closeError != nil {
		testingContext.Fatalf("close error: %v", closeError)
	}

	if len(workflowClient.signals) != 3 {
		testingContext.Fatalf("expected 3 signals, got %d", len(workflowClient.signals))
	}

	bellSignal, ok := workflowClient.signals[0].payload.(workflows.BellSignal)
	if !ok || bellSignal.Context != "bell context" {
		testingContext.Fatalf("unexpected bell signal: %#v", workflowClient.signals[0].payload)
	}
	updateSignal, ok := workflowClient.signals[1].payload.(workflows.UpdateTaskSignal)
	if !ok || updateSignal.L1 != "Next L1" || updateSignal.L2 != "Next L2" {
		testingContext.Fatalf("unexpected update signal: %#v", workflowClient.signals[1].payload)
	}
	terminateSignal, ok := workflowClient.signals[2].payload.(workflows.TerminateSignal)
	if !ok || terminateSignal.Reason == "" {
		testingContext.Fatalf("unexpected terminate signal: %#v", workflowClient.signals[2].payload)
	}
}

func TestSessionStartWorkflowIncludesCollectorInfo(testingContext *testing.T) {
	collectorStart := time.Date(2025, 1, 2, 8, 0, 0, 0, time.UTC)
	collectorInfo := otel.CollectorInfo{
		StartTime:    collectorStart,
		ConfigPath:   "/tmp/collector.yaml",
		DataPath:     "/tmp/otel.json",
		GRPCEndpoint: "127.0.0.1:4317",
		HTTPEndpoint: "127.0.0.1:4318",
	}
	otel.SetActiveCollector(collectorInfo)
	defer otel.ClearActiveCollector()

	workflowClient := &fakeWorkflowClient{runID: "run-456"}
	pty := newScriptedPty()
	session := newSession("9", pty, nil, nil, "title", "role", time.Date(2025, 1, 2, 9, 0, 0, 0, time.UTC), 10, 0, OutputBackpressureBlock, 0, nil, nil, nil)
	session.Command = "/bin/bash"

	startError := session.StartWorkflow(workflowClient, "", "")
	if startError != nil {
		testingContext.Fatalf("start workflow error: %v", startError)
	}
	if !workflowClient.lastRequest.CollectorStartTime.Equal(collectorStart) {
		testingContext.Fatalf("unexpected collector start time: %v", workflowClient.lastRequest.CollectorStartTime)
	}
	if workflowClient.lastRequest.CollectorGRPCEndpoint != collectorInfo.GRPCEndpoint {
		testingContext.Fatalf("unexpected grpc endpoint: %q", workflowClient.lastRequest.CollectorGRPCEndpoint)
	}
	if workflowClient.lastRequest.CollectorHTTPEndpoint != collectorInfo.HTTPEndpoint {
		testingContext.Fatalf("unexpected http endpoint: %q", workflowClient.lastRequest.CollectorHTTPEndpoint)
	}
	if workflowClient.lastRequest.CollectorConfigPath != collectorInfo.ConfigPath {
		testingContext.Fatalf("unexpected config path: %q", workflowClient.lastRequest.CollectorConfigPath)
	}
	if workflowClient.lastRequest.CollectorDataPath != collectorInfo.DataPath {
		testingContext.Fatalf("unexpected data path: %q", workflowClient.lastRequest.CollectorDataPath)
	}

	memo := workflowClient.startOptions.Memo
	if memo["otel_grpc_endpoint"] != collectorInfo.GRPCEndpoint {
		testingContext.Fatalf("unexpected memo grpc endpoint: %v", memo["otel_grpc_endpoint"])
	}
	if memo["otel_http_endpoint"] != collectorInfo.HTTPEndpoint {
		testingContext.Fatalf("unexpected memo http endpoint: %v", memo["otel_http_endpoint"])
	}
	if memo["otel_config_path"] != collectorInfo.ConfigPath {
		testingContext.Fatalf("unexpected memo config path: %v", memo["otel_config_path"])
	}
	if memo["otel_data_path"] != collectorInfo.DataPath {
		testingContext.Fatalf("unexpected memo data path: %v", memo["otel_data_path"])
	}
	if memo["otel_started_at"] == nil {
		testingContext.Fatalf("expected memo otel_started_at")
	}
}
