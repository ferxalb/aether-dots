# AETHER Dots History Tracker

This is the chronological engineering work log for AETHER Dots. It records session-level additions, modifications, structural decisions, verification, and known gaps. Entries can describe implementation that is still in progress.

This file is separate from a product release changelog. `CHANGELOG.md` is reserved for version-specific changes that are complete and ready to ship as Beta, RC, or Final. No release version is being declared here, and this session did not create or update a `CHANGELOG.md`.

For each future work session, append a dated entry describing what changed, why, how it was checked, and what remains unverified. Keep older entries intact.

## 2026-11-03 (6) — Independent Dot models, prompt compaction, performance, and MCP

### Implementation

- Added a per-Dot provider/model override while keeping credentials in the system credential store. Chat, memory compaction, and the existing provider-neutral tool catalog follow the Dot's selected model; provider changes clear Anthropic thread references so a thread from a prior model cannot be reused. Changing a Dot or workspace provider also requires renewed MCP disclosure for affected Dots before their tools run again.
- Made chat compression cover the transcript through a recorded message ID. Compression processes bounded batches, preserves the full local transcript, only summarizes uncovered turns, and rejects the save if the transcript changes while generation is in progress. Editing saved memory clears its coverage marker so future prompts fail safe by retaining recent transcript context.
- Replaced JSON marshal/unmarshal deep copies in store snapshots and transactions with structural copies, compacted local JSON writes, and changed live reply updates to append each stream batch without recopying the accumulated response.
- Implemented an end-to-end user-managed MCP client using the pinned Go SDK's Streamable HTTP transport. It connects only to explicit `127.0.0.1` or `::1` endpoints, refuses redirects, caps response/event sizes, and never starts processes or stores MCP credentials. Users can discover tools, review schemas, grant exact tools per Dot, and accept the model-data disclosure. Runtime sessions are run-scoped; tool schemas are reconciled and changed grants revoked, and authorization is rechecked before and after each call. MCP remains unavailable to Routines, delegated Dots, and iPhone runs.
- Read the pinned MyGo v0.7.3 lifecycle/UI documentation, MCP Go SDK v1.8 transport/client and protocol documentation, and the provider function-calling documentation. Updated README and repository permission guidance with the implementation and its trust boundary: a local server may act within a broader scope than its advertised tools.

### Verification

- `gofmt`, `CGO_ENABLED=0 go build ./...`, and `git diff --check` passed. Automated tests were not run.
- No live user-managed MCP server was available. A source launch with an isolated temporary data directory did not surface a native AETHER window in the CUA app inventory, so the updated UI could not be previewed. Provider switching, model API calls, and local server behavior remain unverified at runtime.

## 2026-11-03 (1) — Bounded macOS Computer Use with Cua Driver

### Implementation

- Added a separate, user-initiated one-click Cua Driver installer for the stable channel. The installer is pinned to the upstream HTTPS URL, requires signature verification, leaves PATH unchanged, and disables optional driver telemetry. AETHER verifies the installed app signature and keeps the driver's MCP socket and policy files in a private app-data directory.
- Connected the installed driver through the official MCP Go SDK over its private local socket. Each runtime starts with a bounded, expiring capability manifest for Apple Notes and Apple Music plus a deny-by-default argument policy. Notes reads return bounded accessibility text from one visible window; Music exposes only play, pause, next, and previous. Screenshot capture, Screen Recording, clipboard, files, browser, shell, generic typing, and other apps remain unavailable.
- Added per-Dot scopes, Space binding, a Notes-to-model disclosure, runtime authorization rechecks, content-free access events, and immediate session invalidation when scopes change or are revoked. Computer Use is excluded from Routines, delegated Dots, and iOS-bridge runs. Added an Integrations card to install/update Cua Driver, open Accessibility settings after first install, cancel setup, and revoke every Dot grant.
- Replaced the old Apple-integration restriction in `AGENTS.md` with the Cua Driver permission and installation boundary, removed the Apple Notes placeholder, updated README setup/troubleshooting guidance, and migrated existing schema-v5 stores to schema v6 with no grants enabled.

### Verification

- Ran `gofmt`, `CGO_ENABLED=0 go build ./...`, and `git diff --check` successfully. Automated tests were not run.
- Cua Driver installation, code-signature verification against a downloaded release, and the macOS Accessibility flow remain unverified on this machine.

## 2026-11-03 (2) — Fix Cua Driver setup and first-use latency

### Implementation

- Traced the Integrations setup failure to `--require-signature`: the installer needs `cosign` on PATH for that flag, while AETHER supplies only macOS system paths and a clean install has no `cosign`. Removed that hidden prerequisite; the upstream installer still verifies release checksums, and AETHER verifies the installed app's macOS code signature.
- Added `pipefail` and bounded installer stderr in the setup error so failures show their actual cause instead of a generic connection message.
- Moved bounded-session warm-up off the install completion path and limits prewarming to installed drivers with an approved Dot grant. Existing grants warm in the background on app startup, and adding a scope preserves a warm session; removing access still stops it immediately.

### Verification

- `gofmt`, `CGO_ENABLED=0 go build ./...`, and `git diff --check` passed. Automated tests were not run.
- Cua Driver installation and the macOS Accessibility flow remain unverified on this machine.

## 2026-11-03 (3) — Computer Use diagnostic and regression coverage

### Implementation

