#!/usr/bin/env bash
# Reproducible offline end-to-end demo: real `aether` CLI -> Dot -> OpenAI-compatible
# mock model -> real `aether-midnight serve-http` -> grounded answer -> persisted chat.
# Spends nothing: the model is the local aether-mockllm. Usage:
#   MIDNIGHT_BIN=/path/to/aether-midnight scripts/e2e-midnight.sh
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MIDNIGHT_BIN="${MIDNIGHT_BIN:?set MIDNIGHT_BIN to the aether-midnight binary}"
WORK="$(mktemp -d)"; BIN="$WORK/bin"; mkdir -p "$BIN"
cleanup() { kill $(jobs -p) 2>/dev/null || true; }; trap cleanup EXIT
(cd "$ROOT" && CGO_ENABLED=0 go build -o "$BIN/aether" ./cmd/aether && CGO_ENABLED=0 go build -o "$BIN/mockllm" ./cmd/aether-mockllm)

start_midnight() { : > "$WORK/mid.out"
  "$MIDNIGHT_BIN" --db "$WORK/midnight.db" serve-http --memory-scope 'dot:*' > "$WORK/mid.out" 2>&1 &
  MPID=$!
  for _ in $(seq 50); do grep -q listening "$WORK/mid.out" && break; sleep 0.1; done
  MURL="$(sed 's/listening //' "$WORK/mid.out")"; }
"$BIN/mockllm" > "$WORK/llm.out" 2>&1 &
for _ in $(seq 50); do grep -q listening "$WORK/llm.out" && break; sleep 0.1; done
export AETHER_MODEL_PROVIDER=openai-compatible AETHER_MODEL_BASE_URL="$(sed 's/listening //' "$WORK/llm.out")" AETHER_MODEL_NAME=mock
A=("$BIN/aether" --data-dir "$WORK/dots")

start_midnight
"${A[@]}" init >/dev/null
DOT="$("${A[@]}" create --name Scout --objective "Project memory assistant" | sed -E 's/.*\(([^)]+)\).*/\1/')"
"${A[@]}" midnight enable "$DOT" --endpoint "$MURL" --accept-model-disclosure
echo "--- remember"; "${A[@]}" send "$DOT" --message "remember: The launch codename is Tangerine and ships on Friday."

echo "--- restart Midnight AND start a fresh CLI process"
kill "$MPID"; wait "$MPID" 2>/dev/null || true
# Same port is not guaranteed; re-point the saved server at the new endpoint.
start_midnight; MURL2="$MURL"
SID="$("${A[@]}" mcp list | cut -f1 | head -1)"; "${A[@]}" mcp remove "$SID"
"${A[@]}" midnight enable "$DOT" --endpoint "$MURL2" --accept-model-disclosure >/dev/null
echo "--- recall (grounded answer)"
ANSWER="$("${A[@]}" send "$DOT" --message "What is the launch codename?")"; echo "$ANSWER"
grep -q Tangerine <<<"$ANSWER" || { echo "FAIL: answer not grounded in persisted memory"; exit 1; }

echo "--- supersede + forget"
"${A[@]}" memory remember "$DOT" "The budget is 10 dollars." --derived >/dev/null      # id 2
"${A[@]}" memory remember "$DOT" "The budget is 20 dollars." --supersedes 2 >/dev/null # id 3
R="$("${A[@]}" memory recall "$DOT" budget)"
grep -q "20 dollars" <<<"$R" && ! grep -q "10 dollars" <<<"$R" || { echo "FAIL: supersession"; exit 1; }
"${A[@]}" memory forget "$DOT" 3 >/dev/null
! "${A[@]}" memory recall "$DOT" budget | grep -q "20 dollars" || { echo "FAIL: forgotten fact recalled"; exit 1; }
echo "--- permission boundary: another scope must be denied"
if curl -s -H 'content-type: application/json' -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"recall","arguments":{"scope":"user:other","query":"x"}}}' "$MURL2" | grep -q 'permission denied'; then echo denied-ok; else echo "FAIL: scope not enforced"; exit 1; fi
echo "--- recovery: Midnight down -> Dot still replies, doctor reports it"
kill "$MPID"; wait "$MPID" 2>/dev/null || true
"${A[@]}" send "$DOT" --message "Are you there?" | grep -q "no Midnight tools" || { echo "FAIL: no graceful degradation"; exit 1; }
DOCTOR="$("${A[@]}" doctor 2>&1 || true)"
grep -q UNREACHABLE <<<"$DOCTOR" || { echo "FAIL: doctor missed outage"; exit 1; }
echo "--- persisted conversation"; "${A[@]}" inspect "$DOT" | tail -n 6
echo "E2E OK (work dir: $WORK)"
