package storage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Theme   string `json:"theme"`
	Profile string `json:"profile"`
	NoColor bool   `json:"no_color"`
}

type HistoryEntry struct {
	Time       time.Time `json:"time"`
	Command    string    `json:"command"`
	Format     string    `json:"format"`
	OutputPath string    `json:"output_path,omitempty"`
}

func Root() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	root := filepath.Join(home, ".dex")
	if err := Ensure(root); err != nil {
		return "", err
	}
	return root, nil
}

func Ensure(root string) error {
	for _, path := range []string{
		root,
		filepath.Join(root, "snapshots"),
		filepath.Join(root, "api"),
		filepath.Join(root, "themes"),
		filepath.Join(root, "benchmarks"),
		filepath.Join(root, "clipboard"),
		filepath.Join(root, "terminal"),
	} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func LoadConfig(root string) (Config, error) {
	data, err := os.ReadFile(filepath.Join(root, "config.toml"))
	if err != nil {
		if os.IsNotExist(err) {
			return Config{Theme: "dark", Profile: "default"}, nil
		}
		return Config{}, err
	}
	config := Config{Theme: "dark", Profile: "default"}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"`)
		switch key {
		case "theme":
			config.Theme = value
		case "profile":
			config.Profile = value
		case "no_color":
			config.NoColor = value == "true"
		}
	}
	return config, nil
}

func SaveConfig(root string, config Config) error {
	if err := Ensure(root); err != nil {
		return err
	}
	data := fmt.Sprintf("theme = %q\nprofile = %q\nno_color = %s\n", config.Theme, config.Profile, strconv.FormatBool(config.NoColor))
	return os.WriteFile(filepath.Join(root, "config.toml"), []byte(data), 0o600)
}

func AppendHistory(root string, entry HistoryEntry) error {
	if err := Ensure(root); err != nil {
		return err
	}
	if entry.Time.IsZero() {
		entry.Time = time.Now()
	}
	file, err := os.OpenFile(filepath.Join(root, "history.db"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

func ListHistory(root string, limit int) ([]HistoryEntry, error) {
	file, err := os.Open(filepath.Join(root, "history.db"))
	if err != nil {
		if os.IsNotExist(err) {
			return []HistoryEntry{}, nil
		}
		return nil, err
	}
	defer file.Close()
	var entries []HistoryEntry
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry HistoryEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err == nil {
			entries = append(entries, entry)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if limit > 0 && len(entries) > limit {
		return entries[len(entries)-limit:], nil
	}
	return entries, nil
}
