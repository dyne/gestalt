package skill

import (
	"encoding/xml"
	"strings"
)

// GeneratePromptXML builds XML metadata for skill discovery.
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
		if license := strings.TrimSpace(entry.License); license != "" {
			builder.WriteString("    <license>")
			writeEscaped(&builder, license)
			builder.WriteString("</license>\n")
		}
		if compatibility := strings.TrimSpace(entry.Compatibility); compatibility != "" {
			builder.WriteString("    <compatibility>")
			writeEscaped(&builder, compatibility)
			builder.WriteString("</compatibility>\n")
		}
		if len(entry.AllowedTools) > 0 {
			writeAllowedTools(&builder, entry.AllowedTools)
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

func writeAllowedTools(builder *strings.Builder, tools []string) {
	cleaned := make([]string, 0, len(tools))
	for _, tool := range tools {
		tool = strings.TrimSpace(tool)
		if tool != "" {
			cleaned = append(cleaned, tool)
		}
	}
	if len(cleaned) == 0 {
		return
	}
	builder.WriteString("    <allowed_tools>\n")
	for _, tool := range cleaned {
		builder.WriteString("      <tool>")
		writeEscaped(builder, tool)
		builder.WriteString("</tool>\n")
	}
	builder.WriteString("    </allowed_tools>\n")
}

func writeEscaped(builder *strings.Builder, value string) {
	if value == "" {
		return
	}
	// strings.Builder writes never fail, so EscapeText errors are not expected here.
	_ = xml.EscapeText(builder, []byte(value))
}
