package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Snapshot struct {
	SystemStatus string `json:"system_status"`
	Connections  int    `json:"connections"`
	Processes    int    `json:"processes"`
	Collections  int    `json:"collections"`
	RecentJSON   int    `json:"recent_json"`
}

type Model struct {
	Sections    []Section
	Cursor      int
	PaletteOpen bool
	Theme       string
	Width       int
	Height      int
}

type Section struct {
	Name   string
	Status string
	Detail []string
}

func NewModel(snapshot Snapshot) Model {
	if snapshot.SystemStatus == "" {
		snapshot.SystemStatus = "Healthy"
	}
	return Model{
		Theme: "dark",
		Sections: []Section{
			{Name: "Network", Status: fmt.Sprintf("%d active connections", snapshot.Connections), Detail: []string{"ip", "ports", "dns", "headers", "ssl"}},
			{Name: "Processes", Status: fmt.Sprintf("%d running", snapshot.Processes), Detail: []string{"list", "tree", "top", "family", "explain-port"}},
			{Name: "API", Status: fmt.Sprintf("%d saved collections", snapshot.Collections), Detail: []string{"get", "post", "schema", "assert", "test-report"}},
			{Name: "JSON", Status: fmt.Sprintf("%d recent files", snapshot.RecentJSON), Detail: []string{"view", "query", "diff", "redact", "fingerprint"}},
			{Name: "System", Status: snapshot.SystemStatus, Detail: []string{"health", "cpu", "memory", "disk", "battery"}},
			{Name: "Regex", Status: "Lab ready", Detail: []string{"test", "explain", "danger", "replace", "benchmark"}},
			{Name: "Benchmark", Status: "Lab ready", Detail: []string{"run", "history", "trend", "export"}},
			{Name: "Files", Status: "Local map ready", Detail: []string{"tree", "size", "largest", "duplicates"}},
			{Name: "Clipboard", Status: "Local history ready", Detail: []string{"save", "search", "export"}},
			{Name: "Terminal", Status: "History ready", Detail: []string{"history", "stats", "aliases", "profile"}},
			{Name: "Settings", Status: "Profile default", Detail: []string{"theme", "profile", "storage"}},
		},
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			if m.PaletteOpen {
				m.PaletteOpen = false
				return m, nil
			}
			_ = SaveSession(m)
			return m, tea.Quit
		case "down", "j":
			if m.Cursor < len(m.Sections)-1 {
				m.Cursor++
			}
		case "up", "k":
			if m.Cursor > 0 {
				m.Cursor--
			}
		case "p", ":":
			m.PaletteOpen = !m.PaletteOpen
		case "t":
			if m.Theme == "dark" {
				m.Theme = "light"
			} else {
				m.Theme = "dark"
			}
		case "home":
			m.Cursor = 0
		case "end":
			m.Cursor = len(m.Sections) - 1
		}
	}
	return m, nil
}

func (m Model) View() string {
	styles := stylesFor(m.Theme)
	title := styles.Title.Render("DEX")
	status := styles.Pill.Render("local first") + " " + styles.Pill.Render("keyboard native") + " " + styles.Pill.Render(m.Theme)

	rows := make([]string, 0, len(m.Sections))
	for i, section := range m.Sections {
		cursor := " "
		style := styles.Row
		if i == m.Cursor {
			cursor = ">"
			style = styles.Selected
		}
		rows = append(rows, style.Render(fmt.Sprintf("%s %-11s %s", cursor, section.Name, section.Status)))
	}
	sidebar := styles.Panel.Width(42).Render(strings.Join(rows, "\n"))

	selected := m.Sections[m.Cursor]
	detailLines := []string{
		styles.Header.Render(selected.Name),
		selected.Status,
		"",
		"Commands",
	}
	for _, item := range selected.Detail {
		detailLines = append(detailLines, "  dex "+strings.ToLower(selected.Name)+" "+item)
	}
	detailLines = append(detailLines, "", "Keys: j/k move  p palette  t theme  q quit")
	detail := styles.Panel.Width(54).Render(strings.Join(detailLines, "\n"))

	body := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, detail)
	if m.PaletteOpen {
		palette := styles.Palette.Render("Command Palette\nnetwork ports\nprocess top\njson lens\nregex lab\nsystem health")
		body = lipgloss.JoinVertical(lipgloss.Left, body, palette)
	}
	return lipgloss.JoinVertical(lipgloss.Left, title+" "+status, body) + "\n"
}

func Run(model Model) error {
	restored, err := LoadSession(model)
	if err == nil {
		model = restored
	}
	_, err = tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	return err
}

func SaveSession(model Model) error {
	path, err := sessionPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(model, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func LoadSession(fallback Model) (Model, error) {
	path, err := sessionPath()
	if err != nil {
		return fallback, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fallback, err
	}
	if err := json.Unmarshal(data, &fallback); err != nil {
		return fallback, err
	}
	if fallback.Cursor >= len(fallback.Sections) {
		fallback.Cursor = 0
	}
	return fallback, nil
}

func sessionPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".dex")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "session.json"), nil
}

type styleSet struct {
	Title    lipgloss.Style
	Header   lipgloss.Style
	Panel    lipgloss.Style
	Row      lipgloss.Style
	Selected lipgloss.Style
	Pill     lipgloss.Style
	Palette  lipgloss.Style
}

func stylesFor(theme string) styleSet {
	if theme == "light" {
		return styleSet{
			Title:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("57")),
			Header:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("57")),
			Panel:    lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("244")).Padding(1, 2).MarginRight(1),
			Row:      lipgloss.NewStyle().Foreground(lipgloss.Color("238")),
			Selected: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("57")),
			Pill:     lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Background(lipgloss.Color("57")).Padding(0, 1),
			Palette:  lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("57")).Padding(1, 2),
		}
	}
	return styleSet{
		Title:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86")),
		Header:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86")),
		Panel:    lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("240")).Padding(1, 2).MarginRight(1),
		Row:      lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
		Selected: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86")),
		Pill:     lipgloss.NewStyle().Foreground(lipgloss.Color("16")).Background(lipgloss.Color("86")).Padding(0, 1),
		Palette:  lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("86")).Padding(1, 2),
	}
}
