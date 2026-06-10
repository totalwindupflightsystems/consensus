---
name: chrome-devtools-mcp
description: >
  Portable, loadable guide for any agent with chrome-devtools MCP access. Covers all 29 tools
  organized by category, step-by-step workflows for UI verification, evidence capture, performance
  auditing, accessibility, SSE verification, and memory leak detection. Includes guardrails,
  Axiom evidence bundle integration, and agent access table. Load this skill when performing
  browser-based UI verification, debugging, or evidence capture using the Chrome DevTools MCP server.
version: "1.0"
tags:
  vertical: [coding]
  category: tooling
  core: false
---

# Chrome DevTools MCP Skill (Portable)

> **"A snapshot gives you UIDs. A screenshot gives you pixels. Always snapshot first."**
>
> **"Evidence without screenshots, console logs, and network requests is unverified."**

This skill is the operational guide for agents using the `chrome-devtools` MCP server in Axiom.
It covers all available tools, common workflows, guardrails, and evidence bundle integration.

## When to Load This Skill

Load this skill when:
- Performing browser-based UI verification for a work item
- Capturing evidence of UI behavior (screenshots, console, network)
- Running performance audits (Core Web Vitals, LCP, CLS, TBT)
- Running Lighthouse accessibility or SEO audits
- Verifying SSE/real-time connections are established
- Debugging a UI issue by inspecting DOM, console, or network
- Detecting memory leaks in long-running UI sessions
- Simulating user interactions (click, fill, drag, keyboard)
- Testing responsive/mobile layouts via emulation

**Do NOT load this skill if:**
- Your agent does not have `mcp.chrome-devtools: true` in its frontmatter (check AGENTS.md)
- You are writing Playwright E2E tests (use `e2e-browser-testing-playwright.md` instead)
- You are doing backend-only work with no browser component

---

## Agent Access Table

Only these agents have `mcp.chrome-devtools: true` per `AGENTS.md`:

| Agent | Role |
|-------|------|
| `@frontend-engineer-axiom` | Primary UI builder/verifier |
| `@accessibility-review-axiom` | WCAG audits |
| `@qa-axiom` | Quality assurance |
| `@performance-axiom` | Core Web Vitals and Lighthouse |
| `@ralph-wiggum-verify` | Verification agent |
| `@dispatch-axiom` | General team agent |
| `@tower-axiom` | Orchestrator |

All other agents have `mcp.chrome-devtools: false` to save context (~17,000 tokens overhead).

---

## Tool Reference Card

### Input Automation (9 tools)

| Tool | One-Line Description | Key Parameters |
|------|---------------------|----------------|
| `click` | Click an element by UID | `uid` (required), `dblClick` (optional) |
| `drag` | Drag element onto another element | `from_uid`, `to_uid` (both required) |
| `fill` | Type into input/textarea or select option | `uid`, `value` (both required) |
| `fill_form` | Fill multiple form fields at once | `elements: [{uid, value}]` (required) |
| `handle_dialog` | Accept or dismiss browser dialog | `action: "accept"|"dismiss"` (required) |
| `hover` | Hover over element (trigger tooltips) | `uid` (required) |
| `press_key` | Press key or key combination | `key` e.g. `"Enter"`, `"Control+A"` |
| `type_text` | Type text into focused input | `text` (required), `submitKey` (optional) |
| `upload_file` | Upload file via file input element | `uid`, `filePath` (both required) |

### Navigation Automation (6 tools)

| Tool | One-Line Description | Key Parameters |
|------|---------------------|----------------|
| `navigate_page` | Navigate to URL, back, forward, or reload | `type: "url"|"back"|"forward"|"reload"`, `url` |
| `new_page` | Open new tab and load URL | `url` (required), `isolatedContext` (optional) |
| `list_pages` | List all open tabs with IDs | none |
| `select_page` | Switch to a specific tab | `pageId` (required) |
| `close_page` | Close a tab | `pageId` (required) |
| `wait_for` | Wait for text to appear on page | `text: [string]` (required), `timeout` (optional) |

### Emulation (2 tools)

| Tool | One-Line Description | Key Parameters |
|------|---------------------|----------------|
| `emulate` | Emulate device viewport, color scheme, network, geolocation | `viewport`, `colorScheme`, `networkConditions`, `cpuThrottlingRate` |
| `resize_page` | Resize browser window | `width`, `height` (both required) |

### Performance (4 tools)

| Tool | One-Line Description | Key Parameters |
|------|---------------------|----------------|
| `performance_start_trace` | Start performance trace recording | `reload` (default true), `autoStop` (default true), `filePath` |
| `performance_stop_trace` | Stop active trace recording | `filePath` (optional) |
| `performance_analyze_insight` | Analyze specific performance insight from trace | `insightSetId` (from start_trace output), `insightName` |
| `take_memory_snapshot` | Capture JS heap snapshot for memory leak analysis | `filePath` (required, `.heapsnapshot`) |

