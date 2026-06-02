package bench

import (
	"context"
	"testing"
)

func TestRunCommand(t *testing.T) {
	result, err := RunCommand(context.Background(), "printf dex")
	if err != nil {
		t.Fatalf("RunCommand returned error: %v", err)
	}
	if result.ExitCode != 0 || result.Stdout != "dex" || result.Duration == "" {
		t.Fatalf("unexpected result: %#v", result)
	}
}
