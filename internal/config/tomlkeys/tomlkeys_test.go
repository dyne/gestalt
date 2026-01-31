package tomlkeys

import "testing"

func TestTableAndDottedKeysAreEquivalent(t *testing.T) {
	cases := []string{
		`[session]
log-max-bytes = 4096
`,
		`session.log-max-bytes = 4096
`,
	}
	for _, input := range cases {
		store, err := Decode([]byte(input))
		if err != nil {
			t.Fatalf("decode toml: %v", err)
		}
		value, ok := store.GetInt("session.log-max-bytes")
		if !ok {
			t.Fatalf("expected session.log-max-bytes value")
		}
		if value != 4096 {
			t.Fatalf("expected 4096, got %d", value)
		}
	}
}

func TestNormalizationHandlesUnderscoresAndCase(t *testing.T) {
	input := `[Session]
LOG_MAX_BYTES = 123
`
	store, err := Decode([]byte(input))
	if err != nil {
		t.Fatalf("decode toml: %v", err)
	}
	value, ok := store.GetInt("session.log-max-bytes")
	if !ok {
		t.Fatalf("expected normalized key to resolve")
	}
	if value != 123 {
		t.Fatalf("expected 123, got %d", value)
	}
}

func TestTypePreservation(t *testing.T) {
	input := `flag = true
count = 7
name = "hello"
`
	store, err := Decode([]byte(input))
	if err != nil {
		t.Fatalf("decode toml: %v", err)
	}
	flag, ok := store.GetBool("flag")
	if !ok || !flag {
		t.Fatalf("expected flag true")
	}
	count, ok := store.GetInt("count")
	if !ok || count != 7 {
		t.Fatalf("expected count 7, got %d", count)
	}
	name, ok := store.GetString("name")
	if !ok || name != "hello" {
		t.Fatalf("expected name hello, got %q", name)
	}
	if _, ok := store.GetString("count"); ok {
		t.Fatalf("expected count to not be a string")
	}
}

func TestArraysArePreservedAsValues(t *testing.T) {
	input := `skills = ["alpha", "beta"]
`
	store, err := Decode([]byte(input))
	if err != nil {
		t.Fatalf("decode toml: %v", err)
	}
	value, ok := store.flat["skills"]
	if !ok {
		t.Fatalf("expected skills key")
	}
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("expected skills to be []any, got %T", value)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(items))
	}
}
