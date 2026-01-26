package plan

import (
	"fmt"
	"strings"
)

// TransformDocument converts an OrgaDocument AST into a PlanDocument.
func TransformDocument(filename string, orgaDoc *OrgaDocument) PlanDocument {
	metadata := Metadata{
		Title:    getStringProp(orgaDoc.Properties, "title"),
		Subtitle: getStringProp(orgaDoc.Properties, "subtitle"),
		Date:     getStringProp(orgaDoc.Properties, "date"),
		Keywords: getStringProp(orgaDoc.Properties, "keywords"),
	}

	headings, stats := extractHeadings(orgaDoc.Children)

	return PlanDocument{
		Filename:  filename,
		Metadata:  metadata,
		Headings:  headings,
		L1Count:   stats.L1Count,
		L2Count:   stats.L2Count,
		PriorityA: stats.PriorityA,
		PriorityB: stats.PriorityB,
		PriorityC: stats.PriorityC,
	}
}

func extractHeadings(nodes []OrgaNode) ([]Heading, Statistics) {
	var headings []Heading
	var stats Statistics
	for _, node := range nodes {
		if node.Type != "section" || node.Level != 1 {
			continue
		}
		heading, childStats, ok := parseSection(node)
		if !ok {
			continue
		}
		headings = append(headings, heading)
		stats = addStats(stats, childStats)
	}
	return headings, stats
}

func parseSection(section OrgaNode) (Heading, Statistics, bool) {
	headline, ok := findHeadline(section.Children)
	if !ok {
		return Heading{}, Statistics{}, false
	}
	keyword, priority, text := parseHeadline(headline)
	body := extractBody(section.Children)

	heading := Heading{
		Level:    section.Level,
		Keyword:  keyword,
		Priority: priority,
		Text:     text,
		Body:     body,
	}

	stats := Statistics{}
	if section.Level == 1 {
		stats.L1Count++
	}
	if section.Level == 2 {
		stats.L2Count++
		switch strings.ToUpper(priority) {
		case "A":
			stats.PriorityA++
		case "B":
			stats.PriorityB++
		case "C":
			stats.PriorityC++
		}
	}

	if section.Level == 1 {
		for _, child := range section.Children {
			if child.Type != "section" || child.Level != 2 {
				continue
			}
			subHeading, subStats, ok := parseSection(child)
			if !ok {
				continue
			}
			heading.Children = append(heading.Children, subHeading)
			stats = addStats(stats, subStats)
		}
	}

	return heading, stats, true
}

func findHeadline(nodes []OrgaNode) (OrgaNode, bool) {
	for _, node := range nodes {
		if node.Type == "headline" {
			return node, true
		}
	}
	return OrgaNode{}, false
}

func extractBody(nodes []OrgaNode) string {
	var paragraphs []string
	seenNested := false
	for _, node := range nodes {
		if node.Type == "headline" {
			continue
		}
		if node.Type == "section" {
			seenNested = true
			continue
		}
		if seenNested {
			continue
		}
		if node.Type == "paragraph" {
			paragraph := renderParagraph(node)
			if strings.TrimSpace(paragraph) != "" {
				paragraphs = append(paragraphs, paragraph)
			}
		}
	}
	return strings.TrimSpace(strings.Join(paragraphs, "\n\n"))
}

func renderParagraph(node OrgaNode) string {
	var builder strings.Builder
	for _, child := range node.Children {
		switch child.Type {
		case "text":
			builder.WriteString(child.Value)
		case "newline":
			builder.WriteString("\n")
		}
	}
	return strings.TrimSpace(builder.String())
}

func parseHeadline(headline OrgaNode) (string, string, string) {
	keyword := ""
	priority := ""
	var textParts []string

	for _, child := range headline.Children {
		switch child.Type {
		case "todo":
			if keyword == "" {
				keyword = strings.ToUpper(child.Keyword)
			}
		case "priority":
			if priority == "" {
				priority = parsePriorityToken(child.Value)
			}
		case "text":
			if child.Value != "" {
				textParts = append(textParts, child.Value)
			}
		}
	}

	rawText := strings.TrimSpace(strings.Join(textParts, " "))
	parsedKeyword, parsedPriority, parsedTitle := parseHeadingText(rawText)
	if keyword == "" {
		keyword = parsedKeyword
	}
	if priority == "" {
		priority = parsedPriority
	}

	title := rawText
	if parsedKeyword != "" {
		title = parsedTitle
	} else if title == "" {
		title = parsedTitle
	}

	return keyword, priority, strings.TrimSpace(title)
}

func parseHeadingText(raw string) (string, string, string) {
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return "", "", ""
	}

	keyword := ""
	if isKeywordToken(fields[0]) {
		keyword = strings.ToUpper(fields[0])
		fields = fields[1:]
	}

	priority := ""
	if len(fields) > 0 {
		priority = parsePriorityToken(fields[0])
		if priority != "" {
			fields = fields[1:]
		}
	}

	return keyword, priority, strings.Join(fields, " ")
}

func isKeywordToken(token string) bool {
	switch strings.ToUpper(token) {
	case "TODO", "WIP", "DONE":
		return true
	default:
		return false
	}
}

func parsePriorityToken(token string) string {
	trimmed := strings.TrimSpace(token)
	if strings.HasPrefix(trimmed, "[#") && strings.HasSuffix(trimmed, "]") && len(trimmed) >= 4 {
		return strings.ToUpper(trimmed[2 : len(trimmed)-1])
	}
	return ""
}

func addStats(base, delta Statistics) Statistics {
	base.L1Count += delta.L1Count
	base.L2Count += delta.L2Count
	base.PriorityA += delta.PriorityA
	base.PriorityB += delta.PriorityB
	base.PriorityC += delta.PriorityC
	return base
}

func getStringProp(props map[string]any, key string) string {
	if props == nil {
		return ""
	}
	value, ok := props[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}
