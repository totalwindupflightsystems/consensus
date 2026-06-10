---
name: hardening-quality
description: Test coverage gap audit — critical paths with no tests, error paths with no tests, assertionless tests, tautology tests, over-mocking, flaky tests, integration boundary gaps. Findings describe specific tests to add with specific behaviors to verify.
---

# Hardening: Quality & Test Coverage Audit

Load `hardening-anti-patterns-axiom` for the shared audit header and finding format.
Load `hardening-quality-axiom` for the full quality audit checklist and remediation patterns.

Run the quality audit prompt against this codebase.

Don't just report low line coverage — report missing tests that would actually catch bugs.

For each finding, acceptance_criteria should describe the specific test to add and the specific behavior it verifies — not "add a test for function X" but "when function X receives an empty list, it returns an empty result rather than raising."

Key areas to check:
- Critical paths with no tests: auth, payments, data writes
- Error paths with no tests: what happens when a dependency returns an error?
- Edge cases with no tests: empty inputs, max-size, unicode, concurrency
- Integration boundaries with no tests: service-to-service, DB, queue, third-party
- Tests that don't test what their name says (assertionless, tautology)
- Flaky tests: depend on timing, ordering, or external state

axiom:trace work_item=hardening-skills-01 jira_ref=SWDE-7 plan=phase-1/task-10/step-6
