package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"gestalt/internal/logging"
)

const (
	shutdownPhaseTimeout = 1 * time.Second
	shutdownTotalTimeout = 5 * time.Second
)

type shutdownPhase struct {
	name string
	stop func(context.Context) error
}

type shutdownCoordinator struct {
	logger       *logging.Logger
	once         sync.Once
	phases       []shutdownPhase
	phaseTimeout time.Duration
	totalTimeout time.Duration
}

func newShutdownCoordinator(logger *logging.Logger) *shutdownCoordinator {
	return &shutdownCoordinator{
		logger:       logger,
		phaseTimeout: shutdownPhaseTimeout,
		totalTimeout: shutdownTotalTimeout,
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
	if ctx == nil {
		ctx = context.Background()
	}
	var runErr error
	coordinator.once.Do(func() {
		phaseTimeout := coordinator.phaseTimeout
		if phaseTimeout <= 0 {
			phaseTimeout = shutdownPhaseTimeout
		}
		totalTimeout := coordinator.totalTimeout
		if totalTimeout <= 0 {
			totalTimeout = shutdownTotalTimeout
		}

		overallCtx := ctx
		if totalTimeout > 0 {
			var cancel context.CancelFunc
			overallCtx, cancel = context.WithTimeout(ctx, totalTimeout)
			defer cancel()
		}

		for _, phase := range coordinator.phases {
			if phase.stop == nil {
				continue
			}
			if err := coordinator.runPhase(overallCtx, phase, phaseTimeout); err != nil {
				runErr = errors.Join(runErr, err)
			}
			if overallCtx.Err() != nil {
				if coordinator.logger != nil {
					coordinator.logger.Warn("shutdown deadline exceeded", map[string]string{
						"error": overallCtx.Err().Error(),
					})
				}
				break
			}
		}
		if runErr != nil && coordinator.logger != nil {
			coordinator.logger.Warn("shutdown completed with errors", map[string]string{
				"error": runErr.Error(),
			})
		}
	})
	return runErr
}

func (coordinator *shutdownCoordinator) runPhase(ctx context.Context, phase shutdownPhase, timeout time.Duration) error {
	if coordinator == nil || phase.stop == nil {
		return nil
	}
	if coordinator.logger != nil {
		coordinator.logger.Info("shutdown phase starting", map[string]string{
			"phase": phase.name,
		})
	}

	phaseCtx := ctx
	var cancel context.CancelFunc
	if timeout > 0 {
		phaseCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	done := make(chan error, 1)
	go func() {
		done <- phase.stop(phaseCtx)
	}()

	select {
	case err := <-done:
		if err != nil && coordinator.logger != nil {
			coordinator.logger.Warn("shutdown phase failed", map[string]string{
				"phase": phase.name,
				"error": err.Error(),
			})
		}
		return err
	case <-phaseCtx.Done():
		err := fmt.Errorf("%s shutdown timed out: %w", phase.name, phaseCtx.Err())
		if coordinator.logger != nil {
			coordinator.logger.Warn("shutdown phase timed out", map[string]string{
				"phase": phase.name,
				"error": phaseCtx.Err().Error(),
			})
		}
		return err
	}
}
