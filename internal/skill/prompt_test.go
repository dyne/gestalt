package skill

import (
	"path/filepath"
	"testing"
)

func TestGeneratePromptXMLEmpty(t *testing.T) {
	if got := GeneratePromptXML(nil); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestGeneratePromptXMLSingleSkill(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "git-workflows")
	entry := &Skill{
		Name:        "git-workflows",
		Description: "Use git & stay safe",
		Path:        skillDir,
	}

	got := GeneratePromptXML([]*Skill{entry})
	absPath, err := filepath.Abs(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}

	expected := "<available_skills><skill><name>git-workflows</name><description>Use git &amp; stay safe</description><location>" +
		absPath + "</location></skill></available_skills>"
	if got != expected {
		t.Fatalf("xml mismatch:\n%s\n!=\n%s", got, expected)
	}
}
