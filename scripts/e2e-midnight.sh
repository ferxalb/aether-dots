#!/usr/bin/env bash
# Reproducible offline end-to-end check on Linux with the real binaries:
#   aether CLI -> Dot -> mock OpenAI-compatible model -> aether-midnight serve-http
# Covers two isolated Dots with per-Dot bearer tokens, forged scopes, revocation
# and rotation, token leakage, restart, supersede/forget, concurrent CLI
# processes, run ownership, kill -9 crash recovery, SIGINT cancellation and
# Midnight outage. Spends nothing. Usage:
#   MIDNIGHT_BIN=/path/to/aether-midnight scripts/e2e-midnight.sh
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MIDNIGHT_BIN="${MIDNIGHT_BIN:?set MIDNIGHT_BIN to the aether-midnight binary}"
WORK="$(mktemp -d)"; BIN="$WORK/bin"; mkdir -p "$BIN"; umask 077
PIDFILE="$WORK/pids"; : > "$PIDFILE"
cleanup() { kill $(jobs -p) $(cat "$PIDFILE") 2>/dev/null || true; }; trap cleanup EXIT
fail() { echo "FAIL: $*"; exit 1; }
step() { echo "--- $*"; }
(cd "$ROOT" && CGO_ENABLED=0 go build -o "$BIN/aether" ./cmd/aether && CGO_ENABLED=0 go build -o "$BIN/mockllm" ./cmd/aether-mockllm)

DB="$WORK/midnight.db"
M() { "$MIDNIGHT_BIN" --db "$DB" "$@"; }
A() { "$BIN/aether" --data-dir "$WORK/dots" "$@" 2>&1 | tee -a "$WORK/cli.log"; }
start_midnight() { local bind="${1:-127.0.0.1:0}"; : > "$WORK/mid.out"
  "$MIDNIGHT_BIN" --db "$DB" serve-http --bind "$bind" > "$WORK/mid.out" 2>&1 &
  MPID=$!
  for _ in $(seq 50); do grep -q listening "$WORK/mid.out" && break; sleep 0.1; done
  MURL="$(sed 's/listening //' "$WORK/mid.out")"; MBIND="${MURL#http://}"; MBIND="${MBIND%/mcp}"; }
start_mock() { local out="$1"; shift; "$BIN/mockllm" "$@" > "$out" 2>&1 & echo $! >> "$PIDFILE"
  for _ in $(seq 50); do grep -q listening "$out" && break; sleep 0.1; done
  sed 's/listening //' "$out"; }
mint() { M token create --name "$1" --memory-scope "dot:$2" --tools "${3:-recall,remember,list_memories,forget}" > "$WORK/tok-$1" 2>>"$WORK/mid.err"; }
rpc() { curl -s -m 5 -H 'content-type: application/json' ${2:+-H "Authorization: Bearer $2"} -d "$3" "$1"; }
call() { rpc "$MURL" "$1" "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"$2\",\"arguments\":$3}}"; }
DR() { A doctor 2>&1 || true; }
wait_for_running() { for _ in $(seq 60); do DR | grep -q 'running=1' && return 0; sleep 0.1; done; return 1; }

FAST="$(start_mock "$WORK/llm.out")"
export AETHER_MODEL_PROVIDER=openai-compatible AETHER_MODEL_BASE_URL="$FAST" AETHER_MODEL_NAME=mock

step "two Dots, one Midnight, one token per Dot"
start_midnight
A init >/dev/null
ALPHA="$(A create --name Alpha --objective "Alpha memory assistant" | sed -E 's/.*\(([^)]+)\).*/\1/')"
BETA="$(A create --name Beta --objective "Beta memory assistant" | sed -E 's/.*\(([^)]+)\).*/\1/')"
mint dot-alpha "$ALPHA"; mint dot-beta "$BETA" recall,remember,list_memories
TA="$(cat "$WORK/tok-dot-alpha")"; TB="$(cat "$WORK/tok-dot-beta")"
A midnight enable "$ALPHA" --endpoint "$MURL" --token-file "$WORK/tok-dot-alpha" --accept-model-disclosure --forget >/dev/null
A midnight enable "$BETA" --endpoint "$MURL" --token-file "$WORK/tok-dot-beta" --accept-model-disclosure >/dev/null
[ "$(stat -c %a "$WORK/dots/mcp-tokens/"* | sort -u)" = "600" ] || fail "token files must be 0600"
A send "$ALPHA" --message "remember: The Alpha codename is Tangerine." >/dev/null
A send "$BETA" --message "remember: The Beta codename is Blueberry." >/dev/null

step "unauthenticated and bad-token local clients get nothing"
for tok in "" "mdn_bad" "${TA}x"; do
  code="$(curl -s -m 5 -o "$WORK/body" -w '%{http_code}' -H 'content-type: application/json' ${tok:+-H "Authorization: Bearer $tok"} -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' "$MURL")"
  [ "$code" = 401 ] || fail "expected 401, got $code"; grep -q recall "$WORK/body" && fail "tool names leaked"; done

