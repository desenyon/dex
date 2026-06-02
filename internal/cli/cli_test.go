package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestJSONViewCommand(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")
	if err := os.WriteFile(path, []byte(`{"name":"Dex"}`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	var out bytes.Buffer
	root := NewRoot(&out, &out)
	root.SetArgs([]string{"json", "view", path, "--raw"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if !strings.Contains(out.String(), `"name": "Dex"`) {
		t.Fatalf("json view output missing formatted field:\n%s", out.String())
	}
}

func TestRegexCommand(t *testing.T) {
	var out bytes.Buffer
	root := NewRoot(&out, &out)
	root.SetArgs([]string{"regex", "test", `user-(\d+)`, "user-42", "--json"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if !strings.Contains(out.String(), `"text": "user-42"`) || !strings.Contains(out.String(), `"groups"`) {
		t.Fatalf("regex output missing match details:\n%s", out.String())
	}
}

func TestRootNoArgsRendersDashboardForNonTerminal(t *testing.T) {
	var out bytes.Buffer
	root := NewRoot(&out, &out)
	root.SetArgs([]string{"--json"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if !strings.Contains(out.String(), `"name": "dashboard"`) {
		t.Fatalf("dashboard output missing:\n%s", out.String())
	}
}

func TestVersionCommandUsesBuildInfo(t *testing.T) {
	var out bytes.Buffer
	root := NewRoot(&out, &out, BuildInfo{Version: "v-test", Commit: "abc123"})
	root.SetArgs([]string{"version", "--json"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if !strings.Contains(out.String(), `"version": "v-test"`) || !strings.Contains(out.String(), `"commit": "abc123"`) {
		t.Fatalf("version output missing build info:\n%s", out.String())
	}
}

func TestWatchWrapperRepeatsUntilContextDone(t *testing.T) {
	options := &globalOptions{watch: true, interval: time.Millisecond}
	count := 0
	cmd := &cobra.Command{
		Use: "watch-test",
		RunE: func(cmd *cobra.Command, args []string) error {
			count++
			return nil
		},
	}
	wrapWatch(cmd, options)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	cmd.SetContext(ctx)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if count < 2 {
		t.Fatalf("expected repeated execution, got %d", count)
	}
}

func TestOutputFormatsAreMutuallyExclusive(t *testing.T) {
	var out bytes.Buffer
	root := NewRoot(&out, &out)
	root.SetArgs([]string{"system", "info", "--json", "--csv"})

	if err := root.Execute(); err == nil {
		t.Fatal("expected mutually exclusive output format error")
	}
}

func TestNetworkSubcommandsExist(t *testing.T) {
	commands := [][]string{
		{"network", "interfaces", "--help"},
		{"network", "hostname", "--help"},
		{"network", "routes", "--help"},
		{"network", "dns", "--help"},
		{"network", "headers", "--help"},
		{"network", "ssl", "--help"},
	}

	for _, args := range commands {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out bytes.Buffer
			root := NewRoot(&out, &out)
			root.SetArgs(args)
			if err := root.Execute(); err != nil {
				t.Fatalf("Execute returned error: %v", err)
			}
		})
	}
}

func TestProcessSubcommandsExist(t *testing.T) {
	commands := [][]string{
		{"process", "search", "--help"},
		{"process", "tree", "--help"},
		{"process", "children", "--help"},
		{"process", "parent", "--help"},
		{"process", "ancestry", "--help"},
		{"process", "top", "--help"},
		{"process", "sockets", "--help"},
		{"process", "explain-port", "--help"},
	}

	for _, args := range commands {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out bytes.Buffer
			root := NewRoot(&out, &out)
			root.SetArgs(args)
			if err := root.Execute(); err != nil {
				t.Fatalf("Execute returned error: %v", err)
			}
		})
	}
}

func TestJSONSubcommandsExist(t *testing.T) {
	commands := [][]string{
		{"json", "format", "--help"},
		{"json", "minify", "--help"},
		{"json", "validate", "--help"},
		{"json", "query", "--help"},
		{"json", "flatten", "--help"},
		{"json", "keys", "--help"},
		{"json", "paths", "--help"},
		{"json", "types", "--help"},
		{"json", "redact", "--help"},
		{"json", "fingerprint", "--help"},
	}

	for _, args := range commands {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out bytes.Buffer
			root := NewRoot(&out, &out)
			root.SetArgs(args)
			if err := root.Execute(); err != nil {
				t.Fatalf("Execute returned error: %v", err)
			}
		})
	}
}

func TestRegexSubcommandsExist(t *testing.T) {
	commands := [][]string{
		{"regex", "explain", "--help"},
		{"regex", "find", "--help"},
		{"regex", "replace", "--help"},
		{"regex", "groups", "--help"},
		{"regex", "escape", "--help"},
		{"regex", "unescape", "--help"},
		{"regex", "danger", "--help"},
		{"regex", "examples", "--help"},
		{"regex", "visual", "--help"},
	}

	for _, args := range commands {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out bytes.Buffer
			root := NewRoot(&out, &out)
			root.SetArgs(args)
			if err := root.Execute(); err != nil {
				t.Fatalf("Execute returned error: %v", err)
			}
		})
	}
}

func TestSystemSubcommandsExist(t *testing.T) {
	commands := [][]string{
		{"system", "dashboard", "--help"},
		{"system", "uptime", "--help"},
		{"system", "profile", "--help"},
		{"system", "snapshot", "--help"},
		{"system", "report", "--help"},
		{"system", "cpu", "--help"},
		{"system", "memory", "--help"},
		{"system", "disk", "--help"},
		{"system", "battery", "--help"},
		{"system", "power", "--help"},
		{"system", "thermal", "--help"},
		{"system", "startup", "--help"},
		{"system", "services", "--help"},
		{"system", "permissions", "--help"},
		{"system", "env", "--help"},
		{"system", "path", "--help"},
		{"system", "shell", "--help"},
		{"system", "terminal", "--help"},
		{"system", "score", "--help"},
	}

	for _, args := range commands {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out bytes.Buffer
			root := NewRoot(&out, &out)
			root.SetArgs(args)
			if err := root.Execute(); err != nil {
				t.Fatalf("Execute returned error: %v", err)
			}
		})
	}
}

func TestSettingsSubcommandsExist(t *testing.T) {
	commands := [][]string{
		{"settings", "show", "--help"},
		{"settings", "theme", "--help"},
		{"settings", "profile", "--help"},
		{"settings", "history", "--help"},
		{"settings", "storage", "--help"},
	}

	for _, args := range commands {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out bytes.Buffer
			root := NewRoot(&out, &out)
			root.SetArgs(args)
			if err := root.Execute(); err != nil {
				t.Fatalf("Execute returned error: %v", err)
			}
		})
	}
}
