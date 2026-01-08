package temporal

import (
	"context"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/converter"
)

type WorkflowClient interface {
	ExecuteWorkflow(ctx context.Context, options client.StartWorkflowOptions, workflow interface{}, args ...interface{}) (client.WorkflowRun, error)
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
	return client.Dial(options)
}
