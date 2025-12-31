package skill

import (
	"fmt"
	"strings"
)

// GeneratePromptXML builds XML metadata for skill discovery with full skill elements.
// Each skill includes name, description, and location (absolute path to SKILL.md).
func GeneratePromptXML(skills []*Skill) string {
	if len(skills) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("<available_skills>\n")
	wrote := false
	for _, entry := range skills {
		if entry == nil {
			continue
		}
		name := strings.TrimSpace(entry.Name)
		description := strings.TrimSpace(entry.Description)
		path := strings.TrimSpace(entry.Path)
		if name == "" || description == "" {
			continue
		}
		
		builder.WriteString("  <skill>\n")
		builder.WriteString("    <name>")
		writeEscaped(&builder, name)
		builder.WriteString("</name>\n")
		builder.WriteString("    <description>")
		writeEscaped(&builder, description)
		builder.WriteString("</description>\n")
		
		if path != "" {
			location := fmt.Sprintf("%s/SKILL.md", path)
			builder.WriteString("    <location>")
			writeEscaped(&builder, location)
			builder.WriteString("</location>\n")
		}
		
		builder.WriteString("  </skill>\n")
		wrote = true
	}
	if !wrote {
		return ""
	}
	builder.WriteString("</available_skills>")
	return builder.String()
}

func writeEscaped(builder *strings.Builder, value string) {
	if value == "" {
		return
	}
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, "<", "&lt;")
	value = strings.ReplaceAll(value, ">", "&gt;")
	builder.WriteString(value)
}
