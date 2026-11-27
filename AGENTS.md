# Repository Guidelines

## Project Structure & Module Organization

AETHER Dots is a native macOS application built with Go and MyGo. `main.go` launches the desktop UI; `cmd/aether/` contains the CLI. Shared code lives in `internal/`: `desktopui` for views, `task` for models and limits, `store` for SQLite persistence, `engine` for chat orchestration, `model` for provider adapters, and `integrations` for external capabilities. Tests sit beside implementation files as `*_test.go`. `resources/` contains bundled resources and license notices; `scripts/` contains end-to-end tooling.

### Naming

User-facing text (UI, CLI output and help, README, model instructions) calls a Dot an **agent** and the product **AETHER**, to avoid confusion with another product named "dots". Code identifiers (`Dot`, `DotID`, ...), persisted formats and IDs (`dots.db`, `dot:<id>` Midnight scopes, `dot-<id>` server records), the `--dot` CLI flag, the data folder (`AETHER Dots` in `internal/store/store.go`), `mygo.json` `name`/`identifier` and the `com.aetherdots.*` Keychain services are unchanged on purpose: changing them orphans existing data, credentials and macOS permissions, so any future change moves them once, with a migration. Write new user-facing text with "agent".

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
- **Model proposes, human approves:** model tools only create a `pending` proposal. Approval happens in the desktop tray or, with the same `engine.ApproveGitHubProposal`, in `aether github proposals approve`, which needs an interactive terminal and the typed content-hash prefix (no `--yes`). iOS bridge, Routines and delegated Dots cannot propose or approve. Approval binds to a SHA-256 of the exact content; edits re-hash; no approve-all or auto-approve.
- **Grants:** separate per-Dot, per-repository, per-operation write grant (off by default, requires the read grant), its own disclosure, same Space binding; revoking cancels pending proposals. Rechecked on every proposal and again at send time with the pinned `gh` login.
- **Execution:** the only write path is `github.Service.Write` (`write.go`) over the enumerated operations: argv built in code (`api --method POST … --input - <path matched against an allow-list>`), owner/name resolved from `GET /repositories/{id}`, read-only preflight (pinned login, token scopes and rate limit via `api -i`, archived/locked/issues-disabled, branches and duplicate PR), JSON body on stdin (never `-f/-F`), `GH_HOST=github.com`, capped output, response verified (link inside the repository, author is the pinned login, PR really a draft). No automatic POST retry; ambiguous outcomes become `uncertain` and are reconciled by the user. The claim is a `BEGIN IMMEDIATE` transition so only one process sends.
- **Limits:** proposals expire after 24h; ≤10 pending per Dot, ≤3 per run; bounded title/body sizes in `internal/task`. Events record kind, repo, target and hash, never the body.
- GitHub content is untrusted evidence; flag proposals made in a run that read it. Read paths stay `--method GET` only.
- Issue search (`aether_github_search_issues`, `GET /search/issues`) is pinned to one granted repository: the code builds `repo:<owner>/<name> is:issue` from the store, and `github.SearchTerms` drops every model-supplied qualifier, operator and symbol, so a query cannot reach another repository. Up to 5 results, bodies capped at 300 bytes, each search recorded as a `github_read` event.
- Agent templates (`internal/task/templates.go`) only prefill name, avatar, objective and instructions. They never grant repositories, write proposals, MCP tools or Computer Use, and count toward the Free agent limit.

### License checks (permission design)

Licensing (`internal/license`, `aether license`) is off unless the build sets `-ldflags "-X aether-dots/internal/license.OrganizationID=<Polar org UUID>"`; optional `Environment=sandbox` and `BuildDate=<RFC 3339>` (needed for the fallback rule). Without the organization ID no license code talks to the network.

- **Network:** only `https://api.polar.sh` (or `https://sandbox-api.polar.sh`), chosen at build time, never from user input or environment. Only `POST /v1/customer-portal/license-keys/{activate,validate,deactivate}` (public endpoints, no credential). Redirects refused, 15 s timeout, responses capped at 64 KiB.
- **Data sent:** the license key, the organization ID, the activation ID and the label `AETHER <os>/<arch>`. No hostname, username, email, telemetry or usage counts.
- **Storage:** `<data-dir>/license.json`, 0600, written atomically; a symlink or group/world-readable file is refused.
- **Never gates local data.** A license only decides the Pro tier. Free users keep every agent, conversation and export.
- **Rules:** Pro while validated within 14 days and before `expires_at`. Perpetual fallback as in JetBrains: a build whose `BuildDate` is on or before the paid-through date keeps Pro forever; annual (expiring) keys qualify at once unless revoked before their expiry (a refund or disabled key), subscription keys after 12 months of validated use. The product is sold as an annual license only. Polar answers 404 for revoked and expired keys alike; a key past its own expiry is not marked revoked, so its paid term still counts. Network failures leave the saved state unchanged.
- **Free / Pro split (`license.Features`):** Free = up to 3 agents, chat, GitHub read access, Midnight. Pro = unlimited agents, GitHub write proposals, Routines. Builds without licensing have everything. Gates only stop creating more or starting paid work: agent creation (desktop `startCreator`, CLI `create`), write-grant saves (desktop panel, CLI `write-grant`), routine creation (desktop Routines tab and the template "Add daily summary" button, CLI), routine runs (`engine.RunRoutineOnce*`, `engine.RunDueRoutines*`, which claim nothing on Free) and proposal tools (`engine.WithGitHubWriteTools`). Existing agents, conversations, routines and grants stay readable, editable, deletable and exportable, and pending proposals can still be approved or rejected. A new paid feature gets its check in `engine` or the store-facing entry point, not only in the UI.
