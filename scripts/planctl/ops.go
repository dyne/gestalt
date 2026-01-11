package main

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

type Result struct {
	Level    int
	Status   string
	Priority string
	Title    string
	Line     int
}

func (result Result) Format() string {
	return fmt.Sprintf("%d\t%s\t%s\t%s\t%d", result.Level, result.Status, result.Priority, result.Title, result.Line)
}

func readLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return []string{}, nil
	}
	return strings.Split(text, "\n"), nil
}

func writeLines(path string, lines []string) error {
	content := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(path, []byte(content), 0o644)
}

func normalizeStatus(status string) string {
	return strings.ToUpper(strings.TrimSpace(status))
}

func statusFilter(values string) map[string]bool {
	if strings.TrimSpace(values) == "" {
		return nil
	}
	filter := make(map[string]bool)
	for _, value := range strings.Split(values, ",") {
		trimmed := normalizeStatus(value)
		if trimmed == "" {
			continue
		}
		filter[trimmed] = true
	}
	return filter
}

func matchesStatus(status string, filter map[string]bool) bool {
	if filter == nil {
		return true
	}
	if status == "" {
		return filter["NONE"] || filter["-"]
	}
	return filter[normalizeStatus(status)]
}

func filterByLevel(level int, heading Heading) bool {
	if level == 0 {
		return true
	}
	return heading.Level == level
}

func listHeadings(plan Plan, level int, statusFilter map[string]bool) []Result {
	results := make([]Result, 0)
	for _, heading := range plan.Headings {
		if !filterByLevel(level, heading) {
			continue
		}
		if !matchesStatus(heading.Status, statusFilter) {
			continue
		}
		results = append(results, Result{
			Level:    heading.Level,
			Status:   heading.Status,
			Priority: heading.Priority,
			Title:    heading.Title,
			Line:     heading.LineIndex + 1,
		})
	}
	return results
}

func findHeadings(plan Plan, level int, statusFilter map[string]bool, query string) []Result {
	query = strings.ToLower(strings.TrimSpace(query))
	results := make([]Result, 0)
	for _, heading := range plan.Headings {
		if !filterByLevel(level, heading) {
			continue
		}
		if !matchesStatus(heading.Status, statusFilter) {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(heading.Title), query) {
			continue
		}
		results = append(results, Result{
			Level:    heading.Level,
			Status:   heading.Status,
			Priority: heading.Priority,
			Title:    heading.Title,
			Line:     heading.LineIndex + 1,
		})
	}
	return results
}

func findHeadingByTitle(plan Plan, level int, title string) ([]Heading, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, errors.New("title is required")
	}
	matches := make([]Heading, 0)
	for _, heading := range plan.Headings {
		if heading.Level == level && heading.Title == title {
			matches = append(matches, heading)
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no heading found with title %q", title)
	}
	return matches, nil
}

func ensureUnique(matches []Heading, title string) (Heading, error) {
	if len(matches) == 1 {
		return matches[0], nil
	}
	return Heading{}, fmt.Errorf("multiple headings found with title %q", title)
}

func findSectionEnd(lines []string, start int) int {
	for index := start + 1; index < len(lines); index++ {
		if heading, ok := parseHeading(lines[index], index); ok && heading.Level == 1 {
			return index
		}
	}
	return len(lines)
}

func setStatus(lines []string, level int, title string, status string) ([]string, error) {
	plan := parsePlan(lines)
	matches, err := findHeadingByTitle(plan, level, title)
	if err != nil {
		return nil, err
	}
	heading, err := ensureUnique(matches, title)
	if err != nil {
		return nil, err
	}
	status = normalizeStatus(status)
	if status == "NONE" || status == "-" {
		status = ""
	}
	if status != "" && status != "TODO" && status != "WIP" && status != "DONE" {
		return nil, fmt.Errorf("unsupported status %q", status)
	}
	lines = append([]string{}, lines...)
	lines[heading.LineIndex] = formatHeading(level, status, heading.Priority, heading.Title)
	return lines, nil
}

func completeL1(lines []string, title string) ([]string, error) {
	plan := parsePlan(lines)
	matches, err := findHeadingByTitle(plan, 1, title)
	if err != nil {
		return nil, err
	}
	heading, err := ensureUnique(matches, title)
	if err != nil {
		return nil, err
	}
	lines = append([]string{}, lines...)
	lines[heading.LineIndex] = formatHeading(1, "DONE", heading.Priority, heading.Title)
	end := findSectionEnd(lines, heading.LineIndex)
	for index := heading.LineIndex + 1; index < end; index++ {
		child, ok := parseHeading(lines[index], index)
		if !ok || child.Level != 2 {
			continue
		}
		if child.Status == "TODO" || child.Status == "WIP" || child.Status == "DONE" {
			lines[index] = formatHeading(child.Level, "", child.Priority, child.Title)
		}
	}
	return lines, nil
}

