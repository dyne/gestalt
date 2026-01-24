package skill

import "testing"

func TestGeneratePromptXMLEmpty(t *testing.T) {
	if got := GeneratePromptXML(nil); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestGeneratePromptXMLSingleSkill(t *testing.T) {
	entry := &Skill{
		Name:        "git-workflows",
		Description: "Use \"git\" & stay <safe> 'always'",
		Path:        "config/skills/git-workflows",
	}

	got := GeneratePromptXML([]*Skill{entry})

	expected := `<available_skills>
  <skill>
    <name>git-workflows</name>
    <description>Use &#34;git&#34; &amp; stay &lt;safe&gt; &#39;always&#39;</description>
  </skill>
</available_skills>`
	if got != expected {
		t.Fatalf("xml mismatch:\ngot:\n%s\nexpected:\n%s", got, expected)
	}
}

func TestGeneratePromptXMLWithMetadata(t *testing.T) {
	entry := &Skill{
		Name:          "git-workflows",
		Description:   "Use git safely",
		License:       "MIT",
		Compatibility: ">=1.0",
		AllowedTools:  []string{"bash", "git"},
		Path:          "config/skills/git-workflows",
	}

	got := GeneratePromptXML([]*Skill{entry})

	expected := `<available_skills>
  <skill>
    <name>git-workflows</name>
    <description>Use git safely</description>
    <license>MIT</license>
    <compatibility>&gt;=1.0</compatibility>
    <allowed_tools>
      <tool>bash</tool>
      <tool>git</tool>
    </allowed_tools>
  </skill>
</available_skills>`
	if got != expected {
		t.Fatalf("xml mismatch:\ngot:\n%s\nexpected:\n%s", got, expected)
	}
}

func TestGeneratePromptXMLMultipleSkills(t *testing.T) {
	skills := []*Skill{
		{
			Name:        "terminal-navigation",
			Description: "Terminal navigation shortcuts and safe command patterns.",
			Path:        "config/skills/terminal-navigation",
		},
		{
			Name:        "code-review",
			Description: "Code review best practices",
			Path:        "config/skills/code-review",
		},
	}

	got := GeneratePromptXML(skills)

	expected := `<available_skills>
  <skill>
    <name>terminal-navigation</name>
    <description>Terminal navigation shortcuts and safe command patterns.</description>
  </skill>
  <skill>
    <name>code-review</name>
    <description>Code review best practices</description>
  </skill>
</available_skills>`
	if got != expected {
		t.Fatalf("xml mismatch:\ngot:\n%s\nexpected:\n%s", got, expected)
	}
}

func TestGeneratePromptXMLSkipInvalidSkills(t *testing.T) {
	skills := []*Skill{
		nil,
		{Name: "", Description: "No name"},
		{Name: "valid", Description: "", Path: "/tmp/valid"},
		{Name: "good", Description: "Good skill", Path: "/tmp/good"},
	}

	got := GeneratePromptXML(skills)

	expected := `<available_skills>
  <skill>
    <name>good</name>
    <description>Good skill</description>
  </skill>
</available_skills>`
	if got != expected {
		t.Fatalf("xml mismatch:\ngot:\n%s\nexpected:\n%s", got, expected)
	}
}
