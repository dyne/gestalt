package terminal

import (
	"strings"
	"testing"
)

func decodeSample(sample string) string {
	replacer := strings.NewReplacer(
		"\\x1b", "\x1b",
		"\\r", "\r",
		"\\n", "\n",
		"\\t", "\t",
		"\\a", "\a",
	)
	return replacer.Replace(sample)
}

func TestStripANSIRemovesSequencesAndControlCodes(t *testing.T) {
	input := "hello\x1b[31mred\x1b[0m\x07world"
	got := StripANSI(input)
	if got != "helloredworld" {
		t.Fatalf("expected %q, got %q", "helloredworld", got)
	}
}

func TestStripANSIPreservesWhitespaceControls(t *testing.T) {
	input := "line1\tline2\nline3\rline4\x00"
	got := StripANSI(input)
	if got != "line1\tline2\nline3\rline4" {
		t.Fatalf("expected whitespace preserved, got %q", got)
	}
}

func TestStripRepeatedChars(t *testing.T) {
	input := "start-----end"
	got := StripRepeatedChars(input, 3)
	if got != "start-end" {
		t.Fatalf("expected collapsed run, got %q", got)
	}
	if StripRepeatedChars("short--", 3) != "short--" {
		t.Fatalf("expected short runs unchanged")
	}
}

func TestStripRepeatedCharsUTF8(t *testing.T) {
	input := "cafééé"
	got := StripRepeatedChars(input, 3)
	if got != "café" {
		t.Fatalf("expected UTF-8 run collapsed, got %q", got)
	}
}

func TestFilterTerminalOutputMixed(t *testing.T) {
	input := "ok\x1b[32mgreen\x1b[0m\n-----\x00done"
	got := FilterTerminalOutput(input)
	if strings.Contains(got, "\x1b") {
		t.Fatalf("expected ANSI stripped, got %q", got)
	}
	if strings.Contains(got, "-----") {
		t.Fatalf("expected repeated chars collapsed, got %q", got)
	}
	if !strings.Contains(got, "green") || !strings.Contains(got, "done") {
		t.Fatalf("expected content preserved, got %q", got)
	}
}

