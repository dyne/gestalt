package otel

import (
	"gestalt/internal/logging"
)

const fallbackScopeName = "gestalt/internal/logging"

func StartLogHubFallback(logger *logging.Logger, options SDKOptions) func() {
	if logger == nil {
		return func() {}
	}
	hub := ActiveLogHub()
	if hub == nil {
		return func() {}
	}
	resource := otlpResourceFromAttributes(resourceAttributesFromOptions(options))
	ch, cancel := logger.Subscribe()
	done := make(chan struct{})

	go func() {
		defer close(done)
		for entry := range ch {
			hub.Append(legacyEntryToOTLP(entry, resource, fallbackScopeName))
		}
	}()

	return func() {
		cancel()
		<-done
	}
}
