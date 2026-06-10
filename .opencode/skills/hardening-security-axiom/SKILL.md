---
name: hardening-security-axiom
description: >
  Security audit skill for any codebase. Covers injection vulnerabilities (SQL, command,
  template), authentication/authorization gaps, hardcoded secrets, input validation,
  CVE-vulnerable dependencies, and data handling (PII in logs, missing encryption).
  Complements security-review-axiom (STRIDE/threat model) — this skill focuses on
  code-level patterns. All findings require requires_human_review: true.
version: "1.0"
tags:
  vertical: [security, coding]
  category: hardening
  core: false
metadata:
  related_skills:
    - hardening-anti-patterns-axiom
    - hardening-intake-axiom
    - security-review-axiom
    - privacy-compliance-axiom
  note: >
    This skill complements security-review-axiom (STRIDE threat modeling, security gates).
    Use this skill for code-level pattern detection. Use security-review-axiom for
    architectural threat modeling and release gates.
---

# Hardening: Security Audit

> **"Security is not a feature. It is a constraint that applies to every feature."**
>
> **"Never invent evidence. If you didn't see the issue, mark it suspected. Do not produce exploit code — produce remediation guidance."**

**⚠️ All security findings MUST have `requires_human_review: true`.** Security findings are never auto-resolved by Axiom without human approval.

This skill is portable — no Axiom-internal dependencies. Any agent in any repo can load it.

## When to Load This Skill

- Quarterly security audit of a codebase
- Before a major release or launch
- After adding new user-facing endpoints
- When a dependency CVE is reported
- When `security-review-axiom` flags a code-level concern

---

## The Security Audit Prompt

Use this prompt (with the shared header from `hardening-anti-patterns-axiom`):

```
Conduct a security review of this codebase. Focus on:

- Injection vulnerabilities: SQL, NoSQL, command injection, template
  injection, unsafe deserialization
- Authentication and authorization: missing auth checks, broken
  access control, privilege escalation paths, session handling
- Secrets and credentials: hardcoded values, secrets in logs,
  overly-broad IAM permissions, long-lived tokens
- Input validation: unvalidated user input reaching sensitive sinks,
  missing CSRF protection, unsafe file uploads, SSRF
- Dependencies: known-vulnerable packages, unpinned versions
- Data handling: PII in logs, insufficient encryption, missing redaction

For each finding, note whether it's exploitable as-is or requires
a specific precondition. Do not produce exploit code — produce
remediation guidance.

All findings in this category: flag with requires_human_review: true.
```

**Note on CVE scanning:** This prompt catches architectural misuse. Pair it with a dedicated SCA tool (Snyk, Trivy, Dependabot) for CVE detection. They complement, not substitute.

---

## Security Audit Checklist

### Injection Vulnerabilities

- [ ] **SQL injection** — string concatenation in queries
- [ ] **NoSQL injection** — unvalidated input in MongoDB/Redis queries
- [ ] **Command injection** — `shell=True` with user input, `os.system()`
- [ ] **Template injection** — user input in Jinja2/Mako/Handlebars templates
- [ ] **Unsafe deserialization** — `yaml.load()` without SafeLoader, `pickle.loads()` on untrusted data
- [ ] **Path traversal** — `..` in file paths, unvalidated file names

### Authentication & Authorization

- [ ] **Missing authentication** — endpoints accessible without login
- [ ] **Missing authorization** — authenticated users can access other users' data
- [ ] **Broken access control** — role checks missing or bypassable
- [ ] **Privilege escalation** — user can elevate their own permissions
- [ ] **Session fixation** — session ID not rotated on login
- [ ] **Insecure JWT** — `alg: none`, weak secret, no expiry

### Secrets & Credentials

- [ ] **Hardcoded secrets** — passwords, API keys, tokens in source code
- [ ] **Secrets in logs** — passwords, tokens logged in plaintext
- [ ] **Secrets in environment** — `.env` files committed to git
- [ ] **Overly-broad IAM** — `*` permissions, admin roles for service accounts
- [ ] **Long-lived tokens** — no expiry, no rotation

### Input Validation

- [ ] **Unvalidated user input** — reaches database, filesystem, or shell
- [ ] **Missing CSRF protection** — state-changing requests without CSRF token
- [ ] **Unsafe file uploads** — no type validation, no size limit, stored in web root
- [ ] **SSRF** — user-controlled URLs fetched by server without validation
- [ ] **Open redirect** — user-controlled redirect URLs

### Dependencies

- [ ] **Known CVEs** — run `pip-audit`, `npm audit`, `trivy`, or `snyk test`
- [ ] **Unpinned versions** — `requirements.txt` without pinned versions
- [ ] **Abandoned packages** — last release > 2 years ago, no security patches

