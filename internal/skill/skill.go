package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var namePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// Skill describes a parsed skill package.
type Skill struct {
	Name          string
	Description   string
	License       string
	Compatibility string
	Metadata      map[string]any
	AllowedTools  []string
	Path          string
	Content       string
}

type frontmatter struct {
	Name          string         `yaml:"name"`
	Description   string         `yaml:"description"`
	License       string         `yaml:"license"`
	Compatibility string         `yaml:"compatibility"`
	Metadata      map[string]any `yaml:"metadata"`
	AllowedTools  []string       `yaml:"allowed_tools"`
}

// ParseFile reads and validates a SKILL.md file on disk.
func ParseFile(path string) (*Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read skill file %s: %w", path, err)
	}
	skill, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parse skill file %s: %w", path, err)
	}
	skill.Path = filepath.Dir(path)
	if err := skill.Validate(); err != nil {
		return nil, fmt.Errorf("validate skill file %s: %w", path, err)
	}
	return skill, nil
}

// Parse decodes frontmatter and body content from SKILL.md data.
func Parse(data []byte) (*Skill, error) {
	frontmatterBytes, body, err := splitFrontmatter(data)
	if err != nil {
		return nil, err
	}

	var fm frontmatter
	if err := yaml.Unmarshal(frontmatterBytes, &fm); err != nil {
		return nil, fmt.Errorf("parse frontmatter: %w", err)
	}

	skill := &Skill{
		Name:          strings.TrimSpace(fm.Name),
		Description:   strings.TrimSpace(fm.Description),
		License:       strings.TrimSpace(fm.License),
		Compatibility: strings.TrimSpace(fm.Compatibility),
		Metadata:      fm.Metadata,
		Content:       body,
	}
	if len(fm.AllowedTools) > 0 {
		allowed := make([]string, 0, len(fm.AllowedTools))
		for _, tool := range fm.AllowedTools {
			tool = strings.TrimSpace(tool)
			if tool != "" {
				allowed = append(allowed, tool)
			}
		}
		if len(allowed) > 0 {
			skill.AllowedTools = allowed
		}
	}

	return skill, nil
}

// Validate ensures required fields and structural rules are satisfied.
func (s Skill) Validate() error {
	name := strings.TrimSpace(s.Name)
	if name == "" {
		return fmt.Errorf("skill name is required")
	}
	if len(name) > 64 {
		return fmt.Errorf("skill name must be 1-64 characters")
	}
	if !namePattern.MatchString(name) {
		return fmt.Errorf("skill name %q is invalid", name)
	}

	description := strings.TrimSpace(s.Description)
	if len(description) == 0 || len(description) > 1024 {
		return fmt.Errorf("skill description must be 1-1024 characters")
	}

	if s.Path != "" {
		dirName := filepath.Base(s.Path)
		if dirName != name {
			return fmt.Errorf("skill name %q does not match directory %q", name, dirName)
		}
		for _, dir := range []string{"scripts", "references", "assets"} {
			if err := validateOptionalDir(s.Path, dir); err != nil {
				return err
			}
		}
	}

	return nil
}

func splitFrontmatter(data []byte) ([]byte, string, error) {
	if len(data) == 0 {
		return nil, "", fmt.Errorf("missing frontmatter")
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 {
		return nil, "", fmt.Errorf("missing frontmatter")
	}

	first := strings.TrimSuffix(lines[0], "\r")
	if strings.TrimSpace(first) != "---" {
		return nil, "", fmt.Errorf("missing frontmatter delimiter")
	}

	end := -1
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSuffix(lines[i], "\r")
		if strings.TrimSpace(line) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return nil, "", fmt.Errorf("missing closing frontmatter delimiter")
	}

	fmLines := make([]string, 0, end-1)
	for _, line := range lines[1:end] {
		fmLines = append(fmLines, strings.TrimSuffix(line, "\r"))
	}
	bodyLines := lines[end+1:]
	for i, line := range bodyLines {
		bodyLines[i] = strings.TrimSuffix(line, "\r")
	}

	return []byte(strings.Join(fmLines, "\n")), strings.Join(bodyLines, "\n"), nil
}

func validateOptionalDir(base, name string) error {
	path := filepath.Join(base, name)
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("check %s directory: %w", name, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", name)
	}
	return nil
}
