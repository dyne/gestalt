package gitlog

import (
	"fmt"
	"strconv"
	"strings"
)

func ParseLogOutput(raw string, maxFilesPerCommit int) ([]Commit, error) {
	if strings.TrimSpace(raw) == "" {
		return []Commit{}, nil
	}
	if maxFilesPerCommit <= 0 {
		maxFilesPerCommit = DefaultMaxFilesPerCommit
	}

	lines := strings.Split(raw, "\n")
	commits := make([]Commit, 0)
	var current *Commit

	flush := func() {
		if current == nil {
			return
		}
		commits = append(commits, *current)
		current = nil
	}

	for _, line := range lines {
		if line == "" {
			continue
		}
		if strings.Count(line, "\x00") >= 2 {
			flush()
			parts := strings.SplitN(line, "\x00", 3)
			sha := strings.TrimSpace(parts[0])
			if sha == "" {
				return nil, fmt.Errorf("invalid git log header: missing sha")
			}
			current = &Commit{
				SHA:         sha,
				ShortSHA:    shortSHA(sha),
				CommittedAt: strings.TrimSpace(parts[1]),
				Subject:     strings.TrimSpace(parts[2]),
				Files:       make([]FileStat, 0),
			}
			continue
		}
		if current == nil {
			continue
		}
		file, added, deleted, binary, err := parseNumstatLine(line)
		if err != nil {
			continue
		}
		current.Stats.FilesChanged++
		if binary {
			current.Stats.HasBinary = true
		} else {
			current.Stats.LinesAdded += added
			current.Stats.LinesDeleted += deleted
		}
		if len(current.Files) < maxFilesPerCommit {
			var addedPtr *int
			var deletedPtr *int
			if !binary {
				addedCopy := added
				deletedCopy := deleted
				addedPtr = &addedCopy
				deletedPtr = &deletedCopy
			}
			current.Files = append(current.Files, FileStat{
				Path:    file,
				Added:   addedPtr,
				Deleted: deletedPtr,
				Binary:  binary,
			})
		} else {
			current.FilesTruncated = true
		}
	}

	flush()
	return commits, nil
}

func parseNumstatLine(line string) (path string, added int, deleted int, binary bool, err error) {
	parts := strings.SplitN(line, "\t", 3)
	if len(parts) != 3 {
		return "", 0, 0, false, fmt.Errorf("invalid numstat line")
	}
	addedRaw := strings.TrimSpace(parts[0])
	deletedRaw := strings.TrimSpace(parts[1])
	path = strings.TrimSpace(parts[2])
	if path == "" {
		return "", 0, 0, false, fmt.Errorf("invalid numstat path")
	}

	if addedRaw == "-" || deletedRaw == "-" {
		return path, 0, 0, true, nil
	}

	added, err = strconv.Atoi(addedRaw)
	if err != nil {
		return "", 0, 0, false, err
	}
	deleted, err = strconv.Atoi(deletedRaw)
	if err != nil {
		return "", 0, 0, false, err
	}
	return path, added, deleted, false, nil
}

func shortSHA(sha string) string {
	if len(sha) <= 12 {
		return sha
	}
	return sha[:12]
}
