---
version: alpha
name: Chronicle
description: Palantir-grade dark-theme operational dashboard for AI-powered investigations — dense data, transparent reasoning, operator-first workflows.
colors:
  primary: "#58a6ff"
  # Background tier
  canvas: "#0d1117"
  surface: "#161b22"
  surface-raised: "#1c2128"
  surface-overlay: "#252c35"
  surface-sunken: "#0a0e14"
  input: "#0a0e14"
  code: "#0d1117"
  code-inline: "#1c2128"
  selection: "#1a3a6b"
  hover: "#1e2d45"
  disabled: "#1a1a1a"
  # Text tier
  text-primary: "#e6edf3"
  text-secondary: "#8b949e"
  text-tertiary: "#484f58"
  text-link: "#58a6ff"
  text-link-hover: "#79c0ff"
  text-code: "#7ee787"
  text-placeholder: "#3a4149"
  text-inverse: "#0d1117"
  # Accent tier
  accent-primary: "#388bfd"
  accent-primary-hover: "#58a6ff"
  accent-success: "#238636"
  accent-warning: "#d29922"
  accent-error: "#da3633"
  accent-purple: "#a371f7"
  accent-cyan: "#39d2c0"
  accent-pink: "#f778ba"
  # Border tier
  border-default: "rgba(48,54,61,0.6)"
  border-hover: "rgba(68,77,87,0.8)"
  border-focus: "#388bfd"
  border-error: "rgba(218,54,51,0.8)"
  # Entity colors
  entity-session: "#388bfd"
  entity-memory: "#39d2c0"
  entity-finding: "#238636"
  entity-task: "#d29922"
  entity-approval: "#da3633"
  entity-evidence: "#f778ba"
  entity-anomaly: "#ff7b72"
  entity-system: "#484f58"
  # Data viz (12-category palette)
  data-0: "#ff7b72"
  data-1: "#d29922"
  data-2: "#3fb950"
  data-3: "#39d2c0"
  data-4: "#58a6ff"
  data-5: "#a371f7"
  data-6: "#f778ba"
  data-7: "#ffa657"
  data-8: "#56d364"
  data-9: "#79c0ff"
  data-10: "#d2a8ff"
  data-11: "#ffb1af"
  # Status
  status-thinking: "#a371f7"
  status-tool-exec: "#d29922"
  status-idle: "#58a6ff"
  status-completed: "#238636"
  status-failed: "#da3633"
  status-paused: "#8b949e"
  # Trust levels
  trust-verified: "#238636"
  trust-high: "#388bfd"
  trust-medium: "#d29922"
  trust-low: "#ffa657"
  trust-quarantine: "#da3633"
  # Glass overlays (background + blur)
  glass-light: "rgba(22,27,34,0.92)"
  glass-medium: "rgba(22,27,34,0.85)"
  glass-heavy: "rgba(22,27,34,0.75)"
typography:
  display-1:
    fontFamily: Inter
    fontSize: 3rem
    fontWeight: 800
    lineHeight: 1.17
    letterSpacing: "-0.035em"
  display-2:
    fontFamily: Inter
    fontSize: 2.25rem
    fontWeight: 800
    lineHeight: 1.22
    letterSpacing: "-0.03em"
  heading-1:
    fontFamily: Inter
    fontSize: 1.875rem
    fontWeight: 700
    lineHeight: 1.2
    letterSpacing: "-0.025em"
  heading-2:
    fontFamily: Inter
    fontSize: 1.5rem
    fontWeight: 700
    lineHeight: 1.17
    letterSpacing: "-0.02em"
  heading-3:
    fontFamily: Inter
    fontSize: 1.25rem
    fontWeight: 600
    lineHeight: 1.2
    letterSpacing: "-0.015em"
  subtitle:
    fontFamily: Inter
    fontSize: 1.125rem
    fontWeight: 600
    lineHeight: 1.33
    letterSpacing: "-0.01em"
  body-lg:
    fontFamily: Inter
    fontSize: 1rem
    fontWeight: 400
    lineHeight: 1.5
    letterSpacing: "-0.005em"
  body:
    fontFamily: Inter
    fontSize: 0.875rem
    fontWeight: 400
    lineHeight: 1.57
  body-small:
    fontFamily: Inter
    fontSize: 0.8125rem
    fontWeight: 400
    lineHeight: 1.54
    letterSpacing: "0.005em"
  caption:
    fontFamily: Inter
    fontSize: 0.6875rem
    fontWeight: 400
    lineHeight: 1.45
    letterSpacing: "0.01em"
  micro:
    fontFamily: Inter
    fontSize: 0.625rem
    fontWeight: 400
    lineHeight: 1.6
    letterSpacing: "0.02em"
  mono-sm:
    fontFamily: "'JetBrains Mono'"
    fontSize: 0.75rem
    fontWeight: 400
    lineHeight: 1.67
  mono-base:
    fontFamily: "'JetBrains Mono'"
    fontSize: 0.8125rem
    fontWeight: 400
    lineHeight: 1.69
  mono-lg:
    fontFamily: "'JetBrains Mono'"
    fontSize: 0.875rem
    fontWeight: 400
    lineHeight: 1.71
  label-caps:
    fontFamily: Inter
    fontSize: 0.6875rem
    fontWeight: 600
    lineHeight: 1.45
    letterSpacing: "0.05em"
