# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

`AGENTS.md` is the authoritative source for repository guidelines and, especially, the **security/permission boundary** (what Dots may and may not access: no local files, no arbitrary network, no shell, GitHub/iOS-bridge/MCP/Computer Use only under their specific designs). Read it before adding any tool, integration, or write operation. Do not widen those boundaries without a separate permission design.

## Commands

All commands use `CGO_ENABLED=0`.

```sh
make dev                                   # MyGo live-reload desktop app
make run                                   # run desktop app once (go run .)
make bundle / make package                 # macOS .app / .app + dmg
CGO_ENABLED=0 go build ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go test ./internal/store -run TestStoreRoundTripAndRejectsChangedEvent   # single test
CGO_ENABLED=0 go test -race ./...          # when touching shared state / goroutines
CGO_ENABLED=0 go tool mygo vet .           # MyGo-specific checks
CGO_ENABLED=0 go run ./cmd/aether --help   # CLI (also: make cli-help)
```

- The Midnight end-to-end test (`cmd/aether/e2e_midnight_test.go`, `scripts/e2e-midnight.sh`) is skipped unless `MIDNIGHT_BIN` points to an `aether-midnight` binary.
- UI is native MyGo (v0.7.3 pinned, not a WebView); test views with `ui.NewTester`. Verify APIs against the pinned dependency, not newer docs.

## Architecture

Two entry points share the same internal packages: `main.go` (desktop app → `internal/desktopui`) and `cmd/aether` (CLI: Dots, routines, GitHub, MCP, skills, Midnight server).

- `internal/task` — data model and schema (`SchemaVersion`, all size/count limits as `Max*` constants, Dot/conversation/event types). Limits live here; check them before adding fields.
- `internal/store` — single local SQLite database (`dots.db`, WAL; `sqlite.go` writes only changed rows, `dots.json` is imported once). Holds Dot profiles, conversations, grants, routines and hash-chained task events. Designed for multiple processes (desktop + CLI) sharing one folder: `BEGIN IMMEDIATE` write lock, `PRAGMA data_version` reload signal, merge-on-change instead of overwrite, incremental event-chain verification, per-Dot run leases (`runlease.go`) with orphaned-run recovery. Snapshots/transactions use structural copies (`clone.go`) — update them when adding fields to `task` types.
- `internal/engine` — runs one chat turn (`run.go`): checks run ownership, builds bounded prompt (profile, skill, memory, Space notes, ≤12 uncovered messages), streams via a `Generator`, persists the reply. Tool wiring lives beside it: `github_tools.go`, `mcp_tools.go`, `computeruse_tools.go`, `delegation.go` (≤3 handoffs/run), `routines.go`. Every tool call rechecks grants, Space binding, disclosure, and interactive-run status; Routines and delegated Dots get a reduced tool set.
- `internal/model` — provider adapters (OpenAI/xAI Responses via OpenAI SDK, Anthropic Messages + optional beta Threads, OpenRouter, OpenAI-compatible) behind one provider-neutral tool catalog (`tools.go`). Per-Dot provider/model override; secrets only from the system credential store (CLI: env vars).
- `internal/integrations` — `github`, `mcp` (loopback-only Streamable HTTP client), `computeruse` (Cua Driver), `remote` (iOS bridge).
- `internal/midnightsup` — opt-in supervisor for one user-approved (SHA-256) local Midnight server; platform code split in `proc_*.go`.
- `internal/desktopui` — MyGo views/actions/settings. Keep sections in focused files; don't grow `app.go`.

## Conventions specific to this repo

- Append dated entries to `HISTORY-TRACKER.md` for each work session; `CHANGELOG.md` is only for release-ready changes.
- `.gitignore` ignores `*.md` except `README.md`, `HISTORY-TRACKER.md`, and `AGENTS.md`, so this `CLAUDE.md` is untracked unless you add an exception (or `git add -f`).
- Commit style: short imperative summaries, optional `feat:`/`fix:`/`docs:` prefix.
