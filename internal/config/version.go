package config

import (
	"fmt"
	"log"

	"gestalt/internal/version"
)

const (
	majorMismatchMessage = "Breaking changes detected. Backup .gestalt/ and run with --force-upgrade"
	minorMismatchMessage = "Config may be outdated. Review .bck files after startup."
)

func CheckVersionCompatibility(installed, current version.VersionInfo) error {
	if installed.Major != current.Major {
		return fmt.Errorf("incompatible major version: %s -> %s. %s", formatVersion(installed), formatVersion(current), majorMismatchMessage)
	}
	if installed.Minor != current.Minor {
		log.Print(minorMismatchMessage)
	}
	if installed.Major == current.Major && installed.Minor == current.Minor && installed.Patch != current.Patch {
		log.Printf("Config updated from %s to %s", formatVersion(installed), formatVersion(current))
	}
	return nil
}

func formatVersion(info version.VersionInfo) string {
	return fmt.Sprintf("%d.%d.%d", info.Major, info.Minor, info.Patch)
}
