package plan

import "testing"

func TestParseCurrentWorkSingleWip(testingContext *testing.T) {
	content := "* TODO [#A] First\n* WIP [#B] Feature One\n** TODO [#C] Step A\n** WIP [#B] Step B\n"
	work, summary, parseError := ParseCurrentWork(content)
	if parseError != nil {
		testingContext.Fatalf("parse error: %v", parseError)
	}
	if work.L1 != "Feature One" || work.L2 != "Step B" {
		testingContext.Fatalf("unexpected work: %#v", work)
	}
	if summary.WipL1Count != 1 || summary.WipL2Count != 1 {
		testingContext.Fatalf("unexpected summary: %#v", summary)
	}
}

func TestParseCurrentWorkMultipleWipL1(testingContext *testing.T) {
	content := "* WIP First\n** WIP Step A\n* WIP Second\n** WIP Step B\n"
	work, summary, parseError := ParseCurrentWork(content)
	if parseError != nil {
		testingContext.Fatalf("parse error: %v", parseError)
	}
	if work.L1 != "First" || work.L2 != "Step A" {
		testingContext.Fatalf("unexpected work: %#v", work)
	}
	if summary.WipL1Count != 2 {
		testingContext.Fatalf("expected 2 WIP L1 entries, got %d", summary.WipL1Count)
	}
}

func TestParseCurrentWorkNoWip(testingContext *testing.T) {
	content := "* TODO [#A] Feature\n** TODO [#B] Step\n"
	work, summary, parseError := ParseCurrentWork(content)
	if parseError != nil {
		testingContext.Fatalf("parse error: %v", parseError)
	}
	if work.L1 != "" || work.L2 != "" {
		testingContext.Fatalf("expected empty work, got %#v", work)
	}
	if summary.WipL1Count != 0 || summary.WipL2Count != 0 {
		testingContext.Fatalf("unexpected summary: %#v", summary)
	}
}

func TestParseCurrentWorkWipWithoutL2(testingContext *testing.T) {
	content := "* WIP [#A] Feature\n** TODO [#B] Step\n"
	work, summary, parseError := ParseCurrentWork(content)
	if parseError != nil {
		testingContext.Fatalf("parse error: %v", parseError)
	}
	if work.L1 != "Feature" || work.L2 != "" {
		testingContext.Fatalf("unexpected work: %#v", work)
	}
	if summary.WipL1Count != 1 || summary.WipL2Count != 0 {
		testingContext.Fatalf("unexpected summary: %#v", summary)
	}
}
