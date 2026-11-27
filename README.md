# AETHER

AETHER is a native macOS app for focused AI teammates. Each **agent** has its own name, icon, objective, working instructions, memory, tool access, and ongoing chat. Your agents and their history stay on your Mac.

## Get started

1. Open AETHER (or build it from source, see below).
2. Go to **Settings**, choose a model provider, and add your API key.
3. Choose **New agent** (`⌘N`) and send your first message.

Use **Customize** to change an agent's name, icon, color, objective, instructions, or model. Press `⌘⇧Space` to open AETHER from any app; the menu bar item reopens it after you close the window.

## Model providers

AETHER works with **OpenAI**, **Anthropic Claude**, **xAI Grok**, **OpenRouter**, and any **OpenAI-compatible** endpoint (including local ones such as `http://localhost`). Set defaults in Settings and override the provider or model per agent in Customize. Switching models keeps the agent's profile and history.

- API keys are stored in the macOS Keychain, never in preferences, agent data, or logs.
- Remote custom endpoints must use HTTPS.
- The **Test** button in Settings checks your connection.
- **Activity** shows running and recent replies, with cancel, timing, and approximate token counts.

## What agents can and can't access

By default, an agent can only chat. It **cannot** read your files or folders, run commands, or reach arbitrary websites. Every extra capability is off until you grant it to a specific agent, and anything that could send your data to the model provider asks for your consent first.

| Capability | What it allows | How to enable |
| --- | --- | --- |
| Specialist agents | Hand a task to up to 12 other agents in the same Space (max 3 handoffs per reply) | Customize → Specialist access |
| GitHub | Read: repositories, issues (including search within a granted repository), pull requests, commits, CI checks and runs. Write: comments, issues and draft pull requests only as proposals you approve | Settings → Integrations, then Customize |
| MCP tools | Use specific tools from MCP servers already running on this Mac | Settings → MCP, then Customize |
| Computer Use | Read the open Apple Notes window, or play/pause/skip in Apple Music | Settings → Integrations, then Customize |

Moving an agent to another Space clears its grants. Scheduled Routines, delegated agents, and iPhone messages get a reduced set of tools.

## Memory and context

Each reply uses the agent's profile, its saved memory, shared Space notes, and recent messages. The full transcript always stays on your Mac. To keep long chats efficient, use **Customize → Compress new transcript turns** to fold older messages into the agent's memory.

## Integrations

### GitHub

