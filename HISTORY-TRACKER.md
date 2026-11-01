# AETHER Dots History Tracker

This is the chronological engineering work log for AETHER Dots. It records session-level additions, modifications, structural decisions, verification, and known gaps. Entries can describe implementation that is still in progress.

This file is separate from a product release changelog. `CHANGELOG.md` is reserved for version-specific changes that are complete and ready to ship as Beta, RC, or Final. No release version is being declared here, and this session did not create or update a `CHANGELOG.md`.

For each future work session, append a dated entry describing what changed, why, how it was checked, and what remains unverified. Keep older entries intact.

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