### Data Handling

- [ ] **PII in logs** — email, phone, SSN, credit card in log output
- [ ] **Unencrypted sensitive data** — passwords stored as plaintext
- [ ] **Missing data redaction** — sensitive fields in API responses
- [ ] **Insecure transmission** — HTTP instead of HTTPS for sensitive data

---

## Detection Patterns

### Grep Commands

```bash
# SQL injection: string concatenation in queries
grep -rn "execute.*f\"\|execute.*format(\|execute.*%\s*(" \
  --include="*.py" | grep -v "parameterized\|%s\|:param\|bindparam"

# Command injection: shell=True
grep -rn "subprocess.*shell=True\|os\.system(\|os\.popen(" --include="*.py"

# Unsafe YAML parsing
grep -rn "yaml\.load(" --include="*.py" | grep -v "Loader=yaml\.SafeLoader\|yaml\.safe_load"

# Unsafe pickle
grep -rn "pickle\.loads\|pickle\.load(" --include="*.py"

# Hardcoded secrets (common patterns)
grep -rn \
  -e "password\s*=\s*['\"][^'\"]\+['\"]" \
  -e "api_key\s*=\s*['\"][^'\"]\+['\"]" \
  -e "secret\s*=\s*['\"][^'\"]\+['\"]" \
  -e "token\s*=\s*['\"][^'\"]\+['\"]" \
  --include="*.py" --include="*.js" --include="*.go" \
  | grep -v "os\.environ\|os\.getenv\|get_secret\|config\.\|settings\.\|env\."

# PII in logs
grep -rn \
  -e "logger.*email\|log.*email" \
  -e "logger.*password\|log.*password" \
  -e "logger.*credit_card\|log.*card_number" \
  -e "logger.*ssn\|log.*social_security" \
  --include="*.py" --include="*.js"

# Missing auth check (Flask/FastAPI routes without auth decorator)
grep -rn "@app\.route\|@router\." --include="*.py" -A 5 \
  | grep -v "login_required\|require_auth\|get_current_user\|Depends"

# SSRF: user-controlled URL fetched by server
grep -rn "requests\.get.*request\.\|httpx\.get.*request\." --include="*.py"

# Path traversal: unvalidated file paths
grep -rn "open(.*request\.\|open(.*params\[" --include="*.py"
```

---

## Anti-Patterns with Fixes

### AP-SEC-001: SQL Injection via String Concatenation

**Severity:** critical | `requires_human_review: true`

```python
# BAD: String concatenation allows SQL injection
def get_user(username):
    query = f"SELECT * FROM users WHERE username = '{username}'"
    cursor.execute(query)
    return cursor.fetchone()

# Exploit: username = "admin' OR '1'='1' --"
# Result: SELECT * FROM users WHERE username = 'admin' OR '1'='1' --'
# Returns ALL users!
```

**Fix:**
```python
# GOOD: Parameterized query
def get_user(username):
    query = "SELECT * FROM users WHERE username = %s"
    cursor.execute(query, (username,))
    return cursor.fetchone()

# GOOD: SQLAlchemy ORM (preferred)
from sqlalchemy import select
from models import User

def get_user(username):
    stmt = select(User).where(User.username == username)
    return session.execute(stmt).scalar_one_or_none()
```

---

### AP-SEC-002: Command Injection via shell=True

**Severity:** critical | `requires_human_review: true`

```python
# BAD: shell=True with user input
import subprocess

def list_directory(dirname):
    result = subprocess.run(f"ls {dirname}", shell=True, capture_output=True)
    return result.stdout

# Exploit: dirname = "/tmp; cat /etc/passwd"
# Result: lists /tmp AND dumps /etc/passwd
```

**Fix:**
```python
# GOOD: Pass arguments as list, shell=False (default)
import subprocess
import re

def list_directory(dirname):
    # Validate input first
    if not re.match(r'^[a-zA-Z0-9_\-/]+$', dirname):
        raise ValueError(f"Invalid directory name: {dirname!r}")
    
    result = subprocess.run(
        ["ls", dirname],
        capture_output=True,
        text=True,
        timeout=10
    )
    return result.stdout
```

---

### AP-SEC-003: Hardcoded Secrets

**Severity:** critical | `requires_human_review: true`

```python
# BAD: Secrets in source code
DATABASE_PASSWORD = "super_secret_password_123"
STRIPE_API_KEY = "sk_live_abc123xyz"
AWS_SECRET_KEY = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
```

**Fix:**
```python
# GOOD: Load from environment variables
import os

DATABASE_PASSWORD = os.environ["DATABASE_PASSWORD"]  # Raises if missing
STRIPE_API_KEY = os.environ.get("STRIPE_API_KEY")    # Returns None if missing

# BETTER: Use a secret manager
import boto3

def get_secret(secret_name: str) -> str:
    client = boto3.client("secretsmanager")
    response = client.get_secret_value(SecretId=secret_name)
    return response["SecretString"]

STRIPE_API_KEY = get_secret("prod/stripe/api_key")
```

