package schema

import (
	"testing"

	"github.com/invopop/jsonschema"
)

func TestRegisterResolveAndCache(t *testing.T) {
	t.Cleanup(func() {
		clearRegistryForTest()
		ClearCache()
	})

	callCount := 0
	if err := Register("Example", func() *jsonschema.Schema {
		callCount++
		return &jsonschema.Schema{Title: "example"}
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	first, err := Resolve("example")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	second, err := Resolve("example")
	if err != nil {
		t.Fatalf("resolve cached: %v", err)
	}

	if first != second {
		t.Fatal("expected cached schema instance")
	}
	if callCount != 1 {
		t.Fatalf("expected provider called once, got %d", callCount)
	}
}

func TestRegisterValidation(t *testing.T) {
	t.Cleanup(func() {
		clearRegistryForTest()
		ClearCache()
	})

	if err := Register("", func() *jsonschema.Schema { return &jsonschema.Schema{} }); err == nil {
		t.Fatal("expected empty name error")
	}
	if err := Register("x", nil); err == nil {
		t.Fatal("expected nil provider error")
	}
}

func TestResolveUnknown(t *testing.T) {
	t.Cleanup(func() {
		clearRegistryForTest()
		ClearCache()
	})

	if _, err := Resolve(""); err == nil {
		t.Fatal("expected empty name lookup error")
	}
	if _, err := Resolve("missing"); err == nil {
		t.Fatal("expected missing schema error")
	}
}

func TestClearCache(t *testing.T) {
	t.Cleanup(func() {
		clearRegistryForTest()
		ClearCache()
	})

	count := 0
	if err := Register("cache-test", func() *jsonschema.Schema {
		count++
		return &jsonschema.Schema{Title: "cache-test"}
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	if _, err := Resolve("cache-test"); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	ClearCache()
	if _, err := Resolve("cache-test"); err != nil {
		t.Fatalf("resolve after clear: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected provider called twice, got %d", count)
	}
}

func clearRegistryForTest() {
	registryMu.Lock()
	registry = map[string]Provider{}
	registryMu.Unlock()
}
