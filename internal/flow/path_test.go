package flow

import "testing"

func TestNormalizeFlowID(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "lowercase and allowed", in: "My_Flow-01", want: "my_flow-01"},
		{name: "replace spaces and symbols", in: "Alpha / Beta#1", want: "alpha-beta-1"},
		{name: "collapse dash runs", in: "a---b   c", want: "a-b-c"},
		{name: "trim boundary dashes", in: " !!!my-flow!!! ", want: "my-flow"},
		{name: "empty when no allowed chars", in: "@@@", want: ""},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := normalizeFlowID(tc.in); got != tc.want {
				t.Fatalf("normalizeFlowID(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestManagedFlowFilename(t *testing.T) {
	t.Parallel()

	got, err := managedFlowFilename("Flow Name")
	if err != nil {
		t.Fatalf("managedFlowFilename: %v", err)
	}
	if got != "flow-name.flow.yaml" {
		t.Fatalf("unexpected filename %q", got)
	}
}

func TestManagedFlowFilenameRejectsEmptyNormalizedID(t *testing.T) {
	t.Parallel()

	_, err := managedFlowFilename("$$$")
	if err == nil {
		t.Fatal("expected validation error")
	}
	validationErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	if validationErr.Kind != ValidationBadRequest {
		t.Fatalf("expected bad_request error, got %q", validationErr.Kind)
	}
}

func TestValidateManagedFilenameCollisions(t *testing.T) {
	t.Parallel()

	err := validateManagedFilenameCollisions([]EventTrigger{
		{ID: "Flow A"},
		{ID: "flow-a"},
	})
	if err == nil {
		t.Fatal("expected collision error")
	}
	validationErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	if validationErr.Kind != ValidationConflict {
		t.Fatalf("expected conflict error, got %q", validationErr.Kind)
	}
}