**Scanning for leaked secrets:**
```bash
# Use gitleaks to scan git history
gitleaks detect --source . --verbose

# Use trufflehog for deep git history scan
trufflehog git file://. --only-verified
```

---

### AP-SEC-004: Missing Authorization Check

**Severity:** critical | `requires_human_review: true`

```python
# BAD: Checks authentication but not authorization
@app.route("/api/users/<int:user_id>/delete", methods=["DELETE"])
def delete_user(user_id):
    if not current_user.is_authenticated:
        return {"error": "Unauthorized"}, 401
    # Any logged-in user can delete ANY user!
    User.query.filter_by(id=user_id).delete()
    db.session.commit()
    return {"status": "deleted"}
```

**Fix:**
```python
# GOOD: Check both authentication AND authorization
@app.route("/api/users/<int:user_id>/delete", methods=["DELETE"])
def delete_user(user_id):
    if not current_user.is_authenticated:
        return {"error": "Unauthorized"}, 401
    
    # Authorization: only admins or the user themselves
    if current_user.id != user_id and not current_user.has_role("admin"):
        return {"error": "Forbidden"}, 403
    
    user = User.query.get_or_404(user_id)
    db.session.delete(user)
    db.session.commit()
    
    # Audit log
    logger.info("user_deleted", 
                deleted_user_id=user_id, 
                actor_id=current_user.id,
                actor_role=current_user.role)
    
    return {"status": "deleted"}
```

---

### AP-SEC-005: Unsafe YAML Parsing

**Severity:** high | `requires_human_review: true`

```python
# BAD: yaml.load() can execute arbitrary Python code
import yaml

def parse_config(config_str):
    return yaml.load(config_str)  # Dangerous!

# Exploit payload:
# !!python/object/apply:os.system ["rm -rf /"]
```

**Fix:**
```python
# GOOD: Always use yaml.safe_load()
import yaml

def parse_config(config_str):
    return yaml.safe_load(config_str)  # Safe: only loads basic YAML types
```

---

### AP-SEC-006: PII in Logs

**Severity:** critical | `requires_human_review: true`

```python
# BAD: Logging PII
logger.info(f"User login: email={email}, password={password}")
logger.info(f"Payment: card={credit_card_number}, cvv={cvv}")
```

**Fix:**
```python
# GOOD: Redact PII before logging
import re
import hashlib

def redact_email(email: str) -> str:
    """Hash email for correlation without exposing PII."""
    return hashlib.sha256(email.encode()).hexdigest()[:16]

def redact_card(card: str) -> str:
    """Show only last 4 digits."""
    return f"****{card[-4:]}" if len(card) >= 4 else "****"

# Log with redacted values
logger.info("user_login",
            email_hash=redact_email(email),  # Correlatable but not PII
            user_id=user_id)                  # Safe to log

logger.info("payment_processed",
            card_last4=redact_card(credit_card_number),
            order_id=order_id)
```

---

### AP-SEC-007: SSRF via User-Controlled URL

**Severity:** high | `requires_human_review: true`

```python
# BAD: Server fetches user-provided URL without validation
@app.route("/api/preview")
def preview_url():
    url = request.args.get("url")
    response = requests.get(url)  # SSRF! Can reach internal services
    return response.content

# Exploit: url = "http://169.254.169.254/latest/meta-data/"
# (AWS metadata service — exposes IAM credentials)
```

**Fix:**
```python
# GOOD: Validate URL against allowlist of safe domains
import ipaddress
import socket
from urllib.parse import urlparse

ALLOWED_SCHEMES = {"https"}
BLOCKED_HOSTS = {"localhost", "127.0.0.1", "0.0.0.0"}
BLOCKED_NETWORKS = [
    ipaddress.ip_network("10.0.0.0/8"),
    ipaddress.ip_network("172.16.0.0/12"),
    ipaddress.ip_network("192.168.0.0/16"),
    ipaddress.ip_network("169.254.0.0/16"),  # Link-local (AWS metadata)
]

def validate_url(url: str) -> bool:
    parsed = urlparse(url)
    
    if parsed.scheme not in ALLOWED_SCHEMES:
        return False
    
    if parsed.hostname in BLOCKED_HOSTS:
        return False
    
    # Resolve hostname and check against blocked networks
    try:
        ip = ipaddress.ip_address(socket.gethostbyname(parsed.hostname))
        for network in BLOCKED_NETWORKS:
            if ip in network:
                return False
    except (socket.gaierror, ValueError):
        return False
    
    return True

@app.route("/api/preview")
def preview_url():
    url = request.args.get("url")
    if not validate_url(url):
        return {"error": "URL not allowed"}, 400
    
    response = requests.get(url, timeout=10)
    return response.content
```

