package output

import (
	"strings"
	"testing"
)

func TestRenderJSON(t *testing.T) {
	got, err := Render(Options{Format: JSON}, []Record{{"name": "Dex", "status": "ok"}})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	if !strings.Contains(got, `"name": "Dex"`) || !strings.Contains(got, `"status": "ok"`) {
		t.Fatalf("JSON output missing fields:\n%s", got)
	}
}

func TestRenderCSV(t *testing.T) {
	got, err := Render(Options{Format: CSV}, []Record{
		{"name": "Dex", "status": "ok"},
		{"name": "Phase 1", "status": "ready"},
	})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	want := "name,status\nDex,ok\nPhase 1,ready\n"
	if got != want {
		t.Fatalf("CSV output mismatch\nwant:\n%q\ngot:\n%q", want, got)
	}
}

func TestRenderTextTable(t *testing.T) {
	got, err := Render(Options{Format: Text}, []Record{{"module": "network", "command": "ip"}})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	if !strings.Contains(got, "MODULE") || !strings.Contains(got, "network") || !strings.Contains(got, "ip") {
		t.Fatalf("text table output missing expected cells:\n%s", got)
	}
}

func TestRenderRawAddsTerminalNewline(t *testing.T) {
	got, err := Render(Options{Format: Raw}, "Ada")
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if got != "Ada\n" {
		t.Fatalf("unexpected raw output: %q", got)
	}
}

func TestRenderRawFormatsStructuredValuesAsJSON(t *testing.T) {
	got, err := Render(Options{Format: Raw}, Record{"status": "healthy"})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if !strings.Contains(got, `"status": "healthy"`) {
		t.Fatalf("unexpected raw structured output: %s", got)
	}
}
