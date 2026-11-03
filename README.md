# AETHER Dots

AETHER Dots is a native macOS workspace for focused AI teammates. Each Dot has its own name, icon, objective, working instructions, and continuing chat. The desktop UI is drawn by MyGo in Go; it is not a WebView.

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

Dots use their saved profile and the conversation history before the current message. You can add a reusable skill to a specific turn with `@skill[Skill name]`. They cannot read or write user files or folders. Replies are saved to chat history in AETHER's local store; Dot tasks do not create output files.

## Model setup

The desktop Settings page supports three real connection modes: OpenAI through the official Go SDK and Responses API, OpenRouter through the SDK's OpenAI-compatible Chat Completions API, and a custom OpenAI-compatible endpoint. Each provider keeps its own model selection; changing providers does not modify Dot profiles or saved conversation messages. OpenRouter loads its live text-input/text-output chat model catalog, including models from many providers, in a searchable modal; its virtualized list keeps every matching model available without building the whole catalog at once. The picker displays each model's published context size and whether it declares native function or reasoning-effort support; AETHER sends OpenRouter functions only when the selected model declares support. OpenAI, OpenRouter, and custom endpoints use one provider-neutral function catalog when the selected model supports function calls. The built-in functions provide local time and arithmetic. Custom endpoints must implement function calls to use them.

Each Dot can be granted access to up to 12 specialist Dots in its Space, with at most three handoffs per run. A handoff sends a typed task, constraints, and expected output to the selected Dot; it does not send the requesting Dot's full chat history. The specialist uses its own profile and saved history. The request and result are saved in both conversations, and each decision is recorded in the task event chain. Grants start empty and are directional; moving a Dot to another Space clears old grants to it. Handoffs can make additional model requests and use the same configured provider. No function has general access to files, folders, apps, accounts, or arbitrary network destinations.

Desktop API keys are stored separately per provider in the system credential store (macOS Keychain on macOS). Provider settings and the custom endpoint URL are stored in local preferences; secrets are never written to preferences, Dot data, or logs. Remote custom endpoints must use HTTPS. Cleartext HTTP is accepted only for loopback endpoints such as localhost.

The CLI chooses a provider with environment variables and uses the same provider adapters:

```sh
export AETHER_MODEL_PROVIDER="openai" # openai, openrouter, or openai-compatible
export AETHER_MODEL_NAME="gpt-6.1-sol"
export OPENAI_API_KEY="..."
export OPENROUTER_API_KEY="..."
export AETHER_MODEL_BASE_URL="https://api.example.com/v1" # custom provider only
export AETHER_COMPATIBLE_API_KEY="..." # optional for local endpoints
# Reasoning effort applies to OpenAI and compatible Chat Completions models.
# Use "default" to omit the effort parameter for compatible providers.
export AETHER_REASONING_EFFORT="medium"
# Text verbosity is supported by the OpenAI Responses provider.
export AETHER_TEXT_VERBOSITY="medium"
# Optional. Leave unset to use the selected provider/model's reply-length default.
# export AETHER_MAX_OUTPUT_TOKENS="16384"
export AETHER_MAX_RETRIES="2"
export AETHER_MODEL_TIMEOUT_SECONDS="90"
```

The desktop Settings page includes provider selection, provider-specific model settings, the live OpenRouter catalog, custom endpoint configuration, a masked API-key field, optional generation controls, and a connection check. Both the OpenAI Responses and OpenRouter/compatible Chat Completions adapters stream text as it arrives and execute the same bounded function catalog; the chat batches UI updates and renders live and saved replies with MyGo's incremental Markdown parser. OpenAI Responses requests disable response storage. A reply receives the Dot profile, the explicitly selected skill, the newest user message, and the saved conversation history before that message. AETHER does not impose a token context window; the selected model/provider enforces its own published window. A 16 MiB local prompt safety ceiling prevents oversized requests from exhausting app memory, and an over-limit conversation fails clearly while its saved messages remain intact.

## Integrations

Settings → **Integrations** includes GitHub, Apple Notes, and Apple Reminders. GitHub is a real read-only integration. Notes and Reminders are marked **Coming soon**; the app does not request Apple Events or Reminders authorization.

