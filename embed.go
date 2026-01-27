package gestalt

import "embed"

// EmbeddedFrontendFS provides the built frontend assets.
//
//go:embed frontend/dist
var EmbeddedFrontendFS embed.FS

// EmbeddedConfigFS provides default agent, prompt, and skill configuration.
//
//go:embed config
var EmbeddedConfigFS embed.FS
