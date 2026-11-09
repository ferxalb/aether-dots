# AETHER Dots

AETHER Dots is a native macOS workspace for focused AI teammates. Each Dot has its own name, icon, objective, working instructions, optional model override, memory, tool grants, and continuing chat. The desktop UI is drawn by MyGo in Go; it is not a WebView.

## Quick commands

```sh
make             # list commands
make dev         # open the live-reload development app
make run         # open the app directly
make bundle      # build the macOS .app
make package     # build the .app and disk image
make cli-help    # show CLI commands
```

## Run

From this directory:

```sh
CGO_ENABLED=0 go run .
```

Choose **New Dot** or press `⌘N` to create a Dot and open its chat immediately. Start by sending a prompt. Use **Customize** to change its name, icon, color, objective, or lasting working instructions. `⌘⇧Space` opens AETHER from another app; the menu bar item can reopen it after the window closes.

Dots use their saved profile, optional editable memory, shared Space notes, and a bounded set of recent conversation messages. Use **Compress new transcript turns** in Customize to update memory in bounded batches; future prompts send that summary and only messages after its coverage marker. The complete transcript stays in AETHER's local store. You can add a reusable skill to a specific turn with `@skill[Skill name]`. Dots do not receive direct user-file or folder access. Optional Computer Use grants a Dot narrowly scoped access to Apple Notes or Music. Replies are saved to local chat history; Dot tasks do not create output files.

## Model setup