### GitHub App setup

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

The provider-neutral GitHub tools work with supported OpenAI, OpenRouter, and OpenAI-compatible function-calling models. If the selected model is known not to support functions, AETHER omits the GitHub tools and reports that it did not query GitHub. A Dot can request a repository health review, open issues and pull requests, commit metadata, check runs, recent workflow runs, and bounded job summaries. Results include GitHub source links and a checked-at time for CI summaries. AETHER records the read operation in the Dot's append-only event history without copying private issue or repository content there.

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

### Troubleshooting and future Apple integrations

- **Not configured:** register a GitHub App with Device Flow enabled and set or embed its public Client ID.
- **Authorized but no installations or repositories:** install the App for the intended account and choose repositories in GitHub's installation settings. Then use **Refresh GitHub access**.
- **Permission required:** verify the read permissions above and whether the App's repository selection includes the selected repository. Refresh to remove access that GitHub has withdrawn.
- **Session expired:** reconnect GitHub. AETHER refreshes expiring user tokens automatically while the refresh token remains valid.
- **Rate limited:** wait for GitHub's returned retry interval before refreshing or asking a Dot to repeat the read.
- **No GitHub function appears:** select a model/provider that supports tool calls; a custom endpoint must implement the existing function-call protocol.

Apple Notes remains a future macOS-only integration. On the development Mac, the installed `Notes.sdef` exposes accounts, folders, notes, HTML `body`, read-only `plaintext`, attachment metadata, and a `password protected` property. That local dictionary is a platform-version observation, not a stable cross-version API contract. A future read-only connector should use a supported Notes scripting surface through explicit Apple Events authorization, including `NSAppleEventsUsageDescription` in the signed app's Info.plist, expose only selected accounts/folders and unlocked notes, and never inspect Notes' private database or require Full Disk Access. Scripting support for rich text, attachments, smart folders, and locked notes must be verified against each supported macOS release. The Settings page does not run scripts or probe Notes.

Apple Reminders remains a separate future EventKit integration. A future implementation should request Reminders authorization only after explicit user opt-in, include `NSRemindersFullAccessUsageDescription` for macOS 14 and later (and the appropriate legacy `NSRemindersUsageDescription` fallback if older systems remain supported) in the actual signed app, and scope reads to selected lists. EventKit full Reminders access is a read/write OS permission, so AETHER would still enforce read-only behavior until separate write approvals exist. It must not request Calendar event access. The Reminders AppleScript dictionary is not a substitute for EventKit authorization. Neither Apple permission is requested by this release.

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

Use `--data-dir <folder>` before the command to select an alternate AETHER store. The desktop app also accepts `--data-dir` or `AETHER_DOTS_DATA_DIR`. The store is a versioned JSON file, written atomically with owner-only permissions, and includes a per-task hash chain for status events. Opening a schema v1 store migrates old task records and removes saved source/output paths, artifact paths, and routine grants; schema v2 stores migrate to the skills and local routines format. Schema v3 conversations migrate to v4 with no specialist grants; grants are opt-in. Schema v4 stores migrate to v5 with GitHub disconnected and no repository selections, Dot grants, or Routine opt-ins; credentials are never part of the task store.

## Package and checks

```sh
CGO_ENABLED=0 go build ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go tool mygo vet .
```

The app's own Dot store and preferences are local. Dot task execution has no user-selected filesystem read or write access, MCP client, or arbitrary-app control. The only cloud connections are explicitly granted read-only GitHub access and the scoped mobile relay described above. Its function catalog is scoped to the current request; a specialist handoff requires a saved same-Space grant and is rechecked when the tool runs.

## Development history

`HISTORY-TRACKER.md` records implementations, decisions, and verification, including work that is still in progress. It is not a release changelog. Reserve `CHANGELOG.md` for version-specific changes that are complete and ready for Beta, RC, or Final product releases.

## License

AETHER Dots is licensed under Apache-2.0. `resources/licenses/` carries this license and notices/license texts for the dependencies linked into the desktop app.