rounded:
  none: 0
  sm: 3px
  md: 6px
  lg: 10px
  xl: 16px
  full: 9999px
spacing:
  "0": 0
  "0_5": 2px
  "1": 4px
  "1_5": 6px
  "2": 8px
  "2_5": 10px
  "3": 12px
  "4": 16px
  "5": 20px
  "6": 24px
  "8": 32px
  "10": 40px
  "12": 48px
  "16": 64px
  "20": 80px
  sidebar: 240px
  sidebar-min: 56px
  topbar: 48px
  statusbar: 28px
components:
  button-primary:
    backgroundColor: "#1f6feb"
    textColor: "white"
    rounded: "{rounded.md}"
    padding: 8px
  button-primary-hover:
    backgroundColor: "#388bfd"
  button-ghost:
    backgroundColor: transparent
    textColor: "{colors.text-secondary}"
    rounded: "{rounded.md}"
    padding: 8px
  button-ghost-hover:
    backgroundColor: "{colors.hover}"
    textColor: "{colors.text-primary}"
  button-success:
    backgroundColor: "{colors.accent-success}"
    textColor: "white"
    rounded: "{rounded.md}"
  button-danger:
    backgroundColor: transparent
    textColor: "{colors.accent-error}"
    rounded: "{rounded.md}"
  button-danger-hover:
    backgroundColor: "{colors.accent-error}"
    textColor: "white"
  card:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.lg}"
    padding: 24px
  card-hover:
    backgroundColor: "{colors.hover}"
  thought-card:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.md}"
    padding: 16px
  thought-card-thinking:
    backgroundColor: "{colors.surface}"
    rounded: "{rounded.md}"
  thought-card-expanded:
    backgroundColor: "{colors.surface-raised}"
    rounded: "{rounded.md}"
  finding-card:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.md}"
    padding: 16px
  finding-card-approved:
    backgroundColor: "{colors.surface}"
    rounded: "{rounded.md}"
  finding-card-rejected:
    backgroundColor: "{colors.surface}"
    rounded: "{rounded.md}"
  input:
    backgroundColor: "{colors.input}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.md}"
    padding: 12px
  input-focus:
    backgroundColor: "{colors.input}"
    rounded: "{rounded.md}"
  badge:
    backgroundColor: "{colors.surface-raised}"
    textColor: "{colors.text-secondary}"
    rounded: "{rounded.full}"
    padding: 2px
  badge-active:
    backgroundColor: "#1f6feb"
    textColor: "white"
  badge-success:
    backgroundColor: "{colors.accent-success}"
    textColor: "white"
  badge-error:
    backgroundColor: "{colors.accent-error}"
    textColor: "white"
  toast:
    backgroundColor: "{colors.glass-medium}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.md}"
    padding: 16px
  sidebar-item:
    backgroundColor: transparent
    textColor: "{colors.text-secondary}"
    rounded: "{rounded.md}"
    padding: 8px
  sidebar-item-active:
    backgroundColor: "#1f6feb"
    textColor: "white"
  modal:
    backgroundColor: "{colors.glass-heavy}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.lg}"
    padding: 32px
  divider:
    backgroundColor: "{colors.border-default}"
    width: 1px
    height: 1px
  tooltip:
    backgroundColor: "{colors.surface-overlay}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.sm}"
    padding: 8px
  progress-bar:
    backgroundColor: "{colors.input}"
    rounded: "{rounded.full}"
    height: 4px
  progress-bar-fill:
    backgroundColor: "{colors.accent-primary}"
    rounded: "{rounded.full}"
    height: 4px
  kpi-card:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.lg}"
    padding: 20px
  table-row:
    backgroundColor: transparent
    textColor: "{colors.text-primary}"
  table-row-hover:
    backgroundColor: "{colors.hover}"
  table-row-selected:
    backgroundColor: "{colors.selection}"
  status-dot-thinking:
    backgroundColor: "{colors.status-thinking}"
    size: 8px
    rounded: "{rounded.full}"
  status-dot-completed:
    backgroundColor: "{colors.status-completed}"
    size: 8px
    rounded: "{rounded.full}"
  status-dot-failed:
    backgroundColor: "{colors.status-failed}"
    size: 8px
    rounded: "{rounded.full}"
