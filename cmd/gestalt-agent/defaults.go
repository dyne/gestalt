package main

import (
	"os"
	"strings"
)

func defaultGestaltURL() string {
	if value := strings.TrimSpace(os.Getenv("GESTALT_URL")); value != "" {
		return value
	}
	return "http://localhost:57417"
}

func defaultGestaltToken() string {
	return strings.TrimSpace(os.Getenv("GESTALT_TOKEN"))
}
