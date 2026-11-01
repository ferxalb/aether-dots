# AETHER Dots

A Go-only macOS desktop alpha for durable, inspectable local work. The task engine and CLI share the same local store. The desktop window uses MyGo's Go-drawn native UI; it is not a WebView or a set of AppKit controls.

## Quick commands

The Makefile provides short commands, similar to `npm run` scripts:

```sh
make             # list commands
make dev         # open the live-reload development app
make run         # open the app directly
make bundle      # build the macOS .app
make package     # build the .app and disk image
make cli-help    # show CLI commands
```

Use `make dev` to see UI changes as you work. MyGo opens a separate **AETHER Dots Dev** app and reloads it when Go code changes.

## Run

From this directory:

```sh
CGO_ENABLED=0 go run .
```

The app opens with an empty `Personal` Space. Use **Choose source folder**, **Choose destination**, enter an outcome, and create a Dot. `⌘⇧Space` opens AETHER from another app; the menu bar item can reopen it after the window closes.

The default `local` mode reads bounded UTF-8 Markdown/text/CSV/JSON files and creates a deterministic list of lines containing open-decision markers. It uses no account or credential and clearly labels the limits of that extraction. `model` mode sends the selected text to the configured OpenAI-compatible endpoint and streams the draft into the event view. Review model output before relying on its conclusions.

Model configuration is environment-based. The OpenAI Responses API is the default; set `AETHER_MODEL_API=chat` for a provider that exposes Chat Completions instead.

```sh
export AETHER_MODEL_BASE_URL="https://api.openai.com/v1"
export AETHER_MODEL_NAME="gpt-6.1-sol"
export AETHER_MODEL_API="responses"
export AETHER_MODEL_API_KEY_ENV="OPENAI_API_KEY"
export OPENAI_API_KEY="..."
```

For a server exposing only Chat Completions, set `AETHER_MODEL_API=chat`.

Only the endpoint, model name, and environment variable name are used as configuration; the key is never written to the task store or logs. The UI's **Test model connection** button and `aether model test` check the provider.

## CLI

```sh
CGO_ENABLED=0 go run ./cmd/aether --help
CGO_ENABLED=0 go run ./cmd/aether spaces list
CGO_ENABLED=0 go run ./cmd/aether capture \
  --objective "Find open decisions and prepare a brief with evidence" \
  --source /path/to/selected-notes --output /path/to/output --mode local
CGO_ENABLED=0 go run ./cmd/aether list
CGO_ENABLED=0 go run ./cmd/aether inspect <dot-id>
CGO_ENABLED=0 go run ./cmd/aether run <dot-id>
CGO_ENABLED=0 go run ./cmd/aether resume <dot-id>
CGO_ENABLED=0 go run ./cmd/aether routine promote <dot-id> "Weekly decision brief"
CGO_ENABLED=0 go run ./cmd/aether routine list
```

Use `--data-dir <folder>` before the command to select an alternate local store. The desktop app also accepts `--data-dir` or `AETHER_DOTS_DATA_DIR`. The default store is under the operating system's user config directory. The JSON file is versioned, written atomically with owner-only permissions, and includes a per-Dot hash chain over the append-only event history.

## Package and checks

```sh
CGO_ENABLED=0 go build -o build/aether .
CGO_ENABLED=0 go tool mygo build
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go tool mygo vet .
```

The first slice has local-folder reach only. It has no MCP client, cloud worker, OS keychain storage, or arbitrary-app control. A model request sends selected local text to the configured endpoint; local extraction does not.

## Development history

`HISTORY-TRACKER.md` records implementations, decisions, and verification, including work that is still in progress. It is not a release changelog. Reserve `CHANGELOG.md` for version-specific changes that are complete and ready for Beta, RC, or Final product releases.

## License

AETHER Dots is licensed under Apache-2.0. `resources/licenses/` carries this license and notices/license texts for the dependencies linked into the desktop app.