---

## Overview

Chronicle is a Palantir Gotham-grade operational dashboard and investigation
workbench for the Consensus agent runtime. It is designed for security
analysts, investigative journalists, legal teams, and researchers — people who
cannot afford to be wrong, who need to see the AI's reasoning, and who must
defend their conclusions under scrutiny.

The visual identity draws from three lineages: **Palantir Gotham** (data
density, graph-centric analysis, operator workflows), **Linear** (keyboard
speed, command palette, zero-latency feel), and **Stripe** (micro-interactions,
glass-morphism depth, gradient overlays). The synthesis is a tool that feels
dangerous in skilled hands — an operator who has mastered Chronicle can move
faster than thought. A newcomer can orient within minutes through progressive
disclosure.

**Core principles:**
1. **Density with clarity.** Information dense but never cluttered. Every pixel
   serves a purpose.
2. **Motion as information.** Animations communicate causality, relationship,
   and state change — never decoration.
3. **Keyboard supremacy.** Every action has a keyboard shortcut. The mouse is
   secondary.
4. **Progressive depth.** Surface shows summary. One click drills to detail.
   Two clicks to raw data.
5. **Trust through transparency.** Never hide the AI's reasoning. The operator
   can always trace a conclusion to its origin.

The dark theme is the default and primary theme. All colors are perceptually
uniform, derived from OKLCH color space for consistent lightness across hues.

## Colors

The color system has five tiers: backgrounds (6 tokens), text (7 tokens),
accents (10 tokens), borders (4 tokens), and semantics (entity, status, trust,
data viz — 30+ tokens). All tokens specify sRGB hex values. The canonical
source uses OKLCH internally for perceptual uniformity; hex values are
fallbacks.

- **Canvas ({colors.canvas}):** Page background. Deep space black-blue, never
  pure black. The darkest surface in the system.
- **Surface ({colors.surface}):** Card, panel, table background. 2.5% lighter
  than canvas for subtle distinction.
- **Surface-raised ({colors.surface-raised}):** Elevated surfaces — hover
  states, expanded cards, modal backgrounds.
- **Input ({colors.input}):** Form fields and code blocks. Darker than surface
  to indicate the user can type here.
- **Text-primary ({colors.text-primary}):** Primary reading text. Nearly white
  but not pure — avoids harsh contrast on dark backgrounds.
- **Text-secondary ({colors.text-secondary}):** Metadata, descriptions, less
  critical information. Readable but muted.
- **Text-tertiary ({colors.text-tertiary}):** Timestamps, hints, disabled
  text. Low contrast by design — these are never the sole carrier of meaning.
- **Accent-primary ({colors.accent-primary}):** The primary interaction color.
  Buttons, links, focus rings, selected states. Used sparingly to preserve
  its signal.
- **Accent-success ({colors.accent-success}):** Approved findings, completed
  tasks, healthy status. Never used for non-success states.
- **Accent-warning ({colors.accent-warning}):** Medium confidence, budget
  thresholds, pending states. Amber — attention without alarm.
- **Accent-error ({colors.accent-error}):** Failures, rejections, critical
  alerts. Red — demands attention. Uses pulse animation for urgency.
- **Accent-purple ({colors.accent-purple}):** AI thinking/reasoning states.
  THINK pane accent. Model badges.
- **Accent-cyan ({colors.accent-cyan}):** Memory events, streaming indicators,
  data flow visualization.

**Entity colors** map each domain object to a consistent hue across all views:
- Session: blue. Memory event: cyan. Finding: green. Task: amber. Approval:
  red. Evidence source: pink. Anomaly: orange-red.

**Data viz palette:** 12-color categorical scale, perceptually uniform, ordered
for maximum distinguishability between adjacent categories.

**Glass overlays** create depth through background blur. Three intensities:
light (92% opacity, 8px blur — persistent panels), medium (85% opacity, 12px
blur — modals), heavy (75% opacity, 20px blur — command palette).

## Typography

Inter is the primary typeface for all UI text. JetBrains Mono for code,
terminal output, and structured data. Inter Display variant for headings above
36px (optical sizing).

The type scale has 15 steps from micro (10px) to display-1 (48px). Line
heights tighten as size increases — body text at 1.57, headings at 1.17-1.2.
Letter-spacing uses negative values on display sizes for tighter headlines, and
positive values on micro/caption for legibility.

**Tabular figures** (`font-variant-numeric: tabular-nums`) are mandatory on all
numeric data — KPIs, costs, iteration counts, timestamps. Digits must occupy
equal width for aligned columns.

