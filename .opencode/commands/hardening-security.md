---
name: hardening-security
description: Security audit — injection vulnerabilities (SQL, command, template), auth/authz gaps, hardcoded secrets, input validation, CVE-vulnerable dependencies, PII in logs. All findings require human review before Axiom executes.
---

# Hardening: Security Audit

Load `hardening-anti-patterns-axiom` for the shared audit header and finding format.
Load `hardening-security-axiom` for the full security audit checklist and remediation patterns.

Run the security audit prompt against this codebase.

**ALL findings in this category MUST have `requires_human_review: true`.** Security findings are never auto-resolved by Axiom without human approval.

Key areas to check:
- Injection: SQL, NoSQL, command injection, template injection, unsafe deserialization
- Auth/authz: missing auth checks, broken access control, privilege escalation
- Secrets: hardcoded values, secrets in logs, overly-broad IAM permissions
- Input validation: unvalidated user input, CSRF, unsafe file uploads, SSRF
- Dependencies: known CVEs (pair with Snyk/Trivy/Dependabot for CVE scanning)
- Data handling: PII in logs, insufficient encryption, missing redaction

Note: This prompt catches architectural misuse. Pair with a dedicated SCA tool (Snyk, Trivy, Dependabot) for CVE detection.

axiom:trace work_item=hardening-skills-01 jira_ref=SWDE-7 plan=phase-1/task-10/step-3
