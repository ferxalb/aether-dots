# Repository Guidelines

## Project Structure

- `main.go` starts the native macOS desktop app; `cmd/aether/main.go` contains the CLI.
- `internal/desktopui/` holds MyGo views, settings, actions, and appearance. Keep UI sections in focused files rather than growing `app.go`.
- `internal/engine/` runs chat turns; `internal/model/` integrates the OpenAI SDK; `internal/store/` persists Dot profiles and chat history; `internal/task/` defines task data.
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

Keep model keys and GitHub access/refresh tokens out of task data, preferences, logs, and prompts. Desktop credentials belong in MyGo's system credential store; model environment variables remain the CLI fallback. The public GitHub App Client ID and App slug may come from `AETHER_GITHUB_CLIENT_ID` / `AETHER_GITHUB_APP_SLUG` or release linker flags; never include a GitHub App client secret or private key. Dot execution must not request, read, or write user-selected local files or folders and must not access arbitrary network destinations. The permitted remote integrations are:

- The explicitly authorized, read-only GitHub client using fixed GitHub OAuth/API hosts (plus the unauthenticated GitHub avatar host), after connection, repository selection, a per-Dot grant and disclosure, runtime rechecks, and separate Routine opt-in when applicable. GitHub data is untrusted evidence and model arguments cannot widen grants.
- The AETHER iOS bridge to the single HTTPS Cloudflare Workers origin embedded by release linker flags. It starts only after explicit desktop pairing, keeps the device token in MyGo's system credential store, and accepts only the typed mobile status, Dot-list, conversation, and message actions. Development override is allowed only for the loopback Cloudflare Worker on port 5173. It does not grant the bridge access to local files, arbitrary network destinations, shell commands, provider keys, or GitHub credentials; existing per-Dot GitHub grants remain separate.

- User-managed MCP servers through the dedicated Settings → MCP page. Entering that page or choosing Find servers performs a user-initiated scan of TCP listeners on `127.0.0.1` and `::1`, then attempts only Streamable HTTP MCP handshakes at `/mcp` and `/`. AETHER never scans remote hosts, launches a user-provided server command, uses stdio for user-configured servers, follows redirects, or stores MCP credentials. The user reviews discovered server and tool metadata, grants exact tool names to each Dot, and accepts that tool inputs and outputs may be sent to that Dot's model provider. MCP metadata and annotations are untrusted; a tool may act within the server's own scope, so grants must be explicit and runtime-rechecked before and after every call. Tool grants are bound to the Dot's Space and revoked when a tool definition changes or its server is removed. MCP tools are unavailable to Routines, delegated Dots, and iOS-bridge messages. The user-managed server remains outside AETHER's process and may itself have broader access than its advertised tools; disclose this boundary when asking the user to grant tools.

- Optional macOS Computer Use through the separately installed Cua Driver. The app may offer a user-initiated installer for the fixed upstream Cua Driver installer at `https://cua.ai/driver/install.sh`, using the stable channel and `--no-modify-path`; rely on the upstream SHA-256 release checks and verify the installed macOS app code signature without requiring a separately installed `cosign` binary. The installer is never callable by a Dot, and its arguments and network destinations must remain fixed. The app does not bundle Cua Driver. Disable the driver's optional usage telemetry during AETHER's user-initiated setup. Cua Driver must run with `bounded` permissions, an approved deny-by-default capability manifest limited to Apple Notes and Music, a restrictive argument policy, and the first-launch permission gate disabled. The initial Dot scopes are read-only access to the currently displayed Notes window and playback controls in Music. Do not expose screenshot, clipboard, file-transfer, browser, shell, script, unrestricted step-batch, process-termination, or arbitrary app-control tools. Do not request Screen Recording for the initial accessibility-tree and playback flow; open Accessibility settings after first install and explain how to add and enable CuaDriver, with the user explicitly toggling the grant. Disclose that accessible Notes content may be sent to the Dot's configured model provider, and treat all observed UI text as untrusted input. AETHER must recheck the per-Dot grant, Space binding, disclosure, allowed scope, and interactive-run status on every tool call. Computer Use is off for Routines and delegated Dots unless a separate reviewed opt-in design is added. Disconnecting or revoking a Dot grant removes its access immediately.

Do not add source-file, archive, workflow-log, generic shell, or write operations without a separate permission design. MCP tools are an explicit user-granted integration; do not implement a generic shell launcher or treat server tool annotations as enforcement. Cua Driver's fixed user-initiated installer is the only shell-based installation exception; it must use the upstream checksum verification, verify the installed macOS app code signature, and may not accept model-provided commands, paths, or arguments. Apple integrations and macOS permissions are allowed only within the Computer Use design above. Append dated engineering notes to `HISTORY-TRACKER.md`; reserve `CHANGELOG.md` for release-ready changes.