**Dense mode** (power-user toggle) reduces font sizes by 0.0625rem, tightens
line-heights by 0.125, and switches to Inter Tight for headings.

## Layout

Spacing is a 4px baseline grid. Every margin, padding, and gap is a multiple
of 4px. No other spacing values are permitted.

The shell has four fixed regions: top bar (48px), sidebar (240px expanded /
56px collapsed), content area (fills remaining space), status bar (28px). The
sidebar uses a 250ms width transition with ease-out-expo easing.

Content max-widths: compact (960px for focused reading), default (1280px for
dashboard), wide (1600px for split panes and graphs), full (100% for
data-dense tables).

**Responsive breakpoints:** mobile (0px, single column), tablet (768px, sidebar
collapsed), desktop (1024px, full layout), wide (1440px, three-column
layouts), ultrawide (1920px, multiple panels visible simultaneously).

## Elevation & Depth

Shadows use multi-layer composition for realistic depth. Each level combines a
tight ambient shadow with a spread directional shadow. Eight levels: `xs`
(hairline border substitute) through `xl` (highest elevation modal) plus
semantic glows (blue for focus rings, green for success, red for error).

Z-index scale: base (0, content), sticky (100, headers), overlay (200,
dropdowns/tooltips), drawer (300, panels/command palette), modal (400,
dialogs), notification (500, toasts), turbo (600, drag preview).

Glass-morphism creates depth by blurring content behind overlays. Applied with
`backdrop-filter: blur()` at three intensity levels.

## Shapes

Border radius follows a 5-step scale: `none` (table edges, button groups),
`sm` (3px — badges, tags, inline code), `md` (6px — default for inputs,
buttons, cards), `lg` (10px — modals, panels), `xl` (16px — hero cards),
`full` (9999px — pills, avatars, status dots).

Interactive elements use `md` radius. Cards use `lg`. Pills and avatars use
`full`. Never mix radius values on the same element — choose one and apply
consistently.

## Components

- **button-primary** is the sole high-emphasis action. One per screen. Used
  for Approve, Create, Submit.
- **button-ghost** is the default low-emphasis action. Used for secondary
  actions, toolbar buttons, navigation. Transparent background, visible on
  hover.
- **button-success** is for positive confirmations (Approve, Complete, Accept).
- **button-danger** is for destructive actions (Delete, Cancel, Deny). Always
  requires confirmation for irreversible operations.
- **card** is the default surface for grouped content. Uses surface background
  with subtle border.
- **thought-card** displays AI reasoning steps in the THINK pane. Has six
  states: default (purple left border), thinking (animated gradient border +
  cursor blink), completed (neutral border), expanded (blue border, raised
  background), flagged (red border), linked (green border).
- **finding-card** displays AI conclusions in the SAYS pane. Has four states:
  draft (neutral), approved (green left border + checkmark badge), rejected
  (red left border, 60% opacity), outdated (dashed border).
- **input** is the text entry field. Darker than surface to indicate
  editability. On focus: blue border + subtle glow.
- **badge** is a small pill for status, counts, labels. Uses full border
  radius. Color-coded by semantic meaning.
- **toast** uses glass-morphism overlay for non-blocking notifications. Four
  variants: success (green border), info (blue), warning (amber), error (red).
- **modal** uses heavy glass overlay. Entry animation: scale 0.95→1 + fade,
  200ms ease-out-expo. Exit: reverse, 150ms.
- **kpi-card** displays a single metric with icon, value (display-2), trend
  indicator, and optional sparkline or progress bar. Click navigates to
  detail view.
- **table-row** alternates between transparent (default), hover (subtle blue
  tint), and selected (blue highlight) states. Click selects; double-click
  navigates to detail.

## Do's and Don'ts

- **Do** use token references (`{colors.accent-primary}`) instead of literal
  hex values in component definitions.
- **Do** apply animations for communication — state change, causality,
  relationship. Every animation has a job.
- **Do** provide keyboard shortcuts for every action. The mouse is secondary.
- **Do** show the AI's reasoning alongside its conclusions. Never hide
  provenance.
- **Do** respect `prefers-reduced-motion` — collapse all animation durations
  to 0ms when the user requests it.
- **Don't** introduce colors outside the palette. Extend the palette first if
  a genuinely new semantic meaning is needed.
- **Don't** use color as the sole differentiator. Always pair color with icon,
  text, or pattern.
- **Don't** nest component variants. `button-primary-hover` is a sibling entry,
  not a child of `button-primary`.
- **Don't** use `text-tertiary` for information the user must act on. It fails
  WCAG AA body text contrast (3.4:1) and is intentionally auxiliary-only.
- **Don't** animate decoration. Motion must communicate — never just look nice.
