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

Use `--data-dir <folder>` before the command to select an alternate AETHER store. The desktop app also accepts `--data-dir` or `AETHER_DOTS_DATA_DIR`. The store is a versioned JSON file, written atomically with owner-only permissions, and includes a per-task hash chain for status events. Opening a schema v1 store migrates old task records and removes saved source/output paths, artifact paths, and routine grants; schema v2 stores migrate to the skills and local routines format. Schema v3 conversations migrate to v4 with no specialist grants; grants are opt-in.

## Package and checks

```sh
CGO_ENABLED=0 go build ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go tool mygo vet .
```

The app's own Dot store and preferences are local. Dot task execution has no user-selected filesystem read or write access, MCP client, cloud worker, or arbitrary-app control. Its function catalog is scoped to the current request; a specialist handoff requires a saved same-Space grant and is rechecked when the tool runs.

## Development history

`HISTORY-TRACKER.md` records implementations, decisions, and verification, including work that is still in progress. It is not a release changelog. Reserve `CHANGELOG.md` for version-specific changes that are complete and ready for Beta, RC, or Final product releases.

## License

AETHER Dots is licensed under Apache-2.0. `resources/licenses/` carries this license and notices/license texts for the dependencies linked into the desktop app.
