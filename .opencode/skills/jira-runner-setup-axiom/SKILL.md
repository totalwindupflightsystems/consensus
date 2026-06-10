# Skill: jira-runner-setup-axiom

**Purpose**: Get Axiom running as a live Jira-to-PR automation pipeline from a fresh Coder workspace. Covers install, Docker image build, credentials, continuous polling via cron, and first pipeline run.

**When to load**: When you need to set up or restore the Jira runner pipeline on a new or reset workspace.

---

## What This Pipeline Does

When a Jira ticket is assigned to `svc_axiom@dexdat.ai` **on any board in the workspace**, Axiom:

1. Polls Jira every 60 seconds and finds the ticket (JQL: `assignee = currentUser() AND resolution = Unresolved`)
2. Creates a Docker volume and clones the repo from `customfield_11906` into it
3. Launches a `axiom-runner` container
4. Inside the container: creates a branch, starts OpenCode, runs `/axiom-jira-intake`, implements the work, pushes the branch, opens a PR, and posts a Jira comment with the PR link

**Any project works** — the JQL has no project filter. Just assign the ticket to `svc_axiom@dexdat.ai` and set the two custom fields.

**Required custom fields on the ticket:**
| Field | ID | What to put |
|-------|----|-------------|
| `codeops_repositories` | `customfield_11906` | GitHub slug, e.g. `fl97inc/Axiom` |
| `codeops_branch` | `customfield_11907` | Target branch, e.g. `main` |

---

## Pre-flight Checklist

Run these before anything else:

```bash
# 1. Docker is accessible
docker ps

# 2. Network reaches Atlassian
curl -sf https://dexdat.atlassian.net | head -1

# 3. AWS IRSA token exists (needed for Bedrock/OpenCode)
ls /var/run/secrets/eks.amazonaws.com/serviceaccount/token
```

If any of these fail, stop — the environment isn't ready.

---

## Step 1 — Install axiom CLI

The system Python is managed by Debian (PEP 668). Use `uv` to create a venv:

```bash
# Install uv if not present
pip install uv --break-system-packages

# Create venv and install axiom
uv venv /home/coder/.venv-axiom --python 3.12
uv pip install -e .axiom/ --python /home/coder/.venv-axiom/bin/python

# Add to PATH permanently
echo 'export PATH="/home/coder/.venv-axiom/bin:$PATH"' >> ~/.bashrc
echo 'export PATH="/home/coder/.venv-axiom/bin:$PATH"' >> ~/.profile
export PATH="/home/coder/.venv-axiom/bin:$PATH"

# Verify
axiom --version
# Expected: axiom 0.19.0+<hash>
```

---

## Step 2 — Build the Runner Docker Image

```bash
cd /home/coder/code/Axiom
docker build -t axiom-runner:latest -f docker/runner/Dockerfile .

# Smoke test (all three must work)
docker run --rm axiom-runner:latest axiom --version
docker run --rm axiom-runner:latest opencode --version
docker run --rm axiom-runner:latest gh --version
```

Build takes ~3-4 minutes on first run (downloads Python 3.13, gh CLI, opencode).
Subsequent builds use Docker layer cache and take ~20 seconds.

---

## Step 3 — Create .env

Create `.env` at the repo root with these values:

```bash
# Jira (Basic auth)
AXIOM_JIRA_BASE_URL=https://dexdat.atlassian.net
AXIOM_JIRA_USER_EMAIL=svc_axiom@dexdat.ai
AXIOM_JIRA_API_TOKEN=<classic API token from id.atlassian.com for svc_axiom@dexdat.ai>
AXIOM_JIRA_PROJECT=SWDE
AXIOM_JIRA_CLOUD_ID=a73deb7f-b0bf-4dd6-ae1c-2a378490eae8
AXIOM_JIRA_FIELD_REPOSITORIES=customfield_11906
AXIOM_JIRA_FIELD_BRANCH=customfield_11907

# GitHub
AXIOM_GITHUB_TOKEN=<GitHub PAT — needs repo + workflow scopes>
AXIOM_GITHUB_ORG=fl97inc
AXIOM_GITHUB_REPO=Axiom

# Auth mode
AXIOM_JIRA_AUTH_MODE=basic

# Short-name aliases for the runner entrypoint (docker/runner/entrypoint.sh)
JIRA_URL=https://dexdat.atlassian.net
JIRA_TOKEN=<same as AXIOM_JIRA_API_TOKEN>
JIRA_USER_EMAIL=svc_axiom@dexdat.ai
GITHUB_TOKEN=<same as AXIOM_GITHUB_TOKEN>
```