- Added regression tests for the fixed installer contract, bounded/sanitized installer diagnostics, filtered installer and driver environments, Dot grant/disclosure change decisions, and the Notes/Music observation and action boundaries.
- Replaced startup's repeated per-Dot grant queries with one indexed pass over conversations and Dots. At 500 synthetic Dots, the benchmark measured about 19.6 µs versus 780 µs for the previous N+1 scan (about 40× faster).
- Ran a live system CUA Music play/pause check and restored playback to its original paused state. Built and opened the current AETHER bundle against an isolated temporary profile; its Integrations screen correctly showed Cua Driver as not installed.

### Verification

- `make bundle`, `CGO_ENABLED=0 go build ./...`, `CGO_ENABLED=0 go test ./...`, `go test -race ./...`, `CGO_ENABLED=0 go tool mygo vet .`, `gofmt`, and `git diff --check` passed.
- The AETHER Cua Driver installer could not be exercised because macOS was locked and the CUA runtime refused UI interaction. Cua Driver is absent; no Notes content was read and no Accessibility permission was changed. Resume the end-to-end integration test after the Mac is unlocked and the user enables CuaDriver under Accessibility.

## 2026-11-03 (4) — Validate the official macOS driver layout

### Implementation

- Retried the install through AETHER after the Mac was unlocked. The official installer placed `cua-driver` inside `/Applications/CuaDriver.app` and created `~/.local/bin/cua-driver` as a symlink to that executable. AETHER's path validator accepted only `~/.cua-driver/packages`, so it rejected this official layout after the download had completed.
- Updated the validator to allow only a binary inside the managed package directory or the exact executable inside the signature-verified CuaDriver app. The packaged Integrations UI now detects the installed driver, and a host smoke test verifies its macOS code signature.
- Added an opt-in live Music integration test that creates a temporary disclosed Dot grant, exercises AETHER's actual per-Dot tool handler, and restores playback to its initial state. It is skipped in normal test runs unless `AETHER_RUN_CUA_LIVE=1` is set.

### Verification

- The Cua Driver installation completed through AETHER's button; the rebuilt app showed `Installed · ready when granted`. The installed bundle's code signature check passed.
- Full tests, race tests, MyGo vet, `CGO_ENABLED=0 go build ./...`, and `make bundle` passed after the path fix.
- Accessibility currently lists no CuaDriver entry. The user must add `/Applications/CuaDriver.app` and explicitly enable it before AETHER's bounded session can start. No Notes content was read, and Screen Recording remains off.
- The opt-in live Music test compiled and correctly skipped without its explicit environment flag; it has not been run because the required Accessibility grant is still pending.

## 2026-11-03 (5) — Narrow and speed up Music computer use

### Implementation

- The live diagnostic showed that AETHER's first `list_apps` call exceeded the bounded manifest because global app inventory requires desktop-display observation. Replaced it with the approved app's `launch_app` response, which supplies the app PID and window metadata. AETHER now avoids unrelated app discovery and can skip a separate window-list call when that response already includes a unique target window.
- Kept Notes restricted to a visible current-Space window. Music uses an app-scoped window, prefers an on-screen window when one exists, and gives a direct recovery message when the player window exposes no unambiguous playback button. Added exact playback labels for common English, Spanish, Portuguese, French, German, and Italian localizations.
- Removed a legacy `post_action_wait` click argument and matching policy rule; the installed Cua Driver 0.38 schema rejects that unknown field.
- Updated README troubleshooting to explain that Music needs its main player window open.

### Verification

- `CGO_ENABLED=0 go test ./...`, `CGO_ENABLED=0 go test -race ./...`, `CGO_ENABLED=0 go build ./...`, `CGO_ENABLED=0 go tool mygo vet .`, `make bundle`, `gofmt`, and `git diff --check` passed.
- The opt-in AETHER test reached the bounded Music accessibility tree with Accessibility on and Screen Recording off. The live system check found Music running with zero open windows; the returned tree contained no unambiguous playback control, so AETHER correctly stopped before sending a Music action. No Notes content was read and playback was not changed. Rerun the live play/pause/restore sequence after the user opens Music's main player window.

## 2026-11-02 (5) — Official MCP Go SDK foundation

### Implementation