Settings supports OpenAI, Anthropic Claude, xAI Grok, OpenRouter, and custom OpenAI-compatible endpoints. OpenAI uses the [official OpenAI Go SDK](https://github.com/openai/openai-go) and Responses API; Anthropic uses its [official Go SDK](https://github.com/anthropics/anthropic-sdk-go) and native Messages API. xAI's official quickstart shows the OpenAI SDK-compatible pattern for its documented [Responses API](https://docs.x.ai/developers/rest-api-reference/inference/responses), so AETHER uses the OpenAI Go SDK against xAI's fixed API origin. Settings hold each provider's model defaults and credentials; Customize can override provider and model for one Dot. Chat history and compacted memory stay provider-neutral, so switching models preserves the same Dot profile and transcript. OpenRouter loads its live text-input/text-output model catalog in a searchable, virtualized modal and displays published context size and function/reasoning support. AETHER uses the same bounded local function catalog across providers when supported. Built-in functions provide local time and arithmetic. Provider-side web and X search remain off; local computer access requires the separate per-Dot grant described below.

Anthropic's beta Message Threads are optional and off by default. While an in-memory thread is active, later turns send only new messages. Anthropic expires a thread after five minutes without use and deletes it no later than one hour after its last use; if the server copy expires, the SDK replays from the in-memory conversation. Anthropic encrypts thread data in memory, provides no early-delete API, and marks Threads ineligible for Zero Data Retention. Turning the feature off or restarting AETHER drops local references; already-sent data remains subject to Anthropic's expiry. Threads do not change input-token billing. Adaptive thinking and model-supported effort controls are used for recognized Claude model IDs; unsupported effort levels are omitted. Anthropic requires `max_tokens`, so when the output limit is blank AETHER sends a 16,384-token cap, including reasoning tokens. See Anthropic's [Message Threads](https://platform.claude.com/docs/en/build-with-claude/message-threads) and [effort controls](https://platform.claude.com/docs/en/build-with-claude/effort) documentation.

Each Dot can be granted access to up to 12 specialist Dots in its Space, with at most three handoffs per run. A handoff sends a typed task, constraints, and expected output to the selected Dot; it does not send the requesting Dot's full chat history. The specialist uses its own profile and saved history. The request and result are saved in both conversations, and each decision is recorded in the task event chain. Grants start empty and are directional; moving a Dot to another Space clears old grants to it. Handoffs can make additional model requests and use the same configured provider. No function has general access to files, folders, accounts, or arbitrary network destinations; local app access is limited to the explicit Computer Use scopes below.

Desktop API keys are stored separately per provider in the system credential store (macOS Keychain on macOS). Provider settings and the custom endpoint URL are stored in local preferences; secrets are never written to preferences, Dot data, or logs. Remote custom endpoints must use HTTPS. Cleartext HTTP is accepted only for loopback endpoints such as localhost.

The CLI chooses a provider with environment variables and uses the same provider adapters:

```sh
export AETHER_MODEL_PROVIDER="openai" # openai, anthropic, xai, openrouter, or openai-compatible
export AETHER_MODEL_NAME="gpt-6.1-sol"
export OPENAI_API_KEY="..."
export ANTHROPIC_API_KEY="..."
export XAI_API_KEY="..."
export OPENROUTER_API_KEY="..."
export AETHER_MODEL_BASE_URL="https://api.example.com/v1" # custom provider only
export AETHER_COMPATIBLE_API_KEY="..." # optional for local endpoints
# Reasoning effort applies to OpenAI, Anthropic, xAI, and compatible models.
# Use "default" to omit the effort parameter for compatible providers.
export AETHER_REASONING_EFFORT="medium"
# Text verbosity is supported by the OpenAI and xAI Responses APIs.
export AETHER_TEXT_VERBOSITY="medium"
# Optional. Leave unset to use the selected provider/model's reply-length default.
# export AETHER_MAX_OUTPUT_TOKENS="16384"
export AETHER_MAX_RETRIES="2"
export AETHER_MODEL_TIMEOUT_SECONDS="90"
```

The Settings page includes provider-specific models and credentials, the OpenRouter catalog, a custom endpoint, optional generation controls, and a connection check. All provider adapters stream text and use the same provider-neutral AETHER function catalog; chat appends stream batches without rebuilding the accumulated reply for each batch. OpenAI and xAI Responses requests disable response storage. Prompts include the Dot profile, selected skill, optional saved memory, shared Space notes, and up to 12 uncovered recent messages. The full transcript remains local. AETHER keeps prompts bounded and fails clearly if their local safety limit is exceeded.

The **Activity** page shows active and recent Dot runs, per-run cancellation, first-token latency, duration, and approximate token counts. Independent conversations can run in parallel; a conversation accepts one active reply at a time. Metrics are approximate because providers expose different usage details.

## Integrations

Settings → **Integrations** includes GitHub and Computer Use. Apple Reminders is marked **Coming soon**. Computer Use installs the separate [Cua Driver](https://github.com/trycua/cua) on demand; AETHER does not bundle it.

### MCP servers

Open the dedicated **MCP** Settings tab to automatically find servers already running on this Mac. AETHER scans only `127.0.0.1` and `::1`, and checks Streamable HTTP on the common `/mcp` and `/` paths; it does not ask you to copy a URL. Servers that use only stdio or a custom path won't appear. AETHER never launches server commands, contacts remote MCP hosts, or stores MCP credentials. Discovered tool schemas are refreshed and changed definitions invalidate their saved grants.

In **Customize → MCP tools**, grant exact tools to one Dot and accept the disclosure that tool inputs and returned data may be sent to that Dot's configured model provider. Changing a Dot's provider, or changing the workspace provider for Dots that use its default, requires accepting that disclosure again. A granted tool may act within the server's own scope, so review and trust the server and each tool before enabling it. Server descriptions, schemas, annotations, and results are untrusted data; annotations such as `readOnlyHint` are not treated as proof of behavior. Grants are bound to the Dot's current Space, rechecked before and after each call, and are unavailable to Routines, delegated Dots, and iPhone messages. Removing a server revokes all of its Dot grants.

### Computer Use setup and scopes

Select **Install Cua Driver** for a one-click stable-channel install. The fixed upstream installer verifies release checksums; AETHER verifies the installed app's macOS code signature. Setup leaves PATH unchanged, disables Cua Driver's optional usage telemetry, and prepares a bounded session for fast first use. Cua Driver runs with an app allowlist and a restrictive argument policy. Its first-launch permission gate is disabled because the initial feature does not need screen capture. After the first install, AETHER opens macOS Accessibility settings; add `/Applications/CuaDriver.app` if it is not listed, and switch it on. AETHER does not request Screen Recording.

In **Customize → Computer Use access**, grant each Dot one or both scopes: read the accessibility text in the currently displayed Apple Notes window, or control playback in Apple Music's open player window with play, pause, next, and previous. The model cannot choose an app, window, or arbitrary UI action. Notes content may be sent to that Dot's configured model provider, so AETHER requires a disclosure before it saves the grant. Observed UI text is untrusted. AETHER records access events without copying note contents into the event log.

Computer Use is available only to local interactive Dot runs. It is not passed to Routines, delegated Dots, or messages from the iPhone. Moving a Dot to a different Space clears its Computer Use grant. **Revoke all Dot access** removes every Computer Use grant and stops the bounded session; the separately installed driver and macOS Accessibility toggle remain under the user's control. AETHER stops its session before opening System Settings and starts a fresh bounded session on the next use after an Accessibility change.

### GitHub through the GitHub CLI (preferred)

If [`gh`](https://cli.github.com) is installed, AETHER reads GitHub through it: run `gh auth login` once, then **Connect GitHub** in Settings (or `aether github login`). AETHER stores no GitHub token, needs no GitHub App, and every call is still a typed, read-only request limited to the repositories you select and grant to a Dot. **Disconnect** only stops AETHER using `gh`; it does not sign you out of `gh`. Without `gh`, the GitHub App setup below applies.

### GitHub App setup (fallback when `gh` is not installed)

AETHER uses GitHub App Device Flow, which GitHub documents for desktop applications. In GitHub, open **Settings → Developer settings → GitHub Apps → New GitHub App** (or edit an App you control). Under **Identifying and authorizing users**, enable **Enable Device Flow**. Leave **Request user authorization (OAuth) during installation** unchecked: AETHER starts a separate Device Flow, while that option starts the web authorization flow during installation. AETHER needs the public **Client ID**, not the App ID. Do not create a client secret for AETHER or embed a private key.

Under **Permissions & events**, set only the repository permissions in the table below to **Read-only** and leave webhook permissions/events disabled. Save the App and copy its Client ID from the App settings. Install the App for the GitHub account or organization that owns the repositories, selecting **Only select repositories** and the repositories AETHER should be able to discover. Device Flow authorization and App installation are separate: the user token can access only resources allowed by both the user and the App installation. AETHER requests `offline_access`, follows GitHub's polling interval, observes `slow_down`, and rotates expiring access tokens when GitHub returns refresh tokens. See GitHub's [Device Flow guide](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-a-user-access-token-for-a-github-app), [App registration settings](https://docs.github.com/en/apps/maintaining-github-apps/modifying-a-github-app-registration), [permission-to-endpoint reference](https://docs.github.com/en/rest/authentication/permissions-required-for-github-apps), and [installation guide](https://docs.github.com/en/apps/using-github-apps/installing-a-github-app-from-a-third-party).

Grant only these repository permissions:

| GitHub App permission | Access | Used for |
| --- | --- | --- |
| Metadata | Read | Repository metadata and installation repository discovery |
| Contents | Read | Commit metadata, including the default branch's latest commit |
| Issues | Read | List and inspect issues |
| Pull requests | Read | List and inspect pull requests |
| Checks | Read | Inspect commit check runs |
| Actions | Read | Inspect recent workflow runs and job summaries |

The GitHub REST API calls also read the authenticated user, accessible App installations, and repositories inside those installations. GitHub limits user access tokens to the intersection of the App's permissions and the signed-in user's access. AETHER does not request write permissions. It does not fetch repository source files, diffs, workflow logs, or archives.

For local development, set the public Client ID before launching AETHER:

```sh
export AETHER_GITHUB_CLIENT_ID="your-public-github-app-client-id"
export AETHER_GITHUB_APP_SLUG="your-github-app-slug" # enables the install link when needed
make run
```

For a distributable MyGo app, embed the public configuration through MyGo's linker flags (the pinned MyGo v0.7.3 reads `MYGO_LDFLAGS`). These values are public identifiers, not credentials:

```sh
MYGO_LDFLAGS="-X aether-dots/internal/integrations/github.EmbeddedClientID=your-public-client-id -X aether-dots/internal/integrations/github.EmbeddedAppSlug=your-app-slug" make bundle
```

The `AETHER_GITHUB_CLIENT_ID` and `AETHER_GITHUB_APP_SLUG` environment variables override the embedded values. Do not put a GitHub App client secret, private key, access token, or refresh token in linker flags, source control, preferences, or the task store.

### Connect and grant access

In Settings → Integrations, select **Connect GitHub**, enter the one-time code on GitHub, then install the App for the accounts and repositories AETHER should be able to discover. Use **Manage repositories** to select the repositories AETHER may offer to Dots. Connection does not grant any Dot access.

Open **Customize** for a Dot, enable GitHub, select exact repositories, and accept the disclosure that retrieved repository metadata, issue/PR text, commit details, and CI results may be sent to that Dot's configured model provider. GitHub tokens never go to a model. Retrieved GitHub text is treated as untrusted evidence. Each operation checks the Dot's current Space, repository grant, local selection, current App installation access, and connection again. Specialists use their own grants. Scheduled Routines need a separate **Allow this Routine to use the Dot's GitHub grants** choice.

The provider-neutral GitHub tools work with supported OpenAI, Anthropic, xAI, OpenRouter, and OpenAI-compatible function-calling models. If the selected model is known not to support functions, AETHER omits the GitHub tools and reports that it did not query GitHub. A Dot can request a repository health review, open issues and pull requests, commit metadata, check runs, recent workflow runs, and bounded job summaries. Results include GitHub source links and a checked-at time for CI summaries. AETHER records the read operation in the Dot's append-only event history without copying private issue or repository content there.

The CLI uses the same Keychain credential store and local grants:

```sh
aether github login
aether github status
aether github refresh
aether github repositories
aether github select <repository-id> [repository-id ...]
aether github grant --allow-model-data <dot-id> <repository-id> [repository-id ...]
aether send <dot-id> --message "Check the latest CI status for my selected repository"
aether github revoke <dot-id>
aether github disconnect
```

Routine use is a separate opt-in:

```sh
aether routines create <dot-id> --name "Repository health" \
  --prompt "Summarize the selected repository's latest CI, issues, and pull requests." \
  --every 24h --github-read-only
```

Disconnect cancels local GitHub requests, removes AETHER's Keychain token and local repository/Dot/Routine grants, and disables further tool calls. It does not uninstall the GitHub App or revoke its authorization on GitHub. Use the **GitHub authorized apps** link in Settings to manage server-side authorization.

### Troubleshooting and Apple Reminders

- **Not configured:** register a GitHub App with Device Flow enabled and set or embed its public Client ID.
- **Authorized but no installations or repositories:** install the App for the intended account and choose repositories in GitHub's installation settings. Then use **Refresh GitHub access**.
- **Permission required:** verify the read permissions above and whether the App's repository selection includes the selected repository. Refresh to remove access that GitHub has withdrawn.
- **Session expired:** reconnect GitHub. AETHER refreshes expiring user tokens automatically while the refresh token remains valid.
- **Rate limited:** wait for GitHub's returned retry interval before refreshing or asking a Dot to repeat the read.
- **No GitHub function appears:** select a model/provider that supports tool calls; a custom endpoint must implement the existing function-call protocol.

- **Computer Use cannot see Notes or Music:** add `/Applications/CuaDriver.app` under **System Settings → Privacy & Security → Accessibility**, switch it on, and open the Notes window or Apple Music player window before retrying. AETHER stops the bounded driver session before opening that pane and starts a fresh session on the next use. Screen Recording is not required by the current accessibility-tree and playback scopes.
- **A model doesn't use an enabled scope:** choose a model that supports function calls, confirm the disclosure and Dot grant in Customize, and verify the model run is local and interactive. Routine and delegated runs do not receive Computer Use tools.

Apple Reminders remains a separate future EventKit integration. A future implementation should request Reminders authorization only after explicit user opt-in, include `NSRemindersFullAccessUsageDescription` for macOS 14 and later (and the appropriate legacy `NSRemindersUsageDescription` fallback if older systems remain supported) in the actual signed app, and scope reads to selected lists. EventKit full Reminders access is a read/write OS permission, so AETHER would still enforce read-only behavior until separate write approvals exist. It must not request Calendar event access. The Reminders AppleScript dictionary is not a substitute for EventKit authorization. This release does not request EventKit or Apple Events authorization.

### AETHER Dots for iPhone

The Expo companion signs in through the AETHER web dashboard. A one-time verifier-bound handoff returns a 30-day opaque session credential, which the app stores in iOS Keychain through Expo SecureStore. Pair the iPhone by entering the short-lived code shown in **Settings → Integrations** on the Mac, then approve the claimed pairing on that Mac. The account dashboard can revoke an iPhone session or desktop at any time.

The iPhone can check desktop status, list Dots, open a conversation when requested, and send a message to a Dot. The Mac runs messages through that Dot's saved provider settings and existing specialist or GitHub grants. Remote message text and results pass through the AETHER Cloudflare Worker and Neon; pending and completed remote command data expires after one day. The relay does not receive local files, provider keys, or GitHub tokens. The desktop must be online and AETHER Dots must be open to receive a request.

Release builds must pin the AETHER Cloudflare Worker origin at link time. Add it to the existing public configuration before bundling:

```sh
MYGO_LDFLAGS="-X aether-dots/internal/integrations/remote.BuildRelayOrigin=https://<worker>.<account>.workers.dev" make bundle
```

For development testing with the Worker deployed for this bridge, use `https://aether-dots-web-dev.enosislabs.workers.dev` in that linker flag. It points to the temporary `aether-bridge-dev` Neon branch, which expires on 2026-11-09; do not use it for production releases.

For local development, run the Cloudflare Worker on port 5173, then start the desktop with `AETHER_RELAY_DEV=1 AETHER_RELAY_URL=http://127.0.0.1:5173 make run`. The development override accepts only loopback HTTP and is unavailable in release configuration.

## Skills and routines

Create skills in Settings or with `aether skills create`. A skill records when it applies, required inputs, steps, validation, output format, and approval boundaries. Skills are shared across Dots, but a chat uses one only when you explicitly name it, for example `@skill[Weekly review]`.

```sh
aether skills create --name "Weekly review" \
  --description "Summarize progress, risks, and next steps." \
  --when-to-use "When asked for a weekly review." \
  --inputs "Use facts provided in the conversation." \
  --steps "Group progress, risks, and next steps." \
  --validation "Check each statement against the supplied facts." \
  --output "Return a short summary with three headings." \
  --approval "Do not contact anyone or take external actions."
```

Create an interval routine in Settings or with the CLI:

```sh
aether routines create <dot-id> --name "Daily review" \
  --prompt "Summarize the latest context and call out missing inputs." \
  --every 24h
aether routines list
aether routines test <routine-id>
aether routines pause <routine-id>
```

Intervals must be at least five minutes. A new or resumed schedule waits for its next interval; **Test now** makes a real model call without moving that schedule. Routines run on the local machine while the desktop app is open, or while `aether routines serve` is running. They use the owning Dot's profile and recent chat context, can consult only its explicitly granted specialists, save results and handoffs in the affected conversations, keep 20 recent run records, and skip missed intervals instead of replaying them. The scheduler caps runs at 200 per routine and 300 total per hour; a rate-limited scheduled run is recorded as failed and is not retried until the next interval. Routine prompts do not access files, folders, apps, or accounts.

## CLI

```sh
CGO_ENABLED=0 go run ./cmd/aether --help
CGO_ENABLED=0 go run ./cmd/aether spaces list
CGO_ENABLED=0 go run ./cmd/aether create --name "Decision partner" \
  --objective "Help me prepare concise decision briefs" \
  --instructions "Use plain language and state uncertainty"
CGO_ENABLED=0 go run ./cmd/aether list
CGO_ENABLED=0 go run ./cmd/aether send <dot-id> --message "Summarize this discussion"
CGO_ENABLED=0 go run ./cmd/aether inspect <dot-id>
```

Use `--data-dir <folder>` before the command to select an alternate AETHER store. The desktop app also accepts `--data-dir` or `AETHER_DOTS_DATA_DIR`. The store is a SQLite database (`dots.db`, WAL mode, owner-only permissions) that writes only the rows a change touches, so cost does not grow with chat history. It includes a per-task hash chain for status events. A pre-existing `dots.json` is imported once and kept as `dots.json.migrated`; it can be deleted once you are satisfied. Opening a schema v1 store migrates old task records and removes saved source/output paths, artifact paths, and routine grants; schema v2 stores migrate to the skills and local routines format. Schema v3 conversations migrate to v4 with no specialist grants; grants are opt-in. Schema v4 stores migrate to v5 with GitHub disconnected and no repository selections, Dot grants, or Routine opt-ins. Schema v5 stores migrate to v6 with no Computer Use grants or model disclosure; credentials are never part of the task store.

## Package and checks

```sh
CGO_ENABLED=0 go build ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go tool mygo vet .
```

The app's own Dot store and preferences are local. Dot task execution has no direct user-selected filesystem access or arbitrary-app control. User-managed MCP discovery scans only TCP listeners on loopback and checks Streamable HTTP MCP handshakes at `/mcp` and `/`; tools still need explicit per-Dot grants and disclosure. AETHER does not start MCP commands or connect to remote MCP hosts. The fixed Cua Driver MCP connection uses AETHER's private socket, a bounded capability manifest, and an argument policy. The only cloud connections are explicitly granted read-only GitHub access and the scoped mobile relay described above. Its function catalog is scoped to the current request; a specialist handoff requires a saved same-Space grant and is rechecked when the tool runs.

## Development history

`HISTORY-TRACKER.md` records implementations, decisions, and verification, including work that is still in progress. It is not a release changelog. Reserve `CHANGELOG.md` for version-specific changes that are complete and ready for Beta, RC, or Final product releases.

## License

AETHER Dots is licensed under Apache-2.0. `resources/licenses/` carries this license and notices/license texts for the dependencies linked into the desktop app.
