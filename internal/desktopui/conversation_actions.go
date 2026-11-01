package desktopui

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"aether-dots/internal/source"
	"aether-dots/internal/task"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *app) startCreator() {
	a.rememberCurrentConversationState()
	a.selectedConversationID = ""
	a.selectedDotID = ""
	a.selectedRoutineID = ""
	a.settingsOpen = false
	a.routinesOpen = false
	a.creatorOpen = true
	a.creatorSpaceManagementOpen = false
	a.dotName = ""
	a.objective = ""
	a.mode = "local"
	a.sourceRoot = ""
	a.outputDir = ""
	a.messageDraft = ""
	a.conversationError = ""
	a.message = ""
}

func (a *app) createConversationFromCreator() {
	conversation, err := a.db.CreateConversation(task.CreateConversation{
		SpaceID: a.selectedSpaceID, Name: a.dotName, Purpose: a.objective,
		Mode: a.mode, SourceRoot: a.sourceRoot, OutputDir: a.outputDir,
	})
	if err != nil {
		a.message = err.Error()
		return
	}
	a.selectedSpaceID = conversation.SpaceID
	a.selectedDotID = ""
	a.selectedRoutineID = ""
	a.creatorOpen = false
	a.routinesOpen = false
	a.creatorSpaceManagementOpen = false
	a.settingsOpen = false
	a.conversationError = ""
	a.message = "Dot created. Send a message when you’re ready to delegate work."
	a.selectConversation(conversation.ID)
}

func (a *app) sendConversationMessage() {
	if a.cancelRun != nil {
		a.conversationError = "Wait for the current delegation to finish or stop it before sending another message."
		return
	}
	conversation, ok := findConversation(a.snapshot, a.selectedConversationID)
	if !ok {
		a.conversationError = "Choose a Dot conversation before sending a message."
		return
	}
	dot, _, err := a.db.CreateConversationRun(conversation.ID, a.messageDraft)
	if err != nil {
		a.conversationError = err.Error()
		return
	}
	a.clearConversationDraft(conversation.ID)
	a.messageDraft = ""
	a.conversationError = ""
	a.message = "Delegation saved. Starting the task with this Dot’s saved permissions."
	a.refresh()
	a.chatScroll.ScrollTo(0, math.MaxFloat32, ui.Smooth)
	a.startRun(dot.ID)
}

func (a *app) stopConversationRun() {
	if a.cancelRun == nil {
		return
	}
	a.cancelRun()
	a.message = "Stop requested; the delegation will pause at its next safe boundary."
}

func (a *app) openVerifiedConversationArtifact(path string) {
	for _, dot := range conversationDots(a.snapshot, a.selectedConversationID) {
		if len(dot.Artifacts) == 0 {
			continue
		}
		artifact := dot.Artifacts[len(dot.Artifacts)-1]
		if artifact.Path != path {
			continue
		}
		if _, err := verifiedConversationReport(dot); err != nil {
			a.conversationError = err.Error()
			return
		}
		if err := mygo.Shell.OpenPath(path); err != nil {
			a.conversationError = err.Error()
		}
		return
	}
	a.conversationError = "Verified result not found in this Dot conversation."
}

// verifiedConversationReport reads only a completed run's artifact under its
// saved output grant. It verifies the size and SHA-256 again before persisting
// the artifact text as the conversation's assistant result.
func verifiedConversationReport(dot task.Dot) (string, error) {
	if dot.Status != task.Done || dot.ConversationID == "" || len(dot.Artifacts) == 0 {
		return "", errors.New("completed conversation Dot has no verified artifact")
	}
	artifact := dot.Artifacts[len(dot.Artifacts)-1]
	if dot.OutputDir == "" || artifact.Path == "" {
		return "", errors.New("completed conversation Dot has no saved output scope")
	}
	if artifact.Bytes <= 0 || artifact.Bytes > source.MaxOutputBytes {
		return "", fmt.Errorf("verified artifact exceeds the %d-byte conversation result limit", source.MaxOutputBytes)
	}
	outputRoot, err := filepath.EvalSymlinks(dot.OutputDir)
	if err != nil {
		return "", fmt.Errorf("resolve approved output folder: %w", err)
	}
	outputInfo, err := os.Stat(outputRoot)
	if err != nil || !outputInfo.IsDir() {
		return "", errors.New("saved output scope is not an available folder")
	}
	artifactPath, err := filepath.EvalSymlinks(artifact.Path)
	if err != nil {
		return "", fmt.Errorf("resolve verified artifact: %w", err)
	}
	relative, err := filepath.Rel(outputRoot, artifactPath)
	if err != nil {
		return "", fmt.Errorf("compare artifact with approved output folder: %w", err)
	}
	if relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return "", errors.New("verified artifact is outside the saved output folder")
	}
	file, err := os.Open(artifactPath)
	if err != nil {
		return "", fmt.Errorf("read verified artifact: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() != artifact.Bytes {
		return "", errors.New("verified artifact changed size or is not a regular file")
	}
	data, err := io.ReadAll(io.LimitReader(file, source.MaxOutputBytes+1))
	if err != nil {
		return "", fmt.Errorf("read verified artifact bytes: %w", err)
	}
	if len(data) > source.MaxOutputBytes || int64(len(data)) != artifact.Bytes {
		return "", errors.New("verified artifact changed size while reading")
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != artifact.SHA256 {
		return "", errors.New("verified artifact SHA-256 changed before it could be saved to the conversation")
	}
	if !utf8.Valid(data) {
		return "", errors.New("verified artifact is not valid UTF-8 conversation text")
	}
	return string(data), nil
}

func (a *app) persistConversationResult(conversationID, dotID string, runErr error) error {
	if runErr != nil || conversationID == "" {
		return nil
	}
	dot, ok := a.db.GetDot(dotID)
	if !ok {
		return errors.New("completed conversation Dot disappeared before its result was saved")
	}
	report, err := verifiedConversationReport(dot)
	if err != nil {
		return err
	}
	return a.db.AppendConversationResult(conversationID, dotID, report)
}