### Network (2 tools)

| Tool | One-Line Description | Key Parameters |
|------|---------------------|----------------|
| `list_network_requests` | List all network requests since last navigation | `resourceTypes` (filter), `pageSize`, `pageIdx` |
| `get_network_request` | Get details of a specific request including body | `reqid`, `requestFilePath`, `responseFilePath` |

### Debugging (6 tools)

| Tool | One-Line Description | Key Parameters |
|------|---------------------|----------------|
| `evaluate_script` | Execute JavaScript in page context | `function` (JS function string), `args` (element UIDs) |
| `list_console_messages` | List console messages since last navigation | `types` (filter), `pageSize`, `pageIdx` |
| `get_console_message` | Get details of a specific console message | `msgid` (from list_console_messages) |
| `lighthouse_audit` | Run Lighthouse audit (accessibility, SEO, best practices — NOT performance) | `device`, `mode`, `outputDirPath` |
| `take_screenshot` | Capture screenshot of page or element | `filePath`, `uid` (element), `fullPage`, `format`, `quality` |
| `take_snapshot` | Capture a11y tree snapshot with UIDs for element interaction | `filePath`, `verbose` |

**Total: 29 tools** across 6 categories (~6,940 cl100k_base tokens for tool definitions).

---

## Core Rules

1. **Snapshot before interact.** Always call `take_snapshot` to get UIDs before `click`, `fill`, `hover`, or `drag`.
2. **Wait for observable state.** Call `wait_for(text=[...])` after navigation before inspecting or interacting.
3. **Evidence triad.** For any verification: screenshot + console messages + network requests.
4. **Localhost only.** Only open `localhost` or dev/staging URLs. Never open production pages with real user data.
5. **Save to file.** Use `filePath` parameter on `take_screenshot` and `take_snapshot` to save evidence to the work item run folder.
6. **Lighthouse ≠ performance.** `lighthouse_audit` covers accessibility/SEO/best-practices. For Core Web Vitals, use `performance_start_trace`.

---

## Workflow 1: UI Verification

**Goal:** Verify a UI feature works as expected after implementation.

```
Step 1: navigate_page(type="url", url="http://localhost:5173/")
Step 2: wait_for(text=["Dashboard"])          ← wait for stable state
Step 3: take_snapshot()                        ← get a11y tree + UIDs
Step 4: take_screenshot(filePath="evidence/initial-state.png")
Step 5: list_console_messages(types=["error", "warn"])  ← check for errors
Step 6: list_network_requests()               ← verify API calls
Step 7: [interact as needed using UIDs from step 3]
         click(uid="<uid>")
         fill(uid="<uid>", value="test value")
         press_key(key="Enter")
Step 8: wait_for(text=["expected result"])
Step 9: take_snapshot()                        ← verify post-interaction state
Step 10: take_screenshot(filePath="evidence/post-action.png")
Step 11: list_console_messages(types=["error"])  ← no new errors
```

**Checklist:**
- [ ] Page loaded without console errors
- [ ] Expected elements present in snapshot
- [ ] Network requests returned expected status codes
- [ ] Post-interaction state matches spec
- [ ] Screenshots saved to evidence path

---

## Workflow 2: Evidence Capture

**Goal:** Produce a complete evidence bundle for a work item verification.

```
Step 1: navigate_page(type="url", url="<target URL>")
Step 2: wait_for(text=["<stable indicator>"])
Step 3: take_screenshot(
          filePath=".memory-bank/work-items/<ID>/runs/<RUN_ID>/ui-state.png",
          fullPage=true
        )
Step 4: list_console_messages()
        → record: 0 errors, N info messages
Step 5: list_network_requests()
        → record: key API calls and their status codes
Step 6: take_snapshot(
          filePath=".memory-bank/work-items/<ID>/runs/<RUN_ID>/dom-snapshot.txt"
        )
```

**Evidence bundle entry (add to verification.md):**
```markdown
## Browser Verification Evidence

**Tool:** Chrome DevTools MCP  
**Date:** <date>  
**URL:** <url>

### Screenshot
![UI State](./<RUN_ID>/ui-state.png)

### Console Messages
- Errors: 0
- Warnings: 0
- Info: ["SSE connected", "Run list loaded"]

### Network Requests
| URL | Method | Status | Type |
|-----|--------|--------|------|
| /api/v1/runs | GET | 200 | fetch |
| /api/v1/events/stream | GET | 200 | eventsource |

### DOM Snapshot (excerpt)
[paste relevant a11y tree section]

### Trace
`axiom:trace work_item=<ID> evidence=runs/<RUN_ID>/ui-state.png`
```

