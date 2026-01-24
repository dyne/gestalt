package temporal

import (
	"context"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/converter"
	"go.temporal.io/sdk/interceptor"
)

type WorkflowClient interface {
	ExecuteWorkflow(ctx context.Context, options client.StartWorkflowOptions, workflow interface{}, args ...interface{}) (client.WorkflowRun, error)
	GetWorkflowHistory(ctx context.Context, workflowID string, runID string, isLongPoll bool, filterType enumspb.HistoryEventFilterType) client.HistoryEventIterator
	QueryWorkflow(ctx context.Context, workflowID, runID, queryType string, args ...interface{}) (converter.EncodedValue, error)
	SignalWorkflow(ctx context.Context, workflowID, runID, signalName string, arg interface{}) error
	Close()
}

type ClientConfig struct {
	HostPort  string
	Namespace string
}

func NewClient(config ClientConfig) (WorkflowClient, error) {
	options := client.Options{
		HostPort:  config.HostPort,
		Namespace: config.Namespace,
	}
	options.Interceptors = []interceptor.ClientInterceptor{temporalTracingInterceptor()}
	return client.Dial(options)
}
