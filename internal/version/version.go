package version

import "strconv"

// Version values are set at build time using -ldflags.
var Version = "dev"
var Major = "0"
var Minor = "0"
var Patch = "0"
var Built = ""
var GitCommit = ""

type VersionInfo struct {
	Version   string `json:"version"`
	Major     int    `json:"major"`
	Minor     int    `json:"minor"`
	Patch     int    `json:"patch"`
	Built     string `json:"built"`
	GitCommit string `json:"git_commit,omitempty"`
}

func GetVersionInfo() VersionInfo {
	return VersionInfo{
		Version:   Version,
		Major:     parseInt(Major),
		Minor:     parseInt(Minor),
		Patch:     parseInt(Patch),
		Built:     Built,
		GitCommit: GitCommit,
	}
}

func parseInt(value string) int {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return parsed
}
