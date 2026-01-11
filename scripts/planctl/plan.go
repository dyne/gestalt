package main

import (
	"fmt"
	"regexp"
	"strings"
)

type Heading struct {
	Level     int
	Status    string
	Priority  string
	Title     string
	LineIndex int
}

type Plan struct {
	Lines    []string
	Headings []Heading
}

var headingPattern = regexp.MustCompile(`^(\*+)\s+(?:(TODO|WIP|DONE)\s+)?(\[#([A-Z])\]\s+)?(.*)$`)

func parseHeading(line string, index int) (Heading, bool) {
	matches := headingPattern.FindStringSubmatch(line)
	if matches == nil {
		return Heading{}, false
	}
	level := len(matches[1])
	status := matches[2]
	priority := matches[4]
	title := strings.TrimSpace(matches[5])
	return Heading{
		Level:     level,
		Status:    status,
		Priority:  priority,
		Title:     title,
		LineIndex: index,
	}, true
}

func parsePlan(lines []string) Plan {
	headings := make([]Heading, 0)
	for index, line := range lines {
		if heading, ok := parseHeading(line, index); ok {
			headings = append(headings, heading)
		}
	}
	return Plan{Lines: lines, Headings: headings}
}

func formatHeading(level int, status, priority, title string) string {
	if level <= 0 {
		return title
	}
	parts := []string{strings.Repeat("*", level)}
	if status != "" {
		parts = append(parts, status)
	}
	if priority != "" {
		parts = append(parts, fmt.Sprintf("[#%s]", priority))
	}
	if title != "" {
		parts = append(parts, title)
	}
	return strings.Join(parts, " ")
}
