package plan

import (
	"bufio"
	"strings"
)

type ParseSummary struct {
	WipL1Count int
	WipL2Count int
}

func ParseCurrentWork(content string) (CurrentWork, ParseSummary, error) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	buffer := make([]byte, 0, 64*1024)
	scanner.Buffer(buffer, 1024*1024)

	var currentWork CurrentWork
	var summary ParseSummary
	selectedL1 := false
	inSelectedL1 := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		level, keyword, title, ok := parseHeading(line)
		if !ok {
			continue
		}
		switch level {
		case 1:
			inSelectedL1 = false
			if keyword == "WIP" {
				summary.WipL1Count++
				if !selectedL1 {
					selectedL1 = true
					currentWork.L1 = title
					inSelectedL1 = true
				}
			}
		case 2:
			if !inSelectedL1 {
				continue
			}
			if keyword == "WIP" {
				summary.WipL2Count++
				if currentWork.L2 == "" {
					currentWork.L2 = title
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return CurrentWork{}, summary, err
	}

	return currentWork, summary, nil
}

func parseHeading(line string) (int, string, string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || !strings.HasPrefix(trimmed, "*") {
		return 0, "", "", false
	}
	level := 0
	for level < len(trimmed) && trimmed[level] == '*' {
		level++
	}
	if level == 0 || level >= len(trimmed) || trimmed[level] != ' ' {
		return 0, "", "", false
	}
	rest := strings.TrimSpace(trimmed[level:])
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return level, "", "", false
	}
	keyword := strings.ToUpper(fields[0])
	titleParts := fields[1:]
	if len(titleParts) > 0 && isPriorityToken(titleParts[0]) {
		titleParts = titleParts[1:]
	}
	title := strings.Join(titleParts, " ")
	return level, keyword, title, true
}

func isPriorityToken(token string) bool {
	return strings.HasPrefix(token, "[#") && strings.HasSuffix(token, "]")
}
