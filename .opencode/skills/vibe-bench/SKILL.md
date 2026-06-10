---
name: vibe-bench
description: "Generate a complete, randomized SaaS application specification for vibe-coding benchmarks. Use when the user wants a random app idea, a vibe coding challenge, a model benchmark prompt, or says 'vibe bench', 'random app', 'benchmark app', or 'give me a project'. Produces full day-1 launch specs including architecture, API, frontend, database schema, and 1-20 scored bonus features."
version: "1.0.0"
---

# Vibe Bench — Random SaaS Application Spec Generator

## Purpose

You are a **startup CTO and product architect** who generates complete, production-ready specifications for hobby SaaS applications. Every spec you produce must be detailed enough that an AI coding agent can take it from zero to a working, deployable application with NO follow-up questions.

Each generated spec is also a **benchmark challenge** — it includes scored bonus features that test a model's creativity, technical depth, and ability to go the extra mile.

---

## How To Generate A Spec

When this skill is invoked, follow this entire process. Do NOT skip sections.

### Phase 1: Ideation (Use `<thinking>` tags)

```
<thinking>
Before I pick anything, let me brainstorm freely.

DOMAIN LOTTERY — I need to pick a random, interesting domain. Let me consider
unusual intersections of fields. Not just "todo app" or "chat app" — something
with personality. What if I combined two unrelated worlds? Like... marine biology
and personal finance? Or competitive cooking and project management? Or urban
gardening and social networking?

Let me land on something specific and opinionated.

Domain: [decide here]

TARGET USER — Who actually NEEDS this? What's their day like? What pain do
they currently solve with spreadsheets, sticky notes, or prayer?

User: [decide here]

CORE VALUE PROP — What is the ONE sentence a user would text their friend
about this app? If I can't make it fit in an excited text message, it's too vague.

Value: [decide here]

TECHNICAL SPICE — What makes this architecturally interesting? Does it need
real-time features? File processing? A recommendation engine? Geolocation?
Complex state machines? I should pick at least one thing that makes the
implementation non-trivial and interesting.

Spice: [decide here]
</thinking>
```

### Phase 2: Application Overview

Produce the following clearly labeled sections:

#### 2.1 — App Identity
- **App Name**: A catchy, brandable name (not generic)
- **Tagline**: One punchy sentence
- **Domain**: The industry/niche
- **Target User Persona**: 2-3 sentences describing who this is for
- **Elevator Pitch**: 3-4 sentences a founder would say to a VC in an elevator

#### 2.2 — Core Problem & Solution
- **The Pain**: What sucks right now without this app? Be specific and visceral.
- **The Solution**: How does this app fix it? Be concrete.
- **Why Now**: What trend, technology, or cultural shift makes this viable today?

---

### Phase 3: Technical Architecture

```
<thinking>
Let me think about the right technical choices for THIS specific app.

What data flows exist? What are the read/write patterns? Is this read-heavy
or write-heavy? Does it need real-time? What's the most complex state
transition in the system?

I should pick a stack that makes sense for the problem — not just default
to the same thing every time. The stack should be justified by the
requirements.

Let me also think about what the database schema looks like. What are the
core entities? How do they relate? Are there any tricky many-to-many
relationships or polymorphic associations?
</thinking>
```

#### 3.1 — Recommended Stack
Specify and JUSTIFY:
- **Frontend**: Framework, UI library, state management, key packages
- **Backend**: Language, framework, ORM, key packages
- **Database**: Type (SQL/NoSQL/both), specific engine, why
- **Auth**: Strategy (JWT, OAuth, magic link, etc.)
- **Hosting/Deploy**: Recommended platform
- **Key Integrations**: Any third-party APIs or services needed

#### 3.2 — Database Schema
Provide a complete schema with:
- All tables/collections
- All columns with types and constraints
- All relationships (foreign keys, indexes)
- Seed data suggestions
- Use a clear format (SQL CREATE TABLE or equivalent)

#### 3.3 — API Specification
Provide a complete REST (or GraphQL) API:
- Every endpoint with method, path, request body, response shape
- Authentication requirements per endpoint
- Rate limiting recommendations
- Error response format
- Group by resource/domain
- Minimum 10 endpoints, more if the app warrants it

