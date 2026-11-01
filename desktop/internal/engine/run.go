package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"aether-dots/desktop/internal/source"
	"aether-dots/desktop/internal/store"
	"aether-dots/desktop/internal/task"
)

var marker = regexp.MustCompile(`(?i)\b(open|pending|unresolved|decision|decide|tbd|todo|blocked|question|feedback|needs review)\b`)

type Generator interface {
	Generate(context.Context, string, []source.Document, func(string)) (string, error)
	Readiness() (bool, string)
	Description() string
}

func Run(ctx context.Context, db *store.Store, dotID string, generator Generator, onDelta func(string)) error {
	dot, ok := db.GetDot(dotID)
	if !ok {
		return errors.New("Dot not found")
	}
	if dot.Status == task.Running {
		return errors.New("Dot already has an active run")
	}
	if dot.Status == task.Done {
		return errors.New("Dot is already done; create a new Dot or run its saved Routine to repeat the work")
	}
	if dot.SourceRoot == "" || dot.OutputDir == "" {
		return setWaiting(db, dotID, "Choose a local source folder and an output folder before running this Dot.")
	}
	if err := db.SetStatus(dotID, task.Running, "Run started on this device. The selected source and output are local-only."); err != nil {
		return err
	}
	if err := db.SetCheckpoint(dotID, "reading selected text files"); err != nil {
		return fail(db, dotID, err)
	}
	root, err := filepath.EvalSymlinks(dot.SourceRoot)
	if err != nil {
		return fail(db, dotID, err)
	}
	outputDir, err := filepath.EvalSymlinks(dot.OutputDir)
	if err != nil {
		return fail(db, dotID, fmt.Errorf("resolve output folder: %w", err))
	}
	outputInfo, err := os.Stat(outputDir)
	if err != nil || !outputInfo.IsDir() {
		return fail(db, dotID, errors.New("output destination must be an existing folder selected by the user"))
	}
	relativeOutput, err := filepath.Rel(root, outputDir)
	if err != nil {
		return fail(db, dotID, fmt.Errorf("compare source and output folders: %w", err))
	}
	if relativeOutput == "." || (relativeOutput != ".." && !strings.HasPrefix(relativeOutput, ".."+string(os.PathSeparator))) {
		return fail(db, dotID, errors.New("choose an output folder outside the selected source folder"))
	}
	root, docs, err := source.ReadFolder(root)
	if err != nil {
		return fail(db, dotID, err)
	}
	if err := db.AppendEvent(dotID, "source_read", fmt.Sprintf("Read %d selected text files from %s.", len(docs), root)); err != nil {
		return fail(db, dotID, err)
	}
	if err := db.SetCheckpoint(dotID, fmt.Sprintf("read %d text files; composing %s brief", len(docs), dot.Mode)); err != nil {
		return fail(db, dotID, err)
	}
	var report string
	if dot.Mode == "model" {
		ready, reason := false, "model provider is not configured"
		if generator != nil {
			ready, reason = generator.Readiness()
		}
		if !ready {
			message := "Model mode needs configuration: " + reason + ". Set it and resume, or switch this Dot to local extraction."
			_ = db.SetStatus(dotID, task.WaitingInput, message)
			return errors.New(message)
		}
		if err := db.AppendEvent(dotID, "model_started", fmt.Sprintf("Sending the selected text files to %s for a bounded draft.", generator.Description())); err != nil {
			return fail(db, dotID, err)
		}
		var lastSaved time.Time
		var partial strings.Builder
		report, err = generator.Generate(ctx, dot.Objective, docs, func(delta string) {
			partial.WriteString(delta)
			if onDelta != nil {
				onDelta(delta)
			}
			if time.Since(lastSaved) >= 700*time.Millisecond {
				_ = db.SetCheckpoint(dotID, partial.String())
				lastSaved = time.Now()
			}
		})
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
				return pause(db, dotID)
			}
			return fail(db, dotID, err)
		}
		if err := db.AppendEvent(dotID, "model_finished", "Model returned a draft. Claims still need human review; only the local file write will be mechanically verified."); err != nil {
			return fail(db, dotID, err)
		}
	} else {
		report = localBrief(dot.Objective, docs)
		if err := db.AppendEvent(dotID, "local_extraction", "Built a deterministic list of source lines containing open-decision markers; no model was used."); err != nil {
			return fail(db, dotID, err)
		}
	}
	if err := ctx.Err(); err != nil {
		return pause(db, dotID)
	}
	if strings.TrimSpace(report) == "" || len(report) > source.MaxOutputBytes {
		return fail(db, dotID, errors.New("report was empty or exceeded the 2 MiB output limit"))
	}
	if err := db.SetCheckpoint(dotID, "writing and verifying the local artifact"); err != nil {
		return fail(db, dotID, err)
	}
	outputPath := filepath.Join(outputDir, "decision-brief-"+dot.ID+".md")
	data := []byte(report)
	if err := writeAtomic(outputPath, data); err != nil {
		return fail(db, dotID, fmt.Errorf("write local artifact: %w", err))
	}
	check, err := os.ReadFile(outputPath)
	if err != nil {
		return fail(db, dotID, fmt.Errorf("verify local artifact: %w", err))
	}
	if len(check) != len(data) || string(check) != string(data) {
		return fail(db, dotID, errors.New("saved artifact did not match the generated bytes"))
	}
	sum := sha256.Sum256(check)
	artifact := task.Artifact{Path: outputPath, SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(check)), VerifiedAt: time.Now().UTC()}
	if err := db.AddArtifact(dotID, artifact); err != nil {
		return fail(db, dotID, err)
	}
	return db.SetStatus(dotID, task.Done, "Completed: the brief exists at the chosen destination and its saved bytes match the recorded SHA-256.")
}