**Note on the two sets of vars**: The `AXIOM_*` vars are read by the host-side `axiom` CLI. The short-name vars (`JIRA_URL`, `JIRA_TOKEN`, `GITHUB_TOKEN`) are read by `docker/runner/entrypoint.sh` inside the container. Both sets must be present.

**Where to get the Jira token**: Log in to `id.atlassian.com` as `svc_axiom@dexdat.ai` → Security → API tokens → Create classic token.

**GitHub token scopes needed**: `repo`, `workflow`, `pull_request` (classic PAT is fine; fine-grained PAT also works).

---

## Step 4 — Verify Connectivity

```bash
export PATH="/home/coder/.venv-axiom/bin:$PATH"
set -a && source .env && set +a

# Test Jira
axiom jira-poll --once --dry-run
# Expected: Found 1 matching tickets: SWDE-13: [Axiom Pipeline Test]...

# Test GitHub API
curl -sf -H "Authorization: token $AXIOM_GITHUB_TOKEN" \
  https://api.github.com/repos/fl97inc/Axiom \
  | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'repo={d[\"full_name\"]} push={d[\"permissions\"][\"push\"]}')"
# Expected: repo=fl97inc/Axiom push=True

# Test AWS/Bedrock
python3 -c "import boto3; c=boto3.client('sts'); print(c.get_caller_identity()['Arn'])"
# Expected: arn:aws:sts::535002886782:assumed-role/solo-coder-workspace-bedrock/...
```

---

## Step 5 — Start the Continuous Poller (cron-managed)

The poller runs as a background process managed by cron. It polls every 60 seconds and auto-restarts if it dies.

### First-time setup (run once per workspace)

```bash
# Install cron
sudo apt-get install -y cron
sudo /usr/sbin/cron

# Install the crontab (idempotent — safe to run again)
(crontab -l 2>/dev/null; echo "# Axiom Jira poller — start on reboot, restart every 5 min if stopped") | crontab -
(crontab -l 2>/dev/null; echo "@reboot /home/coder/code/Axiom/scripts/run-jira-poller.sh >> /home/coder/code/Axiom/_tmp/jira-poller.log 2>&1") | crontab -
(crontab -l 2>/dev/null; echo "*/5 * * * * /home/coder/code/Axiom/scripts/run-jira-poller.sh >> /home/coder/code/Axiom/_tmp/jira-poller.log 2>&1") | crontab -

# Start it now (don't wait for reboot)
/home/coder/code/Axiom/scripts/run-jira-poller.sh
```

### Day-to-day commands

```bash
# Check if running
./scripts/run-jira-poller.sh --status
# Expected: RUNNING (pid=XXXXX)

# Watch the log
tail -f _tmp/jira-poller.log

# Single test cycle (dry-run)
./scripts/run-jira-poller.sh --once --dry-run   # not yet — use:
set -a && source .env && set +a
axiom jira-poll --once --dry-run

# Stop the poller
kill $(cat _tmp/jira-poller.pid)

# Restart
./scripts/run-jira-poller.sh
```

---

## Step 6 — Cut a Ticket and Watch It Work

1. Go to any Jira project on `dexdat.atlassian.net`
2. Create a ticket with a clear description of what to implement
3. Set the two custom fields:
   - **codeops_repositories** (`customfield_11906`): `org/repo` e.g. `fl97inc/Axiom`
   - **codeops_branch** (`customfield_11907`): `main` (or whatever branch to PR against)
