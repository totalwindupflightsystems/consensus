# Cross-Platform Quickstart Guide

This guide covers Consensus setup on **Linux**, **macOS**, and **Windows (WSL2)**. Each platform has its own gotchas — read the section matching your OS.

---

## Quick Reference

| Platform | Docker | Go Binary | Primary Gotcha |
|----------|--------|-----------|----------------|
| Linux | `docker run` | `go build` + `./bin/consensus serve` | Port 8090 conflicts, `init` reports wrong port |
| macOS | Docker Desktop | `go build` + Homebrew Go | Docker resource limits, Apple Silicon architecture |
| Windows | Docker Desktop (WSL2) | Go in WSL2 | WSL2 networking, Firewall blocks, line endings |

---

## Prerequisites (All Platforms)

Before you start:

1. **API Key**: You need a [DeepSeek API key](https://platform.deepseek.com/api_keys) (or OpenRouter key)
2. **Docker** (recommended) OR **Go 1.23+** (for binary builds)
3. **~1 GB disk space** (Docker image) OR **~50 MB** (Go binary + sources)

---

## Option 1: Docker (Recommended — All Platforms)

Docker is the easiest path. One command to pull, one to run.

### Step 1: Pull the image

```bash
docker pull ghcr.io/totalwindupflightsystems/consensus:latest
```

> ⚠️ **Gotcha:** First pull downloads ~300 MB. On slow connections this can take several minutes. You'll see progress bars — wait for them to complete.

### Step 2: Run

```bash
docker run -d \
  --name consensus \
  -p 8090:8090 \
  -v consensus-data:/home/consensus/data \
  -e DEEPSEEK_API_KEY="$DEEPSEEK_API_KEY" \
  ghcr.io/totalwindupflightsystems/consensus:latest
```

### Step 3: Verify

```bash
curl http://localhost:8090/api/v1/health
# → {"status":"healthy","uptime":"2s","db":"sqlite"}
```

### Step 4: Open Chronicle

```bash
# macOS
open http://localhost:8090/chronicle/

# Linux (with desktop)
xdg-open http://localhost:8090/chronicle/

# Windows (WSL2)
/mnt/c/Windows/System32/cmd.exe /c start http://localhost:8090/chronicle/
```

---

## Platform-Specific Docker Gotchas

### Linux

| Gotcha | Symptom | Fix |
|--------|---------|-----|
| **Port 8090 already in use** | `docker: port is already allocated` | Use `CONSENSUS_PORT=8095`: `docker run -e CONSENSUS_PORT=8095 -p 8095:8095 ...` |
| **Docker daemon not running** | `Cannot connect to the Docker daemon` | `sudo systemctl start docker` (systemd), or `sudo service docker start` (SysVinit) |
| **`$DEEPSEEK_API_KEY` empty** | Container starts but API calls fail | `export DEEPSEEK_API_KEY="sk-..."` before the docker run command |
| **DNS resolution in container** | `lookup api.deepseek.com: no such host` | Check `/etc/docker/daemon.json` DNS settings; try `--dns 8.8.8.8` |
| **SELinux blocking volume mount** | Permission denied on `/home/consensus/data` | Add `:Z` suffix to volume: `-v consensus-data:/home/consensus/data:Z` |
| **snap-installed Docker** | Path differences | Use full path `/snap/bin/docker` or add to PATH |

### macOS

| Gotcha | Symptom | Fix |
|--------|---------|-----|
| **Docker Desktop not running** | `docker: Cannot connect` | Start Docker Desktop from Applications |
| **Resource starvation** | Container exits or is slow | Docker Desktop → Settings → Resources: allocate ≥4 GB RAM and ≥2 CPUs |
| **Apple Silicon (M1/M2/M3/M4)** | Image platform mismatch warning | Image is multi-arch (amd64/arm64) — warning is cosmetic, runs fine |
| **`docker-credential-desktop` not found** | `error getting credentials` | Reinstall Docker Desktop or run `docker login ghcr.io` explicitly |
| **macOS firewall blocking** | `curl: (7) Failed to connect` | System Settings → Network → Firewall → allow Docker |
| **File sharing permissions** | Volume mount fails | Docker Desktop → Settings → Resources → File Sharing → add `/Users` |

### Windows (WSL2)

| Gotcha | Symptom | Fix |
|--------|---------|-----|
| **WSL2 not installed** | `wsl: command not found` | `wsl --install` from PowerShell (Admin), reboot |
| **Docker Desktop WSL2 integration disabled** | `docker: command not found` in WSL | Docker Desktop → Settings → Resources → WSL Integration → enable for your distro |
| **Windows Firewall blocks port** | Browser can't reach `localhost:8090` | Allow port 8090 in Windows Defender Firewall: `netsh advfirewall firewall add rule name="Consensus" dir=in action=allow protocol=TCP localport=8090` |
| **`curl` not installed in WSL** | `curl: command not found` | `sudo apt install curl -y` (Ubuntu/Debian) |
| **Line endings (CRLF)** | `env: '\r': No such file or directory` when sourcing env files | Use `dos2unix` or configure git: `git config --global core.autocrlf input` |
| **WSL2 IP changes on reboot** | Docker port forwarding breaks | Use `localhost` (not WSL2 IP). Docker Desktop auto-forwards. |
| **Docker volume persistence across WSL restarts** | Data lost after `wsl --shutdown` | Docker volumes persist — data is safe. Only `tmpfs` data is lost. |

---

## Option 2: Go Binary (Local Development)

If you prefer to run without Docker, build from source.

### Step 1: Install Go

```bash
# Linux (Ubuntu/Debian)
sudo apt install golang-go -y  # or: snap install go --classic

# macOS
brew install go

# Windows (WSL2)
sudo apt install golang-go -y
```

Verify: `go version` → should show Go 1.23 or later.

### Step 2: Clone and build

```bash
git clone https://github.com/totalwindupflightsystems/consensus.git
cd consensus
go build -o bin/consensus ./cmd/consensus/
```

### Step 3: Initialize

```bash
./bin/consensus init
```

> ⚠️ **Gotcha:** `init` reports `Server URL: http://127.0.0.1:8094` but the actual default port is **8090**. This display bug is cosmetic — the server starts on port 8090 unless you override it with `CONSENSUS_PORT`.

### Step 4: Serve

```bash
export DEEPSEEK_API_KEY="sk-..."
./bin/consensus serve
```

### Step 5: Verify

```bash
curl http://localhost:8090/api/v1/health
```

---

## Platform-Specific Go Binary Gotchas

### Linux

| Gotcha | Symptom | Fix |
|--------|---------|-----|
| **Port 8090 occupied** | `api: listen tcp :8090: bind: address already in use` | `CONSENSUS_PORT=8095 ./bin/consensus serve` |
| **Non-Consensus service on port 8090** | `curl` returns 404 or unexpected response | Verify with `ss -tlnp \| grep 8090`. Use `CONSENSUS_PORT` to pick a different port. |
| **Missing C compiler for SQLite** | `go build` fails with CGO errors | `CGO_ENABLED=0 go build ...` uses pure-Go SQLite driver |
| **Go not in PATH** | `go: command not found` | Add `export PATH=$PATH:/usr/local/go/bin` to `~/.bashrc` |
| **Old Go version** | `go: go.mod requires go >= 1.23` | Update: `sudo apt install golang-1.23` or use `snap refresh go --channel=1.23/stable` |

### macOS

| Gotcha | Symptom | Fix |
|--------|---------|-----|
| **Xcode Command Line Tools missing** | `xcrun: error: invalid active developer path` | `xcode-select --install` |
| **Homebrew Go vs official Go** | Version mismatch | Prefer `brew install go` — it auto-updates |
| **Gatekeeper blocks binary** | `"consensus" cannot be opened because the developer cannot be verified` | `xattr -d com.apple.quarantine bin/consensus` |
| **Apple Silicon arch mismatch** | Built for arm64, running on amd64 | `GOARCH=amd64 go build` for Intel Macs |

### Windows (WSL2)

| Gotcha | Symptom | Fix |
|--------|---------|-----|
| **Git not installed in WSL** | `git: command not found` | `sudo apt install git -y` |
| **Repo cloned to Windows filesystem** | `go build` fails with permission errors | Clone inside WSL filesystem (`/home/<user>/`, NOT `/mnt/c/`) |
| **WSL2 clock drift** | TLS certificate errors on `go build` | `sudo hwclock -s` to sync clock |
| **File permission issues** | `init` can't create files | `chmod -R u+w .` in the consensus directory |

---

## Post-Setup: First Steps

Once Consensus is running:

1. **Health check**: `curl http://localhost:8090/api/v1/health`
2. **Chronicle UI**: Open `http://localhost:8090/chronicle/` in your browser
3. **Create a session**: Use the Chronicle UI or the API
4. **Run the demo** (Go binary only):
   ```bash
   DEEPSEEK_API_KEY=$DEEPSEEK_API_KEY go test -v -run TestDemo -timeout 300s ./demo/
   ```

---

## Troubleshooting Matrix

| Symptom | Most Likely Cause | Check |
|---------|------------------|-------|
| `connection refused` | Server not running | `docker ps` or `ps aux | grep consensus` |
| `404 Not Found` on `/api/v1/health` | Wrong port or non-Consensus service on 8090 | `ss -tlnp \| grep 8090` |
| `401 Unauthorized` | Missing or wrong `DEEPSEEK_API_KEY` | `echo $DEEPSEEK_API_KEY` — must start with `sk-` |
| `500 Internal Server Error` | Database migration failed | Check logs: `docker logs consensus` or `journalctl -u consensus` |
| Chronicle page blank | Static assets not served | Check browser console for 404s on CSS/JS files |
| Docker exits immediately | `DEEPSEEK_API_KEY` not set | Container requires the env var — check `docker logs consensus` |

---

## Environment Variable Quick Reference

| Variable | Required | Default | Notes |
|----------|----------|---------|-------|
| `DEEPSEEK_API_KEY` | Yes | — | Starts with `sk-`. Get from platform.deepseek.com |
| `OPENROUTER_API_KEY` | No | — | Alternative: use OpenRouter instead of DeepSeek direct |
| `CONSENSUS_PORT` | No | `8090` | Change if port 8090 is occupied |
| `CONSENSUS_DB_URL` | No | `sqlite://...` | PostgreSQL: `postgres://user:pass@host/db?sslmode=require` |
| `CONSENSUS_API_KEY` | No | — | Protect your API with an auth key |
| `CONSENSUS_AUTO_SYNC` | No | — | Auto-refresh model registry (e.g., `24h`) |

---

> **Verified on:** Linux (Ubuntu 24.04, Go 1.26.5, Docker 29.1.3) — 2026-07-22
> **Needs verification:** macOS, Windows (WSL2) — these sections are based on known platform behaviors; report any issues via GitHub Issues.
