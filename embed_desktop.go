package gestalt

import "embed"

// EmbeddedDesktopFrontendFS provides the built desktop frontend assets.
//
//go:embed frontend/build
var EmbeddedDesktopFrontendFS embed.FS
