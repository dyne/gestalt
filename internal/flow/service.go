package flow

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"gestalt/internal/logging"
	"gestalt/internal/temporal"
	"gestalt/internal/temporal/workflows"

	"go.temporal.io/sdk/client"
)

const (
	RouterWorkflowID             = "gestalt-flow-router"
	RouterWorkflowType           = "FlowRouterWorkflow"
	RouterWorkflowConfigSignal   = "flow.config_updated"
	defaultSignalTimeout         = 5 * time.Second
	defaultConfigFilename        = "automations.json"
	defaultConfigDirectory       = "flow"
	defaultGestaltStateDirectory = ".gestalt"
)

var ErrTemporalUnavailable = errors.New("temporal client unavailable")

type Service struct {
	repo          Repository
	temporal      temporal.WorkflowClient
	logger        *logging.Logger
	activityDefs  []ActivityDef
	activityIndex map[string]ActivityDef
}

func DefaultConfigPath() string {
	return filepath.Join(defaultGestaltStateDirectory, defaultConfigDirectory, defaultConfigFilename)
}

func NewService(repo Repository, temporalClient temporal.WorkflowClient, logger *logging.Logger) *Service {
	defs := ActivityCatalog()
	return &Service{
		repo:          repo,
		temporal:      temporalClient,
		logger:        logger,
		activityDefs:  defs,
		activityIndex: activityIndex(defs),
	}
}

func (s *Service) ActivityCatalog() []ActivityDef {
	if s == nil {
		return ActivityCatalog()
	}
	return cloneActivityDefs(s.activityDefs)
}

func (s *Service) TemporalAvailable() bool {
	return s != nil && s.temporal != nil
}

func (s *Service) LoadConfig() (Config, error) {
	if s == nil || s.repo == nil {
		return DefaultConfig(), errors.New("flow repository unavailable")
	}
	cfg, err := s.repo.Load()
	if err != nil {
		if errors.Is(err, ErrInvalidConfig) {
			s.logWarn("flow config invalid", map[string]string{
				"error": err.Error(),
			})
			return cfg, nil
		}
		return cfg, err
	}
	return cfg, nil
}

func (s *Service) SaveConfig(ctx context.Context, cfg Config) (Config, error) {
	if s == nil || s.repo == nil {
		return DefaultConfig(), errors.New("flow repository unavailable")
	}
	normalized := normalizeConfig(cfg)
	if err := ValidateConfig(normalized, s.activityDefs); err != nil {
		return normalized, err
	}
	if err := s.repo.Save(normalized); err != nil {
		return normalized, err
	}
	if err := s.signalConfigUpdated(ctx, normalized); err != nil {
		return normalized, err
	}
	return normalized, nil
}

func (s *Service) signalConfigUpdated(ctx context.Context, cfg Config) error {
	if s.temporal == nil {
		return ErrTemporalUnavailable
	}
	signalContext := ctx
	if signalContext == nil {
		signalContext = context.Background()
	}
	signalContext, cancel := context.WithTimeout(signalContext, defaultSignalTimeout)
	defer cancel()

	options := client.StartWorkflowOptions{
		ID:        RouterWorkflowID,
		TaskQueue: workflows.SessionTaskQueueName,
	}
	_, err := s.temporal.SignalWithStartWorkflow(signalContext, RouterWorkflowID, RouterWorkflowConfigSignal, cfg, options, RouterWorkflowType)
	return err
}

func (s *Service) logWarn(message string, fields map[string]string) {
	if s == nil || s.logger == nil {
		return
	}
	s.logger.Warn(message, fields)
}

func normalizeConfig(cfg Config) Config {
	if cfg.Version == 0 {
		cfg.Version = ConfigVersion
	}
	if cfg.Triggers == nil {
		cfg.Triggers = []EventTrigger{}
	}
	for i, trigger := range cfg.Triggers {
		if trigger.Where == nil {
			trigger.Where = map[string]string{}
			cfg.Triggers[i] = trigger
		}
	}
	if cfg.BindingsByTriggerID == nil {
		cfg.BindingsByTriggerID = map[string][]ActivityBinding{}
	}
	for triggerID, bindings := range cfg.BindingsByTriggerID {
		for i, binding := range bindings {
			if binding.Config == nil {
				binding.Config = map[string]any{}
				bindings[i] = binding
			}
		}
		cfg.BindingsByTriggerID[triggerID] = bindings
	}
	return cfg
}