---

## Workflow 3: Performance Audit

**Goal:** Measure Core Web Vitals and identify performance bottlenecks.

```
Step 1: navigate_page(type="url", url="http://localhost:5173/")
        ← navigate to the page BEFORE starting trace
Step 2: performance_start_trace(
          reload=true,      ← reload page and record from navigation start
          autoStop=true,    ← stop automatically after page load
          filePath="evidence/trace.json.gz"
        )
        → returns: { insightSetId: "set-001", insights: [...] }
Step 3: [wait for trace to complete — autoStop handles this]
Step 4: performance_analyze_insight(
          insightSetId="set-001",
          insightName="LCPBreakdown"
        )
        → returns: LCP value, breakdown by phases
Step 5: performance_analyze_insight(
          insightSetId="set-001",
          insightName="DocumentLatency"
        )
Step 6: performance_analyze_insight(
          insightSetId="set-001",
          insightName="RenderBlocking"
        )
Step 7: take_screenshot(filePath="evidence/performance-state.png")
```

**Performance budgets (from specs/38-UX-Design-Principles.md):**
- LCP < 2.5s
- FCP < 1.8s
- CLS < 0.1

**Key insight names:**
- `LCPBreakdown` — Largest Contentful Paint phases
- `DocumentLatency` — TTFB and document load
- `RenderBlocking` — Render-blocking resources
- `SlowCSSSelector` — CSS selector performance
- `ForcedReflow` — Layout thrashing detection
- `ImageDelivery` — Image optimization opportunities

---

## Workflow 4: Accessibility Audit (Lighthouse)

**Goal:** Verify WCAG 2.1 AA compliance and SEO best practices.

```
Step 1: navigate_page(type="url", url="http://localhost:5173/")
Step 2: wait_for(text=["Dashboard"])
Step 3: lighthouse_audit(
          device="desktop",
          mode="navigation",       ← reloads page for clean audit
          outputDirPath="evidence/lighthouse/"
        )
        → returns: scores for accessibility, SEO, best-practices
Step 4: [if testing post-interaction state]
        lighthouse_audit(
          device="desktop",
          mode="snapshot"          ← audits current state without reload
        )
Step 5: take_screenshot(filePath="evidence/lighthouse-state.png")
```

**Important:** `lighthouse_audit` does NOT include performance scores. Use `performance_start_trace` for Core Web Vitals.

**Accessibility target:** Score ≥ 90 (WCAG 2.1 AA) per `specs/38-UX-Design-Principles.md`.

---

## Workflow 5: SSE / Real-Time Verification

**Goal:** Verify SSE connections are established and delivering events.

```
Step 1: navigate_page(type="url", url="http://localhost:5173/")
Step 2: wait_for(text=["connected"])   ← wait for SSE connection indicator
Step 3: list_network_requests(resourceTypes=["eventsource", "fetch"])
        → find the SSE request (e.g., /api/v1/events/stream)
        → note its reqid
Step 4: get_network_request(reqid=<SSE_reqid>)
        → verify: status=200, response is streaming
Step 5: evaluate_script(function=() => {
          const el = document.querySelector('[data-testid="connection-status"]');
          return el?.getAttribute('data-status');
        })
        → should return "connected"
Step 6: take_screenshot(filePath="evidence/sse-connected.png")
Step 7: list_console_messages(types=["error"])
        → should be empty (no SSE errors)
```

**What to verify:**
- SSE endpoint appears in network requests with status 200
- Connection status indicator shows "connected"
- No console errors related to SSE
- Events are being received (check DOM for updated content)

---

## Workflow 6: Memory Leak Detection

**Goal:** Detect memory leaks in long-running UI sessions (SSE connections, complex flows).

```
Step 1: navigate_page(type="url", url="http://localhost:5173/")
Step 2: wait_for(text=["Dashboard"])
Step 3: evaluate_script(function=() => performance.memory?.usedJSHeapSize)
        → record baseline heap size (e.g., 15MB)
Step 4: take_memory_snapshot(filePath="evidence/heap-before.heapsnapshot")
Step 5: [simulate activity: navigate, interact, wait for SSE events — 5+ minutes]
Step 6: evaluate_script(function=() => performance.memory?.usedJSHeapSize)
        → record current heap size
Step 7: take_memory_snapshot(filePath="evidence/heap-after.heapsnapshot")
Step 8: [compare heap sizes — growth > 50MB over 30min is a leak signal]
```

**Memory budget:** < 50MB growth over 30 minutes for SSE connections (from `ui-performance-monitoring.md`).

**Note:** `.heapsnapshot` files can be opened in Chrome DevTools Memory panel for detailed analysis.

