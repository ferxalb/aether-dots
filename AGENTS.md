# Repository Guidelines

## Project Structure

- `main.go` starts the native macOS desktop app; `cmd/aether/main.go` contains the CLI.
- `internal/desktopui/` holds MyGo views, settings, actions, and appearance. Keep UI sections in focused files rather than growing `app.go`.
- `internal/engine/` runs tasks; `internal/model/` integrates the OpenAI SDK; `internal/source/` reads approved local files; `internal/store/` persists task history; `internal/task/` defines task data.
- Tests live beside their package in `*_test.go` files. App resources and third-party license notices are in `resources/`.

## Build, Test, and Development Commands

- `make dev` runs the MyGo live-reload app; `make run` starts it once.
- `make bundle` builds the macOS app; `make package` also creates a disk image.
- `make cli-help` prints CLI usage. Run CLI commands with `CGO_ENABLED=0 go run ./cmd/aether ...`.
- `CGO_ENABLED=0 go build ./...` compiles all packages; `CGO_ENABLED=0 go test ./...` runs the full test suite; `CGO_ENABLED=0 go tool mygo vet .` checks MyGo-specific issues.

## Coding Style and Naming

Use standard Go formatting with `gofmt`. Keep package names lowercase, exported identifiers in `PascalCase`, and unexported identifiers in `camelCase`. Name tests `TestThing` and keep them in the package they exercise. Follow the pinned MyGo v0.7.3 UI lifecycle and identity patterns; verify APIs against the pinned dependency before using newer examples.

## Testing Guidelines

Use Go's standard `testing` package. Add focused tests alongside the changed package; use `ui.NewTester` for native view behavior. Run the relevant package tests during iteration and `CGO_ENABLED=0 go test ./...` before submitting. Use `go test -race ./...` when changing shared state or goroutine coordination. For UI changes, include a screenshot or describe the native preview reviewed.

## Commits and Pull Requests

Recent commits use concise imperative summaries, with Conventional Commit prefixes such as `feat:` also present. Prefer a short `feat:`, `fix:`, or `docs:` summary that describes the change. PRs should explain user-visible behavior, link an issue when relevant, list build/test commands and results, and include screenshots for visual changes.

## Security and Configuration

Keep model keys out of task data and preferences files. Desktop credentials belong in MyGo's system credential store; `OPENAI_API_KEY` is the CLI fallback. Preserve the app's explicit local source and output scopes when changing task behavior. Append dated engineering notes to `HISTORY-TRACKER.md`; reserve `CHANGELOG.md` for release-ready changes.