func insertL2(lines []string, l1Title, title, priority string) ([]string, error) {
	plan := parsePlan(lines)
	matches, err := findHeadingByTitle(plan, 1, l1Title)
	if err != nil {
		return nil, err
	}
	l1, err := ensureUnique(matches, l1Title)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(title) == "" {
		return nil, errors.New("new L2 title is required")
	}
	priority = strings.ToUpper(strings.TrimSpace(priority))
	if priority == "-" || priority == "NONE" {
		priority = ""
	}
	if priority != "" && len(priority) != 1 {
		return nil, fmt.Errorf("priority must be a single letter, got %q", priority)
	}
	end := findSectionEnd(lines, l1.LineIndex)
	lastL2 := -1
	for index := l1.LineIndex + 1; index < end; index++ {
		child, ok := parseHeading(lines[index], index)
		if ok && child.Level == 2 {
			lastL2 = index
		}
	}
	insertAt := end
	if lastL2 != -1 {
		for index := lastL2 + 1; index < end; index++ {
			child, ok := parseHeading(lines[index], index)
			if ok && child.Level <= 2 {
				insertAt = index
				break
			}
		}
	}
	newLine := formatHeading(2, "TODO", priority, strings.TrimSpace(title))
	lines = append([]string{}, lines...)
	lines = append(lines[:insertAt], append([]string{newLine}, lines[insertAt:]...)...)
	return lines, nil
}

func showL1(lines []string, title string) ([]string, error) {
	plan := parsePlan(lines)
	matches, err := findHeadingByTitle(plan, 1, title)
	if err != nil {
		return nil, err
	}
	heading, err := ensureUnique(matches, title)
	if err != nil {
		return nil, err
	}
	end := findSectionEnd(lines, heading.LineIndex)
	section := make([]string, 0, end-heading.LineIndex)
	section = append(section, lines[heading.LineIndex:end]...)
	return section, nil
}

func current(plan Plan) ([]Result, []string) {
	warnings := make([]string, 0)
	wipL1s := make([]Heading, 0)
	for _, heading := range plan.Headings {
		if heading.Level == 1 && heading.Status == "WIP" {
			wipL1s = append(wipL1s, heading)
		}
	}
	if len(wipL1s) > 1 {
		warnings = append(warnings, "multiple WIP L1 headings found")
	}
	if len(wipL1s) == 0 {
		return nil, warnings
	}
	l1 := wipL1s[0]
	results := []Result{{
		Level:    l1.Level,
		Status:   l1.Status,
		Priority: l1.Priority,
		Title:    l1.Title,
		Line:     l1.LineIndex + 1,
	}}

	end := findSectionEnd(plan.Lines, l1.LineIndex)
	wipL2s := make([]Heading, 0)
	for _, heading := range plan.Headings {
		if heading.Level != 2 || heading.Status != "WIP" {
			continue
		}
		if heading.LineIndex > l1.LineIndex && heading.LineIndex < end {
			wipL2s = append(wipL2s, heading)
		}
	}
	if len(wipL2s) > 1 {
		warnings = append(warnings, "multiple WIP L2 headings found under current L1")
	}
	if len(wipL2s) > 0 {
		l2 := wipL2s[0]
		results = append(results, Result{
			Level:    l2.Level,
			Status:   l2.Status,
			Priority: l2.Priority,
			Title:    l2.Title,
			Line:     l2.LineIndex + 1,
		})
	}
	return results, warnings
}

func lint(plan Plan) ([]Result, bool) {
	wipL1s := make([]Heading, 0)
	wipL2s := make([]Heading, 0)
	for _, heading := range plan.Headings {
		if heading.Status != "WIP" {
			continue
		}
		switch heading.Level {
		case 1:
			wipL1s = append(wipL1s, heading)
		case 2:
			wipL2s = append(wipL2s, heading)
		}
	}
	results := make([]Result, 0, len(wipL1s)+len(wipL2s))
	for _, heading := range append(wipL1s, wipL2s...) {
		results = append(results, Result{
			Level:    heading.Level,
			Status:   heading.Status,
			Priority: heading.Priority,
			Title:    heading.Title,
			Line:     heading.LineIndex + 1,
		})
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Line < results[j].Line
	})
	violations := len(wipL1s) > 1 || len(wipL2s) > 1
	return results, violations
}
