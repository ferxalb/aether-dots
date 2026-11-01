package desktopui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"aether-dots/internal/model"
)

const (
	preferencesFile = "preferences.json"
	openAIKeyName   = "openai-api-key"
	openAIService   = "com.aetherdots.desktop"
)

type preferences struct {
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoning_effort"`
	TextVerbosity   string `json:"text_verbosity"`
	MaxOutputTokens string `json:"max_output_tokens"`
	MaxRetries      string `json:"max_retries"`
	TimeoutSeconds  string `json:"timeout_seconds"`
	Appearance      string `json:"appearance"`
	Palette         string `json:"palette"`
}

func defaultPreferences() preferences {
	config := model.FromEnvironment()
	current := preferences{
		Model:           config.Model,
		ReasoningEffort: model.DefaultReasoningEffort,
		TextVerbosity:   model.DefaultTextVerbosity,
		MaxOutputTokens: strconv.FormatInt(model.DefaultMaxOutputTokens, 10),
		MaxRetries:      strconv.Itoa(model.DefaultMaxRetries),
		TimeoutSeconds:  "90",
		Appearance:      "System",
		Palette:         "obsidian",
	}
	if validReasoningEffort(config.ReasoningEffort) {
		current.ReasoningEffort = config.ReasoningEffort
	}
	if validTextVerbosity(config.TextVerbosity) {
		current.TextVerbosity = config.TextVerbosity
	}
	if validOutputTokens(strconv.FormatInt(config.MaxOutputTokens, 10)) {
		current.MaxOutputTokens = strconv.FormatInt(config.MaxOutputTokens, 10)
	}
	if config.MaxRetries >= 0 && config.MaxRetries <= 5 {
		current.MaxRetries = strconv.Itoa(config.MaxRetries)
	}
	if config.Timeout > 0 {
		seconds := int64(config.Timeout / time.Second)
		if validTimeout(strconv.FormatInt(seconds, 10)) {
			current.TimeoutSeconds = strconv.FormatInt(seconds, 10)
		}
	}
	return current
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
	if !validReasoningEffort(current.ReasoningEffort) {
		current.ReasoningEffort = model.DefaultReasoningEffort
	}
	if !validTextVerbosity(current.TextVerbosity) {
		current.TextVerbosity = model.DefaultTextVerbosity
	}
	if !validOutputTokens(current.MaxOutputTokens) {
		current.MaxOutputTokens = "8192"
	}
	if !validRetryCount(current.MaxRetries) {
		current.MaxRetries = "2"
	}
	if !validTimeout(current.TimeoutSeconds) {
		current.TimeoutSeconds = "90"
	}
	if !validAppearance(current.Appearance) {
		current.Appearance = "System"
	}
	if !validPalette(current.Palette) {
		current.Palette = "obsidian"
	}
	return current, nil
}

func savePreferences(dataDir string, current preferences) error {
	current.Model = strings.TrimSpace(current.Model)
	if current.Model == "" {
		return errors.New("OpenAI model name is required")
	}
	if !validReasoningEffort(current.ReasoningEffort) || !validTextVerbosity(current.TextVerbosity) ||
		!validOutputTokens(current.MaxOutputTokens) || !validRetryCount(current.MaxRetries) || !validTimeout(current.TimeoutSeconds) {
		return errors.New("model settings contain an unsupported generation option")
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

func validReasoningEffort(value string) bool {
	switch value {
	case "none", "minimal", "low", "medium", "high", "xhigh", "max":
		return true
	default:
		return false
	}
}

func validTextVerbosity(value string) bool {
	return value == "low" || value == "medium" || value == "high"
}

func validOutputTokens(value string) bool {
	switch value {
	case "256", "512", "1024", "2048", "4096", "8192", "16384", "32768", "65536":
		return true
	default:
		return false
	}
}

func validRetryCount(value string) bool {
	return value == "0" || value == "1" || value == "2" || value == "3" || value == "4" || value == "5"
}

func validTimeout(value string) bool {
	return value == "30" || value == "45" || value == "60" || value == "90" || value == "120" || value == "180" || value == "300"
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
