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

// EmbeddedScipAssetsFS provides embedded SCIP indexer assets and manifest.
//
//go:embed assets/scip
var EmbeddedScipAssetsFS embed.FS
