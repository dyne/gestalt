package config

import "testing"

func TestDecideConffile(t *testing.T) {
	tests := []struct {
		name  string
		input ConffileDecisionInput
		want  ConffileDecision
	}{
		{
			name: "dest missing installs",
			input: ConffileDecisionInput{
				DestExists: false,
			},
			want: ConffileDecisionInstall,
		},
		{
			name: "baseline missing prompts",
			input: ConffileDecisionInput{
				DestExists:  true,
				HasBaseline: false,
				LocalHash:   "local",
				NewHash:     "new",
			},
			want: ConffileDecisionPrompt,
		},
		{
			name: "up to date skips",
			input: ConffileDecisionInput{
				DestExists:  true,
				HasBaseline: true,
				LocalHash:   "same",
				OldHash:     "old",
				NewHash:     "same",
			},
			want: ConffileDecisionSkip,
		},
		{
			name: "package updated installs",
			input: ConffileDecisionInput{
				DestExists:  true,
				HasBaseline: true,
				LocalHash:   "old",
				OldHash:     "old",
				NewHash:     "new",
			},
			want: ConffileDecisionInstall,
		},
		{
			name: "local modified keeps",
			input: ConffileDecisionInput{
				DestExists:  true,
				HasBaseline: true,
				LocalHash:   "local",
				OldHash:     "old",
				NewHash:     "old",
			},
			want: ConffileDecisionKeep,
		},
		{
			name: "conflict prompts",
			input: ConffileDecisionInput{
				DestExists:  true,
				HasBaseline: true,
				LocalHash:   "local",
				OldHash:     "old",
				NewHash:     "new",
			},
			want: ConffileDecisionPrompt,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DecideConffile(tc.input)
			if got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}
