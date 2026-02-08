package schemas

import "github.com/invopop/jsonschema"

type OllamaConfig struct {
	Model *string `json:"model,omitempty"`
	Host *string `json:"host,omitempty"`
	Format *string `json:"format,omitempty"`
	Insecure *bool `json:"insecure,omitempty"`
	Verbose *bool `json:"verbose,omitempty"`
	NoWordWrap *bool `json:"nowordwrap,omitempty"`
}

func OllamaSchema() *jsonschema.Schema {
	return GenerateSchema(OllamaConfig{})
}