Format each endpoint like:
```
VERB /api/resource
Auth: required | public
Body: { field: type, ... }
Response 200: { field: type, ... }
Response 4xx: { error: string, code: string }
Purpose: What this does and when the frontend calls it
```

---

### Phase 4: Frontend Specification

```
<thinking>
What pages does this app actually need? Let me walk through the user's
journey from first landing on the site to becoming a power user.

1. They land on the marketing/landing page
2. They sign up
3. They hit the dashboard — what do they see FIRST?
4. They do the core action — what does that flow look like step by step?
5. They configure settings
6. They come back the next day — what pulls them back?

Each of these moments needs a page or view. Let me map them out and think
about what components each page needs.

Also — what makes this app FEEL good? Micro-interactions? Transitions?
Smart defaults? I should call out specific UX details that elevate this
beyond a CRUD app.
</thinking>
```

#### 4.1 — Page Map
List every page/route with:
- Route path
- Page title
- Key components on the page
- User actions available
- Data requirements (what API calls on mount)

#### 4.2 — Component Hierarchy
For the 3 most complex pages, provide:
- Component tree (parent → children)
- Props each component accepts
- State each component manages
- Key interactions/events

#### 4.3 — UX Requirements
- Navigation pattern (sidebar, tabs, top nav, etc.)
- Responsive behavior (mobile-first? desktop-first? breakpoints?)
- Loading states and skeleton screens
- Empty states (first-time user with no data)
- Error states
- Toast/notification patterns
- At least 3 specific micro-interactions or UX polish details

---

### Phase 5: Business Logic & Rules

```
<thinking>
Every app has hidden complexity in its business rules. What are the
non-obvious rules for THIS app?

Things like:
- What happens at boundaries? (limits reached, plans maxed out, etc.)
- What are the state machines? (e.g., order: draft → submitted → processing → complete → archived)
- What validations exist beyond "field is required"?
- What calculations or derived data exist?
- Are there any time-based rules? (expiration, cooldowns, streaks, etc.)
- What are the permission levels and who can do what?

Let me write these out explicitly so the coding agent doesn't have to guess.
</thinking>
```

#### 5.1 — Business Rules
Document every non-obvious rule as a numbered list. Minimum 8 rules.

#### 5.2 — User Roles & Permissions
Define every role and what each can/cannot do. Use a matrix or clear list.

#### 5.3 — State Machines
For any entity with lifecycle states, provide:
- All states
- All valid transitions
- What triggers each transition
- Side effects of each transition (emails sent, records created, etc.)

---

### Phase 6: Bonus Features (The Benchmark Scoring System)

```
<thinking>
This is where the benchmark gets interesting. I need to generate bonus
features that are:

1. RELATED to the core app (not random gimmicks)
2. VARIED in difficulty (some quick wins, some meaty challenges)
3. TESTABLE (you can clearly tell if it was implemented)
4. CREATIVE (things that show the model went above and beyond)

I should think about features across these categories:
- Data visualization / charts / dashboards
- Real-time / live updates
- Smart defaults / AI-powered suggestions
- Social features / sharing / collaboration
- Gamification / streaks / achievements
- Import/export / integrations
- Accessibility / i18n
- Performance optimizations
- Easter eggs / delight features
- Advanced search / filtering

Let me pick a number between 1 and 20 and generate that many features,
each with a point value based on difficulty.

I should also think about which features COMBINE well — if an agent
implements features 3 and 7 together, do they create something greater
than the sum of their parts? I should note those synergies.
</thinking>
```

#### 6.1 — Bonus Feature Table

Generate between **1 and 20** bonus features. Randomly determine the count — don't always pick the same number. Present as a table:

| # | Feature Name | Description | Difficulty | Points | Category |
|---|-------------|-------------|------------|--------|----------|
| 1 | ... | Detailed description of what "done" looks like | Easy/Medium/Hard/Epic | 1-10 | Category |

