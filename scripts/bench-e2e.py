#!/usr/bin/env python3
"""Offline end-to-end benchmark: real aether CLI + real aether-midnight + mock model.
Reports aggregate latency percentiles and RSS only (no host identifiers).
Usage: MIDNIGHT_BIN=... AETHER_BIN=... MOCKLLM_BIN=... scripts/bench-e2e.py [runs] [memories]"""
import json, os, re, subprocess, sys, tempfile, time, urllib.request

runs = int(sys.argv[1]) if len(sys.argv) > 1 else 40
memories = int(sys.argv[2]) if len(sys.argv) > 2 else 2000
MID, AE, LLM = (os.environ[k] for k in ("MIDNIGHT_BIN", "AETHER_BIN", "MOCKLLM_BIN"))
work = tempfile.mkdtemp()

def start(cmd):
    p = subprocess.Popen(cmd, stdout=subprocess.PIPE, text=True)
    return p, re.search(r"http://\S+", p.stdout.readline()).group(0)

def pct(xs, q):
    xs = sorted(xs); return xs[min(len(xs) - 1, int(q * len(xs)))]

def summary(name, xs):
    return {"case": name, "n": len(xs), "p50_ms": round(pct(xs, .5), 1),
            "p95_ms": round(pct(xs, .95), 1), "p99_ms": round(pct(xs, .99), 1)}

def rss_kib(pid):
    for l in open(f"/proc/{pid}/status"):
        if l.startswith("VmRSS"): return int(l.split()[1])

def cpu_ticks(pid):
    f = open(f"/proc/{pid}/stat").read().rsplit(")", 1)[1].split(); return int(f[11]) + int(f[12])

t0 = time.perf_counter()
mid, murl = start([MID, "--db", f"{work}/m.db", "serve-http", "--memory-scope", "dot:*"])
startup_ms = (time.perf_counter() - t0) * 1000
llm, lurl = start([LLM])
env = dict(os.environ, AETHER_MODEL_PROVIDER="openai-compatible", AETHER_MODEL_BASE_URL=lurl, AETHER_MODEL_NAME="mock")
A = [AE, "--data-dir", f"{work}/dots"]

def cli(*a):
    return subprocess.run(A + list(a), env=env, capture_output=True, text=True, check=True).stdout

def rpc(method, params, i=1):
    req = urllib.request.Request(murl, json.dumps({"jsonrpc": "2.0", "id": i, "method": method, "params": params}).encode(),
                                 {"content-type": "application/json"})
    return json.load(urllib.request.urlopen(req))

def mk_dot(name):
    return re.search(r"\(([0-9a-f]+)\)", cli("create", "--name", name)).group(1)

dot = mk_dot("Bench")
cli("midnight", "enable", dot, "--endpoint", murl, "--accept-model-disclosure")
plain = mk_dot("NoTools")
scope = f"dot:{dot}"
words = "alpha bravo charlie delta echo foxtrot golf hotel india juliet kilo lima mike november oscar papa quebec romeo sierra tango".split()
batch = [{"scope": scope, "kind": "source", "content": f"record {i} about {words[i % 20]} {words[(i * 7) % 20]} project note {i}"} for i in range(memories)]
for s in range(0, memories, 500):
    rpc("tools/call", {"name": "remember_batch", "arguments": {"memories": batch[s:s + 500]}})
rpc("tools/call", {"name": "remember", "arguments": {"scope": scope, "kind": "source", "content": "The launch codename is Tangerine."}})

results = []
# idle resources (5s)
c0, t = cpu_ticks(mid.pid), time.time(); time.sleep(5)
idle_cpu_pct = (cpu_ticks(mid.pid) - c0) / os.sysconf("SC_CLK_TCK") / (time.time() - t) * 100

lat = []
for i in range(runs * 5):
    t = time.perf_counter()
    rpc("tools/call", {"name": "recall", "arguments": {"scope": scope, "query": f"{words[i % 20]} note", "limit": 5}}, i)
    lat.append((time.perf_counter() - t) * 1000)
results.append(summary(f"midnight recall over HTTP ({memories} memories, new connection each)", lat))

lat = []
for _ in range(runs):
    t = time.perf_counter(); cli("list"); lat.append((time.perf_counter() - t) * 1000)
results.append(summary("cli startup (aether list)", lat))

lat = []
for _ in range(runs):
    t = time.perf_counter(); cli("memory", "recall", dot, "codename"); lat.append((time.perf_counter() - t) * 1000)
results.append(summary("cli memory recall (connect+recall)", lat))

for label, d in (("send, no tools (CLI+store+model path)", plain), ("send, Midnight recall tool (full grounded flow)", dot)):
    lat = []
    for _ in range(runs):
        t = time.perf_counter(); out = cli("send", d, "--message", "What is the launch codename?"); lat.append((time.perf_counter() - t) * 1000)
    if d == dot: assert "Tangerine" in out
    results.append(summary(label, lat))

print(json.dumps({"midnight_startup_ms": round(startup_ms, 1), "midnight_rss_mib": round(rss_kib(mid.pid) / 1024, 1),
                  "midnight_idle_cpu_pct": round(idle_cpu_pct, 2), "results": results}, indent=1))
mid.kill(); llm.kill()
