# AETHER Dots History Tracker

This is the chronological engineering work log for AETHER Dots. It records session-level additions, modifications, structural decisions, verification, and known gaps. Entries can describe implementation that is still in progress.

This file is separate from a product release changelog. `CHANGELOG.md` is reserved for version-specific changes that are complete and ready to ship as Beta, RC, or Final. No release version is being declared here, and this session did not create or update a `CHANGELOG.md`.

For each future work session, append a dated entry describing what changed, why, how it was checked, and what remains unverified. Keep older entries intact.

## 2026-11-01 (04) — Calm native shell and task workflow refresh

### Workspace shell and navigation

- Removed the inset card frame and moved the toolbar out of the content scroll. The fixed toolbar starts below the native title-bar drag strip, while the task body scrolls independently.
- Rebuilt the expanded and collapsed sidebar around a clear New Dot action, active Space, recent Dot history, Routines, and a pinned Settings footer. History is filtered to the active Space, ordered by most recent update, and shows each Dot's status and selected state.
- Moved Space creation and renaming into a compact sidebar disclosure. Selecting a newly created Space clears stale task selection; running a Routine keeps the active Space context.
- Made Obsidian the new-install and invalid-palette fallback while preserving every valid saved appearance and palette preference.

### Capture and result surfaces

- Reworked capture into a compact outcome composer with explicit Read and Write rows, a concise execution disclosure, and a visible Create Dot action at the 760×560 minimum window size.
- Grouped detail views into Result, Local permissions, and bounded Progress sections. Results appear before the audit trail, which shows the full event history in its own scroll area.
- Preserved the explicit local source and output folders and the existing local/model behavior.

### Verification

- Corrected a stale engine test config literal to use the current empty `model.Config` for its missing-key scenario; production engine and model behavior did not change.
- `CGO_ENABLED=0 go build ./...`, `CGO_ENABLED=0 go test ./...`, and `CGO_ENABLED=0 go tool mygo vet .` passed.
- Reviewed the live AETHER Dots Dev native window, dark capture/detail Tester previews at 1120×760 and 760×560, and light collapsed-model capture previews at both sizes. PNGs are in `/tmp/aether-ui-review/` (`capture-1120x760.png`, `capture-760x560.png`, `detail-1120x760.png`, `detail-760x560.png`, `capture-light-collapsed-model-1120x760.png`, `capture-light-collapsed-model-760x560.png`).

## 2026-11-01 (03) — Settings page, themes, and official OpenAI SDK

### Session objective

Replace the environment-only model status panel with a first-class settings page, add appearance choices, use OpenAI's official Go SDK, and split the native UI into focused source files.

### Implementation

- Added a dedicated Settings destination in the app sidebar with OpenAI model configuration and an asynchronous connection check using the Responses API.
- Replaced hand-written Responses/Chat Completions HTTP streaming with `github.com/openai/openai-go/v3`. Requests stream through the SDK and set `store: false`; provider-compatible endpoints and the Chat Completions toggle were removed.
- Added local preferences for model name, appearance mode, and seven palettes. The API key is stored through MyGo's system credential store and never enters the task store or preferences file. `OPENAI_API_KEY` remains a CLI and desktop fallback.
- Added System, Light, and Dark appearance modes and Obsidian, Forest, Ocean, Silver, Violet, Rose, and Amber palettes. The system mode follows macOS appearance.
- Split the former desktop UI source into app/runtime, navigation/layout, task views, settings, theme, preferences, and action files.
- Updated the README, CLI help, and existing model/UI tests for the new configuration flow.

### Verification

- `gofmt`, `go mod tidy`, `git diff --check`, and `CGO_ENABLED=0 go build ./...` completed successfully.
- The test suite and native GUI were not run during this session.

## 2026-11-01 (02) — Native workspace refresh and command shortcuts

### Session objective

Refresh the native desktop workspace using the supplied design references, remove the visible title strip while retaining window movement, add a collapsible sidebar, and make common run/build commands easy to invoke.

### UI work

- Replaced the standard visible title bar with MyGo's hidden inset title bar. Native window controls remain, and a clear `DragWindow` region at the top preserves title-bar window movement; normal resize behavior remains enabled.
- Restyled the workspace with a system-aware light/dark palette, violet and teal accents, larger corner radii, rounded source/output cards, clearer model credential status, and macOS sidebar vibrancy over translucent surfaces.
- Added vector icons for the brand, Spaces, recent Dots, Routines, task statuses, folder actions, and primary actions.
- Added a 248 px expanded sidebar and a 72 px icon rail. Spaces, new-Dot capture, recent Dots, Routines, and runtime status remain available when collapsed, with labels and tooltips for icon-only controls.
- Updated the task capture and model configuration sections with rounded cards and icon-led actions. Kept the space name field neutral because it is also used by the existing rename action.
- Added a high-contrast preference adjustment to retain stronger borders, secondary text, focus rings, and scrollbars under the desktop accessibility setting.

### Developer commands

- Added a root `Makefile` with `make dev` (MyGo live-reload app), `make run`, `make bundle`, `make package`, `make cli-help`, and `make help`.
- Documented these commands in the README. MyGo's `dev` target launches a distinct **AETHER Dots Dev** bundle, avoiding confusion with an already-running production app.

### Verification completed

- `gofmt` completed for `internal/desktopui/app.go`.
- `CGO_ENABLED=0 go build -o /tmp/aether-dots-ui-check .` passed after the UI and theme changes.
- An isolated MyGo development bundle was built from a temporary project copy. The native preview showed the refreshed layout; the sidebar collapsed and expanded correctly, with accessible labels present.
- `make help` printed the documented targets, and `make -n dev run bundle package cli-help` resolved each command as intended.
- `git diff --check` passed.

