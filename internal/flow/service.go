package flow

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"gestalt/internal/logging"
)

const (
	defaultConfigDirectory       = "config/flows"
	defaultGestaltStateDirectory = ".gestalt"
	defaultFlowDeduperSize       = 500
)

var ErrDispatcherUnavailable = errors.New("flow dispatcher unavailable")

// Dispatcher executes a single activity request.
type Dispatcher interface {
	Dispatch(ctx context.Context, request ActivityRequest) error
}

type Service struct {
	repo          Repository
	dispatcher    Dispatcher
	logger        *logging.Logger
	activityDefs  []ActivityDef
	activityIndex map[string]ActivityDef
	deduper       *EventDeduper
}

func DefaultConfigPath() string {
	return filepath.Join(defaultGestaltStateDirectory, defaultConfigDirectory)
}

// ConfigPath returns the flow configuration directory for a given config root.
func ConfigPath(configDir string) string {
	if strings.TrimSpace(configDir) == "" {
		return DefaultConfigPath()
	}
	return filepath.Join(configDir, "flows")
}

func NewService(repo Repository, dispatcher Dispatcher, logger *logging.Logger) *Service {
	defs := ActivityCatalog()
	return &Service{
		repo:          repo,
		dispatcher:    dispatcher,
		logger:        logger,
		activityDefs:  defs,
		activityIndex: activityIndex(defs),
		deduper:       NewEventDeduper(defaultFlowDeduperSize),
	}
}

func (s *Service) ActivityCatalog() []ActivityDef {
	if s == nil {
		return ActivityCatalog()
	}
	return cloneActivityDefs(s.activityDefs)
}

func (s *Service) ConfigPath() string {
	if s == nil || s.repo == nil {
		return ""
	}
	type pathProvider interface {
		Path() string
	}
	if provider, ok := s.repo.(pathProvider); ok {
		return provider.Path()
	}
	return ""
}

func (s *Service) DispatcherAvailable() bool {
	return s != nil && s.dispatcher != nil
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
	if err := s.SignalConfig(ctx, normalized); err != nil {
		return normalized, err
	}
	return normalized, nil
}

func (s *Service) SignalConfig(_ context.Context, _ Config) error {
	if s == nil {
		return errors.New("flow service unavailable")
	}
	return nil
}

func (s *Service) SignalEvent(ctx context.Context, fields map[string]string, eventID string) error {
	if s == nil {
		return errors.New("flow service unavailable")
	}
	if s.dispatcher == nil {
		return ErrDispatcherUnavailable
	}
	if fields == nil {
		fields = map[string]string{}
	}
	trimmedEventID := strings.TrimSpace(eventID)
	if trimmedEventID == "" {
		trimmedEventID = BuildEventID(fields)
	}
	if trimmedEventID == "" {
		return nil
	}
	if s.deduper != nil && s.deduper.Seen(trimmedEventID) {
		return nil
	}
	cfg, err := s.LoadConfig()
	if err != nil {
		return err
	}
	matches := MatchBindings(cfg, fields)
	for _, match := range matches {
		request := ActivityRequest{
			EventID:    trimmedEventID,
			TriggerID:  match.Trigger.ID,
			ActivityID: match.Binding.ActivityID,
			Event:      fields,
			Config:     match.Binding.Config,
		}
		if dispatchErr := s.dispatcher.Dispatch(ctx, request); dispatchErr != nil {
			s.logWarn("flow activity dispatch failed", map[string]string{
				"error":       dispatchErr.Error(),
				"event_id":    trimmedEventID,
				"trigger_id":  match.Trigger.ID,
				"activity_id": match.Binding.ActivityID,
			})
		}
	}
	return nil
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