func TestFilterTerminalOutputRealWorldBellSample(t *testing.T) {
	sample := decodeSample(`2026/01/25 06:48:39 level=info msg="temporal bell recorded" context="\r\n   \x1b[97m  - \x1b[1mDraft new L1 sections\x1b[22m with detailed L2 steps for features you want to add\x1b[39m\r\n   \x1b[97m  - \x1b[1mExtend existing L1 sections\x1b[22m with additional L2 tasks\x1b[39m\r\n   \x1b[97m  - \x1b[1mRefine L2 tasks\x1b[22m with more architectural detail\x1b[39m\r\n   \x1b[97m  - \x1b[1mAsk clarifying questions\x1b[22m before committing architectural decisions to the PLAN\x1b[39m\r\n\r\n   \x1b[1m\x1b[97mWhat I need from you:\x1b[39m\x1b[22m\r\n\r\n   \x1b[97mTell me what feature or change you want to plan. I'll:\x1b[39m\r\n\r\n   \x1b[97m  - Research the codebase (using code search when relevant\x1b[39m\r\n\r\n \x1b[35m◉ Rea\x1b[95md\x1b[35ming current PLAN\x1b[39m \x1b[37m(Esc to cancel · 1.3 KiB)\x1b[39m\r\n\r\n ~/devel/gestalt\x1b[37m[⎇ main*]\x1b[39m
   \x1b[37mclaude-sonnet-4.5 (1x)\x1b[39m\r\n \x1b[37m──────────────────────────────────────────────────────────────────────────────────────────
──────────────────────────────────────────────────────────────────────────────────────────────────────\x1b[39m\r\n \x1b[37m> \x1b[39m\r\n   \x1b[7m \x1b[27m\r\n \x1b[37m───────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
─────────────────────────────────────────────────────────────────────────\x1b[39m\r\n \x1b[1m\x1b[97mCtrl+c\x1b[22m\x1b[37m Exit\x1b[39m \x1b[37m·\x1b[39m \x1b[1m\x1b[97mCtrl+r\x1b[22m\x1b[37m Expand recent\x1b[39m
                                                             \x1b[37mRemaining requests: 58%\x1b[39m\r\n\u200d\r\n\n\x1b[2K\x1b[1A\x1b[2K\x1b[1A
\x1b[2K\x1b[1A\x1b[2K\x1b[1A\x1b[2K\x1b[1A\x1b[2K\x1b[1A\x1b[2K\x1b[1A\x1b[2K\x1b[1A\x1b[2K\x1b[1A\x1b[2K\x1b[1A\x1b[2K\x1b[1A\x1b[2K\x1b[1A\x1b[2K\x1b[1A\x1b[2K\x1b[1A\x1b[2K\x1b[1A\x1b[2K\x1b[1A\x1b[2K\x1b[1A\x1b[2K\x1b[1A\x1b[2K\x1b[1A\x1b[2K\x1b[1A\x1b[2K\x1b[1A\x1b[2K\x1b[1A\x1b[2K\x1b[1A\x1b[2K\x1b[G \x1b[35m●\x1b[39m \x1b[97mGood! I can see the current PLAN structure. Now I understand your project and\x1b[39m\r\n   \x1b[97mworkflow. I'm ready to help you draft new L1 features or extend existing ones in\x1b[39m\r\n   \x1b[36m.gestalt/PLAN.org\x1b[97m.\x1b[39m\r\n\r\n   \x1b[1m\x1b[97mWhat I can do for you:\x1b[39m\x1b[22m\r\n\r\n   \x1b[97m  - \x1b[1mDraft new L1 sections\x1b[22m with detailed L2 steps for features you want to add\x1b[39m\r\n   \x1b[97m  - \x1b[1mExtend existing L1 sections\x1b[22m with additional L2 tasks\x1b[39m\r\n   \x1b[97m  - \x1b[1mRefine L2 tasks\x1b[22m with more architectural detail\x1b[39m\r\n   \x1b[97m  - \x1b[1mAsk clarifying questions\x1b[22m before committing architectural decisions to the PLAN\x1b[39m\r\n\r\n   \x1b[1m\x1b[97mWhat I need from you:\x1b[39m\x1b[22m\r\n\r\n   \x1b[97mTell me what feature or change you want to plan. I'll:\x1b[39m\r\n\r\n   \x1b[97m  - Research the codebase (using code search when relevant\x1b[39m\r\n\r\n \x1b[35m◉ Read\x1b[95mi\x1b[35mng current PLAN\x1b[39m \x1b[37m(Esc to cancel · 1.3 KiB)\x1b[39m\r\n\r\n ~/
devel/gestalt\x1b[37m[⎇ main*]\x1b[39m
                                       \x1b[37mclaude-sonnet-4.5 (1x)\x1b[39m\r\n \x1b[37m──────────────────────────────────────────────────────
──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────\x1b[39m\r\n \x1b[37m> \x1b[39m\r\n   \x1b[7m \x1b[27m\r\n \x1b[37m───────────────────────────────────────────────────────────────────────────────────
─────────────────────────────────────────────────────────────────────────────────────────────────────────────\x1b[39m\r\n \x1b[1m\x1b[97mCtrl+c\x1b[22m\x1b[37m Exit\x1b[39m \x1b[37m·\x1b[39m \x1b[1m\x1b[97mCtrl+r\x1b[22m\x1b[37m Expand recent\x1b[39m
                                                                                                 \x1b[37mRemaining requests: 58%\x1b[39m\r\n\u200d\r\n\n\a" terminal_id="2" timestamp="2026-01-25T05:48:39Z"`)

	cleaned := FilterTerminalOutput(sample)
	if strings.Contains(cleaned, "\x1b") {
		t.Fatalf("expected ANSI stripped, got %q", cleaned)
	}
	if strings.Contains(cleaned, "────────────────") {
		t.Fatalf("expected repeated chars collapsed, got %q", cleaned)
	}
	if !strings.Contains(cleaned, "temporal bell recorded") {
		t.Fatalf("expected key text preserved, got %q", cleaned)
	}
	if !strings.Contains(cleaned, "Draft new L1 sections") {
		t.Fatalf("expected content preserved, got %q", cleaned)
	}
}

