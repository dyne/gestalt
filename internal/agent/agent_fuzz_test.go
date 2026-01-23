package agent

import (
	"testing"

	"github.com/BurntSushi/toml"
)

type promptWrapper struct {
	Prompt PromptList `toml:"prompt"`
}

func FuzzPromptListUnmarshalTOML(f *testing.F) {
	seeds := [][]byte{
		[]byte(`"coder"`),
		[]byte(`["coder","architect"]`),
		[]byte(`""`),
		[]byte(`[]`),
		[]byte(`123`),
		[]byte(`{}`),
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		payload := append([]byte("prompt = "), data...)
		var wrapper promptWrapper
		_, _ = toml.Decode(string(payload), &wrapper)
	})
}