### Not verified or not included in this session

- No automated Go test suite was run.
- The top drag region uses MyGo's documented native `DragWindow` behavior. A drag gesture was issued in the preview, but window position was not separately measured afterward.
- `make bundle` and `make package` were inspected in dry-run mode, not run from the project root. The isolated MyGo bundle build completed successfully before the final small contrast and label refinements; the final source state was verified with the Go build.

## 2026-11-01 (01) — Local desktop slice and root consolidation

### Session objective

Implement the first useful AETHER Dots desktop task loop, then make the repository root the single Go project after reviewing the long-term layout.

### Application work added

- Built a native macOS UI with pinned MyGo v0.7.3. It provides Spaces, Dot capture, recent Dots, event and artifact inspection, local/model execution selection, native source and destination folder pickers, and Routine management.
- Added the quick-capture composer, menu-bar access, and `⌘⇧Space` global shortcut.
- Added a versioned local JSON store. Writes use owner-only permissions and atomic replacement. Dot events have sequence numbers and a per-Dot hash chain; runs retain checkpoints, status changes, and artifact records.
- Implemented the local task flow: capture an outcome and explicit local folders, read selected source files, generate a brief, atomically write it to the chosen destination, compare the saved bytes, record the SHA-256, and inspect or resume the Dot later.
- Kept local extraction deterministic and explicitly limited to matching open-decision markers. It does not use a model or claim to infer whether a decision is unresolved.
- Restricted source reads to supported UTF-8 text beneath the chosen folder: at most 32 files, 256 KiB per file, and 2 MiB total. Symbolic links and hidden, build, and dependency directories are skipped. Output is capped at 2 MiB and must be outside the source folder.
- Added optional streaming model execution. The default is the OpenAI Responses API with `gpt-6.1-sol`; Chat Completions is selectable for compatible endpoints. The Responses request disables server-side storage. Credentials are read from a named environment variable and are not written to the task store or logs.
- Added model-configuration waiting and recovery behavior: a Dot without a configured model credential can wait for input, switch to local mode, and resume.
- Added Routine promotion and replay. A Routine carries the successful Dot's original source and output folder grants; editing its name or objective does not widen those grants.
- Added the `aether` CLI for Spaces, capture, list, inspect, mode changes, run/resume, model connection checks, and Routine promotion/edit/run.

### Repository layout consolidation

- The first implementation was placed in `desktop/` to honor the initial instruction to preserve the root starter app. After discussing the long-term layout, consolidated the application into the existing root Go module so the repository has one canonical entry point and one dependency graph.
- Replaced the root starter `main.go` with the AETHER Dots application bootstrap and updated root `mygo.json` to the `com.aetherdots.desktop` application identifier.
- Moved the CLI to `cmd/aether/` and application packages to `internal/desktopui`, `internal/task`, `internal/store`, `internal/source`, `internal/engine`, and `internal/model`.
- Updated Go import paths from `aether-dots/desktop/...` to `aether-dots/...`.
- Merged the app README, implementation decisions, Apache-2.0 license, and dependency license notices into the repository root. Preserved the existing root icon and included all license texts in the app bundle.
- Updated `.gitignore` so the root README and history tracker remain visible despite the Markdown ignore rule.
- Removed the old root starter `main_test.go`, which exercised the sample counter UI replaced by the product app; the AETHER UI tests remain in `internal/desktopui/app_test.go`.
- Moved the previous nested `desktop/` tree, including its build output, to `/tmp/aether-dots-desktop-before-root-consolidation` as a temporary rollback copy. It is outside the repository and is not used by the root build.
- `go.mod` already contained the root module name, Go version, and the pinned MyGo dependency graph. `go mod tidy` completed without requiring a dependency change.

### Code corrections during the session

- Aligned the endpoint-label helper with the model configuration API and updated its test after the final compiler pass caught the mismatch.
- Tightened output-folder validation so a Dot cannot write its artifact inside the folder it reads.
- Kept the endpoint label free of URL paths, credentials, query strings, and fragments.

### Verification completed

- `gofmt` completed for the Go sources.
- `go mod tidy` completed from the repository root.
- `CGO_ENABLED=0 go test ./...` passed from the root module.
- `go test -race ./...` passed from the root module.
- `go tool mygo vet .` passed from the root module.
- No-CGO root GUI and CLI builds passed:
  - `CGO_ENABLED=0 go build -o /tmp/aether-dots-root-gui .`
  - `CGO_ENABLED=0 go build -o /tmp/aether-dots-root-cli ./cmd/aether`
- `CGO_ENABLED=0 go tool mygo build --skip-dmg --sign -` passed from the root and produced `build/darwin-amd64/AETHER Dots.app` (about 12.2 MB, ad-hoc signed; no DMG was produced).
- CLI acceptance smoke flow passed against temporary folders: capture, local run, artifact verification, inspection, Routine promotion, and Routine replay.
- Recovery smoke flow passed: a model Dot without a key entered a waiting state, switched to local mode, resumed, and completed with a verified artifact.
- Native GUI interaction was exercised with folder selection, Dot execution, event/artifact inspection, persisted state after restart, and Routine replay. The root-built app was opened again after consolidation and its capture screen was confirmed.
- Model streaming behavior was tested with local HTTP test servers for Responses and Chat Completions. The Responses test checks streaming and `store: false`.

### Not verified or not included in this slice

- No live model endpoint request was made because `OPENAI_API_KEY` was unavailable in the environment. Model transport was exercised with local test servers only.
- The app remains local-first. MCP configuration, cloud sync, device handoff, OS keychain storage, and arbitrary-app control were not implemented.
- No production signing, notarization, DMG, or non-macOS build was verified.
