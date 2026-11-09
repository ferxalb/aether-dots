package desktopui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aether-dots/internal/model"
)

const connectionStateFile = "connection-state.json"

// connectionRecord is the last connection-test result for one provider. It
// holds no secrets. It is only trusted while the provider, model and endpoint
// still match Fingerprint, so changing any of them reads as "Not tested".
type connectionRecord struct {
	Fingerprint string    `json:"fingerprint"`
	OK          bool      `json:"ok"`
	CheckedAt   time.Time `json:"checked_at"`
}

func connectionFingerprint(p preferences) string {
	parts := []string{p.Provider, strings.TrimSpace(*p.selectedModel())}
	if p.Provider == string(model.ProviderCompatible) {
		parts = append(parts, strings.TrimSpace(p.CompatibleBaseURL))
	}
	return strings.Join(parts, "\x00")
}

func loadConnectionRecords(dataDir string) map[string]connectionRecord {
	records := map[string]connectionRecord{}
	b, err := os.ReadFile(filepath.Join(dataDir, connectionStateFile))
	if err != nil || json.Unmarshal(b, &records) != nil || records == nil {
		return map[string]connectionRecord{}
	}
	return records
}

func saveConnectionRecords(dataDir string, records map[string]connectionRecord) error {
	b, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dataDir, ".connection-*.tmp")
	if err != nil {
		return err
	}
	path := tmp.Name()
	defer func() { _ = os.Remove(path) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(path, filepath.Join(dataDir, connectionStateFile))
}

// syncConnectionState derives the visible connection state from the saved
// record for the current provider, model and endpoint.
func (a *app) syncConnectionState() {
	if a.modelConnection == "Checking…" {
		return
	}
	a.modelConnection, a.modelConnectionAt = "Not tested", time.Time{}
	record, ok := a.connectionRecords[a.prefs.Provider]
	if !ok || record.Fingerprint != connectionFingerprint(a.prefs) {
		return
	}
	a.modelConnectionAt = record.CheckedAt
	if record.OK {
		a.modelConnection = "Connected"
	} else {
		a.modelConnection = "Connection failed"
	}
}

func (a *app) recordConnection(prefs preferences, ok bool) {
	if a.connectionRecords == nil {
		a.connectionRecords = map[string]connectionRecord{}
	}
	a.connectionRecords[prefs.Provider] = connectionRecord{
		Fingerprint: connectionFingerprint(prefs), OK: ok, CheckedAt: time.Now().UTC(),
	}
	if err := saveConnectionRecords(a.dataDir, a.connectionRecords); err != nil && !errors.Is(err, os.ErrNotExist) {
		a.adt.Warning("Couldn't remember the test result", err.Error())
	}
}

// invalidateConnection forgets the saved result, e.g. after the key changes.
func (a *app) invalidateConnection() {
	if _, ok := a.connectionRecords[a.prefs.Provider]; ok {
		delete(a.connectionRecords, a.prefs.Provider)
		_ = saveConnectionRecords(a.dataDir, a.connectionRecords)
	}
	a.modelConnection = ""
	a.syncConnectionState()
}

func relativeTime(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case t.IsZero():
		return ""
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d/time.Minute))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d/time.Hour))
	default:
		return fmt.Sprintf("%dd ago", int(d/(24*time.Hour)))
	}
}

// settingsDirty reports unsaved preference edits or a typed API key.
func (a *app) settingsDirty() bool {
	return a.prefs != a.savedPrefs || strings.TrimSpace(a.apiKeyDraft) != ""
}
