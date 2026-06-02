package process

import "testing"

func TestParsePSList(t *testing.T) {
	input := `  101     1 0.0 0.1 /sbin/launchd
  202   101 1.2 0.3 Core Audio Driver
`

	got := parsePSList(input)
	if len(got) != 2 {
		t.Fatalf("expected two processes, got %#v", got)
	}
	if got[1].PID != 202 || got[1].PPID != 101 || got[1].Command != "Core Audio Driver" || got[1].CPU != "1.2" || got[1].Memory != "0.3" {
		t.Fatalf("unexpected process: %#v", got[1])
	}
}

func TestParsePSInspect(t *testing.T) {
	input := `  202   101 1.2 0.3 00:00:10 Core Audio Driver
`

	got := parsePSList(input)
	if len(got) != 1 {
		t.Fatalf("expected one process, got %#v", got)
	}
	if got[0].Elapsed != "00:00:10" || got[0].Command != "Core Audio Driver" {
		t.Fatalf("unexpected inspected process: %#v", got[0])
	}
}

func TestRelationships(t *testing.T) {
	processes := []Process{
		{PID: 1, PPID: 0, Command: "launchd"},
		{PID: 10, PPID: 1, Command: "shell"},
		{PID: 11, PPID: 10, Command: "node"},
		{PID: 12, PPID: 10, Command: "go"},
	}

	children := ChildrenOf(processes, 10)
	if len(children) != 2 || children[0].PID != 11 || children[1].PID != 12 {
		t.Fatalf("unexpected children: %#v", children)
	}

	ancestry := AncestryOf(processes, 11)
	if len(ancestry) != 3 || ancestry[0].PID != 11 || ancestry[1].PID != 10 || ancestry[2].PID != 1 {
		t.Fatalf("unexpected ancestry: %#v", ancestry)
	}
}

func TestSearch(t *testing.T) {
	processes := []Process{
		{PID: 10, Command: "node server.js"},
		{PID: 11, Command: "go test"},
	}

	got := Search(processes, "NODE")
	if len(got) != 1 || got[0].PID != 10 {
		t.Fatalf("unexpected search result: %#v", got)
	}
}