---

## Finding Templates

### HARDEN-SEC-SQL-INJECTION

```yaml
id: HARDEN-SEC-SQL-INJECTION
severity: critical
category: security
location: "path/to/file.py:42"
description: "SQL query built via string concatenation allows SQL injection."
impact: >
  Attacker can inject arbitrary SQL, potentially extracting all data from the database,
  modifying records, or dropping tables. Exploitable as-is with no preconditions.
recommendation: >
  Replace string concatenation with parameterized query:
  cursor.execute("SELECT * FROM users WHERE username = %s", (username,))
  Or use SQLAlchemy ORM which parameterizes automatically.
acceptance_criteria:
  - "Malicious input (e.g., \"' OR '1'='1\") is rejected or escaped, verified by sending the payload and observing no data leak"
  - "Query log shows parameterized query, not string-concatenated query"
  - "Unit test verifies injection payload returns empty result, not all records"
verification_tier: 3
confidence: confirmed
assumptions: "Database is PostgreSQL/MySQL; parameterization syntax may differ for other DBs"
requires_human_review: true
```

### HARDEN-SEC-HARDCODED-SECRET

```yaml
id: HARDEN-SEC-HARDCODED-SECRET
severity: critical
category: security
location: "path/to/config.py:15"
description: "API key hardcoded in source code."
impact: >
  Secret exposed in version control. Anyone with repo access (or who finds the repo
  in a leak) has the API key. Cannot rotate without code change and redeployment.
recommendation: >
  Move to environment variable: API_KEY = os.environ["API_KEY"]
  Or use secret manager: API_KEY = get_secret("prod/api/key")
  Rotate the exposed key immediately after removing from code.
acceptance_criteria:
  - "GitLeaks scan returns 0 findings for this secret pattern"
  - "Secret loaded from environment variable or secret manager, verified by removing env var and observing startup failure with clear error"
  - "Exposed key rotated and old key revoked"
verification_tier: 3
confidence: confirmed
assumptions: "Secret is still valid and not yet rotated"
requires_human_review: true
```

---

## CVE Scanning Integration

Pair this skill with automated CVE scanning:

```bash
# Python: pip-audit
pip-audit --requirement requirements.txt

# Python: safety
safety check -r requirements.txt

# Node.js: npm audit
npm audit --audit-level=high

# Docker images: trivy
trivy image myapp:latest

# Multi-language: snyk
snyk test

# GitHub: Dependabot
# Add .github/dependabot.yml to enable automatic PRs for vulnerable deps
```

**Dependabot configuration:**
```yaml
# .github/dependabot.yml
version: 2
updates:
  - package-ecosystem: "pip"
    directory: "/"
    schedule:
      interval: "weekly"
    open-pull-requests-limit: 10
  - package-ecosystem: "npm"
    directory: "/"
    schedule:
      interval: "weekly"
```

---

## OWASP Top 10 Quick Reference (2023)

| # | Category | Key Pattern to Check |
|---|---|---|
| A01 | Broken Access Control | Missing authz checks, IDOR, path traversal |
| A02 | Cryptographic Failures | Hardcoded secrets, weak encryption, HTTP not HTTPS |
| A03 | Injection | SQL, command, template, SSTI |
| A04 | Insecure Design | Missing security controls in architecture |
| A05 | Security Misconfiguration | Default creds, verbose errors, open S3 buckets |
| A06 | Vulnerable Components | Outdated deps with CVEs |
| A07 | Auth Failures | Weak passwords, no MFA, broken session management |
| A08 | Software/Data Integrity | Unsigned packages, CI/CD attacks |
| A09 | Security Logging Failures | Missing audit trails, no alerting on auth failures |
| A10 | SSRF | User-controlled URLs fetched by server |

---

## Acceptance Criteria Templates (Tier 3+)

All security acceptance criteria must be verifiable at Tier 3 or higher:

1. **Injection blocked**: Send malicious payload → verify it is rejected or escaped → verify no data leak
2. **Auth enforced**: Send request without credentials → verify 401 → send with wrong role → verify 403
3. **Secret not in code**: Run GitLeaks → verify 0 findings → verify secret loads from env/vault
4. **CVE-free**: Run `pip-audit`/`npm audit` → verify 0 high/critical findings
5. **PII not in logs**: Trigger login/payment → grep logs for PII patterns → verify 0 matches

---

axiom:trace work_item=hardening-skills-01 spec=hardening-security-axiom jira_ref=SWDE-7 plan=phase-1/task-3/step-1
