package agent

import "testing"

func FuzzPromptListUnmarshal(f *testing.F) {
	seeds := [][]byte{
		[]byte(`"coder"`),
		[]byte(`["coder","architect"]`),
		[]byte(`""`),
		[]byte(`[]`),
		[]byte(`null`),
		[]byte(`123`),
		[]byte(`{}`),
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		var prompts PromptList
		_ = prompts.UnmarshalJSON(data)
	})
}
