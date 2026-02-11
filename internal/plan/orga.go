package plan

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	Position   *OrgaPosition  `json:"position,omitempty"`
}

type OrgaPosition struct {
	Start OrgaLocation `json:"start"`
	End   OrgaLocation `json:"end"`
}

type OrgaLocation struct {
	Offset int `json:"offset"`
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
	starts := []string{}
	if cwd, err := os.Getwd(); err == nil {
		starts = append(starts, cwd)
	}
	if executable, err := os.Executable(); err == nil {
		starts = append(starts, filepath.Dir(executable))
	}
	if _, file, _, ok := runtime.Caller(0); ok {
		starts = append(starts, filepath.Dir(filepath.Dir(file)))
	}

	for _, start := range starts {
		if candidate, ok := findScriptPath(start); ok {
			return candidate, nil
		}
	}
	return "", errors.New("orga parser script not found")
}

func findScriptPath(start string) (string, bool) {
	current := start
	for {
		candidate := filepath.Join(current, "scripts", "parse-org.js")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, true
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", false
		}
		current = parent
	}
}
