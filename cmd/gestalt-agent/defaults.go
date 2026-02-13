package main

import (
	"os"
	"strings"
)

func defaultGestaltHost() string {
	return "127.0.0.1"
}

func defaultGestaltPort() int {
	return 57417
}

func defaultGestaltToken() string {
	return strings.TrimSpace(os.Getenv("GESTALT_TOKEN"))
}