4. Assign the ticket to **`svc_axiom@dexdat.ai`**
5. Within 60 seconds the poller picks it up — watch `_tmp/jira-poller.log`

That's it. No other action needed.

---

## Step 7 — Monitor the Container

```bash
# Is it running?
docker ps | grep axiom-<jira-key-lowercase>   # e.g. axiom-swde-13

# Watch logs live
docker logs -f axiom-<jira-key-lowercase>

# Key events to look for (in order):
# runner_entrypoint_started
# environment_validated
# workspace_already_cloned
# work_branch_ready
# opencode_health_ok          ← OpenCode is up
# running_jira_intake         ← axiom run started
# jira_intake_completed       ← intake done, plan produced
# running_step_loop           ← implementation loop started
# push_branch_completed       ← code pushed to GitHub
# pr_created                  ← PR opened
# jira_comment_posted         ← comment on ticket with PR link
```

---

## Architecture: Why Docker Volumes (Not Bind Mounts)

This Coder workspace runs inside a Kubernetes pod. The Docker daemon is a sidecar that runs on the **outer host**, not inside the pod. When you pass a bind mount path like `-v /home/coder/code/...:/workspace`, Docker resolves it on the outer host where that path doesn't exist — the container sees an empty directory.

**Solution**: Use named Docker volumes. The launcher:
1. Creates a named volume (`axiom-swde-13-workspace`)
2. Clones the repo directly into the volume using a setup container
3. Mounts the volume into the main runner container

This is handled automatically by `ContainerLauncher` in `.axiom/src/axiom/control_plane/container_launcher.py`.

---

## Architecture: AWS/Bedrock Auth

OpenCode uses Amazon Bedrock for LLM inference. Auth uses EKS IRSA (IAM Roles for Service Accounts):

- The Kubernetes service account token lives at `/var/run/secrets/eks.amazonaws.com/serviceaccount/token`
- The container launcher mounts this token read-only into every runner container
- The env vars `AWS_ROLE_ARN` and `AWS_WEB_IDENTITY_TOKEN_FILE` tell boto3/botocore to use web identity auth
- Role: `arn:aws:iam::535002886782:role/solo-coder-workspace-bedrock`

No static AWS credentials are needed or used.

---

## Key Files

| File | Purpose |
|------|---------|
| `.env` | All credentials (not committed) |
| `docker/runner/Dockerfile` | Runner image definition |
| `docker/runner/entrypoint.sh` | 12-step lifecycle inside the container |
| `.axiom/src/axiom/control_plane/poller.py` | Jira polling loop |
| `.axiom/src/axiom/control_plane/container_launcher.py` | Docker volume + container lifecycle |
| `.axiom/src/axiom/control_plane/workspace_manager.py` | Repo clone management |
| `.axiom/src/axiom/cli/main.py` | `axiom jira-poll` CLI handler |
| `_tmp/jira_poller_state.db` | SQLite state tracker (delete to reset) |

---

## Test Ticket

**SWDE-13** — "[Axiom Pipeline Test] Add GET /hello endpoint to axiom HTTP server"
- URL: `https://dexdat.atlassian.net/browse/SWDE-13`
- Assigned to: `svc_axiom@dexdat.ai`
- `customfield_11906` = `fl97inc/Axiom`
- `customfield_11907` = `main`

---

## Troubleshooting

### Jira 401
```bash
# Test auth directly
source .env
python3 -c "
import urllib.request, base64, json, os
creds = base64.b64encode(f'{os.environ[\"AXIOM_JIRA_USER_EMAIL\"]}:{os.environ[\"AXIOM_JIRA_API_TOKEN\"]}'.encode()).decode()
req = urllib.request.Request('https://dexdat.atlassian.net/rest/api/3/myself',
    headers={'Authorization': f'Basic {creds}', 'Accept': 'application/json'})
with urllib.request.urlopen(req) as r:
    print(json.loads(r.read()).get('displayName'))
"
# Token may have expired (1 year max). Regenerate at id.atlassian.com.
```