step "cross-Dot isolation (server-enforced)"
A send "$ALPHA" --message "What is the codename?" | grep -q Tangerine || fail "alpha not grounded"
OUT="$(A send "$ALPHA" --message "What is the codename?")"; grep -q Blueberry <<<"$OUT" && fail "alpha saw beta"
A send "$BETA" --message "What is the codename?" | grep -q Blueberry || fail "beta not grounded"
OUT="$(A send "$BETA" --message "What is the codename?")"; grep -q Tangerine <<<"$OUT" && fail "beta saw alpha"
OUT="$(A send "$BETA" --message "forge:dot:$ALPHA codename")"
grep -q 'tool error' <<<"$OUT" || fail "forged scope was not denied: $OUT"; grep -q Tangerine <<<"$OUT" && fail "forged scope leaked"
call "$TB" recall "{\"scope\":\"dot:$ALPHA\",\"query\":\"codename\"}" | grep -q 'permission denied' || fail "direct forged recall"
call "$TB" recall '{"query":"codename"}' | grep -q Tangerine && fail "scope omission reached alpha"
call "$TB" forget '{"id":1}' | grep -q 'permission denied' || fail "beta must not hold forget"
call "$TA" forget "{\"scope\":\"dot:$BETA\",\"id\":2}" | grep -q 'permission denied' || fail "alpha forged forget"
call "$TA" index_repository '{"repo_path":"/"}' | grep -q 'permission denied' || fail "ungranted tool callable"

step "token leakage"
for tok in "$TA" "$TB"; do
  grep -rlF "$tok" "$WORK/dots" --exclude-dir=mcp-tokens && fail "token in Dots data"
  grep -qF "$tok" "$WORK/cli.log" "$WORK/mid.out" "$WORK/llm.out" && fail "token in logs/output"
  grep -qF "$tok" "$DB.tokens.json" && fail "plaintext token in Midnight registry"; done
A mcp list | grep -q 'mdn_' && fail "mcp list prints a token"
echo no-leaks

step "Midnight restart on the same endpoint; persisted memory + same tokens"
kill "$MPID"; wait "$MPID" 2>/dev/null || true
start_midnight "$MBIND"
A send "$ALPHA" --message "What is the codename?" | grep -q Tangerine || fail "memory lost across restart"

step "revocation is immediate; rotation restores access"
M token revoke dot-beta 2>/dev/null
A send "$BETA" --message "ping" | grep -q 'no Midnight tools' || fail "revoked Dot should degrade, not leak"
DR | grep -q REJECTED || fail "doctor must report the rejected credentials"
A send "$ALPHA" --message "What is the codename?" | grep -q Tangerine || fail "revoking beta broke alpha"
mint dot-beta2 "$BETA" recall,remember,list_memories
A midnight enable "$BETA" --endpoint "$MURL" --token-file "$WORK/tok-dot-beta2" --accept-model-disclosure >/dev/null
A send "$BETA" --message "What is the codename?" | grep -q Blueberry || fail "rotation lost memory"

step "supersede + forget"
ID1="$(A memory remember "$ALPHA" "The budget is 10 dollars." --derived | grep -o '"id":[0-9]*' | head -1 | cut -d: -f2)"
A memory remember "$ALPHA" "The budget is 20 dollars." --supersedes "$ID1" >/dev/null
R="$(A memory recall "$ALPHA" budget)"
grep -q "20 dollars" <<<"$R" && ! grep -q "10 dollars" <<<"$R" || fail "supersession"
ID2="$(grep -o '"id":[0-9]*' <<<"$R" | head -1 | cut -d: -f2)"
A memory forget "$ALPHA" "$ID2" >/dev/null
A memory recall "$ALPHA" budget | grep -q "20 dollars" && fail "forgotten fact recalled"
{ A memory forget "$BETA" "$ID2" || true; } | grep -q 'not granted' || fail "beta lacks forget (client-side grant check)"

step "concurrent CLI processes"
PIDS=(); for i in $(seq 8); do "$BIN/aether" --data-dir "$WORK/dots" memory remember "$ALPHA" "concurrent note $i" >/dev/null 2>&1 & PIDS+=($!); done
for p in "${PIDS[@]}"; do wait "$p" || fail "concurrent writer failed"; done
[ "$(A memory list "$ALPHA" | grep -o 'concurrent note' | wc -l)" = 8 ] || fail "concurrent writes lost"
PIDS=(); for i in 1 2 3 4; do "$BIN/aether" --data-dir "$WORK/dots" create --name "Spare$i" >/dev/null 2>&1 & PIDS+=($!); done
for p in "${PIDS[@]}"; do wait "$p" || fail "concurrent create failed"; done
[ "$(A list | grep -c Spare)" = 4 ] || fail "concurrent store writes lost"

