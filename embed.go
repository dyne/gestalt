package gestalt

import "embed"

// EmbeddedFrontendFS provides the built frontend assets.
//
//go:embed frontend/dist
var EmbeddedFrontendFS embed.FS

// EmbeddedConfigFS provides default agent, prompt, and skill configuration.
// It also includes the generated manifest.json for hash verification.
//
//go:embed config
var EmbeddedConfigFS embed.FS