func localBrief(objective string, docs []source.Document) string {
	var b strings.Builder
	b.WriteString("# Local evidence extraction\n\n")
	b.WriteString("**Objective:** ")
	b.WriteString(oneLine(objective))
	b.WriteString("\n\n")
	b.WriteString("This report was created without a model. It lists source lines containing open-decision markers; it does not infer whether a decision is actually unresolved. Review the cited lines before acting.\n\n")
	b.WriteString("## Candidate open items\n\n")
	count := 0
	for _, doc := range docs {
		for i, line := range strings.Split(doc.Text, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || !marker.MatchString(line) {
				continue
			}
			count++
			b.WriteString(fmt.Sprintf("- `%s:%d` — %s\n", doc.Path, i+1, oneLine(line)))
		}
	}
	if count == 0 {
		b.WriteString("No source lines matched the local open-decision marker list. This does not establish that no open decisions exist.\n")
	}
	b.WriteString("\n## Sources read\n\n")
	for _, doc := range docs {
		b.WriteString(fmt.Sprintf("- `%s` (%d bytes, SHA-256 `%s`)\n", doc.Path, doc.Bytes, doc.SHA256))
	}
	return b.String()
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	return strings.ReplaceAll(s, "|", "\\|")
}

func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".brief-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func setWaiting(db *store.Store, id, message string) error {
	_ = db.SetStatus(id, task.WaitingInput, message)
	return errors.New(message)
}

func pause(db *store.Store, id string) error {
	_ = db.AppendEvent(id, "checkpoint_saved", "Run stopped at a safe boundary; resume will re-read the selected local source.")
	_ = db.SetCheckpoint(id, "safe to resume from source read")
	_ = db.SetStatus(id, task.Paused, "Run paused. Resume re-reads the bounded local source before continuing.")
	return context.Canceled
}

func fail(db *store.Store, id string, cause error) error {
	_ = db.AppendEvent(id, "error", cause.Error())
	_ = db.SetStatus(id, task.Failed, "Run failed: "+cause.Error())
	return cause
}
