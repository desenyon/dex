package bench

import (
	"context"
	"os/exec"
	"time"
)

type Result struct {
	Command  string `json:"command"`
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	Duration string `json:"duration"`
}

func RunCommand(ctx context.Context, command string) (Result, error) {
	start := time.Now()
	cmd := exec.CommandContext(ctx, "sh", "-lc", command)
	stdout, err := cmd.Output()
	result := Result{
		Command:  command,
		Stdout:   string(stdout),
		Duration: time.Since(start).Round(time.Millisecond).String(),
	}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			result.Stderr = string(exitErr.Stderr)
			return result, nil
		}
		return result, err
	}
	return result, nil
}