Install the [GitHub CLI](https://cli.github.com), run `gh auth login`, then select **Connect GitHub** in Settings. AETHER stores no GitHub token. Choose which repositories AETHER may use under **Manage repositories**, then grant specific repositories to an agent in Customize. Granted repositories can be read by that agent. **Disconnect** stops AETHER from using `gh` but doesn't sign you out of it.

**Writing to GitHub is separate and always approved by you.** In Customize, an agent can also be allowed to *propose* a comment, a new issue or a draft pull request (from a branch that already exists on GitHub) for repositories it can read. A proposal is only text waiting in **GitHub approvals** in the sidebar: nothing is sent until you read the exact text and choose **Approve and send**. It is then published under your own GitHub account, which may be public. Proposals expire after 24 hours, and Routines, delegated agents and the iPhone cannot create them. The CLI offers the same flow: `aether github write-grant`, then `aether github proposals [show|approve|reject]` (approving needs a terminal and typing back part of the content hash).

If you switch `gh` to a different account (`gh auth switch`), AETHER stops giving agents GitHub tools until you reconnect, and reconnecting as the new account clears the old account's repository selections and grants. If `gh` lives somewhere unusual, set `AETHER_GH_PATH` to its full path. A direct GitHub API integration may come later; for now `gh` is the only way AETHER reaches GitHub.

### MCP servers

**Settings → MCP** finds MCP servers already running on this Mac (Streamable HTTP on `localhost`, at `/mcp` or `/`). AETHER never starts server commands or connects to remote MCP hosts. Grant individual tools to an agent in **Customize → MCP tools**. Only enable servers and tools you trust: a granted tool can act within its server's own scope.

### Computer Use

Select **Install Cua Driver** in Settings → Integrations, then turn on `/Applications/CuaDriver.app` under **System Settings → Privacy & Security → Accessibility**. Screen Recording isn't needed. **Revoke all agent access** removes every Computer Use grant at once.

### iPhone

Pair the AETHER iPhone app with the code shown in **Settings → Integrations**, then approve it on the Mac. From your phone you can see your agents and send them messages; the Mac runs the reply, so AETHER must be open and online. Message text passes through the AETHER relay and expires after one day. Your files, API keys, and GitHub access never leave the Mac. You can revoke an iPhone from the account dashboard at any time.

## Issue triage agent

Choose **Set up issue triage** on the welcome screen, **Create a triage agent** in Settings → Integrations → GitHub, or run `aether create --template triage`. The agent starts with triage instructions: classify each issue, search the repository for duplicates, find the code involved when Midnight's code search tools are granted (citing path and line), suggest labels and a priority, and draft a reply. The template grants nothing by itself: give it a repository in its profile. With AETHER Pro and a write grant, it proposes the reply as a comment you approve; otherwise the draft stays in the chat. With AETHER Pro, **Add daily summary** in the agent's profile schedules a daily triage pass as a Routine once a repository is granted.

## Skills and Routines

**Skills** are reusable instructions shared across agents (when to use it, steps, output format, limits). A chat uses a skill only when you name it, for example `@skill[Weekly review]`. Create skills in Settings.

**Routines** send an agent a prompt on a schedule (every 5 minutes or longer). They run while the app is open, or while `aether routines serve` is running. Missed runs are skipped, not replayed. **Test now** runs one immediately. GitHub access in a Routine needs its own opt-in.

## Free and Pro

Builds with licensing offer AETHER Pro as an annual license. **Free** includes up to 3 agents, chat, GitHub read access and Midnight. **Pro** adds unlimited agents, GitHub write proposals and Routines. When a license year ends, the version you have keeps Pro for good; newer versions need a renewal. Your agents, chats and data never depend on the license: without Pro nothing is deleted or locked, you just can't create more agents than the Free limit or run Pro features. Manage the license in **Settings → General** or with `aether license`.

## Command line

The `aether` CLI shares the same data as the app:

```sh
aether --help
aether create --name "Decision partner" --objective "Prepare concise decision briefs"
aether list
aether send <agent-id> --message "Summarize this discussion"
aether routines create <agent-id> --name "Daily review" --prompt "Summarize the latest context" --every 24h
```

The CLI reads its model settings from environment variables:

```sh
export AETHER_MODEL_PROVIDER="anthropic"   # openai, anthropic, xai, openrouter, openai-compatible
export AETHER_MODEL_NAME="<model-id>"
export ANTHROPIC_API_KEY="..."             # or OPENAI_API_KEY, XAI_API_KEY, OPENROUTER_API_KEY
export AETHER_MODEL_BASE_URL="..."         # openai-compatible only
```

## Your data

Everything is stored locally in one SQLite database (`dots.db`), readable only by your user account. Use `--data-dir <folder>` or `AETHER_DOTS_DATA_DIR` to choose a different location. If you're upgrading from an older version, your existing `dots.json` is imported automatically and kept as `dots.json.migrated`.

## Build from source

Requires Go 1.27 or later on macOS.

```sh
make run        # run the app
make bundle     # build the .app
make package    # build the .app and a disk image
CGO_ENABLED=0 go run ./cmd/aether --help   # run the CLI
```

## License

AETHER is source-available under the [Functional Source License 1.1, Apache 2.0 future license](LICENSE) (FSL-1.1-ALv2). You may use, modify and redistribute it for any purpose except offering a competing commercial product or service; each version becomes Apache-2.0 two years after its release. Versions published before this change remain under Apache-2.0. Dependency licenses and notices are in `resources/licenses/` (regenerate with `scripts/update-license-notices.sh` after changing `go.mod`).