---

## Workflow 7: Multi-Tab / Isolated Context Testing

**Goal:** Test flows that require multiple tabs or isolated browser contexts.

```
Step 1: new_page(url="http://localhost:5173/")
        → opens tab 1 (main session)
Step 2: new_page(url="http://localhost:5173/", isolatedContext="session-2")
        → opens tab 2 in isolated context (separate cookies/storage)
Step 3: list_pages()
        → returns: [{pageId: 1, url: "..."}, {pageId: 2, url: "..."}]
Step 4: select_page(pageId=1)
        → switch to tab 1
Step 5: [interact with tab 1]
Step 6: select_page(pageId=2)
        → switch to tab 2
Step 7: [verify tab 2 state]
Step 8: close_page(pageId=2)
```

---

## Guardrails and Safety Rules

### Security
- **MUST NOT** open production URLs with real user data in the MCP browser.
- **MUST NOT** call `evaluate_script` on pages with sensitive data (credentials, PII).
- **SHOULD** use `--isolated=true` flag when testing to prevent cookie/state leakage.
- **SHOULD** use a separate Chrome profile for MCP-controlled automation.

### Evidence Integrity
- **MUST NOT** claim a UI step is verified without captured evidence (screenshot + console + network).
- **MUST NOT** fabricate evidence. If a tool call fails, record the failure.
- **MUST** save evidence to `.memory-bank/work-items/<ID>/runs/<RUN_ID>/` paths.

### Context Window
- **SHOULD** use `take_snapshot(verbose=false)` (default) to keep snapshot output compact.
- **SHOULD** use `list_console_messages(pageSize=50)` to limit output for large logs.
- **SHOULD** use `list_network_requests(pageSize=20)` to limit output for pages with many requests.
- **SHOULD NOT** call `take_snapshot` repeatedly without interacting — it produces redundant output.

### Interaction Safety
- **MUST** get UIDs from `take_snapshot` before any `click`, `fill`, `hover`, or `drag` call.
- **MUST** call `wait_for` after navigation before interacting.
- **MUST** call `handle_dialog` promptly if a browser dialog appears (it blocks further interaction).

---

## Integration with Axiom Evidence Bundle Schema

Evidence from Chrome DevTools MCP maps to the evidence bundle schema (`specs/27-Evidence-Bundle-Schema.md`):

```
.memory-bank/work-items/<WORK_ITEM_ID>/runs/<RUN_ID>/
  verification.md          ← include browser evidence section here
  ui-state.png             ← from take_screenshot
  dom-snapshot.txt         ← from take_snapshot
  heap-before.heapsnapshot ← from take_memory_snapshot (if memory testing)
  heap-after.heapsnapshot  ← from take_memory_snapshot (if memory testing)
  trace.json.gz            ← from performance_start_trace (if perf testing)
  lighthouse/              ← from lighthouse_audit (if accessibility testing)
```

**Trace line format:**
```
axiom:trace work_item=<ID> spec=specs/35-Web-UI-Dashboard.md evidence=runs/<RUN_ID>/ui-state.png
```

---

## Quick Reference: Common Mistakes

| Mistake | Correct Approach |
|---------|-----------------|
| `take_screenshot` to find elements | `take_snapshot` to get UIDs, then interact |
| Interact immediately after navigate | `wait_for(text=[...])` first |
| Skip console check | `list_console_messages(types=["error"])` after every major action |
| Use `lighthouse_audit` for performance | Use `performance_start_trace` for Core Web Vitals |
| Claim verified without evidence | Capture screenshot + console + network |
| Open production URLs | Only open localhost/dev/staging |
| Load this skill for non-browser agents | Check `mcp.chrome-devtools: true` in agent frontmatter first |

---

## Memory Bank References

- `.memory-bank/best-practices/chrome-devtools-mcp.md` — Full best practice guide (patterns, anti-patterns, Axiom callouts)
- `.memory-bank/best-practices/e2e-browser-testing-playwright.md` — Playwright E2E testing (CI regression suites)
- `.memory-bank/best-practices/ui-performance-monitoring.md` — Performance budgets and Lighthouse CI
- `.memory-bank/best-practices/ui-accessibility-testing-automation.md` — Accessibility testing patterns
- `.memory-bank/best-practices/testing-verification-evidence.md` — Evidence bundle requirements
- `.memory-bank/best-practices/opencode-server-integration.md` — OpenCode server integration (SSE patterns)

## Trace

`axiom:trace work_item=chrome-devtools-mcp-skill spec=specs/27-Evidence-Bundle-Schema.md plan=chrome-devtools-mcp/skill doc=.opencode/skills/chrome-devtools-mcp/SKILL.md`
