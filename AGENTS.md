# Repository Guidelines

## Project Structure & Module Organization

AETHER Dots is a native macOS application built with Go and MyGo. `main.go` launches the desktop UI; `cmd/aether/` contains the CLI. Shared code lives in `internal/`: `desktopui` for views, `task` for models and limits, `store` for SQLite persistence, `engine` for chat orchestration, `model` for provider adapters, and `integrations` for external capabilities. Tests sit beside implementation files as `*_test.go`. `resources/` contains bundled resources and license notices; `scripts/` contains end-to-end tooling.

## Build, Test, and Development Commands

Use macOS and the Go version declared in `go.mod` (currently 1.27.3). Make targets set `CGO_ENABLED=0`.

- `make dev`: launch the desktop app with MyGo live reload.
- `make run`: run the desktop app once.
- `make bundle` / `make package`: build the `.app` / `.app` plus disk image.
- `make cli-help`: show CLI commands.
- `CGO_ENABLED=0 go build ./...`: compile all packages.
- `CGO_ENABLED=0 go test ./...`: run automated tests.
- `CGO_ENABLED=0 go tool mygo vet .`: run MyGo-specific checks.

## Coding Style & Naming Conventions

Format Go changes with `gofmt`; use standard Go naming and tab indentation. Keep packages lowercase, exported identifiers in PascalCase, and focused files named by responsibility, such as `github_tools.go`. Keep UI sections out of an oversized `app.go`. Verify MyGo APIs against the version pinned in `go.mod`. When extending task models, update limits and structural copies in `internal/store/clone.go`.

## Testing Guidelines

Use Go's `testing` package and `Test...` functions; UI tests use `ui.NewTester`. Run targeted tests with `CGO_ENABLED=0 go test ./internal/store -run TestName`. Add regression coverage for changed behavior, especially persistence and permissions. Midnight end-to-end tests require `MIDNIGHT_BIN`. No numeric coverage threshold is specified; report skipped or unavailable checks.

## Commit & Pull Request Guidelines

Use short imperative commit summaries; history includes `feat:`, `fix:`, `perf:`, and optional scopes. PRs should explain the problem, resulting behavior, validation, and relevant issues. Include screenshots for UI changes. Append dated session notes to `HISTORY-TRACKER.md`; reserve release-ready changes for the changelog.

## Security & Configuration

Preserve explicit per-Dot grants and runtime permission checks. Do not add arbitrary file, shell, or network access without a separate permission design. Keep MCP loopback-only and preserve reduced tool access for routines, delegation, and iPhone runs. Store desktop API keys in Keychain; never commit secrets or local databases. Use `AETHER_DOTS_DATA_DIR` for isolated development data.

### GitHub write approvals (permission design)

GitHub access goes only through the user's own `gh` binary; AETHER holds no GitHub token. Writes are allowed only as human-approved proposals:

- **Operations (closed set):** `issue_comment` (`POST /repos/{o}/{r}/issues/{n}/comments`), `issue_create` (`POST /repos/{o}/{r}/issues`), `pr_draft` (`POST /repos/{o}/{r}/pulls`, `draft:true` hard-coded, same-repo existing `head`, no branch creation or push). Labels, assignees, milestones, edits, close, ready-for-review and merge are out of scope. Adding an operation needs a new design.
- **Model proposes, human approves:** model tools only create a `pending` proposal. Approval happens in the desktop tray (CLI parity later, TTY-only). iOS bridge, Routines and delegated Dots cannot propose or approve. Approval binds to a SHA-256 of the exact content; edits re-hash; no approve-all or auto-approve.
- **Grants:** separate per-Dot, per-repository, per-operation write grant (off by default, requires the read grant), its own disclosure, same Space binding; revoking cancels pending proposals. Rechecked on every proposal and again at send time with the pinned `gh` login.
- **Execution:** the only write path is a typed `ghRunner.write` over the enumerated operations: argv built in code (`api --method POST … --input - <validated path>`), JSON body on stdin (never `-f/-F`), `GH_HOST=github.com`, capped output, verified `html_url`. No automatic POST retry; ambiguous outcomes become `uncertain` and are reconciled by the user. The claim is a `BEGIN IMMEDIATE` transition so only one process sends.
- **Limits:** proposals expire after 24h; ≤10 pending per Dot, ≤3 per run; bounded title/body sizes in `internal/task`. Events record kind, repo, target and hash, never the body.
- GitHub content is untrusted evidence; flag proposals made in a run that read it. Read paths stay `--method GET` only.
