---
name: morty-monitor-build-axiom
description: >
  Auto-build and install the morty-monitor Go TUI binary for fleet monitoring. Covers build commands, installation paths, and verification.
version: "1.0"
tags:
  vertical: ['operations', 'coding']
  category: operations
  core: false
---
# morty-monitor-build-axiom — Auto-build and Install morty-monitor

axiom:trace work_item=morty-monitor-go-tui-01 spec=specs/97-Morty-Monitor-Go-TUI.md#REQ-MMG-013,REQ-MMG-014 plan= jira_ref=

---

## Purpose

Ensure `morty-monitor` binary is installed and up-to-date before the user tries to run it.

This skill is a companion to `specs/97-Morty-Monitor-Go-TUI.md` and is automatically loaded by `rick-and-morty-axiom`.

---

## When to Load

- **Automatically** loaded by `rick-and-morty-axiom` skill
- When the user runs `morty-monitor` and the binary is not installed
- When the user asks about monitoring the Morty fleet

---

## Build Check Logic

The skill checks if `morty-monitor` is installed and up-to-date:

### Step 1: Check if binary exists in PATH

```bash
which morty-monitor
```

If found, proceed to Step 3 (version check).

### Step 2: Check fallback locations

If not in PATH, check:

1. `$(go env GOPATH)/bin/morty-monitor`
2. `~/.local/bin/morty-monitor`
3. `morty/_tmp/morty-monitor` (repo-local fallback)

If found in any fallback, proceed to Step 3. If not found, trigger build (Step 4).

### Step 3: Version check (stamp file comparison)

Compare the git tree hash of `morty/cmd/morty-monitor/` against the stamp file:

```bash
# Get current tree hash
CURRENT_HASH=$(git rev-parse HEAD:morty/cmd/morty-monitor/ 2>/dev/null || echo "none")

# Get stamp file hash
STAMP_FILE="{binary_path}.stamp"
STORED_HASH=$(cat "$STAMP_FILE" 2>/dev/null || echo "none")

# Compare
if [[ "$CURRENT_HASH" != "$STORED_HASH" ]]; then
  echo "morty-monitor is outdated. Rebuilding..."
  # Trigger build
fi
```

### Step 4: Build from source

```bash
cd morty/
CGO_ENABLED=0 go build -o morty-monitor ./cmd/morty-monitor/
```

---

## Install Procedure

### Primary Install Location

```bash
# Build
cd morty/
make morty-monitor

# Install to GOPATH/bin
mkdir -p $(go env GOPATH)/bin
cp morty-monitor $(go env GOPATH)/bin/

# Update stamp file
echo $(git rev-parse HEAD:morty/cmd/morty-monitor/) > $(go env GOPATH)/bin/morty-monitor.stamp
```

### Fallback 1: ~/.local/bin

If GOPATH/bin is not writable:

```bash
mkdir -p ~/.local/bin
cp morty-monitor ~/.local/bin/
echo $(git rev-parse HEAD:morty/cmd/morty-monitor/) > ~/.local/bin/morty-monitor.stamp
```

### Fallback 2: Repo-local

If ~/.local/bin is not writable:

```bash
mkdir -p morty/_tmp/
cp morty-monitor morty/_tmp/
echo $(git rev-parse HEAD:morty/cmd/morty-monitor/) > morty/_tmp/morty-monitor.stamp
```

---

## Stamp File

**Purpose**: Track which version of the source code was used to build the binary.

**Location**: `{binary_path}.stamp` (e.g., `~/.local/bin/morty-monitor.stamp`)

**Content**: Git tree hash of `morty/cmd/morty-monitor/` at build time.

**Example**:
```
a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0
```

---

## Integration with rick-and-morty-axiom

The `rick-and-morty-axiom` skill MUST be updated to:

### 1. Load this skill at startup

Add to the top of `rick-and-morty-axiom/SKILL.md`:

```markdown
## Prerequisites

Before using this skill, verify the following are available in your environment:

| Dependency | Why needed | Check |
|---|---|---|
| `morty-monitor` binary | Fleet monitoring TUI | `morty-monitor --version` or run `make install-morty-monitor` |

**Note**: If `morty-monitor` is not installed, this skill will auto-build and install it. See `.opencode/skills/morty-monitor-build-axiom/SKILL.md`.
```

### 2. Update the "Installing morty-monitor.sh" section

Replace the current section with:

```markdown
### Installing morty-monitor

**Recommended**: Use the compiled Go binary for better performance and keyboard interactions.

```bash
# Build and install (run once, or when morty-monitor source changes)
cd Axiom/morty && make install-morty-monitor

# Verify installation
morty-monitor --version
```

**Fallback**: If you cannot build the Go binary, use the bash script:

```bash
cp Axiom/morty/scripts/morty-monitor.sh ./.morty/morty-monitor.sh
chmod +x ./.morty/morty-monitor.sh
bash ./.morty/morty-monitor.sh
```

**When to use which**:
- **Binary** (recommended): Production monitoring, fleets of 10+ Mortys, CI pipelines
- **Bash script** (fallback): Environments without Go, ad-hoc debugging, quick one-off checks
```

### 3. Update usage examples

Replace `bash morty-monitor.sh` with `morty-monitor` in all examples:

```bash
# One-shot snapshot (paste into Slack, save to evidence):
morty-monitor --once

# Keep a live dashboard running in a second terminal:
morty-monitor

# Custom refresh interval:
morty-monitor --refresh 5

# Scan a wider port range:
MORTY_PORT_START=9000 MORTY_PORT_END=9999 morty-monitor
```

---

## Verification

After installation, verify:

```bash
morty-monitor --version
morty-monitor --help
morty-monitor --once
```

Expected output:
- `--version` shows version string
- `--help` shows usage
- `--once` renders a single snapshot and exits 0

---

## Troubleshooting

### "morty-monitor: command not found"

The binary is not in PATH. Solutions:

1. Add GOPATH/bin to PATH: `export PATH="$PATH:$(go env GOPATH)/bin"`
2. Add ~/.local/bin to PATH: `export PATH="$PATH:~/.local/bin"`
3. Use full path: `~/.local/bin/morty-monitor`

### "permission denied" during install

The install location is not writable. Use a fallback:

```bash
make morty-monitor
mkdir -p ~/.local/bin
cp morty-monitor ~/.local/bin/
```

### "go: command not found"

Go is not installed. Either:
1. Install Go from https://go.dev/dl/
2. Use the bash script fallback: `bash morty/scripts/morty-monitor.sh`

### Binary is outdated

Rebuild and reinstall:

```bash
cd morty/
make morty-monitor
make install-morty-monitor
```

---

## Related Specs

- `specs/97-Morty-Monitor-Go-TUI.md` — Morty Monitor Go TUI spec
- `specs/67-Go-Agent-Orchestration-Engine.md` — Morty engine spec
- `specs/96-Morty-Admin-API-Lifecycle-Fields.md` — Admin API schema
- `.opencode/skills/rick-and-morty-axiom/SKILL.md` — Rick + Morty supervision pattern
