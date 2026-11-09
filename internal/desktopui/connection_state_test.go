package desktopui

import "testing"

func TestConnectionResultPersistsUntilModelOrKeyChanges(t *testing.T) {
	a := newMessengerTestApp(t)
	a.recordConnection(a.prefs, true)
	a.modelConnection = ""
	a.syncConnectionState()
	if a.modelConnection != "Connected" {
		t.Fatalf("state = %q, want Connected", a.modelConnection)
	}
	// A fresh app on the same folder remembers the result.
	b := &app{dataDir: a.dataDir, prefs: a.prefs, connectionRecords: loadConnectionRecords(a.dataDir)}
	b.syncConnectionState()
	if b.modelConnection != "Connected" || b.modelConnectedAtZero() {
		t.Fatalf("restart lost the result: %q", b.modelConnection)
	}
	*b.prefs.selectedModel() = "another-model"
	b.syncConnectionState()
	if b.modelConnection != "Not tested" {
		t.Fatalf("changed model kept %q", b.modelConnection)
	}
	*b.prefs.selectedModel() = *a.prefs.selectedModel()
	b.invalidateConnection()
	if b.modelConnection != "Not tested" {
		t.Fatalf("changed key kept %q", b.modelConnection)
	}
}

func TestSettingsDirtyTracksUnsavedChanges(t *testing.T) {
	a := newMessengerTestApp(t)
	a.savedPrefs = a.prefs
	if a.settingsDirty() {
		t.Fatal("fresh settings must not be dirty")
	}
	a.prefs.TextVerbosity = "high"
	if !a.settingsDirty() {
		t.Fatal("edited preference must be dirty")
	}
	a.prefs = a.savedPrefs
	a.apiKeyDraft = "sk-new"
	if !a.settingsDirty() {
		t.Fatal("typed key must be dirty")
	}
}

func (a *app) modelConnectedAtZero() bool { return a.modelConnectionAt.IsZero() }