func TestFilterTerminalOutputRealWorldAnsiBurst(t *testing.T) {
	sample := decodeSample(`b[0m\x1b[48;2;45;45;54m\x1b[K\x1b[73;2H\x1b[0m\x1b[48;2;45;45;54m\x1b[K\x1b[74;37H\x1b[0m\x1b[49m\x1b[K\x1b[39m\x1b[49m\x1b[0m\x1b[?25h\x1b[72;3H\x1b[?2026l\x1b[?2026h\x1b[68;2H\x1b[0m\x1b[49m\x1b[K\x1b[69;67H\x1b[0m\x1b[49m\x1b[K\x1b[70;2H\x1b[0m\x1b[49m\x1b[K\x1b[71;2H\x1b[0m\x1b[48;2;45;45;54m\x1b[K\x1b[72;24H\x1b[0m\x1b[48;2;45;45;54m\x1b[K\x1b[73;2H\x1b[0m\x1b[48;2;45;45;54m\x1b[K\x1b[74;37H\x1b[0m\x1b[49m\x1b[K\x1b[39m\x1b[49m\x1b[0m\x1b[?25h\x1b[72;3H\x1b[?2026l\x1b[?2026h\x1b[68;2H\x1b[0m\x1b[49m\x1b[K\x1b[69;67H\x1b[0m\x1b[49m\x1b[K\x1b[70;2H\x1b[0m\x1b[49m\x1b[K\x1b[71;2H\x1b[0m\x1b[48;2;45;45;54m\x1b[K\x1b[72;24H\x1b[0m\x1b[48;2;45;45;54m\x1b[K\x1b[73;2H\x1b[0m\x1b[48;2;45;45;54m\x1b[K\x1b[74;37H\x1b[0m\x1b[49m\x1b[K\x1b[39m\x1b[49m\x1b[0m\x1b[?25h\x1b[72;3H\x1b[?2026l\x1b[?2026h\x1b[68;2H\x1b[0m\x1b[49m\x1b[K\x1b[69;67H\x1b[0m\x1b[49m\x1b[K\x1b[70;2H\x1b[0m\x1b[49m\x1b[K\x1b[71;2H\x1b[0m\x1b[48;2;45;45;54m\x1b[K\x1b[72;24H\x1b[0m\x1b[48;2;45;45;54m\x1b[K\x1b[73;2H\x1b[0m\x1b[48;2;45;45;54m\x1b[K\x1b[74;37H\x1b[0m\x1b[49m\x1b[K\x1b[39m\x1b[49m\x1b[0m\x1b[?25h\x1b[72;3H\x1b[?2026l\x1b[?2026h\x1b[68;2H\x1b[0m\x1b[49m\x1b[K\x1b[69;67H\x1b[0m\x1b[49m\x1b[K\x1b[70;2H\x1b[0m\x1b[49m\x1b[K\x1b[71;2H\x1b[0m\x1b[48;2;45;45;54m\x1b[K\x1b[72;24H\x1b[0m\x1b[48;2;45;45;54m\x1b[K\x1b[73;2H\x1b[0m\x1b[48;2;45;45;54m\x1b[K\x1b[74;37H\x1b[0m\x1b[49m\x1b[K\x1b[39m\x1b[49m\x1b[0m\x1b[?25h\x1b[72;3H\x1b[?2026l\x1b[?2026h\x1b[68;2H\x1b[0m\x1b[49m\x1b[K\x1b[69;67H\x1b[0m\x1b[49m\x1b[K\x1b[70;2H\x1b[0m\x1b[49m\x1b[K\x1b[71;2H\x1b[0m\x1b[48;2;45;45;54m\x1b[K\x1b[72;24H\x1b[0m\x1b[48;2;45;45;54m\x1b[K\x1b[73;2H\x1b[0m\x1b[48;2;45;45;54m\x1b[K\x1b[74;37H\x1b[0m\x1b[49m\x1b[K\x1b[39m\x1b[49m\x1b[0m\x1b[?25h\x1b[72;3H\x1b[?2026l\x1b[?2026h\x1b[68;1H\x1b[J\x1b[68;74r\x1b[68;1H\x1bM\x1bM\x1b[r\x1b[1;69r\x1b[67;1H\r\n\n\x1b[39;49m\x1b[K\x1b[39m\x1b[49m\x1b[0m\r\n\n\x1b[39;49m\x1b[K\x1b[38;5;1;49m■ Conversation interrupted - tell the model what to do differently. Something went wrong? Hit /feedback to report the issue.\x1b[39m\x1b[49m\x1b[0m\x1b[r\x1b[72;3H\x1b[70;2H\x1b[0m\x1b[49m\x1b[K\x1b[71;2H\x1b[0m\x1b[48;2;45;45;54m\x1b[K\x1b[72;24H\x1b[0m\x1b[48;2;45;45;54m\x1b[K\x1b[73;2H\x1b[0m\x1b[48;2;45;45;54m\x1b[K\x1b[74;37H\x1b[0m\x1b[49m\x1b[K\x1b[39m\x1b[49m\x1b[0m\x1b[?25h\x1b[72;3H\x1b[?2026l" terminal_id="1"`)

	cleaned := FilterTerminalOutput(sample)
	if strings.Contains(cleaned, "\x1b") {
		t.Fatalf("expected ANSI stripped, got %q", cleaned)
	}
	if !strings.Contains(cleaned, "Conversation interrupted") {
		t.Fatalf("expected message preserved, got %q", cleaned)
	}
	if strings.Contains(cleaned, "■■■") {
		t.Fatalf("expected repeated chars collapsed, got %q", cleaned)
	}
}
