package skill

import (
	"bytes"
	"encoding/xml"
	"path/filepath"
	"strings"
)

// GeneratePromptXML builds XML metadata for skill discovery.
func GeneratePromptXML(skills []*Skill) string {
	if len(skills) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("<available_skills>")
	for _, entry := range skills {
		if entry == nil {
			continue
		}
		builder.WriteString("<skill>")
		writeElement(&builder, "name", strings.TrimSpace(entry.Name))
		writeElement(&builder, "description", strings.TrimSpace(entry.Description))
		location := skillLocation(entry)
		writeElement(&builder, "location", location)
		builder.WriteString("</skill>")
	}
	builder.WriteString("</available_skills>")
	return builder.String()
}

func skillLocation(entry *Skill) string {
	if entry == nil || strings.TrimSpace(entry.Path) == "" {
		return ""
	}
	path := filepath.Join(entry.Path, "SKILL.md")
	absolute, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return absolute
}

func writeElement(builder *strings.Builder, name, value string) {
	builder.WriteString("<")
	builder.WriteString(name)
	builder.WriteString(">")
	if value != "" {
		var escaped bytes.Buffer
		_ = xml.EscapeText(&escaped, []byte(value))
		builder.Write(escaped.Bytes())
	}
	builder.WriteString("</")
	builder.WriteString(name)
	builder.WriteString(">")
}
