package main

import (
	"context"
	"errors"
	"sync"

	"gestalt/internal/logging"
)

type shutdownPhase struct {
	name string
	stop func(context.Context) error
}

type shutdownCoordinator struct {
	logger *logging.Logger
	once   sync.Once
	phases []shutdownPhase
}

func newShutdownCoordinator(logger *logging.Logger) *shutdownCoordinator {
	return &shutdownCoordinator{
		logger: logger,
	}
}

func (coordinator *shutdownCoordinator) Add(name string, stop func(context.Context) error) {
	if coordinator == nil || stop == nil {
		return
	}
	coordinator.phases = append(coordinator.phases, shutdownPhase{
		name: name,
		stop: stop,
	})
}

func (coordinator *shutdownCoordinator) Run(ctx context.Context) error {
	if coordinator == nil {
		return nil
	}
	var runErr error
	coordinator.once.Do(func() {
		for _, phase := range coordinator.phases {
			if phase.stop == nil {
				continue
			}
			if coordinator.logger != nil {
				coordinator.logger.Info("shutdown phase starting", map[string]string{
					"phase": phase.name,
				})
			}
			if err := phase.stop(ctx); err != nil {
				runErr = errors.Join(runErr, err)
				if coordinator.logger != nil {
					coordinator.logger.Warn("shutdown phase failed", map[string]string{
						"phase": phase.name,
						"error": err.Error(),
					})
				}
			}
		}
	})
	return runErr
}