### SWDE-13 not found by poller
```bash
# Check if already dispatched
sqlite3 _tmp/jira_poller_state.db "SELECT issue_key, state FROM tracked_tickets;"
# Reset
rm -f _tmp/jira_poller_state.db
```

### Container exits immediately
```bash
docker logs axiom-swde-13
# Common causes:
# - Missing env vars (JIRA_URL, JIRA_TOKEN, GITHUB_TOKEN, JIRA_USER_EMAIL)
# - OpenCode crashes (check opencode_stderr log event)
# - axiom run fails (check jira_intake_failed log event)
```

### OpenCode crashes on startup
The entrypoint runs `opencode serve --port 4096` from `WORKSPACE_ROOT`. OpenCode reads `opencode.jsonc` from the working directory for model/provider config. If the repo's `opencode.jsonc` is missing or malformed, OpenCode will exit. The repo's config uses Amazon Bedrock — ensure AWS credentials are forwarded (see AWS/Bedrock Auth section above).

### Docker volume is empty / bind mount not working
This is the DinD issue described in the Architecture section. The fix is already in place (named volumes). If you see empty `/workspace` inside a container, check that `ContainerLauncher` is using `--volume volume_name:/workspace` not `--volume /host/path:/workspace`.

### GitHub clone fails with "Invalid username or token"
The workspace manager uses `https://oauth2:<token>@github.com/...` for cloning. Classic PATs work with this format. Fine-grained PATs also work. If you see this error, check that `AXIOM_GITHUB_TOKEN` is set and the token has `repo` scope.

---

## Resetting for a Fresh Run

```bash
# Stop and remove the container
docker rm -f axiom-swde-13

# Remove the workspace volume
docker volume rm axiom-swde-13-workspace

# Reset the state tracker
rm -f _tmp/jira_poller_state.db

# Remove host-side workspace clone (optional)
rm -rf workspace/swde-13/

# Now re-run
axiom jira-poll --once
```

---

## Known Fixes Applied in This Session (2026-04-25)

These bugs were found and fixed during the first live run. All fixes are committed.

| Bug | Fix | File |
|-----|-----|------|
| `axiom jira-poll` didn't wire `ContainerLauncher` or `WorkspaceManager` | Added instantiation + wiring in CLI handler | `cli/main.py` |
| `PollerConfig` had no `env_file` | Added `env_file` from `AXIOM_ENV_FILE` or `.env` | `cli/main.py` |
| `github_token=None` passed to `ensure_running` | Read from `AXIOM_GITHUB_TOKEN` env var | `poller.py` |
| Clone URL used `x-access-token:` prefix (rejected by GitHub) | Changed to `oauth2:` prefix | `workspace_manager.py` |
| `REPO_URL` passed as bare slug (`fl97inc/Axiom`) | Expanded to full `https://github.com/...` URL | `container_launcher.py` |
| Bind mount path invisible to Docker daemon (DinD) | Switched to named Docker volumes; clone inside volume via setup container | `container_launcher.py` |
| `opencode serve --workspace` flag doesn't exist in v1.14.24 | Removed `--workspace` flag; use `cd` instead | `entrypoint.sh` |
| `AXIOM_OPENCODE_BASE_URL` not set before `axiom run` | Added `export AXIOM_OPENCODE_BASE_URL` before kickoff and ralph loop | `entrypoint.sh` |
| Dirty worktree blocked `axiom run` | Added `git add -A && git commit` before kickoff | `entrypoint.sh` |
| AWS/Bedrock creds not forwarded to container | Added IRSA token mount + `AWS_*` env vars to container launch | `container_launcher.py` |

---

`axiom:trace work_item=jira-coder-pipeline-01 spec=specs/05-Jira-Integration.md jira_ref=SWDE-13`