step "run ownership, kill -9 crash recovery, SIGINT cancellation"
SLOW="$(start_mock "$WORK/slow.out" --delay 60s)"
AETHER_MODEL_BASE_URL="$SLOW" "$BIN/aether" --data-dir "$WORK/dots" send "$ALPHA" --message "slow question" >/dev/null 2>&1 & SP=$!
wait_for_running || fail "run never started"
D="$(DR)"; grep -q 'active-elsewhere=1' <<<"$D" && grep -q 'recovered=0' <<<"$D" || fail "live CLI run must not be recovered: $D"
{ A send "$ALPHA" --message "second" || true; } | grep -qi 'already queued or running' || fail "second run on a busy Dot must be refused"
kill -9 "$SP"; wait "$SP" 2>/dev/null || true
DR | grep -q 'recovered=1' || fail "crashed run not detected/recovered"
DR | grep -q 'recovered=0 .*active-elsewhere=0' || fail "recovery not idempotent"
A send "$ALPHA" --message "What is the codename?" | grep -q Tangerine || fail "Dot unusable after crash"
AETHER_MODEL_BASE_URL="$SLOW" "$BIN/aether" --data-dir "$WORK/dots" send "$ALPHA" --message "slow again" > "$WORK/int.out" 2>&1 & SP=$!
wait_for_running || fail "second run never started"
kill -INT "$SP"; wait "$SP" 2>/dev/null || true
grep -q 'Reply paused' "$WORK/int.out" || fail "SIGINT did not pause the reply"
DR | grep -q 'recovered=0 .*active-elsewhere=0' || fail "cancelled run left a lease"

step "recovery: Midnight down -> Dot still replies, doctor reports it"
kill "$MPID"; wait "$MPID" 2>/dev/null || true
A send "$ALPHA" --message "Are you there?" | grep -q "no Midnight tools" || fail "no graceful degradation"
DR | grep -q UNREACHABLE || fail "doctor missed outage"
step "supervised Midnight: approve, start once, crash, restart on the same endpoint, stop cleanly"
{ A midnight server start || true; } | grep -q 'no Midnight binary approved' || fail "start must require approval"
{ A midnight server approve --binary "$MIDNIGHT_BIN" --db "$WORK/sup.db" || true; } | grep -q -- '--yes' || fail "approval must be explicit"
A midnight server approve --binary "$MIDNIGHT_BIN" --db "$WORK/sup.db" --yes >/dev/null
A midnight server start | grep -q 'auth enforced' || fail "supervised start"
SUPPID="$(A midnight server status | sed -nE 's/.*pid=([0-9]+).*/\1/p')"
SUPURL="$(A midnight server status | sed -nE 's/.*(http:[^ ]+\/mcp).*/\1/p')"
A midnight server start >/dev/null
[ "$(pgrep -fc "sup.db serve-http" || true)" = "1" ] || fail "duplicate Midnight servers"
SM() { "$MIDNIGHT_BIN" --db "$WORK/sup.db" "$@"; }
GAMMA="$(A create --name Gamma --objective "Supervised memory assistant" | sed -E 's/.*\(([^)]+)\).*/\1/')"
SM token create --name gamma --memory-scope "dot:$GAMMA" --tools recall,remember,list_memories > "$WORK/tok-gamma"
A midnight enable "$GAMMA" --endpoint "$SUPURL" --token-file "$WORK/tok-gamma" --accept-model-disclosure >/dev/null
A send "$GAMMA" --message "remember: The Gamma codename is Kumquat." | grep -qi 'remember\|stored\|saved\|memory' || fail "supervised remember"
grep -rq "$(cat "$WORK/tok-gamma")" "$WORK/dots/midnight" && fail "token leaked into supervisor state"
tr '\0' ' ' < "/proc/$SUPPID/cmdline" | grep -q "mdn_" && fail "token on supervised argv"
tr '\0' '\n' < "/proc/$SUPPID/environ" | grep -qE 'AETHER_|mdn_' && fail "credential in supervised environment"
kill -9 "$SUPPID"; sleep 0.2
A midnight server status | grep -q 'stopped' || fail "crash not detected"
A send "$GAMMA" --message "Are you there?" | grep -q "no Midnight tools" || fail "no degradation during supervised outage"
A midnight server start | grep -q "$SUPURL" || fail "restart changed the endpoint"
A midnight server status | grep -q 'dot-.*: ok' || fail "reconnect after restart"
A send "$GAMMA" --message "What is the Gamma codename?" | grep -q Kumquat || fail "memory lost across supervised restart"
step "token expiry degrades and is reported"
DELTA="$(A create --name Delta --objective "Short-lived token" | sed -E 's/.*\(([^)]+)\).*/\1/')"
SM token create --name delta --memory-scope "dot:$DELTA" --tools recall,remember,list_memories --ttl-secs 2 > "$WORK/tok-delta"
A midnight enable "$DELTA" --endpoint "$SUPURL" --token-file "$WORK/tok-delta" --accept-model-disclosure >/dev/null
sleep 3
A midnight server status | grep -q "dot-$DELTA: REJECTED" || fail "expired token not reported"
A send "$DELTA" --message "hello" | grep -q "no Midnight tools" || fail "expired token should degrade"
A midnight server stop | grep -q stopped || fail "stop"
sleep 0.2; [ "$(pgrep -fc "sup.db serve-http" || true)" = "0" ] || fail "orphaned Midnight after stop"

step "persisted conversation"; A inspect "$ALPHA" | tail -n 4
echo "E2E OK (work dir: $WORK)"