- Added `github.com/modelcontextprotocol/go-sdk` v1.8.0 as a direct dependency and introduced `internal/integrations/mcp.NewClient` as an inert client base. It advertises no capabilities (including the SDK's default roots capability), selects no transport or destination, and does not connect.
- Added the SDK's upstream license text and its license notice to `resources/licenses/`.
- Kept MCP connections and Dot-visible tools outside this change; they require separate transport and permission design under the repository's integration boundaries.

### Verification

- `gofmt`, `git diff --check`, and `CGO_ENABLED=0 go build ./internal/integrations/mcp` passed.
- `CGO_ENABLED=0 go build ./...` did not pass because the existing untracked `internal/integrations/computeruse/service.go` refers to a missing `toolCallResult.Text` field at lines 276, 278, and 281. This file was not changed in this session.
- Automated tests were not run.

## 2026-11-02 (4) — Provider SDKs, bounded memory, and visible run activity

### Implementation

- Added Anthropic's official Go SDK and native streaming Messages adapter, with the existing bounded client-side function catalog, streamed tool loops, secret redaction, retry/timeout configuration, adaptive thinking, and model-aware effort levels. Unsupported Claude effort levels are omitted so a model is not sent an invalid setting.
- Added optional Anthropic beta Message Threads using the official SDK helper. Threads are off by default; Settings explains server expiry, local-memory replay, lack of early deletion, billing, encryption, and Zero Data Retention eligibility. The local cache is bounded, process-only, keyed without storing raw credentials, and cleared when the option is disabled.
- Added xAI Grok through the official OpenAI Go SDK and xAI's fixed Responses API origin. Requests set `store=false`, use conversation prompt affinity, stream text, and keep xAI-hosted web and X search tools disabled. Provider-specific keys remain in the system credential store.
- Reduced repeated prompt payloads while preserving the complete transcript locally: active turns include a capped set of recent messages, optional editable Dot memory, and user-authored shared Space notes. Memory generation is a model-drafted local note that remains editable.
- Added an Activity view with run controls and local approximate latency/token metrics, plus per-conversation run guards so separate Dots can reply concurrently. Lists follow stable IDs and preserve reading position while their rows change.
- Updated provider settings, privacy copy, README, CLI configuration, and the recent-context store expectation. The earlier (3) entry describes the state before this implementation.

### Verification

- `gofmt`, `git diff --check`, `CGO_ENABLED=0 go build ./...`, and `CGO_ENABLED=0 go tool mygo vet .` passed after the provider, context, metrics, and UI changes.

### Not verified in this session

- Automated tests, native visual preview, and live model-provider requests were not run. Anthropic and xAI connections require user API credentials; optional Anthropic Message Threads also require the current beta API availability.

## 2026-11-02 (3) — Long-chat UI performance review

### Chat rendering

- Replaced the eagerly built chat transcript column with MyGo v0.7.3's virtualized `ui.List`, keyed by saved message ID. The list builds only visible message rows plus its small overscan, while retaining access to the complete transcript.
- Kept the live streamed reply as a keyed list row. The list follows new content while the user is at the end, preserves the reading position when they scroll upward, and jumps to the end when they send a new message.
- Precomputed task-to-conversation and latest-status indexes when the saved snapshot refreshes, so each streamed text update does not rescan all task records.
- Left prompt/history semantics unchanged. Every saved conversation turn still reaches the model; this pass changes rendering cost only.

### System review findings

- The desktop currently has OpenAI Responses, OpenRouter Chat Completions, and a custom OpenAI-compatible Chat Completions adapter. xAI Chat Completions can use the existing custom adapter; Claude's OpenAI SDK compatibility is documented for evaluation with limitations, so full Claude support should use the native Messages API. xAI's server-side web/X search tools are not exposed through the current Chat Completions adapter.
- Current turns assemble all prior conversation messages into each provider prompt, up to a 16 MiB local safety limit. Long conversations can increase latency and exceed a model's context window. OpenAI and xAI document in-memory WebSocket continuation while `store=false`; Anthropic's beta message threads send only new turns but are not ZDR-eligible. These transport optimizations need a per-provider opt-in, expiry/replay handling, and a clear explanation of provider-side memory before adoption. A future compaction design should preserve the full local transcript and show users what summary is sent.
- OpenAI Dots, Claude Duet, and Grok Bot emphasize persistent work across devices, proactive progress/task views, reusable memory, schedules, and broad app connectors; all rely on cloud execution environments. AETHER already has local chats, routines, bounded specialist handoffs, and mobile/web control, while preserving a much narrower local permission model. The next product gap to explore is durable, visible task progress and reusable user-authored Space context. Any first knowledge feature should use text the user explicitly adds; local file indexing requires a separate permission design.

### Verification

- `gofmt` and `git diff --check` passed.
- `CGO_ENABLED=0 go build ./...` passed.
- Automated tests and a native UI preview were not run in this pass.

## 2026-11-02 (2) — Scoped iPhone relay and connected dashboard

### Implementation

- Added an iPhone pairing card to native Settings and a narrowly scoped desktop relay client. The desktop accepts only a build-time Cloudflare Worker origin, stores the device token in Keychain, and exposes fixed status, Dot listing, conversation reading, and message-sending actions. It grants no file, shell, arbitrary network, or app-control capability.
- Added the Expo companion sign-in and workspace flows. Browser authorization uses a verifier-bound, one-time handoff; the app stores its opaque session token in iOS SecureStore and can revoke it on sign-out.
- Added the Worker dashboard and API routes for mobile approval, pairing, device/session revocation, and recent activity. The Worker forwards a small request allowlist to the Neon Function; the Function requires the Worker origin secret, verifies owner sessions, bounds command payloads and retention, and applies pairing and authorization rate limits.
- Added migrations `0001` through `0004`, the hourly cleanup trigger, setup and privacy documentation, and a local development path. Updated this project's security boundary to explicitly allow only the scoped Aether relay connection.

### Verification

- Desktop: `gofmt`, `git diff --check`, `CGO_ENABLED=0 go test ./...`, `CGO_ENABLED=0 go build ./...`, `CGO_ENABLED=0 go tool mygo vet .`, and race tests for desktop UI and remote integration packages passed.
- Mobile: `pnpm run lint`, `pnpm exec tsc --noEmit`, and `git diff --check` passed.
- Web: `pnpm run typecheck`, `pnpm run build`, and `git diff --check` passed. Local pairing, scheduled cleanup, one-time mobile exchange, logout, and replay checks passed. The final direct Function smoke returned 401 for missing origin/auth/maintenance credentials and 200 for health and authorized dev cleanup.
- Deployed Neon Function deployment 6 is active on the temporary `aether-bridge-dev` branch; migrations `0001` through `0004` were applied to that dev branch only. Restored the local `.neon` selector to `production` with `--no-env-pull`; production data was not migrated or changed.

### Not verified or not deployed

- The Cloudflare Worker was not deployed because Wrangler is not authenticated in this environment. The local Worker source is configured, but publishing it requires the existing Cloudflare login.
- The native preview was stopped after startup attached to an existing AETHER workspace, so the new Settings card was not visually verified in an isolated window.
- No production Neon schema migration, production Function deployment, or end-to-end iPhone-to-desktop session was performed.

## 2026-11-02 (1) — Native integrations and read-only GitHub

### GitHub integration

- Added a fifth Settings tab, **Integrations**, with a native GitHub card and clearly disabled Apple Notes and Apple Reminders cards. Upcoming Apple integrations request no macOS permissions.
- Implemented GitHub App Device Flow using the public Client ID and `offline_access`, GitHub-directed polling intervals and `slow_down`, cancellation, expiring user tokens, refresh-token rotation, and a separate MyGo credential-store service (`com.aetherdots.github`). No client secret or private key is used.
- Added environment and MyGo linker-flag configuration for the public App Client ID and slug. Missing configuration produces a visible setup state.
- Added authenticated user, accessible App installation, repository discovery, and selection. Authenticated requests use fixed GitHub hosts, bounded responses, no redirects, request timeouts, cancellation, GitHub API version headers, pagination, and sanitized error mapping.
- Added explicit per-Dot grants tied to stable repository IDs, an AI-provider disclosure, same-Space binding, current selection and installation checks, immediate local revocation, and a separate opt-in for scheduled Routines. Specialists receive only their own grants.
- Added bounded provider-neutral read-only tools for repositories, issues, pull requests, commit metadata, check runs, Actions runs, and job summaries. They return GitHub links, mark remote data as untrusted, and record read activity without storing issue or repository text. Source files, diffs, workflow logs, archives, and write operations remain unavailable.
- Added matching CLI commands for login, status, refresh, selection, Dot grants, Routine opt-in, revocation, and local disconnect.
- Bumped task-store schema to v5. Older stores migrate with GitHub disconnected and no selected repositories, Dot grants, or Routine opt-ins. Credentials remain outside the JSON task store.

### Apple integration research and documentation

- Documented the development Mac's Notes scripting dictionary as an observation, not a cross-version guarantee. A future read-only Notes connector requires explicit Apple Events authorization and the signed app's usage description; private Notes databases and Full Disk Access remain out of scope.
- Documented a future EventKit Reminders path with explicit opt-in, selected lists, `NSRemindersFullAccessUsageDescription` on current macOS, and no Calendar access. Notes and Reminders are not implemented in this milestone.
- Updated README setup, permissions, configuration, consent, CLI use, disconnection, troubleshooting, schema migration, and known limitations. Updated the project `AGENTS.md` to permit only the fixed-host, explicitly authorized GitHub client.
- Refreshed stale assertions to test the current settings label, optional provider-default output limit, and conversation context assembled at request time.

### Verification

- `gofmt` completed for changed Go files; `git diff --check` passed.
- `CGO_ENABLED=0 go test ./...` passed.
- `CGO_ENABLED=0 go build ./...` passed.
- `go tool mygo vet .` passed.
- `CGO_ENABLED=1 go test -race ./internal/integrations/github ./internal/store ./internal/engine ./internal/desktopui` passed.
- `make bundle` built and signed `build/darwin-amd64/AETHER Dots.app`; `codesign --verify --deep --strict` passed. The bundle contains no Notes or Reminders privacy usage strings because those integrations are not enabled.
- Launched the bundle and inspected the native Settings and Integrations UI. No real GitHub account flow was started.

### Not verified in this session

- Live Device Flow authorization, App installation, real repository discovery, token rotation against GitHub, and model-provider calls require a registered GitHub App Client ID and user account; no credential was available here.
- Production Developer ID signing, notarization, DMG packaging, and a non-macOS build were not performed.

## 2026-11-01 (19) — Permissioned Dot handoffs and provider-neutral tools

### Session objective

Make AETHER Dots: independent specialists, clearer task handoffs, provider-neutral capabilities, and auditable permission checks.

### Implementation

- Added a shared model function catalog. OpenAI Responses and OpenAI-compatible Chat Completions now use the same local time, calculator, and request-scoped tool contracts. Function calls have bounded arguments, outputs, rounds, and call counts; parallel execution is disabled for Responses.
- Added directional specialist grants in each Dot profile. Grants are empty by default, same-Space only, and limited to 12 peers. Moving a Dot clears old grants to it. A run can hand off up to three typed tasks, one level deep.
- The store rechecks each grant atomically when creating a handoff. A request contains only the task, constraints, and expected output; it does not copy the requesting Dot's full transcript. The receiving Dot uses its own profile and history.
- Saved incoming requests and specialist results in both conversations with source attribution. Accepted, rejected, failed, and completed handoffs are recorded in the task event chains. Routines and CLI runs use the same per-task capability binding.
- Added specialist selection and privacy details to Customize, plus visible handoff attribution and a “consulting …” run status in chat. Bumped the store schema to v4; schema v3 skills and routines are preserved and receive no automatic grants.
- Kept the existing no-local-files boundary. No shell, browser, account, or arbitrary-network connector was added.

### Verification

- `gofmt`, `git diff --check`, and `CGO_ENABLED=0 go build ./...` passed.
- Automated tests, live provider function calls, and a native window preview were not run. Provider-specific function-call behavior and visual layout remain unverified against configured accounts and the running macOS app.

## 2026-11-01 (18) — Reasoning effort across model providers

### Session objective

Apply the thinking-effort preference to OpenAI, OpenRouter, and custom OpenAI-compatible requests, using model capability metadata where available.

### Implementation

- Kept OpenAI Responses effort settings and added `reasoning_effort` to streamed OpenRouter and custom-compatible Chat Completions requests through the OpenAI Go SDK.
- Added a “Use model default” choice for compatible providers so the request can omit the effort field. The CLI accepts `AETHER_REASONING_EFFORT=default` for the same behavior.
- Read OpenRouter's reasoning capability and supported-effort metadata. Known unsupported values are omitted; supported effort levels are reflected in Settings, while manually entered model IDs remain user-configurable with an explicit unknown-support note.
- Kept reply verbosity provider-specific to OpenAI Responses because the compatible Chat Completions path does not guarantee that parameter.

### Verification

- `gofmt`, `CGO_ENABLED=0 go build ./...`, `CGO_ENABLED=0 go tool mygo vet .`, and `git diff --check` passed.
- Automated tests and live model requests were not run; endpoint-specific reasoning support remains unverified without configured credentials.

## 2026-11-01 (17) — OpenRouter model dialog and streamed Markdown chat

### Session objective

Improve OpenRouter model selection and shared dropdown styling, stream responses in the chat, and render Markdown efficiently as replies arrive.

### Implementation

- Moved OpenRouter catalog search and model selection into a keyboard-accessible MyGo modal. All catalog matches remain searchable and are rendered in a virtualized list; users can still enter an exact model ID.
- Added one shared `SelectBase` dropdown component and moved provider, appearance, profile, routine, and response-option selectors onto it.
- Streamed OpenRouter and custom-compatible Chat Completions text deltas from the official OpenAI Go SDK adapter as they arrive. Kept streamed API keys redacted across chunk boundaries and capped cumulative response output at 2 MiB.
- Batched live UI updates at 25 frames per second and used MyGo's incremental Markdown `Stream`/`View` widgets with stable assistant row identities. Saved replies now display the full bounded response as Markdown.
- Updated README documentation for the model dialog, stream behavior, and Markdown rendering.

### Verification

- `gofmt`, `CGO_ENABLED=0 go build ./...`, `CGO_ENABLED=0 go tool mygo vet .`, and `git diff --check` passed.
- Automated tests were not run. No live provider request or native window preview was made; streamed runtime behavior and visual layout remain unverified against a configured provider/window.

## 2026-11-01 (16) — Provider defaults and OpenRouter model discovery

### Session objective

Use the selected model's own context and reply defaults, make OpenRouter model selection provider-wide rather than OpenAI-specific, and preserve chat data when providers or models change.

### Implementation

- Removed the implicit 8,192-token output cap. A blank optional setting now omits the output-token parameter for both OpenAI Responses and compatible Chat Completions requests. Existing unmarked 8,192 settings migrate to the provider/model default; explicit limits remain selectable.
- Removed the hard-coded OpenRouter model default. Settings can load and search OpenRouter's authenticated model catalog for text-input/text-output Chat Completions models, showing published context size and whether native functions are supported. Models without declared function support receive no function definitions.
- Kept conversation messages durable and provider-neutral. New runs assemble the complete saved conversation at request time, subject to a 16 MiB local prompt safety ceiling; oversized conversations fail without truncating or deleting saved messages.
- Kept OpenRouter catalog credentials in the system credential store, sent keys only as HTTPS authorization headers, blocked redirects, bounded catalog response sizes, and avoided logging response bodies or keys.
- Updated Settings, CLI help, README, and this history entry to describe model defaults and catalog behavior.

### Verification

- `gofmt`, `CGO_ENABLED=0 go build ./...`, `CGO_ENABLED=0 go tool mygo vet .`, and `git diff --check` passed.
- Automated tests were not run. No live OpenRouter catalog request or native window preview was made; account authorization and catalog/UI behavior remain unverified against a real key.

## 2026-11-01 (15) — Provider adapters and secure model settings

### Session objective

Replace the single-provider settings path with real, swappable OpenAI, OpenRouter, and custom OpenAI-compatible connections while preserving local chat history and provider credentials.

### Implementation

- Added a provider-neutral runtime contract. OpenAI continues through the official Go SDK Responses API; OpenRouter and custom compatible endpoints use the official SDK's streamed Chat Completions API.
- Added provider selection, separate model settings per provider, custom base URL settings, provider-specific credential-store entries, and CLI environment selection. Provider changes do not rewrite existing Dot or conversation records. OpenRouter defaults to `openai/gpt-6.1-sol`; custom endpoints require a model ID and tool-call support.
- Added a local function-call loop for compatible providers with only current time and bounded arithmetic functions. It rejects unknown functions and malformed/oversized arguments, limits calls and rounds, and grants no file, shell, account, or network access.
- Kept keys out of preferences and task data. Custom remote endpoints require HTTPS, loopback HTTP is allowed for local servers, API redirects are blocked, and configured keys are redacted from provider errors and generated text before reaching the UI or conversation store.
- Documented provider setup, CLI environment variables, key storage, and function limits.

### Verification

- `gofmt`, `CGO_ENABLED=0 go build ./...`, `CGO_ENABLED=0 go tool mygo vet .`, and `git diff --check` passed.
- Automated tests were not run. No live provider request or native window preview was made; credentials and endpoint-specific behavior remain unverified in this session.

## 2026-11-01 (14) — ADT full-component adaptation and responsive feedback

### Session objective

Adapt the full native ADT interaction and layout to feel more responsive, accurate, and usable.

### Implementation

- Traced the slow morph to MyGo's `ElementTransition.Size` and `Motion.Collapse`: size transitions lay out the element at every intermediate size. Removed animated size and collapse from ADT, made the newest notice's details and actions available immediately, and kept at most one stack item expanded as hover moves between notices.
- Kept entry, exit, stack-position, color, and drag-return motion short (75–110 ms). Entry and exit fade and move without collapsing; expansion changes geometry once instead of animating every frame. Cards keep the title first with details below, including when the stack is anchored at the bottom.
- Left MyGo in charge of toast timing, focus, dismissal, and actions, while painting the card stack last in the app's root layer. This keeps bottom notices visible above the native settings scroll area.
- Replaced the hand-rotated loading icon, whose `Context.AnimationFrame` requested a full view build on every display refresh, with MyGo's native accessible spinner that schedules paint-only updates at its own cadence.
- Replaced rune-count layout estimates with MyGo text measurement for pill width and expanded description height, including actual font metrics and wrapping. Kept a close control visible on persistent loading notices so they can be dismissed without a pointer gesture.
- Added an “Add API key” action to the Settings saved notice when neither Keychain nor the environment has a key; activating it opens AI & responses and resets that section to the top.
- Replaced MyGo's generic inverse toast fill with the active theme surface, text, muted text, border, and theme-aware shadow. Status colors stay on the icon badge; titles use readable primary text and actions use the palette accent. Removed animated card colors and text crossfades, leaving a short position/opacity entrance and a brief body fade.
- Precomputed each conversation's latest task status once per view and reused it in the header and sidebar. Replaced legacy-task conversation checks that repeatedly scanned all conversations with a single lookup map; avatar ink now switches between light and dark for contrast.

### Verification

- `gofmt`, `git diff --check`, `CGO_ENABLED=0 go build ./...`, and `make bundle` passed; APIs were checked against pinned MyGo v0.7.3. Native preview confirmed the Settings saved notice renders over Preferences in dark Violet with a theme-matched surface, readable description, violet action, semantic success badge, and accessible dismiss control. Automated tests were not run. No frame-time recording was captured; animation changes were verified from MyGo transition behavior and the bundled native preview.

## 2026-11-01 (13) — ADT dynamic behavior and reusable API

### Session objective

Refine ADT into a reusable, animated notification component with useful behavior for real app feedback.

### Implementation

- Added six configurable corner and center positions, per-notice theme and icon overrides, duration and expansion controls, sticky loading states, action buttons, stable-ID updates, and dynamic loading-to-result transitions.
- Matched the compact pill-to-card interaction with spring-driven width, height, position, and color changes; notices expand on hover and briefly on arrival, then collapse before expiring. Only one stack item auto-expands at a time, and hovering an older notice makes it the active item. Added swipe dismissal and semantic status icons.
- Sized the pointer-pause viewport to the visible stack instead of the full window, so notices no longer pause while the pointer is elsewhere in the app.

### Verification

- `CGO_ENABLED=0 go build ./...`, `make bundle`, and `git diff --check` passed. Reviewed the expanded success notice in the bundled native app at the minimum window size and confirmed it responds to the active theme.
- Tests were not run. No live model request was made.

## 2026-11-01 (12) — AETHER Dynamic Toast

### Session objective

Replace persistent inline status banners with reusable, contextual toast notifications.

### Implementation

- Added the ADT component with info, success, warning, and error variants, unique/updateable IDs, optional actions, manual dismissal, and configurable placement and width.
- Styled cards from the active theme, with semantic colors, status icons, soft depth, and spring entry/exit motion that follows the system's reduced-motion preference.
- Routed Settings, skills, routines, and workspace feedback through ADT. Settings notices appear at the lower right so they don't cover the heading or controls.

### Verification

- `CGO_ENABLED=0 go build ./...`, `make bundle`, and `git diff --check` passed. Reviewed the native Settings confirmation toast in the bundled app at the minimum window size.
- Automated tests were not run. A live OpenAI request was not made.

## 2026-11-01 (11) — User-centered Settings redesign

### Session objective

Replace the technical, crowded Settings experience with a clearer user-facing layout based on the supplied screenshots.

### Implementation

- Removed the irrelevant “Chat only · no file access” sidebar tagline and organized Settings into General, AI & responses, Skills, and Routines sections with persistent navigation and save actions.
- Rewrote model and privacy descriptions in plain language, tucked advanced response controls behind an optional disclosure, and clarified appearance choices.
- Reworked skill and routine creation into focused empty states and edit forms; added routine frequency presets and clearer run/status feedback.

### Verification

- `git diff --check`, `CGO_ENABLED=0 go build ./...`, and `make bundle` passed. The native app and Settings screens were reviewed at 1120×760 and 760×560.
- Tests were not run in this session.

## 2026-11-01 (10) — Shared skills, routines, and model controls

- Added shared skill and routine models, validated store operations, schema 3 migration, explicit `@skill[Name]` selection scoped to one turn, and a shared engine path used by both CLI and desktop. Local routines can run on intervals while the app is open or `aether routines serve` is running; they skip missed intervals, retain 20 recent runs, and use per-routine and global hourly caps.
- Added CLI commands and desktop Settings forms for managing skills and routines, testing or pausing routines, and inspecting recent outcomes. Added shared generation controls for reasoning effort, response detail, output-token limit, retry count, and timeout, with desktop preferences and CLI environment variables.
- Kept Dot execution limited to its profile and conversation context. Routines do not use event/webhook triggers or connectors, and they cannot access user files, folders, apps, or accounts. No live model request or native macOS window was used for verification.
- `CGO_ENABLED=0 go test -count=1 ./...`, `CGO_ENABLED=0 go test -race ./...`, `CGO_ENABLED=0 go build ./...`, `CGO_ENABLED=0 go tool mygo vet .`, `make cli-help`, and `git diff --check` passed. `TestWriteUIPreviews` passed; dark and light MyGo-rendered Settings, skills, and routine screens were reviewed at 1120×760 and 760×560. Preview files are in `/tmp/aether-dots-ui-review/`.

## 2026-11-01 (09) — Restore window dragging on the compact header

- Made the main header a window drag handle so users can move the window from its open area without restoring the removed blank inset. Header buttons remain interactive; the separate sidebar titlebar drag area remains available.
- `CGO_ENABLED=0 go test ./...`, `CGO_ENABLED=0 go build ./...`, `CGO_ENABLED=0 go tool mygo vet .`, and `git diff --check` passed. The updated MyGo preview was rendered; no live drag gesture was captured in this pass.

## 2026-11-01 (08) — Remove the main-pane titlebar gap

- Removed the empty titlebar spacer from the main pane so the Dot header begins at the top of the window. Kept the native titlebar drag region in the clear sidebar band, away from header actions.
- Left Customize trailing in the main header; the previous request was to remove the vertical band above the Dot identity.
- `CGO_ENABLED=0 go test ./...`, `CGO_ENABLED=0 go build ./...`, `CGO_ENABLED=0 go tool mygo vet .`, and `git diff --check` passed. Reviewed a MyGo-rendered dark chat preview; no live native window was captured.

## 2026-11-01 (07) — Compact Dot header and floating chat composer

- Moved the Customize action beside the Dot identity in the chat header, removing the large empty stretch across the top bar. The profile view keeps its Back to chat action on the trailing edge.
- Removed the chat footer's full-width background and divider. Inset the composer as a softly raised, rounded surface so it reads as a floating input while remaining available at the minimum window size.
- `CGO_ENABLED=0 go test ./...`, `CGO_ENABLED=0 go build ./...`, `CGO_ENABLED=0 go tool mygo vet .`, and `git diff --check` passed.
- Rendered and reviewed dark and light chat previews at 1120×760 and 760×560 with MyGo's headless renderer. No live native window was captured in this refinement.

## 2026-11-01 (06) — Chat-first Dot redesign and removal of folder access

- Replaced the task-first workflow with persistent Dots that are created immediately from New Dot, then opened as conversations. Users can set a Dot's name, icon, color, objective, working instructions, and Space; a normal chat prompt starts its work and can refine the profile over time.
- Redesigned the shell, searchable Dot list, empty state, chat transcript, composer, and profile editor around a clearer hierarchy and quieter surfaces. Added direct Space management inside the profile editor and a `⌘N` shortcut for creating a Dot.
- Removed user-selected folder reads and writes from the desktop, CLI, task model, engine, model context, and execution UI. Model requests now contain the Dot profile and bounded conversation context only. The app retains its own local conversation store and does not expose that store as Dot file access.
- Added schema migration for old task data: verify the existing event history, clear old grants and artifacts, replace old task text with generic legacy activity, then persist the new chat-only schema. Removed folder-based execution and Routine commands.
- Used the durable teammate framing from [xAI's bot guide](https://docs.x.ai/grok-bot/bots) as a reference for creating a reusable Dot first and personalizing its role and identity afterward.
- Verification passed: `CGO_ENABLED=0 go test ./...`, `CGO_ENABLED=0 go test -race ./...`, `CGO_ENABLED=0 go build ./...`, `CGO_ENABLED=0 go tool mygo vet .`, and `git diff --check`.
- Rendered and reviewed MyGo previews for Chat, Profile, and Welcome in dark and light appearance at 1120×760 and 760×560. The minimum-height profile uses a scrollable editor; no live native macOS window was captured during this pass.

## 2026-11-01 (05) — Persisted Dot conversations

- Added an optional Conversation record for a named Dot's purpose, execution mode, Space, and exact selected source/output folders. Existing `dots.json` records remain readable at schema version 1; legacy Dots continue to appear in history.
- Each chat submission is atomically saved with one linked child Dot in `Ready` state. The child inherits the Conversation grants, records its prior run link, and runs through the existing engine. The store reserves one queued/running conversation delegation at a time.
- Model-mode runs receive the saved purpose, newest request, and up to six recent user/verified-result messages within a 16 KiB context budget; older messages may be omitted and long entries are excerpted. Local mode retains fixed marker extraction and receives only the saved purpose and current request.
- Successful assistant messages contain the exact persisted artifact bytes only after the desktop rereads the artifact under its approved output folder and verifies its size and SHA-256. Interrupted linked runs are marked Paused at desktop startup with a persisted event; CLI store reads do not perform this recovery.
- `CGO_ENABLED=0 go test -race ./internal/store ./internal/engine` and `CGO_ENABLED=0 go test -race ./internal/desktopui` passed during the conversation-state work.

### Desktop messenger UI

- Split the workflow into a compact Dot Creator and a dedicated conversation page. Creating a named Dot saves its purpose, Space, execution mode, and fixed folder grants, then opens an empty chat; a task starts only after the user sends a message.
- Replaced the icon-only rail and form-first home with a persistent 280 px Dots sidebar. It has global Search, a compact create button, readable named Conversation rows with latest message/status and time, retained legacy task history under Older Dots, and Settings and Routines footer links.
- The chat uses a quiet fixed toolbar, selected Dot identity/status, a scrollable message and verified-result transcript, and a pinned composer. The editor draft is restored per Conversation; startup selection and new chats follow the transcript end.
- Added a Details route for saved grants, linked task runs, and Routines. The sidebar Routines route preserves access for users with legacy Dots and no Conversations. Creator space management is an explicit disclosure with add and rename controls.
- Tuned Obsidian dark mode to neutral charcoal: #222222 main canvas, #141414 sidebar, #2C2C2C surfaces, and #3B3B3B borders. System appearance remains the default; saved preferences and the other palettes are preserved.
- Kept the existing source/output scope model and run engine. Sidebar previews are bounded, long session names truncate within fixed-width rows, and timestamps retain enough width to remain readable.

### Final verification

- `CGO_ENABLED=0 go build ./...`, `CGO_ENABLED=0 go test ./...`, `CGO_ENABLED=0 go test -race ./...`, `CGO_ENABLED=0 go tool mygo vet .`, and `git diff --check` passed.
- The opt-in `TestWriteUIPreviews` render passed for dark/light Chat and Creator routes at 1120×760 and 760×560. Reviewed previews include `/tmp/aether-ui-review/chat-dark-Dark-minimum-760x560.png`, `/tmp/aether-ui-review/chat-dark-Dark-normal-1120x760.png`, `/tmp/aether-ui-review/creator-dark-Dark-minimum-760x560.png`, and `/tmp/aether-ui-review/creator-dark-Light-minimum-760x560.png`.
- No live macOS window launch or native screenshot was captured during this pass; visual review used MyGo's headless `ui.Render` previews. The min-size Creator and chat layouts were checked in those rendered previews.

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


## 2026-11-06 — Midnight integration (CLI)

- New CLI commands: `mcp [list|add|refresh|remove|grant|revoke]`, `midnight enable`, `memory [remember|recall|list|forget]`, `doctor`/`status`, `init`, `model show`. Interactive CLI runs now load the Dot's granted loopback MCP tools through `engine.WithMCPTools`, the same path and grant/fingerprint/disclosure checks as Desktop; Routines and delegated Dots still get none. The shared tool-catalog converter moved to `internal/integrations/mcp` and `store.SetConversationMCPGrants` mirrors the profile editor's validation.
- Store: `dots.json` writes now take an exclusive `flock`, reload if another process changed the file, then apply and persist, so Desktop and CLI no longer overwrite each other (tests: two handles, four concurrent handles). `RecoverInterruptedRuns` uses the same path. Known gap: Desktop startup still marks any `running` Dot as interrupted, including one a live CLI process owns.
- Fixed `send <dot-id> --message …` (flags after the positional were rejected).
- `cmd/aether-mockllm` (offline OpenAI-compatible mock), `scripts/e2e-midnight.sh` (real binaries: remember → restart Midnight → grounded recall → supersede → forget → scope denial → outage recovery) and `scripts/bench-e2e.py`. Midnight must be started by the user: `aether-midnight serve-http --memory-scope 'dot:<id>'`.
- Not verified: macOS Desktop UI; `internal/desktopui` `TestDotProfileCanBeCustomizedFromChat` fails on unmodified `main` in this headless sandbox (viewport scroll).

## 2026-11-06 — Alpha hardening: authenticated MCP + run ownership (Linux)

- **Authenticated Midnight sessions.** Midnight's HTTP server now requires `Authorization: Bearer mdn_…`; tokens carry per-token tool, memory-scope and repo policy and are enforced server-side (the scope in model-generated tool arguments never grants access). `internal/integrations/mcp` gained `WithBearer`, `SaveToken/LoadToken/DeleteToken` (0600 files under `<data-dir>/mcp-tokens/<server-id>`, never in `dots.json`, listings, events, or logs; a loosened mode fails closed) and `ErrUnauthorized`. `store.MCPConnectOptions` feeds Desktop settings, the engine and the CLI. `aether midnight enable <dot> --endpoint … --token-file F|--token-stdin` creates one server record per Dot (`…/mcp/dot-<id>`), so each Dot has its own token, catalog and grants; failures roll back the record. `mcp add` takes the same token flags; `memory` commands route only through the Dot's own granted server. `doctor` reports `REJECTED` for revoked/expired tokens.
- **Run ownership.** A Dot is `Running` only while a live process holds an exclusive flock on `runs/<dot-id>.lease` (taken inside `SetStatus(Running)`, released on any other status). `RecoverInterruptedRuns` (Desktop startup), `RecoverOrphanedRuns` (`doctor`, `send`) and `engine.Run` pause only runs whose owner is gone; a Dot running in a live CLI/Desktop process is untouched and cannot be started twice. kill -9 releases the lock, which is the crash detector. Legacy builds that never took a lease look orphaned if still running.
- E2E (`scripts/e2e-midnight.sh`, real binaries, offline mock model): two isolated Dots, forged scopes, 401s, revocation/rotation, token-leak greps, same-port restart, supersede/forget, 8 concurrent CLI writers, live-run protection, kill -9 recovery, SIGINT pause, Midnight outage.
- Not verified: macOS Desktop runtime; the one `internal/desktopui` viewport test still fails on unmodified `main` in this headless sandbox.
