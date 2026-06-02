package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestDashboardViewContainsCoreSections(t *testing.T) {
	model := NewModel(Snapshot{
		SystemStatus: "Healthy",
		Connections:  42,
		Processes:    183,
		Collections:  7,
		RecentJSON:   12,
	})

	view := model.View()
	for _, want := range []string{"DEX", "Network", "Processes", "API", "JSON", "System", "Regex", "q quit"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}

func TestNavigationAndPalette(t *testing.T) {
	model := NewModel(Snapshot{})
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	model = updated.(Model)
	if model.Cursor != 1 {
		t.Fatalf("expected cursor 1, got %d", model.Cursor)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	model = updated.(Model)
	if !model.PaletteOpen {
		t.Fatal("expected command palette to open")
	}
}

func TestThemeCycle(t *testing.T) {
	model := NewModel(Snapshot{})
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	model = updated.(Model)
	if model.Theme == "dark" {
		t.Fatal("expected theme to change")
	}
}
