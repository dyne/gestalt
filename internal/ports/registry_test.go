package ports

import (
	"sync"
	"testing"
)

func TestPortRegistrySetAndGet(t *testing.T) {
	registry := NewPortRegistry()
	registry.Set("backend", 8080)

	port, found := registry.Get("backend")
	if !found {
		t.Fatalf("expected backend port to be found")
	}
	if port != 8080 {
		t.Fatalf("expected port 8080, got %d", port)
	}
}

func TestPortRegistryNormalizesServiceNames(t *testing.T) {
	registry := NewPortRegistry()
	registry.Set(" Backend ", 9000)

	port, found := registry.Get("backend")
	if !found || port != 9000 {
		t.Fatalf("expected normalized lookup to return 9000, got %d (found=%v)", port, found)
	}
}

func TestPortRegistryUnknownService(t *testing.T) {
	registry := NewPortRegistry()

	if port, found := registry.Get("missing"); found || port != 0 {
		t.Fatalf("expected missing service to return (0, false), got (%d, %v)", port, found)
	}
}

func TestPortRegistryConcurrentAccess(t *testing.T) {
	registry := NewPortRegistry()
	services := []string{"backend", "frontend", "temporal", "otel"}

	var waitGroup sync.WaitGroup
	for index, service := range services {
		waitGroup.Add(1)
		go func(serviceName string, port int) {
			defer waitGroup.Done()
			for iteration := 0; iteration < 100; iteration++ {
				registry.Set(serviceName, port)
				_, _ = registry.Get(serviceName)
			}
		}(service, 8000+index)
	}
	waitGroup.Wait()

	for index, service := range services {
		port, found := registry.Get(service)
		if !found {
			t.Fatalf("expected service %q to be present", service)
		}
		expectedPort := 8000 + index
		if port != expectedPort {
			t.Fatalf("expected port %d for %s, got %d", expectedPort, service, port)
		}
	}
}

func TestPortRegistryRejectsInvalidInputs(t *testing.T) {
	registry := NewPortRegistry()
	registry.Set("", 8080)
	registry.Set("backend", 0)

	if _, found := registry.Get("backend"); found {
		t.Fatalf("expected invalid inputs to be ignored")
	}
}
