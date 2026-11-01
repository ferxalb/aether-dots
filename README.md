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

Dots use only their saved profile and recent chat context. They cannot read or write user files or folders. Replies are saved to chat history in AETHER's local store; Dot tasks do not create output files.

## Model setup

Model replies use the official OpenAI Go SDK and Responses API. The desktop Settings page stores the selected model in local preferences and the API key in the system credential store. The CLI reads its key from `OPENAI_API_KEY`.

```sh
export AETHER_MODEL_NAME="gpt-6.1-sol"
export OPENAI_API_KEY="..."
```

The desktop Settings page includes model selection, a secure API-key field, appearance mode, seven color palettes, and a connection check. The API key is never written to the chat store or preferences file. Requests use the OpenAI Responses API with response storage disabled. Each reply includes the Dot profile, the newest user message, and up to 12 recent chat messages within a 24 KiB context budget; older messages may be omitted and long entries excerpted.

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

Use `--data-dir <folder>` before the command to select an alternate AETHER store. The desktop app also accepts `--data-dir` or `AETHER_DOTS_DATA_DIR`. The store is a versioned JSON file, written atomically with owner-only permissions, and includes a per-task hash chain for status events. Opening a schema v1 store migrates old task records and removes saved source/output paths, artifact paths, and routine grants.

## Package and checks

```sh
CGO_ENABLED=0 go build ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go tool mygo vet .
```

The app's own Dot store and preferences are local. Dot task execution has no user-selected filesystem read or write access, no MCP client, cloud worker, or arbitrary-app control.

## Development history

`HISTORY-TRACKER.md` records implementations, decisions, and verification, including work that is still in progress. It is not a release changelog. Reserve `CHANGELOG.md` for version-specific changes that are complete and ready for Beta, RC, or Final product releases.

## License

AETHER Dots is licensed under Apache-2.0. `resources/licenses/` carries this license and notices/license texts for the dependencies linked into the desktop app.
