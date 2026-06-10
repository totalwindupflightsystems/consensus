---
name: runtime-profiler-axiom
description: >-
  Universal attach-anywhere runtime profiler (Rust binary). Covers attach/wrap modes,
  platform capabilities (macOS/Linux), symbol resolution (DWARF/source-maps/JIT),
  output formats (JSON/speedscope/SARIF/markdown), streaming (stdout/file/Loki/CloudWatch),
  HTTP API, and integration patterns with Axiom agents. Load this skill when profiling
  running processes, analyzing hotspots, generating flamegraphs, or when any agent needs
  to understand runtime performance characteristics of a system under test.
version: "1.0"
tags:
  vertical: [performance, observability, debugging]
  category: profiling
  core: false
spec: specs/103-Runtime-Profiler.md
---

# Runtime Profiler (`axiom-profiler`)

Universal attach-anywhere runtime profiler for Axiom. A Rust binary that attaches to or wraps any running process, resolves symbols back to source lines, and emits structured performance data consumable by agents and observability platforms.

**Spec**: `specs/103-Runtime-Profiler.md`
**Binary**: `profiler/target/release/axiom-profiler`
**Source**: `profiler/src/`

<!-- axiom:trace work_item=runtime-profiler-01 spec=specs/103-Runtime-Profiler.md -->

---

## When to Load This Skill

Load when an agent needs to:

- Profile a running process to find performance hotspots
- Generate flamegraph data for a slow test suite or service
- Understand what a process is actually doing at runtime (CPU sampling)
- Attach to a process without restarting it (live profiling)
- Produce evidence for performance-related acceptance criteria
- Compare runtime behavior before/after a change (regression detection)
- Integrate profiling into CI/CD for automated perf gates

**Do NOT load** when:
- Only static code analysis is needed (use `code-analysis-axiom` instead)
- Only code structure/call-graph is needed (use `code-graph-intelligence-axiom` instead)
- The work is spec-only, doc-only, or config-only with no runtime component

---

## Quick Start

```bash
# Build the profiler
cd profiler && cargo build --release

# Check platform compatibility
./target/release/axiom-profiler check-compatibility

# Attach to a running process (30s sample)
sudo ./target/release/axiom-profiler attach --pid <PID> --duration 30s

# Profile by process name
sudo ./target/release/axiom-profiler attach --name python3 --duration 30s

# Start with HTTP API for remote control
./target/release/axiom-profiler attach --pid <PID> --api --api-port 6060

# Output as speedscope (for visual flamegraph)
sudo ./target/release/axiom-profiler attach --pid <PID> --duration 10s --format speedscope > profile.json
```

---

## Platform Requirements

| Platform | Status | Requirements |
|----------|--------|-------------|
| **macOS** | ✅ Full | `sudo` or `com.apple.security.get-task-allow` entitlement |
| **macOS (no sudo)** | ⚠️ Partial | Tick counts only; no stack traces → `hotspots: []` |
| **Linux (perf_event)** | ✅ Full (v0.4+) | `CAP_SYS_ADMIN` or `/proc/sys/kernel/perf_event_paranoid <= 1` |
| **Linux (eBPF)** | 🔧 Phase 2 | Stub in v0.1; full in v0.4 |

---

## Output Formats

| Format | Use Case | Flag |
|--------|----------|------|
| `json` | Machine consumption, agent parsing, evidence bundles | `--format json` |
| `speedscope` | Visual flamegraph (open in speedscope.app) | `--format speedscope` |
| `sarif` | CI integration, SARIF-compatible tooling | `--format sarif` |
| `markdown` | Human-readable summary, PR comments | `--format markdown` |

---

## HTTP API

When started with `--api`, the profiler exposes:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/ready` | GET | Readiness probe |
| `/metrics` | GET | Prometheus metrics |
| `/v1/profile/start` | POST | Start a profiling session |
| `/v1/profile/stop` | POST | Stop active session |
| `/v1/profile/status` | GET | Current session status |
| `/v1/profile/results` | GET | Get results of last session |

---

## Symbol Resolution

The profiler resolves runtime samples back to source through multiple strategies:

| Language | Strategy | Quality |
|----------|----------|---------|
| Rust/C/C++ | DWARF debug info | Full (file + line + function) |
| Go | pclntab | Full (function + line) |
| Python | Frame introspection via offsets | Good (function + file) |
| Ruby | Thread state introspection | Good (v3.1+ validated) |
| Node.js | V8 JIT map + source maps | Good (original TS/JS lines) |
| Java/JVM | JVMTI + async-profiler bridge | Planned (Phase 3) |

---

## Integration with Axiom

### As Evidence in Verification

```bash
# Profile during a test run and capture as evidence
sudo axiom-profiler attach --name pytest --duration 60s --format json \
  > .memory-bank/work-items/<id>/runs/<run>/profile.json
```

### In CI (flamegraph gate)

```yaml
# .github/workflows/perf.yml
- name: Profile test suite
  run: |
    axiom-profiler wrap --cmd "pytest tests/" --duration 120s \
      --format sarif --output perf-results.sarif
    # Fail if any hotspot exceeds threshold
    axiom-profiler check --input perf-results.sarif --threshold 500ms
```

See `specs/90-CI-Flamegraph-Profiling.md` for the CI flamegraph integration contract.

### With Code Analysis

The profiler complements the static analysis stack:
- **`axiom analyze`** → "what does the code look like?" (quality, lint, complexity)
- **`axiom-code-intel`** → "how does the code connect?" (call graph, blast radius)
- **`axiom-profiler`** → "what actually happens at runtime?" (hotspots, CPU distribution)

Use all three together for full-picture performance investigation: analyze finds suspicious complexity, code-intel traces the call chain, profiler confirms where time is actually spent.

---

## Key Source Files

| Path | Purpose |
|------|---------|
| `profiler/src/main.rs` | CLI entry point |
| `profiler/src/sampler/` | Platform-specific sampling (macOS task_info, Linux perf_event) |
| `profiler/src/resolver/` | Symbol resolution (DWARF, pclntab, Python/Ruby offsets) |
| `profiler/src/output/` | Format emitters (JSON, speedscope, SARIF, markdown) |
| `profiler/src/api/` | HTTP API server |
| `profiler/src/streaming/` | Output streaming (stdout, file, Loki, CloudWatch) |
| `profiler/tests/` | Integration tests |
| `profiler/KNOWN_LIMITATIONS.md` | Current platform/feature limitations |

---

## Configuration

```toml
# profiler.toml (optional — CLI flags override)
[sampling]
rate_hz = 99           # Sample rate (default 99 Hz to avoid lock-step)
duration_seconds = 30  # Default profile duration

[output]
format = "json"        # Default output format
include_source = true  # Include source context in output

[security]
source_context_roots = ["/home/user/project/src"]  # P0: only show source from these paths
```

---

## Known Limitations (v0.4)

See `profiler/KNOWN_LIMITATIONS.md` for the full list. Key ones:

- **L-2**: macOS requires sudo or entitlements for stack traces
- **L-5**: Linux eBPF sampler is Phase 2+ (perf_event works in v0.4)
- **L-3**: HTTP API sessions report hotspots only when platform sampling works
- **Wrap mode**: Not yet implemented (Phase 2)

axiom:trace work_item=runtime-profiler-01 spec=specs/103-Runtime-Profiler.md
