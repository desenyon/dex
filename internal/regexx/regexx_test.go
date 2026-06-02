package regexx

import "testing"

func TestTestPatternReportsMatchesAndGroups(t *testing.T) {
	got, err := TestPattern(`user-(\d+)`, "first user-42 second user-7")
	if err != nil {
		t.Fatalf("TestPattern returned error: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 matches, got %#v", got)
	}
	if got[0].Text != "user-42" || got[0].Groups[0] != "42" {
		t.Fatalf("unexpected first match: %#v", got[0])
	}
	if got[1].Text != "user-7" || got[1].Groups[0] != "7" {
		t.Fatalf("unexpected second match: %#v", got[1])
	}
}

func TestReplaceFindEscapeAndDanger(t *testing.T) {
	replaced, err := Replace(`user-(\d+)`, "id:$1", "user-42")
	if err != nil {
		t.Fatalf("Replace returned error: %v", err)
	}
	if replaced != "id:42" {
		t.Fatalf("unexpected replacement: %q", replaced)
	}

	escaped := Escape("hello.world")
	if escaped != `hello\.world` {
		t.Fatalf("unexpected escape: %q", escaped)
	}

	report, err := Danger(`(a+)+$`)
	if err != nil {
		t.Fatalf("Danger returned error: %v", err)
	}
	if !report.Risky {
		t.Fatalf("expected risky report: %#v", report)
	}
}

func TestExplainAndExamples(t *testing.T) {
	explanation, err := Explain(`^(user|admin)-\d+$`)
	if err != nil {
		t.Fatalf("Explain returned error: %v", err)
	}
	if explanation.Pattern == "" || len(explanation.Nodes) == 0 {
		t.Fatalf("unexpected explanation: %#v", explanation)
	}

	examples, err := Examples(`^\d{3}$`)
	if err != nil {
		t.Fatalf("Examples returned error: %v", err)
	}
	if len(examples.Matching) == 0 || len(examples.NonMatching) == 0 {
		t.Fatalf("unexpected examples: %#v", examples)
	}
}
