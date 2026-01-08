package temporalworker

import (
	"errors"
	"sync"
	"time"

	"gestalt/internal/temporal"
	"gestalt/internal/temporal/activities"
	"gestalt/internal/temporal/workflows"
	"gestalt/internal/terminal"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

const defaultMaxConcurrentActivities = 10
const defaultMaxConcurrentWorkflowTasks = 10
const defaultWorkerStopTimeout = 5 * time.Second
const defaultDeadlockDetectionTimeout = 10 * time.Second

var workerMutex sync.Mutex
var activeWorker worker.Worker

func StartWorker(temporalClient temporal.WorkflowClient, manager *terminal.Manager) error {
	if temporalClient == nil {
		return errors.New("temporal client is required")
	}
	if manager == nil {
		return errors.New("terminal manager is required")
	}

	sdkClient, ok := temporalClient.(client.Client)
	if !ok {
		return errors.New("temporal client does not support worker")
	}

	workerMutex.Lock()
	if activeWorker != nil {
		workerMutex.Unlock()
		return errors.New("temporal worker already running")
	}
	workerMutex.Unlock()

	activityLogger := manager.Logger()
	activityHandlers := activities.NewSessionActivities(manager, activityLogger)

	workerOptions := worker.Options{
		MaxConcurrentActivityExecutionSize:     defaultMaxConcurrentActivities,
		MaxConcurrentWorkflowTaskExecutionSize: defaultMaxConcurrentWorkflowTasks,
		MaxConcurrentActivityTaskPollers:       2,
		MaxConcurrentWorkflowTaskPollers:       2,
		WorkerStopTimeout:                      defaultWorkerStopTimeout,
		DeadlockDetectionTimeout:               defaultDeadlockDetectionTimeout,
	}

	workerInstance := worker.New(sdkClient, workflows.SessionTaskQueueName, workerOptions)
	workerInstance.RegisterWorkflow(workflows.SessionWorkflow)
	workerInstance.RegisterActivity(activityHandlers)

	startError := workerInstance.Start()
	if startError != nil {
		return startError
	}

	workerMutex.Lock()
	activeWorker = workerInstance
	workerMutex.Unlock()

	if activityLogger != nil {
		activityLogger.Info("temporal worker started", map[string]string{
			"task_queue": workflows.SessionTaskQueueName,
		})
	}

	return nil
}

func StopWorker() {
	workerMutex.Lock()
	workerInstance := activeWorker
	activeWorker = nil
	workerMutex.Unlock()

	if workerInstance != nil {
		workerInstance.Stop()
	}
}
