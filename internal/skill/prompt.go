package skill

import (
	"encoding/xml"
	"strings"
)

// GeneratePromptXML builds XML metadata for skill discovery using only name and description.
// Note: location is intentionally omitted; skills content is not printed to the terminal.
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
	// strings.Builder writes never fail, so EscapeText errors are not expected here.
	_ = xml.EscapeText(builder, []byte(value))
}
