package desktopui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"aether-dots/internal/model"
)

const (
	preferencesFile = "preferences.json"
	openAIKeyName   = "openai-api-key"
	openAIService   = "com.aetherdots.desktop"
)

type preferences struct {
	Model      string `json:"model"`
	Appearance string `json:"appearance"`
	Palette    string `json:"palette"`
}

func defaultPreferences() preferences {
	return preferences{
		Model:      model.FromEnvironment().Model,
		Appearance: "System",
		Palette:    "violet",
	}
}

func loadPreferences(dataDir string) (preferences, error) {
	current := defaultPreferences()
	b, err := os.ReadFile(filepath.Join(dataDir, preferencesFile))
	if errors.Is(err, os.ErrNotExist) {
		return current, nil
	}
	if err != nil {
		return preferences{}, fmt.Errorf("read settings: %w", err)
	}
	if err := json.Unmarshal(b, &current); err != nil {
		return preferences{}, fmt.Errorf("decode settings: %w", err)
	}
	if strings.TrimSpace(current.Model) == "" {
		current.Model = model.DefaultModel
	}
	if !validAppearance(current.Appearance) {
		current.Appearance = "System"
	}
	if !validPalette(current.Palette) {
		current.Palette = "violet"
	}
	return current, nil
}

func savePreferences(dataDir string, current preferences) error {
	current.Model = strings.TrimSpace(current.Model)
	if current.Model == "" {
		return errors.New("OpenAI model name is required")
	}
	if !validAppearance(current.Appearance) || !validPalette(current.Palette) {
		return errors.New("settings contain an unsupported appearance or color theme")
	}
	b, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	tmp, err := os.CreateTemp(dataDir, ".preferences-*.tmp")
	if err != nil {
		return fmt.Errorf("create settings file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("protect settings file: %w", err)
	}
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write settings: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync settings: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close settings: %w", err)
	}
	if err := os.Rename(tmpPath, filepath.Join(dataDir, preferencesFile)); err != nil {
		return fmt.Errorf("replace settings: %w", err)
	}
	return nil
}

func validAppearance(value string) bool {
	switch value {
	case "System", "Light", "Dark":
		return true
	default:
		return false
	}
}

func validPalette(value string) bool {
	switch value {
	case "obsidian", "forest", "ocean", "silver", "violet", "rose", "amber":
		return true
	default:
		return false
	}
}
