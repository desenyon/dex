package storage

import (
	"path/filepath"
	"testing"
)

func TestConfigRoundTrip(t *testing.T) {
	root := t.TempDir()
	config := Config{Theme: "dark", Profile: "default", NoColor: true}

	if err := SaveConfig(root, config); err != nil {
		t.Fatalf("SaveConfig returned error: %v", err)
	}
	got, err := LoadConfig(root)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if got.Theme != config.Theme || got.Profile != config.Profile || !got.NoColor {
		t.Fatalf("unexpected config: %#v", got)
	}
}

func TestHistoryAppendAndList(t *testing.T) {
	root := t.TempDir()
	entry := HistoryEntry{Command: "dex system health", Format: "json", OutputPath: filepath.Join(root, "out.json")}
	if err := AppendHistory(root, entry); err != nil {
		t.Fatalf("AppendHistory returned error: %v", err)
	}

	entries, err := ListHistory(root, 10)
	if err != nil {
		t.Fatalf("ListHistory returned error: %v", err)
	}
	if len(entries) != 1 || entries[0].Command != entry.Command || entries[0].Format != entry.Format {
		t.Fatalf("unexpected entries: %#v", entries)
	}
}
