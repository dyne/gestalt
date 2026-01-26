package plan

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// OrgaDocument represents the parsed org AST from the orga library.
type OrgaDocument struct {
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties"`
	Children   []OrgaNode     `json:"children"`
}

type OrgaNode struct {
	Type       string         `json:"type"`
	Level      int            `json:"level,omitempty"`
	Keyword    string         `json:"keyword,omitempty"`
	Priority   string         `json:"priority,omitempty"`
	Tags       []string       `json:"tags,omitempty"`
	Actionable bool           `json:"actionable,omitempty"`
	Properties map[string]any `json:"properties,omitempty"`
	Children   []OrgaNode     `json:"children,omitempty"`
	Value      string         `json:"value,omitempty"`
	Key        string         `json:"key,omitempty"`
}

// ParseWithOrga shells out to the Node.js orga parser script to parse an org file.
func ParseWithOrga(path string) (*OrgaDocument, error) {
	scriptPath, err := resolveOrgaScriptPath()
	if err != nil {
		return nil, err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve org path: %w", err)
	}

	cmd := exec.Command("node", scriptPath, absPath)
	cmd.Dir = filepath.Dir(scriptPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("orga parse failed: %w: %s", err, strings.TrimSpace(string(output)))
	}

	var doc OrgaDocument
	if err := json.Unmarshal(output, &doc); err != nil {
		return nil, fmt.Errorf("orga json decode failed: %w", err)
	}
	return &doc, nil
}

func resolveOrgaScriptPath() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working dir: %w", err)
	}
	current := cwd
	for {
		candidate := filepath.Join(current, "scripts", "parse-org.js")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return "", errors.New("orga parser script not found")
}