**Point Scale:**
- **Easy (1-2 pts)**: Can be done in a few lines of code. Polish and UX touches.
- **Medium (3-5 pts)**: Requires meaningful implementation. New component or endpoint.
- **Hard (6-8 pts)**: Significant feature. Multiple components, complex logic, or integration.
- **Epic (9-10 pts)**: Major undertaking. Would impress in a demo. Shows deep technical skill.

#### 6.2 — Feature Synergy Bonuses
Identify 2-4 combinations of features that, when implemented together, earn bonus points:
- "Features X + Y together: +N bonus points because [reason]"

#### 6.3 — Scoring Guide
```
Total Possible Points: [sum]
----------------------------
Bronze:  30% of total  — App works, basics are solid
Silver:  50% of total  — Polished, several bonus features
Gold:    70% of total  — Impressive, well-integrated features
Platinum: 85%+ of total — Exceptional, demo-ready showcase
```

---

### Phase 7: Development Roadmap

#### 7.1 — Implementation Order
Provide a numbered, sequential build order:
1. What to build first (project setup, schema, auth)
2. What to build second (core CRUD)
3. What to build third (primary user flow)
4. ...and so on

Each step should include:
- What files/components to create
- Estimated complexity (low/medium/high)
- Dependencies on previous steps

#### 7.2 — Day-1 Launch Checklist
What must be true before this can be shown to a real user:
- [ ] Core flow works end-to-end
- [ ] Auth works (signup, login, logout)
- [ ] Data persists correctly
- [ ] Basic error handling exists
- [ ] Mobile-responsive
- [ ] Deployed to a public URL
- [ ] ... (add app-specific items)

---

### Phase 8: Final Spec Document

```
<thinking>
Let me review everything I've generated and make sure:

1. Is the spec internally consistent? Do the API endpoints match the
   database schema? Do the frontend pages call the right endpoints?
2. Are there any gaps? Could a coding agent get stuck anywhere because
   I left something ambiguous?
3. Is the scope realistic for a vibe-coding session? Not too small
   (boring), not too large (impossible)?
4. Are the bonus features genuinely interesting and varied?
5. Does this app have PERSONALITY? Would someone actually want to use it?

Let me fix any issues I notice before presenting the final spec.
</thinking>
```

Compile ALL of the above into a single, clean, well-formatted specification document. Use clear headers, consistent formatting, and include a table of contents at the top.

---

## Output Format

The final output must be a **single comprehensive markdown document** containing all phases above. It should:

1. Start with a quick **TL;DR** box summarizing the app in 5 lines
2. Include a **Table of Contents** with links to each section
3. Use consistent heading levels (H2 for phases, H3 for sections, H4 for subsections)
4. Include all `<thinking>` tags so the model's reasoning is visible
5. End with the **Scoring Guide** and **Launch Checklist**

## Quality Standards

The spec MUST be:
- **Complete**: No "TODO" or "TBD" sections. Everything filled in.
- **Specific**: Real table names, real endpoint paths, real component names. No hand-waving.
- **Consistent**: Schema matches API matches frontend. No contradictions.
- **Buildable**: An AI agent should be able to start coding immediately with zero clarification questions.
- **Interesting**: The app should be something you'd actually want to demo. Not another todo app.

## Randomization Guidelines

To ensure variety across generations:
- **Never** generate the same app concept twice in a conversation
- Pull from diverse domains: health, finance, education, creative tools, logistics, social, gaming, productivity, sustainability, food, travel, real estate, pets, music, sports, science, fashion, legal, agriculture, construction — and WEIRD INTERSECTIONS of these
- Vary the technical complexity: some specs should be frontend-heavy, some backend-heavy, some full-stack balanced
- Vary the bonus feature count: genuinely randomize between 1 and 20
- Vary the stack recommendations: don't always suggest the same framework

---

## Example Invocations

The user might say:
- "Give me a vibe bench"
- "Generate a random app benchmark"
- "I need a vibe coding challenge"
- "Random SaaS spec please"
- "Benchmark app"
- "Hit me with a project"
- "vibe-bench"

All of these should trigger this skill.

---

`axiom:trace work_item=SWDE-22 spec=specs/13-Command-Registry.md jira_ref=SWDE-22`
