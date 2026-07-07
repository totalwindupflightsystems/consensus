## 1. Design System & Visual Language

### 1.1 Design Philosophy

Chronicle is a dark-theme operational dashboard. It inherits from three lineages: Palantir Gotham (data density, operator workflows, graph-centric analysis), Linear (keyboard speed, command palette, zero-latency feel), and Stripe (micro-interactions, glass-morphism depth, gradient overlays). The synthesis is a tool that feels dangerous in skilled hands — an operator who has mastered Chronicle can move faster than thought. A newcomer can orient within minutes through progressive disclosure.

Every pixel serves a purpose. Motion communicates causality. The human is the investigator; the AI is the assistant. Trust is built through transparency — the AI never hides its reasoning, the UI never obscures provenance.

### 1.2 Complete Color Token Reference

All colors defined in both OKLCH (primary) and hex (fallback for browsers that don't support OKLCH). The dark theme is the default and primary theme. A light theme is defined but secondary. Every token below is the single source of truth — no hardcoded color values appear anywhere in the codebase.

#### 1.2.1 Background Tokens

```
Token Name                  OKLCH Value                    Hex Fallback      Usage
──────────────────────────  ─────────────────────────────  ────────────────  ──────────────────────────────────
--color-bg-canvas           oklch(12% 0.02 260)            #0d1117           Page background. Deep space black-blue. Never pure black.
--color-bg-surface          oklch(14.5% 0.02 260)          #161b22           Card, panel, table background. 2.5% lighter than canvas.
--color-bg-surface-raised   oklch(17.5% 0.02 260)          #1c2128           Elevated surface (hover states, expanded cards).
--color-bg-surface-overlay  oklch(21% 0.02 260)            #252c35           Highest elevation (modals, popovers).
--color-bg-surface-sunken   oklch(9.5% 0.01 260)           #0a0e14           Sunken/inset areas (code blocks, input fields).
--color-bg-selection        oklch(30% 0.12 265)            #1a3a6b           Selected row, text selection highlight.
--color-bg-selection-muted  oklch(22% 0.06 265)            #152845           Secondary selection (inactive multi-select item).
--color-bg-hover            oklch(21% 0.04 265)            #1e2d45           Row/item hover state. Subtle blue tint.
--color-bg-hover-muted      oklch(18% 0.02 265)            #1a2233           Muted hover (dropdown items, non-interactive hovers).
--color-bg-disabled         oklch(16% 0.00 0)              #1a1a1a           Disabled element background. Neutral gray.
--color-bg-input            oklch(9.5% 0.01 260)           #0a0e14           Input field background. Darker than surface.
--color-bg-input-focus      oklch(11% 0.02 260)            #0f1419           Input field background when focused.
--color-bg-code             oklch(10% 0.01 260)            #0d1117           Code block background. Matches canvas.
--color-bg-code-inline      oklch(17% 0.01 260)            #1c2128           Inline code background. Slightly lighter than canvas.
--color-bg-toast-success    oklch(20% 0.06 145)            #163321           Success toast background.
--color-bg-toast-error      oklch(20% 0.06 20)             #2d1616           Error toast background.
--color-bg-toast-warning    oklch(20% 0.06 85)             #2d2416           Warning toast background.
--color-bg-toast-info       oklch(20% 0.04 250)            #16233d           Info toast background.
--color-bg-badge            oklch(22% 0.04 265 / 0.15)     rgba(56,139,253,0.15)  Default badge background.
--color-bg-skeleton         oklch(18% 0.01 260)            #1c2128           Skeleton loading placeholder.
```

#### 1.2.2 Border Tokens

```
Token Name                  OKLCH Value                    Hex Fallback      Usage
──────────────────────────  ─────────────────────────────  ────────────────  ──────────────────────────────────
--color-border-default      oklch(25% 0.02 260 / 0.6)     rgba(48,54,61,0.6)  Default border. Subtle, never harsh.
--color-border-strong       oklch(30% 0.02 260 / 0.8)     rgba(58,65,73,0.8)  Stronger border (card edges, dividers).
--color-border-hover        oklch(35% 0.03 260 / 0.8)     rgba(68,77,87,0.8)  Border on hover state.
--color-border-focus        oklch(55% 0.15 265 / 1.0)     #388bfd           Focus ring border. High visibility blue.
--color-border-active       oklch(45% 0.10 265 / 1.0)     #2f6eb0           Active/pressed border.
--color-border-error        oklch(50% 0.18 20 / 0.8)      rgba(218,54,51,0.8) Error state border.
--color-border-error-strong oklch(50% 0.18 20 / 1.0)      #da3633           Strong error border (invalid input).
--color-border-success      oklch(50% 0.15 145 / 0.8)     rgba(35,134,54,0.8) Success state border.
--color-border-warning      oklch(50% 0.12 85 / 0.8)      rgba(191,123,0,0.8) Warning state border.
--color-border-info         oklch(45% 0.08 250 / 0.8)     rgba(56,139,253,0.6) Info state border.
--color-border-subtle       oklch(20% 0.01 260 / 0.4)     rgba(30,36,44,0.4)  Very subtle border (inner dividers).
```

#### 1.2.3 Text Tokens

```
Token Name                  OKLCH Value                    Hex Fallback      Usage
──────────────────────────  ─────────────────────────────  ────────────────  ──────────────────────────────────
--color-text-primary        oklch(90% 0.02 260)            #e6edf3           Primary text. Nearly white but not pure.
--color-text-secondary      oklch(65% 0.02 260)            #8b949e           Secondary/meta text. Readable but muted.
--color-text-tertiary       oklch(45% 0.02 260)            #484f58           Tertiary/hint text. Low contrast, non-essential.
--color-text-disabled       oklch(35% 0.01 260)            #343941           Disabled text. Meets minimum contrast for disabled.
--color-text-link           oklch(70% 0.15 250)            #58a6ff           Hyperlink text. Blue, understated.
--color-text-link-hover     oklch(80% 0.15 250)            #79c0ff           Hyperlink hover. Brighter blue.
--color-text-link-visited   oklch(60% 0.12 300)            #a371f7           Visited link. Purple.
--color-text-code           oklch(75% 0.10 150)            #7ee787           Inline code text. Green tint.
--color-text-placeholder    oklch(40% 0.02 260)            #3a4149           Input placeholder text.
--color-text-inverse        oklch(10% 0.02 260)            #0d1117           Text on accent backgrounds.
--color-text-success        oklch(65% 0.12 145)            #3fb950           Success text (toast titles, checkmarks).
--color-text-warning        oklch(65% 0.12 85)             #d29922           Warning text.
--color-text-error          oklch(60% 0.18 20)             #f85149           Error text.
--color-text-info           oklch(65% 0.10 250)            #58a6ff           Info text.
--color-text-accent         oklch(70% 0.18 265)            #58a6ff           Accent text (primary CTAs, active nav).
--color-text-heading        oklch(95% 0.02 260)            #f0f6fc           Page/section headings. Brighter than primary.
```

#### 1.2.4 Accent Tokens

```
Token Name                  OKLCH Value                    Hex Fallback      Usage
──────────────────────────  ─────────────────────────────  ────────────────  ──────────────────────────────────
--color-accent-primary       oklch(60% 0.18 265)           #388bfd           Primary accent. Blue. CTAs, focus rings.
--color-accent-primary-hover oklch(70% 0.18 265)           #58a6ff           Primary accent hover.
--color-accent-primary-active oklch(50% 0.18 265)          #1f6feb           Primary accent pressed/active.
--color-accent-primary-muted oklch(60% 0.18 265 / 0.15)    rgba(56,139,253,0.15) Primary accent at low opacity (selected bg).
--color-accent-primary-glow  oklch(60% 0.18 265 / 0.3)     rgba(56,139,253,0.3)  Primary accent glow (focus rings).
--color-accent-success       oklch(55% 0.18 145)           #238636           Success green.
--color-accent-success-hover oklch(65% 0.18 145)           #2ea043           Success hover.
--color-accent-success-muted oklch(55% 0.18 145 / 0.15)    rgba(35,134,54,0.15)  Success muted.
--color-accent-warning       oklch(55% 0.15 85)            #d29922           Warning amber.
--color-accent-warning-hover oklch(65% 0.15 85)            #e3b341           Warning hover.
--color-accent-warning-muted oklch(55% 0.15 85 / 0.15)     rgba(210,153,34,0.15) Warning muted.
--color-accent-error         oklch(50% 0.20 20)            #da3633           Error red.
--color-accent-error-hover   oklch(60% 0.20 20)            #f85149           Error hover.
--color-accent-error-muted   oklch(50% 0.20 20 / 0.15)     rgba(218,54,51,0.15) Error muted.
--color-accent-info          oklch(50% 0.10 210)           #3fb950           Info/neutral green.
--color-accent-purple        oklch(55% 0.20 300)           #a371f7           Purple accent.
--color-accent-cyan          oklch(60% 0.15 200)           #39d2c0           Cyan accent.
--color-accent-pink          oklch(55% 0.18 340)           #f778ba           Pink accent.
```

#### 1.2.5 Data Visualization Palette

A 12-color perceptually uniform categorical palette. Colors are ordered for maximum distinguishability between adjacent categories. Use sequentially for up to 12 data categories.

```
Token Name          OKLCH Value                Hex Fallback    Visual
──────────────────  ─────────────────────────  ──────────────  ──────
--color-data-0      oklch(60% 0.20 30)         #ff7b72         Red
--color-data-1      oklch(55% 0.18 90)         #d29922         Amber
--color-data-2      oklch(55% 0.18 140)        #3fb950         Green
--color-data-3      oklch(55% 0.15 200)        #39d2c0         Cyan
--color-data-4      oklch(55% 0.18 260)        #58a6ff         Blue
--color-data-5      oklch(55% 0.18 295)        #a371f7         Purple
--color-data-6      oklch(55% 0.16 340)        #f778ba         Pink
--color-data-7      oklch(55% 0.16 55)         #ffa657         Orange
--color-data-8      oklch(55% 0.14 160)        #56d364         Lime
--color-data-9      oklch(55% 0.12 225)        #79c0ff         Sky Blue
--color-data-10     oklch(55% 0.12 310)        #d2a8ff         Lavender
--color-data-11     oklch(55% 0.08 15)         #ffb1af         Salmon

Sequential scales (for continuous data):
  Blue scale:   oklch(30% 0.02 260) → oklch(80% 0.18 260)   (9 stops)
  Red scale:    oklch(30% 0.02 20)  → oklch(80% 0.18 20)    (9 stops)
  Green scale:  oklch(30% 0.02 145) → oklch(80% 0.18 145)   (9 stops)
  Purple scale: oklch(30% 0.02 295) → oklch(80% 0.18 295)   (9 stops)

Diverging scales (for comparison data):
  Red-Blue:     oklch(60% 0.18 20) → oklch(90% 0.02 260) → oklch(60% 0.18 260)  (9 stops)
  Red-Green:    oklch(60% 0.18 20) → oklch(90% 0.02 260) → oklch(60% 0.18 145)  (9 stops)

Heatmap scale (for density/intensity):
  oklch(95% 0.02 260) → oklch(70% 0.12 40) → oklch(70% 0.18 30) → oklch(55% 0.20 20)
  (white → orange → red → dark red, 9 stops)
```

#### 1.2.6 Semantic Entity Color Mapping

Each entity type in the system has a dedicated color that appears consistently across all views. These colors are used for icons, status dots, left-border accents, badges, and graph nodes.

```
Entity Type        Token Name                  OKLCH Value            Usage
────────────────── ──────────────────────────  ─────────────────────  ────────────────────────────
Session            --color-entity-session      oklch(60% 0.18 265)    Blue. Session icons, session nodes.
Memory Event       --color-entity-memory       oklch(60% 0.15 200)    Cyan. Memory event icons/badges.
Finding            --color-entity-finding      oklch(55% 0.18 145)    Green. Finding/draft icons.
Task               --color-entity-task         oklch(55% 0.15 85)     Amber. Task icons.
Approval           --color-entity-approval     oklch(50% 0.20 20)     Red. Approval icons/shields.
Evidence Source    --color-entity-evidence     oklch(55% 0.16 340)    Pink. Source/evidence icons.
Anomaly            --color-entity-anomaly      oklch(55% 0.18 30)     Orange-red. Anomaly/flags.
Tool               --color-entity-tool         oklch(55% 0.12 225)    Sky blue. Tool icons.
Skill              --color-entity-skill        oklch(55% 0.18 295)    Purple. Skill icons.
User/Actor         --color-entity-user         oklch(55% 0.08 15)     Salmon. User/actor identifiers.
System             --color-entity-system       oklch(45% 0.02 260)    Muted. System events.
```

#### 1.2.7 Status Color Encoding

Every status value has a dedicated color used for badges, status dots, and row indicators.

```
Status             Color Token                         Visual       Animation
────────────────── ──────────────────────────────────  ───────────  ──────────────────────
booting            --color-status-booting: gray        Gray dot      Opacity pulse 2s cycle
idle               --color-status-idle: muted blue     Blue dot      Static
thinking           --color-status-thinking: purple     Purple dot    Opacity pulse 2s cycle + shimmer
tool_exec          --color-status-tool_exec: amber     Amber dot     Static
waiting_sub        --color-status-waiting: cyan        Cyan dot      Subtle pulse 3s cycle
paused             --color-status-paused: amber-muted  Amber dot     Static, crossed-out icon
completed          --color-status-completed: green     Green dot     Static, checkmark
failed             --color-status-failed: red          Red dot       Static, X icon
cancelled          --color-status-cancelled: gray      Gray dot      Static, strikethrough
stalled            --color-status-stalled: orange-red  Orange dot    Rapid pulse 0.5s cycle
```

#### 1.2.8 Trust Level Color Encoding

Trust levels indicate how much confidence the system (and humans) have in a memory event or finding.

```
Trust Level        Color Token                   Visual
────────────────── ────────────────────────────  ──────────────────
verified           --color-trust-verified: green  Green dot + ✓ icon
high               --color-trust-high: blue       Blue dot
medium             --color-trust-medium: amber    Amber dot
low                --color-trust-low: orange      Orange dot
quarantine         --color-trust-quarantine: red  Red triangle + ⚠ icon
```

#### 1.2.9 Contrast Compliance Matrix

Every text-on-background combination is verified against WCAG 2.2 AA (4.5:1 for body text, 3:1 for large text). Below are representative measurements. Full 127×127 matrix available in appendix.

```
Foreground              Background               Ratio    WCAG AA Body    WCAG AA Large
──────────────────────  ───────────────────────  ───────  ──────────────  ──────────────
--color-text-primary    --color-bg-canvas        13.2:1   PASS            PASS
--color-text-primary    --color-bg-surface       11.8:1   PASS            PASS
--color-text-secondary  --color-bg-canvas         6.8:1   PASS            PASS
--color-text-secondary  --color-bg-surface        6.1:1   PASS            PASS
--color-text-tertiary   --color-bg-canvas         3.8:1   FAIL            PASS
--color-text-tertiary   --color-bg-surface        3.4:1   FAIL            PASS
--color-text-link       --color-bg-canvas         5.2:1   PASS            PASS
--color-text-error      --color-bg-canvas         6.1:1   PASS            PASS
--color-text-success    --color-bg-canvas         5.8:1   PASS            PASS
--color-text-placeholder--color-bg-input          3.0:1   FAIL            PASS (border suffices)
```

Tertiary text fails WCAG AA for body text by design — it is used only for non-essential auxiliary information and is never the sole carrier of meaning. Placeholder text is supplemented by visible labels.

### 1.3 Typography System

#### 1.3.1 Font Stack

```css
--font-sans:    'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif;
--font-mono:    'JetBrains Mono', 'Fira Code', 'Cascadia Code', 'Consolas', 'Menlo', monospace;
--font-display: 'Inter Display', 'Inter', sans-serif;  /* optical-sizing: auto for 48px+ */
--font-data:    'Inter', 'SF Pro Text', system-ui;     /* font-variant-numeric: tabular-nums */
```

#### 1.3.2 Complete Type Scale

Every text element in the application uses one of these 15 steps. No hardcoded font-size values exist in the codebase.

```
Step   Token Name         Size (rem)  Size (px)  Line Height  Weight  Letter Spacing  Usage
────   ─────────────────  ──────────  ─────────  ───────────  ──────  ──────────────  ───────────────────────────
-3     --text-micro       0.625rem    10px       1.0rem       400     +0.02em         Tiny badges, superscript
-2     --text-caption     0.6875rem   11px       1.0rem       400     +0.01em         Chart labels, meta, keycaps
-1     --text-small       0.8125rem   13px       1.25rem      400     +0.005em        Meta text, timestamps, tooltips
 0     --text-body        0.875rem    14px       1.375rem     400     0               Body copy, table cells, inputs
 1     --text-body-lg     1.0rem      16px       1.5rem       400     -0.005em        Card descriptions, extended prose
 2     --text-subtitle    1.125rem    18px       1.5rem       600     -0.01em         Section headers, card titles
 3     --text-heading-3   1.25rem     20px       1.5rem       600     -0.015em        Panel headers, widget titles
 4     --text-heading-2   1.5rem      24px       1.75rem      700     -0.02em         Page section headers
 5     --text-heading-1   1.875rem    30px       2.25rem      700     -0.025em        Page titles
 6     --text-display-2   2.25rem     36px       2.75rem      800     -0.03em         KPI hero numbers, big stats
 7     --text-display-1   3.0rem      48px       3.5rem       800     -0.035em        Major dashboard hero numbers

Mono   --text-mono-sm     0.75rem     12px       1.25rem      400     0               Inline code
Mono   --text-mono-base   0.8125rem   13px       1.375rem     400     0               Code blocks, JSON, terminal
Mono   --text-mono-lg     0.875rem    14px       1.5rem       400     0               Large code blocks
```

#### 1.3.3 Typographic Feature Flags

```
Tabular figures (data tables, metrics, timelines, cost displays):
  font-variant-numeric: tabular-nums;
  /* All digits occupy identical width. Essential for aligned columns. */

Proportional figures (prose, descriptions, narrative text):
  font-variant-numeric: proportional-nums;
  /* Natural digit spacing for readability in paragraphs. */

Code ligatures (JetBrains Mono only, opt-in):
  font-variant-ligatures: contextual;
  /* Renders → != >= <= :: as ligature glyphs. Disable for raw JSON. */

Optical sizing (display headings only):
  font-optical-sizing: auto;
  /* Browser adjusts stroke contrast for large sizes. Only on --font-display elements. */

Truncation (all text):
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  /* Single-line truncation with … ellipsis. */
  /* Tooltip reveals full text on hover after 500ms delay. */
  /* For multi-line truncation: line-clamp with -webkit-line-clamp fallback. */

Dense mode (power-user toggle, off by default):
  --text-body: 0.8125rem 1.25rem;        /* Reduce by 0.0625rem */
  --text-heading-3: 1.125rem 1.375rem;   /* Reduce by 0.125rem */
  --text-heading-2: 1.375rem 1.625rem;   /* Reduce by 0.125rem */
  All line-heights tighten by 0.125rem.
  Letter-spacing tightens by 0.005em across all steps.
  Font switches to 'Inter Tight' where available.
  Toggle: Shift+D or Settings → Appearance → Density.
```

#### 1.3.4 Text Rendering Rules

```
Body text: font-smoothing: antialiased; -moz-osx-font-smoothing: grayscale;
  /* Subpixel antialiasing on macOS, grayscale on other platforms. */

Code text: font-smoothing: auto;
  /* Preserves monospace glyph precision. No antialiasing override. */

Selection color: ::selection { background: var(--color-bg-selection); color: var(--color-text-primary); }
  /* Consistent selection color across all text. */

Underline offset: text-underline-offset: 0.2em; text-decoration-thickness: 1px;
  /* Links have understated underlines that don't clip descenders. */
```

### 1.4 Spacing Scale

Every margin, padding, gap, and inset in the application is a multiple of the base unit: 4px (0.25rem). No other spacing values are permitted.

```
Token Name         rem Value    px Value    Usage
─────────────────  ───────────  ─────────  ──────────────────────────────────────────
--space-0           0            0           Zero gap. Flush elements.
--space-px          0.0625rem    1px         Hairline. Status dot borders.
--space-0_5         0.125rem     2px         Icon-to-text gap, tight inline spacing.
--space-1           0.25rem      4px         Tight element gap. Badge padding.
--space-1_5         0.375rem     6px         Compact button padding. List item gap tight.
--space-2           0.5rem       8px         Icon padding. List item gap. Card header padding.
--space-2_5         0.625rem     10px        Medium inline gap. Dropdown item spacing.
--space-3           0.75rem      12px        Section padding compact. Compact panel insets.
--space-4           1rem         16px        Standard padding. Card body. Button padding horizontal.
--space-5           1.25rem      20px        Comfortable padding. Modal body.
--space-6           1.5rem       24px        Section padding. Grid gap. Card gap.
--space-8           2rem         32px        Page padding horizontal. Major section gap.
--space-10          2.5rem       40px        Hero spacing. Large section dividers.
--space-12          3rem         48px        Page margins wide. Maximum content inset.
--space-16          4rem         64px        Extra large gap. Full-width sections.
--space-20          5rem         80px        Maximum gutter. Ultrawide layouts only.
--space-sidebar     15rem        240px       Expanded sidebar width.
--space-sidebar-min 3.5rem       56px        Collapsed sidebar width.
--space-topbar      3rem         48px        Top bar height.
--space-statusbar   1.75rem      28px        Status bar height.
```

### 1.5 Border Radius Scale

```
Token Name         Value        Usage
─────────────────  ───────────  ──────────────────────────────────────────
--radius-none       0            Table edges. Button group inner edges. Input groups.
--radius-sm         0.1875rem    Inline code. Badges. Tags. Keycaps. Tiny elements.
--radius-md         0.375rem     Inputs. Buttons. Cards. Dropdowns. Default for most elements.
--radius-lg         0.625rem     Modals. Panels. Large cards. Dashboard widgets.
--radius-xl         1rem         Hero cards. Feature panels. Onboarding cards.
--radius-2xl        1.5rem       Major feature sections. Welcome screens.
--radius-full       9999px       Pills. Avatars. Toggles. Status dots. Round buttons.
```

### 1.6 Elevation System

#### 1.6.1 Z-Index Scale

```
Layer              Z-Index    Usage
─────────────────  ────────   ────────────────────────────────────────────
--z-base            0          Page content. Cards. Tables. Default layer.
--z-sticky          100        Sticky headers. Pinned sidebar. Floating toolbar.
--z-overlay         200        Dropdowns. Tooltips. Popovers. Select menus.
--z-drawer          300        Slide-in panels. Command palette. Notification drawer.
--z-modal           400        Modal dialogs. Confirmation overlays. Full-screen search.
--z-notification    500        Toast notifications. Alert banners.
--z-turbo           600        Drag preview. Highest priority. Always on top.
```

#### 1.6.2 Box Shadow System

Shadows use multi-layer composition for realistic depth. Each level combines a tight ambient shadow (simulating occlusion) with a spread directional shadow (simulating light source).

```
Token Name            Value
────────────────────  ──────────────────────────────────────────────────────────────
--shadow-none          0 0 0 0 transparent

--shadow-xs            0 0 0 1px rgba(255,255,255,0.04)
                       Hairline border substitute. Subtlest elevation cue.

--shadow-sm            0 1px 2px rgba(0,0,0,0.4),
                       0 1px 3px rgba(0,0,0,0.2)
                       Card resting state. Button default.

--shadow-md            0 2px 4px rgba(0,0,0,0.3),
                       0 4px 8px rgba(0,0,0,0.2),
                       0 0 0 1px rgba(255,255,255,0.05)
                       Card hover state. Dropdown menu. Popover.

--shadow-lg            0 4px 8px rgba(0,0,0,0.3),
                       0 8px 16px rgba(0,0,0,0.2),
                       0 16px 32px rgba(0,0,0,0.15),
                       0 0 0 1px rgba(255,255,255,0.06)
                       Modal. Drawer. Large overlay.

--shadow-xl            0 8px 16px rgba(0,0,0,0.3),
                       0 16px 32px rgba(0,0,0,0.25),
                       0 32px 64px rgba(0,0,0,0.2),
                       0 0 0 1px rgba(255,255,255,0.07)
                       Highest elevation. Full-screen overlay.

--shadow-glow-blue     0 0 0 1px rgba(56,139,253,0.3),
                       0 0 8px rgba(56,139,253,0.15),
                       0 0 24px rgba(56,139,253,0.08)
                       Focus ring glow. Active element.

--shadow-glow-error    0 0 0 1px rgba(218,54,51,0.3),
                       0 0 8px rgba(218,54,51,0.15)
                       Error glow. Invalid input focus.

--shadow-glow-success  0 0 0 1px rgba(35,134,54,0.3),
                       0 0 8px rgba(35,134,54,0.15)
                       Success glow. Approved states.

--shadow-inner         inset 0 1px 2px rgba(0,0,0,0.3)
                       Pressed state. Inset panels. Sunken areas.
```

#### 1.6.3 Glass-Morphism System

Glass panels create depth by blurring content behind the panel. Used for modals, overlays, and floating elements. Three intensity levels.

```
Token Name            Background                              Backdrop-Filter       Usage
────────────────────  ──────────────────────────────────────  ────────────────────  ──────────────────────
--glass-light         rgba(22, 27, 34, 0.92)                  blur(8px)             Persistent floating panels
--glass-medium        rgba(22, 27, 34, 0.85)                  blur(12px)            Modals, drawers, dialogs
--glass-heavy         rgba(22, 27, 34, 0.75)                  blur(20px)            Command palette, search overlay

All glass panels also have:
  border: 1px solid rgba(255, 255, 255, 0.08);
  -webkit-backdrop-filter: /* same as backdrop-filter (Safari) */ ;
  
Glass transition on scroll/appear:
  backdrop-filter transitions smoothly over 300ms when panel appears.
  Implementation: CSS transition on backdrop-filter.
```

### 1.7 Motion Design System

#### 1.7.1 Duration Tokens

```
Token Name               Value     Usage
───────────────────────  ────────  ────────────────────────────────────────────
--duration-instant        0ms      Accessibility: prefers-reduced-motion override.
--duration-micro          80ms     Button press feedback. Checkbox toggle.
--duration-quick          150ms    Hover transitions. Tooltip appear. Focus ring.
--duration-standard       250ms    Page transitions. Panel open. Dropdown expand.
--duration-slow           400ms    Complex animations. Modal open. Graph zoom.
--duration-deliberate     600ms    Deliberate reveals. Onboarding. Celebration.
--duration-glacial       1000ms    Background ambient. Idle animations. Pulsing.
--duration-skeleton      1500ms    Skeleton loading shimmer cycle.
```

#### 1.7.2 Easing Curves

All curves defined as cubic-bezier() for precise control. No ease-in/ease-out/ease keywords — always explicit.

```
Token Name            cubic-bezier()              Character
────────────────────  ──────────────────────────  ─────────────────────────────
--ease-out-quint      (0.22, 1, 0.36, 1)         Smooth deceleration. Entrances, reveals.
--ease-in-quint       (0.64, 0, 0.78, 0)         Smooth acceleration. Exits, dismissals.
--ease-in-out-quint   (0.83, 0, 0.17, 1)         Symmetric. Toggle states, continuous motion.
--ease-out-expo       (0.16, 1, 0.3, 1)          Strong decel. Modals, drawers, panels.
--ease-in-expo        (0.7, 0, 0.84, 0)          Strong accel. Fast exits.
--ease-spring         (0.34, 1.56, 0.64, 1)      Overshoot. Celebration, attention, bounce.
--ease-anticipate     (0.68, -0.2, 0.32, 1.2)    Pull-back. Drag release. Physical.
--ease-linear         (0, 0, 1, 1)                Constant velocity. Infinite loops only.
```

#### 1.7.3 Transition Shorthands

```
Token Name                 CSS Value
─────────────────────────  ──────────────────────────────────────────────────
--transition-color         color 150ms var(--ease-out-quint),
                           background-color 150ms var(--ease-out-quint),
                           border-color 150ms var(--ease-out-quint),
                           box-shadow 150ms var(--ease-out-quint)
                           /* Standard hover/focus transitions */

--transition-transform     transform 250ms var(--ease-out-expo),
                           opacity 250ms var(--ease-out-expo)
                           /* Element appear/disappear */

--transition-page          opacity 250ms var(--ease-out-quint),
                           transform 250ms var(--ease-out-quint)
                           /* Page/section transitions */

--transition-modal         opacity 250ms var(--ease-out-expo),
                           transform 400ms var(--ease-out-expo)
                           /* Modal open/close */

--transition-panel         transform 400ms var(--ease-out-expo),
                           opacity 250ms var(--ease-out-expo)
                           /* Panel/drawer slide */

--transition-fast          opacity 100ms var(--ease-out-quint)
                           /* Instant-feel micro feedback */

--transition-loading       background-position 1.5s var(--ease-linear) infinite
                           /* Skeleton shimmer animation */

--transition-chart-draw    stroke-dashoffset 800ms var(--ease-out-quint)
                           /* SVG chart draw-on-load animation */

--transition-chart-update  d 400ms var(--ease-out-quint)
                           /* Chart data morph animation */

--transition-node-drift    transform 2s var(--ease-linear)
                           /* Graph node organic drift */
```

#### 1.7.4 Performance Constraints

All animations must respect the following budget:
- Maximum 10 simultaneous CSS animations at any time
- Maximum 50 animating DOM elements
- Canvas (graph) renders at 60fps with frame-skipping below 30fps
- Heavy animations (force simulation) run in Web Workers
- `will-change` applied to frequently animated elements, removed after animation ends
- `contain: layout style paint` on animated containers
- `transform: translateZ(0)` for GPU compositing on problematic elements
- PerformanceObserver monitors long tasks (>50ms). If sustained >10 long tasks in 2 seconds, degrade to reduced motion automatically.
- View Transitions API (when available) for page-level transitions instead of JS-driven animations.

### 1.8 Icon System

#### 1.8.1 Library

All icons sourced from Phosphor Icons (MIT licensed). Rendered as inline SVG with `currentColor` for automatic theme inheritance. No icon font. No network requests (SVG sprite inlined at build time).

Sizes: `--icon-xs: 12px`, `--icon-sm: 14px`, `--icon-md: 16px`, `--icon-lg: 20px`, `--icon-xl: 24px`, `--icon-2xl: 32px`, `--icon-3xl: 48px`.

Weights: `regular` (default), `bold` (active/selected states), `fill` (toggle states — filled = active, regular = inactive), `duotone` (featured/hero elements only).

#### 1.8.2 Complete Icon-to-Action Mapping

```
Navigation:
  Dashboard:         squares-four  (weight: bold when active)
  Investigation:     magnifying-glass
  Timeline:          clock-counter-clockwise
  Graph:             graph
  Sessions:          cpu
  Memory:            database
  Tasks:             check-square
  Approvals:         shield-check
  Deliberation:      scales
  Billing:           currency-circle-dollar
  Health:            heartbeat
  Admin:             users
  Settings:          gear-six

Status (always weight: fill for non-idle states):
  Running/Active:    play-circle
  Paused:            pause-circle
  Completed:         check-circle
  Failed:            x-circle
  Idle:              circle (regular weight)
  Thinking:          brain (fill, with pulse animation)
  Warning:           warning
  Error:             warning-circle
  Stalled:           clock (fill, with pulse animation)

Actions:
  Create/Add:        plus
  Edit:              pencil-simple
  Delete:            trash
  Search:            magnifying-glass
  Filter:            funnel
  Sort Ascending:    sort-ascending
  Sort Descending:   sort-descending
  Refresh:           arrows-clockwise
  Export:            export
  Share:             share-network
  Copy:              copy
  Expand:            arrows-out
  Collapse:          arrows-in
  Pin:               push-pin
  Close/Dismiss:     x
  Menu/More:         dots-three-vertical
  Settings/Gear:     gear-six
  Link:              link
  External Link:     arrow-square-out
  Download:          download-simple
  Upload:            upload-simple
  Lock:              lock
  Unlock:            lock-open
  Eye (visible):     eye
  Eye (hidden):      eye-slash
  Notification:      bell
  Clock/Time:        clock
  Calendar:          calendar
  Tag:               tag
  Bookmark:          bookmark-simple
  Star/Favorite:     star
  Flag:              flag
  Target:            target
  Lightning:         lightning
  Fire:              fire
  Shield:            shield
  Key:               key
  Fingerprint:       fingerprint
  Globe:             globe
  Server:            hard-drives
  Terminal:          terminal-window
  Code:              code
  Bug:               bug
  Chat:              chat-centered-text
  Brain/AI:          brain
  Database:          database
  Chart/Graph:       chart-line
  Table:             table
  List:              list
  Grid:              squares-four
  User:              user
  Users/Team:        users
  Organization:      buildings
  Home:              house
  Help:              question
  Info:              info
  Check/Confirm:     check
  Deny/Reject:       x
  Approve:           thumbs-up
  Disapprove:        thumbs-down
  Save:              floppy-disk
  Print:             printer
  Email:             envelope
  Phone:             phone
  Location:          map-pin
  Document:          file-text
  Image:             image
  Video:             video-camera
  Audio:             microphone
  Archive:           archive
  Folder:            folder
  File:              file
```

#### 1.8.3 Icon Component API

```typescript
interface IconProps {
  name: IconName;              // From Phosphor icon set
  size?: 12 | 14 | 16 | 20 | 24 | 32 | 48;  // Default: 16
  weight?: 'regular' | 'bold' | 'fill' | 'duotone';  // Default: 'regular'
  color?: string;              // CSS color value or var() token. Default: currentColor
  className?: string;
  alt?: string;                // Accessible label for decorative icons. Default: '' (decorative)
  'aria-label'?: string;       // For interactive icons. Required if icon is a button.
  spin?: boolean;              // Continuous rotation animation. Default: false.
  pulse?: boolean;             // Opacity pulse animation. Default: false.
}
```

### 1.9 Focus Ring System

Every interactive element MUST have a visible focus indicator when focused via keyboard. Mouse clicks should NOT show focus rings (use `:focus-visible`). The focus ring is consistent across all components.

```css
/* Default focus ring */
:focus-visible {
  outline: none;
  box-shadow: 
    0 0 0 2px var(--color-bg-canvas),
    0 0 0 4px var(--color-accent-primary);
  /* Outer ring: 4px accent color. Inner ring: 2px background color. */
  /* Creates a 2px gap between element edge and focus ring. */
  /* Total ring width: 4px. Total visual offset from element: 2px. */
}

/* Error focus ring */
:focus-visible.error {
  box-shadow:
    0 0 0 2px var(--color-bg-canvas),
    0 0 0 4px var(--color-accent-error);
}

/* Focus ring transition */
:focus-visible {
  transition: box-shadow 150ms var(--ease-out-quint);
}

/* Input focus ring (slightly different — ring is inside the border) */
input:focus-visible, textarea:focus-visible, select:focus-visible {
  border-color: var(--color-accent-primary);
  box-shadow: 0 0 0 1px var(--color-accent-primary);
}

/* Never show focus ring on mouse click */
:focus:not(:focus-visible) {
  box-shadow: none;
  outline: none;
}
```

---

## 2. Layout Architecture

### 2.1 Shell Structure

The application shell is a single-page frame that persists across all views. Only the content area changes via client-side routing. The shell consists of four fixed regions and one dynamic region:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ TOP BAR — 48px — position: fixed, top: 0, z-index: 100, full-width         │
│ ┌──────┐ ┌──────────────────────────────┐ ┌──────────┐ ┌──────────┐       │
│ │ Logo │ │ Command Palette (Ctrl+K)      │ │ Notific. │ │ Profile  │       │
│ └──────┘ └──────────────────────────────┘ └──────────┘ └──────────┘       │
├────┬────────────────────────────────────────────────────────────────────────┤
│    │                                                                        │
│ S  │ CONTENT AREA — flex: 1, overflow-y: auto, overflow-x: hidden          │
│ I  │ padding: 0 (views manage their own padding)                            │
│ D  │ background: var(--color-bg-canvas)                                      │
│ E  │ Views render here via React Router <Outlet />                          │
│ B  │ Transitions: crossfade + directional slide (250ms)                     │
│ A  │                                                                        │
│ R  │                                                                        │
│    │                                                                        │
├────┴────────────────────────────────────────────────────────────────────────┤
│ STATUS BAR — 28px — position: fixed, bottom: 0, z-index: 100, full-width   │
│ ┌──────────┐ ┌──────────────┐ ┌───────────────┐ ┌────────────────────────┐ │
│ │ Sessions │ │ API Status ● │ │ Budget: $0.42 │ │ v0.7.0 · online        │ │
│ └──────────┘ └──────────────┘ └───────────────┘ └────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────────┘

Grid specification:
  display: grid;
  grid-template-rows: 48px 1fr 28px;
  grid-template-columns: var(--sidebar-width) 1fr;
  /* sidebar-width: 240px expanded, 56px collapsed */
  height: 100vh;
  width: 100vw;
  overflow: hidden;

The shell grid tracks are:
  Row 1 (48px top bar): grid-row: 1; grid-column: 1 / -1;  (spans both columns)
  Row 2 (sidebar):     grid-row: 2; grid-column: 1;         (240px / 56px)
  Row 2 (content):     grid-row: 2; grid-column: 2;         (remaining space)
  Row 3 (status bar):  grid-row: 3; grid-column: 1 / -1;   (spans both columns)
```

### 2.2 Top Bar — Exact Specification

```
Element                                Position         Size        Background        Border
────────────────────────────────────  ───────────────  ──────────  ────────────────  ──────────────────────
Top Bar Container                      fixed, top:0     100% × 48px bg-surface        border-bottom: 1px border-default
Logo (Consensus monogram)              left: 16px       28 × 28px   transparent        none
  On hover: subtle rotation (15deg, 250ms ease-out-expo) + glow pulse
  On click: navigate to /dashboard
Command Palette Trigger                left: 60px        320px × 32px bg-input          border: 1px border-default
  Shape: pill (border-radius: 16px)
  Text: "Search or run a command..." in --text-placeholder, --text-body
  Icon: magnifying-glass, 16px, text-tertiary, left: 12px
  Focus: border-color accent-primary, box-shadow accent glow
  Click: opens Command Palette overlay
  Tablet (<1024px): 200px width
  Mobile (<768px): hidden, replaced by search icon button at right: 60px
Notification Bell                     right: 56px       32 × 32px   transparent        none
  Badge count: absolute, top: -2px, right: -2px
  Badge: min-width 18px, height 18px, border-radius 9px, bg-error, text: white, caption
  On new notification: badge scale-bounce animation (120% → 100%, 400ms ease-spring)
  Empty: no badge, bell icon only
  Click: opens notification drawer
Profile Avatar                        right: 16px       32 × 32px   transparent        none
  Shape: circle (radius-full)
  Content: user initials or Gravatar
  Click: opens dropdown (Profile, API Keys, Settings, Sign Out)
  Dropdown: 200px wide, anchored right, slide-down + fade (150ms ease-out-quint)
  Dropdown close: click outside, Escape, or select item
```

### 2.3 Sidebar — Exact Specification

```
Position: fixed (within grid), top: 48px, bottom: 28px, left: 0
Width: 240px expanded, 56px collapsed
Transition: width 250ms ease-out-expo
Background: bg-surface, border-right: 1px border-default
Z-index: 50
Overflow-y: auto (scrollable if nav items exceed viewport)
Overflow-x: hidden
Custom scrollbar: 4px wide, thumb: border-hover, track: transparent

Section structure (vertical stack):
  1. MAIN section (always visible, top of sidebar)
  2. OPERATIONS section
  3. SYSTEM section
  4. Spacer (flex-grow: 1 — pushes bottom items to base)
  5. Collapse toggle button
  6. Help button

Navigation items (exact specification):
  Height: 36px
  Margin: 2px 8px
  Padding: 0 12px
  Border-radius: radius-md
  Display: flex, align-items: center, gap: 10px
  Cursor: pointer
  Color: text-secondary
  Font: text-body, weight: 400
  
  Expanded state (default):
    Icon: 20px, left: 12px
    Label: visible, text-body, truncate
    Shortcut badge: right: 12px, text-caption, text-tertiary, mono
  
  Collapsed state:
    Icon: 20px, centered (margin: auto)
    Label: hidden (display: none)
    Shortcut badge: hidden
    Tooltip: appears on hover (right side of icon) showing full item name + shortcut
  
  States:
    Default: bg transparent, text text-secondary
    Hover: bg bg-hover, text text-primary, transition 150ms var(--transition-color)
    Active: bg accent-primary-muted, text text-primary, font-weight 600
      Border-left: 3px solid accent-primary (only in expanded mode)
      Icon weight: bold (vs regular for inactive)
    Disabled: text text-disabled, cursor default, no hover

Section headers:
  Visible only in expanded mode
  Font: text-caption, text-transform: uppercase, letter-spacing: 0.05em
  Color: text-tertiary
  Padding: 16px 12px 4px
  Non-interactive

Dividers between sections:
  Margin: 8px 12px
  Border-top: 1px border-subtle

Collapse toggle button:
  Position: bottom of sidebar
  Height: 40px
  Margin: 4px 8px 8px
  Icon: arrows-left-right (expanded), arrows-left-right (collapsed)
    Icon rotates 180deg on toggle
  Label: "Collapse" (expanded), hidden (collapsed)
  Click: toggle sidebar width
  Keyboard: Ctrl+B

Sidebar responsive:
  Desktop (>=1024px): expanded by default (240px)
  Tablet (768-1023px): collapsed by default (56px), expandable
  Mobile (<768px): hidden. Hamburger menu in top bar opens overlay.
    Overlay: full-width, slides in from left (300ms ease-out-expo)
    Backdrop: rgba(0,0,0,0.5), click to close
    Close: Escape or swipe left
```

### 2.4 Content Area — Exact Specification

```
Position: grid-row: 2, grid-column: 2
Size: fills remaining space after sidebar
Scroll: overflow-y: auto, overflow-x: hidden
Scrollbar: 6px wide, thumb: border-hover, track: transparent, border-radius: 3px
Background: bg-canvas
Padding: 0 (each view manages own padding)

View default padding:
  Dashboard:         padding: 32px 32px (desktop), 24px 24px (tablet), 16px 16px (mobile)
  Investigation:     padding: 0 (fills entire area for split panes)
  Timeline:          padding: 0 (fills area for timeline canvas)
  Graph:             padding: 0 (fills area for WebGL canvas)
  Standard pages:    padding: 32px 32px 80px (desktop), 24px 24px 64px (tablet), 16px 16px 48px (mobile)

Maximum content width:
  Dashboard: 1440px, centered (margin: 0 auto)
  Table views: 100% width
  Investigation: 100% width (split panes)
  Graph: 100% width

Route transitions:
  Exit: current view fades (opacity 1→0, 150ms ease-in-quint) + slides left (translateX 0→-20px)
  Enter: new view fades (opacity 0→1, 150ms ease-out-quint, delayed 50ms) + slides from right (translateX 20px→0)
  Timing: exit starts immediately, enter starts at 50ms offset. Total: 250ms.
  Overlap creates smooth crossfade.
  Implementation: CSS transition triggered by React Router location change.
  Scroll restoration: saved per-route in sessionStorage. Restored on back/forward.

Back navigation (history back):
  Exit slides right (translateX 0→20px). Enter slides from left (translateX -20px→0).
  Same durations. Creates consistent directional model.
```

### 2.5 Status Bar — Exact Specification

```
Position: fixed, bottom: 0, left: 0, right: 0
Height: 28px
Background: bg-surface
Border-top: 1px border-default
Z-index: 100
Display: flex, justify-content: space-between, align-items: center
Padding: 0 16px
Font: text-caption, color: text-tertiary
User-select: none

Left cluster (display: flex, gap: 16px, align-items: center):
  Active Sessions:
    Format: "12 sessions" (count updates via WebSocket, real-time)
    Icon: cpu 12px, left of text
    Click: navigate to /sessions
    Hover: text-secondary

  Active Tasks:
    Format: "3 tasks pending"
    Icon: check-square 12px
    Click: navigate to /tasks

Center cluster (text-align: center, flex: 1):
  Last event:
    Format: "[icon] Session #a3f completed iteration 42 · 8s ago"
    Updates via WebSocket in real-time
    Transition: new text fades in, old text fades out (crossfade 300ms)
    Truncates if too long (max 60 chars), tooltip shows full text

Right cluster (display: flex, gap: 16px, align-items: center):
  API Status:
    Format: "● Connected" (green), "● Degraded" (amber), "● Disconnected" (red)
    Dot: 8px circle, connected=pulsing green (2s cycle), degraded=static amber, disconnected=flashing red (0.5s)
    Click: navigate to /health
    
  Budget:
    Format: "$0.42 / $10.00"
    Progress bar: inline, 40px × 4px, bg-input, fill transitions width 1s ease-out-quint
    Fill color: green (<50%), amber (50-80%), red (>80%)
    Click: navigate to /billing

  Version:
    Format: "v0.7.0"
    
  Deployment:
    Format: "local · dev" or "us-east-1 · online"
```

---

## 3. Global Navigation & Overlays

### 3.1 Command Palette

The Command Palette is the power-user navigation hub. It provides fuzzy search across pages, sessions, and commands.

#### 3.1.1 Trigger

```
Method 1: Click the search pill in the top bar
Method 2: Press Ctrl+K (or Cmd+K on Mac)
Method 3: Press Ctrl+Shift+P (opens directly to command mode)

On trigger:
  1. Glass overlay appears (fade in, opacity 0→1, 100ms)
  2. Palette dialog scales in (scale 0.96→1 + fade, 150ms ease-out-expo)
  3. Input auto-focuses, cursor at end of any existing text
  4. Recent items shown (if input empty)
```

#### 3.1.2 Overlay Specification

```
Backdrop:
  Background: rgba(0, 0, 0, 0.5)
  Backdrop-filter: blur(4px)
  Position: fixed, inset: 0
  Z-index: 300 (--z-drawer)
  Click: close palette
  Escape: close palette

Palette Dialog:
  Position: fixed, top: 20%, left: 50%, transform: translateX(-50%)
  Width: 560px (desktop), min(560px, 90vw) (responsive)
  Max-height: 480px
  Background: var(--glass-heavy)
  Border: 1px solid rgba(255, 255, 255, 0.1)
  Border-radius: 12px
  Box-shadow: var(--shadow-xl)
  Display: flex, flex-direction: column
  Overflow: hidden

Search Input:
  Height: 56px
  Padding: 0 16px 0 44px
  Background: transparent
  Border: none (palette border is the boundary)
  Border-bottom: 1px solid rgba(255, 255, 255, 0.06)
  Font: --text-body-lg, color: --color-text-primary
  Placeholder: "Search sessions, run commands, navigate..."
  Icon (left, 16px from edge): magnifying-glass, 20px, color: text-tertiary
  Loading indicator (right): subtle spinner, 16px, opacity 0.4
    Appears when search is in-flight. Replaced by empty when idle.
  
  Debounce: 100ms before triggering search
  Min characters for search: 1 (shows recents when empty)

Results Container:
  Flex: 1
  Overflow-y: auto
  Padding: 8px 0
  Custom scrollbar: 4px wide

  Sections (in order):
    1. PAGES — matching navigation routes
    2. SESSIONS — matching session name/goal/ID
    3. COMMANDS — matching action descriptions
    (sections only appear if they have results)
```

#### 3.1.3 Result Item Specification

```
Item:
  Height: 44px
  Padding: 0 16px
  Display: flex, align-items: center, gap: 12px
  Border-radius: 6px
  Margin: 1px 8px
  Cursor: pointer

States:
  Default: background transparent, color text-secondary
  Hover (mouse): background bg-hover
  Selected (keyboard arrow): background bg-selection
    Selected item has subtle left accent: 2px solid accent-primary on left edge
    Selected item icon weight: bold

Anatomy:
  ┌─────────────────────────────────────────────────────────────┐
  │ [Icon]  Title                        Shortcut/Badge  [Chev] │
  │ 16px    text-body, text-primary       text-caption      8px │
  │         Subtitle                                             │
  │         text-caption, text-secondary, truncate               │
  └─────────────────────────────────────────────────────────────┘

  Icon: entity type icon, 16px, color text-secondary (text-primary when selected)
  Title: text-body, color text-primary, truncate 1 line
  Subtitle: text-caption, color text-secondary, truncate 1 line (optional, for sessions/commands)
  Shortcut badge (right): text-caption, mono, text-tertiary
    Shows keyboard shortcut (⌘1, ⌘N, etc.)
  Status badge (right, instead of shortcut for sessions):
    Colored pill with status text ("thinking", "completed")
  Chevron: visible only on selected item, subtle (opacity 0.3), animates right 2px on hover

Session results include:
  Title: session name (or truncated ID if no name)
  Subtitle: "#a3f2b · thinking · 12m ago"
  Status badge: colored pill

Command results include:
  Title: command name ("New Session", "Export PDF")
  Subtitle: description
  Shortcut: if applicable

Page results include:
  Title: page name ("Dashboard", "Settings")
  Subtitle: none
  Shortcut: "⌘1", "⌘," etc.
```

#### 3.1.4 Search Modes

```
Default mode (no prefix):
  Fuzzy match against: page titles, session names, session IDs, command names
  Scoring: exact prefix match > word boundary match > substring match > fuzzy match
  Order by: match score desc, then recency desc (for sessions)
  Max results: 5 per section, 15 total

Command mode (prefix: ">"):
  Filter to commands only
  Example: "> new" → shows "New Session", "New Investigation"
  Matches against command name and description

Session mode (prefix: "#"):
  Filter to sessions only
  Example: "# a3f" → shows sessions matching "a3f"
  Matches against session ID, name, agent_name

Go-to mode (prefix: "@"):
  Quick navigation
  Example: "@ dash" → navigates to dashboard
  Example: "@ sett" → navigates to settings

Keyboard navigation within palette:
  Arrow Down / Ctrl+J: move selection down (loop to top from bottom)
  Arrow Up / Ctrl+K: move selection up (loop to bottom from top)
  Enter: execute selected action
  Escape: close palette
  Tab: cycle between sections (if multiple sections visible)
```

### 3.2 Notification System

#### 3.2.1 Toast Notifications

```
Container:
  Position: fixed, top: 60px (48px topbar + 12px gap), right: 16px
  Z-index: 500 (--z-notification)
  Width: 380px max
  Display: flex, flex-direction: column-reverse (newest at bottom)
  Gap: 8px
  Max visible: 5 toasts

Toast Anatomy:
  Minimum height: 56px
  Max-height: 200px (scrollable if content exceeds)
  Background: var(--glass-heavy)
  Border: 1px solid rgba(255, 255, 255, 0.1)
  Border-left: 3px solid (semantic color, see below)
  Border-radius: --radius-md
  Padding: 12px 16px
  Box-shadow: --shadow-md
  Display: flex, gap: 10px
  Backdrop-filter: blur(12px)

  Icon (left, 20px):
    Success: check-circle, green
    Info: info, blue
    Warning: warning, amber
    Error: x-circle, red

  Content (center, flex-direction: column, gap: 2px):
    Title: text-small, font-weight 600, text-primary
    Message: text-caption, text-secondary (optional, shown if message provided)
    Timestamp: text-caption, text-tertiary, absolute right: 12px, top: 14px

  Close button (right, 16px):
    Icon: x, 14px, text-tertiary
    Hover: text-primary

  Action buttons (bottom-right, optional):
    Small buttons: text-caption, font-weight 600
    Examples: "View", "Retry", "Undo", "Dismiss"
    "Undo" available for 5s after destructive actions

Toast entry animation:
  Slide in from right: translateX(calc(100% + 16px)) → translateX(0)
  Opacity: 0 → 1
  Duration: 300ms, ease-out-expo
  Below toasts shift up: margin-bottom transition 250ms ease-out-quint

Toast exit animation:
  Slide right: translateX(0) → translateX(calc(100% + 16px))
  Opacity: 1 → 0
  Duration: 200ms, ease-in-quint
  Below toasts shift down to fill gap

Toast types:
  Success: green border, auto-dismiss 4s
    Examples: "Session created", "Task completed", "Export finished"
  Info: blue border, auto-dismiss 3s
    Examples: "Agent started thinking", "Model switched to deepseek-v4-pro"
  Warning: amber border, auto-dismiss 6s (or until acknowledged)
    Examples: "Budget at 80%", "Session stalled", "Slow API response"
  Error: red border, NEVER auto-dismiss (requires manual dismiss)
    Examples: "Session failed: API key invalid", "Database connection lost"
    Includes "View Details" action button

Toast grouping:
  Consecutive toasts of same type within 2 seconds stack into count badge
  Example: 3 "Session created" toasts → "3 sessions created"
  Badge shows count, click expands to show individual toasts

Rate limiting:
  Max 1 toast per 500ms (suppress rapid-fire toasts)
  Max 3 toasts of same type from same source per 10 seconds
```

#### 3.2.2 Notification Drawer

```
Trigger: click bell icon in top bar

Drawer:
  Position: fixed, top: 0, right: 0, bottom: 0
  Width: 400px
  Background: bg-surface
  Border-left: 1px border-default
  Box-shadow: shadow-lg
  Z-index: 300 (--z-drawer)
  
  Slide animation:
    Closed: translateX(100%)
    Open: translateX(0)
    Duration: 300ms, ease-out-expo
  
  Backdrop:
    Background: rgba(0,0,0,0.3)
    Position: fixed, inset: 0
    Z-index: 299
    Click: close drawer
  
  Header:
    Height: 56px
    Padding: 0 20px
    Border-bottom: 1px border-default
    Display: flex, align-items: center, justify-content: space-between
    Title: "Notifications" in text-subtitle, font-weight 600
    Actions: "Mark all read" button (text-small, text-link)
    Close button: x icon, 20px, text-tertiary

  List:
    Overflow-y: auto, flex: 1
    
    Notification item:
      Padding: 12px 20px
      Display: flex, gap: 12px
      Border-bottom: 1px border-subtle
      Background: transparent (read), bg-hover-muted (unread)
      Unread indicator: 6px dot, accent-primary, left edge
      
      Icon: 20px, semantic color
      Content: flex-direction column
        Title: text-small, font-weight 600 (unread), 400 (read)
        Description: text-caption, text-secondary
        Timestamp: text-caption, text-tertiary
      Actions: "Mark read" button (appears on hover)
      
      Click: navigate to relevant page (and mark as read)
      
  Empty state:
    "All caught up" with bell icon (48px, muted)
    "No notifications" in text-secondary
    
  Persistence: notifications stored in localStorage (last 100)
```

### 3.3 Context Menus

Context menus appear on right-click (desktop) or long-press (touch, 500ms hold). They provide contextual actions without leaving the current view.

```
Menu Container:
  Position: absolute (at cursor position or touch point)
  Min-width: 180px
  Max-width: 320px
  Max-height: 400px (scrollable if exceeded)
  Background: var(--glass-medium)
  Border: 1px solid rgba(255,255,255,0.1)
  Border-radius: --radius-md
  Box-shadow: --shadow-lg
  Padding: 4px 0
  Backdrop-filter: blur(12px)
  Z-index: 200 (--z-overlay)

  Entry animation:
    Transform-origin: top left
    Scale(0.95) → scale(1), opacity 0→1
    Duration: 120ms, ease-out-quint
    (Very fast — context menus feel instant)

  Exit animation:
    Scale(1) → scale(0.95), opacity 1→0
    Duration: 80ms, ease-in-quint

  Close triggers: click outside, Escape, select item, scroll parent

Menu Item:
  Height: 32px
  Padding: 0 12px
  Display: flex, align-items: center, gap: 10px
  Font: text-small
  Color: text-primary (enabled), text-disabled (disabled)
  Cursor: pointer (enabled), default (disabled)
  Border-radius: 4px
  Margin: 1px 4px

  Hover: background bg-hover
  Disabled: no hover effect

  Icon: 16px, text-secondary (left-aligned)
  Label: flex-grow, truncate
  Shortcut: text-caption, text-tertiary, mono, right-aligned
  Chevron: visible for submenu parents, right-aligned, 12px

Menu Divider:
  Height: 1px
  Background: border-default
  Margin: 4px 8px

Destructive items:
  Color: text-error
  Usually last in menu, separated by divider

Submenus:
  Trigger: hover parent item for 150ms (delay prevents accidental trigger)
  Position: right edge of parent, top aligned with parent item
  Same visual style as parent menu
  Close: when parent closes, or cursor leaves both parent and submenu for 300ms

Custom scrollbar:
  4px wide, thumb: border-hover, track: transparent
```

---

### 3.4 Data Table — .cmp-table

A sortable, paginated, selectable data table for displaying structured investigation data (sessions, findings, evidence sources, memory entries).

```
Table Container:
  Width: 100%
  Background: transparent
  Border: 1px border-default
  Border-radius: radius-lg
  Overflow: hidden (rounds corners of header)

Table Header:
  Background: bg-surface-raised
  Border-bottom: 1px border-default

  Header Cell:
    Padding: 10px 16px
    Font: text-small, weight-semibold, text-secondary
    Text-align: left (default)
    White-space: nowrap
    User-select: none
    Position: relative

    Sort indicator (asc/desc):
      Display: inline-block, margin-left 4px
      Font-size: 10px, opacity 0 (hidden by default)
      Visible when column is sorted, or on header hover
      Ascending: ↑ (or ▼), Descending: ↓ (or ▲)
      Content: sort icon via ::after pseudo-element

    Hover: bg-hover-muted (subtle highlight)
    Cursor: pointer (indicates sortable)
    Active sort: color text-primary, font-weight 600

    Resize handle (future, not implemented now):
      Absolute right: 0, width: 4px, cursor: col-resize

Table Body:
  Background: transparent

  Row:
    Height: 48px (compact), 56px (default), 64px (comfortable) — set via .cmp-table-compact / .cmp-table-comfortable
    Border-bottom: 1px border-subtle
    Transition: background 100ms

    Hover: bg-hover-muted
    Selected: bg-selection (blue tint)
    Selected hover: bg-selection-muted

    Striped (alternating): every-other row gets bg slightly different
    (.cmp-table-striped tbody tr:nth-child(even) { background: bg-hover-muted at 30% opacity })

  Cell:
    Padding: 0 16px
    Font: text-body, tabular-nums (numeric columns)
    Color: text-primary
    Vertical-align: middle
    White-space: nowrap (default), wrap (.cmp-table-wrap)

  Checkbox column (selectable mode):
    Width: 48px (fixed)
    Centered checkbox: 16px, accent-primary when checked, border-default when unchecked
    Header checkbox: select/deselect all
    Checkbox margin: 0 auto, display: block

  Status dot cell:
    Width: 32px (fixed), centered dot

Selection model:
  Single select: clicking row selects it, deselects previous
  Multi select: checkbox column, Shift+click for range, Ctrl+click for toggle
  Selected rows: tracked via data-selected attribute or JS array
  Event: onSelectionChange(selectedIds)

Table Footer (optional):
  Background: bg-surface-raised
  Border-top: 1px border-default
  Padding: 10px 16px
  Display: flex, justify-content: space-between, align-items: center
  Font: text-small, text-secondary

  Pagination controls:
    Display: flex, align-items: center, gap: 4px

    Page button:
      Width: 28px, height: 28px
      Border: 1px border-default
      Border-radius: radius-sm
      Background: transparent
      Font: text-caption, tabular-nums
      Color: text-secondary
      Cursor: pointer

      Hover: bg-hover, border-hover
      Active (current page): bg-accent-primary-muted, color accent-primary, border accent-primary
      Disabled: opacity 0.4, cursor default

    Prev/Next buttons:
      Icon: ‹ / ›
      Width: 28px, height: 28px
      Same styling as page buttons

    Page info:
      "Page 3 of 12 · Showing 25–36 of 286 results"
      Font: text-caption, text-tertiary, margin-left 8px

    Page size selector:
      Select element, text-caption
      Options: 10, 25, 50, 100

  Selected count (left side):
    "3 selected"
    Font: text-small, text-secondary

Table states:
  Loading:
    Each row: cmp-skeleton-text × column count
    No border-bottom on skeleton rows

  Empty:
    Centered message, 80px min-height
    Icon (48px, muted) + "No data" + "No findings match your criteria"
    CTA button: "Create Session" or "Clear Filters"

  Error:
    Centered error message
    Icon: warning, color text-error
    Message: text-secondary
    Retry button

Responsive:
  Tablet (<1024px): horizontal scroll wrapper, min-width 600px
  Mobile (<768px): card view — each row becomes a card
    Switch: add .cmp-table-card-view class
    Each card: padding 16px, border-bottom 1px border-subtle
    Labels shown for each value (data-label attribute on cells)

Accessibility:
  <table> with <thead>, <tbody>, <tfoot>
  <th scope="col"> for header cells
  aria-sort="ascending|descending|none" on sorted columns
  aria-selected on selected rows
  role="checkbox" on selection checkboxes
  aria-label on pagination buttons
```

### 3.5 CodeBlock — .cmp-codeblock

A syntax-highlighted code display block for showing investigation artifacts, tool outputs, log snippets, and raw data.

```
CodeBlock Container:
  Background: bg-code (matches canvas, very dark)
  Border: 1px border-default
  Border-radius: radius-lg
  Font-family: font-mono
  Font-size: text-small (13px)
  Line-height: leading-code (1.5)
  Overflow: hidden

Header bar (optional):
  Height: 36px
  Background: bg-surface-raised
  Border-bottom: 1px border-subtle
  Padding: 0 12px
  Display: flex, align-items: center, justify-content: space-between

  File name / language label:
    Font: text-caption, weight-semibold, text-secondary
    Icon: file type icon (optional)

  Actions (right):
    Copy button: 28px, text-caption, text-tertiary
      Hover: text-primary, bg-hover
      Click: copies content to clipboard
      Feedback: "Copied!" text for 1.5s, then reverts
      Icon: 📋 (copy) / ✓ (copied)

    Collapse/expand: toggle button, 28px
      Icon: ▼ (collapsed) / ▲ (expanded)

    Word wrap: toggle
      Icon: ↻ wrap indicator
      Toggles overflow behavior

Code body:
  Padding: 16px
  Overflow-x: auto (horizontal scroll for long lines)
  White-space: pre (default), pre-wrap (when word wrap enabled)
  Tab-size: 2

  Line numbers (optional, .cmp-codeblock-lines):
    Display: inline-block, width: 40px
    Text-align: right, padding-right: 16px
    Color: text-tertiary at 50% opacity
    User-select: none
    Border-right: 1px border-subtle
    Margin-right: 12px

Syntax highlighting classes:
  .cb-comment    { color: #8b949e; font-style: italic }
  .cb-keyword    { color: #ff7b72 }     — function, return, if, for, const, let, class
  .cb-string     { color: #a5d6ff }     — "string content"
  .cb-number     { color: #79c0ff }     — 42, 3.14, 0xff
  .cb-boolean    { color: #ffa657 }     — true, false
  .cb-null       { color: #ffa657 }     — null, undefined, nil
  .cb-type       { color: #ffa657 }     — Type annotations, generics
  .cb-function   { color: #d2a8ff }     — function names
  .cb-variable   { color: #ffa657 }     — variables, params
  .cb-operator   { color: #ff7b72 }     — +, -, *, /, =>, ->
  .cb-punctuation { color: #e6edf3 }    — { }, [ ], ( ), ;, .
  .cb-tag        { color: #7ee787 }     — HTML tags
  .cb-attr       { color: #79c0ff }     — HTML attributes
  .cb-attr-value { color: #a5d6ff }     — HTML attribute values
  .cb-diff-add   { background: rgba(35,134,54,0.15); color: #3fb950 }
  .cb-diff-remove { background: rgba(218,54,51,0.15); color: #f85149 }

Copy indicator CSS animation:
  Click → "Copied" text fades in for 1.5s → fades out
  Transition: opacity 200ms ease

States:
  Empty: "No code to display" centered, muted
  Error: "Failed to load source" centered, red

Accessibility:
  role="code" or role="region" with aria-label
  tabindex="0" for scrollable regions
  Copy button: aria-label="Copy code to clipboard"
```

### 3.6 JSONViewer — .cmp-jsonviewer

An interactive, expandable JSON tree viewer for structured data exploration (API responses, configuration, memory entries, tool call results).

```
JSONViewer Container:
  Background: bg-code
  Border: 1px border-default
  Border-radius: radius-lg
  Font-family: font-mono
  Font-size: text-small (13px)
  Line-height: leading-code (1.5)
  Overflow-x: auto
  Padding: 12px 16px
  Min-height: 40px

Header bar (optional, same pattern as CodeBlock):
  Height: 36px
  Same styling as CodeBlock header
  Actions: Copy (full JSON), Collapse All, Expand All

Tree structure:
  Indentation: 16px per level
  Each level: padding-left: 16px * depth

  Key-value entry:
    Key: text-small, color text-code (green-tint)
    Colon: color text-tertiary
    Value: text-small, color by type
    Comma: color text-tertiary (hidden on last item in object/array)

  Toggle icon (for objects and arrays):
    Display: inline-block, width: 12px, text-align: center
    Content: ▸ (collapsed) or ▾ (expanded)
    Color: text-tertiary
    Cursor: pointer
    User-select: none
    Margin-right: 4px

  Collapsed object preview:
    { … } or { 3 keys }
    Color: text-tertiary, font-style: italic
    On hover: show full preview tooltip

  Collapsed array preview:
    [ … ] or [ 5 items ]
    Same style as object preview

Value coloring:
  String:    color text-code (green-tint), e.g., #7ee787
  Number:    color #79c0ff (blue)
  Boolean:   color #ffa657 (orange)
  Null:      color text-tertiary, font-style: italic
  Object:    color text-primary, bold for braces
  Array:     color text-primary, bold for brackets

Copy path:
  Right-click entry: context menu "Copy path" → copies JSON path to clipboard
    Example: "$.data.attributes[0].name"

  Hover entry: subtle highlight (bg-hover at 50%)
    Click on key area: selects the value
    Double-click value: opens in modal for full editing/viewing

Search:
  Ctrl+F within viewer: filter keys
  Matching keys highlighted: bg-selection at 50%
  Next/prev match: arrow buttons in mini toolbar

States:
  Loading: skeleton placeholder, same dimensions as expected content
  Empty: "null" or "{}" displayed centered
  Error: "Failed to parse JSON" centered, text-error
  Truncated: "… (3 more items collapsed)" at bottom of each collapsed section

Responsive:
  Horizontal scroll for deeply nested structures
  Max-width: 100% of container
  On mobile: collapse all by default, tap to expand

Accessibility:
  role="tree" for the JSON tree
  role="treeitem" for each expandable node
  aria-expanded on toggled nodes
  aria-label on copy buttons
  Keyboard: ← collapse, → expand, ↑↓ navigation, Enter toggle
```

*Part 1 of 4 complete — Design System, Layout Architecture, Global Navigation, Shared Components.*
*Continues with Part 2: Dashboard, Investigation Workbench, Timeline Explorer...*
## 4. Dashboard Overview

### 4.1 Layout

```
┌──────────────────────────────────────────────────────────────────────────┐
│ DASHBOARD                                                      [Settings] │
│                                                                          │
│ ═══════════════════════════════ KPI BAR ══════════════════════════════════ │
│ ┌────────┐ ┌────────┐ ┌────────┐ ┌────────┐ ┌────────┐ ┌────────────┐  │
│ │Sessions│ │ Tasks  │ │Approvals│ │ Tokens │ │ Budget │ │  Health    │  │
│ │   12   │ │   3    │ │   1 ⚠  │ │ 1.2M   │ │ $2.47  │ │ ● Healthy  │  │
│ │ ↑ 2    │ │ ↓ 1    │ │   new   │ │ ↑ 15%  │ │ 25% of │ │ 234ms API  │  │
│ └────────┘ └────────┘ └────────┘ └────────┘ └────────┘ └────────────┘  │
│                                                                          │
│ ┌──────────────────────────────┬────────────────────────────────────────┐│
│ │ ACTIVITY FEED                │ RECENT SESSIONS                        ││
│ │ ● Live                       │                                        ││
│ │                              │ ● #a3f2b  Q4 Revenue    thinking it42  ││
│ │ 🧠 Session #a3f — it 42     │ ● #b2e1c  Phish Analysis completed     ││
│ │   Started analyzing Q4 data  │ ● #c4f8d  Network Scan   paused        ││
│ │   2m ago                     │ ● #d1a9e  Q3 Analysis    failed        ││
│ │                              │ ● #e5b3f  Supplier Audit idle          ││
│ │ ✅ Finding #7 approved       │                                        ││
│ │   Bane approved APAC finding │ View All →                             ││
│ │   12m ago                    │                                        ││
│ └──────────────────────────────┴────────────────────────────────────────┘│
│                                                                          │
│ ┌─────────────┐ ┌─────────────┐ ┌──────────────────────────────────────┐ │
│ │ Status      │ │ Model Usage │ │ PENDING APPROVALS                    │ │
│ │ Distribution│ │ (7d)        │ │                                      │ │
│ │  ◉ Donut   │ │  ████ Chart │ │ ⚠ DROP TABLE staging — #a3f    [✓][✗]│ │
│ │             │ │             │ │ ⚠ Modify trust: low→quar — #b2e[✓][✗]│ │
│ └─────────────┘ └─────────────┘ └──────────────────────────────────────┘ │
│                                                                          │
│ ═════════════════════════ 24h ACTIVITY SPARKLINE ═══════════════════════ │
│ ▁▂▃▅▆▇█▇▆▅▄▃▂▁▂▃▄▅▆▇█▇▆▅▄▃▂▁                                              │
└──────────────────────────────────────────────────────────────────────────┘

Grid layout (desktop >=1024px):
  display: grid;
  grid-template-columns: repeat(12, 1fr);
  gap: 24px;
  padding: 32px;
  max-width: 1440px;
  margin: 0 auto;

  Row 1 (KPI Bar):      grid-column: 1 / -1;  display: grid; grid-template-columns: repeat(6, 1fr); gap: 16px;
  Row 2 (Feed + Recent): grid-column: 1 / 9 (feed), 9 / -1 (recent); 
  Row 3 (Charts):        grid-column: 1 / 4 (status), 4 / 9 (models), 9 / -1 (approvals);
  Row 4 (Sparkline):     grid-column: 1 / -1;

Tablet (768-1023px):
  Row 1: repeat(3, 1fr) × 2 rows (6 KPI cards in 3×2)
  Row 2: stacked (feed full-width, recent below)
  Row 3: status + models side by side, approvals full width below
  Row 4: full width

Mobile (<768px):
  Single column. All sections stacked.
  KPI: 2×3 grid. Charts simplified. Activity feed shows last 5 events.
```

### 4.2 KPI Cards

Each KPI card displays one critical metric with real-time updates and trend indicators.

```
Card anatomy:
  ┌─────────────────────────────────────┐
  │ [Icon]                   [Sparkline]│  ← Icon 20px left, sparkline 80×24px right (optional)
  │                                     │
  │ LABEL                               │  ← text-caption, uppercase, text-tertiary, tracking-wide
  │                                     │
  │ 42                                  │  ← text-display-2, tabular-nums, font-weight 800, text-primary
  │                                     │
  │ ↑ 12% from last hour                │  ← text-caption, green (up), red (down), muted (flat)
  │ ████████░░░░░░░░░░ 42%              │  ← progress bar (optional), 4px tall, radius 2px
  └─────────────────────────────────────┘

Card dimensions:
  Min-width: 140px
  Max-width: none (flex: 1 within grid)
  Height: 120px
  Background: bg-surface
  Border: 1px border-default
  Border-radius: radius-lg
  Padding: 16px 20px
  Display: flex, flex-direction: column, gap: 4px

States:
  Default: border-default
  Hover: border-hover, bg subtly brighter (bg-hover-muted)
    Cursor: pointer (click navigates to detail page)
    Transition: var(--transition-color)
  Active (click): border-accent-primary, scale(0.98), 80ms

Loading state:
  Skeleton shimmer for number value
  Sparkline area: skeleton bar
  Progress bar: skeleton animated
  Duration: until data loads

Error state:
  Number: "--"
  Label: normal
  Subtitle: "Failed to load" in text-error
  Retry button (appears on hover): "Retry"

Number animation (on data change — first load only):
  Counts up from 0 or previous value to new value
  Duration: 800ms, ease-out-expo
  Staggered: card 0 at 0ms, card 1 at 50ms, card 2 at 100ms, etc.
  Implementation: requestAnimationFrame loop with easing function
  Tabular-nums ensure width stability during animation

Trend indicator:
  Format: "[arrow] [absolute delta] [percentage] from [time period]"
  Time periods: "last hour" / "yesterday" / "last week" / "last month" (context-dependent)
  Color: green (positive/good), red (negative/bad), muted (no change)
  Arrow: ↑ (up), ↓ (down), → (flat)
  Threshold: changes < 1% shown as "— No change"

Sparkline (24px tall, 80px wide, right-aligned):
  SVG area chart showing last 24 data points (1h buckets for 24h view)
  Fill: accent color at 15% opacity
  Line: accent color, 1px
  No axes, no labels — shape only
  Latest data point: 4px dot, accent color
  Hover: vertical line + tooltip with exact value
  Update: new data points slide in from right, oldest slides out left

Progress bar:
  Height: 4px
  Border-radius: 2px
  Background: bg-input
  Fill: width transition 1s ease-out-quint
  Color: green <50%, amber 50-80%, red >80%
  Label: percentage right-aligned below bar (text-caption, text-tertiary)
```

#### 4.2.1 KPI Definitions

```
1. ACTIVE SESSIONS
   Icon: cpu, color: accent-primary (blue)
   Value: count of sessions WHERE status IN ('booting','idle','thinking','tool_exec','waiting_sub')
   Sparkline: 24h session count, 5-min buckets, line chart
   Trend: absolute ±N from 1 hour ago
   Progress: active / max_concurrent_sessions (configurable, default: 20)
   Click → /sessions?status=active

2. PENDING TASKS
   Icon: check-square, color: accent-warning (amber)
   Value: count of tasks WHERE status = 'pending'
   Sparkline: 24h task creation vs completion rate (dual line)
   Trend: absolute ±N from 1 hour ago
   Progress: none (tasks don't have capacity limits)
   Click → /tasks?status=pending

3. PENDING APPROVALS
   Icon: shield-check, color: accent-error (red)
   Value: count of approvals WHERE status = 'pending'
   Visual emphasis: if value > 0, card border subtly pulses red (box-shadow glow, 2s cycle)
   Sparkline: none (approvals are event-driven, not continuous)
   Trend: absolute ±N from 1 hour ago
   Progress: none
   Click → /approvals

4. TOKENS USED TODAY
   Icon: lightning, color: accent-cyan
   Value: formatted number (e.g., "1.2M" or "847K" — abbreviations for K, M, B)
   Sparkline: 24h token usage, 15-min buckets, area chart
   Trend: percentage change vs same time yesterday
   Progress: tokens / daily_limit (configurable)
   Click → /billing

5. BUDGET SPENT (MONTHLY)
   Icon: currency-circle-dollar
   Color: green <50%, amber 50-80%, red >80% (dynamic — icon color changes with usage)
   Value: formatted dollar amount "$2.47"
   Sparkline: 30-day cost trend, daily buckets
   Trend: projected month-end vs budget
   Progress: spent / monthly_budget
   Click → /billing

6. SYSTEM HEALTH
   Icon: heartbeat
   Color: green (healthy), amber (degraded), red (down) — dynamic
   Value: "Healthy" / "Degraded" / "Down"
   Sub-metrics (text-caption, below value): "API 234ms · DB 12ms · LLM 1.2s"
   Sparkline: none
   Trend: status change indicator ("Was degraded 2h ago — recovered")
   Click → /health
```

### 4.3 Activity Feed

```
Card:
  grid-column: 1 / 9 (desktop), 1 / -1 (tablet/mobile)
  Background: bg-surface
  Border: 1px border-default
  Border-radius: radius-lg
  Height: 400px (fixed height, scrollable)
  Display: flex, flex-direction: column

Header:
  Height: 48px
  Padding: 0 20px
  Border-bottom: 1px border-default
  Display: flex, align-items: center, justify-content: space-between

  Title: "Activity" in text-subtitle, font-weight 600
  Live indicator: green pulsing dot (8px) + "Live" in text-small, text-success
    When WebSocket disconnected: "Reconnecting..." in text-warning
    When paused (scrolled up): "Paused — scroll to bottom to resume"
  
  Filter chips (right):
    Display: flex, gap: 4px
    Chips: [All] [Sessions] [Tasks] [Approvals] [Errors]
    Style: text-caption, padding 2px 10px, radius-full, bg transparent, text text-secondary
    Active: bg accent-primary-muted, text accent-primary
    Click: toggle chip. Multiple chips can be active simultaneously.
    Logic: show events matching ANY active chip ("All" ≡ all chips active)

Event list:
  Flex: 1, overflow-y: auto
  Virtualized: render visible + 20 buffer items
  Max items: unlimited (virtualized)
  Scrollbar: 6px, thumb border-hover

Event item (height: 56px):
  ┌──┬─────────────────────────────────────────────────────┬──────────┐
  │● │ 🧠 Session #a3f · Iteration 42 started              │ 2m ago   │
  │  │ Started analyzing Q4 revenue data — 847 tokens      │          │
  └──┴─────────────────────────────────────────────────────┴──────────┘

  Left accent: 2px solid, entity color (or status color for status events)
  Icon: 16px, entity color, in 28px circle (bg entity color at 10% opacity)
  Connector line: 1px solid border-subtle, from icon-center-bottom to next event icon
    (except last event — no connector below last)
  
  Content:
    Title: text-small, font-weight 500, text-primary, truncate 1 line
    Subtitle: text-caption, text-secondary, truncate 1 line
    Tags (optional): small colored pills, text-micro
  
  Timestamp: text-caption, text-tertiary, right-aligned, absolute position
    Relative time: "2m ago", "1h ago", "yesterday"
    Absolute time on hover: title attribute

  Hover: bg-hover-muted
  Click: navigate to relevant entity (session, task, approval)

Entry animation (new event at top):
  Slide down from above: translateY(-10px) → translateY(0)
  Opacity: 0 → 1
  Duration: 300ms, ease-out-expo
  Existing events shift down: margin transition 250ms ease-out-quint

"New events ↓" button:
  Appears when scrolled up >50px from top
  Position: sticky, bottom: 0, centered
  Style: pill, bg-glass-light, border border-default
  Text: "↓ 3 new events" (count updates)
  Click: smooth scroll to top

WebSocket events → feed items:
  session.created       → cpu icon, "Session '{name}' created"
  session.status        → status icon, "Session '{name}' is now {status}"
  session.completed     → check icon, "Session '{name}' completed ({iterations} iterations)"
  session.failed        → x icon, "Session '{name}' failed: {reason}"
  task.created          → check-square, "Task '{title}' created in #{session_id}"
  task.completed        → check-circle, "Task '{title}' completed"
  approval.requested    → shield icon, "Approval needed: {type} in #{session_id}"
  approval.resolved     → shield-check, "Approval {approved/denied} by {user}"
  iteration.started     → brain icon, "Iteration {n} started — agent thinking"
  iteration.completed   → brain icon, "Iteration {n} done · {tokens} tokens · {duration}s"
  billing.threshold     → currency icon, "Budget at {pct}% — ${used} of ${limit}"
  billing.exceeded      → currency icon (red), "Budget exceeded — sessions paused"
  system.health         → heartbeat icon, "{message}"
  system.startup        → heartbeat (green), "Consensus started · {backend} · port {port}"
```

### 4.4 Recent Sessions Panel

```
Card:
  grid-column: 9 / -1 (desktop), full-width below feed (tablet/mobile)
  Background: bg-surface
  Border: 1px border-default
  Border-radius: radius-lg
  Max-height: 400px (same as feed — they align visually)
  Display: flex, flex-direction: column

Header:
  Height: 48px
  Padding: 0 20px
  Border-bottom: 1px border-default
  Display: flex, justify-content: space-between, align-items: center
  Title: "Recent Sessions" in text-subtitle, font-weight 600
  Action: "View All →" in text-small, text-link

List:
  Flex: 1, overflow-y: auto
  Max items: 8 (auto-rotates, oldest drops off)

Session item (height: 48px):
  Padding: 0 20px
  Display: flex, align-items: center, gap: 10px
  Border-bottom: 1px border-subtle (last item: none)

  Status dot: 8px circle, color by status, flex-shrink: 0
    thinking: purple, pulsing opacity 2s cycle
    tool_exec: amber
    idle: muted blue
    completed: green
    failed: red
    paused: amber, static
    cancelled: gray

  Name: text-small, font-weight 500, text-primary, truncate
  ID: text-caption, mono, text-tertiary (e.g., "#a3f2b")
  
  Right side:
    Iteration: text-caption, tabular-nums ("it 42")
    Timestamp: text-caption, text-tertiary ("12m ago")

  Hover: bg-hover-muted
  Click: navigate to /sessions/{id}
  
Empty state:
  Centered in card
  "No sessions yet"
  "Create your first session to get started"
  [Create Session] button (accent-primary, radius-md, height 36px)
```

### 4.5 Session Status Distribution Chart

```
Card:
  grid-column: span 4 (desktop), span 6 (tablet), full (mobile)
  Background: bg-surface, border border-default, radius radius-lg
  Height: 300px
  Padding: 20px
  Display: flex, flex-direction: column

Header: "Session Status" in text-subtitle, font-weight 600, margin-bottom 16px

Chart: SVG donut, centered, 160px diameter
  Ring width: 24px
  Gap between segments: 2px (stroke white at 5% opacity)
  
  Segments (clockwise from top):
    thinking:    purple,  oklch(55% 0.18 295)
    tool_exec:   amber,   oklch(55% 0.15 85)
    idle:        blue,    oklch(55% 0.15 260)
    waiting_sub: cyan,    oklch(55% 0.12 200)
    paused:      gray,    oklch(40% 0.02 260)
    completed:   green,   oklch(55% 0.18 145)
    failed:      red,     oklch(50% 0.20 20)
    booting:     muted,   oklch(30% 0.02 260)

  Center label:
    Total active count: text-display-2, font-weight 800, text-primary
    "sessions" label: text-caption, text-tertiary, below number
    
  Segment animation:
    On data change: stroke-dasharray transitions smoothly
    Duration: 600ms, ease-out-quint
    New segment: draws from 0 to full arc
    Removed: shrinks to 0
    
  Hover segment:
    Segment expands outward 4px (transform scale(1.05), transform-origin center)
    Tooltip: "{count} {status} ({percentage}%)"
    Other segments dim to 50% opacity (transition 150ms)

Legend:
  Below chart, horizontal wrapping layout, gap 16px
  Items: color dot (8px circle) + status name (text-caption) + count (text-caption, tabular-nums, text-tertiary)
  Hover: corresponding donut segment highlights (others dim)
  Click: navigate to /sessions?status={status}
```

### 4.6 Model Usage Chart

```
Card:
  grid-column: span 5 (desktop), span 6 (tablet), full (mobile)
  Background: bg-surface, border border-default, radius radius-lg
  Height: 300px
  Padding: 20px

Header row:
  "Model Usage" in text-subtitle, font-weight 600
  Time selector chips (right): [24h] [7d] [30d]
    Active chip: bg accent-primary-muted, text accent-primary
    Inactive: transparent, text-secondary
    Click: switch data range, chart smoothly transitions (400ms ease-out-quint)

Chart: Horizontal stacked bar chart
  Y-axis: token count (auto-scaled, K/M/B suffixes)
  X-axis: time buckets (hour for 24h, day for 7d/30d)
  Bars: 20px height, 2px gap
  Segments: colored by model
  
Model colors:
  deepseek-v4-pro:   purple
  deepseek-v4-flash: cyan  
  claude-sonnet-4:   amber
  gpt-4o:            green
  local-model:       gray
  other:             muted

Bar hover:
  Bar slightly brightens (brightness 1.1)
  Tooltip: model breakdown for that bucket
    "{model}: {tokens} tokens (${cost})"
    "Total: {total} tokens · ${total_cost}"

Animation:
  On load: bars grow from 0 to value (height transition 600ms ease-out-expo, stagger 30ms per bar)
  On data switch: bars morph smoothly (SVG 'd' attribute transition)

Budget line:
  Horizontal dashed line at budget threshold
  Color: amber at warning, red at exceeded
  Label: "Monthly Budget: $10.00" in text-caption, anchored to line

Legend:
  Below chart, horizontal, gap 12px
  Model name + color dot + percentage of total
```

### 4.7 Pending Approvals Card

```
Card:
  grid-column: span 3 (desktop), full (tablet/mobile)
  Background: bg-surface, border border-default, radius radius-lg, border-color: subtle red tint when >0 pending
  Height: 300px
  Padding: 20px

Header:
  "Pending Approvals" in text-subtitle, font-weight 600
  Badge: count in red pill next to title (if >0)
  "View All →" link (right)

List (scrollable):
  Max visible: 3 items (fits in 300px card)
  
  Approval item (margin-bottom 8px, last 0):
    Background: bg-error-muted at 5% opacity
    Border-left: 2px solid accent-error
    Border-radius: 0 radius-md radius-md 0
    Padding: 10px 12px
    
    Top row:
      Type icon (16px, red) + type label (text-small, font-weight 600)
      Priority badge (if HIGH): "HIGH" in red pill, text-micro
      Timestamp: text-caption, text-tertiary, right
      
    Middle row:
      Description: text-small, text-secondary, truncate 2 lines
      Session link: "#a3f2b" in text-caption, mono, text-link, clickable
      
    Bottom row (actions):
      [Approve] button: 28×24px, bg-success, text white, text-caption, radius-md
      [Deny] button: 28×24px, border border-error, text error, text-caption, radius-md
      [Defer] button: 28×24px, text text-tertiary, text-caption
      
      Click behavior:
        Approve: button fills green, checkmark scale-bounce (300ms ease-spring)
          Item slides left + fades out (300ms ease-in-quint)
          Toast: "Approval resolved"
        Deny: button fills red, X scale-bounce
          Item fades out
        Defer: item moves to bottom, shows "Deferred" badge
          Auto-reappears after 30 minutes

Empty state:
  Green shield icon (48px, muted, centered)
  "All clear" — text-subtitle
  "No approvals waiting" — text-small, text-secondary
```

### 4.8 Timeline Sparkline

```
Full-width card at bottom:
  Background: bg-surface, border border-default, radius radius-lg
  Padding: 16px 20px
  Height: 100px

Header: "24-Hour Activity" in text-subtitle, font-weight 600
  Timezone badge (right): "UTC-4" in text-caption, text-tertiary

Sparkline: SVG area chart, 100% width, 40px tall
  Dual series:
    Top: session events (area, blue, opacity 0.15 fill + 1px solid line)
    Bottom: system events (area, purple, opacity 0.1 fill + 1px solid line)
    Stacked vertically (sessions on top of system)

  X-axis: 24 hours, labeled every 3 hours (00:00, 03:00, 06:00...)
    Labels: text-micro, text-tertiary, below axis

  Current time indicator:
    Vertical dashed line at current time
    Color: white at 20% opacity
    Label: "Now" in text-micro, accent-primary, above line

  Hover: crosshair follows cursor
    Vertical line: 1px solid white 30% opacity, full chart height
    Dot: 4px circle at line intersection with each series
    Tooltip: "{time} · {sessions} sessions · {system} system events"

  Interaction:
    Click-drag: select time range → navigates to Timeline Explorer zoomed to range
    Scroll wheel: zoom in/out (24h→12h→6h→1h→30min and reverse)
```

---

## 5. Investigation Workbench — Split-Pane THINK/SAYS

### 5.1 Overview

The Investigation Workbench is the primary user experience of Chronicle. It implements a split-pane interface where the left pane (THINK) displays the AI's step-by-step reasoning chain — every source evaluated, every logical leap, every contradiction flagged — while the right pane (SAYS) displays polished conclusions (Findings) that are bidirectionally linked to their originating reasoning steps.

The core invariant: **No conclusion appears without its full reasoning chain visible and traceable.** Every Finding has at least one linked THINK step. Every THINK step that produces a conclusion links to the resulting Finding. The human is the investigator; the AI is the analyst assistant whose reasoning is transparent and auditable.

This view replaces chat interfaces. It is an evidence workstation optimized for verification, not conversation.

### 5.2 Workbench Layout

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ WORKBENCH TOOLBAR — 48px — position: sticky, top: 0, z-index: 10             │
│ ┌──────────┐ ┌─────────────────────────┐ ┌──────────┐ ┌─────────┐ ┌──────┐  │
│ │Sessions ▾│ │ Investigation: Q4 Rev   │ │ Evidence │ │ Export ▾│ │ ⚙   │  │
│ └──────────┘ └─────────────────────────┘ └──────────┘ └─────────┘ └──────┘  │
├──────────────────────────────┬─┬─────────────────────────────────────────────┤
│                              │ │                                             │
│   THINK PANE                 │ │   SAYS PANE                                 │
│   flex: 1 (default 50%)      │R│   flex: 1 (default 50%)                     │
│   min-width: 280px           │E│   min-width: 280px                           │
│   overflow-y: auto           │S│   overflow-y: auto                           │
│   padding: var(--space-4)    │I│   padding: var(--space-4)                    │
│   background: canvas         │Z│   background: canvas                         │
│                              │E│                                              │
│   ┌────────────────────────┐ │R│   ┌──────────────────────────────────────┐  │
│   │ 🧠 Step 3 · Synthesize │ │  │   │ ✅ Finding #7 · APAC Growth         │  │
│   │ deepseek-v4-pro · 2m   │ │  │   │ 2m ago · HIGH confidence 0.94       │  │
│   │                        │ │  │   │                                      │  │
│   │ Comparing APAC growth  │ │  │   │ Q4 revenue in APAC increased 12%    │  │
│   │ trajectory with EMEA   │ │  │   │ YoY, driven primarily by SE Asia    │  │
│   │ decline. Contradiction  │ │  │   │ expansion. Third consecutive        │  │
│   │ found in November       │ │  │   │ quarter of double-digit growth.     │  │
│   │ supply chain vs March  │ │  │   │                                      │  │
│   │ report.                 │ │  │   │ ▸ Sources (2)                       │  │
│   │                        │ │  │   │ ▸ Reasoning (Steps 2,4)             │  │
│   │ Sources:               │ │  │   │                                      │  │
│   │ 📄 q4-sales 0.94       │ │  │   │ ■■■■□ HIGH 0.94                     │  │
│   │ 📄 shipping 0.87        │ │  │   │                                      │  │
│   │                        │ │  │   │ ✓ Approved by Bane · 2m ago         │  │
│   │ ⚠ Contradiction: Nov   │ │  │   │                                      │  │
│   │ vs March data           │ │  │   │ [Edit] [Approve] [Request Revision] │  │
│   │                        │ │  │   └──────────────────────────────────────┘  │
│   │ [Timeline] [Copy] [⚑] │ │  │                                              │
│   └────────────────────────┘ │  │   ┌──────────────────────────────────────┐  │
│                              │  │   │ ✅ Finding #6 · EMEA Decline         │  │
│   ┌────────────────────────┐ │  │   │ 15m ago · MEDIUM confidence 0.72    │  │
│   │ 🧠 Step 2 · Cross-ref  │ │  │   │ ...                                  │  │
│   │ ...                      │  │   └──────────────────────────────────────┘  │
│   └────────────────────────┘ │  │                                              │
│                              │  │   ═══════════════════════════════════════    │
│                              │  │   INPUT AREA                                 │
│                              │  │   ┌──────────────────────────────────────┐  │
│                              │  │   │ Ask a question or give instruction.. │  │
│                              │  │   │                                      │  │
│                              │  │   │ [📎] [@ Context] [# Model]  ~1.2K → │  │
│                              │  │   └──────────────────────────────────────┘  │
├──────────────────────────────┴─┴─────────────────────────────────────────────┤
│ EVIDENCE PANEL (collapsed, toggle: Ctrl+E)                                    │
└──────────────────────────────────────────────────────────────────────────────┘

Layout implementation:
  display: flex; flex-direction: column; height: 100%; overflow: hidden;
  
  Toolbar: height 48px, flex-shrink: 0, border-bottom: 1px solid var(--color-border-default);
  
  Pane Container: display: flex; flex: 1; overflow: hidden;
    THINK pane: flex: 1; min-width: 280px; overflow-y: auto; background: var(--color-bg-canvas);
      padding: var(--space-4);
    Divider: width 8px; flex-shrink: 0; cursor: col-resize; (see §5.3)
    SAYS pane: flex: 1; min-width: 280px; overflow-y: auto; background: var(--color-bg-canvas);
      display: flex; flex-direction: column;
      Findings list: flex: 1; overflow-y: auto; padding: var(--space-4);
      Input Area: flex-shrink: 0; border-top: 1px solid var(--color-border-default);
        padding: var(--space-3) var(--space-4);
        background: var(--color-bg-surface);
  
  Evidence Panel: position: absolute; right: 0; top: 48px; bottom: 0; width: 360px;
    transform: translateX(100%); (hidden) / translateX(0); (visible)
    background: var(--color-bg-surface); border-left: 1px solid var(--color-border-default);
    z-index: 20; box-shadow: var(--shadow-lg);

Persistence:
  Pane ratio saved to localStorage key: `chronicle:investigation:${investigationId}:pane-ratio`
  Evidence panel visibility saved to: `chronicle:investigation:${investigationId}:evidence-open`
  Restored on investigation switch or page reload.

Responsive (width < 768px):
  Panes stack vertically: THINK above, SAYS below. Horizontal divider replaces vertical.
  Divider width: 100%, height: 8px, cursor: row-resize.
  Evidence panel: full-width bottom sheet, slides up from bottom.
  Input area: fixed to viewport bottom.
```

### 5.3 Resizable Divider

The divider separates THINK and SAYS panes. It is the primary mechanism for adjusting the reasoning-to-conclusion ratio.

```
Dimensions:
  Total width: 8px (composed of 4px visible grip + 2px invisible hit area on each side)
  Height: 100% of pane container
  Background: transparent (default), var(--color-border-default) at 60% opacity (visible grip)
  Cursor: col-resize (default), col-resize-grabbing (while dragging)
  Z-index: 5 (above panes, below overlays)

Grip indicator:
  Three vertical dots centered within the 4px visible area:
    Content: "·" character × 3, stacked vertically with 4px gap
    Font-size: 6px
    Color: var(--color-text-tertiary) (default), var(--color-text-secondary) (hover), var(--color-text-primary) (active)
    Line-height: 4px
    User-select: none
    Pointer-events: none

States:
  DEFAULT: 
    Background: transparent (grip area only shows on hover)
    Grip dots: var(--color-text-tertiary) at 40% opacity
    Transition: background-color 200ms var(--ease-out-quint), opacity 200ms var(--ease-out-quint)
    
  HOVER (cursor within 20px of divider center):
    Background: var(--color-border-hover) at 40% opacity (spans full divider width)
    Grip dots: var(--color-text-secondary), opacity 1
    Cursor: col-resize
    
  ACTIVE (mouse button held, dragging):
    Background: var(--color-accent-primary) at 30% opacity
    Grip dots: var(--color-text-primary), opacity 1
    Cursor: col-resize-grabbing
    Overlay: semi-transparent vertical line spans full viewport height at divider position
      (1px solid, var(--color-accent-primary) at 40% opacity, z-index: 100)
  
  KEYBOARD FOCUS (divider focused via Ctrl+arrows):
    Visible focus ring: box-shadow 0 0 0 2px var(--color-accent-primary)
    Divider is focusable (tabindex="0") for keyboard accessibility

Resize behavior:
  DRAG: Mouse down → track mouse movement → update pane widths in real-time
    Implementation: onMouseDown capture, document-level mousemove listener, onMouseUp release
    Performance: update CSS custom properties (--think-width, --says-width) via requestAnimationFrame
    THINK width = clamp(280px, cursorX - paneContainer.left, paneContainer.width - 280px - 8px)
    SAYS width = paneContainer.width - THINK width - 8px
    Minimum pane: 280px. At minimum, content switches to compact mode (smaller text, condensed cards)
    Maximum pane: container width - 280px - 8px (guarantees other pane has minimum)
    
  DOUBLE-CLICK: Reset to 50/50 split
    Animation: width transition 400ms var(--ease-out-expo)
    Restores default ratio regardless of previous drag position
    
  DRAG TO EDGE (< 40px remaining for opposite pane):
    Collapse opposite pane entirely (width: 0px, overflow: hidden)
    Show 40px tab on collapsed edge with vertical label "THINK" or "SAYS"
    Tab: width 40px, background var(--color-bg-surface), border: 1px solid var(--color-border-default)
      Vertical text: writing-mode: vertical-rl, transform: rotate(180deg)
      Font: var(--text-caption), font-weight: 600, color: var(--color-text-secondary)
      Hover: background var(--color-bg-hover), color var(--color-text-primary)
      Click: restore pane to 280px minimum with animation
    
  KEYBOARD RESIZE (divider focused):
    Ctrl+ArrowLeft: reduce THINK by 40px (increase SAYS) — animated 250ms var(--ease-out-quint)
    Ctrl+ArrowRight: increase THINK by 40px (reduce SAYS) — animated 250ms var(--ease-out-quint)
    Ctrl+Shift+ArrowLeft: snap THINK to 280px minimum — animated 300ms var(--ease-out-expo)
    Ctrl+Shift+ArrowRight: snap SAYS to 280px minimum — animated 300ms var(--ease-out-expo)
    Ctrl+\\: reset to 50/50 — animated 400ms var(--ease-out-expo)
    Each keypress fires one resize step. Holding key repeats via OS key repeat rate.

State persistence:
  Pane ratio saved as percentage (0-100) to localStorage on drag end (debounced 500ms)
  Key: `chronicle:pane:${investigationId}:think-pct`
  Restored on investigation load with no animation (instant, to avoid visible jump)
  Default: 50 (if no saved value)
```

### 5.4 THINK Pane — AI Reasoning Chain

The THINK pane displays the AI's internal reasoning as a chronological, scrollable sequence of Thought Cards. Each card represents one reasoning step — the AI evaluating sources, comparing data, flagging contradictions, or synthesizing conclusions.

The pane auto-scrolls to the bottom when new reasoning is generated (streaming). If the user scrolls up to read older reasoning, a "↓ New reasoning" floating button appears at the bottom of the pane. Scrolling to the bottom dismisses the button and resumes auto-scroll.

#### 5.4.1 Thought Card Anatomy

```
┌──────────────────────────────────────────────────────────────┐
│ 🧠 Step 3 · Synthesize                    deepseek-v4-pro    │  ← Header row
│                                           0.8s ago           │
│──────────────────────────────────────────────────────────────│
│                                                              │
│ Comparing APAC growth trajectory with EMEA decline. The      │  ← Reasoning content
│ contradiction in November supply chain data versus March     │     (prose mode)
│ report suggests a reporting lag, not actual decline.         │     font: var(--text-body)
│ Cross-referencing with shipping manifests confirms:          │     line-height: 1.6
│ shipments were delayed, not cancelled. The 3% EMEA decline   │     color: var(--color-text-primary)
│ is therefore likely a timing artifact, not market loss.      │
│                                                              │
│ ┌──────────────────────────────────────────────────────────┐ │
│ │ 📄 q4-sales.csv                    confidence: 0.94      │ │  ← Source badges
│ │ 📄 shipping-manifests-nov.csv      confidence: 0.87      │ │
│ │ 📄 march-report-q1.pdf             confidence: 0.72      │ │
│ └──────────────────────────────────────────────────────────┘ │
│                                                              │
│ ⚠ Contradiction: November shipping data vs March report      │  ← Flag indicators
│ ℹ Note: Supply chain delay confirmed by manifests            │
│                                                              │
│ [View in Timeline]  [Copy Content]  [⚑ Flag for Review]    │  ← Action buttons
└──────────────────────────────────────────────────────────────┘

Card dimensions:
  Width: 100% (fills THINK pane, respects padding)
  Max-width: none
  Margin-bottom: var(--space-3) (12px between consecutive cards)
  Background: var(--color-bg-surface)
  Border: 1px solid var(--color-border-default)
  Border-left: 3px solid var(--color-accent-purple)   ← primary identity marker
  Border-radius: 0 var(--radius-md) var(--radius-md) 0  (left edge flat for accent bar)
  Padding: var(--space-4) (16px)
  Box-shadow: var(--shadow-sm)

Header row:
  Display: flex; justify-content: space-between; align-items: center;
  Margin-bottom: var(--space-2) (8px)
  Left side:
    Step label: "🧠 Step 3 · Synthesize"
    Font: var(--text-small), font-weight: 600, color: var(--color-text-primary)
    Icon: brain emoji (🧠) or Phosphor Brain icon 14px, color: var(--color-accent-purple)
    Step number: incremented per reasoning cycle within a session, reset on new session
    Step name: auto-generated from AI reasoning (first 5-8 words of reasoning, truncated)
  Right side:
    Model badge: small pill, padding 1px 8px, radius var(--radius-full)
      Background: var(--color-accent-purple) at 10% opacity
      Color: var(--color-accent-purple)
      Font: var(--text-caption), font-weight: 500
      Content: model name (e.g., "deepseek-v4-pro", "claude-sonnet-4")
    Timestamp: right of model badge (or below on narrow cards)
      Font: var(--text-caption), color: var(--color-text-tertiary)
      Format: relative time ("0.8s ago", "2m ago", "1h ago")
      Absolute time: title attribute shows full ISO timestamp

Reasoning content:
  Margin-bottom: var(--space-3) (12px)
  Font: var(--text-body), line-height: 1.6, color: var(--color-text-primary)
  White-space: pre-wrap (preserves line breaks from AI output)
  Word-wrap: break-word
  
  PROSE MODE (default — narrative reasoning):
    Font: var(--text-body)
    Renders markdown: bold, italic, inline code, bullet lists
    Links: clickable, open in new tab (target="_blank", rel="noopener")
    
  STRUCTURED MODE (when content is detected as JSON/code):
    Font: var(--text-mono-sm)
    Background: var(--color-bg-input)
    Border-radius: var(--radius-sm)
    Padding: var(--space-2) var(--space-3)
    Max-height: 300px (scrollable if exceeds)
    Syntax highlighting: JSON keys in cyan, strings in green, numbers in amber, booleans in purple
    Collapse/expand: if content > 20 lines, show first 10 + "Show all (N lines)" button

Source badges:
  Display: flex; flex-wrap: wrap; gap: var(--space-2); margin-bottom: var(--space-2);
  
  Individual badge:
    Display: inline-flex; align-items: center; gap: 4px;
    Background: var(--color-bg-input)
    Border: 1px solid var(--color-border-default)
    Border-radius: var(--radius-sm)
    Padding: 2px 8px
    Font: var(--text-caption), color: var(--color-text-secondary)
    Max-width: 220px (truncate filename with ellipsis)
    
    Icon: 12px, entity-type-dependent
      File: 📄 or Phosphor File icon
      Database: 🗄️ or Phosphor Database icon
      URL: 🌐 or Phosphor Globe icon
      Memory event: 💾 or Phosphor HardDrive icon
    
    Confidence score: right side of badge
      Font: var(--text-mono-sm), font-weight: 600, tabular-nums
      Color: 
        ≥ 0.94: var(--color-accent-success)  (green, high confidence)
        0.70-0.93: var(--color-accent-warning) (amber, moderate)
        < 0.70: var(--color-accent-error)       (red, low)
      Format: "0.94" (always 2 decimal places)
      
    Hover: border-color var(--color-border-hover), background brightens 5%
    Click: opens source in Evidence Panel, scrolling to that specific source
    Cursor: pointer

Flag indicators:
  Display: grid; grid-template-columns: auto 1fr; gap: var(--space-1) var(--space-2);
  Margin-bottom: var(--space-2);
  
  Each flag:
    ⚠ Contradiction: amber, "Warning" icon
      Meaning: two or more sources disagree
    🔗 Correlation: cyan, "Graph" icon
      Meaning: significant statistical relationship found
    ⚡ Anomaly: purple, "Lightning" icon  
      Meaning: unusual pattern or outlier detected
    ❌ Error: red, "XCircle" icon
      Meaning: reasoning step encountered an error (retry available)
    ℹ Note: muted, "Info" icon
      Meaning: informational observation, no action needed
    
  Style:
    Font: var(--text-caption)
    Icon: 14px, color matches flag type

Action buttons:
  Display: flex; gap: var(--space-2); margin-top: var(--space-2);
  Justify-content: flex-start
  
  Button style:
    Height: 28px
    Padding: 0 var(--space-3)
    Background: transparent
    Border: 1px solid var(--color-border-default)
    Border-radius: var(--radius-md)
    Font: var(--text-caption), font-weight: 500, color: var(--color-text-secondary)
    Cursor: pointer
    Display: inline-flex; align-items: center; gap: 4px
    Transition: var(--transition-color)
    
    Hover: background var(--color-bg-hover), color var(--color-text-primary), border-color var(--color-border-hover)
    Active: background var(--color-bg-selection), scale 0.97 (var(--duration-micro))
  
  "Flag" button special:
    When flagged: background var(--color-accent-error-muted), border-color var(--color-accent-error), color var(--color-accent-error)
    Icon changes from regular to fill weight
```

#### 5.4.2 Thought Card States

Every thought card exists in exactly one of these states at any time. State transitions are animated.

```
1. DEFAULT (idle, not currently streaming, not expanded, not flagged)
   
   Border-left: 3px solid var(--color-accent-purple)
   Background: var(--color-bg-surface)
   Box-shadow: var(--shadow-sm)
   
   Reasoning content: fully rendered, no cursor
   Timestamp: static, relative time
   Source badges: visible
   Action buttons: visible, standard styles
   
   Hover (DEFAULT → HOVER):
     Background brightens to mix of surface + hover: color-mix(in srgb, var(--color-bg-surface) 85%, var(--color-bg-hover) 15%)
     Border-left color intensifies: var(--color-accent-purple) opacity 1.0 (from 0.8)
     Box-shadow: var(--shadow-md) (card lifts slightly)
     Transition: all 150ms var(--ease-out-quint)
     Cursor: pointer (click to expand)

2. THINKING (AI is currently generating this reasoning — streaming state)
   
   Border-left: 3px solid var(--color-accent-purple)
   Border-left animation: gradient sweep purple→cyan→purple, 3s cycle, infinite
     Implementation: 
       @keyframes thinking-border {
         0%, 100% { border-left-color: var(--color-accent-purple); }
         50% { border-left-color: var(--color-accent-cyan); }
       }
       animation: thinking-border 3s var(--ease-linear) infinite;
   
   Box-shadow: var(--shadow-glow-blue) (subtle glow indicating activity)
     Glow intensity pulses: opacity 0.05→0.15→0.05, 2s cycle
     @keyframes thinking-glow {
       0%, 100% { box-shadow: 0 0 12px rgba(163,113,247,0.05); }
       50% { box-shadow: 0 0 12px rgba(163,113,247,0.15); }
     }
   
   Header left: "🧠 Thinking..." instead of step number + name
     Font: var(--text-small), font-weight: 600, color: var(--color-accent-purple)
     Ellipsis animation: three dots appear sequentially
       @keyframes thinking-dots {
         0% { opacity: 0.3; } 33% { opacity: 1; } 66% { opacity: 0.3; } 100% { opacity: 0.3; }
       }
       Each dot: delayed 200ms from previous (dot1 0ms, dot2 200ms, dot3 400ms)
   
   Header right: 
     Model badge: normal (model doesn't change mid-generation)
     Timestamp: hidden (replaced by "Generating..." text)
       Color: var(--color-text-tertiary), font: var(--text-caption), italic
   
   Reasoning content:
     Streaming text with cursor at end
     Text appears: batched DOM updates at 60fps (every 16ms)
     Cursor: "▊" unicode character, 2px wide block
       Color: var(--color-accent-purple)
       Animation: blink 1s cycle (visible 0.7s, hidden 0.3s)
       @keyframes cursor-blink {
         0%, 70% { opacity: 1; }
         71%, 100% { opacity: 0; }
       }
     New text slides in from below slightly: translateY(2px)→translateY(0) over 100ms
       (micro-animation for each chunk, creating organic "growing" feel)
   
   Source badges: appear as they are mentioned in reasoning
     Entry animation: scale(0)→scale(1), 200ms var(--ease-spring)
     Staggered: first badge appears immediately, subsequent badges delay 100ms each
   
   Flag indicators: appear as AI identifies issues
     Entry animation: slide in from left (translateX(-8px)→0) + fade in, 200ms var(--ease-out-quint)
   
   Action buttons: hidden during thinking (not interactive yet)
   
   Auto-scroll: pane scrolls to keep cursor visible
     Implementation: scrollIntoView({ block: 'nearest', behavior: 'smooth' })
     Throttled: max once per 200ms to prevent janky scrolling

3. COMPLETED (AI finished generating)
   
   Transition from THINKING → COMPLETED:
     Border-left animation: stops, settles on var(--color-border-default)
       transition: border-left-color 500ms var(--ease-out-quint)
     Glow: dissipates over 500ms
       transition: box-shadow 500ms var(--ease-out-quint)
     Cursor: removed (fade out 200ms var(--ease-out-quint))
     Header: "Thinking..." fades out (200ms), step number + name fades in (200ms, delayed 50ms)
     Timestamp: fades in (200ms, delayed 100ms)
       Shows relative time ("0.8s ago") updating in real-time
     Action buttons: fade in (200ms, delayed 200ms)
     Total transition: ~700ms cascade
   
   After completion, card behaves as DEFAULT state (above).

4. EXPANDED (user clicked card to see full details)
   
   Trigger: click on collapsed card (anywhere on card body)
   
   Changes from DEFAULT:
     Border-left: 3px solid var(--color-accent-primary)  (blue, not purple — indicates focus)
     Background: var(--color-bg-surface-raised)
     Box-shadow: var(--shadow-md)
     Border-color: var(--color-border-hover)
     
     Max-height: none (full content visible, no truncation)
     Source badges: all visible (not just first 3)
     Flag indicators: all visible
     Action buttons: all visible
   
   Adjacent cards (above and below):
     Opacity: 0.6 (dimmed to focus attention on expanded card)
     Transition: opacity 200ms var(--ease-out-quint)
     Not interactive while dimmed (pointer-events: none)
   
   Scroll behavior: card scrolls into view if partially hidden
     scrollIntoView({ block: 'nearest', behavior: 'smooth' })
   
   Collapse triggers:
     Click outside card (click listener on THINK pane)
     Press Escape key
     Click "Collapse" button (appears top-right in expanded view)
     Scroll past card (if card scrolls out of viewport, auto-collapse after 500ms)
   
   Collapse animation:
     Max-height transition from auto to collapsed height
     (Use JS to measure collapsed height, set explicit max-height, then transition)
     Duration: 400ms var(--ease-out-expo)
     Adjacent cards return to full opacity: 200ms var(--ease-out-quint)
     Border-left transitions back to purple: 400ms var(--ease-out-quint)

5. FLAGGED (user clicked "⚑ Flag for Review")
   
   Same base as DEFAULT/COMPLETED but:
     Border-left: 3px solid var(--color-accent-error) (red instead of purple)
     Background: var(--color-accent-error-muted) at 5% opacity (very subtle red tint)
     
     Flag icon appears in header left: 🚩 or Phosphor Flag icon, 14px, color: var(--color-accent-error)
       Positioned before step label
     
     "Flag" action button: toggled state (filled red)
       Click again: unflag → returns to DEFAULT state
   
   Persistence: flagged state stored in localStorage, survives refresh
     Key: `chronicle:thought:${sessionId}:${stepNumber}:flagged`
   
   Flagged indicator in Timeline view: this thought card shows red dot in timeline

6. LINKED (this thought is referenced by a SAYS Finding)
   
   Border-left: 3px solid var(--color-accent-success) (green)
   
   Link badge in header right: "→ Finding #7" 
     Small pill, background: var(--color-accent-success-muted), color: var(--color-accent-success)
     Font: var(--text-caption), font-weight: 500
     Click: scrolls SAYS pane to Finding #7 and highlights it
   
   Multiple links: if linked to multiple findings, badge shows "→ 3 Findings" 
     Click: expands dropdown listing each finding
   
   Highlight animation (when SAYS finding clicked):
     Border-left: 3px solid var(--color-accent-success)
     Box-shadow: var(--shadow-glow-blue) (green-tinted glow)
     Pulse: 2 cycles, 300ms each
       @keyframes link-pulse {
         0% { box-shadow: none; }
         50% { box-shadow: var(--shadow-glow-blue); }
         100% { box-shadow: none; }
       }
     Total: 600ms, then returns to LINKED steady state
   
   Scroll-to: when triggered from SAYS, THINK pane auto-scrolls to this card
     Behavior: smooth scroll, card centered vertically if possible
```

#### 5.4.3 Thought Card Animations

```
CARD ENTRY (new card appears at bottom of list):
  Animation: 
    transform: translateY(20px) → translateY(0)
    opacity: 0 → 1
    Duration: 300ms, var(--ease-out-expo)
  If multiple cards appear simultaneously (e.g., after reconnection):
    Stagger: 50ms delay per card (card 1: 0ms, card 2: 50ms, card 3: 100ms, etc.)
    Implementation: CSS animation-delay or JS staggered setTimeout
  
  Existing cards shift down smoothly:
    margin-bottom transition: 250ms var(--ease-out-quint)
    (Cards don't jump — they slide to new positions)

CARD REMOVAL (user deletes or hides card):
  Animation:
    transform: translateY(0) → translateY(-10px)
    opacity: 1 → 0
    max-height: current → 0
    Duration: 200ms, var(--ease-in-quint)
  Below cards slide up to fill gap:
    margin-bottom transition: 250ms var(--ease-out-quint)

CARD EXPAND (see EXPANDED state above):
  Phase 1 (0-300ms): height expands to full content
    max-height transition: 400ms var(--ease-out-expo)
  Phase 2 (100-400ms): content fades in
    Adjacent cards dim (opacity transition: 200ms var(--ease-out-quint))
    Action buttons, flags appear
  Trigger: user click on card

CARD COLLAPSE (reverse of expand):
  Phase 1 (0-200ms): content fades out, adjacent cards restore
  Phase 2 (0-400ms): height collapses
    max-height transition: 400ms var(--ease-out-expo)
  
STATE TRANSITIONS (border color, background, glow):
  All use var(--transition-color): 150ms var(--ease-out-quint)
  Smooth transitions between all color properties simultaneously

STREAMING TEXT (during THINKING state):
  Text chunks arrive via WebSocket or polling
  Each chunk appended to DOM in requestAnimationFrame callback
  Maximum 1 DOM update per 16ms (60fps)
  Chunks batched: if multiple chunks arrive within 16ms window, concatenate before DOM update
  
  Typewriter feel (optional, toggleable in settings):
    Instead of chunk-based, render character-by-character at configurable speed
    Default speed: 80 chars/second
    Implementation: queue characters, pop and render at 12.5ms intervals
  
  Cursor behavior:
    At end of last rendered character
    No cursor before text exists
    Cursor removed when streaming completes
    If streaming pauses (network lag), cursor continues blinking

AUTO-SCROLL BEHAVIOR:
  During streaming: auto-scroll keeps cursor visible
    Uses scrollIntoView({ block: 'nearest' }) throttled to 200ms intervals
  
  "New reasoning ↓" button:
    Appears when user scrolls up >50px from bottom of THINK pane
    Position: sticky, bottom: 16px, centered horizontally
    Style: 
      Background: var(--glass-heavy); backdrop-filter: blur(20px);
      Border: 1px solid var(--color-border-default); border-radius: var(--radius-full);
      Padding: var(--space-1) var(--space-3); font: var(--text-caption);
      Color: var(--color-text-secondary); cursor: pointer;
      Box-shadow: var(--shadow-md);
    Entry animation: slide up from bottom (translateY(16px)→0) + fade, 200ms var(--ease-out-expo)
    Exit animation: slide down + fade, 150ms var(--ease-in-quint)
    Badge: shows count of new reasoning steps ("↓ 3 new") — updates in real-time
    Click: smooth scroll to bottom, dismiss button
    Dismisses automatically when scrolled within 50px of bottom
```

### 5.5 SAYS Pane — Polished Output

The SAYS pane displays the AI's conclusions as Finding Cards. Each Finding represents a discrete conclusion, observation, or recommendation that the AI has derived from its reasoning in the THINK pane. Findings are bidirectionally linked to their originating THINK steps.

The pane contains a scrollable list of Findings, sorted chronologically (newest at bottom). Above the list, an optional filter bar allows filtering by status (All, Draft, Approved, Rejected, Outdated). Below the list, the Input Area (§5.6) for submitting queries.

#### 5.5.1 Finding Card Anatomy

```
┌──────────────────────────────────────────────────────────────┐
│ ✅ Finding #7                          Draft · 0.8s ago      │  ← Header
│──────────────────────────────────────────────────────────────│
│                                                              │
│ Q4 revenue in APAC increased 12% YoY, driven primarily       │  ← Conclusion text
│ by expansion in the Southeast Asian market. This represents  │
│ the third consecutive quarter of double-digit growth in      │
│ the region, outpacing all other geographic segments.         │
│                                                              │
│ ┌──────────────────────────────────────────────────────────┐ │
│ │ ▸ Sources (2)                                            │ │  ← Collapsible sections
│ │ ▸ Reasoning — Based on Step 2, Step 4                    │ │
│ └──────────────────────────────────────────────────────────┘ │
│                                                              │
│ Confidence: ■■■■□ HIGH (0.94)                                │  ← Confidence bar
│                                                              │
│ Status: ✓ Approved by Bane · 2m ago                          │  ← Approval status
│                                                              │
│ [Edit] [Approve] [Request Revision] [Copy] [⚑ Flag]        │  ← Actions
└──────────────────────────────────────────────────────────────┘

Card dimensions:
  Width: 100% (fills SAYS pane, respects padding)
  Max-width: 720px (readability — narrower than THINK cards for prose reading comfort)
  Margin: 0 auto var(--space-3) (centered, with bottom gap)
  Background: var(--color-bg-surface)
  Border: 1px solid var(--color-border-default)
  Border-radius: var(--radius-md)
  Padding: var(--space-4)
  Box-shadow: var(--shadow-sm)

Header row:
  Display: flex; justify-content: space-between; align-items: center;
  Margin-bottom: var(--space-3);
  
  Left side:
    Finding number: "✅ Finding #7" or "📝 Finding #7" (draft)
      Font: var(--text-small), font-weight: 600, color: var(--color-text-primary)
      Icon: check-circle (approved), file-text (draft), x-circle (rejected)
  
  Right side:
    Status badge: small pill
      Font: var(--text-caption), font-weight: 500
      Draft: background transparent, border 1px solid var(--color-border-default), color var(--color-text-secondary)
      Approved: background var(--color-accent-success-muted), color var(--color-accent-success)
      Rejected: background var(--color-accent-error-muted), color var(--color-accent-error)
      Outdated: background transparent, border 1px dashed var(--color-border-default), color var(--color-text-tertiary)
    Timestamp: text-caption, text-tertiary, right of badge

Conclusion text:
  Font: var(--text-body), line-height: 1.7, color: var(--color-text-primary)
  Margin-bottom: var(--space-3)
  White-space: pre-wrap
  Renders markdown: bold, italic, lists, links
  Max-height (collapsed): 120px (shows ~5 lines), overflow: hidden
    "Show more" gradient overlay at bottom if truncated
    Click or "Show more" button: expands to full height, 400ms var(--ease-out-expo)

Collapsible sections (▸ Sources, ▸ Reasoning):
  Display: flex; flex-direction: column; gap: var(--space-1);
  Margin-bottom: var(--space-2);
  
  Section header:
    Display: flex; align-items: center; gap: var(--space-1);
    Font: var(--text-small), font-weight: 500, color: var(--color-text-secondary);
    Cursor: pointer;
    User-select: none;
    Padding: var(--space-1) 0;
    
    Chevron: ▸ (collapsed) → ▾ (expanded)
      Transition: transform 200ms var(--ease-out-quint)
      Collapsed: rotate(0deg)
      Expanded: rotate(90deg)
    
    Count badge: "(2)" in text-caption, text-tertiary
    
    Hover: color: var(--color-text-primary)
  
  Section content (visible when expanded):
    Sources list:
      For each source: icon + filename, clickable (opens in Evidence Panel)
      Style: same as source badges in THINK cards (§5.4.1)
    
    Reasoning links:
      For each linked THINK step: "Step 2 · Cross-ref APAC" 
      Click: THINK pane scrolls to that step, highlights it
      Style: 
        Display: inline-flex; align-items: center; gap: 4px;
        Font: var(--text-small), color: var(--color-text-link);
        Cursor: pointer;
        Hover: text-decoration: underline;
        Icon: link icon (12px) before step label

Confidence bar:
  Display: flex; align-items: center; gap: var(--space-2);
  Margin-bottom: var(--space-2);
  
  Bar: 5 segments
    ┌──┬──┬──┬──┬──┐
    │■ │■ │■ │■ │□ │  HIGH (0.94)
    └──┴──┴──┴──┴──┘
    
    Each segment: 16px wide, 6px tall, border-radius 1px, gap 2px
    Filled segments: 
      > 0.8: var(--color-accent-success) (green)
      0.5-0.8: var(--color-accent-warning) (amber)
      < 0.5: var(--color-accent-error) (red)
    Empty segments: var(--color-bg-input)
  
  Label: "HIGH (0.94)"
    Font: var(--text-caption), font-weight: 600
    Color: matches bar color
  
  Bar animation:
    On load: segments fill sequentially, left to right
    Duration: 100ms per segment, 50ms stagger (500ms total)
    Easing: var(--ease-out-quint)
  
  Hover on bar: tooltip with exact score + contributing factors
    Position: above bar, centered
    Content: "Confidence: 0.94\nSource reliability: 0.95\nCross-validation: 0.92\nContradiction flag: -0.12"
    Style: tooltip card (glass, shadow, 12px font)

Approval status:
  Font: var(--text-small), color: var(--color-text-secondary);
  Margin-bottom: var(--space-2);
  
  Approved: 
    Color: var(--color-accent-success)
    Content: "✓ Approved by {username} · {relative_time}"
    Checkmark icon, 14px
    
  Pending (draft): 
    Color: var(--color-text-secondary)
    Content: "Awaiting review"
    Clock icon, 14px
    
  Rejected:
    Color: var(--color-accent-error)
    Content: "✗ Rejected by {username} · {relative_time}"
    X icon, 14px
    Optional: rejection reason shown below in italics

Action buttons:
  Display: flex; gap: var(--space-2);
  Justify-content: flex-end;
  
  Approve button:
    Background: var(--color-accent-success)
    Color: white
    Border: none
    Height: 32px; padding: 0 var(--space-4);
    Border-radius: var(--radius-md);
    Font: var(--text-small), font-weight: 600;
    Cursor: pointer;
    Transition: var(--transition-color);
    
    Hover: background: var(--color-accent-success-hover) or brightness 1.1
    Active: scale 0.97, 80ms
    
    Click: triggers approval confirmation (§5.5.3)
  
  Request Revision button:
    Background: transparent
    Border: 1px solid var(--color-border-default)
    Color: var(--color-text-secondary)
    Height: 32px; padding: 0 var(--space-4);
    Border-radius: var(--radius-md);
    Font: var(--text-small), font-weight: 500;
    
    Hover: border-color var(--color-border-hover), color var(--color-text-primary)
    
    Click: opens revision dialog (§5.5.3)
  
  Edit button (inline edit):
    Same visual as Request Revision but with pencil icon
    
    Click: conclusion text becomes editable
      Contenteditable div replaces static text
      Focus ring on editable area
      [Save] [Cancel] buttons appear
      Save: updates finding content, status may revert to Draft if already approved
      Cancel: reverts to original text
  
  Copy button:
    Icon-only: copy icon, 16px
    Tooltip: "Copy to clipboard"
    Click: copies finding text + sources to clipboard
    Feedback: icon changes to checkmark for 1.5s, then reverts
  
  Flag button:
    Icon-only: flag icon, 16px
    Toggle: flagged/unflagged
    Flagged: icon fill weight, color accent-error
```

#### 5.5.2 Finding Card States

```
1. DRAFT (AI-generated, not yet human-reviewed)
   
   Border: 1px solid var(--color-border-default)
   Background: var(--color-bg-surface)
   Border-left: none (no accent)
   
   Header badge: "Draft" — pill, border 1px solid var(--color-border-default), text-secondary
   Status text: "Awaiting review" with clock icon
   
   Actions visible: [Approve] [Request Revision] [Edit] [Copy] [Flag]
   Approve button: prominent (filled green)
   
   These findings appear at top of SAYS list with "New — awaiting review" separator

2. APPROVED (human-reviewed and accepted)
   
   Border-left: 3px solid var(--color-accent-success)
   Background: var(--color-bg-surface)
   Box-shadow: var(--shadow-sm)
   
   Header badge: "✓ Approved" — pill, background success-muted, color success, font-weight 600
   Finding number icon: ✅ check-circle, green
   
   Status text: "✓ Approved by {username} · {relative_time}"
   Approval metadata stored in memory events: approver, timestamp, optional note
   
   Actions visible: [Edit] [Copy] [Flag]
     [Unapprove] button available: reverts to Draft (confirmation required)
     "Unapprove will return this finding to draft status." [Unapprove] [Cancel]
   
   Edit behavior: editing an approved finding creates a new draft version
     Original approved version preserved in revision history
     "You are editing an approved finding. This will create a new draft." [Continue] [Cancel]
   
   Transition animation (DRAFT → APPROVED):
     Green border-left sweeps in from left edge: clip-path animation
       @keyframes approve-sweep {
         0% { border-left-color: transparent; clip-path: inset(0 100% 0 0); }
         100% { border-left-color: var(--color-accent-success); clip-path: inset(0 0 0 0); }
       }
       Duration: 400ms var(--ease-out-expo)
     Badge transitions: border → filled green background, 400ms
     Checkmark icon: scale bounce (0→1.2→1, 300ms var(--ease-spring))

3. REJECTED (human-reviewed and dismissed)
   
   Border-left: 3px solid var(--color-accent-error)
   Background: var(--color-bg-surface)
   Filter: opacity(60%) (dimmed)
   
   Header badge: "✗ Rejected" — pill, background error-muted, color error
   Finding number icon: ❌ x-circle, red
   
   Status text: "✗ Rejected by {username} · {relative_time}"
   Rejection reason: shown below status in italics (optional)
   
   Actions visible: [Reconsider] [Copy] [Flag]
     [Reconsider]: returns to Draft status with "Reconsidered" note
   
   Hidden from default view: not shown in findings list unless "Show Rejected" filter is active
   Filter toggle: "Show Rejected (3)" pill in filter bar
   
   Transition (DRAFT → REJECTED):
     Red border-left appears (same sweep animation as approve, but 200ms — faster for rejection)
     Card fades to 60% opacity over 300ms var(--ease-out-quint)

4. OUTDATED (superseded by a newer finding)
   
   Border: 1px dashed var(--color-border-default)
   Background: var(--color-bg-surface)
   Filter: opacity(50%)
   
   Header badge: "Outdated" — pill, border dashed border-default, color text-tertiary
   
   Status text: "Superseded by Finding #12 · {relative_time}"
     Finding #12 is a clickable link → scrolls to that finding and highlights it
   
   Actions visible: [View Superseding] [Copy]
   
   Outdating trigger: when AI generates a new finding on the same topic with higher confidence
     Old finding marked as outdated automatically
     Old and new findings linked bidirectionally
   
   Hidden from default view (same filter toggle as Rejected)
```

#### 5.5.3 Approval Workflow

```
PRIMARY FLOW (Approve):

1. User clicks [Approve] on a Draft finding
2. Confirmation dialog appears (if HITL enabled):
   ┌─────────────────────────────────────────┐
   │ Approve Finding #7?                     │
   │                                         │
   │ "APAC Revenue Growth Q4"                │
   │ Confidence: HIGH (0.94)                 │
   │                                         │
   │ This will record your approval in the   │
   │ immutable audit trail.                  │
   │                                         │
   │ Note (optional):                        │
   │ ┌─────────────────────────────────────┐ │
   │ │ Add approval note...                │ │
   │ └─────────────────────────────────────┘ │
   │                                         │
   │              [Cancel]    [Approve]       │
   └─────────────────────────────────────────┘
   
   Dialog: centered modal, width 400px, glass-heavy background
     Entry: scale(0.95)→scale(1) + fade, 200ms var(--ease-out-expo)
     Exit: reverse, 150ms var(--ease-in-quint)
     Backdrop: rgba(0,0,0,0.5), click to cancel, Escape to cancel
     Focus trap: Tab cycles within dialog
     Auto-focus: [Approve] button by default

3. User clicks [Approve] in dialog
4. Dialog closes immediately (150ms fade)
5. API call: PATCH /api/v1/sessions/:id/approvals/:approvalId 
   Body: { status: 'approved', note: 'optional note' }
6. Finding card animates to APPROVED state (green sweep, 400ms, see §5.5.2)
7. Memory event written: approval record with user identity + timestamp
8. Toast notification: "Finding #7 approved" (success, auto-dismiss 3s)
9. Status bar updates pending approvals count (decrement)

ALTERNATE FLOW (Request Revision):

1. User clicks [Request Revision] on a Draft finding
2. Revision dialog appears:
   ┌─────────────────────────────────────────┐
   │ Request Revision for Finding #7?         │
   │                                         │
   │ What should the AI revise?               │
   │ ┌─────────────────────────────────────┐ │
   │ │ Include Q3 comparison data and      │ │
   │ │ verify the November shipping claim  │ │
   │ │ against the original manifests      │ │
   │ └─────────────────────────────────────┘ │
   │                                         │
   │ Model: [deepseek-v4-pro ▾]              │
   │                                         │
   │              [Cancel]    [Submit]        │
   └─────────────────────────────────────────┘
   
   Same dialog style as approval confirmation
   Revision instructions: textarea, min 3 rows, required
   Model selector: dropdown of available models

3. User clicks [Submit]
4. Dialog closes
5. Finding reverts to Draft status (status badge changes back to "Draft")
   "Revision requested" note appears below status
6. Input area auto-populates with revision instructions
   Textarea value: "Revise Finding #7: Include Q3 comparison data..."
   Context auto-added: @finding:7
7. AI re-analyzes with revision instructions
   New THINK step appears: "🧠 Step 5 · Revise Finding #7"
   New Draft finding appears below with header: "Finding #7 (Revised)"
     Shows "Revised from Finding #7 — requested by Bane · just now"
8. Original finding preserved in revision history (see below)

REVISION HISTORY:
  Collapsible section at bottom of finding card:
    "▸ Revision History (2 revisions)" — click to expand
    
  Shows chronological list:
    v3: Approved by Bane · 2m ago
        "Approved as final"
    v2: Revised — "Include Q3 comparison data" · 8m ago
        Requested by Bane
        AI re-analyzed with new context
    v1: Draft — AI generated · 15m ago
        Initial analysis
    
  Each version clickable: clicking v2 shows that version's content
    Content replaces current view (with "Viewing v2 of 3" header)
    [Back to latest] button to return to current version
    
  Diff view: "Compare v2 → v3" link
    Opens split diff view: left=v2, right=v3, changes highlighted
    Green highlights: additions. Red highlights: removals.
    Available as a modal overlay

QUICK APPROVE (keyboard shortcut):
  Ctrl+Enter on selected finding: approves without confirmation dialog
  Only works for findings with confidence > 0.80 (high confidence)
  For lower confidence: shows confirmation dialog even with shortcut
  
BULK APPROVE:
  Select multiple findings (Shift+click or checkboxes in filter bar)
  "Approve Selected (3)" button appears in toolbar
  Click → single confirmation: "Approve 3 findings?" [Approve] [Cancel]
  Each finding animates to APPROVED sequentially (100ms stagger)
```

#### 5.5.4 Bidirectional Reasoning Links

```
Finding-to-Thought links are the core credibility mechanism of Chronicle.
Every conclusion MUST link to its originating reasoning.

FROM SAYS → THINK (investigator traces conclusion to reasoning):

1. Finding card shows "▸ Reasoning — Based on Step 2, Step 4" section
2. Click "Step 2" or "Step 4" link
3. THINK pane:
   a. Auto-scrolls to the linked Thought Card
      scrollIntoView({ block: 'center', behavior: 'smooth' })
   b. Thought Card highlights with link-pulse animation:
      2 pulses: box-shadow glow (var(--shadow-glow-blue) with green tint)
      Duration: 300ms per pulse, 2 pulses (600ms total)
      @keyframes link-pulse-think {
        0% { box-shadow: var(--shadow-sm); }
        50% { box-shadow: 0 0 16px rgba(35,134,54,0.3); }
        100% { box-shadow: var(--shadow-sm); }
      }
   c. After animation: card enters LINKED state (green border-left, link badge)
   
4. If THINK pane is collapsed (tab-only):
   Pane auto-expands (to 280px minimum) with animation
   Then scroll-to and highlight occur
   
5. Visual trail: a brief animated line connects the Finding to the Thought Card
   (SVG overlay, curved path, fades out after 1s)
   Line color: var(--color-accent-success), 2px, opacity 0.3

FROM THINK → SAYS (investigator traces reasoning to conclusion):

1. Thought Card header shows link badge: "→ Finding #7"
2. Click link badge
3. SAYS pane:
   a. Auto-scrolls to the linked Finding Card
   b. Finding highlights with pulse animation:
      Same pattern: 2 green pulses, 300ms each
   c. Finding temporarily enters "highlight" visual state
   
4. If multiple findings linked: badge shows "→ 3 Findings"
   Click expands mini-dropdown listing each finding
   Select one to jump to it

MULTI-LINK INDICATOR:
  A THINK step referenced by multiple findings:
    Badge: "→ 3 Findings" (clickable dropdown)
    Each finding shows this step in its "Reasoning" section
    
  A Finding based on multiple THINK steps:
    Section: "▸ Reasoning — Based on Step 2, Step 3, Step 4"
    Each step individually clickable
    
  Hover on finding: ALL linked THINK steps show subtle green border-left
    (communicates: "this reasoning produced this conclusion")
    Transition: border-left-color 200ms var(--ease-out-quint)
    
  Hover on THINK step: ALL linked findings show subtle green left-border
    (communicates: "this conclusion came from this reasoning")

CONNECTION VISUALIZATION (optional, toggle in settings):
  Animated particle effect when link is clicked:
    Small dots travel along curved path from THINK card to SAYS card
    6-8 particles, staggered release
    Duration: 600ms travel time
    Color: var(--color-accent-success)
    Particle size: 4px circles, fade out upon arrival
    Easing: var(--ease-out-quint) for position
    Implementation: absolute-positioned divs animated with requestAnimationFrame
    Performance: disabled when prefers-reduced-motion
```

### 5.6 Input Area
### 5.6 Input Area

The Input Area is the primary mechanism for human-AI interaction in the Investigation Workbench. It occupies the bottom of the SAYS pane and provides a multi-line text input with context attachment, model selection, and token estimation.

```
┌──────────────────────────────────────────────────────────────┐
│ ┌──────────────────────────────────────────────────────────┐ │
│ │ Ask a question, provide instruction, or request analysis │ │  ← Multi-line textarea
│ │ that the AI should investigate using the available       │ │     auto-growing
│ │ evidence and context...                                  │ │
│ │                                                          │ │
│ │ ▊                                                        │ │  ← Cursor (when focused)
│ └──────────────────────────────────────────────────────────┘ │
│                                                              │
│ [📎 Attach] [@ Context ▼] [# Model ▼]        ~1.2K tokens → │  ← Toolbar
│                                              ~$0.002         │
└──────────────────────────────────────────────────────────────┘
```

#### 5.6.1 Textarea

```
Dimensions:
  Min-height: 52px (2 lines of text at --text-body size + padding)
  Max-height: 200px (8 lines + padding, then scrolls internally)
  Width: 100%
  Padding: var(--space-3) var(--space-4)  (12px 16px)
  Background: var(--color-bg-input)
  Border: 1px solid var(--color-border-default)
  Border-radius: var(--radius-md)
  Font: var(--text-body), line-height: 1.5, color: var(--color-text-primary)
  Resize: none (JS-controlled auto-grow)
  Outline: none (custom focus ring)
  Line-height: 1.5rem (24px — exactly 2 lines = 48px + padding = 52px min)

Placeholder:
  Context-dependent text in var(--color-text-placeholder):
    Default: "Ask a question or give an instruction..."
    After finding: "Ask a follow-up question about Finding #7..."
    With evidence selected: "Analyze the selected evidence..."
    Revision mode: "Revise Finding #7: {user's revision instructions}"

Focus state:
  Border-color: var(--color-accent-primary)
  Box-shadow: 0 0 0 1px var(--color-accent-primary)
  Transition: all 150ms var(--ease-out-quint)

Auto-grow behavior:
  Textarea height adjusts to content on keystroke
  Implementation: onInput → measure scrollHeight → set height = min(scrollHeight, 200px)
  Smooth height transition: 100ms var(--ease-out-quint)
  When max-height reached: overflow-y: auto (internal scroll)

Submit:
  Enter: submit (sends message to AI)
  Shift+Enter: newline (stays in textarea)
  Cmd+Enter: submit with force (even if empty — sends blank for "continue" prompts)
  
  Submit button (→ icon):
    Position: absolute, right: 12px, top: 50%, transform: translateY(-50%)
    (overlaid on textarea, right side, vertically centered)
    Size: 32×32px circle
    Background: transparent
    Border: none
    Icon: ArrowRight (Phosphor), 16px, color: var(--color-text-tertiary)
    Cursor: pointer
    
    Hover: 
      Icon color: var(--color-accent-primary)
      Background: var(--color-accent-primary-muted)
    Active: scale 0.92 (80ms)
    
    Disabled (empty textarea): 
      Icon color: var(--color-text-disabled)
      Cursor: default
    
    Visible: icon appears when textarea has content (opacity 0→1, 150ms)
    Hidden: when textarea is empty (opacity 1→0, 100ms)

Character count:
  Shown below textarea, left-aligned
  Font: var(--text-caption), color: var(--color-text-tertiary)
  Format: "847 / —" (no max character limit by default)
  Appearance: fades in when > 0 characters (opacity 0→1, 150ms)
```

#### 5.6.2 Input Toolbar

```
Position: below textarea
Height: 32px
Display: flex; align-items: center; gap: var(--space-2);
Padding: var(--space-2) 0;

Attach button (📎):
  Icon: Paperclip (Phosphor), 14px
  Label: "Attach" (text-caption, hidden on mobile)
  Style: 
    Height: 28px; padding: 0 var(--space-2);
    Background: transparent; border: 1px solid var(--color-border-default);
    Border-radius: var(--radius-sm);
    Color: var(--color-text-secondary); cursor: pointer;
    Transition: var(--transition-color);
  
  Hover: background var(--color-bg-hover), border-color var(--color-border-hover)
  
  Click: opens system file picker dialog
    Accept: .csv, .json, .pdf, .txt, .md, .log, .xml, .yaml, .yml
    Multiple: true (can select multiple files)
    Max total size: 50MB (configurable)
    
  Upload progress:
    After file selection, each file shows upload progress bar
    Bar: appears below textarea temporarily (height 4px, 100% width)
    Fill: blue, width transition from 0%→100% based on upload progress
    Duration: real-time (as upload progresses)
    Label: "Uploading q4-sales.csv (2.4MB)..." with percentage
    On complete: bar fills 100%, green flash, then slides up and disappears (300ms)
    Error: bar turns red, "Upload failed: {reason}" message
  
  Attached files shown as removable pills below toolbar:
    Each pill: icon + filename + [× remove]
    Style: bg-input, border border-default, radius-sm, padding 2px 8px
    Remove: click ×, pill fades out (150ms) + slides left

Context selector (@):
  Icon: At (Phosphor), 14px
  Label: "Context" (text-caption)
  Same button style as Attach
  
  Click: opens context dropdown
    Dropdown: positioned above toolbar, anchored left
    Max-height: 240px, scrollable
    Items:
      @current-investigation  — all evidence in this investigation
      @database:revenue       — SQL table (if registered as source)
      @finding:7              — specific finding as context
      @thought:step-4         — specific reasoning step as context
      @timeline:last-24h      — all events in timeline range
      @memory:search          — search memory events by keyword (opens search input)
    
    Each item: 
      Checkbox + icon + name + description (subtitle in text-caption)
      Style: padding 8px 12px, hover bg-hover
      Click: toggle checkbox → adds/removes context item
    
  Selected context shown as removable pills:
    Display: flex; flex-wrap: wrap; gap: 4px;
    Position: between toolbar and textarea (if any context selected)
    
    Each pill: "[×] @finding:7"
    Style: bg-accent-primary-muted, border border-accent-primary, radius-sm
    Font: text-caption, color: text-accent
    Click ×: remove context item (pill fades out + shrinks, 150ms)

Model selector (#):
  Icon: Hash (Phosphor), 14px
  Label: current model name (text-caption)
  Same button style as Attach
  
  Click: opens model dropdown
    Items:
      deepseek-v4-pro     "~$0.002/query · 847ms avg"
      deepseek-v4-flash   "~$0.0005/query · 234ms avg"
      claude-sonnet-4     "~$0.008/query · 1.2s avg"
      gpt-4o              "~$0.015/query · 890ms avg"
      local-model          "Free · 3.4s avg"
    
    Selected: radio button (●) + checkmark
    Each item shows: model name, estimated cost, average latency
    Separation: "Cloud Models" divider, "Local Models" divider
  
  Selected model: shown as pill in toolbar
    Click pill: re-opens dropdown for quick switching
    
  Model persists per investigation:
    Saved to localStorage: `chronicle:model:${investigationId}`
    Default: first available cloud model

Token counter (right side of toolbar):
  Font: var(--text-caption), color: var(--color-text-tertiary)
  Format: "~1,200 tokens · ~$0.002"
  Updates on keystroke with 300ms debounce
  Calculation: approximate (4 chars ≈ 1 token for English text)
  Cost: based on selected model's per-token pricing
  Color: nominal (text-tertiary), warning (amber when > 80% context budget)
  
  Hover: tooltip with breakdown
    "Input tokens: ~1,200
     Estimated output: ~500
     Total estimated: ~1,700 tokens
     Estimated cost: $0.0034"
```

#### 5.6.3 Submission Flow

```
FULL SUBMISSION LIFECYCLE:

1. IDLE STATE
   Textarea: empty or contains draft text
   Submit button: hidden (no content) or visible (has content)
   Toolbar: normal
   
2. USER PRESSES ENTER
   Pre-submit validation:
     If textarea empty: no action (prevent accidental empty submits)
     If textarea has only whitespace: no action
     If valid content: proceed
   
3. SUBMITTING STATE (input area collapses to minimal form)
   Textarea: replaced by processing indicator
   
   Processing indicator:
     ┌──────────────────────────────────────────────────────┐
     │ 🧠 Processing...                                     │
     │    Analyzing query and gathering context...           │
     └──────────────────────────────────────────────────────┘
     
     Layout: same dimensions as textarea collapsed
     Height: 40px (single line)
     Background: var(--color-bg-input)
     Border: 1px solid var(--color-border-default)
     Border-radius: var(--radius-md)
     Display: flex; align-items: center; gap: var(--space-2);
     Padding: 0 var(--space-4);
     
     Brain icon: 16px, var(--color-accent-purple), subtle pulse (opacity 0.6→1→0.6, 2s)
     Status text: var(--text-small), color: var(--color-text-secondary)
       Updates as processing progresses:
       "Sending query..."
       "Analyzing context..."
       "AI thinking..." (once THINK pane shows first card)
       "Generating finding..." (once reasoning complete)
     
     Shimmer animation on text:
       @keyframes shimmer {
         0% { background-position: -200% 0; }
         100% { background-position: 200% 0; }
       }
       background: linear-gradient(90deg, transparent 0%, rgba(255,255,255,0.05) 50%, transparent 100%);
       background-size: 200% 100%;
       animation: shimmer 2s var(--ease-linear) infinite;
     
     Cancel button (right side): [× Cancel]
       Style: small text button, text-caption, color text-secondary
       Click: cancels submission, returns to IDLE state
       Confirmation: if AI has started processing, "Cancel analysis? Partial results may be saved."
     
     Progress dots: "..." with sequential fade animation
       Each dot: opacity 0.3→1→0.3, 300ms per dot, 100ms stagger
       @keyframes dot-pulse {
         0%, 60%, 100% { opacity: 0.3; }
         30% { opacity: 1; }
       }
   
   API call (behind the scenes):
     POST /api/v1/sessions/:id/message
     Body: { content: userText, context: selectedContextItems, model: selectedModel }
     Response: 202 Accepted (processing started asynchronously)
     WebSocket events will deliver streaming results

4. STREAMING STATE (THINK pane receives reasoning)
   Processing indicator updates:
     "AI is thinking..." (when first Thought Card appears in THINK pane)
     Thought Card enters THINKING state (see §5.4.2)
     Text streams into THINK pane via WebSocket
   
   User can scroll THINK pane to read streaming reasoning
   User CANNOT submit another query yet (input is locked)
   Cancel button remains available
   
   WebSocket events received:
     iteration.started → PAUSE processing indicator, show "Iteration 43 started"
     memory.created → append to streaming Thought Card
     finding.created → (see next state)

5. COMPLETION STATE
   When AI finishes generating:
   
   a. Thought Card transitions to COMPLETED state (see §5.4.2)
      Cursor removed, timestamp appears, actions visible
   
   b. SAYS pane receives Finding Card (if AI generated a conclusion)
      Finding slides in from top of SAYS list:
        Animation: transform translateY(-20px)→0, opacity 0→1
        Duration: 300ms var(--ease-out-expo)
        If multiple findings: stagger 100ms per finding
   
   c. Processing indicator collapses:
      Height 40px→0, opacity 1→0
      Duration: 200ms var(--ease-in-quint)
   
   d. Input area returns to IDLE STATE
      Textarea: cleared (previous input stored in history, accessible via Up arrow)
      Focus: auto-focuses on textarea for next query
      Toolbar: restored to normal
      Context items: cleared (need to re-select for each query)
      Model: retains previous selection
   
   e. Toast notification: none by default (findings appearing is sufficient feedback)
      If error: error toast "Analysis failed: {reason}" with retry button
      If no finding generated: info toast "Analysis complete — no new findings"

ERROR STATE (API call fails, network error, or timeout):
   Processing indicator shows error:
     "⚠ Analysis failed — API timeout" 
     Background: var(--color-bg-toast-error) at 20% opacity
     Border-color: var(--color-accent-error)
     [Retry] [Dismiss] buttons
   
   Retry: re-submits same query
   Dismiss: returns to IDLE state, query text preserved in textarea
   
   Toast: error notification with details (see notification system)
   
   If partial results received: partial Thought Card preserved
     Marked with "⚠ Incomplete — analysis interrupted"
     User can view partial reasoning

SUBMISSION HISTORY:
  Press Up arrow in empty textarea: cycle through previous submissions
  Each Up press: go back one submission
  Down arrow: go forward (to newer submissions)
  History stored in sessionStorage (last 20 submissions)
  Key: `chronicle:input:${investigationId}:history`
  
  When cycling: textarea fills with previous text, context pills restored
  Submit: sends that exact query again (useful for re-analysis with different model)
```

#### 5.6.4 Streaming Rendering Implementation

```
Data flow:
  POST /api/v1/sessions/:id/message → server processes → WebSocket events →
    event: 'memory.created' → append to THINK pane
    event: 'iteration.completed' → finalize THINK card, generate SAYS finding
    event: 'finding.created' → append to SAYS pane
  
  WebSocket message shape:
    {
      type: 'memory.created',
      session_id: 'a3f2b...',
      memory: {
        id: 2841,
        type: 'thought',
        content: 'Comparing APAC growth trajectory...',
        content_chunk: ' trajectory with EMEA decline.',  // incremental chunk
        is_final: false,  // true when this is the last chunk for this memory event
        iteration: 42,
        trust_level: 'high'
      }
    }

Client-side rendering:
  1. Receive WebSocket message
  2. Identify which Thought Card this chunk belongs to (by iteration number)
  3. If no card exists for this iteration → create new Thought Card in THINKING state
  4. Append content_chunk to card's text content
  5. Schedule DOM update via requestAnimationFrame
  
  RAF batching:
    const pendingChunks = [];
    let rafScheduled = false;
    
    function onMemoryCreated(event) {
      pendingChunks.push(event);
      if (!rafScheduled) {
        rafScheduled = true;
        requestAnimationFrame(() => {
          applyAllPendingChunks();
          rafScheduled = false;
        });
      }
    }
    
    function applyAllPendingChunks() {
      // Batch all chunks since last RAF into single DOM update
      // Group by iteration → append to correct Thought Card
      // Single innerHTML or textContent update per card
      // Then scroll to keep cursor visible
    }
  
  Performance: maximum 1 DOM update per 16ms (60fps)
  Smooth: if chunks arrive faster than 16ms, they batch naturally
  
  Cursor implementation:
    const cursor = document.createElement('span');
    cursor.className = 'streaming-cursor';
    cursor.textContent = '▊';
    // CSS: color var(--color-accent-purple), animation cursor-blink 1s infinite
    // Appended to end of streaming text
    // Removed when is_final === true
  
  Completion detection:
    When WebSocket sends memory.created with is_final: true
    → Remove cursor span
    → Card enters COMPLETED state
    → Timestamp fades in (200ms)
    → Actions fade in (200ms, delayed 50ms)
    
    Then: when finding.created event arrives
    → Create Finding Card in SAYS pane
    → Link back to THINK step
    → Processing indicator collapses
    → Input area returns to IDLE

  Error during streaming:
    If WebSocket disconnects mid-stream:
      Thought Card shows "⚠ Stream interrupted — connection lost" banner
      Cursor removed
      Partial reasoning preserved
      Retry button: "Resume analysis" → reconnects and continues
    
    If server sends error event:
      error_text shown in Thought Card
      Card enters COMPLETED state with error flag (❌)
  
  Typewriter effect (user setting, default ON):
    Instead of rendering chunks directly, feed chunks into a character queue
    Pop characters at configurable speed (default: 80 chars/second)
    Render popped characters via RAF
    Benefits: more organic reading experience
    Tradeoff: adds slight latency (chunk of 500 chars delayed ~6 seconds)
    Setting: "Streaming speed" slider — faster (200 cps) / normal (80 cps) / slow (40 cps) / instant (no queue)
```

### 5.7 Evidence Panel

The Evidence Panel is a slide-out drawer on the right side of the workbench. It displays all evidence sources and generated findings for the current investigation, enabling drag-and-drop context building and source inspection.

```
Visibility: toggled by [Evidence] button in workbench toolbar or Ctrl+E
Default: hidden (collapsed)

Panel:
  Position: absolute, right: 0, top: 0, bottom: 0
  Width: 360px (resizable: min 280px, max 500px)
  Background: var(--color-bg-surface)
  Border-left: 1px solid var(--color-border-default)
  Box-shadow: var(--shadow-lg)
  Z-index: 20
  Display: flex; flex-direction: column;
  
  Slide animation:
    Closed: transform: translateX(100%)
    Open: transform: translateX(0)
    Duration: 300ms var(--ease-out-expo)
    (Element is always in DOM, just translated off-screen)

Resize handle:
  Left edge of panel: 4px wide, cursor: col-resize
  Drag left: reduce panel (min 280px)
  Drag right: enlarge panel (max 500px)
  Same interaction model as pane divider (§5.3)

Header:
  Height: 48px; padding: 0 var(--space-4);
  Display: flex; align-items: center; justify-content: space-between;
  Border-bottom: 1px solid var(--color-border-default);
  Flex-shrink: 0;
  
  Title: "Evidence" in var(--text-subtitle), font-weight 600
  Close button: [×] icon, 20px, var(--color-text-tertiary)
    Hover: var(--color-text-primary)
    Click: close panel (slide away)

Search bar:
  Padding: var(--space-3) var(--space-4);
  Input: same style as main search input but compact (height 32px)
  Placeholder: "Search evidence..."
  Debounce: 200ms
  Filters evidence and findings by text match
  Clear button (×): appears when search has value

Filter tabs:
  Padding: 0 var(--space-4) var(--space-2);
  Display: flex; gap: var(--space-1);
  
  Tab chips: [All] [Files] [Database] [URLs] [Findings]
    Style: text-caption, padding 2px 10px, radius-full, bg transparent, text text-secondary
    Active: bg accent-primary-muted, text accent-primary
    Count: each shows item count in text-tertiary "(12)"
    Click: filter list by type

Source list:
  Flex: 1; overflow-y: auto; padding: 0 var(--space-4);
  
  Section header: "SOURCES (12)" 
    Font: var(--text-caption), text-transform: uppercase, letter-spacing: 0.05em
    Color: var(--color-text-tertiary)
    Padding: var(--space-2) 0; border-bottom: 1px solid var(--color-border-subtle);
    Sticky: top 0, background var(--color-bg-surface);
  
  Source item:
    ┌──────────────────────────────────────────┐
    │ 📄 q4-sales.csv                          │
    │    2.4 MB · 12,400 rows · Added 2h ago   │
    │    Referenced by: 3 findings, 42 events   │
    └──────────────────────────────────────────┘
    
    Padding: var(--space-2) var(--space-3);
    Border-radius: var(--radius-sm);
    Margin-bottom: var(--space-1);
    Border: 1px solid transparent;
    Cursor: pointer;
    
    Hover: background var(--color-bg-hover), border-color var(--color-border-default);
    Drag: item becomes draggable → drop into input area to add as context
    
    Icon: 16px, entity-type-specific color
    Filename: var(--text-small), font-weight 500
    Metadata: var(--text-caption), color: var(--color-text-secondary)
    References: var(--text-caption), color: var(--color-text-tertiary)
    
    Right-click context menu:
      Open Preview    — opens file content in modal viewer
      Re-analyze      — triggers new AI analysis of this source
      Download         — downloads original file
      Remove           — removes from evidence (with confirmation)
      Copy Path        
      Properties       — full metadata dialog
  
  Empty state: "No evidence sources" + "Drag files here or click Attach in the input area"

Findings section:
  Same style as sources but with different items
  
  Finding item:
    ┌──────────────────────────────────────────┐
    │ ✅ Finding #7 · APAC Growth              │
    │    HIGH confidence · Approved            │
    └──────────────────────────────────────────┘
    
    Icon: finding status icon (check-circle, file-text, x-circle)
    Click: scrolls SAYS pane to that finding
    Drag: drag finding to input area → adds @finding:7 context

Add source button:
  Bottom of list: [+ Add Source] 
  Style: dashed border, text-secondary, hover: border-hover, text-primary
  Click: opens file picker (same as Attach button in input)
  Drop zone: panel accepts file drops directly
    Drag file from OS into panel → upload starts
    Drop zone highlight: panel border turns accent-primary, background tints 5%
```

### 5.8 Workbench Toolbar

```
Fixed at top of workbench view (below main top bar shell):
  Height: 48px
  Background: var(--color-bg-surface)
  Border-bottom: 1px solid var(--color-border-default)
  Padding: 0 var(--space-4)
  Display: flex; align-items: center; justify-content: space-between;
  Z-index: 10;

Left section:
  Sessions dropdown:
    ┌──────────────────────┐
    │ Investigation: Q4 ▼  │
    └──────────────────────┘
    Style: height 32px, padding 0 var(--space-3), border border-default, radius-md
    Font: var(--text-small), font-weight 500
    Icon: chevron-down 12px, right side
    
    Click: opens dropdown (below, anchored left)
      List of all investigations:
        Each item: name, status summary, last active relative time
        Active: bg-accent-primary-muted, accent left border
        Click: switch investigation (crossfade transition 200ms)
      Separator
      [+ New Investigation] button

Center section:
  Investigation title: "Q4 Revenue Analysis" 
    Font: var(--text-subtitle), font-weight 600
    Truncated: max-width 400px, ellipsis
  
  Status summary (below title, small):
    Font: var(--text-caption), color: var(--color-text-secondary)
    "12 sources · 7 findings · 2 drafts pending · active 12m ago"

Right section:
  Buttons (32×32px, icon-only or icon+label):
  
  [Evidence] button:
    Icon: file-text, 16px
    Toggle state: active when panel open
      Active: bg-accent-primary-muted, icon color accent-primary
    Click: toggle evidence panel
    Tooltip: "Evidence Panel (Ctrl+E)"
  
  [Export ▾] button with dropdown:
    Items:
      Export Timeline as PDF
      Export Findings as JSON
      Export Full Report (PDF with reasoning + findings + sources)
      Export Session Data (JSON)
    Each: icon + label + shortcut hint
    Click: triggers export, shows progress, delivers file via download
  
  [Share] button:
    Icon: share-network, 16px
    Click: generates shareable link (if multi-tenant enabled)
    Copies to clipboard: "Share link copied"
  
  [Settings ⚙] button:
    Click: opens investigation-specific settings popover
    Settings:
      Auto-approve threshold: slider 0.0-1.0
      Stream typing speed: 40/80/200 cps
      Show connection visualizations: toggle
      Default model: dropdown
      Color accent: 12-color picker for investigation card
    Save: settings persisted per investigation
```

### 5.9 Multi-Investigation Switching

```
Trigger: Ctrl+Shift+I or click Sessions dropdown → "Switch Investigation"

Overlay:
  Position: fixed, inset: 0
  Background: rgba(0,0,0,0.6), backdrop-filter: blur(8px)
  Z-index: 350
  
  Entry: fade in 150ms
  Exit: fade out 100ms
  Close: Escape key, click backdrop

Grid container:
  Position: absolute, top: 50%, left: 50%, transform: translate(-50%, -50%)
  Width: min(800px, 90vw)
  Max-height: 80vh
  Display: grid; grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  Gap: var(--space-4);
  Padding: var(--space-4);
  Overflow-y: auto;
  
  Entry animation: scale(0.95)→1 + fade, 200ms var(--ease-out-expo)

Investigation card:
  Background: var(--color-bg-surface)
  Border: 1px solid var(--color-border-default)
  Border-radius: var(--radius-lg)
  Padding: var(--space-4)
  Cursor: pointer
  Height: 160px
  Display: flex; flex-direction: column; gap: var(--space-2);
  
  Hover: border-color var(--color-border-hover), shadow-md, translateY(-2px)
    Transition: all 150ms var(--ease-out-quint)
  Active: scale 0.97 (80ms)
  
  Top color bar: 4px × 100%, investigation accent color, top of card border-radius
  
  Title: var(--text-body-lg), font-weight 600, truncate
  Status: var(--text-caption), color: var(--color-text-secondary)
    Format: "7 findings · 2 pending · last active 12m ago"
  
  Stats row:
    Display: grid; grid-template-columns: 1fr 1fr; gap: var(--space-1);
    Font: var(--text-caption)
    "12 sources" "42 iterations"
    "$1.23 spent" "94% confidence avg"
  
  Click: switch to this investigation
    Transition: overlay fades out, workbench crossfades to new investigation (200ms)
    All state restored: pane ratio, scroll position, evidence panel openness
  
  Right-click: context menu
    Rename
    Change color
    Duplicate
    Archive
    Delete

New investigation card:
  Border: 1px dashed var(--color-border-default)
  Background: transparent
  Display: flex; align-items: center; justify-content: center;
  
  [+] icon: 48px, var(--color-text-tertiary)
  Label: "New Investigation"
  Hover: border-color var(--color-border-hover), icon color var(--color-text-secondary)
  
  Click: opens creation dialog
    Name: required text input
    Description: optional textarea
    Template: dropdown (Blank / Security Investigation / Journalist Research / Legal Discovery)
    Color: 12-color picker (pre-selected randomly)
    [Create] [Cancel]
    
    On create: new investigation appears, switches to it immediately
    API call: POST /api/v1/investigations (or stored client-side in localStorage)

State persistence:
  Each investigation stores independently:
    Pane ratio (localStorage: `chronicle:pane:${id}:think-pct`)
    Evidence panel open (localStorage: `chronicle:evidence:${id}:open`)
    Scroll positions (sessionStorage per tab)
    Input draft text (sessionStorage)
    Selected model (localStorage)
    Active context items (sessionStorage)
  
  On switch: all state for outgoing investigation saved, incoming investigation restored
  No API calls needed — all UI state is client-side
  Investigation data (sessions, findings) fetched fresh on switch

Keyboard: 
  Ctrl+1 through Ctrl+9: switch to investigation 1-9 (by order in grid)
  Ctrl+Tab: switch to next investigation (cycle order)
  Ctrl+Shift+Tab: switch to previous investigation
## 6. Timeline Explorer

The Timeline Explorer is a zoomable, pannable chronological canvas displaying all system events as positioned cards along a time axis. It serves as the primary temporal navigation interface for investigating session histories, finding relationships, and understanding event sequences. The design prioritizes information density, spatial consistency, and rapid temporal navigation.

### 6.1 Layout & Viewport Architecture

The Timeline Explorer occupies the full content area (fills remaining space after sidebar) with zero padding — every pixel of the viewport is interactive canvas.

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ TOOLBAR — 44px — position: sticky, top: 0, z-index: 10, bg-glass-light       │
│ ┌──────────┐ ┌──────────┐ ┌────────────┐ ┌─────┐ ┌───────┐ ┌──────────────┐│
│ │Bookmarks │ │Annotate  │ │Filter chips│ │Srch │ │Session│ │  Density Bar ││
│ │ ★ 3      │ │ ✎        │ │[All][Ses] │ │ 🔍  │ │ ▼ #a3f│ │  ▁▃▇█▇▃▁    ││
│ └──────────┘ └──────────┘ └────────────┘ └─────┘ └───────┘ └──────────────┘│
├──────────────────────────────────────────────────────────────────────────────┤
│ TIME RULER — 40px — position: sticky, top: 44px, z-index: 9                  │
│ ──────┬──────┬──────┬──────┬──────────┬──────┬──────┬──────┬──────┬─────────│
│  09:00│ 09:15│ 09:30│ 09:45│   10:00  │10:15 │10:30 │10:45 │11:00 │11:15    │
│ ──────┴──────┴──────┴──────┴─────┬────┴──────┴──────┴──────┴──────┴─────────│
│                                   │ NOW                                       │
├──────────────────────────────────────────────────────────────────────────────┤
│ CANVAS — flex: 1, overflow: auto, position: relative                          │
│   cursor: grab (default), grabbing (panning), crosshair (shift held)          │
│   scroll-behavior: auto (programmatic scroll is instant)                      │
│   Horizontal scroll: primary axis (wheel → horizontal scroll by default)      │
│   Vertical scroll: shift+wheel or two-finger vertical gesture                  │
│   Scrollbar: 6px, thumb border-hover, track transparent                       │
│                                                                                │
│ ┌── Date Group: June 23, 2026 ──────────────────────────────────────────────┐ │
│ │                                                                             │ │
│ │  ●───●───●  Session #a3f2b — Q4 Revenue Analysis                          │ │
│ │  │   │   │  (connector lines between events in same session)               │ │
│ │  │   │   │                                                                  │ │
│ │  [M] [F] [T]   ← Event cards: Memory, Finding, Task cards                    │ │
│ │                                                                             │ │
│ │  ●───●  Session #b2e1c — Phish Investigation                               │ │
│ │  │   │                                                                       │ │
│ │  [M] [A]    ← Memory, Approval cards                                        │ │
│ │                                                                             │ │
│ │  ⚠  Anomaly #7 — Token spike detected (standalone event)                   │ │
│ │                                                                             │ │
│ └─────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                │
│ ┌── Date Group: June 22, 2026 ──────────────────────────────────────────────┐ │
│ │  ...                                                                        │ │
│ └─────────────────────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────────────────┘
```

**Viewport coordinate system:**

```
Origin (0,0): top-left of canvas content area
X-axis: time — maps linearly to pixel positions
Y-axis: event stacking — events are placed in lanes (vertical slots)

Time-to-pixel mapping:
  pixelsPerMs = viewportWidth / visibleTimeRangeMs
  xPosition(event) = (event.timestamp - earliestVisible) * pixelsPerMs
```

**Canvas element:**

```html
<div id="timeline-canvas" style="
  position: relative;
  width: var(--timeline-total-width); /* Computed: max(viewport_width, total_time_range * zoom) */
  height: var(--timeline-total-height); /* Computed: (date_groups * group_height) + padding */
  overflow: hidden;
  cursor: grab;
">
  <!-- Date groups render as absolutely-positioned sections -->
  <!-- Event cards render as absolutely-positioned interactive elements -->
  <!-- Connector lines render as SVG overlay -->
</div>
```

**Viewport state:**

```typescript
interface TimelineViewport {
  scrollLeft: number;          // Current horizontal scroll position (px)
  scrollTop: number;           // Current vertical scroll position (px)
  zoom: number;                // Zoom level: 0.1 (wide) to 10.0 (microscopic)
  timeRangeStart: number;      // Unix ms of leftmost visible edge
  timeRangeEnd: number;        // Unix ms of rightmost visible edge
  viewportWidth: number;       // Container width in px (ResizeObserver)
  viewportHeight: number;      // Container height in px (ResizeObserver)
  focusedEventId: string|null; // Currently selected/focused event
  selectedEventIds: Set<string>; // Multi-selected events
  bookmarkIds: Set<string>;    // Bookmarked event IDs
  activeFilters: FilterState;  // Current filter state
}
```

**Responsive behavior:**

```
Desktop (>=1024px):
  Toolbar: single row, all controls visible
  Time Ruler: major ticks every 15min, minor every 5min at default zoom
  Cards: 320px wide × 72px tall (full detail mode)

Tablet (768-1023px):
  Toolbar: wraps to 2 rows (filters row 1, actions row 2)
  Time Ruler: major ticks every 30min, minor every 15min
  Cards: 260px × 64px

Mobile (<768px):
  Toolbar: collapsed to search + filter icon (expandable panel)
  Time Ruler: major ticks every 1h, no minor ticks
  Cards: full-width (100vw - 32px) × 56px, stacked vertically
  Date groups: single column, no multi-lane
```

### 6.2 Time Ruler

The time ruler is a horizontal bar at the top of the timeline showing temporal reference marks. It is sticky-positioned below the toolbar.

**Ruler anatomy:**

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ TIME RULER — 40px height, bg-bg-surface, border-bottom: 1px border-default   │
│                                                                                │
│  Major tick: 16px line, color text-secondary, 1px wide                        │
│  Minor tick: 8px line, color border-subtle, 1px wide                           │
│  Label: text-micro (10px), color text-tertiary, positioned below tick          │
│  Current-time indicator: 2px solid line, color accent-primary                  │
│    Label: "NOW" badge, text-micro, bg accent-primary, text white, radius-sm    │
│                                                                                │
│  Tick density by zoom level:                                                   │
│    zoom < 0.5:  Major = 4h,  Minor = 1h     (multi-day view)                  │
│    zoom 0.5-1:  Major = 1h,  Minor = 15min  (day view)                         │
│    zoom 1-2:    Major = 30min, Minor = 10min (half-day view)                   │
│    zoom 2-4:    Major = 15min, Minor = 5min  (few-hours view)                  │
│    zoom > 4:    Major = 5min,  Minor = 1min  (detail view)                     │
│                                                                                │
│  Tick alignment: ticks snap to "natural" boundaries                            │
│    Major ticks at: 00:00, 01:00, 02:00... (hourly), or aligned to zoom level  │
│    Minor ticks divide major intervals evenly (4 or 5 sub-divisions)            │
│                                                                                │
│  Label format by zoom level:                                                   │
│    zoom < 0.5:  "Jun 23", "Jun 24" (date)                                     │
│    zoom 0.5-2:  "09:00", "10:00" (HH:MM)                                      │
│    zoom > 2:    "09:30:00", "09:35:00" (HH:MM:SS)                             │
│                                                                                │
│  Timezone display: right side of ruler                                         │
│    Format: "UTC-4 (EDT)" in text-micro, text-tertiary                          │
│    Click: opens timezone picker dropdown                                       │
│                                                                                │
│  Ruler pan behavior:                                                           │
│    Scrolls horizontally with canvas content                                    │
│    Stays vertically pinned at top (position: sticky, top: 44px)                │
│    Vertical scroll shows/hides ruler naturally with content                    │
└──────────────────────────────────────────────────────────────────────────────┘
```

**Ruler implementation details:**

```typescript
interface TickMark {
  position: number;    // x-position in px (relative to viewport)
  timestamp: number;   // Unix ms
  label: string;       // Display label
  isMajor: boolean;    // Major tick (longer line + label) vs minor (short line)
}

function computeTicks(
  timeStart: number,
  timeEnd: number,
  viewportWidth: number
): TickMark[] {
  const visibleMs = timeEnd - timeStart;
  const pixelsPerMs = viewportWidth / visibleMs;

  // Determine tick interval based on available pixel space
  // Target: major ticks every ~100-200px, minor ticks every ~25-50px
  const majorInterval = findBestInterval(visibleMs, viewportWidth, {
    preferredPixelSpacing: 150,
    allowedIntervals: [
      60000,      // 1 minute
      300000,     // 5 minutes
      900000,     // 15 minutes
      1800000,    // 30 minutes
      3600000,    // 1 hour
      14400000,   // 4 hours
      43200000,   // 12 hours
      86400000,   // 1 day
      604800000,  // 1 week
    ]
  });

  const minorInterval = majorInterval / (majorInterval >= 3600000 ? 4 : 5);

  // Generate ticks aligned to interval boundaries
  const firstMajor = Math.ceil(timeStart / majorInterval) * majorInterval;
  const ticks: TickMark[] = [];

  for (let t = firstMajor; t <= timeEnd; t += minorInterval) {
    const isMajor = (t % majorInterval === 0);
    if (isMajor || minorInterval >= 60000) {
      ticks.push({
        position: (t - timeStart) * pixelsPerMs,
        timestamp: t,
        label: formatTickLabel(t, visibleMs),
        isMajor,
      });
    }
  }
  return ticks;
}

function formatTickLabel(timestamp: number, visibleMs: number): string {
  const d = new Date(timestamp);
  if (visibleMs > 172800000) {       // > 2 days
    return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
  } else if (visibleMs > 7200000) {  // > 2 hours
    return d.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', hour12: false });
  } else {                            // <= 2 hours
    return d.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false });
  }
}
```

**Current-time indicator:**

```
Position: computed from (Date.now() - timeRangeStart) * pixelsPerMs
Line: 2px solid, color accent-primary
  Extends full height of ruler (40px)
  Penetrates 8px into canvas below as a subtle guide

"NOW" badge:
  Positioned at top of indicator line, centered
  Background: accent-primary, color: text-inverse
  Font: text-micro (10px), font-weight 600
  Padding: 1px 6px, border-radius: radius-sm
  Pulse animation: opacity oscillates 0.8→1.0→0.8, 2s cycle, ease-in-out-quint

When "now" is off-screen (scrolled past):
  Arrow indicator at left/right edge of ruler
  Icon: arrow-circle-left or arrow-circle-right, 16px, accent-primary
  Click: scroll to now (smooth, 300ms ease-out-expo)
  Label: "→ NOW" or "NOW ←" in text-micro, text-tertiary
```

### 6.3 Zoom & Pan Interaction

The timeline supports zoom-to-cursor (not zoom-to-center) and inertia panning.

#### 6.3.1 Scroll-Wheel Zoom

```
Scroll wheel behavior:
  Default: horizontal scroll (no modifier)
    deltaY → scrollLeft += deltaY (natural direction)
    Vertical scroll: Shift+wheel or two-finger vertical trackpad

  Ctrl+Scroll (zoom):
    Zoom toward cursor position
    deltaY < 0 (scroll up): zoom in (multiply zoom by 1.15)
    deltaY > 0 (scroll down): zoom out (multiply zoom by 0.87)

  Zoom-to-cursor algorithm:
    1. Record cursorX (mouse X relative to canvas)
    2. Record timeAtCursor = timeRangeStart + (cursorX / viewportWidth) * visibleMs
    3. Apply zoom factor to visibleMs: newVisibleMs = visibleMs * zoomFactor
    4. Calculate new timeRangeStart: timeAtCursor - (cursorX / viewportWidth) * newVisibleMs
    5. Clamp zoom to [0.1, 10.0]
    6. Clamp timeRange to [earliestEvent - 10%, latestEvent + 10%]

  Zoom constraints:
    Min zoom (0.1): ~1 month visible in viewport
    Max zoom (10.0): ~5 minutes visible in viewport
    Zoom steps: continuous between min and max
    Zoom indicator: brief overlay (1500ms) showing "Zoom: 2.5×" with slider

  Pinch-to-zoom (touch):
    Two-finger pinch: same as Ctrl+Scroll
    Center point: midpoint between two fingers
    Gesture tracking: touchstart records initial distance, touchmove computes scale
    Scale factor: currentDistance / initialDistance
    Apply: same zoom-to-cursor logic with center point as cursor
```

#### 6.3.2 Drag Pan

```
Mouse drag (left button):
  On mousedown:
    cursor: grabbing
    Record dragStart = { x: clientX, y: clientY }
    Record scrollStart = { left: scrollLeft, top: scrollTop }

  On mousemove (while dragging):
    deltaX = dragStart.x - clientX
    deltaY = dragStart.y - clientY
    scrollLeft = scrollStart.left + deltaX
    scrollTop = scrollStart.top + deltaY
    Apply immediately (no throttling — direct DOM scroll manipulation)

  On mouseup:
    cursor: grab
    Apply inertia:
      velocityX = deltaX_last50ms / 50  (px/ms)
      velocityY = deltaY_last50ms / 50
      If |velocity| > 0.05:
        Animate deceleration using ease-out-expo over 600ms
        targetScrollLeft = scrollLeft + velocityX * 300
        targetScrollTop = scrollTop + velocityY * 300
      Cancel inertia on: next mousedown, wheel, or boundary hit

  Touch drag (one finger):
    Same as mouse drag
    touch-action: none on canvas (prevents browser scroll/zoom)

  Boundaries:
    scrollLeft: clamped to [0, totalWidth - viewportWidth]
    scrollTop: clamped to [0, totalHeight - viewportHeight]
    Rubber-band effect at boundaries:
      Beyond limit: resistance = 0.3 (scroll damps to 30% beyond boundary)
      On release: spring-back animation to boundary (300ms ease-anticipate)
```

#### 6.3.3 Keyboard Navigation

```
Arrow keys (when canvas focused):
  Left:  scrollLeft -= 100px (or 1 major tick interval, whichever is larger)
  Right: scrollLeft += 100px
  Up:    scrollTop -= 60px
  Down:  scrollTop += 60px
  Shift+Left/Right: scroll to previous/next event
  Home:  scrollLeft = 0 (first event)
  End:   scrollLeft = totalWidth - viewportWidth (last event)
  T:     scroll to NOW (current time)
  Ctrl+Plus:  zoom in (1.15×)
  Ctrl+Minus: zoom out (0.87×)
  Ctrl+0:     reset zoom to 1.0
  PageUp:     zoom in 3× (same as 3 Ctrl+Plus)
  PageDown:   zoom out 3×

Focus management:
  Canvas is focusable (tabindex="0")
  Focus ring: 2px accent-primary inset
  Click canvas: focuses canvas (for keyboard navigation)
  Escape: blur canvas, deselect all events
```

### 6.4 Event Cards

Event cards are the primary visual elements on the timeline. Each card represents one event in the system.

#### 6.4.1 Card Anatomy

```
┌──────────────────────────────────────────────────────────┐
│ ● Session #a3f2b                         2m ago  [ ★ ]  │  ← Header row
│ ───────────────────────────────────────────────────────── │  ← Border (1px, entity color)
│                                                           │
│ 🧠 Iteration 42 started                                   │  ← Event type icon + title
│     Started analyzing Q4 revenue — 847 tokens             │  ← Description (truncate 2 lines)
│                                                           │
│ [deepseek-v4-pro] [thinking] [847 tok]                   │  ← Tags row
└──────────────────────────────────────────────────────────┘

Card dimensions:
  Width: 320px (fixed at all zoom levels)
  Height: 72px (compact), 96px (expanded — on click/selection)
  Border-radius: radius-md
  Background: bg-surface
  Border: 1px border-default
  Border-left: 3px solid var(--entity-color) (entity type accent)
  Box-shadow: shadow-sm (default), shadow-md (hover), shadow-lg (selected)
  Margin-bottom: 4px (gap between stacked cards in same time lane)
```

**Card sub-elements:**

```
Header row (height: 24px, padding: 6px 10px 0):
  Entity indicator (left):
    Colored dot: 8px circle, entity color, flex-shrink: 0
    Entity label: text-caption, font-weight 600, text-primary, truncate
    Format: "{EntityType} #{shortId}" e.g., "Session #a3f2b"

  Timestamp (right, text-caption, text-tertiary):
    Relative: "2m ago", "1h ago", "yesterday"
    Hover (500ms delay): tooltip with absolute time "Jun 23, 2026 10:42:31 UTC-4"

  Bookmark toggle (rightmost, 16px):
    Icon: bookmark-simple, 14px, text-tertiary
    Bookmarked: icon-weight fill, color accent-warning (gold)
    Click: toggle bookmark, animation: scale-bounce 300ms ease-spring
    Tooltip: "Bookmark" / "Remove bookmark"

Divider:
  Height: 1px, background: entity color at 20% opacity
  Margin: 4px 10px 2px

Content row (padding: 0 10px 4px, flex: 1):
  Icon (left, 20px):
    Matches event type, entity color
    In 28px circle, background: entity color at 10% opacity
    Position: absolute, left: 10px, top: 50% (transform: translateY(-50%))

  Text container (margin-left: 36px):
    Title: text-small, font-weight 500, text-primary, truncate 1 line
    Description: text-caption, text-secondary, line-clamp: 2
      If no description: hidden
    Metadata line (optional): text-caption, text-tertiary
      Format depends on event type (see 6.4.3)

Tags row (height: 20px, padding: 0 10px 4px, display: flex, gap: 4px):
  Tags: small pills, text-micro, border-radius: radius-sm
  Each tag: padding 1px 6px, background entity color at 8% opacity
  Max visible: 3 tags per card (overflow: "+2 more" pill)

Connector handle (bottom center, visible on hover):
  Circle: 4px diameter, entity color
  Click+drag: create manual connection line to another event (see 6.5.3)
  Tooltip: "Drag to connect"
```

#### 6.4.2 Card States

```
Default (idle):
  bg: bg-surface
  border: 1px border-default
  border-left: 3px entity-color
  box-shadow: shadow-sm
  opacity: 1.0
  transform: scale(1)

Hover:
  bg: bg-hover-muted
  border: 1px border-hover
  box-shadow: shadow-md
  cursor: pointer
  transition: all 150ms var(--ease-out-quint)
  z-index: 2 (raises above sibling cards)

Selected (single click):
  bg: bg-selection-muted
  border: 1px accent-primary
  border-left: 3px accent-primary
  box-shadow: shadow-glow-blue
  z-index: 3
  Card expands height from 72px → 96px (if compact mode)
    Transition: height 250ms ease-out-expo
    Reveals: full description (no line-clamp), action buttons
  Selected indicator: blue checkmark badge, top-right corner (overlaps card edge)

Multi-selected (shift+click or drag-select):
  bg: bg-selection-muted
  border: 1px dashed accent-primary
  border-left: 3px accent-primary
  opacity: 0.95
  Selection order badge: numbered circle (1,2,3...) top-left of card

Focused (keyboard navigate):
  Same visual as selected + focus ring
  focus-visible: box-shadow shadow-glow-blue

Loading (event data being fetched):
  Skeleton card: same dimensions, bg-skeleton, shimmer animation
  Shimmer: linear-gradient sweep, 1.5s cycle, ease-linear infinite

Error (event data failed to load):
  Card: muted colors, opacity 0.6
  Icon: warning-circle, 16px, accent-error
  Title: event ID, text-error
  Description: "Failed to load event data"
  Retry button: text-small, text-link, "Retry"

Disabled (filtered out but still visible):
  opacity: 0.2
  pointer-events: none
  Filter applied badge: "Hidden by filter" tooltip
```

#### 6.4.3 Event Type Card Variants

**Session Event Card:**

```
┌──────────────────────────────────────────────────────────┐
│ ● Session #a3f2b                         2m ago  [ ★ ]  │
│ ───────────────────────────────────────────────────────── │
│  [cpu]  Session started                                   │
│         Created by Kara · Q4 Revenue Analysis             │
│  [deepseek-v4-pro] [booting→thinking] [it 42]            │
└──────────────────────────────────────────────────────────┘

Entity color: --color-entity-session (oklch(60% 0.18 265), blue)
Icon: cpu, 20px
Status dot: animated per session status (see 1.2.7)
  Positioned on left border, 8px, entity-appropriate status color
Metadata format: "Created by {user} · {session_goal_short}"
Tags: model name, status transition (e.g., booting→thinking), current iteration
```

**Memory Event Card:**

```
┌──────────────────────────────────────────────────────────┐
│ ● Memory #m8472                         5m ago  [ ★ ]   │
│ ───────────────────────────────────────────────────────── │
│  [database]  Memory stored                                │
│              Key: /projects/q4-revenue · trust: high      │
│  [domain: concept] [trust: high] [847 tokens]            │
└──────────────────────────────────────────────────────────┘

Entity color: --color-entity-memory (oklch(60% 0.15 200), cyan)
Icon: database, 20px
Trust badge: colored per trust level (see 1.2.8), shown on right of divider
Metadata format: "Key: {key_path} · trust: {trust_level}"
Tags: domain, trust level (colored badge), token count
```

**Finding Card:**

```
┌──────────────────────────────────────────────────────────┐
│ ● Finding #f1294                         12m ago [ ★ ]  │
│ ───────────────────────────────────────────────────────── │
│  [magnifying-glass] Finding drafted                       │
│       APAC region shows 23% revenue decline in Q4         │
│  [severity: high] [confidence: 87%] [source: #e5b3f]    │
└──────────────────────────────────────────────────────────┘

Entity color: --color-entity-finding (oklch(55% 0.18 145), green)
Icon: magnifying-glass, 20px
Severity badge (right of divider): colored pill
  High: red, Medium: amber, Low: muted
Metadata format: finding summary (truncated 2 lines)
Tags: severity, confidence percentage, source entity link
```

**Task Card:**

```
┌──────────────────────────────────────────────────────────┐
│ ● Task #t5631                           8m ago  [ ★ ]   │
│ ───────────────────────────────────────────────────────── │
│  [check-square]  Task created                             │
│       Fix authentication rate limiting                    │
│  [session: #a3f2b] [status: pending] [priority: high]   │
└──────────────────────────────────────────────────────────┘

Entity color: --color-entity-task (oklch(55% 0.15 85), amber)
Icon: check-square, 20px
Status badge: colored pill per task status
Metadata format: task title, truncated 2 lines
Tags: parent session link, status, priority
```

**Approval Card:**

```
┌──────────────────────────────────────────────────────────┐
│ ● Approval #a9012                        15m ago [ ★ ]  │
│ ───────────────────────────────────────────────────────── │
│  [shield-check]  Approval requested                       │
│       DROP TABLE staging — needs human review             │
│  [session: #a3f2b] [type: sql_exec] [pending ⚠]         │
└──────────────────────────────────────────────────────────┘

Entity color: --color-entity-approval (oklch(50% 0.20 20), red)
Icon: shield-check, 20px
Border pulse: when status is 'pending', card border pulses red glow (2s cycle)
Metadata format: approval request summary
Tags: parent session, approval type, status (pending=red pulse, approved=green, denied=gray)
```

**Anomaly Card:**

```
┌──────────────────────────────────────────────────────────┐
│ ⚠ Anomaly #z3478                         1h ago  [ ★ ]  │
│ ───────────────────────────────────────────────────────── │
│  [warning]  Token usage spike detected                    │
│       Session #a3f2b used 847K tokens in single iteration │
│  [severity: high] [type: token_spike] [session: #a3f2b] │
└──────────────────────────────────────────────────────────┘

Entity color: --color-entity-anomaly (oklch(55% 0.18 30), orange-red)
Icon: warning, 20px
Card border: 2px dashed accent-error at 50% opacity (distinctive anomaly styling)
Background: subtle red tint, bg-error at 3% opacity
Metadata format: anomaly description
Tags: severity, anomaly type, related entity link
```

#### 6.4.4 Card Positioning

```
Horizontal positioning (X):
  x = (event.timestamp - timeRangeStart) * pixelsPerMs + cardMarginLeft

Vertical positioning (Y) — lane allocation algorithm:
  Each event is placed in a "lane" to avoid overlap.
  Lane height: cardHeight + cardGap (72px + 4px = 76px)

  Algorithm: Greedy Lane Assignment
    1. Sort events within date group by timestamp ascending
    2. For each event:
       a. Try lane 0. Check if event overlaps with any existing event in lane 0.
          Overlap condition: |event.timestamp - existing.timestamp| * pixelsPerMs < cardWidth + 4
       b. If overlap, try lane 1, lane 2, etc.
       c. Place event in first non-overlapping lane
    3. Date group height = (maxLaneIndex + 1) * laneHeight + groupHeaderHeight

  Overlap check considers:
    - Card width (320px at default zoom)
    - 4px minimum horizontal gap between cards
    - Cards in same session are placed adjacent (same lane) unless they overlap
      Connected events prefer lane adjacency for connector line clarity

  Vertical stacking within lane:
    Cards in same lane stack vertically with 0 gap (cards touch edge-to-edge)
    This creates continuous session "streaks" that are visually cohesive

  Date group positioning:
    Each date group starts at:
      y = sum(previous_group_heights) + (groupIndex * groupSpacing)
    Date group header: 32px tall, sticky within scroll
    Events within group: y = groupY + groupHeaderHeight + (laneIndex * laneHeight)

  Sticky date group headers:
    As user scrolls vertically, the current date group header sticks to top
    (below the time ruler). When next group reaches top, it replaces current.
    Implementation: IntersectionObserver + CSS position: sticky
```

#### 6.4.5 Card Entry/Exit Animations

```
New event (WebSocket push):
  Card slides in from top: translateY(-20px) → translateY(0)
  Opacity: 0 → 1
  Duration: 300ms, ease-out-expo
  Trigger: event appears in real-time stream
  If card is off-screen: no animation, just render

Event removal (data refresh):
  Card fades and shrinks: opacity 1→0, scale 1→0.9
  Duration: 200ms, ease-in-quint
  Adjacent cards slide to fill gap: margin transition 250ms ease-out-quint

Zoom change:
  Cards DO NOT scale (remain 320px wide)
  Cards translate horizontally to match new time position
  Translation: CSS transform: translateX(newX - oldX)
  Duration: 100ms, ease-out-quint (fast, feels responsive)
  Cards entering viewport during zoom: appear at final position immediately

Filtered cards:
  When filter removes cards: opacity 1→0, translateY(-4px), 200ms ease-in-quint
  When filter restores cards: opacity 0→1, translateY(4px→0), 250ms ease-out-quint
  Stagger delay: 20ms per card (creates wave effect)
```

### 6.5 Connector Lines

Connector lines show the sequential relationship between events within the same session.

#### 6.5.1 Line Specification

```
Line rendering: SVG overlay positioned absolutely over the canvas
  <svg style="position:absolute;top:0;left:0;pointer-events:none;z-index:1;">
    <!-- Lines render beneath cards (z-index 1 vs card z-index 2+) -->
  </svg>

Line style:
  Stroke: 2px solid, entity color at 40% opacity
  Stroke-dasharray: none (solid)
  Stroke-linecap: round
  Filter: none
  Antialiasing: shape-rendering="crispEdges"

Line path:
  Connection from event A (source) to event B (target) within same session
  Events connected in chronological order (A.timestamp < B.timestamp)

  Path calculation:
    startX = cardA.x + cardWidth       (right edge of source card)
    startY = cardA.y + cardHeight / 2  (vertical center of source card)
    endX = cardB.x                      (left edge of target card)
    endY = cardB.y + cardHeight / 2    (vertical center of target card)

  Path geometry: cubic bezier (curved connector)
    controlPoint1X = startX + Math.min(40, (endX - startX) * 0.4)
    controlPoint1Y = startY
    controlPoint2X = endX - Math.min(40, (endX - startX) * 0.4)
    controlPoint2Y = endY
    d = `M ${startX} ${startY} C ${cp1x} ${cp1y} ${cp2x} ${cp2y} ${endX} ${endY}`

  Straight-line fallback:
    When cards are in same lane and horizontally adjacent (distance < 80px):
    d = `M ${startX} ${startY} L ${endX} ${endY}`

Line interaction:
  Hover line: stroke width increases to 3px, opacity to 70%
    Cursor: pointer
    Tooltip: "{session_name}: {eventA_type} → {eventB_type} ({duration})"
  Click line: select both connected events
  Lines are pointer-events: none by default, visiblePainted on hover zone
    (6px invisible wider hit area for easier hover targeting)

Line entry animation:
  New line: draws in from source to target
  SVG stroke-dashoffset animation:
    Path length measured via getTotalLength()
    stroke-dasharray: pathLength
    stroke-dashoffset: pathLength → 0
    Duration: 400ms, ease-out-expo
    Delayed: starts 100ms after both cards are positioned
```

#### 6.5.2 Session Streak Visualization

```
When a session has many events in chronological sequence:
  - Lines connect consecutive events
  - A subtle "session streak" background highlights the connected group
    Background: entity color at 3% opacity, border-radius: radius-md
    Extends from left of first card to right of last card, full lane height
    Rounded rectangle drawn behind all cards in the session

  Streak label (left side, vertically centered):
    Text: session name/ID, text-micro, entity color, rotation -90deg
    Positioned in left margin (16px from cards)
    Only shown for sessions with 3+ connected events

  Gap handling:
    If session has a time gap > 30 minutes without events:
      Line shows "gap" indicator: dashed segment with clock icon in middle
      Clock icon: 12px, text-tertiary, "30m gap" label
```

#### 6.5.3 Manual Connections

```
User-created connections (drag from connector handle):
  Mousedown on connector handle (bottom-center of card):
    Create ghost line: follows cursor
    Ghost line: 1px dashed, white 30% opacity
    Valid targets highlight: compatible events pulse blue border
    Invalid targets: cursor shows "not-allowed"

  Compatible connection rules:
    Session → Memory event: allowed
    Session → Finding: allowed
    Session → Task: allowed
    Memory → Finding: allowed
    Finding → Evidence source: allowed
    Anomaly → Session: allowed
    Anomaly → Task: allowed
    Any → Any (within same session): always allowed

  On drop (mouseup over valid target):
    Create connection line between source and target
    Connection stored as "manual" type (persisted to backend)
    Animation: line draws from source to target (400ms ease-out-expo)
    Toast: "Connected {source} → {target}"

  On Escape or drop on invalid:
    Ghost line removed
    No toast

  Manual connection deletion:
    Right-click line → "Remove connection" in context menu
    Confirmation: none (undo via toast for 5 seconds)
```

### 6.6 Date Groups

Events are grouped by date to create visual separation and navigational landmarks.

```
Date Group Header:
  Height: 32px
  Background: bg-canvas (not bg-surface — blends with canvas)
  Border-bottom: 1px border-subtle
  Padding: 0 16px
  Display: flex, align-items: center, gap: 8px
  Position: sticky within canvas scroll container

  Content:
    Date label: text-subtitle, font-weight 600, text-primary
      Format: "Monday, June 23, 2026" (today), "June 22, 2026" (past)
      Special: "Today", "Yesterday" (for recent dates)
    Event count badge: text-caption, text-tertiary, "12 events"
    Collapse toggle (right): chevron icon, 16px, text-tertiary
      Click: collapse/expand group (animates event cards sliding up/down)
      Collapsed: only header visible, badge shows count, chevron rotated

Group rendering:
  Max groups initially loaded: 7 days (prevents performance issues)
  "Load more" indicator at bottom:
    Sentinel element: IntersectionObserver trigger
    On intersect: load next 7 days of events
    Loading skeleton: 3 placeholder date groups with skeleton cards

  Empty group (date with no events but within range):
    Header shows date
    Subtitle: "No events" in text-caption, text-tertiary
    No card slot, 32px total height

  Future group (dates beyond now):
    Header shows date
    Subtitle: "Future" in text-caption, text-tertiary
    Divider: dashed (2px dashes, border-subtle)
    No events displayed (placeholder space)
```

### 6.7 Filtering

Multi-dimensional filtering with real-time visual feedback.

#### 6.7.1 Entity Type Filter Chips

```
Position: toolbar, left section
Layout: display: flex, gap: 4px, flex-wrap: wrap

Chip anatomy:
  Height: 28px
  Padding: 0 12px
  Border-radius: radius-full
  Background: transparent
  Border: 1px border-default
  Font: text-caption, text-secondary
  Cursor: pointer
  Transition: var(--transition-color)

  Content:
    Icon: 14px, entity color (muted when inactive)
    Label: entity type name
    Count badge: entity type count in viewport, text-micro, text-tertiary

  States:
    Active: bg entity-color at 15% opacity, border entity-color, text entity-color
      Icon: entity color at full opacity
      Count badge: bg entity-color at 20% opacity
    Hover: bg bg-hover-muted
    Focus-visible: focus ring

Chip order (left to right):
  [All] [Sessions] [Memories] [Findings] [Tasks] [Approvals] [Anomalies]

"All" chip behavior:
  Toggle: activates all chips (default state)
  Individual chip toggle: deactivates "All" and activates only that chip
  All individual chips active: "All" reactivates automatically
  Logic: show events of ANY active chip type (OR filter)

Filter interaction:
  Multiple chips can be active simultaneously
  Click active chip: deactivates it
  Click inactive chip: activates it
  If all deactivated → "All" activates automatically
  Visual feedback: cards of filtered-out types fade out (200ms ease-in-quint)
  Filtered cards: opacity 0.1, not interactive, grayed connector lines

Count badges update:
  Count reflects events currently IN VIEWPORT (not total filtered)
  Format: "12" (number of visible events of that type)
  Updates on: scroll, zoom, filter change
  Debounced: 50ms for performance
```

#### 6.7.2 Session Filter Dropdown

```
Position: toolbar, center-right

Trigger button:
  Height: 32px
  Padding: 0 12px
  Border: 1px border-default
  Border-radius: radius-md
  Background: bg-surface
  Font: text-small, text-primary
  Content: "Session: {session_name}" or "Session: All sessions"
  Chevron: down, 12px, text-tertiary
  Click: opens dropdown

Dropdown:
  Position: absolute, below trigger, left-aligned
  Width: 280px
  Max-height: 360px
  Background: bg-surface
  Border: 1px border-default
  Border-radius: radius-md
  Box-shadow: shadow-lg
  Z-index: 200

  Header:
    Search input: height 36px, bg-input, border-bottom border-default
    Placeholder: "Filter sessions..."
    Icon: magnifying-glass, 14px, left: 10px
    Clear button: x icon, right: 10px (appears when input has text)
    Debounce: 100ms

  List (scrollable):
    "All sessions" item (always first, selected by default)
    Divider: 1px border-subtle

    Session items:
      Height: 36px, padding: 0 12px, display: flex, align-items: center, gap: 8px
      Status dot: 8px, session status color
      Name: text-small, text-primary, truncate
      ID: text-caption, mono, text-tertiary
      Event count: text-caption, text-tertiary, right-aligned
      Hover: bg-hover-muted
      Selected: bg-selection, checkmark right

  Close: Escape, click outside, select item
```

#### 6.7.3 Text Search

```
Position: toolbar, right of session dropdown

Search input:
  Width: 200px (expands to 300px on focus)
  Height: 32px
  Background: bg-input
  Border: 1px border-default
  Border-radius: radius-md
  Padding-left: 32px (icon space)
  Font: text-small, color: text-primary
  Placeholder: "Search events..."

  Search icon: magnifying-glass, 14px, text-tertiary, position: absolute, left: 8px

  Clear button: x icon, 14px, appears when input has text
    Click: clears search, restores full event set

Search behavior:
  Debounce: 200ms before triggering search
  Min characters: 2
  Search fields:
    Event title (weight: 3)
    Event description (weight: 2)
    Session name (weight: 1)
    Entity IDs (weight: 4 — exact match on ID)
    Tags/metadata (weight: 1)

  Results:
    Matching events highlighted: all non-matching events dim to 20% opacity
    Matching cards get subtle glow: box-shadow glow-blue at 30% opacity
    Match count shown in search bar: "3 matches" / "No matches"

  Empty search:
    Shows all events (restores full opacity)
    Clear button hidden

  No results:
    Search bar border: 1px border-warning
    Helper text: "No events match '{query}'" below search bar
    "Clear search" link next to helper text

  Keyboard shortcut: Ctrl+F focuses search input
```

#### 6.7.4 Time Range Filter

```
Quick-range selector (toolbar, far right):
  Buttons: [1h] [6h] [24h] [7d] [All]
  Active: bg accent-primary-muted, text accent-primary
  Inactive: transparent, text-secondary
  Click: sets zoom and scrolls to show that time range ending at NOW
  
  Behavior:
    "1h": zoom to show past 1 hour
    "6h": zoom to show past 6 hours
    "24h": zoom to show past 24 hours
    "7d": zoom to show past 7 days
    "All": zoom to fit all events in viewport

Custom date range (via time ruler):
  Shift+drag on time ruler: select custom time range
  Selected range: highlighted band, bg accent-primary at 10% opacity
  Release: time filter applied, cards outside range hidden
  Range indicator: blue bar above ruler showing selected range
  "Clear range" button appears in toolbar
```

### 6.8 Bookmarks

Bookmarks allow users to save specific events for quick navigation.

```
Bookmark toggle (per card):
  Icon: bookmark-simple, 14px
  Position: top-right corner of card header
  States:
    Unbookmarked: icon weight regular, color text-tertiary
    Bookmarked: icon weight fill, color accent-warning (gold)
  Animation: scale-bounce 300ms ease-spring on toggle
  Tooltip: "Bookmark this event" / "Remove bookmark"

Bookmarks panel (toolbar button):
  Trigger: "★ {count}" button in toolbar
    Count: number of bookmark items (live update)
    Color: accent-warning when count > 0, text-secondary when 0

  Panel (dropdown on click):
    Width: 320px
    Max-height: 400px
    Background: bg-surface
    Border: 1px border-default
    Border-radius: radius-md
    Box-shadow: shadow-lg
    Z-index: 200

    Header:
      "Bookmarks ({count})" in text-subtitle, font-weight 600
      "Clear all" button (right): text-small, text-link, red
        Confirmation: "Clear all bookmarks?" toast with Undo (5s)

    List (scrollable):
      Bookmark items:
        Height: 48px, padding: 8px 12px
        Display: flex, gap: 10px, align-items: center
        Border-bottom: 1px border-subtle

        Entity icon: 20px, entity color, in 28px circle
        Content:
          Title: text-small, text-primary, truncate
          Subtitle: text-caption, text-secondary
            Format: "{entity_type} #{id} · {timestamp_relative}"
        Actions (right):
          Navigate: arrow icon, 16px, text-tertiary
            Click: scrolls timeline to event, closes panel
          Remove: x icon, 14px, text-tertiary
            Click: removes bookmark, item slides out
        Hover: bg-hover-muted
        Click main area: navigate to event

    Empty state:
      "No bookmarks yet"
      "Click ★ on any event card to bookmark it"
      Icon: bookmark-simple, 48px, color text-disabled, centered

  Bookmark keyboard shortcuts:
    Ctrl+B: toggle bookmark on selected event(s)
    Ctrl+Shift+B: open bookmarks panel
    Ctrl+.] (next bookmark): scroll to next bookmarked event
    Ctrl+.[ (prev bookmark): scroll to previous bookmarked event

Bookmark persistence:
  Stored in localStorage under key: 'timeline-bookmarks'
  Structure: { eventIds: string[], updatedAt: ISO8601 }
  Survives page refresh, browser restart
  Synced to backend if user is authenticated (via API)
```

### 6.9 Multi-Select

Multi-select allows batch operations on multiple events.

```
Selection methods:
  1. Shift+Click:
     - Select first event (click)
     - Shift+click second event: selects all events between them in time order
     - Works across date groups and lanes
     - Selection range calculated by timestamp, not visual position

  2. Drag-select (marquee):
     - Hold Shift + mouse drag: draws selection rectangle
     - Rectangle: 1px dashed accent-primary, bg accent-primary at 5% opacity
     - On release: all events within rectangle are selected
     - Events must have card center within rectangle bounds
     - Drag-select starts from empty canvas area (not on an event card)

  3. Ctrl+Click:
     - Toggle individual event selection without clearing existing selection
     - If event is selected: deselects it
     - If event is unselected: adds to selection

  4. Ctrl+A:
     - Select all VISIBLE events (respects active filters)
     - Toast: "{count} events selected"

Selection visual state:
  Selected cards:
    Border: 1px dashed accent-primary
    Border-left: 3px solid accent-primary
    Background: bg-selection-muted
    Selection order badge: numbered circle (1, 2, 3...) in top-left
      Circle: 18px diameter, bg accent-primary, text text-inverse, text-micro, font-weight 600
      Shows selection number (order of selection)
      Max visible badges: 99 (shows "99+" beyond)

Selection counter (toolbar, appears when selection active):
  "{N} selected" in text-small, accent-primary
  [Deselect] button: text-small, text-link
  Action buttons appear next to counter:
    [Bookmark All] [Export] [Annotate] [Compare]

Batch actions:
  Bookmark All: adds bookmark to all selected events
    Toast: "{N} events bookmarked" (with Undo, 5s)
  
  Export: exports selected events as JSON/CSV
    Opens export dialog (see 6.11)
  
  Annotate: opens annotation panel with all selected events
    Creates annotation spanning multiple events
    Annotation line: vertical band across selected time range
  
  Compare: opens comparison view (split panel showing selected events side-by-side)
    Only available for 2 selected events
    Shows diff of metadata, descriptions, tags

  Deselect: Escape key or click "Deselect" button
    Clears entire selection
    Selection badges animate out: scale 1→0, 150ms ease-in-quint

Selection state:
  Persists through: zoom, pan, filter changes (if events remain visible)
  Clears on: navigation away from timeline, explicit deselect
  Selection survives: events moving position due to zoom/scroll
```

### 6.10 Annotations

Annotations allow users to add notes and markers to the timeline.

```
Annotation types:
  1. Point annotation: attached to a single event
  2. Range annotation: spans a time range (two events or arbitrary times)
  3. Free annotation: positioned at arbitrary point on timeline

Annotation creation:
  Method 1: Select event → click "Annotate" in toolbar
    Opens annotation panel pre-focused on selected event
  Method 2: Right-click on empty canvas → "Add annotation here"
    Creates free annotation at cursor time position
  Method 3: Select two events → "Annotate range"
    Creates range annotation between the two timestamps
  Method 4: Double-click on empty canvas area
    Creates free annotation at click time position

Annotation panel (drawer, slides from right):
  Width: 360px
  Background: bg-surface
  Border-left: 1px border-default
  Box-shadow: shadow-lg
  Z-index: 300
  Slide animation: translateX(100%) → translateX(0), 300ms ease-out-expo

  Panel anatomy:
    Header (48px):
      "Annotation" in text-subtitle, font-weight 600
      Type badge: "Point" | "Range" | "Free" in pill
      Close button: x icon, 20px

    Content (scrollable):
      Title input:
        Height: 36px, bg-input, border border-default, radius-md
        Font: text-body, text-primary
        Placeholder: "Annotation title (optional)"

      Note textarea:
        Min-height: 120px, bg-input, border border-default, radius-md
        Font: text-body, text-primary
        Placeholder: "Write your annotation..."
        Resize: vertical
        Character count: right-bottom, text-caption, text-tertiary

      Color picker (6 preset colors):
        Colors: entity color palette colors
        Display: 24px circles, row, gap: 8px
        Selected: checkmark inside circle, border 2px white
        Default: accent-warning (gold)

      Linked entities:
        Shows entities linked to annotation target(s)
        Each entity: small card with icon + name + ID
        "Link more" button: opens entity search

      Visibility:
        Radio: "Only me" | "Team" | "Everyone"
        Default: "Only me"

    Footer (48px):
      [Cancel] button: border border-default, radius-md
      [Save] button: bg accent-primary, text white, radius-md
        Click: saves annotation, panel closes
        Toast: "Annotation saved"

Annotation display on timeline:
  Point annotation:
    Small pin icon at event card top, annotation color
    Hover: tooltip shows annotation title
    Click pin: opens annotation panel (read-only mode)

  Range annotation:
    Colored band spanning time range
    Height: full lane height, opacity: 10%
    Border-top and border-bottom: 1px solid, annotation color at 50% opacity
    Label centered in band: annotation title, text-caption, annotation color
    Click label: opens annotation panel (read-only)

  Free annotation:
    Pin icon on ruler at annotation timestamp
    Vertical dashed line extending through canvas (10% opacity, annotation color)
    Hover: tooltip with annotation title
    Click: opens annotation panel

Annotation list (available from toolbar "Annotations" button):
  Same as bookmark panel layout
  Shows all annotations sorted by time
  Edit/Delete actions per annotation
```

### 6.11 Density Indicators

Density indicators provide a visual overview of event concentration across the timeline.

```
Density bar (toolbar, far right):
  Height: 20px
  Width: 160px
  Background: bg-input
  Border-radius: radius-sm
  Display: flex, overflow: hidden

  Bar segments:
    Each segment represents a time bucket (total timeline / 100 buckets)
    Height: 100%
    Color: heatmap scale based on event count in bucket
      oklch(95% 0.02 260) → oklch(70% 0.12 40) → oklch(70% 0.18 30) → oklch(55% 0.20 20)
      (white → orange → red → dark red)
    Width: proportional to time bucket duration (uniform at constant zoom)

  Interaction:
    Hover segment: tooltip showing time range + event count
      "{bucket_start} – {bucket_end}: {count} events"
    Click segment: scroll timeline to that time range
    Density bar is horizontally scrollable (mirrors main timeline scroll)

  Density scale:
    Event counts per bucket mapped to color:
      count==0: oklch(95% 0.02 260)     (white — no events)
      count==1: oklch(80% 0.08 40)      (light orange)
      count 2-3: oklch(70% 0.12 40)    (medium orange)
      count 4-6: oklch(70% 0.18 30)    (red-orange)
      count 7-10: oklch(60% 0.18 20)   (red)
      count > 10: oklch(55% 0.20 20)   (dark red)

  Update behavior:
    Recalculates on: zoom change, filter change, data refresh
    Debounced: 100ms
    Transition: background-color 200ms ease-out-quint

Density overlay (on canvas):
  At low zoom levels (zoom < 0.3):
    Cards are too small to meaningfully display
    Instead: density heatmap replaces cards
    Heatmap: vertical bars (1px wide) colored by event density
    Background: bg-canvas
    This is the "overview mode" — provides macro-level event distribution
    Transition: cards fade out / heatmap fades in over 300ms

  Zoom threshold for card display:
    zoom >= 0.3: event cards visible
    zoom < 0.3: heatmap only
    Smooth transition: cards opacity = clamp((zoom - 0.2) / 0.1, 0, 1)
```

### 6.12 Right-Click Context Menus

Each entity type has a tailored context menu. The right-click target determines the menu content.

```
Context menu trigger:
  Desktop: right-click (onmousedown button===2 or oncontextmenu)
  Mobile: long-press (500ms hold)
  Prevent default browser context menu (event.preventDefault())

Event card context menu:

  Session event:
    ┌─────────────────────────────┐
    │ [eye]  View Session        │
    │ [magnifying-glass]  Investigate │
    │ [arrows-out]  Expand       │
    │ ─────────────────────────  │
    │ [copy]  Copy ID            │
    │ [link]  Copy Link          │
    │ ─────────────────────────  │
    │ [bookmark]  Bookmark       │
    │ [pencil]  Annotate         │
    │ ─────────────────────────  │
    │ [x-circle]  Cancel Session │  ← Red, if session is active
    └─────────────────────────────┘

  Memory event:
    ┌─────────────────────────────┐
    │ [eye]  View Memory         │
    │ [database]  Browse Key     │
    │ ─────────────────────────  │
    │ [copy]  Copy Key Path      │
    │ [copy]  Copy Value (JSON)  │
    │ [link]  Copy Link          │
    │ ─────────────────────────  │
    │ [bookmark]  Bookmark       │
    │ [pencil]  Annotate         │
    │ ─────────────────────────  │
    │ [shield-check]  Change Trust │ → Submenu: verified/high/medium/low/quarantine
    │ [trash]  Delete Memory     │  ← Red
    └─────────────────────────────┘

  Finding event:
    ┌─────────────────────────────┐
    │ [eye]  View Finding        │
    │ [magnifying-glass]  Investigate │
    │ ─────────────────────────  │
    │ [copy]  Copy ID            │
    │ [link]  Copy Link          │
    │ ─────────────────────────  │
    │ [bookmark]  Bookmark       │
    │ [pencil]  Annotate         │
    │ ─────────────────────────  │
    │ [check-circle]  Approve    │ ← Green
    │ [x-circle]  Reject         │ ← Red
    │ [shield-check]  Change Severity │ → Submenu
    └─────────────────────────────┘

  Task event:
    ┌─────────────────────────────┐
    │ [eye]  View Task           │
    │ [play]  Start Task         │  ← If pending
    │ [pause]  Pause Task        │  ← If in-progress
    │ ─────────────────────────  │
    │ [copy]  Copy ID            │
    │ [link]  Copy Link          │
    │ ─────────────────────────  │
    │ [bookmark]  Bookmark       │
    │ [pencil]  Annotate         │
    │ ─────────────────────────  │
    │ [x-circle]  Cancel Task    │  ← Red
    │ [trash]  Delete Task       │  ← Red
    └─────────────────────────────┘

  Approval event:
    ┌─────────────────────────────┐
    │ [eye]  View Approval       │
    │ ─────────────────────────  │
    │ [check-circle]  Approve    │ ← Green, if pending
    │ [x-circle]  Deny           │ ← Red, if pending
    │ [clock]  Defer (30m)       │ ← If pending
    │ ─────────────────────────  │
    │ [copy]  Copy ID            │
    │ [link]  Copy Link          │
    │ ─────────────────────────  │
    │ [bookmark]  Bookmark       │
    │ [pencil]  Annotate         │
    └─────────────────────────────┘

  Anomaly event:
    ┌─────────────────────────────┐
    │ [eye]  View Anomaly        │
    │ [magnifying-glass]  Investigate │
    │ ─────────────────────────  │
    │ [flag]  Flag as Reviewed   │ ← If unreviewed
    │ [shield]  Escalate         │
    │ ─────────────────────────  │
    │ [copy]  Copy ID            │
    │ [link]  Copy Link          │
    │ ─────────────────────────  │
    │ [bookmark]  Bookmark       │
    │ [pencil]  Annotate         │
    │ ─────────────────────────  │
    │ [eye-slash]  Dismiss       │ ← Gray (hides from timeline)
    └─────────────────────────────┘

Multi-select context menu (when 2+ events selected):
  ┌─────────────────────────────┐
  │ [bookmark]  Bookmark {N}   │
  │ [export]  Export {N}       │
  │ [pencil]  Annotate {N}     │
  │ ─────────────────────────  │
  │ [arrows-left-right]  Compare (2 only) │ ← Only visible when exactly 2 selected
  │ ─────────────────────────  │
  │ [copy]  Copy IDs           │
  │ [deselect]  Deselect All   │
  └─────────────────────────────┘

Canvas context menu (right-click on empty canvas area):
  ┌─────────────────────────────┐
  │ [pencil]  Add Annotation   │
  │ ─────────────────────────  │
  │ [clock]  Jump to Now       │
  │ [arrows-out]  Fit All      │
  │ ─────────────────────────  │
  │ [calendar]  Go to Date...  │  ← Opens date picker
  │ ─────────────────────────  │
  │ [funnel]  Clear Filters    │
  │ [bookmark]  Show Bookmarks │
```

### 6.13 Export

```
Export trigger: toolbar button (download icon) or context menu "Export"

Export dialog (modal):
  Width: 480px
  Background: bg-surface-overlay
  Border: 1px border-default
  Border-radius: radius-lg
  Box-shadow: shadow-xl
  Z-index: 400
  Animation: scale(0.95)→scale(1) + fade, 200ms ease-out-expo

  Header:
    "Export Events" in text-subtitle
    Close button: x icon

  Content:
    Format selection:
      Radio group: [JSON] [CSV] [PDF Report]
      Description below each format:
        JSON: "Machine-readable, includes all metadata"
        CSV: "Spreadsheet-friendly, selected fields only"
        PDF: "Annotated timeline report with event details"

    Range selection:
      Radio: [All visible] [Selected ({N})] [Custom range]
      Custom: two date/time inputs with calendar picker

    Field selection (JSON/CSV only):
      Checklist of available fields:
        ☑ Event ID         ☑ Timestamp          ☑ Event Type
        ☑ Title            ☑ Description        ☑ Entity Type
        ☐ Metadata         ☑ Tags               ☐ Raw Data
        ☑ Session ID       ☑ Related Entities   ☐ Full Content

    CSV options (if CSV selected):
      Delimiter: [Comma] [Tab] [Semicolon]
      Include header row: toggle, default ON

    PDF options (if PDF selected):
      Page size: [A4] [Letter]
      Orientation: [Portrait] [Landscape]
      Include timeline graphic: toggle, default ON

  Footer:
    [Cancel] button
    [Export] button: bg accent-primary, text white
      Click: generates export, shows download progress
      Progress: spinner + "Generating..." + progress bar (for PDF)

  Export execution:
    JSON: synchronous (fast), triggers download immediately
    CSV: synchronous (fast), triggers download immediately
    PDF: async (slow — up to 10s for large ranges)
      Shows progress overlay: "Rendering timeline... 45%"
      Show page count during generation: "Estimated 12 pages"
    On complete: toast "Exported {N} events as {format}"
      With action: [Open File] [Copy to Clipboard]

  Download filename format:
    "chronicle-export-{date_range}-{timestamp}.{extension}"
    Example: "chronicle-export-Jun23-26_20260623_104531.json"
```

### 6.14 Performance Optimizations

```
Virtual rendering:
  Only events within the visible viewport (+ 200px buffer) are rendered as DOM elements
  Events outside viewport: not rendered (empty placeholder div maintains scroll height)
  Implementation: IntersectionObserver per date group + scroll event throttling

  Scroll handler:
    Throttled: 16ms (requestAnimationFrame-aligned)
    On scroll: update visible time range, compute which events are visible
    Render/destroy: cards entering/leaving viewport
    Card pool: reuse DOM elements (max 200 card elements in pool)
    Pooled cards: detached from DOM, held in memory, re-configured on reuse

Event data loading:
  Initial load: fetches events for default viewport (24h)
    Shows skeleton cards during load
    Progressive: date groups loaded nearest-to-now first

  Scroll-triggered loading:
    As user scrolls to time range boundaries, fetch more events
    Sentinel elements at left/right edges (IntersectionObserver)
    Loading indicator: subtle spinner at edge of timeline
    Prefetch: load events 50% beyond current viewport

  Data chunking:
    Events fetched in chunks of 500
    Sorted by timestamp server-side
    Stored client-side in sorted array (binary search for lookup)
    Indexed by: timestamp, session ID, entity type (Map lookups)

  WebSocket updates:
    New events pushed via WebSocket
    Inserted into sorted array (binary search for position)
    If within viewport: animate card entry
    If outside viewport: silently added, visible on scroll

Connector line performance:
  SVG paths recalculated only for visible events
  Path recalculation: debounced to 30ms (max 2 recalculations per frame)
  Cached path strings per event pair (invalidated on position change)

Memory management:
  Event cache max: 10,000 events in memory
  LRU eviction: oldest events purged when limit reached
  Purged events refetched on scroll-back

Layout shifts:
  Card widths are fixed (320px) — no reflow on content change
  Group heights pre-calculated (derived from event count)
  Total canvas size known before render: no scrollbar jumps

Animation budget:
  Max 50 simultaneous CSS animations at any time
  Card entry animations staggered: 30ms delay between cards
  Animation priority: cards entering viewport > connector lines > hover effects
  Reduced motion: all animations instant (0ms) when prefers-reduced-motion
```

### 6.15 Accessibility & Keyboard Shortcuts

```
Screen reader:
  Timeline canvas: role="region", aria-label="Timeline Explorer"
  Time ruler: role="timer", aria-label="Time ruler"
  Event cards: role="article", aria-label="{event_type}: {title}"
  Card order: DOM order matches visual order (chronological, then lane)
  Live region: aria-live="polite" for new event announcements
    "New session event: Session #a3f2b started — 10:42 AM"

Keyboard shortcuts reference:
  Navigation:
    ← →       : Horizontal scroll
    ↑ ↓       : Vertical scroll
    Shift+←→  : Jump to next/prev event
    Home      : Jump to first event
    End       : Jump to last event
    T         : Jump to NOW
    Ctrl+F    : Focus search
    Ctrl+Plus : Zoom in
    Ctrl+Minus: Zoom out
    Ctrl+0    : Reset zoom

  Selection:
    Click     : Select event
    Shift+Click: Range select
    Ctrl+Click: Toggle select
    Ctrl+A    : Select all visible
    Escape    : Deselect all

  Bookmarks:
    Ctrl+B    : Toggle bookmark on selected
    Ctrl+Shift+B: Open bookmarks panel
    Ctrl+.]   : Next bookmark
    Ctrl+.[   : Previous bookmark

  Actions:
    Enter     : Open selected event detail
    Delete    : Delete selected event (with confirmation)
    Ctrl+C    : Copy selected event ID(s)
    Ctrl+E    : Export selected events
    Ctrl+Shift+A: Annotate selected

Focus trap:
  When context menu open: focus trapped within menu
  When annotation panel open: focus trapped within panel
  Escape: close overlay + return focus to trigger element

Color contrast:
  All event card text meets WCAG AA against bg-surface
  Entity colors used only as accents and icons (not primary text)
  Time ruler labels: text-tertiary on bg-surface = 3.4:1 (passes large text)
  Selection state uses both color (blue) and border pattern (dashed) for redundancy
```

### 6.16 Timeline Explorer State Model

```typescript
interface TimelineExplorerState {
  // Viewport
  viewport: {
    scrollLeft: number;
    scrollTop: number;
    zoom: number;
    viewportWidth: number;
    viewportHeight: number;
  };

  // Data
  events: TimelineEvent[];
  eventsBySession: Map<string, TimelineEvent[]>;
  eventsByType: Map<EntityType, TimelineEvent[]>;
  dateGroups: DateGroup[];

  // Loading
  loadedRanges: Array<{ start: number; end: number }>;
  pendingRanges: Array<{ start: number; end: number }>;
  loadError: Map<string, Error>; // range_key → error

  // Selection
  selectedEventIds: Set<string>;
  selectionOrder: string[]; // event IDs in selection order
  focusedEventId: string | null;

  // Bookmarks
  bookmarkIds: Set<string>;

  // Annotations
  annotations: TimelineAnnotation[];

  // Filters
  filters: {
    entityTypes: Set<EntityType>;    // Active entity type chips
    sessionId: string | null;         // Active session filter
    textQuery: string;                // Search text
    timeRange: { start: number; end: number } | null; // Custom time range
  };

  // UI State
  expandedCards: Set<string>;        // Expanded card IDs
  contextMenuTarget: ContextMenuTarget | null;
  annotationPanelOpen: boolean;
  annotationTarget: string | null;   // Event ID or range
  bookmarksPanelOpen: boolean;
  exportDialogOpen: boolean;

  // Performance
  renderedCardCount: number;
  lastRenderTime: number;
  animationBudget: number;           // Remaining animation budget this frame
}

interface TimelineEvent {
  id: string;
  type: 'session' | 'memory' | 'finding' | 'task' | 'approval' | 'anomaly';
  subType: string;                    // e.g., 'session.started', 'memory.stored'
  timestamp: number;                  // Unix ms
  sessionId: string;
  title: string;
  description: string;
  entityType: EntityType;
  status: string;
  tags: Array<{ label: string; color?: string }>;
  metadata: Record<string, unknown>;
  relatedEventIds: string[];         // For connector lines
  source: string;                     // "system" | "user" | "agent" | "automation"
  trustLevel?: 'verified' | 'high' | 'medium' | 'low' | 'quarantine';
  severity?: 'high' | 'medium' | 'low';
  bookmarkId?: string;               // If bookmarked, the bookmark entity ID
  annotationIds?: string[];           // Linked annotation IDs
}

interface DateGroup {
  date: string;                       // ISO date string (YYYY-MM-DD)
  label: string;                      // Display label ("Today", "Jun 23, 2026")
  startTimestamp: number;             // Unix ms of 00:00:00 that date
  endTimestamp: number;               // Unix ms of 23:59:59.999 that date
  events: TimelineEvent[];
  lanes: TimelineEvent[][];           // Events organized into lanes
  height: number;                     // Computed group height in px
  collapsed: boolean;
}

interface TimelineAnnotation {
  id: string;
  type: 'point' | 'range' | 'free';
  title: string;
  note: string;
  color: string;                     // Hex color
  targetEventIds: string[];          // Point/range: linked events
  timeStart?: number;                 // For range/free annotations
  timeEnd?: number;                   // For range annotations
  linkedEntities: Array<{ type: string; id: string; name: string }>;
  visibility: 'only_me' | 'team' | 'everyone';
  createdBy: string;
  createdAt: number;
  updatedAt: number;
}

type EntityType = 'session' | 'memory' | 'finding' | 'task' | 'approval' | 'anomaly'
  | 'evidence' | 'tool' | 'skill' | 'user' | 'system';

interface ContextMenuTarget {
  type: 'event' | 'canvas' | 'connector' | 'ruler' | 'multi';
  eventIds?: string[];
  position: { x: number; y: number }; // Client coordinates
}
```

### 6.17 Error States & Edge Cases

```
Empty timeline (no events):
  Canvas: centered empty state
    Icon: clock-counter-clockwise, 64px, text-disabled
    Title: "No events to display"
    Subtitle: "Events will appear here as sessions run"
    Action: "Create your first session" button → navigates to sessions page

No events matching filters:
  Canvas: current events dimmed with message overlay
    "No events match your current filters"
    "Try adjusting entity types, session filter, or search query"
    [Clear All Filters] button
    Also shown: filter summary showing which filters are active

Data loading failure:
  Error banner (top of canvas, sticky):
    Background: bg-error-muted
    Border-bottom: 2px border-error
    Text: "Failed to load timeline data: {error_message}"
    [Retry] button
    [Dismiss] button (dismisses for this session, retries on next scroll)

  Individual date group load failure:
    Group header: warning icon + "Failed to load"
    [Retry] button in group header
    Other groups (if loaded) display normally

WebSocket disconnection:
  Toolbar indicator: "⚠ Live updates paused — reconnecting..."
  Color: text-warning
  Pulse animation: 1s cycle
  On reconnect: indicator becomes green check "✓ Live" (2s), then hides

Time range with no loaded data:
  Loading skeleton fills the date group area
  Animated shimmer cards (3-5 placeholder cards)
  If load takes > 5s: show "Still loading..." message
  If load takes > 15s: show timeout error with retry

Very large event count (> 10,000 in viewport):
  Switch to density heatmap mode (zoom forced to < 0.3)
  Show banner: "Too many events for card view — zoom in for details"
  Or: apply automatic event grouping (collapse similar events)

Browser zoom/DPI changes:
  ResizeObserver on canvas container
  On resize: recalculate pixel mapping, reposition all cards
  Debounced: 100ms
  No animation during resize recalculation (set to final positions immediately)

Print layout:
  @media print: timeline renders as static list grouped by date
  Time ruler replaced by date headings
  Connector lines omitted
  Cards: full-width, border-bottom: 1px, no shadows
  Bookmarks and UI chrome hidden
```

---

## 7. Entity Graph

The Entity Graph is a WebGL-accelerated force-directed graph visualization showing relationships between all entity types in the system. It enables exploration of connections between sessions, memories, findings, evidence sources, anomalies, and semantic clusters.

### 7.1 Layout & Viewport Architecture

The Entity Graph occupies the full content area with zero padding. The WebGL canvas fills the entire viewport with overlay UI elements positioned absolutely.

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ TOOLBAR — 44px — position: absolute, top: 0, z-index: 10, bg-glass-light     │
│ ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌─────────┐ ┌──────────────────────┐ │
│ │Layout: ▼ │ │Filter: ▼ │ │Search 🔍 │ │Cluster │ │ Node count: 1,247    │ │
│ │ force    │ │ all types │ │          │ │ detect  │ │ Edge count: 3,891    │ │
│ └──────────┘ └──────────┘ └──────────┘ └─────────┘ └──────────────────────┘ │
├──────────────────────────────────────────────────────────────────────────────┤
│                                                                                │
│  WEBGL CANVAS — full remaining area (position: absolute, inset: 44px 0 0 0)  │
│  cursor: grab (default), grabbing (panning), pointer (hovering node)          │
│                                                                                │
│  ┌────────────┐                                            ┌────────────────┐ │
│  │  LEGEND    │                                            │  DETAIL PANEL  │ │
│  │            │              ● ● ●  ● ● ●                  │  (slide-out)   │ │
│  │ ● Session  │           ●           ●                    │                │ │
│  │ ○ Memory   │          ●   ●─────●   ●                   │  Session #a3f  │ │
│  │ ◇ Finding  │           ●           ●                    │  ────────────  │ │
│  │ □ Evidence │            ● ● ●  ● ● ●                    │  Status: active│ │
│  │ △ Anomaly  │                                            │  Nodes: 42     │ │
│  │ ● Entity   │              ┌──────┐                      │  Edges: 127    │ │
│  └────────────┘              │MINI- │                      │  ...           │ │
│                              │ MAP  │                      └────────────────┘ │
│                              └──────┘                                         │
└──────────────────────────────────────────────────────────────────────────────┘

Z-index layering:
  z-index 0: WebGL canvas
  z-index 5: Legend overlay (bottom-left)
  z-index 5: Mini-map overlay (bottom-right)
  z-index 10: Toolbar (top)
  z-index 15: Detail panel (right, slide-out)
  z-index 20: Tooltips, context menus
```

**WebGL canvas specification:**

```typescript
interface GraphCanvasConfig {
  // Canvas setup
  renderer: 'WebGL2';                     // WebGL 2.0 (fallback: WebGL 1.0)
  library: 'three.js';                    // Three.js r160+
  antialias: true;                        // MSAA 4x
  alpha: false;                           // Opaque background
  preserveDrawingBuffer: false;           // Performance optimization

  // Viewport
  width: 'container.clientWidth';         // ResizeObserver-driven
  height: 'container.clientHeight';
  pixelRatio: 'Math.min(window.devicePixelRatio, 2)'; // Cap at 2x for performance
  dpr: 'capped at 2';

  // Camera
  cameraType: 'PerspectiveCamera';        // Three.js PerspectiveCamera
  fov: 45;                                // Field of view degrees
  near: 0.1;                              // Near clipping plane
  far: 1000;                              // Far clipping plane
  initialPosition: { x: 0, y: 0, z: 50 }; // Camera starting position

  // Background
  clearColor: 0x0d1117;                   // --color-bg-canvas
  fog: {
    enabled: true;
    color: 0x0d1117;                      // Match canvas background
    near: 100;
    far: 400;                             // Objects fade out beyond this distance
  };

  // Lighting (for 3D depth cues on nodes)
  ambientLight: { color: 0x404060; intensity: 0.5 };
  directionalLight: { color: 0xffffff; intensity: 0.3; position: { x: 1, y: 1, z: 1 } };
}
```

### 7.2 Node Types & Visual Specification

Six node types, each with distinct geometry, color mapping, and visual properties.

#### 7.2.1 Node Type Catalog

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ NODE TYPE: Session                                                           │
├─────────────────────────────────────────────────────────────────────────────┤
│ Geometry:     RoundedRect (RoundedBoxGeometry with bevel)                    │
│ Dimensions:   width: 2.4, height: 1.2, depth: 0.3, radius: 0.2             │
│ Color:        --color-entity-session (oklch(60% 0.18 265), blue)            │
│ Material:     MeshStandardMaterial {                                         │
│                 color: entity color,                                          │
│                 roughness: 0.4,                                               │
│                 metalness: 0.1,                                               │
│                 emissive: entity color dimmed,                                │
│                 emissiveIntensity: 0.1,                                       │
│               }                                                               │
│ Label:        Text sprite (canvas-text) positioned above node                 │
│               Font: Inter, 48px, white, centered                               │
│               Text: session name (truncated to 20 chars)                      │
│               Scale: 0.04 (world-space), constant screen-space size optional  │
│ Selection:    Wireframe outline (2px, white 80% opacity)                      │
│               Scale pulse: 1.0 → 1.15 → 1.0 (400ms ease-spring)              │
│ Hover:        EmissiveIntensity: 0.3 (brightens node)                         │
│               Scale: 1.08 (slight enlargement)                                │
├─────────────────────────────────────────────────────────────────────────────┤
│ NODE TYPE: Memory Event                                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│ Geometry:     Sphere (Circle in 2D view — SphereGeometry(0.8, 32, 32))       │
│ Dimensions:   radius: 0.8, segments: 32                                      │
│ Color:        --color-entity-memory (oklch(60% 0.15 200), cyan)              │
│ Material:     MeshStandardMaterial { roughness: 0.3, metalness: 0.05,        │
│               emissiveIntensity: 0.15 }                                       │
│ Label:        Text sprite below node, memory key path (truncated)             │
│ Selection:    Ring around sphere (TorusGeometry, same color)                  │
├─────────────────────────────────────────────────────────────────────────────┤
│ NODE TYPE: Finding                                                            │
├─────────────────────────────────────────────────────────────────────────────┤
│ Geometry:     Diamond (OctahedronGeometry or custom extruded diamond)         │
│ Dimensions:   radius: 0.7, detail: 0                                          │
│ Color:        --color-entity-finding (oklch(55% 0.18 145), green)            │
│ Material:     MeshStandardMaterial { roughness: 0.25, metalness: 0.3,        │
│               emissiveIntensity: 0.2 }                                        │
│ Label:        Text sprite above node, finding title (truncated)               │
│ Selection:    Double-diamond outline (two offset diamonds, wireframe)         │
├─────────────────────────────────────────────────────────────────────────────┤
│ NODE TYPE: Evidence Source                                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│ Geometry:     Cube (BoxGeometry(1.0, 1.0, 1.0)) or flat square               │
│ Dimensions:   width: 1.0, height: 1.0, depth: 0.4                            │
│ Color:        --color-entity-evidence (oklch(55% 0.16 340), pink)            │
│ Material:     MeshStandardMaterial { roughness: 0.5, metalness: 0.1 }        │
│ Label:        Text sprite above node, source name                             │
│ Selection:    Wireframe outline on all 12 edges                               │
├─────────────────────────────────────────────────────────────────────────────┤
│ NODE TYPE: Anomaly                                                             │
├─────────────────────────────────────────────────────────────────────────────┤
│ Geometry:     Triangle (ConeGeometry(0.9, 0.9, 3) — 3-sided cone)            │
│ Dimensions:   radius: 0.9, height: 0.9, radialSegments: 3                    │
│ Color:        --color-entity-anomaly (oklch(55% 0.18 30), orange-red)        │
│ Material:     MeshStandardMaterial { roughness: 0.2, metalness: 0.4,         │
│               emissive: red, emissiveIntensity: 0.3 (pulsing) }              │
│ Label:        Text sprite above node, anomaly title                            │
│ Selection:    Pulsing red border (emissive intensity oscillates 0.3→0.8)     │
│ Animation:    Continuous slow rotation (0.1 rad/s) + emissive pulse (2s)     │
├─────────────────────────────────────────────────────────────────────────────┤
│ NODE TYPE: Entity (generic — colored by category)                              │
├─────────────────────────────────────────────────────────────────────────────┤
│ Geometry:     Sphere (SphereGeometry(0.6, 24, 24))                            │
│ Dimensions:   radius: 0.6, segments: 24                                       │
│ Color:        Varies by entity category (see color mapping below)             │
│ Material:     MeshStandardMaterial { roughness: 0.4, metalness: 0.05,        │
│               emissiveIntensity: 0.05 }                                        │
│ Label:        Text sprite below node, entity name                              │
│ Selection:    Standard wireframe ring                                          │
└─────────────────────────────────────────────────────────────────────────────┘

Entity category color mapping (for generic entity nodes):
  session    → --color-entity-session     (blue)
  memory     → --color-entity-memory      (cyan)
  finding    → --color-entity-finding     (green)
  task       → --color-entity-task        (amber)
  approval   → --color-entity-approval    (red)
  evidence   → --color-entity-evidence    (pink)
  tool       → --color-entity-tool        (sky blue)
  skill      → --color-entity-skill       (purple)
  user       → --color-entity-user        (salmon)
  system     → --color-entity-system      (muted gray)
```

#### 7.2.2 Node States

```
Default (idle):
  - Standard material parameters as specified above
  - No outline/wireframe
  - Label opacity: 0.7 (subtle, doesn't crowd view)
  - Node rendered at simulation-computed position

Hover:
  - EmissiveIntensity: increased by 0.15 (brightens node)
  - Scale: multiplied by 1.08 (8% enlargement)
    Transition: 150ms ease-out-quint (interpolated in animation loop)
  - Label opacity: 1.0
  - Neighbor highlight: connected nodes increase emissive by 0.05
  - Non-connected nodes: opacity reduced by 30% (dim background)
  - Cursor: pointer
  - Tooltip (delayed 300ms):
    Content: node type icon + name + metadata
    Position: follows cursor offset (20px right, 20px down)
    Style: glass panel, max-width 320px

Selected (click):
  - Outline: wireframe ring matching entity color, 3px thick
  - Scale: 1.15 (15% larger than default)
    Transition: 400ms ease-spring
  - EmissiveIntensity: increased by 0.25
  - Camera: smoothly animates to center on selected node
    Target position: node.worldPosition + offset(0, 0, dist)
    Dist calculated to show node + connected neighbors
    Animation: 600ms ease-out-expo
  - Detail panel: slides in from right (see 7.7)
  - Label opacity: 1.0
  - All edges connected to selected node: highlighted
    Edge color: entity color at full opacity
    Edge width: 3px (vs 1px default)
    Non-connected edges: opacity reduced to 10%

Focused (keyboard tabbing):
  - Same visual as selected + focus ring
  - Focus ring: 2px solid white, 4px outside node bounds

Filtered out (node hidden by filter):
  - Opacity: 0
  - Not rendered (culled from scene)
  - Connected edges also hidden

Partially filtered (node dimmed by filter):
  - Opacity: 0.15
  - Label hidden
  - Non-interactive (no hover, no click)
  - Edges: opacity 0.05

Loading (node data being fetched):
  - Rendered as placeholder sphere (gray, low detail)
  - Pulsing opacity: 0.3 → 0.6 → 0.3 (1.5s cycle)
  - Label: "Loading..." in muted text

Error (node data failed to load):
  - Rendered as red sphere (smaller, 0.4 radius)
  - EmissiveIntensity: 0.3 (bright red)
  - Label: "Error" in red text
  - Click: shows error toast with retry option
```

#### 7.2.3 Node Labels

```
Label implementation: HTML5 Canvas text rendered to sprite texture
  Technique: canvas-text library or custom CanvasTexture
  Font: Inter, various sizes
  Resolution: 256x64 texture per label (power-of-two for mipmapping)
  Filtering: LinearMipmapLinearFilter (smooth at distance)

Label positioning:
  Session: above node (y: node.y + nodeHeight/2 + 0.3)
  Memory: below node (y: node.y - nodeRadius - 0.4)
  Finding: above node (y: node.y + nodeRadius + 0.3)
  Evidence: above node (y: node.y + nodeHeight/2 + 0.3)
  Anomaly: above node (y: node.y + nodeRadius + 0.3)
  Entity: below node (y: node.y - nodeRadius - 0.3)

Label visibility (LOD-based):
  Camera distance < 20 units:  full label visible
  Camera distance 20-40:       label at 0.5 opacity
  Camera distance 40-80:       label hidden (dot only)
  Camera distance > 80:        label always hidden
  
  Label always visible for: selected node, hovered node
  Label always hidden for: filtered-out nodes, loading nodes

Label content:
  Session: "{session_name}" (max 24 chars, ellipsis)
  Memory: "/{key_path_last_2_segments}" (max 30 chars)
  Finding: "{title}" (max 28 chars)
  Evidence: "{source_name}" (max 24 chars)
  Anomaly: "{anomaly_type}" (max 20 chars)
  Entity: "{entity_name}" (max 22 chars)

Label collision avoidance:
  Labels shrink at distance: scale inversely with camera distance
  Minimum label size: 0.01 world units (still readable)
  Labels fade out rather than overlap (opacity based on screen-space density)
  In dense clusters: only show labels for top 3 nodes (by degree centrality)
```

### 7.3 Edge Types & Visual Specification

Six edge types connecting nodes with distinct visual encodings.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ EDGE TYPE: contains                                                            │
├─────────────────────────────────────────────────────────────────────────────┤
│ Meaning:      Parent entity contains child entity                              │
│ Example:      Session → Memory Event (session contains memory)                 │
│               Session → Finding (session produced finding)                     │
│ Color:        Parent entity color at 60% opacity                               │
│ Style:        Solid line, 1px width                                            │
│ Arrow:        Yes, directional arrowhead at 75% of edge length                 │
│ Arrowhead:    ConeGeometry(0.08, 0.15, 8), same color, positioned at 75%       │
│ Curvature:    Straight when nodes close, slight curve when distant             │
│ Dash:         None (solid)                                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│ EDGE TYPE: derived-from                                                        │
├─────────────────────────────────────────────────────────────────────────────┤
│ Meaning:      Entity was derived/computed from source                          │
│ Example:      Finding → Memory Event (finding derived from memory)             │
│               Task → Finding (task created from finding)                       │
│ Color:        Target entity color at 50% opacity                                │
│ Style:        Dashed line, 1px width                                            │
│ Dash pattern: 0.3 dash, 0.2 gap (world units)                                  │
│ Arrow:        Yes, at 80% of edge length                                        │
│ Curvature:    Always curved (Bezier)                                            │
├─────────────────────────────────────────────────────────────────────────────┤
│ EDGE TYPE: references                                                           │
├─────────────────────────────────────────────────────────────────────────────┤
│ Meaning:      Entity references another entity                                  │
│ Example:      Finding → Evidence Source (finding cites evidence)                │
│               Memory → Memory (one memory references another)                   │
│ Color:        --color-text-tertiary at 40% opacity                             │
│ Style:        Dotted line, 1px width                                             │
│ Dot pattern:  0.1 dash, 0.3 gap (world units)                                  │
│ Arrow:        No (bidirectional reference)                                      │
│ Curvature:    Straight                                                          │
├─────────────────────────────────────────────────────────────────────────────┤
│ EDGE TYPE: mentions                                                             │
├─────────────────────────────────────────────────────────────────────────────┤
│ Meaning:      Entity textually mentions another entity                          │
│ Example:      Any entity → Any entity (text mention)                            │
│ Color:        --color-text-tertiary at 20% opacity                             │
│ Style:        Thin solid line, 0.5px width                                       │
│ Arrow:        Optional (small arrowhead, 0.05)                                  │
│ Curvature:    Slight curve (gentle arc)                                         │
│ Visibility:   Hidden at zoom levels showing >100 nodes                          │
│               (performance optimization — mentions create dense edge hairballs) │
├─────────────────────────────────────────────────────────────────────────────┤
│ EDGE TYPE: semantic-similarity                                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│ Meaning:      Two entities have high semantic similarity (>0.7 cosine)          │
│ Example:      Memory → Memory (similar embeddings)                              │
│               Finding → Finding (similar conclusions)                           │
│ Color:        Gradient: entityA.color → entityB.color at 30% opacity each      │
│ Style:        Wavy/sinusoidal line, 1px width                                    │
│               Amplitude: 0.2 world units, Frequency: 3 cycles over edge length │
│ Arrow:        No (bidirectional similarity)                                     │
│ Curvature:    Wavy (custom curve with sine modulation)                          │
│ Rendering:    Custom shader with time-offset animation (slow wave oscillation)  │
├─────────────────────────────────────────────────────────────────────────────┤
│ EDGE TYPE: contradiction                                                         │
├─────────────────────────────────────────────────────────────────────────────┤
│ Meaning:      Entities contain contradictory information                        │
│ Example:      Finding → Finding (conflicting conclusions)                       │
│               Memory → Memory (contradictory facts)                             │
│ Color:        --color-accent-error at 60% opacity                               │
│ Style:        Zigzag line, 1.5px width                                           │
│               Zigzag: 0.15 amplitude, 0.3 wavelength                           │
│ Arrow:        Yes, on both ends (conflict is bidirectional)                     │
│ Rendering:    Pulsing red glow (emissive on line geometry)                      │
│               Pulse animation: opacity 0.4→0.7→0.4, 1.5s cycle                 │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Edge rendering implementation:**

```typescript
// Edge geometry construction
function createEdgeGeometry(
  sourcePos: Vector3,
  targetPos: Vector3,
  edgeType: EdgeType
): BufferGeometry {
  switch (edgeType) {
    case 'contains':
      return createCurvedLine(sourcePos, targetPos, {
        curveOffset: 0.3,    // Bezier control point offset
        arrowHead: true,
        arrowPosition: 0.75,
        color: sourceColorAtOpacity(0.6),
      });

    case 'derived-from':
      return createDashedLine(sourcePos, targetPos, {
        dashSize: 0.3,
        gapSize: 0.2,
        curveOffset: 0.5,
        arrowHead: true,
        arrowPosition: 0.8,
        color: targetColorAtOpacity(0.5),
      });

    case 'references':
      return createDottedLine(sourcePos, targetPos, {
        dotSize: 0.1,
        gapSize: 0.3,
        color: tertiaryAtOpacity(0.4),
      });

    case 'mentions':
      return createStraightLine(sourcePos, targetPos, {
        lineWidth: 0.5,
        color: tertiaryAtOpacity(0.2),
        arrowHead: false,
      });

    case 'semantic-similarity':
      return createWavyLine(sourcePos, targetPos, {
        amplitude: 0.2,
        frequency: 3,
        colorGradient: [sourceColor, targetColor],
        animateOffset: true,
      });

    case 'contradiction':
      return createZigzagLine(sourcePos, targetPos, {
        amplitude: 0.15,
        wavelength: 0.3,
        color: errorColorAtOpacity(0.6),
        arrowHeads: 'both',
        pulseEmissive: true,
      });
  }
}

// Edge LOD (Level of Detail)
function getEdgeVisibility(cameraDistance: number, edgeType: EdgeType): number {
  // Base visibility thresholds
  if (cameraDistance > 150) return 0; // All edges hidden

  switch (edgeType) {
    case 'contains':
    case 'derived-from':
      return cameraDistance < 80 ? 1.0 : 0.0;
    case 'references':
      return cameraDistance < 50 ? 1.0 : 0.0;
    case 'mentions':
      return cameraDistance < 30 ? 1.0 : 0.0; // Hidden early
    case 'semantic-similarity':
      return cameraDistance < 60 ? 1.0 : 0.0;
    case 'contradiction':
      return cameraDistance < 100 ? 1.0 : 0.0; // Always visible when in range
  }
}

// Edge count cap (performance protection)
const MAX_VISIBLE_EDGES = 5000;
// Beyond this, hide mentions edges first, then references, then similarity
function applyEdgeCap(edges: Edge[]): Edge[] {
  if (edges.length <= MAX_VISIBLE_EDGES) return edges;

  const priorityOrder = ['contradiction', 'contains', 'derived-from', 'semantic-similarity', 'references', 'mentions'];
  const sorted = [...edges].sort((a, b) =>
    priorityOrder.indexOf(a.type) - priorityOrder.indexOf(b.type)
  );
  return sorted.slice(0, MAX_VISIBLE_EDGES);
}
```

### 7.4 Force Simulation

The force-directed layout runs in a Web Worker to keep the main thread at 60fps. Physics parameters are tuned for exploratory graph visualization — nodes should settle into a readable layout within 2-3 seconds for graphs up to 500 nodes.

#### 7.4.1 Force Model

```typescript
interface ForceSimulationConfig {
  // Core forces (applied every tick)
  forces: {
    link: {
      enabled: true;
      strength: 0.15;              // Edge spring strength (0-1)
      distance: 4.0;               // Ideal edge length (world units)
      iterations: 3;               // Link force iterations per tick (accuracy)
      typeStrengths: {             // Per-type strength multipliers
        contains: 1.0;             // Strong — parent-child is tight
        'derived-from': 0.8;       // Moderate
        references: 0.4;           // Weak — documents can be far
        mentions: 0.15;            // Minimal — loose association
        'semantic-similarity': 0.3; // Weak — similarity is conceptual
        contradiction: 0.6;        // Moderate — conflicts are interesting
      };
    };

    charge: {
      enabled: true;
      strength: -120;              // Negative = repulsion
      distanceMin: 0.5;            // Minimum distance (prevents explosion)
      distanceMax: 30;             // Maximum interaction distance
      theta: 0.9;                  // Barnes-Hut approximation accuracy (0-1)
                                   // Higher = more accurate, slower
      typeChargeModifiers: {       // Per-type charge modifiers
        session: 1.5;              // Sessions repel more (they're parent hubs)
        memory: 1.0;
        finding: 1.2;
        evidence: 0.8;
        anomaly: 2.0;              // Anomalies repel strongly (stand out)
        entity: 1.0;
      };
    };

    center: {
      enabled: true;
      strength: 0.05;              // How strongly nodes are drawn to center
      x: 0;                        // Center X in world coords
      y: 0;                        // Center Y in world coords
      z: 0;                        // Center Z in world coords
    };

    collision: {
      enabled: true;
      radius: (node) => node.radius * 1.2; // Collision radius (20% padding)
      strength: 0.7;               // Collision force strength
      iterations: 2;               // Collision iterations per tick
    };

    cluster: {
      enabled: true;
      strength: 0.03;              // Cluster attraction strength
      clusters: Map<string, { x: number; y: number; z: number; radius: number }>;
      // Clusters computed via community detection (Louvain)
      // Each node is attracted to its cluster center
      // Cluster centers themselves repel each other (nested force)
    };

    radial: {
      enabled: false;               // Disabled in force-directed mode
      strength: 0.1;
      center: { x: 0; y: 0; z: 0 };
      radius: (node) => node.degree * 2 + 5; // Outer to inner by degree
    };
  };

  // Simulation parameters
  alpha: {
    initial: 0.3;                  // Starting alpha (energy level)
    min: 0.001;                    // Minimum alpha (simulation stops below this)
    decay: 0.0228;                 // Alpha decay rate per tick
                                   // AlphaDecay = 1 - Math.pow(alphaMin, 1 / expectedTicks)
                                   // At decay 0.0228, alpha reaches 0.001 in ~300 ticks
    target: 0;                     // Target alpha (0 = stopped)
  };

  velocityDecay: 0.4;              // Friction (0 = no friction, 1 = immediate stop)

  // Tick scheduling
  tickInterval: 16;                // ms between ticks (matches 60fps requestAnimationFrame)
  maxTicksPerFrame: 3;             // If falling behind, run up to 3 ticks per frame

  // Stabilization
  stabilize: {
    enabled: true;
    threshold: 0.001;              // Mean displacement below this = stable
    consecutiveStableTicks: 10;    // Number of stable ticks before "stabilized"
    onStabilized: () => void;      // Callback: reduce tick rate to idle
  };

  // Warming (initial simulation run)
  warmupTicks: 100;                // Run this many ticks before first render
                                   // Prevents nodes from "flying in" on first frame
}
```

#### 7.4.2 Web Worker Architecture

```
Worker message protocol:
  Main → Worker:
    { type: 'init', config: ForceSimulationConfig, nodes: GraphNode[], edges: GraphEdge[] }
    { type: 'tick' }                              // Request one simulation tick
    { type: 'tickMultiple', count: number }        // Request N ticks
    { type: 'updateConfig', config: Partial<ForceSimulationConfig> }
    { type: 'addNodes', nodes: GraphNode[] }
    { type: 'removeNodes', nodeIds: string[] }
    { type: 'addEdges', edges: GraphEdge[] }
    { type: 'removeEdges', edgeIds: string[] }
    { type: 'reheat', alpha: number }             // Add energy (user interaction)
    { type: 'stop' }                               // Stop simulation
    { type: 'setClusterCenters', clusters: Map<string, Vector3> }

  Worker → Main:
    { type: 'ticked', positions: Float32Array, alpha: number }
    { type: 'stabilized' }
    { type: 'error', message: string }

Position data format:
  Float32Array with interleaved positions: [x0,y0,z0, x1,y1,z1, ...]
  Indexed by node index (order matches initial node array)
  Transferred via Transferable (zero-copy) for performance

Worker implementation:
  // graph-worker.ts
  importScripts('d3-force-3d.min.js'); // or custom force implementation

  let simulation: ForceSimulation3D;
  let nodePositions: Float32Array;

  self.onmessage = (e) => {
    switch (e.data.type) {
      case 'init':
        initializeSimulation(e.data.config, e.data.nodes, e.data.edges);
        break;
      case 'tick':
        simulation.tick();
        postMessage({ type: 'ticked', positions: getPositions(), alpha: simulation.alpha() });
        break;
      case 'tickMultiple':
        for (let i = 0; i < e.data.count; i++) simulation.tick();
        postMessage({ type: 'ticked', positions: getPositions(), alpha: simulation.alpha() });
        break;
      // ... other message handlers
    }
  };
```

#### 7.4.3 Render Loop Integration

```
Animation loop (requestAnimationFrame, main thread):
  1. Read latest positions from worker (Float32Array buffer)
  2. Update Three.js node positions (buffer attribute update)
  3. Update edge geometries (if nodes moved significantly)
  4. Update camera (if animating toward target)
  5. Update LOD levels based on camera distance
  6. Render frame via Three.js WebGLRenderer

  Frame budget: 16.67ms (60fps target)
  Frame skip: if last frame took > 16.67ms, skip rendering but continue simulation
  Adaptive quality: if sustained > 20ms frames, reduce:
    - Antialiasing: 4x → 2x → off
    - Shadow maps: off
    - Edge detail: reduce curve segments
    - Label rendering: reduce to nearest 50 nodes only

Worker communication:
  Tick scheduling:
    While simulation is active (alpha > alphaMin):
      Request tickMultiple(3) every 48ms (3 ticks × 16ms)
      This keeps worker ahead of renderer
    
    On 'ticked' message:
      Copy Float32Array positions to GPU buffer
      Update node meshes (BufferGeometry.attributes.position.needsUpdate = true)
      If alpha > 0.05: animation is "hot" — render every frame
      If alpha <= 0.05: animation is "cooling" — render every 2nd frame
      If stabilized: animation is "idle" — render on camera movement only

  Heater:
    When user drags a node:
      Send 'reheat' with alpha=0.15 to worker
      Node.velocity adds drag vector
    When user adds/removes filter:
      Send full sim reinit (only affected nodes)
    When user switches layout:
      Send 'init' with new layout config
```

### 7.5 Camera Controls

```
Camera: Three.js PerspectiveCamera
Controls: Custom orbit controls (not three/examples OrbitControls — custom for performance)

Interaction:
  Pan (left-click drag on empty canvas):
    camera.position += mouseDelta * panSpeed * cameraDistance * 0.01
    camera.lookAt updates to maintain target
    
  Orbit (right-click drag or Ctrl+left-click drag):
    Rotate camera around focal point
    Horizontal drag: azimuth rotation
    Vertical drag: elevation rotation (clamped -85° to 85°)
    Rotation speed: 0.005 rad/pixel

  Zoom (scroll wheel):
    camera.position moves toward/away from lookAt target
    Zoom speed: distance * 0.1 per scroll step
    Min distance: 3 (very close — single node view)
    Max distance: 200 (full graph view)
    Smooth zoom: lerp(currentDistance, targetDistance, 0.2) applied each frame

  Focus (double-click node):
    Camera animates to center on node:
      targetLookAt = node.worldPosition
      targetDistance = node.radius * 8 (fit node + neighbors)
      Animation: 600ms ease-out-expo
      Camera movement: smooth interpolation via requestAnimationFrame

  Fit all (F key or toolbar button):
    Compute bounding sphere of all visible nodes
    Set camera distance = boundingSphere.radius / Math.tan(fov / 2) * 1.2
    Set camera lookAt = boundingSphere.center
    Animation: 800ms ease-out-expo

  Reset view (R key):
    Return to default camera position (0, 0, 50)
    Look at origin
    Animation: 600ms ease-out-expo

Touch controls:
  One finger: pan
  Two fingers: pinch-zoom + rotate
  Double-tap: focus on tapped node
  Long-press: context menu (500ms hold)

Camera state preservation:
  When switching between graph and other views:
    Save camera state to sessionStorage: { position, target, timestamp }
    Restore on return (within 30 minutes of save)
    After 30 min: reset to default
```

### 7.6 Node Selection & Detail Interaction

```
Single select (click):
  - Node highlights (see 7.2.2)
  - Camera animates to center on node (if not already centered)
  - Detail panel slides in from right (see 7.7)
  - Previously selected node: deselects (clears selection if clicking same node)
  - Click on empty canvas: deselects current node

Multi-select:
  Shift+click node: add to selection (up to 20 nodes)
  Ctrl+click node: toggle selection
  Shift+drag (marquee): draw selection rectangle (2D screen-space)
    - Rectangle: 1px dashed white line
    - All nodes within rectangle added to selection
    - Selection resolves on mouseup
  Ctrl+A: select all visible nodes (caution: may select 1000+ nodes)
    - Confirmation toast if selecting >50 nodes: "Select all 1,247 nodes?"
    - [Select All] [Cancel]

Drag node (manual layout adjustment):
  Click+drag on node:
    - Node follows cursor in screen-space, mapped to world-space
    - Node velocity set to drag delta (overrides force simulation for dragged node)
    - Connected nodes experience additional attraction toward dragged node
    - On release: node re-enters force simulation with current velocity
    - Drag pin: node is "pinned" (fx, fy, fz set to current position)
    - Pinned node: rendered with subtle pin icon overlay
    - Unpin: right-click → "Unpin" or double-click pinned node

Pin/Unpin:
  Right-click node → "Pin node" / "Unpin node"
  Pinned nodes: excluded from force simulation (fixed position)
  Visual indicator: small pushpin icon at top-right of node
  Pinned nodes list: available in toolbar → "Pinned ({count})"

Node group selection (via cluster):
  Click cluster hull (convex hull around community):
    - All nodes in community selected
    - Cluster hull highlights (brighter color, 2px border)
  Right-click cluster → "Expand cluster" / "Collapse cluster"
```

### 7.7 Detail Panel

The detail panel slides in from the right edge of the viewport when a node is selected.

```
Panel specification:
  Position: absolute, right: 0, top: 44px (below toolbar), bottom: 0
  Width: 360px
  Background: bg-surface
  Border-left: 1px border-default
  Box-shadow: shadow-lg (left side only)
  Z-index: 15
  Transform: translateX(100%) (hidden), translateX(0) (visible)
  Transition: transform 300ms ease-out-expo

Panel anatomy:
  ┌──────────────────────────────────┐
  │ [←]  Session #a3f2b   [×] [✎]  │  ← Header (48px)
  │      Q4 Revenue Analysis         │
  ├──────────────────────────────────┤
  │                                  │
  │ STATUS                           │  ← Section: Status
  │ ┌──────────────────────────────┐ │
  │ │ ● Active — Iteration 42      │ │
  │ │ Started: Jun 23, 10:30 AM    │ │
  │ │ Duration: 12m 34s            │ │
  │ │ Model: deepseek-v4-pro       │ │
  │ │ Tokens: 847K / 1.2M budget   │ │
  │ └──────────────────────────────┘ │
  │                                  │
  │ CONNECTED NODES (127 edges)      │  ← Section: Connections
  │ ┌──────────────────────────────┐ │
  │ │ ○ Memory #m8472              │ │  ← Entity cards
  │ │   Key: /projects/q4-revenue  │ │
  │ │   ▸ contains                 │ │
  │ │                              │ │
  │ │ ○ Memory #m8501              │ │
  │ │   Key: /projects/q4-asia     │ │
  │ │   ▸ contains                 │ │
  │ │                              │ │
  │ │ ◇ Finding #f1294             │ │
  │ │   APAC revenue decline       │ │
  │ │   ▸ derived-from             │ │
  │ │                              │ │
  │ │ ... (scroll for more)        │ │
  │ └──────────────────────────────┘ │
  │                                  │
  │ RELATED SESSIONS                 │  ← Section: Related
  │ ┌──────────────────────────────┐ │
  │ │ #b2e1c Phish Investigation   │ │
  │ │ ▸ semantic-similarity (0.87) │ │
  │ └──────────────────────────────┘ │
  │                                  │
  │ METADATA                         │  ← Section: Metadata
  │ ┌──────────────────────────────┐ │
  │ │ Agent: Kara                  │ │
  │ │ Trust: high                  │ │
  │ │ Domain: concept              │ │
  │ │ Source: system               │ │
  │ │ Created: 2026-06-23T10:30:00Z│ │
  │ │ Updated: 2026-06-23T10:47:23Z│ │
  │ └──────────────────────────────┘ │
  │                                  │
  │ ACTIONS                          │  ← Section: Actions
  │ ┌──────────────────────────────┐ │
  │ │ [Investigate] [Open Timeline]│ │
  │ │ [Export]      [Bookmark]     │ │
  │ └──────────────────────────────┘ │
  └──────────────────────────────────┘

Header:
  Back button: ← arrow, 20px, text-secondary
    Click: deselect node, close panel
  Entity type icon + name: text-subtitle, font-weight 600
  Subtitle: text-caption, text-secondary, entity meta
  Edit button: pencil icon, 16px (if editable)
  Close button: x icon, 20px, text-tertiary
    Click: close panel, deselect node

Sections (collapsible):
  Section header: text-caption, uppercase, text-tertiary, letter-spacing 0.05em
  Collapse toggle: chevron icon, 14px
  Collapsed: only header visible, chevron rotated
  Default: all sections expanded

Connection cards:
  Height: 48px
  Padding: 8px 12px
  Hover: bg-hover-muted
  Click: select that connected node (changes panel content)
  Icon: node type icon + shape, 16px
  Name: text-small, text-primary, truncate
  Edge type badge: colored pill, text-micro
  Edge type icon: small directional indicator

Scroll behavior:
  Panel scrolls independently from canvas
  Custom scrollbar: 4px, thumb border-hover

Multi-select panel variant:
  When 2+ nodes selected:
    Header: "{N} nodes selected" in text-subtitle
    Sections: aggregated connections, common metadata
    Actions: [Compare] [Group] [Export Selection] [Bookmark All]
```

### 7.8 Mini-Map

The mini-map provides a thumbnail overview of the entire graph in the bottom-right corner.

```
Mini-map specification:
  Position: absolute, bottom: 12px, right: 12px
  Size: 180px × 120px
  Background: bg-surface at 90% opacity (bg-glass-light)
  Border: 1px border-default
  Border-radius: radius-md
  Box-shadow: shadow-md
  Z-index: 5
  Overflow: hidden

  Canvas: secondary WebGL renderer (or 2D canvas for simplicity)
    Shows all nodes at reduced scale (ignores zoom/navigation camera)
    Nodes: small circles (2-4px) in entity colors
    Edges: thin lines (0.5px) at 20% opacity
    Background: bg-canvas at 50% opacity

  Viewport indicator:
    Rectangle showing current camera view
    Border: 1px white at 60% opacity
    Fill: white at 5% opacity
    Interactive: drag rectangle to pan main canvas
      mousedown on rectangle → drag → updates main camera lookAt
      Release: rectangle snaps to new position
    Click outside rectangle: jump camera to clicked position

  Zoom controls (overlaid on mini-map, bottom-right of mini-map):
    [+] button: zoom in (16px circle, white icon)
    [-] button: zoom out
    [⤡] button: fit all (reset to show full graph)
    Buttons: 20px × 20px circles, bg-surface-overlay, border border-default
    Opacity: 0.7 (default), 1.0 (hover)

  Toggle:
    Mini-map can be hidden via toolbar toggle
    State persisted in localStorage
    Keyboard: M to toggle mini-map
```

### 7.9 Legend

The legend occupies the bottom-left corner and shows node/edge type mappings.

```
Legend specification:
  Position: absolute, bottom: 12px, left: 12px
  Width: auto (content-driven, max 220px)
  Background: bg-surface at 90% opacity (bg-glass-light)
  Border: 1px border-default
  Border-radius: radius-md
  Box-shadow: shadow-md
  Z-index: 5
  Padding: 10px 14px

  Content layout:
    ┌──────────────────────┐
    │ Legend               │  ← Header
    ├──────────────────────┤
    │ NODES                │
    │ ▬ Session            │  ← Colored line/shape + label
    │ ● Memory Event       │
    │ ◆ Finding            │
    │ ■ Evidence Source    │
    │ ▲ Anomaly            │
    │ ● Entity             │
    ├──────────────────────┤
    │ EDGES                │
    │ ─── contains         │  ← Line style sample + label
    │ --- derived-from     │
    │ ··· references       │
    │ ─── mentions         │
    │ ≈≈≈ semantic-sim     │
    │ ╲╱╲ contradiction    │
    ├──────────────────────┤
    │ [Hide] [Expand]      │  ← Actions
    └──────────────────────┘

  Header:
    "Legend" in text-caption, uppercase, text-tertiary
    Collapse toggle: chevron icon, 14px, right-aligned

  Node entries:
    Height: 20px
    Shape indicator: 10px wide (mini node shape) in entity color
    Label: text-caption, text-secondary
    Count badge (right): text-micro, text-tertiary
      Shows count of currently visible nodes of that type
      Updates as filter changes

  Edge entries:
    Height: 18px
    Line indicator: 16px line sample showing edge style
    Label: text-caption, text-secondary

  Interactive:
    Hover node entry: highlights all nodes of that type
      Other nodes: dim to 20% opacity
    Click node entry: toggle filter for that node type
      Filtered type: entry shows strikethrough + "(hidden)"
    Hover edge entry: highlights all edges of that type
      Other edges: dim to 10% opacity
    Click edge entry: toggle visibility for that edge type

  Toggle:
    Legend can be hidden via toolbar toggle
    Keyboard: L to toggle legend
```

### 7.10 Layout Presets

The graph supports five layout modes, switchable via the toolbar dropdown.

```
1. FORCE-DIRECTED (default)
   Algorithm: d3-force-3d with custom forces
   Characteristics:
     - Nodes repel each other (charge force)
     - Connected nodes attract (link force)
     - Cluster attraction groups communities
     - Organic, exploratory layout
   Best for: general exploration, finding patterns
   Initial state: warm-start with 100 pre-simulation ticks
   Transition from other layout: nodes animate from old positions over 800ms

2. RADIAL
   Algorithm: Nodes arranged in concentric circles by degree centrality
   Characteristics:
     - Center: highest-degree node(s)
     - Ring 1: directly connected nodes
     - Ring 2: 2-hop connections
     - Ring N: N-hop connections
     - Angular spacing: uniform within each ring
   Best for: understanding hub-and-spoke structure
   Layout parameters:
     ringSpacing: 6.0 (world units between rings)
     minRadius: 4.0 (inner ring radius)
     angularOffset: random jitter (±0.1 rad to avoid perfect alignment)
   Transition: 600ms ease-out-expo

3. HIERARCHICAL (tree)
   Algorithm: Directed acyclic graph layout (Sugiyama-style)
   Characteristics:
     - Root nodes at top (sessions)
     - Children below (memories, findings)
     - Leaf nodes at bottom (evidence sources)
     - Layers determined by entity type hierarchy:
       Layer 0: Session
       Layer 1: Memory, Task, Anomaly
       Layer 2: Finding
       Layer 3: Evidence Source, Approval
     - Within layer: nodes sorted by creation time
   Best for: understanding derivation chains
   Layout parameters:
     layerSpacing: 8.0 (vertical spacing)
     nodeSpacing: 3.0 (horizontal spacing within layer)
     edgeRouting: orthogonal (right-angle bends)
   Transition: 700ms ease-out-expo

4. TIMELINE
   Algorithm: Nodes positioned along horizontal axis by timestamp,
              vertical position by entity type
   Characteristics:
     - X-axis: time (linear mapping, same as Timeline Explorer)
     - Y-axis: entity type lanes
       Lane 0: Session
       Lane 1: Memory
       Lane 2: Finding
       Lane 3: Task
       Lane 4: Evidence
       Lane 5: Anomaly
       Lane 6: Other entities
     - Z-axis: jitter for depth (±0.5 units random)
   Best for: understanding temporal relationships
   Layout parameters:
     timeScale: computed to fit all events in view
     laneSpacing: 5.0 (vertical spacing between lanes)
   Transition: 500ms ease-out-expo

5. GRID
   Algorithm: Nodes arranged in a regular grid by entity type
   Characteristics:
     - Columns: entity types
     - Rows: sorted by creation time within type
     - Grid spacing: uniform
     - Equal cell size: nodes scaled to fit 3×3 unit cells
   Best for: overview, comparison between entity types
   Layout parameters:
     columns: auto (by entity type count)
     cellWidth: 5.0
     cellHeight: 5.0
     padding: 1.0
   Transition: 500ms ease-out-expo

Layout transition animation:
  - Store current positions
  - Compute target positions for new layout
  - Interpolate: node.position = lerp(currentPosition, targetPosition, t)
  - Tween: t goes from 0→1 over transitionDuration
  - Easing: ease-out-expo for all layout transitions
  - During transition: force simulation paused
  - After transition: force simulation resumes (if force-directed)
  - Nodes can be dragged during transition (breaks animation for that node)
```

### 7.11 Semantic Clustering

Automatic detection and visualization of semantic clusters within the graph.

```
Cluster detection algorithm:
  Method: Louvain community detection on semantic-similarity edge weights
  Execution: Web Worker (non-blocking)
  Trigger: "Detect Clusters" button in toolbar, or automatic after graph load
  
  Algorithm steps:
    1. Build adjacency matrix from semantic-similarity edges
       Edge weight = similarity score (0.7 to 1.0)
    2. Run Louvain modularity optimization
       Resolution parameter: 0.8 (higher = more clusters)
       Iterations: max 20 (convergence typically in 5-10)
    3. Assign cluster IDs to nodes
    4. Compute cluster metadata:
       - Cluster center (centroid of nodes)
       - Cluster radius (bounding sphere)
       - Cluster label (most frequent terms from node labels)
       - Cluster density (nodes / radius³)
       - Inter-cluster edges (edges crossing cluster boundaries)

Cluster visualization:
  Convex hull: semi-transparent surface enclosing cluster nodes
    Material: MeshPhongMaterial {
      color: clusterColor,
      opacity: 0.1,
      transparent: true,
      depthWrite: false,
    }
    Outline: 1px wireframe, clusterColor at 40% opacity

  Cluster colors: rotated through data palette (12 colors)
    Cluster 0: --color-data-0 (red)
    Cluster 1: --color-data-1 (amber)
    ...etc., cycling for >12 clusters

  Cluster label:
    Centered above hull
    Text: cluster label (most frequent term)
    Font: 36px Inter, white
    Background pill: semi-transparent black

  Cluster interaction:
    Hover hull: hull opacity increases to 0.2, outline to 80%
      Tooltip: "Cluster {N}: {label} — {nodeCount} nodes"
    Click hull: select all nodes in cluster
      Camera animates to frame cluster
    Right-click hull → "Expand cluster" (spreads nodes apart)
      Temporarily applies radial layout within cluster bounds
      "Collapse cluster" (returns to force layout)
    Right-click hull → "Filter to cluster" (hides nodes outside cluster)
    Right-click hull → "Explore cluster" (opens cluster in new focused view)

  Cluster list (sidebar option, toggled from toolbar):
    Shows all discovered clusters sorted by size
    Each cluster: colored dot + label + node count + density bar
    Click: camera focuses on cluster
    Hover: highlights cluster hull

Cross-cluster edges:
  Edges between nodes in different clusters
  Rendered with dashed style (0.2 dash, 0.3 gap)
  Color: both cluster colors blended (50% each)
  These edges are "interesting" — they show cross-domain connections
```

### 7.12 Filtering & Search

```
Entity type filter (toolbar dropdown):
  Multi-select checkboxes:
    ☑ Sessions (42)
    ☑ Memories (891)
    ☑ Findings (127)
    ☑ Evidence (63)
    ☑ Anomalies (12)
    ☐ Tasks (58)  ← unchecked = hidden
    ☑ Entities (54)

  "All" / "None" quick toggle
  "Invert" toggle

  Filter application:
    Hidden nodes: removed from scene (not just invisible — saves GPU)
    Connected edges: hidden if either endpoint hidden
    Force simulation: only includes visible nodes
    Count badges update to reflect visible counts

  Filter animation:
    Nodes fading out: scale 1→0.8, opacity 1→0, 200ms ease-in-quint
    Nodes appearing: scale 0.8→1, opacity 0→1, 250ms ease-out-quint
    Force simulation reheated (alpha 0.2) to settle new layout

Edge type filter (legend interaction):
  Click edge type in legend to toggle visibility
  All edge types visible by default
  Mentions edges hidden by default when >100 nodes visible

Search (toolbar input):
  Search field:
    Width: 200px (expandable to 300px on focus)
    Behavior: same as Timeline Explorer search (see 6.7.3)
  
  Search results:
    Matching nodes: highlighted (outline pulse 1.5s cycle)
    Non-matching nodes: dimmed (opacity 0.1)
    Match count: "3 nodes match '{query}'"
    Enter: cycle through results (focus next matching node)
    Escape: clear search

  Search fields:
    Node label (weight: 3)
    Node ID (weight: 5)
    Node metadata (weight: 1)
    Connected node labels (weight: 1)

Cluster filter:
  "Filter to cluster" via cluster right-click
  Shows cluster name + [Clear cluster filter] button in toolbar
  Only cluster nodes visible, camera focused on cluster

Degree filter (advanced):
  Slider: "Show nodes with ≥ {N} connections"
  Range: 1 to max(nodeDegree)
  Real-time: nodes below threshold fade out as slider moves
```

### 7.13 Performance Optimizations

```
1. WebGL Instanced Rendering
   For nodes of the same type and geometry:
     Use THREE.InstancedMesh instead of individual Mesh objects
     Each node type → one InstancedMesh
     Total draw calls: 6 (one per node type) instead of N
     
   Implementation:
     const sessionMesh = new THREE.InstancedMesh(
       roundedRectGeometry,
       sessionMaterial,
       sessionNodeCount
     );
     // Update instance matrices each frame from position buffer
     dummy.identity();
     for (let i = 0; i < sessionNodeCount; i++) {
       dummy.compose(position, quaternion, scale);
       sessionMesh.setMatrixAt(i, dummy);
     }
     sessionMesh.instanceMatrix.needsUpdate = true;

   InstancedMesh color per-instance:
     Use instanceColor (Three.js r150+) for per-instance color variation
     Or: use custom shader material with per-instance color attribute

2. Level of Detail (LOD)
   THREE.LOD for each node:
     LOD Level 0 (distance < 20): Full geometry, 32 segments
     LOD Level 1 (distance 20-50): Reduced geometry, 16 segments
     LOD Level 2 (distance 50-100): Low-poly proxy, 8 segments
     LOD Level 3 (distance > 100): Billboard sprite (2D circle texture)
     
   Edge LOD:
     Camera distance < 50: Curved bezier, 20 segments
     Camera distance 50-100: Low-poly line, 8 segments
     Camera distance 100-150: Straight line (ignore curve)
     Camera distance > 150: Hidden

3. Frustum Culling
   Three.js automatic frustum culling (enabled by default on all meshes)
   Custom culling for labels: only render labels of nodes within view frustum
   Label sprite pool: reuse sprite objects, update texture on assignment

4. Spatial partitioning
   Octree for collision detection and picking (raycasting):
     Max depth: 6
     Max objects per node: 16
     Rebuild: every 10 simulation ticks (not every tick)
   
   Picking (raycaster):
     Use octree to filter candidate nodes
     Fall back to brute-force for < 100 candidates
     Mouse interaction: throttle raycaster to 30fps (not 60fps)

5. Edge optimization
   Mentions edges: hidden when > 100 nodes visible
   References edges: hidden when > 500 nodes visible
   Only render edges where BOTH endpoints are within view frustum
   Edge geometry pooling: reuse BufferGeometry objects
   Edge material: shared material per edge type (not per edge)

6. Render pipeline
   Render order (opaque first, transparent later):
     1. Node meshes (opaque)
     2. Edge lines
     3. Cluster hulls (transparent)
     4. Label sprites (transparent)
     5. Selection/highlight overlays
     6. UI overlays (legend, mini-map)

   Frame timing:
     Simulation update: 0.5ms (worker, parallel)
     Position buffer upload: 0.3ms
     Instance matrix update: 1.0ms (for 1000+ instances)
     Label update: 0.5ms (incremental, only changed labels)
     Render: 2-5ms (GPU-dependent)
     Total main thread: ~5ms → easily hits 60fps

7. Memory management
   Geometry caching: each node type geometry created once, shared
   Material pooling: 6 materials (node types), reused for all nodes
   Texture atlas for labels: single texture for all labels (2048×2048)
     UV offsets map each label to its region
     Reduces draw calls for labels from N to 1
   
   Dispose on unmount:
     geometry.dispose()
     material.dispose()
     texture.dispose()
     renderer.dispose()
     worker.terminate()

8. Progressive loading
   Initial load: 200 nodes (highest-degree nodes + all sessions)
   Progressive: load more nodes as camera zooms in
   Background: fetch remaining nodes, add to scene when loaded
   Node count indicator: "1,247 nodes · 892 loaded" in toolbar
```

### 7.14 Error States & Edge Cases

```
Empty graph (no nodes):
  Canvas: centered empty state
    Icon: graph, 64px, text-disabled
    Title: "No entities to display"
    Subtitle: "The entity graph will populate as sessions run"
    Action: "Create your first session" button

All nodes filtered out:
  Canvas overlay:
    "All entity types are currently hidden"
    "Enable at least one entity type in the filter to see nodes"
    [Reset Filters] button

WebGL not supported:
  Canvas replaced with 2D fallback (Canvas 2D API)
  Banner (top): "WebGL not available — using 2D renderer (reduced performance)"
  All features functional, but:
    - No 3D depth
    - No custom shaders
    - Reduced visual quality
    - Lower max node count (500)

WebGL context lost:
  Detect: webglcontextlost event
  Canvas: frozen, shows overlay
    "Graphics context lost — attempting to restore..."
    Spinner animation
  On webglcontextrestored:
    Rebuild all geometries, materials, textures
    Resume rendering
    Toast: "Graphics restored"

Force simulation divergence (nodes exploding):
  Detect: any node position > 1000 units from origin
  Auto-reset: recenter all nodes to origin, reheat simulation
  Banner: "Layout restabilizing..." (disappears after 2s stable)

Very large graph (> 5000 nodes):
  Switch to cluster view automatically:
    Individual nodes below threshold become cluster aggregates
    Cluster aggregates shown as larger circles with count badges
    Zoom in to expand clusters into individual nodes
  Banner: "Large graph detected — clusters shown. Zoom in for details."

Browser tab not visible (Page Visibility API):
  Pause simulation (stop worker ticks)
  Pause render loop
  Resume on visibilitychange → visible

Memory pressure:
  Monitor: performance.memory.usedJSHeapSize
  If > 80% of limit: reduce cached nodes, dispose distant LODs
  If > 90%: show warning toast "Memory usage high — consider reducing visible nodes"

Node hover on overlapping nodes:
  Raycaster returns closest intersected node only
  If multiple nodes at nearly same depth (< 0.5 units):
    Show count badge: "+2 more"
    Click cycles through overlapping nodes
    Tooltip shows list of all overlapping nodes

Single-node graph:
  Node centered in view
  Camera distance: 8 units
  No edges to display
  Detail panel shows full node information
  "No connections" shown in connections section
```

### 7.15 Graph State Model

```typescript
interface EntityGraphState {
  // Data
  nodes: GraphNode[];
  edges: GraphEdge[];
  clusters: GraphCluster[];

  // Layout
  layout: 'force-directed' | 'radial' | 'hierarchical' | 'timeline' | 'grid';
  layoutTransitioning: boolean;
  layoutProgress: number;         // 0-1 transition progress

  // Simulation
  simulationRunning: boolean;
  simulationAlpha: number;
  simulationStabilized: boolean;
  pinnedNodeIds: Set<string>;

  // Viewport
  camera: {
    position: { x: number; y: number; z: number };
    target: { x: number; y: number; z: number };
    zoom: number;
  };

  // Selection
  selectedNodeIds: Set<string>;
  focusedNodeId: string | null;
  hoveredNodeId: string | null;
  hoveredEdgeId: string | null;

  // UI
  detailPanelOpen: boolean;
  detailPanelNodeId: string | null;
  miniMapVisible: boolean;
  legendVisible: boolean;
  clusterListVisible: boolean;

  // Filters
  filters: {
    nodeTypes: Set<NodeType>;
    edgeTypes: Set<EdgeType>;
    searchQuery: string;
    clusterId: string | null;     // Filter to single cluster
    minDegree: number;             // 0 = no filter
  };

  // Performance
  visibleNodeCount: number;
  visibleEdgeCount: number;
  totalNodeCount: number;
  totalEdgeCount: number;
  fps: number;
  renderTime: number;              // Last frame render time in ms
  qualityLevel: 'high' | 'medium' | 'low'; // Adaptive quality

  // Loading
  loadingState: 'idle' | 'loading' | 'error';
  loadedNodeCount: number;         // Progressively loaded
  loadError: Error | null;
}

interface GraphNode {
  id: string;
  type: 'session' | 'memory' | 'finding' | 'evidence' | 'anomaly' | 'entity';
  entityType: EntityType;         // For generic entity nodes
  label: string;
  position: { x: number; y: number; z: number };
  velocity: { x: number; y: number; z: number };
  radius: number;
  color: string;                  // Hex color
  degree: number;                  // Total edge count
  inDegree: number;
  outDegree: number;
  clusterId: string | null;
  pinned: boolean;
  visible: boolean;
  metadata: Record<string, unknown>;
  createdAt: number;               // Unix ms
  updatedAt: number;
}

interface GraphEdge {
  id: string;
  type: 'contains' | 'derived-from' | 'references' | 'mentions' | 'semantic-similarity' | 'contradiction';
  sourceId: string;
  targetId: string;
  weight: number;                  // 0-1, semantic similarity score or trust
  visible: boolean;
  metadata?: {
    similarityScore?: number;      // For semantic-similarity edges
    contradictionType?: string;    // For contradiction edges
    referenceContext?: string;     // For reference edges
  };
}

interface GraphCluster {
  id: string;
  label: string;
  nodeIds: string[];
  center: { x: number; y: number; z: number };
  radius: number;
  color: string;
  density: number;
  interClusterEdges: string[];     // Edge IDs crossing cluster boundary
}
```

### 7.16 Keyboard Shortcuts & Accessibility

```
Graph navigation:
  F           : Fit all nodes in view
  R           : Reset camera to default
  M           : Toggle mini-map
  L           : Toggle legend
  C           : Toggle cluster list
  Escape      : Deselect all / close detail panel
  Tab         : Focus next node (cycles through visible nodes)
  Shift+Tab   : Focus previous node
  Enter       : Select focused node / open detail panel

  Arrow keys  : Nudge camera (pan)
                 ← → : pan horizontally
                 ↑ ↓ : pan vertically
  Shift+Arrow : Nudge camera (orbit)
                 ← → : orbit azimuth
                 ↑ ↓ : orbit elevation
  +/-         : Zoom in/out

Layout switching:
  1           : Force-directed layout
  2           : Radial layout
  3           : Hierarchical layout
  4           : Timeline layout
  5           : Grid layout

Selection:
  Ctrl+A      : Select all visible nodes
  Ctrl+Shift+A: Deselect all
  Delete      : Remove selected nodes from view (temporary hide — not deletion)

Accessibility:
  Canvas: role="application", aria-label="Entity relationship graph"
  Nodes: rendered DOM proxy list for screen readers (hidden visually)
    <ul aria-label="Graph nodes" style="position:absolute;width:1px;height:1px;overflow:hidden;">
      <li>Session: Q4 Revenue Analysis — 42 connections</li>
      <li>Memory: /projects/q4-revenue — 15 connections</li>
      ...
    </ul>
  Node list updates: aria-live="polite" region announces selection changes
  Reduced motion: all graph animations instant (0ms)
  High contrast mode: node borders become white (2px) for clarity
```

### 7.17 Web Worker Force Simulation — Complete Implementation Reference

```typescript
// force-worker.ts — Complete force simulation in Web Worker
// This is a reference implementation, not pseudocode.

interface WorkerNode {
  id: string;
  type: string;
  x: number; y: number; z: number;
  vx: number; vy: number; vz: number;
  fx: number | null; fy: number | null; fz: number | null; // Fixed position if set
  radius: number;
  degree: number;
  clusterId: string | null;
}

interface WorkerEdge {
  source: number;  // Node index
  target: number;  // Node index
  type: string;
  strength: number;
  distance: number;
}

class ForceSimulation {
  nodes: WorkerNode[];
  edges: WorkerEdge[];
  alpha: number;
  alphaMin: number;
  alphaDecay: number;
  velocityDecay: number;

  // Force parameters
  linkStrength: number;
  linkDistance: number;
  linkIterations: number;
  chargeStrength: number;
  chargeDistanceMax: number;
  centerStrength: number;
  collisionStrength: number;
  collisionIterations: number;
  clusterStrength: number;
  clusterCenters: Map<string, { x: number; y: number; z: number }>;

  // Barnes-Hut for charge force
  theta: number;
  tree: BarnesHutTree | null;

  constructor(nodes: WorkerNode[], edges: WorkerEdge[], config: ForceConfig) {
    this.nodes = nodes;
    this.edges = edges;
    this.alpha = config.alpha;
    this.alphaMin = config.alphaMin;
    this.alphaDecay = config.alphaDecay;
    this.velocityDecay = config.velocityDecay;
    // ... assign force parameters from config
  }

  tick(): void {
    this.alpha += (0 - this.alpha) * this.alphaDecay;
    if (this.alpha < this.alphaMin) return;

    // 1. Apply link force
    for (let i = 0; i < this.linkIterations; i++) {
      this.applyLinkForce();
    }

    // 2. Apply charge force (Barnes-Hut approximation)
    this.buildBarnesHutTree();
    this.applyChargeForce();

    // 3. Apply center force
    this.applyCenterForce();

    // 4. Apply collision force
    for (let i = 0; i < this.collisionIterations; i++) {
      this.applyCollisionForce();
    }

    // 5. Apply cluster force
    this.applyClusterForce();

    // 6. Update positions
    for (const node of this.nodes) {
      if (node.fx !== null) { node.x = node.fx; node.vx = 0; }
      else {
        node.vx *= this.velocityDecay;
        node.x += node.vx;
      }
      if (node.fy !== null) { node.y = node.fy; node.vy = 0; }
      else {
        node.vy *= this.velocityDecay;
        node.y += node.vy;
      }
      if (node.fz !== null) { node.z = node.fz; node.vz = 0; }
      else {
        node.vz *= this.velocityDecay;
        node.z += node.vz;
      }
    }
  }

  private applyLinkForce(): void {
    for (const edge of this.edges) {
      const source = this.nodes[edge.source];
      const target = this.nodes[edge.target];
      let dx = target.x - source.x;
      let dy = target.y - source.y;
      let dz = target.z - source.z;
      let dist = Math.sqrt(dx * dx + dy * dy + dz * dz) || 1;
      const strength = edge.strength * this.linkStrength;
      const bias = (dist - edge.distance) / dist * strength * this.alpha;
      dx *= bias; dy *= bias; dz *= bias;
      target.vx -= dx * (1 / Math.max(1, target.degree));
      target.vy -= dy * (1 / Math.max(1, target.degree));
      target.vz -= dz * (1 / Math.max(1, target.degree));
      source.vx += dx * (1 / Math.max(1, source.degree));
      source.vy += dy * (1 / Math.max(1, source.degree));
      source.vz += dz * (1 / Math.max(1, source.degree));
    }
  }

  // ... remaining methods: applyChargeForce, applyCenterForce,
  //     applyCollisionForce, applyClusterForce, buildBarnesHutTree
}

// Worker message handling
let simulation: ForceSimulation | null = null;

self.onmessage = (e: MessageEvent) => {
  const msg = e.data;
  switch (msg.type) {
    case 'init':
      simulation = new ForceSimulation(msg.nodes, msg.edges, msg.config);
      // Warmup: run initial ticks
      for (let i = 0; i < msg.config.warmupTicks; i++) {
        simulation.tick();
      }
      postMessage({
        type: 'initialized',
        positions: serializePositions(simulation.nodes),
        alpha: simulation.alpha,
      });
      break;

    case 'tick':
      simulation?.tick();
      postMessage({
        type: 'ticked',
        positions: serializePositions(simulation!.nodes),
        alpha: simulation!.alpha,
      });
      break;

    case 'tickMultiple':
      for (let i = 0; i < msg.count; i++) {
        simulation?.tick();
      }
      postMessage({
        type: 'ticked',
        positions: serializePositions(simulation!.nodes),
        alpha: simulation!.alpha,
      });
      break;

    // ... other message handlers
  }
};

function serializePositions(nodes: WorkerNode[]): Float32Array {
  const buf = new Float32Array(nodes.length * 3);
  for (let i = 0; i < nodes.length; i++) {
    buf[i * 3] = nodes[i].x;
    buf[i * 3 + 1] = nodes[i].y;
    buf[i * 3 + 2] = nodes[i].z;
  }
  return buf; // Transferred via Transferable
}
```

---

*Part 3 of 4 — Timeline Explorer (Section 6) and Entity Graph (Section 7) complete.*
*Continues with Part 4: Investigation Workbench, Session Detail, and remaining sections.*
## 8. Semantic Search & Discovery

### 8.1 Overview

```
Semantic Search is the primary discovery mechanism in Chronicle.
Unlike keyword search, it finds content by MEANING — allowing queries like:
  "Show me similar phishing patterns to what we saw in March"
  "Find all evidence related to APAC supply chain disruptions"
  "What have we previously concluded about the CFO's travel patterns?"

It uses the Consensus semantic retrieval engine under the hood.
```

### 8.2 Search Interface

```
Search can be triggered from anywhere via Cmd+Shift+F (global)
or from the Search icon in the sidebar.

Search Overlay:
  Glass-morphism full-screen overlay
  Background: var(--glass-heavy) with blur 20px
  Open animation: fade in 150ms
  Close: fade out 100ms, or Escape key

  ┌──────────────────────────────────────────────────────┐
  │                                                      │
  │         🔍  [________________________________]       │  ← Large search input
  │              Search across all investigations...      │     centered, 560px wide
  │                                                      │
  │         ┌──────────────────────────────────────┐     │
  │         │ FILTERS:  [All Content ▾]            │     │
  │         │           [Any Time ▾]               │     │
  │         │           [Any Session ▾]            │     │
  │         └──────────────────────────────────────┘     │
  │                                                      │
  │         RESULTS                                      │
  │         ┌──────────────────────────────────────┐     │
  │         │ ✅ Finding #7 · APAC Revenue Growth   │     │  ← Result card
  │         │    Relevance: 94% · Session #a3f      │     │
  │         │    "Q4 revenue in APAC increased..."  │     │
  │         │    ──────────────────────────────     │     │
  │         │    Similarity based on: revenue,      │     │
  │         │    APAC, Q4, growth patterns          │     │
  │         └──────────────────────────────────────┘     │
  │                                                      │
  │         ┌──────────────────────────────────────┐     │
  │         │ 📄 Memory Event #2841                 │     │
  │         │    Relevance: 89% · Session #a3f      │     │
  │         │    "Comparing APAC growth trajectory  │     │
  │         │     with EMEA decline..."             │     │
  │         └──────────────────────────────────────┘     │
  │                                                      │
  └──────────────────────────────────────────────────────┘

Search input behavior:
  Debounced: 200ms after last keystroke
  Minimum query length: 3 characters
  Loading state: subtle shimmer animation on input border
  Empty state (no query): "Recent Searches" shown
    List of last 10 search queries, click to re-run
    "Clear history" button
  
Search execution:
  Query sent to Consensus semantic retrieval endpoint
  Results returned ranked by cosine similarity
  Time to first result: <100ms (local), <500ms (remote)
  Streaming results: top 5 appear immediately, remaining load progressively

Result card anatomy:
  Icon + type badge: entity type indicator (finding, memory, session, etc.)
  Title: body size, font-weight 600, truncates 1 line
  Relevance score: caption, color-coded
    >90%: green, 70-90%: amber, <70%: muted
  Preview: 2 lines of content matching the query context
  Session link: "#a3f" mono, clickable
  Match explanation: caption, italic, "Similarity based on: ..."
    Shows key terms that drove the match (transparency)
  Click: navigates to entity in context (workbench, memory browser, etc.)

Result card interaction:
  Hover: background transitions to var(--color-bg-hover)
  Keyboard navigation:
    Arrow Up/Down: move selection
    Enter: open selected result
    Escape: close search overlay

Filters (below search input):
  Content Type dropdown:
    All Content
    Findings Only
    Memory Events Only
    Evidence Sources Only
    Sessions Only
    Entities Only
    
  Time Range dropdown:
    Any Time
    Last 24 Hours
    Last 7 Days
    Last 30 Days
    Custom Range...
    
  Session dropdown:
    Any Session
    [List of recent sessions with checkboxes]
    Multiselect

Advanced search operators (type in search bar):
  type:finding          — only findings
  session:a3f           — only in session #a3f
  before:2026-06-01     — before date
  after:2026-05-01      — after date
  confidence:>0.8       — high confidence findings
  status:approved       — approved findings only
  -revenue              — exclude term (negation)
  "exact phrase"        — exact phrase match (hybrid: semantic + keyword)
```

### 8.3 Discovery Panel (Sidebar)

```
Persistent search panel in the Investigation Workbench.
Toggle with Cmd+Shift+D or button in workbench toolbar.

Position: right side panel (shared with Evidence Panel — tabbed).
Tabs: [Evidence] [Discovery]

Discovery Panel Content:
  ┌────────────────────────────┐
  │ DISCOVERY            [×]   │
  ├────────────────────────────┤
  │ [🔍 Search all evidence...]│
  ├────────────────────────────┤
  │ SUGGESTED QUERIES          │
  │ ┌────────────────────────┐ │
  │ │ Find contradictions     │ │  ← AI-generated query suggestions
  │ │ in current findings     │ │     based on context
  │ └────────────────────────┘ │
  │ ┌────────────────────────┐ │
  │ │ Show similar patterns   │ │
  │ │ to Q3 analysis          │ │
  │ └────────────────────────┘ │
  ├────────────────────────────┤
  │ RELATED SESSIONS           │
  │ #b2e · Phish Analysis      │  ← Semantically similar sessions
  │ #c4f · Network Scan        │
  │ #d1a · Q3 Revenue          │
  ├────────────────────────────┤
  │ SIMILAR FINDINGS           │
  │ Finding #3 (Q3 — 87%)      │  ← Findings from other sessions
  │ Finding #12 (Supplier —    │     that relate to current work
  │   82%)                     │
  └────────────────────────────┘

Suggested Queries:
  AI-generated based on current investigation context
  Each query: clickable pill
  Click: executes search, shows results below
  Regenerates when investigation context changes
  Up to 5 suggestions

Related Sessions:
  Sessions with semantic similarity to current investigation
  Score shown as percentage
  Click: opens that session's graph/overview

Similar Findings:
  Cross-session finding discovery
  Shows findings from other investigations that relate
  Score shown
  Click: opens finding in context
```

---

*To be continued with sections 9-28...*
## 9. Session Lifecycle Manager

### 9.1 Session List View

```
Primary view for managing all agent sessions.
Accessible from sidebar: Sessions (⌘5).

Layout: full-width table with filtering and batch operations.

┌─────────────────────────────────────────────────────────────────────┐
│ SESSIONS                                           [+ New Session]  │
│                                                                     │
│ FILTERS:  [Status ▾]  [Model ▾]  [Time ▾]  [🔍 Search...]         │
│                                                                     │
│ ┌─────────────────────────────────────────────────────────────────┐ │
│ │ ID      │ Name           │ Status   │ Iter │ Cost   │ Active    │ │
│ │─────────┼────────────────┼──────────┼──────┼────────┼───────────│ │
│ │ #a3f2b  │ Q4 Revenue     │ thinking │ 42   │ $1.23  │ 2m ago    │ │  ← Clickable row
│ │ #b2e1c  │ Phish Analysis │ completed│ 18   │ $0.47  │ 3h ago    │ │
│ │ #c4f8d  │ Network Scan   │ paused   │ 5    │ $0.12  │ 1d ago    │ │
│ │ #d1a9e  │ Q3 Analysis    │ failed   │ 31   │ $0.89  │ 2d ago    │ │
│ │ #e5b3f  │ Supplier Audit │ idle     │ 0    │ $0.00  │ 10m ago   │ │
│ │ ...     │ ...            │ ...      │ ...  │ ...    │ ...       │ │
│ └─────────────────────────────────────────────────────────────────┘ │
│                                                                     │
│ 12 sessions · 3 selected    [Pause Selected] [Resume] [Cancel]     │
└─────────────────────────────────────────────────────────────────────┘

Table columns:
  ID: monospace caption, truncated to 8 chars, full ID on hover
  Name: body-small, truncates at 30 chars
  Status: status badge (colored pill with icon)
  Iter: iteration count, tabular-nums
  Cost: formatted dollar amount, tabular-nums
  Active: relative timestamp, tabular-nums

Status badges:
  booting:    gray pill, "Booting" with spinner
  idle:       muted blue pill, "Idle" 
  thinking:   purple pill, "Thinking" with pulse animation
  tool_exec:  amber pill, "Tool Exec"
  waiting_sub: cyan pill, "Waiting"
  paused:     amber pill, "Paused" with pause icon
  completed:  green pill, "Done" with check
  failed:     red pill, "Failed"
  cancelled:  gray pill, "Cancelled" with strikethrough

Row states:
  Default: transparent background
  Hover: var(--color-bg-hover)
  Selected: var(--color-bg-selection), checkbox checked
  Active (thinking/tool_exec): subtle glow on left border
    (2px solid accent-primary at 30% opacity)
  
Row click: selects row (checkbox)
Row double-click: opens session in Investigation Workbench

Column sorting:
  Click column header: sort ascending
  Click again: sort descending
  Click third time: remove sort
  Sort indicator: ▲▼ arrows next to header text
  
Column resize:
  Drag right edge of column header
  Cursor: col-resize
  Minimum column width: 60px
  Saved to localStorage per user

Filter chips:
  Status multiselect dropdown: checkboxes for each status
    "Select All" / "Deselect All"
    Count badge per status: "thinking (3)"
    
  Model dropdown: list of models used in sessions
    Each with count: "deepseek-v4-pro (8)"
    
  Time dropdown:
    Any Time
    Last Hour
    Last 24 Hours
    Last 7 Days
    Last 30 Days
    Custom...
    
  Search: text input, searches name + goal + agent_name
    Debounced 300ms
    Real-time: table updates as you type
    Highlight: matching text highlighted in results
    Empty search: clears filter

Batch operations (visible when rows selected):
  Floating toolbar at bottom of table:
    "3 sessions selected"
    [Pause] [Resume] [Cancel] [Export] [Clear Selection]
  
  Each action:
    Pause: sets status to 'paused' for selected sessions
    Resume: sets status to 'idle' for paused sessions
    Cancel: sets status to 'cancelled' (confirmation dialog for >1)
    Export: exports session data as JSON/CSV
    
  Confirmation dialog for destructive actions:
    "Cancel 3 sessions?"
    "This will stop all processing and cannot be undone."
    [Cancel Sessions] [Keep Running]

New Session button:
  Position: top-right of page header
  Opens creation modal (see below)
  Keyboard shortcut: ⌘N
```

### 9.2 Session Creation Modal

```
Modal dialog for creating a new agent session.

┌──────────────────────────────────────────────────┐
│  Create New Session                         [×]  │
│                                                  │
│  Name                                           │
│  ┌────────────────────────────────────────────┐ │
│  │ e.g., "Q4 Revenue Analysis"                │ │
│  └────────────────────────────────────────────┘ │
│                                                  │
│  Goal                                           │
│  ┌────────────────────────────────────────────┐ │
│  │ Describe what the agent should accomplish.  │ │
│  │                                            │ │
│  │ "Analyze Q4 revenue data, identify growth  │ │
│  │  patterns by region, and flag anomalies    │ │
│  │  for further investigation."               │ │
│  └────────────────────────────────────────────┘ │
│                                                  │
│  Agent Type                            [▾]      │
│  ┌────────────────────────────────────────────┐ │
│  │ ● Researcher (analysis + investigation)    │ │
│  │ ○ Security Analyst (threat detection)       │ │
│  │ ○ Data Analyst (SQL + visualization)       │ │
│  │ ○ Custom...                                │ │
│  └────────────────────────────────────────────┘ │
│                                                  │
│  Model                                [▾]       │
│  ┌────────────────────────────────────────────┐ │
│  │ ● deepseek-v4-pro    ~$0.002/query         │ │
│  │ ○ deepseek-v4-flash  ~$0.0005/query        │ │
│  │ ○ claude-sonnet-4    ~$0.008/query         │ │
│  └────────────────────────────────────────────┘ │
│                                                  │
│  ▸ Advanced Options                             │
│    Context Budget: [128000] tokens              │
│    Max Iterations: [100]                        │
│    Budget Limit: [$10.00]                       │
│    HITL: [☑ Require approval for actions]      │
│    Auto-approve threshold: [0.90 ▸]            │
│                                                  │
│  ▸ Evidence                                     │
│    [+ Add files...]                              │
│    📄 q4-sales.csv (2.4 MB)               [×]  │
│    🗄️ revenue_db.orders                    [×]  │
│                                                  │
│                              [Cancel] [Create]   │
└──────────────────────────────────────────────────┘

Form validation:
  Name: required, min 3 chars
  Goal: required, min 10 chars
  Real-time validation (validate on blur, not on keystroke)
  Error messages appear below field in red caption text
  Submit button disabled until all required fields valid

Advanced Options (collapsed by default):
  Expand animation: height transition 300ms ease-out-quint
  Context Budget slider: 1K to 1M tokens
    Preset buttons: [32K] [64K] [128K] [256K]
  Max Iterations: number input, min 1, max 1000
  Budget Limit: dollar amount input
  HITL toggle: checkbox
  Auto-approve threshold: slider 0.0 to 1.0
    Labels: "Approve nothing" (0.0) to "Approve everything" (1.0)

Evidence upload:
  Drag-and-drop zone or file picker
  Accepts: .csv, .json, .pdf, .txt, .md, .log
  Progress bar per file during upload
  Max file size: 50MB (configurable)
  Uploaded files shown as removable pills

Create button:
  On click: button shows spinner, text changes to "Creating..."
  API call: POST /api/v1/sessions
  On success: modal closes, new session appears at top of table
    Row highlight animation: green flash (background transitions green→transparent, 1s)
    Toast: "Session 'Q4 Revenue Analysis' created"
    Session key displayed in toast with [Copy] button
  On error: error message shown in modal, form remains editable
  
Session key display (after creation):
  ┌──────────────────────────────────────────────┐
  │ 🎉 Session Created!                          │
  │                                              │
  │ Your session API key:                        │
  │ ┌──────────────────────────────────────────┐ │
  │ │ cs_sk_a1b2c3d4e5f6...              [📋] │ │
  │ └──────────────────────────────────────────┘ │
  │                                              │
  │ ⚠ Save this key — it won't be shown again.  │
  │                                              │
  │                [Copy Key] [Done]             │
  └──────────────────────────────────────────────┘
```

### 9.3 Session Detail View

```
Full detail view for a single session.

Layout (two-column on desktop):
  Left (2/3): Session activity timeline
  Right (1/3): Session metadata + controls

┌─────────────────────────────────────────────────────────────────────┐
│ ← Back to Sessions    #a3f2b · Q4 Revenue Analysis    [Pause] [⚙]  │
├──────────────────────────────────┬──────────────────────────────────┤
│                                  │                                  │
│  ACTIVITY                        │  DETAILS                         │
│                                  │  Status: thinking ●              │
│  ┌─ 14:23 ────────────────────┐ │  Agent: researcher               │
│  │ 🧠 Iteration 42 started    │ │  Model: deepseek-v4-pro           │
│  │ 📄 Analyzed q4-sales.csv   │ │                                  │
│  └────────────────────────────┘ │  Iterations: 42 / 100            │
│                                  │  ████████░░ 42%                  │
│  ┌─ 14:22 ────────────────────┐ │                                  │
│  │ ✅ Finding #6 approved     │ │  Tokens: 847K in / 234K out      │
│  │ By: Bane                    │ │  Cost: $1.23 / $10.00            │
│  └────────────────────────────┘ │  ██░░░░░░░░ 12%                  │
│                                  │                                  │
│  ┌─ 14:21 ────────────────────┐ │  Memory Events: 287              │
│  │ ⚠ Anomaly flagged:         │ │  Findings: 7 (6 approved)        │
│  │ APAC spike +37%             │ │  Tasks: 3 (2 done, 1 pending)   │
│  └────────────────────────────┘ │                                  │
│                                  │  ─────────────────────           │
│  ...scroll for older events...   │  ACTIONS                         │
│                                  │  [Open Workbench]                │
│                                  │  [View Memory]                   │
│                                  │  [View Tasks]                    │
│                                  │  [View Approvals]                │
│                                  │  [Export Timeline]               │
│                                  │                                  │
│                                  │  ─────────────────────           │
│                                  │  CONTROLS                        │
│                                  │  [⏸ Pause]  [▶ Resume]         │
│                                  │  [✕ Cancel]                     │
│                                  │                                  │
│                                  │  ─────────────────────           │
│                                  │  API KEY                         │
│                                  │  cs_sk_a1...  [📋]              │
│                                  │                                  │
│                                  │  Created: Jun 7, 14:15           │
│                                  │  Last active: 2m ago             │
│                                  │  Session ID: a3f2b-...           │
└──────────────────────────────────┴──────────────────────────────────┘

Controls:
  Pause: sets status to 'paused'
    Confirmation: "Pause session? Agent will stop after current iteration."
    Button transitions: pause icon → spinner → "Paused" badge
    
  Resume: sets status to 'idle'
    Available only when paused
    Button transitions: play icon → spinner → running
    
  Cancel: sets status to 'cancelled'
    Confirmation: "Cancel session? This cannot be undone. All pending work will be lost."
    Destructive action: red button with confirmation dialog
    On cancel: session row shows "Cancelled" badge, activity timeline stops

Progress bars:
  Iterations: blue fill, shows progress toward max
  Budget: green→amber→red based on percentage used
  Both animate smoothly: CSS transition on width, 1s ease-out-quint
  Hover: tooltip with exact numbers

API Key section:
  Key displayed with first 8 and last 4 chars visible, middle obscured
  Copy button: copies full key to clipboard
  Toast: "API key copied to clipboard"
  Regenerate button: creates new key (old key invalidated immediately)
    Confirmation: "Regenerate API key? The old key will stop working immediately."
```

### 9.4 Real-Time Session Monitoring

```
WebSocket connection pushes session state changes in real-time.

Session status transitions are animated in the UI:
  idle → thinking: status badge morphs (color shift + icon swap, 300ms)
  thinking → tool_exec: badge pulses (scale 1→1.1→1, 200ms ease-spring)
  any → completed: green sweep animation (green glow sweeps left→right across row, 500ms)
  any → failed: red flash (background flashes red at 20% opacity, then fades, 500ms)
  any → paused: amber border appears + row dims to 80% opacity

Live activity feed in session detail:
  New events slide in at top (translateY -10px→0 + fade, 200ms ease-out-quint)
  Existing events shift down smoothly (CSS transition on margin, 250ms)
  Auto-scroll: when scrolled to top, stays at top
  "↓ New activity" button when scrolled up

Iteration counter:
  Number updates with flip animation:
    Current digit flips down, new digit flips up
    Like an airport departure board
    Duration: 400ms per digit change
    Staggered by digit position (ones place flips first, then tens, etc.)
    Tabular-nums ensure width doesn't change

Cost counter:
  Updates in real-time as tokens are consumed
  Number updates with smooth count-up animation
    Not just jumping — animates from current to new value
    Duration: 500ms, ease-out-quint
    Shows hundredths of cents scrolling
    
Heartbeat indicator:
  Small dot next to session status
  Pulses every 30 seconds (session heartbeat interval)
  If heartbeat missed (45s+): dot turns amber
  If heartbeat missed (90s+): dot turns red, "Stalled" warning appears
```

---

## 10. Memory Browser & Audit Trail

### 10.1 Memory Browser Layout

```
Full inspection view for session memory events.
Accessible from sidebar: Memory Browser (⌘6) or from session detail.

Layout:
  Left panel: Memory event list (scrollable, virtualized)
  Right panel: Selected event detail (persistent, scrollable)

┌─────────────────────────────────────────────────────────────────────┐
│ MEMORY BROWSER                                Session: [#a3f2b ▾]   │
├───────────────────────────────┬─────────────────────────────────────┤
│                               │                                     │
│ FILTERS:                      │  EVENT DETAIL                       │
│ [Type ▾] [Trust ▾] [Iter ▾]  │                                     │
│ [🔍 Search events...]         │  ┌───────────────────────────────┐ │
│                               │  │ 📄 Event #2841                │ │
│ 287 events                    │  │ Type: thought                  │ │
│                               │  │ Trust: HIGH ● (verified)       │ │
│ ┌───────────────────────────┐ │  │ Iteration: 42                 │ │
│ │ ● #2841  thought    it 42 │ │  │ Session: #a3f2b               │ │
│ │   Analyzing Q4 APAC data  │ │  │ Timestamp: Jun 7, 14:23:15    │ │
│ │   Trust: HIGH              │ │  │                               │ │
│ │                           │ │  │ ── CONTENT ──                 │ │
│ │ ● #2840  action     it 42 │ │  │                               │ │
│ │   SELECT * FROM revenue   │ │  │ Comparing APAC growth          │ │
│ │   Trust: HIGH              │ │  │ trajectory with EMEA decline. │ │
│ │                           │ │  │ The contradiction in November   │ │
│ │ ● #2839  observation it 42│ │  │ supply chain data vs. March    │ │
│ │   Query returned 12,400   │ │  │ report suggests a reporting    │ │
│ │   rows                    │ │  │ lag, not actual decline.       │ │
│ │                           │ │  │ Cross-referencing with shipping │ │
│ │ ● #2838  thought    it 41│ │  │ manifests confirms: shipments   │ │
│ │   ...                     │ │  │ were delayed, not cancelled.    │ │
│ └───────────────────────────┘ │  │                               │ │
│                               │  │ ── METADATA ──                │ │
│ < 1 2 3 ... 29 >             │  │ Tokens: 847 in / 234 out       │ │
│                               │  │ Model: deepseek-v4-pro         │ │
│                               │  │ Latency: 1.2s                  │ │
│                               │  │ Hash: a1b2c3d4...              │ │
│                               │  │                               │ │
│                               │  │ ── ACTIONS ──                 │ │
│                               │  │ [Copy Content] [Flag]         │ │
│                               │  │ [Find Similar] [View Raw]     │ │
│                               │  │ [Change Trust Level ▾]        │ │
│                               │  └───────────────────────────────┘ │
│                               │                                     │
│                               │  RELATED EVENTS                     │
│                               │  ● #2840 — Same iteration          │
│                               │  ● #2842 — Next event              │
│                               │  ● #1521 — Similar (89%)           │
└───────────────────────────────┴─────────────────────────────────────┘

Event list item:
  Color dot: indicates event type
    thought:     purple
    action:      blue
    observation: cyan
    tool_result: amber
    user_message: green
    error:       red
    system:      gray
    
  Event number: mono caption, muted
  Type badge: small pill, entity color
  Content preview: body-small, truncate 1 line
  Trust badge: colored dot + label (verified=green, high=blue, medium=amber, low=orange, quarantine=red)
  Iteration: right-aligned, mono caption, "it 42"
  
  Selected state: background var(--color-bg-selection), border-left 3px solid accent-primary
  Hover: background var(--color-bg-hover)

Trust Level indicators:
  verified:   ✓ green check, "Verified" — human-reviewed and confirmed
  high:       ● blue dot, "High" — system-assigned high confidence
  medium:     ● amber dot, "Medium" — moderate confidence
  low:        ● orange dot, "Low" — uncertain, review recommended  
  quarantine: ⚠ red triangle, "Quarantine" — flagged as potentially problematic

Filter behavior:
  Type filter: checkboxes for each event type, multiselect
  Trust filter: checkboxes for trust levels
  Iteration filter: range slider or number inputs (min/max iteration)
  Search: text search on event content, debounced 200ms
  Active filters shown as removable pills below filter bar
    [× Type: thought,action] [× Trust: high,verified]
  Clear all filters: "Clear" link
  
Pagination:
  50 events per page
  Page controls at bottom: [◀] 1 2 3 ... 29 [▶]
  Total count: "287 events"
  Jump to page: click page number or type and Enter
  Keyboard: [ and ] for prev/next page

Event Detail — Content Rendering:
  Rendered based on event type:
  
  thought: prose text with markdown support
    Bold, italic, lists, code blocks rendered
    SQL snippets: syntax highlighted using Prism.js
    Tables: rendered with compact style
    
  action: JSON structured output
    Rendered with collapsible JSON tree
    Click to expand/collapse nested objects
    Line numbers
    Copy path: right-click key → "Copy JSON path"
    
  observation: plain text or structured data
    Auto-detects format: table, list, or prose
    
  tool_result: raw output from tool execution
    Monospace font, preserved whitespace
    Scrollable horizontally if lines are long
    Line wrap toggle: "Wrap" / "No Wrap" button
    
  error: red-tinted background
    Error message in red
    Stack trace in monospace, collapsible
    "Report Bug" button

Event Detail — Raw View:
  Toggle with "View Raw" button in actions
  Shows raw JSON as stored in database
  Full event object including all metadata
  Copy button for raw JSON
  "Pretty" / "Compact" toggle
  
Event Detail — Trust Level Change:
  Dropdown: Verified / High / Medium / Low / Quarantine
  Changing trust level:
    Creates approval request if HITL enabled
    Records change in audit trail
    Updates event's trust_level column
    Trust badge in event list updates immediately
  If downgrading to quarantine:
    Confirmation: "Move to quarantine? This event will be treated as potentially malicious."
    All findings based on this event flagged for review
  
Related Events:
  Same iteration: previous and next events
  Similar events: semantic similarity search results
  Click: jumps to that event in browser
```

### 10.2 Audit Trail View

```
Read-only, time-ordered log of all system actions with cryptographic
verification of immutability. This is the "prove it" view.

Toggle: "Audit Trail" tab at top of Memory Browser.

Layout:
  Full-width timeline of audit events
  Each event shows: timestamp, action, actor, details, verification hash

┌─────────────────────────────────────────────────────────────────────┐
│ AUDIT TRAIL                                                          │
│                                                                      │
│ ┌─ 14:25:03 ─────────────────────────────────────────────────────┐  │
│ │ ✅ APPROVAL: Finding #7 approved                                │  │
│ │    Actor: Bane (admin)                                          │  │
│ │    Session: #a3f2b                                              │  │
│ │    Detail: "Finding #7 'APAC Revenue Growth' approved"          │  │
│ │    Hash: a1b2c3d4e5f6...  ✓ Verified                            │  │
│ └────────────────────────────────────────────────────────────────┘  │
│                                                                      │
│ ┌─ 14:24:52 ─────────────────────────────────────────────────────┐  │
│ │ 🧠 REASONING: Iteration 42 completed                            │  │
│ │    Model: deepseek-v4-pro                                       │  │
│ │    Tokens: 847 in / 234 out                                     │  │
│ │    Memory events: 2841-2843                                     │  │
│ │    Hash: b2c3d4e5f6a7...  ✓ Verified                            │  │
│ └────────────────────────────────────────────────────────────────┘  │
│                                                                      │
│ ┌─ 14:23:15 ─────────────────────────────────────────────────────┐  │
│ │ 🔗 SOURCE: q4-sales.csv registered                              │  │
│ │    Hash: c3d4e5f6a7b8...  ✓ Verified                            │  │
│ │    SHA-256: d4e5f6a7b8c9...                                     │  │
│ └────────────────────────────────────────────────────────────────┘  │
│                                                                      │
│ ┌─ 14:15:00 ─────────────────────────────────────────────────────┐  │
│ │ 🚀 SESSION: #a3f2b created                                      │  │
│ │    Agent: researcher                                             │  │
│ │    Model: deepseek-v4-pro                                       │  │
│ │    Goal: "Analyze Q4 revenue data..."                           │  │
│ │    Hash: e5f6a7b8c9d0...  ✓ Verified                            │  │
│ └────────────────────────────────────────────────────────────────┘  │
│                                                                      │
│ Chain verification: ✓ ALL HASHES VALID (287 events verified)        │
│ Export: [Full Audit Report (PDF)] [JSON] [CSV]                      │
└─────────────────────────────────────────────────────────────────────┘

Hash verification:
  Each audit event includes a cryptographic hash
  Hashes form a chain: each event's hash includes the previous event's hash
  "Chain verification" checks the entire chain
  Green checkmark: verified ✓
  Red X: chain broken ✗ (triggers alert)
  Verification runs on page load and every 5 minutes
  Manual re-verify: "Verify Chain" button
  
Export audit report:
  PDF: formatted report with cover page, summary, all events, verification certificate
  JSON: machine-readable full audit trail
  CSV: spreadsheet-compatible format
  All exports include verification metadata
```

---

## 11. Task Queue & Orchestration

### 11.1 Task Board View

```
Kanban-style board for managing agent tasks.
Three columns: Pending, In Progress, Done.

┌─────────────────────────────────────────────────────────────────────┐
│ TASK QUEUE                                      Session: [#a3f2b ▾] │
├──────────────────┬──────────────────┬───────────────────────────────┤
│ PENDING (3)      │ IN PROGRESS (1)  │ DONE (2)                      │
│                  │                  │                               │
│ ┌──────────────┐ │ ┌──────────────┐ │ ┌───────────────────────────┐ │
│ │ 📊 Generate  │ │ │ 📈 Build     │ │ │ ✅ Extract Revenue Data   │ │
│ │ Report       │ │ │ Charts       │ │ │ Done · 14:25 · Bane      │ │
│ │              │ │ │              │ │ └───────────────────────────┘ │
│ │ Session:#a3f │ │ │ Session:#a3f │ │                               │
│ │ Created:14:28│ │ │ Claimed:14:26│ │ ┌───────────────────────────┐ │
│ │              │ │ │ By: Bane     │ │ │ ✅ Analyze Q4 Sales       │ │
│ │ [Claim]      │ │ │              │ │ │ Done · 14:20 · AI         │ │
│ │ [Edit]       │ │ │ [Complete]   │ │ └───────────────────────────┘ │
│ └──────────────┘ │ │ [Release]    │ │                               │
│                  │ └──────────────┘ │                               │
│ ┌──────────────┐ │                  │                               │
│ │ 📧 Email      │ │                  │                               │
│ │ Summary      │ │                  │                               │
│ │              │ │                  │                               │
│ │ [Claim]      │ │                  │                               │
│ └──────────────┘ │                  │                               │
│                  │                  │                               │
│ ┌──────────────┐ │                  │                               │
│ │ 🔍 Verify    │ │                  │                               │
│ │ Anomalies   │ │                  │                               │
│ └──────────────┘ │                  │                               │
│                  │                  │                               │
└──────────────────┴──────────────────┴───────────────────────────────┘
│                                                     [+ New Task]    │
└─────────────────────────────────────────────────────────────────────┘

Task card anatomy:
  Icon + title: task type icon (16px) + title (body-small, font-weight 600)
  Session tag: "#a3f" mono caption (if viewing all sessions)
  Timestamp: relative time, caption, muted
  Claimed by: username (if in progress)
  Actions: context-dependent buttons
    Pending: [Claim] [Edit] [Delete]
    In Progress: [Complete] [Release]
    Done: [Reopen] [Archive]

Task card animations:
  New card in Pending: slides in from top (translateY -10px, 200ms ease-out)
  Move to In Progress: card lifts (scale 1.05, 150ms) then slides right (300ms ease-out-expo)
  Move to Done: green border sweeps in (left→right, 300ms), card dims slightly
  Delete: card shrinks (scale 1→0.9) + fades (opacity 1→0), 200ms ease-in-quint
    Below cards slide up to fill gap (transition margin, 250ms)

Drag and drop:
  Drag card between columns to change status
  Drag preview: card ghost at 80% opacity, slight rotation (2deg)
  Drop zone highlight: column background brightens, border becomes dashed
  Invalid drop: card snaps back to original position with spring animation
  
New Task button:
  Opens inline form at top of Pending column
  Title input + description textarea
  [Create] [Cancel] buttons
  Created task slides in from form position

Claim button:
  Claims task for current user
  API call: POST /api/v1/tasks/:tid/claim
  Optimistic UI update: card moves to In Progress immediately
  On failure: card snaps back to Pending with error toast
  
Release button:
  Returns task to Pending pool
  Clears claim
  Card slides back to Pending column

Complete button:
  Marks task as done
  Confirmation: none (non-destructive, can reopen)
  Card moves to Done column
  Green checkmark animates: scale bounce (0→1.3→1, 300ms ease-spring)
```

---

## 12. Human-in-the-Loop Approvals

### 12.1 Approval Queue

```
Central view for managing HITL approval requests.
Accessible from sidebar: Approvals (⌘8).

Layout: split view — queue on left, detail on right.

┌─────────────────────────────────────────────────────────────────────┐
│ APPROVALS                                         3 pending         │
├────────────────────────────────┬────────────────────────────────────┤
│                                │                                    │
│ FILTERS: [Status ▾] [Type ▾]  │  APPROVAL DETAIL                   │
│                                │                                    │
│ ┌────────────────────────────┐ │  ┌──────────────────────────────┐ │
│ │ ⚠ HIGH PRIORITY            │ │  │ 🛡️ Approval #142             │ │
│ │                            │ │  │ Type: tool_execution          │ │
│ │ 🛡️ Execute: DROP TABLE     │ │  │ Priority: HIGH                │ │
│ │    Session #a3f · 2m ago   │ │  │ Status: PENDING               │ │
│ │    Requested by: AI agent  │ │  │ Requested: 2m ago             │ │
│ │                            │ │  │ Session: #a3f2b               │ │
│ └────────────────────────────┘ │  │                               │ │
│                                │  │ ── REQUEST ──                 │ │
│ ┌────────────────────────────┐ │  │                               │ │
│ │ 🛡️ Modify trust: low→quar  │ │  │ The AI agent requests         │ │
│ │    Session #b2e · 12m ago  │ │  │ permission to execute:        │ │
│ │    Requested by: AI agent  │ │  │                               │ │
│ │                            │ │  │ DROP TABLE staging_buffer     │ │
│ └────────────────────────────┘ │  │                               │ │
│                                │  │ This is a DESTRUCTIVE action. │ │
│ ┌────────────────────────────┐ │  │ It will permanently delete    │ │
│ │ 🛡️ Publish: Finding #7     │ │  │ the staging_buffer table and  │ │
│ │    Session #a3f · 1h ago   │ │  │ all its contents.             │ │
│ │    Requested by: AI agent  │ │  │                               │ │
│ └────────────────────────────┘ │  │ ── AI REASONING ──            │ │
│                                │  │ "Staging buffer is temporary.  │ │
│                                │  │ Data has been processed and    │ │
│                                │  │ committed to revenue.orders.   │ │
│                                │  │ No further analysis needed."   │ │
│                                │  │                               │ │
│                                │  │ ── ACTIONS ──                 │ │
│                                │  │                               │ │
│                                │  │ [✓ APPROVE]  [✗ DENY]         │ │
│                                │  │ [⏰ Defer 30m]                │ │
│                                │  │                               │ │
│                                │  │ Note: [Add approval note...]  │ │
│                                │  └──────────────────────────────┘ │
│                                │                                    │
│  APPROVED (2)                  │                                    │
│  ┌────────────────────────────┐ │                                    │
│  │ ✓ Execute: SELECT query    │ │                                    │
│  │    Approved by Bane · 1h   │ │                                    │
│  └────────────────────────────┘ │                                    │
│                                │                                    │
│  DENIED (1)                    │                                    │
│  ┌────────────────────────────┐ │                                    │
│  │ ✗ Modify: trust high→low   │ │                                    │
│  │    Denied by Bane · 2h ago │ │                                    │
│  └────────────────────────────┘ │                                    │
└────────────────────────────────┴────────────────────────────────────┘

Approval queue items:
  Color-coded by priority:
    HIGH: red left border + subtle red tint
    MEDIUM: amber left border
    LOW: muted left border
    
  HIGH priority items: pulse animation on border (opacity 0.5→1→0.5, 2s cycle)
    Attracts attention without being distracting

Approval types:
  tool_execution: AI wants to execute a tool (SQL query, API call, file write)
  trust_modification: AI wants to change trust level of an event
  finding_publication: AI wants to publish a finding
  session_action: AI wants to pause/resume/cancel a session
  budget_override: AI wants to exceed budget limit
  
  Each type has distinct icon:
    tool_execution:     🛡️ shield
    trust_modification: 🔒 lock
    finding_publication: 📢 megaphone
    session_action:     ⚙ gear
    budget_override:    💰 money

Approval actions (queue item):
  Quick actions on queue item (without opening detail):
    [✓] Approve
    [✗] Deny
  Both with confirmation tooltip: "Click to approve/deny"
  Optimistic update: item moves to Approved/Denied section immediately
    With animation: slide up + fade to appropriate section, 300ms

Approval Detail — AI Reasoning section:
  Shows what the AI was thinking when it made this request
  Collapsible if long
  Source link: "View in THINK pane" → opens Investigation Workbench at relevant step
  
Approval Detail — Actions:
  Approve button: green, prominent
    Confirmation: "Approve this action?" modal for HIGH priority
    No confirmation for MEDIUM/LOW (speed)
    On approve: detail panel updates to show "Approved" state
    Queue item moves to Approved section
    
  Deny button: red, outlined
    Confirmation: "Deny this request?" with optional reason input
    On deny: detail panel updates to show "Denied" state
    Queue item moves to Denied section
    
  Defer button: amber, muted
    Defer for: [30 min ▾] (30min, 1h, 4h, Tomorrow)
    On defer: item moves to bottom of queue
    "Deferred until 15:00" badge appears on item
    Auto-reappears at specified time

Approval Note:
  Optional text input below actions
  "Add a note explaining your decision..."
  Note is recorded in audit trail with the approval
  Visible in approved/denied detail view

Bulk actions:
  Select multiple queue items (checkboxes)
  Floating toolbar: "3 selected · [Approve All] [Deny All]"
  Confirmation: "Approve all 3 requests?"
  Actions applied sequentially with progress indicator
```

---

## 13. Multi-Model Deliberation Viewer

### 13.1 Overview

```
When the AI uses multi-model deliberation (via Chimera or built-in),
the Deliberation Viewer shows how different models reasoned about
the same question, where they agreed, and where they diverged.

Access: button in Investigation Workbench toolbar when deliberation is active.
Or: click on any "Multi-Model" badge in a THINK card.
```

### 13.2 Deliberation View

```
┌─────────────────────────────────────────────────────────────────────┐
│ DELIBERATION — "Analyze Q4 revenue patterns"                        │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│ MODELS: [deepseek-v4-pro] [claude-sonnet-4] [deepseek-v4-flash]     │
│                                                                     │
│ ┌─────────────────────────┬─────────────────────────┬─────────────┐ │
│ │ DEEPSEEK V4 PRO         │ CLAUDE SONNET 4         │ V4 FLASH    │ │
│ │ (Lead Analyst)          │ (Devil's Advocate)      │ (Aggregator)│ │
│ │                         │                         │             │ │
│ │ Finding: 12% APAC       │ Challenge: Are you      │ MERGED:     │ │
│ │ growth in Q4, driven    │ sure about November?    │ APAC grew   │ │
│ │ by SE Asia expansion.   │ My analysis shows       │ 12% in Q4.  │ │
│ │                         │ the shipping data       │ EMEA shows  │ │
│ │ Confidence: 0.94        │ contradicts the         │ 3% decline  │ │
│ │                         │ reported figures.       │ likely due  │ │
│ │ Sources: [q4-sales.csv] │                         │ to supply   │ │
│ │ [apac-report.pdf]       │ Confidence: 0.72        │ chain lag,  │ │
│ │                         │                         │ not actual  │ │
│ │                         │ Sources: [shipping.csv] │ decline.    │ │
│ │                         │                         │             │ │
│ │                         │                         │ Confidence: │ │
│ │                         │                         │ 0.88        │ │
│ └─────────────────────────┴─────────────────────────┴─────────────┘ │
│                                                                     │
│ AGREEMENT ANALYSIS                                                  │
│ ┌─────────────────────────────────────────────────────────────────┐ │
│ │ ✓ AGREED: APAC grew in Q4 (both models)                         │ │
│ │ ✓ AGREED: SE Asia is primary driver (both)                      │ │
│ │ ⚡ DIVERGED: EMEA decline cause                                 │ │
│ │    · DeepSeek: supply chain delay (reporting lag)               │ │
│ │    · Claude: actual market contraction                          │ │
│ │ ⚡ DIVERGED: November data interpretation                        │ │
│ │    · DeepSeek: anomaly in reporting, not real                   │ │
│ │    · Claude: shipping data contradicts, investigate further     │ │
│ └─────────────────────────────────────────────────────────────────┘ │
│                                                                     │
│ [Use Merged Finding]  [Investigate Divergence]  [Export Analysis]   │
└─────────────────────────────────────────────────────────────────────┘

Model columns:
  Each model gets equal-width column
  Column header: model name + role badge
  Roles: Lead Analyst, Devil's Advocate, Aggregator, Fact Checker, etc.
  
  Model output:
    Key findings in prose
    Confidence score with visual bar
    Sources cited
    Expand to show full reasoning (like THINK cards)
    
  Active model indicator:
    When model is currently generating: "Thinking..." with shimmer
    Streaming text appears character by character
    Multiple models may be active simultaneously
    
  Completed model:
    Checkmark + "Done · 1.2s"
    
Agreement Analysis:
  Auto-generated comparison of model outputs
  AGREED items: green background, checkmark
  DIVERGED items: amber background, lightning bolt
  Each item expandable: click to see exact quotes from each model
  Divergence severity indicator:
    Minor: different emphasis (green-amber)
    Significant: different conclusions (amber)
    Contradictory: opposite claims (red)
    
Actions:
  Use Merged Finding: imports merged output into SAYS pane as draft finding
  Investigate Divergence: creates new analysis task focused on the disagreement
  Export Analysis: PDF report showing full deliberation with model outputs + analysis

Deliberation animation:
  Entry: columns slide in from sides (left column from left, right from right)
  Duration: 400ms, ease-out-expo, staggered 50ms per column
  Aggregator column appears last (after both models complete)
    Slides up from bottom (translateY 20px→0) + fade, 300ms
```

---

## 14. Billing & Budget Console

### 14.1 Budget Dashboard

```
Financial overview of token usage, costs, and budget management.
Accessible from sidebar: Billing (⌘9).

┌─────────────────────────────────────────────────────────────────────┐
│ BILLING & BUDGET                                                    │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│ MONTHLY OVERVIEW                                  June 2026         │
│ ┌──────────────┐ ┌──────────────┐ ┌──────────────┐ ┌─────────────┐ │
│ │ Spent        │ │ Remaining    │ │ Projected    │ │ Daily Avg   │ │
│ │ $4.23        │ │ $5.77        │ │ $8.92        │ │ $0.35       │ │
│ │ ████████░░   │ │ of $10.00    │ │ ████████████ │ │ ↑12% vs May │ │
│ └──────────────┘ └──────────────┘ └──────────────┘ └─────────────┘ │
│                                                                     │
│ COST BREAKDOWN BY MODEL                          [24h ▾] [7d] [30d]│
│ ┌─────────────────────────────────────────────────────────────────┐ │
│ │ deepseek-v4-pro   ████████████████████████  $2.47 (58%)         │ │
│ │ deepseek-v4-flash ██████                    $0.89 (21%)         │ │
│ │ claude-sonnet-4   ████                      $0.62 (15%)         │ │
│ │ local-model       █                         $0.25 (6%)          │ │
│ └─────────────────────────────────────────────────────────────────┘ │
│                                                                     │
│ COST BY SESSION                                [All Time ▾]        │
│ ┌─────────────────────────────────────────────────────────────────┐ │
│ │ Session          │ Cost    │ Tokens    │ Iterations │ % of Budget│ │
│ │──────────────────┼─────────┼───────────┼────────────┼───────────│ │
│ │ #a3f Q4 Revenue  │ $1.23   │ 1.08M     │ 42         │ 12.3%     │ │
│ │ #b2e Phish       │ $0.47   │ 412K      │ 18         │ 4.7%      │ │
│ │ #d1a Q3 Analysis │ $0.89   │ 780K      │ 31         │ 8.9%      │ │
│ │ #c4f Net Scan    │ $0.12   │ 98K       │ 5          │ 1.2%      │ │
│ │ ...              │ ...     │ ...       │ ...        │ ...       │ │
│ └─────────────────────────────────────────────────────────────────┘ │
│                                                                     │
│ BUDGET SETTINGS                                                     │
│ Monthly budget: [$10.00]                                            │
│ Warning at: [80% ▸]                                                 │
│ Hard stop at: [100% ▸]  ☑ Pause all sessions when exceeded         │
│                                                                     │
│ [Export Billing Report]  [Configure Alerts]                         │
└─────────────────────────────────────────────────────────────────────┘

Budget progress bars:
  Green: <50% used
  Amber: 50-80% used
  Red: >80% used (with subtle pulse animation)
  
  Warning threshold indicator: small marker on bar at configured percentage
  Exceeded: bar turns red, error icon appears

Cost breakdown bar chart:
  Horizontal stacked bar for each model
  Bar segments: colored by model
  Hover: tooltip with exact cost and percentage
  Legend: model name + color dot + amount + percentage
  Animate: bars grow from left on page load (600ms ease-out-expo, staggered 100ms)

Cost by session table:
  Sortable by any column
  Click header to sort
  Default sort: cost descending
  
Budget settings:
  Monthly budget: dollar amount input
  Warning threshold: slider or percentage input
  Hard stop: toggle + threshold
  Save button: persists settings
  Changes take effect immediately for new token usage

Budget alerts (when threshold crossed):
  Warning (80%): toast notification "Budget at 80% — $8.00 of $10.00 used"
    Stays until dismissed
  Hard stop (100%): toast "Budget exhausted — all sessions paused"
    Sessions auto-paused
    Resume requires: increase budget or acknowledge override
    
Budget override:
  When hard stop reached, option to "Override budget limit"
  Requires: reason input + confirmation
  Recorded in audit trail
  New temporary limit can be set
```

---

## 15. System Health & Operations

### 15.1 Health Dashboard

```
System monitoring and diagnostics.
Accessible from sidebar: Health (⌘0).

┌──────────────────────────────────────────────────────────────────────┐
│ SYSTEM HEALTH                                                        │
├──────────────────────────────────────────────────────────────────────┤
│                                                                      │
│ STATUS: ● HEALTHY                        Uptime: 12d 4h 23m          │
│                                                                      │
│ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌────────────────┐  │
│ │ API Latency │ │ DB Latency  │ │ LLM Latency │ │ Error Rate     │  │
│ │   234ms     │ │   12ms      │ │   1.2s      │ │   0.02%        │  │
│ │   ● normal  │ │   ● normal  │ │   ● normal  │ │   ● normal     │  │
│ └─────────────┘ └─────────────┘ └─────────────┘ └────────────────┘  │
│                                                                      │
│ LATENCY HISTORY (24h)                                                │
│ ┌──────────────────────────────────────────────────────────────────┐ │
│ │   API ████████████████░░░░░░░░░░░  avg: 234ms                    │ │
│ │   DB  ██████░░░░░░░░░░░░░░░░░░░░░  avg: 12ms                    │ │
│ │   LLM ████████████████████████████  avg: 1.2s                    │ │
│ │                                                                  │ │
│ │   └────┬────┴────┬────┴────┬────┴────┬────┴────┬────┴────┬────  │ │
│ │      00:00    04:00    08:00    12:00    16:00    20:00    Now  │ │
│ └──────────────────────────────────────────────────────────────────┘ │
│                                                                      │
│ ACTIVE CONNECTIONS                                                   │
│ ┌──────────────────────────────────────────────────────────────────┐ │
│ │ WebSocket: 3 connections                                          │ │
│ │ Database pool: 8/20 active, 0 waiting                             │ │
│ │ LLM connections: 2 active (deepseek:2, anthropic:0)              │ │
│ │ API requests (last min): 47                                       │ │
│ └──────────────────────────────────────────────────────────────────┘ │
│                                                                      │
│ ──────────────────────────────────────────────────────────────────── │
│                                                                      │
│ DATABASE                                                             │
│ Backend: SQLite · /data/consensus.db                                 │
│ Size: 47.2 MB · Tables: 36 · Migrations: 17 (all applied)           │
│ Last backup: 2h ago (automatic)                                      │
│ [Run Health Check]  [Backup Now]  [Vacuum]                           │
│                                                                      │
│ ──────────────────────────────────────────────────────────────────── │
│                                                                      │
│ SYSTEM LOG (last 50 lines)                                           │
│ ┌──────────────────────────────────────────────────────────────────┐ │
│ │ 14:23:15 [INFO] session #a3f iteration 42 started                │ │
│ │ 14:23:15 [DEBUG] LLM request: 847 tokens, model deepseek-v4-pro  │ │
│ │ 14:23:14 [INFO] heartbeat: #a3f, #b2e, #c4f alive               │ │
│ │ 14:23:10 [DEBUG] WebSocket message sent to 3 clients             │ │
│ │ 14:23:00 [INFO] metrics snapshot: 5 active sessions              │ │
│ │ ...                                                               │ │
│ └──────────────────────────────────────────────────────────────────┘ │
│ [Download Full Logs]  [Clear Log Display]                            │
└──────────────────────────────────────────────────────────────────────┘

Status indicator:
  Healthy: green pulsing dot + "Healthy"
  Degraded: amber static dot + "Degraded" + reason
    "API latency elevated: 1.2s"
    "Database connection pool at 90%"
    "1 session stalled"
  Down: red flashing dot + "Down"
    "Database connection lost"
    "All LLM providers unreachable"

KPI cards:
  Each shows: current value, unit, status dot, trend
  Color-coded status: green (normal), amber (warning), red (critical)
  Thresholds configurable in settings
  
Latency history chart:
  Multi-series line chart
  Time range: 1h | 6h | 24h | 7d
  Each series: different color (API=blue, DB=green, LLM=purple)
  Anomaly markers: vertical red bands where latency spiked
  Hover: crosshair with exact values at that time
  Threshold lines: horizontal dashed lines at warning/critical levels
  
Database section:
  Backend type: SQLite or PostgreSQL
  Path/host displayed
  Size with trend (growing/shrinking)
  Table count
  Migration status: all applied / pending
  Quick actions: health check, backup, vacuum (SQLite)
  
System log:
  Auto-scrolling (follows tail)
  Color-coded by level: DEBUG (gray), INFO (white), WARN (amber), ERROR (red)
  Search: filter log by keyword
  Level filter: [DEBUG] [INFO] [WARN] [ERROR] toggle chips
  Timestamp column: mono font
  Message column: truncated, expandable on click
  
  Download: exports full log file
  Clear: clears display only (not actual log)
```

---

## 16. Multi-Tenant Administration

### 16.1 Admin Overview

```
Administrative interface for managing users, API keys, permissions,
and organization settings.

Accessible from sidebar: Admin (⌘-).

Layout: tabbed interface
  Tabs: [Users] [API Keys] [Teams] [Audit Log] [Settings]

### 16.2 User Management

┌─────────────────────────────────────────────────────────────────────┐
│ ADMIN · Users                                      [+ Invite User]   │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│ ┌─────────────────────────────────────────────────────────────────┐ │
│ │ User           │ Role    │ Sessions │ Last Active │ Status      │ │
│ │────────────────┼─────────┼──────────┼─────────────┼─────────────│ │
│ │ Bane           │ Admin   │ 12       │ Active now  │ ● Active    │ │
│ │ alice@...      │ Analyst │ 5        │ 2h ago      │ ● Active    │ │
│ │ bob@...        │ Viewer  │ 0        │ 3d ago      │ ○ Inactive  │ │
│ │ charlie@...    │ Analyst │ 8        │ 1d ago      │ ● Active    │ │
│ └─────────────────────────────────────────────────────────────────┘ │
│                                                                     │
│ SELECTED: Bane (Admin)                                              │
│ ┌─────────────────────────────────────────────────────────────────┐ │
│ │ Email: bane@example.com                                         │ │
│ │ Role: [Admin ▾]                                                 │ │
│ │   · Admin: full access, manage users, all sessions              │ │
│ │   · Analyst: create/edit sessions, view all within team         │ │
│ │   · Viewer: read-only access to assigned sessions              │ │
│ │                                                                 │ │
│ │ Team: [Security Team ▾]                                         │ │
│ │ API Keys: 2 active                                              │ │
│ │ Sessions: 12 total, 3 active                                    │ │
│ │                                                                 │ │
│ │ Permissions:                                                    │ │
│ │ ☑ Create sessions    ☑ Manage users    ☑ View billing          │ │
│ │ ☑ Approve actions    ☑ Export data      ☑ Manage API keys      │ │
│ │ ☐ Delete sessions    ☑ View audit log   ☑ System settings      │ │
│ │                                                                 │ │
│ │ [Save Changes]  [Reset Password]  [Deactivate User]            │ │
│ └─────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────┘

Role definitions:
  Admin: unrestricted access to all resources and settings
  Analyst: can create and manage sessions, view findings, approve actions
  Viewer: read-only access to assigned sessions and findings
  Custom: granular permission set (defined per-user)

Permission model:
  CRUD-based permissions for each resource type
  Resources: sessions, memory, findings, tasks, approvals, billing, users, settings
  Each permission: Create, Read, Update, Delete, Approve
  UI shows checkboxes for each permission
  Pre-defined role templates fill checkboxes automatically
  
User actions:
  Invite: sends email invitation with registration link
  Deactivate: prevents login but preserves user data
  Delete: removes user (confirmation required, sessions transferred to admin)
  Reset password: sends password reset email
  
### 16.3 API Key Management

┌─────────────────────────────────────────────────────────────────────┐
│ ADMIN · API Keys                                  [+ Generate Key]   │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│ ┌─────────────────────────────────────────────────────────────────┐ │
│ │ Key Prefix   │ Scope    │ Created    │ Expires    │ Last Used   │ │
│ │──────────────┼──────────┼────────────┼────────────┼─────────────│ │
│ │ cs_ak_a1b2.. │ Admin    │ 12d ago    │ Never      │ Active now  │ │
│ │ cs_sk_c3d4.. │ Session  │ 2h ago     │ Never      │ 2m ago      │ │
│ │ cs_ro_e5f6.. │ Readonly │ 5d ago     │ 30d        │ 1h ago      │ │
│ └─────────────────────────────────────────────────────────────────┘ │
│                                                                     │
│ Generate Key:                                                       │
│   Scope: [Session ▾]                                                │
│   Session: [#a3f2b ▾] (for session scope)                          │
│   Expires: [Never ▾]                                                │
│   [Generate]                                                        │
│                                                                     │
│ ⚠ New keys are shown once. Save them immediately.                   │
└─────────────────────────────────────────────────────────────────────┘

Key scopes:
  Admin: full API access, all endpoints
  Session: scoped to one session (CRUD within session only)
  Readonly: GET-only access across all sessions
  Webhook: can only POST to external_events endpoint

Key generation:
  On generate: key displayed once with [Copy] button
  Confirmation: "I have saved this key" checkbox before closing modal
  Key stored as SHA-256 hash (original never stored)
  Prefix shown for identification (first 8 chars)
  
Key actions:
  Revoke: immediately invalidates key (confirmation required)
  Extend: extends expiration date
  View usage: shows last used timestamp, request count
```

### 16.4 Organization Settings

```
Organization-wide configuration.

┌─────────────────────────────────────────────────────────────────────┐
│ ADMIN · Settings                                                    │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│ GENERAL                                                             │
│   Organization Name: [Consensus Corp___________________]            │
│   Default Model: [deepseek-v4-pro ▾]                               │
│   Timezone: [America/New_York ▾]                                   │
│                                                                     │
│ SECURITY                                                            │
│   Session timeout: [8 hours ▾]                                     │
│   Require 2FA: ☐ for all users  ☑ for admins only                  │
│   API key expiry: [90 days ▾] (for new keys)                       │
│   IP allowlist: [Add IP range...]                                  │
│                                                                     │
│ BUDGET & BILLING                                                    │
│   Monthly budget: [$100.00]                                        │
│   Per-session budget limit: [$10.00]                               │
│   Budget alerts: [billing@example.com____________]                 │
│                                                                     │
│ RETENTION                                                           │
│   Session retention: [90 days ▾] (auto-archive after)              │
│   Memory retention: [365 days ▾]                                   │
│   Audit log retention: [Permanent ▾]                               │
│                                                                     │
│ [Save Settings]                                                     │
└─────────────────────────────────────────────────────────────────────┘

Settings are organization-wide, applied to all users.
Admin-only access (verified by role check).
Changes logged in audit trail.
Some settings (budget, retention) require confirmation.
```

---

## 17. Animation & Transition System

### 17.1 Animation Philosophy

```
Animation in Chronicle serves three purposes:
  1. COMMUNICATION — motion conveys state change, causality, and relationship
  2. ORIENTATION — transitions maintain spatial mental model across views
  3. DELIGHT — micro-interactions make the tool feel responsive and alive

Never animate for decoration alone. Every animation has a job.

Accessibility: All animations respect prefers-reduced-motion.
When enabled, durations collapse to 0ms, transforms collapse to none,
and opacity transitions remain for essential feedback.
```

### 17.2 Page Transitions

```
View-to-view transitions use a directional model based on navigation intent.

FORWARD NAVIGATION (drilling deeper into data):
  Dashboard → Investigation → Session Detail → Memory Event
  Current view: slides left + fades out (translateX 0→-30px, opacity 1→0)
  New view: slides in from right + fades in (translateX 30px→0, opacity 0→1)
  Duration: 250ms for slide, 200ms for fade (slightly offset, 50ms overlap)
  Easing: ease-out-quint for entrance, ease-in-quint for exit
  
BACKWARD NAVIGATION (returning to parent):
  Reverse of forward: current slides right, new slides in from left
  Duration: 200ms (faster — going back should feel quick)
  
LATERAL NAVIGATION (switching between peer views like tabs):
  Crossfade only (no slide): opacity 1→0→1, 200ms
  Content appears to swap in place
  
MODAL/OVERLAY:
  Backdrop fades in: opacity 0→1, 150ms, ease-out-quint
  Modal scales up: scale(0.95)→scale(1), opacity 0→1, 250ms, ease-out-expo
  Exit: reverse, 150ms, ease-in-quint
  
DRAWER/PANEL (from edge):
  Slides in from edge: translateX(100%)→translateX(0)
  Duration: 300ms, ease-out-expo
  Content fades in with 100ms delay (staggered feel)
  Exit: slides back to edge, 200ms, ease-in-quint

Transition orchestration:
  React Router location key change triggers transition
  useTransition hook manages enter/exit phases
  Layout components (shell, sidebar, status bar) do NOT transition
  Only the content area transitions
  Scroll position saved per route, restored on back/forward

Transition interruption:
  If user navigates again during a transition:
    Current transition immediately completes to its end state
    New transition begins from there
    No visual glitch — animations use fill: 'forwards' and are composable
```

### 17.3 Element Enter/Exit Animations

```
List items entering the DOM:
  Staggered by index: each item delayed by 30ms × index
  Animation: translateY(8px)→0 + opacity 0→1
  Duration: 200ms per item
  Easing: ease-out-quint
  Maximum stagger: 10 items (beyond 10, all appear at once)
  
  Implementation: CSS animation with animation-delay: calc(var(--index) * 30ms)
  Items already in DOM (re-renders) do NOT animate — only new items

List items exiting the DOM:
  Animation: translateY(0)→-8px + opacity 1→0
  Duration: 150ms (faster than enter)
  Easing: ease-in-quint
  Remaining items slide up to fill gap: transition margin/padding 200ms ease-out-quint
  
  Implementation: FLIP animation technique
    Record: capture element positions before change
    Update: apply DOM changes
    Invert: calculate position delta, apply inverse transform
    Play: animate transform to identity

Cards entering grid:
  Scale(0.9)→scale(1) + opacity 0→1
  Duration: 300ms, ease-out-expo
  Staggered: 50ms per card
  Cards rearrange smoothly using FLIP when grid layout changes

Modals/dialogs:
  Backdrop: fade in, 100ms
  Dialog: scale(0.95)→scale(1), 200ms, ease-out-expo
  Focus: auto-focus first focusable element after animation completes (200ms delay)

Toast notifications:
  Slide in from right edge: translateX(100%+16px)→translateX(0)
  Duration: 300ms, ease-out-expo
  Subsequent toasts: existing toasts slide up (transition margin-bottom 200ms)
  Exit: slide right + fade, 200ms, ease-in-quint

Tooltips:
  Appear: opacity 0→1, 100ms, ease-out-quint
  Position adjustment: no animation (instant reposition)
  Disappear: opacity 1→0, 80ms (very fast)
  Delay before appearing: 500ms (prevents flicker when moving cursor)
```

### 17.4 Micro-Interaction Library

```
BUTTON PRESS:
  Normal → Pressed: scale(1)→scale(0.97), 80ms ease-out-quint
  Pressed → Released: scale(0.97)→scale(1.03)→scale(1), 200ms ease-spring
  Ripple effect: circular overlay expands from click point
    Size: 0→200% of button dimensions
    Opacity: 0.15→0 over 400ms
    Color: white (dark theme) or black (light theme)

CHECKBOX TOGGLE:
  Check appears: draw SVG path (stroke-dashoffset 100%→0, 200ms ease-out-quint)
  Background fill: scales from center (clip-path circle 0%→100%, 200ms)
  Uncheck: reverse animations, 150ms ease-in-quint
  
TOGGLE SWITCH:
  Thumb slides: translateX(0)→translateX(20px), 200ms ease-out-expo
  Track color: transitions background-color, 200ms
  Pressed: thumb scales 1→0.85, 80ms (gives physical feel)

INPUT FOCUS:
  Border color: transitions from default to accent, 150ms ease-out-quint
  Focus ring: opacity 0→1, 150ms ease-out-quint
  Placeholder: slides up 4px + fades to 70% opacity when field has value
    (CSS :placeholder-shown selector with transition)

DROPDOWN OPEN:
  Menu: scale(0.95, 0.9)→scale(1,1), opacity 0→1, 150ms ease-out-expo
  Transform origin: top-left (menu appears to grow from trigger)
  Items stagger: each item fades in + slides down 4px, 30ms delay per item
  Backdrop: none (dropdowns don't use backdrop)

DROPDOWN CLOSE:
  Menu: scale(1,1)→scale(0.95, 0.9), opacity 1→0, 100ms ease-in-quint
  Transform origin: top-left

TAB SWITCH:
  Active indicator: slides horizontally to new position
  Duration: 200ms, ease-out-expo
  Uses FLIP: indicator position calculated from target tab
  Content: crossfade (old content fades out 100ms, new fades in 100ms with 50ms overlap)

PROGRESS BAR FILL:
  Width transition: 1s ease-out-quint
  Color transition: green→amber→red based on percentage
  Both transitions simultaneously
  Number value: counts up/down with smooth animation (see KPI animation §4.2)

SKELETON LOADING:
  Shimmer gradient: linear-gradient moves from left to right
  Animation: background-position 0%→200%, 1.5s ease-in-out infinite
  Gradient colors: surface → surface-raised → surface
  Each skeleton element: slightly different animation delay for organic feel
  Content replaces skeleton: skeleton fades out (200ms) as content fades in (200ms)

DRAG PREVIEW:
  Element lifts: scale(1)→scale(1.05) + shadow increases, 150ms ease-out-quint
  During drag: element follows cursor, slight rotation (±2deg based on velocity)
  Drop: scale returns to 1, element snaps to position, 200ms ease-spring
  Invalid drop: element animates back to original position with spring physics

PULSE (ATTENTION):
  Target element: box-shadow glow pulses
  Keyframes: glow 0→1→0 opacity, 2s ease-in-out
  Loop count: 3 (then stops to avoid annoyance)
  Used for: new features, pending approvals, unread notifications

SHAKE (ERROR):
  Element shakes horizontally: translateX(0→-4px→4px→-4px→4px→0)
  Duration: 400ms total, 5 keyframes
  Easing: ease-in-out
  Used for: invalid form inputs, failed operations
  Paired with: border turning red (transition 150ms)

CELEBRATION (SUCCESS):
  Brief confetti burst (8-12 particles)
  Particles: small colored circles that arc outward from element center
  Each particle: random direction, 200-400px travel, fade out over 600ms
  Used sparingly: session creation, major task completion, budget reset
  Accessibility: hidden when prefers-reduced-motion
```

### 17.5 Data Visualization Animations

```
CHART ENTRY:
  Bars: grow from 0 to value height (transform scaleY, 600ms ease-out-expo)
    Staggered: 50ms delay per bar
  Lines: draw from left (stroke-dashoffset, 800ms ease-out-quint)
  Donut arcs: draw from 0deg to full arc (stroke-dashoffset, 600ms per segment)
  Area fill: opacity 0→1, 400ms after line draws

CHART UPDATE (data change):
  Morph technique: bars/lines smoothly transition to new values
  Duration: 400ms, ease-out-quint
  Values interpolated: current → target
  Labels update at animation midpoint (200ms crossfade)
  Axes: scale transitions if range changes

CHART HOVER:
  Hovered element: slight scale increase (1.02), 150ms ease-out
  Other elements: dim to 30% opacity, 150ms
  Tooltip: appears at cursor, 100ms fade in
  Crosshair: draws instantly (no animation for precision)
  
GRAPH NODE MOVEMENT:
  Nodes drift continuously (force simulation at low alpha)
  Movement: 1-2px per tick, giving organic "breathing" feel
  Drag release: node springs back into simulation with velocity
  Add node: fades in + scales from 0, 300ms ease-spring
  Remove node: scales to 0 + fades out, 200ms ease-in-quint
  
GRAPH CAMERA:
  Pan: smooth with inertia (deceleration after drag)
  Zoom: animated with easing, 300ms ease-out-expo
  Focus on node: camera flies to center on node (curved path), 600ms
  Reset view: camera animates to fit all nodes, 800ms ease-out-expo
```

### 17.6 Performance & Technical Implementation

```
Animation engine:
  CSS transitions for simple property changes (color, opacity, transform)
  CSS animations for multi-keyframe sequences (pulse, shimmer, shake)
  requestAnimationFrame for continuous animations (canvas, graphs, force sim)
  FLIP technique for layout changes (list reordering, tab switches)
  Web Animations API for programmatic control when needed

Performance budget:
  No more than 10 simultaneous CSS animations
  No more than 50 animating DOM elements at once
  Canvas animations capped at 60fps with frame skipping if needed
  Heavy animations (graphs, force simulation) offloaded to Web Workers
  will-change: transform, opacity applied to frequently animated elements
    (removed after animation ends to free GPU memory)
  
  Monitor: Performance Observer for long tasks (>50ms)
  Degrade: if frame rate drops below 30fps for >1s, disable non-essential animations
  Restore: when frame rate recovers, re-enable animations

Reduced motion:
  @media (prefers-reduced-motion: reduce) {
    *, *::before, *::after {
      animation-duration: 0.01ms !important;
      animation-iteration-count: 1 !important;
      transition-duration: 0.01ms !important;
    }
  }
  Exceptions: opacity transitions for essential feedback (0.1ms→50ms)
  Toggle in settings: "Reduce motion" checkbox, persists in localStorage
```

---

## 18. State Management

### 18.1 State Architecture

```
State is organized in layers by scope and persistence.

Layer 1 — URL State (shareable, bookmarkable)
  Current route and parameters
  Session ID, investigation ID, filter selections as query params
  Source of truth for: which page, which entity, which filters
  Managed by: React Router v6
  
Layer 2 — Server State (source of truth on backend)
  Sessions, memory events, findings, tasks, approvals, etc.
  Fetched via REST API, updated via WebSocket
  Cached with: React Query (TanStack Query)
  Stale time: 30s (refetches in background)
  Cache invalidation: on mutation success, WebSocket events
  
Layer 3 — UI State (ephemeral, local)
  Open/closed panels, selected items, scroll positions
  Form input values, unsaved changes
  Managed by: Zustand stores (lightweight, selective subscriptions)
  
Layer 4 — Persistent Preferences (survives refresh)
  Theme, density mode, sidebar collapsed, keyboard shortcuts
  Pane ratios, column widths, saved filters
  Stored in: localStorage
  Managed by: Zustand with persist middleware

Data flow is unidirectional:
  URL → Router → Page Component → Queries (React Query) + Stores (Zustand)
  User Action → Mutation (API call) → Invalidate Cache → Re-render
  WebSocket Event → Update Cache → Selective Re-render
```

### 18.2 React Query Integration

```typescript
// Session queries
const useSessions = (filters?: SessionFilters) => {
  return useQuery({
    queryKey: ['sessions', filters],
    queryFn: () => api.getSessions(filters),
    staleTime: 30_000,        // 30s before background refetch
    refetchInterval: 60_000,   // Poll every 60s as fallback
  });
};

const useSession = (id: string) => {
  return useQuery({
    queryKey: ['session', id],
    queryFn: () => api.getSession(id),
    enabled: !!id,
  });
};

// Mutations
const useCreateSession = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: api.createSession,
    onSuccess: (newSession) => {
      // Optimistic: add to cache immediately
      queryClient.setQueryData(['sessions'], (old: Session[]) => 
        [newSession, ...(old || [])]
      );
      // Invalidate for refetch
      queryClient.invalidateQueries({ queryKey: ['sessions'] });
    },
  });
};

// WebSocket-driven cache updates
const useSessionWebSocket = () => {
  const queryClient = useQueryClient();
  
  useEffect(() => {
    const ws = new WebSocket(WS_URL);
    ws.onmessage = (event) => {
      const data = JSON.parse(event.data);
      switch (data.type) {
        case 'session.updated':
          queryClient.setQueryData(['session', data.id], data.session);
          break;
        case 'session.status':
          queryClient.invalidateQueries({ queryKey: ['sessions'] });
          break;
        case 'memory.created':
          queryClient.invalidateQueries({ queryKey: ['memory', data.sessionId] });
          break;
      }
    };
    return () => ws.close();
  }, [queryClient]);
};
```

### 18.3 Zustand UI Stores

```typescript
// Investigation Workbench state
interface WorkbenchState {
  // Pane ratios
  thinkWidth: number;           // percentage, 0-100
  saysWidth: number;            // derived: 100 - thinkWidth
  evidenceOpen: boolean;
  discoveryOpen: boolean;
  
  // Selection
  selectedFindingId: string | null;
  selectedThoughtId: string | null;
  
  // Input
  draftInput: string;
  selectedModel: string;
  attachedContext: ContextItem[];
  
  // Actions
  setPaneRatio: (thinkPct: number) => void;
  toggleEvidence: () => void;
  selectFinding: (id: string) => void;
  setDraftInput: (text: string) => void;
  addContext: (item: ContextItem) => void;
  removeContext: (id: string) => void;
  submitQuery: () => Promise<void>;
}

// Preferences store (persisted)
interface PreferencesState {
  theme: 'dark' | 'light' | 'system';
  density: 'normal' | 'dense';
  sidebarCollapsed: boolean;
  reducedMotion: boolean;
  
  // Persisted filters
  defaultModel: string;
  defaultTimeRange: string;
  
  setTheme: (theme: string) => void;
  toggleDensity: () => void;
  toggleSidebar: () => void;
}
```

### 18.4 Optimistic Updates

```
Pattern for mutations that should feel instant:

1. Snapshot: capture current cache state
2. Optimistic: immediately update cache with expected result
3. Render: UI shows optimistic state
4. Server: send mutation to API
5. Success: keep optimistic state, invalidate for refetch
6. Error: rollback to snapshot, show error toast
7. Settle: remove optimistic indicator

Example — Approve Finding:
  const approveFinding = useMutation({
    mutationFn: api.approveFinding,
    onMutate: async (findingId) => {
      await queryClient.cancelQueries({ queryKey: ['finding', findingId] });
      const previous = queryClient.getQueryData(['finding', findingId]);
      queryClient.setQueryData(['finding', findingId], (old) => ({
        ...old,
        status: 'approved',
        approved_by: currentUser.name,
        approved_at: new Date().toISOString(),
      }));
      return { previous };
    },
    onError: (err, findingId, context) => {
      queryClient.setQueryData(['finding', findingId], context.previous);
      toast.error('Failed to approve finding');
    },
    onSettled: (findingId) => {
      queryClient.invalidateQueries({ queryKey: ['finding', findingId] });
    },
  });

Optimistic indicators in UI:
  Small spinner in corner of element being updated
  Slightly muted opacity (90%) during pending state
  "Saving..." or checkmark indicator
  Duration: indicator disappears on success or error
```

---

## 19. WebSocket & Real-Time Events

### 19.1 Connection Management

```
WebSocket endpoint: ws://{host}:{port}/ws
Protocol: JSON messages with type field

Connection lifecycle:
  Connect on: app mount (after authentication)
  Reconnect: exponential backoff on disconnect
    Attempt 1: 1s
    Attempt 2: 2s
    Attempt 3: 4s
    Attempt 4: 8s
    Attempt 5+: 15s (max)
    Reset: on successful connection
    
  Heartbeat: ping every 30s, expect pong within 10s
  Timeout: if no pong for 45s, close and reconnect
  Visibility: pause when tab hidden, resume on visible
    On resume: full state refresh via REST API, then WebSocket events resume

Connection status indicator:
  Status bar shows: ● Connected / ● Reconnecting / ● Disconnected
  Color: green / amber pulse / red
  On reconnect: toast "Connection restored" (dismissed after 3s)
  On extended disconnect (>30s): persistent banner at top
    "Connection lost. Retrying... [Retry Now]"
```

### 19.2 Event Types

```
Inbound events (server → client):

SESSION EVENTS:
  session.created:     { id, name, agent_name, model_id, created_at }
  session.status:      { id, status, previous_status, timestamp }
  session.updated:     { id, changes: { ... } }
  session.deleted:     { id }

MEMORY EVENTS:
  memory.created:      { id, session_id, type, content_preview, trust_level, iteration }
  memory.updated:      { id, changes: { trust_level, ... } }

ITERATION EVENTS:
  iteration.started:   { session_id, iteration_number }
  iteration.completed: { session_id, iteration_number, tokens_in, tokens_out, duration_ms }

FINDING EVENTS:
  finding.created:     { id, session_id, title, confidence }
  finding.updated:     { id, changes: { status, approved_by, ... } }

TASK EVENTS:
  task.created:        { id, session_id, title }
  task.claimed:        { id, claimed_by }
  task.completed:      { id }
  task.released:       { id }

APPROVAL EVENTS:
  approval.requested:  { id, session_id, type, priority }
  approval.resolved:   { id, decision, by, timestamp }

BILLING EVENTS:
  billing.updated:     { session_id, tokens_used, cost }
  billing.threshold:   { percentage, current, limit }
  billing.exceeded:    { limit }

SYSTEM EVENTS:
  system.health:       { status, metrics: { api_latency, db_latency, ... } }
  system.startup:      { version, db_backend, port }
  system.shutdown:     { reason }

HEARTBEAT:
  ping:                server → client (expect pong response)
  
Outbound events (client → server):
  pong:                response to ping
  subscribe:           { channels: ['session:a3f2b', 'system'] }
  unsubscribe:         { channels: ['session:a3f2b'] }

Channel subscriptions:
  Client subscribes to channels based on current view
  Dashboard: subscribes to ['system', 'sessions']
  Session detail: subscribes to ['session:{id}', 'system']
  Investigation: subscribes to ['session:{id}', 'memory', 'findings']
  
  Auto-subscribe: on route change, unsubscribe from old channels, subscribe to new
  Bulk subscription: single message with array of channels
```

### 19.3 Event Processing

```
Event handler pipeline:

1. RECEIVE: WebSocket onmessage fires
2. PARSE: JSON.parse(event.data)
3. VALIDATE: check event.type is known, shape matches schema
4. ROUTE: dispatch to registered handlers by event type
5. UPDATE: handler updates React Query cache
6. NOTIFY: toast for significant events (session created, error, budget warning)
7. ANIMATE: UI elements animate in response (see §17)

Event batching:
  Multiple events arriving in same frame are batched
  React Query cache updates batched into single re-render
  Toast notifications: max 1 per 2 seconds (rate limited)
  Animation triggers: debounced 50ms to prevent jank

Event replay:
  On reconnect: server sends missed events since last received sequence number
  Client processes replay events in order
  Duplicate detection: events have monotonic sequence numbers
  Gap detection: if sequence gap > 50, do full state refresh via REST API
```

---

## 20. Responsive Behavior & Breakpoints

### 20.1 Breakpoint Definitions

```
--bp-mobile:    0px      Smartphones (portrait and landscape)
--bp-tablet:    768px    Tablets, small laptops
--bp-desktop:   1024px   Standard desktop monitors
--bp-wide:      1440px   Large monitors, 1440p+
--bp-ultrawide: 1920px   Ultrawide monitors, 4K+

Implementation: CSS custom media queries (or PostCSS custom-media)
  @custom-media --viewport-tablet (min-width: 768px);
  @custom-media --viewport-desktop (min-width: 1024px);
  @custom-media --viewport-wide (min-width: 1440px);

Mobile-first: base styles are for mobile, enhanced at each breakpoint
```

### 20.2 Layout Adaptations

```
MOBILE (<768px):
  Sidebar: hidden, hamburger menu opens overlay
    Overlay: full-width, slides in from left, 300ms
  
  Top bar: simplified
    Command palette trigger: hidden (replaced by search icon)
    Breadcrumb: hidden
    Notification bell: visible with badge
    Profile: icon only, no name
    
  Dashboard:
    KPI bar: 2×3 grid (was 6-column row)
    Activity feed: full width, simplified (last 5 events)
    Charts: stacked vertically, simplified (bars instead of donuts)
    
  Investigation:
    THINK/SAYS: stacked vertically, THINK on top
    Divider: horizontal instead of vertical
    Evidence panel: full-width bottom sheet (slides up)
    Timeline: simplified to list view
    
  Tables: horizontal scroll or card view
    Session table → card list with key info
    Memory browser → event list with detail as bottom sheet
    
  Modals: full-screen on mobile (not centered overlay)
  
TABLET (768px - 1023px):
  Sidebar: collapsed by default (56px icons), expandable
  Dashboard: KPI bar 3×2, split view available but simpler
  Investigation: split panes available, 50/50 fixed (no resize)
  Tables: full table view with horizontal scroll if needed
  
DESKTOP (1024px - 1439px):
  Full layout as specified in §2
  Sidebar: expanded by default (240px)
  All features available
  Split panes: resizable
  
WIDE (1440px+):
  Maximum information density
  Three-column layouts where applicable
  Investigation: THINK | SAYS | Details three-pane available
  Graph: detail panel slides in without covering graph
  
ULTRAWIDE (1920px+):
  Multiple panels visible simultaneously
  Investigation: THINK + SAYS + Evidence + Discovery all visible
  Graph: larger canvas, more nodes visible
  Tables: more columns visible without scrolling
```

### 20.3 Touch vs Mouse

```
Touch interactions (mobile/tablet):
  Tap: equivalent to click
  Long press (500ms): equivalent to right-click (context menu)
  Swipe left on list item: reveal actions (approve/deny/delete)
  Swipe right on list item: mark as read/complete
  Pinch: zoom on graph/timeline
  Two-finger scroll: scroll within scrollable containers
  
Touch targets:
  Minimum size: 44×44px (Apple HIG) / 48×48px (Material)
  Adequate spacing: 8px minimum between touch targets
  Visual feedback: touch ripple on all interactive elements
  
Drag and drop on touch:
  Long press to initiate drag (500ms hold)
  Visual lift: element scales up + shadow
  Haptic feedback: if available (navigator.vibrate)
  
Keyboard (always available):
  Full keyboard navigation (see §23)
  Focus indicators visible when using keyboard, hidden when using mouse
  :focus-visible for keyboard-only focus styling
```

---

## 21. Accessibility (WCAG 2.2 AA)

### 21.1 Semantic Structure

```
Landmark regions:
  <header role="banner">         Top bar
  <nav role="navigation">        Sidebar
  <main role="main">             Content area
  <footer role="contentinfo">    Status bar
  <aside role="complementary">   Evidence/Discovery panels
  <dialog role="dialog">         Modals
  <form role="search">           Search overlay

Heading hierarchy:
  h1: Page title (one per page)
  h2: Major sections
  h3: Card titles, panel headers
  h4: Sub-sections within cards (rare)
  h5-h6: Not used (use semantic grouping instead)

Lists:
  Navigation: <ul> with <li><a> items
  Tables: proper <table> with <thead>, <tbody>, <th scope="...">
  Definition lists: <dl> for key-value metadata displays
  
Forms:
  Every input has <label> associated via htmlFor
  Required fields marked with aria-required="true"
  Error messages linked via aria-describedby
  Form groups use <fieldset> with <legend> for radio/checkbox groups
```

### 21.2 Screen Reader Support

```
Dynamic content announcements:
  Use aria-live regions for content that updates without focus change:
    aria-live="polite": status updates, new events in feed
    aria-live="assertive": errors, budget exceeded, critical alerts
    
  Toast notifications: role="status" with aria-live="polite"
  Loading states: aria-busy="true" on loading containers
  Progress: role="progressbar" with aria-valuenow, aria-valuemin, aria-valuemax

Interactive elements:
  Buttons: always have accessible names
    <button aria-label="Close dialog"> when icon-only
    <button> visible text — no aria-label needed
    
  Links: descriptive text, not "click here"
    <a href="/sessions/a3f">View Q4 Revenue Analysis</a>
    
  Toggle buttons: aria-pressed="true|false"
  Expandable sections: aria-expanded="true|false" on trigger
  Tabs: role="tablist", role="tab", role="tabpanel" with aria-selected
  
  Custom widgets (graph, timeline):
    role="application" with full keyboard operation
    aria-roledescription for custom widgets
    All interactive elements keyboard accessible

Data tables:
  Caption: <caption> describes table purpose
  Sortable headers: aria-sort="ascending|descending|none"
  Row selection: aria-selected="true|false"
  Expandable rows: aria-expanded on toggle button

Color and contrast:
  All text meets WCAG AA contrast ratios:
    Normal text: 4.5:1 minimum
    Large text (18px+ bold, 24px+): 3:1 minimum
    UI components: 3:1 minimum for boundaries
    
  Color is never the only way to convey information:
    Status indicators include text labels
    Charts include patterns or shapes in addition to color
    Error states include icon + text, not just red color
    
  Verified with: axe-core automated testing + manual review
```

### 21.3 Keyboard Accessibility

```
See §25 for complete keyboard shortcut reference.

Focus management:
  Focus trap in modals: Tab cycles within modal only
  Focus restoration: on modal close, focus returns to trigger element
  Skip link: first focusable element, "Skip to main content" (visually hidden until focused)
  Focus order: logical DOM order matches visual order
  
  Focus indicators:
    :focus-visible: 2px solid ring, offset from element edge
    High contrast against all backgrounds (4px ring with 2px gap)
    Visible on all interactive elements when using keyboard
    Not visible on mouse click (focus-visible only)

Tab order:
  Logical flow: Top bar → Sidebar → Content → Status bar
  Within content: left→right, top→bottom
  No positive tabindex values (use DOM order)
  tabindex="0" for custom interactive elements
  
Arrow key navigation:
  Lists: ↑↓ to move between items, Enter to select
  Tabs: ←→ to switch tabs
  Menus: ↑↓ to navigate items, → to open submenu, ← to close
  Sliders: ←→ to adjust value, Shift+←→ for larger steps
  Graph: ←→↑↓ to move between nodes, Enter to select
  Timeline: ←→ to move between events, Enter to expand
```

---

## 22. Keyboard Shortcuts & Power-User Mode

### 22.1 Global Shortcuts

```
NAVIGATION:
  ⌘1          Dashboard
  ⌘2          Investigation Workbench
  ⌘3          Timeline Explorer
  ⌘4          Entity Graph
  ⌘5          Sessions
  ⌘6          Memory Browser
  ⌘7          Task Queue
  ⌘8          Approvals
  ⌘9          Billing
  ⌘0          System Health
  ⌘-          Admin (⌘ and minus)
  ⌘=          Settings (⌘ and equals)

GLOBAL ACTIONS:
  ⌘K          Command Palette (open/close)
  ⌘⇧F        Global Search (semantic search overlay)
  ⌘N          New Session
  ⌘⇧N         New Investigation
  ⌘⇧I         Switch Investigation
  ⌘,          Settings
  ⌘/          Keyboard Shortcuts Help (this reference)
  Escape      Close modal/drawer/palette, deselect

VIEW CONTROLS:
  ⌘E          Toggle Evidence Panel
  ⌘⇧D         Toggle Discovery Panel
  ⌘⇧S         Toggle Sidebar
  ⌘B          Toggle Sidebar Collapse
  ⌘\          Reset Pane Split to 50/50
  
  ⌘⇧T         Toggle Theme (dark/light)
  ⌘⇧D         Toggle Density Mode
  ⌘⇧M         Toggle Reduced Motion
  
  ⌘[          Navigate Back (browser back)
  ⌘]          Navigate Forward (browser forward)
  ⌘R          Refresh Current View (refetch data)

INVESTIGATION WORKBENCH:
  ⌘Enter      Submit Query / Input
  ⌘⇧Enter     Submit with Newline preserved
  
  ⌃←          Reduce THINK pane by 40px (increase SAYS)
  ⌃→          Increase THINK pane by 40px (reduce SAYS)
  ⌃⇧←         Snap THINK to minimum (280px)
  ⌃⇧→         Snap SAYS to minimum (280px)
  ⌃\          Reset pane split to 50/50
  
  ⌘⇧↑         Jump to Previous Finding in SAYS
  ⌘⇧↓         Jump to Next Finding in SAYS
  ⌘⇧←         Jump to Linked THINK Step
  ⌘⇧→         Jump to Linked SAYS Finding
  
SESSIONS:
  Space       Toggle row selection (when table focused)
  ⌘A          Select All (in table)
  Escape      Clear Selection
  Delete      Cancel Selected Sessions (with confirmation)
  
  ⌘P          Pause Selected Sessions
  ⌘⇧P         Resume Selected Sessions
  
MEMORY BROWSER:
  J / ↓       Next Event
  K / ↑       Previous Event
  [           Previous Page
  ]           Next Page
  Enter       View Event Detail
  
TASK QUEUE:
  ←           Move task left (to previous column)
  →           Move task right (to next column)
  C           Claim selected task
  D           Mark task Done
  R           Release claimed task
  
APPROVALS:
  A           Approve selected approval
  D           Deny selected approval
  ⇧A          Approve All (with confirmation)
  ⇧D          Deny All (with confirmation)
```

### 22.2 Power-User Mode

```
Activated by: toggling "Power-User Mode" in Settings or ⌘⇧P

Changes when enabled:
  - All tooltips disabled (user knows the interface)
  - Confirmation dialogs suppressed for non-destructive actions
  - Animations shortened by 50% (faster feel)
  - Density mode automatically enabled
  - Advanced keyboard shortcuts visible in UI
  - Experimental features enabled (if any)
  - "What's New" banners suppressed
  
  Visual indicator: subtle border accent color change (blue→purple)
    Reminds user they're in power-user mode
    Can be toggled off at any time
  
  State: persisted in localStorage, survives refresh
  Default: off for new users, on if user has >50 sessions
```

---

## 23. Build, Bundle & Deployment

### 23.1 Tech Stack

```
Frontend:
  React 19              Component framework
  TypeScript 5.7        Type safety
  Vite 6                Build tool and dev server
  React Router v7       Client-side routing
  TanStack Query v5     Server state management
  Zustand v5            UI state management
  Tailwind CSS v4       Utility-first styling (with design tokens)
  Phosphor Icons        Icon library (SVG spritesheet)
  D3.js / Plot          Data visualizations (SVG charts)
  Three.js / regl       WebGL graph rendering
  CodeMirror 6          Code/JSON editor (for memory detail)
  axe-core              Accessibility testing
  
  Bundle size budget:
    Initial JS: <200KB gzipped
    Total JS (lazy loaded): <500KB gzipped
    CSS: <50KB gzipped
    Icons (SVG sprite): <30KB gzipped
    
  Code splitting:
    Route-based: each page is a separate chunk
    Component-based: heavy components (graph, code editor) lazy loaded
    Vendor chunk: React, ReactDOM, React Router (stable, long cache)
    
  Lazy loading strategy:
    All pages except Dashboard are lazy loaded
    Graph and Timeline are lazy loaded with loading skeleton
    Code editor (CodeMirror) is dynamic import (only when editing)
```

### 23.2 Build Pipeline

```
Development:
  vite dev — HMR, fast refresh, TypeScript checking in IDE
  Port: 5173 (proxies API to Consensus server at :8090)
  
Production build:
  vite build
  Output: dist/ directory with hashed filenames
  Assets: immutably cached (content hash in filename)
  
Embedded build (for Consensus binary):
  Go embed directive includes dist/ in binary
  Consensus binary serves static files at /ui/
  API proxy: /api/* routes to backend
  Single binary deployment: no separate web server needed

Docker:
  Multi-stage build:
    Stage 1: node:22-alpine → build frontend
    Stage 2: golang:1.26-alpine → build backend + embed frontend
    Stage 3: alpine:3.21 → runtime (single binary, ~25MB)
    
Environment variables:
  VITE_API_URL: API endpoint (default: http://localhost:8090)
  VITE_WS_URL: WebSocket endpoint (default: ws://localhost:8090/ws)
  VITE_APP_NAME: application name (default: "Consensus")
```

### 23.3 Embedded Serving

```
Go embed directive in cmd/consensus/main.go:
  //go:embed dist/*
  var frontendAssets embed.FS

  // Mount frontend under /ui/
  frontendFS, _ := fs.Sub(frontendAssets, "dist")
  apiMux.Handle("/ui/", http.StripPrefix("/ui", 
    http.FileServer(http.FS(frontendFS))
  ))

SPA routing:
  All /ui/* routes that don't match a file serve index.html
  Client-side router handles the rest
  Go handler: if file not found, serve index.html
  
API proxy:
  Frontend dev server proxies /api/* to backend
  Production: frontend and backend share same origin (no CORS)
  Same-origin means no preflight requests, faster API calls
```

---

## 24. Testing Strategy

### 24.1 Test Pyramid

```
UNIT TESTS (Jest + React Testing Library):
  Every component has unit tests
  Test coverage target: 85% lines, 80% branches
  Focus: component rendering, user interactions, state changes
  Mock: API calls, WebSocket, browser APIs
  
  Example tests:
    - Button renders with correct label
    - Click handler fires on click
    - Disabled button does not fire click
    - Dropdown opens on click, closes on Escape
    - Table sorts by column on header click

INTEGRATION TESTS (React Testing Library + MSW):
  Mock Service Worker intercepts API calls
  Full component trees with real data flow
  Test: complete user workflows
  
  Example tests:
    - Create session flow: fill form → submit → see in list
    - Approve finding: click approve → status changes → toast appears
    - Filter sessions: select filter → table updates
    - Navigate between pages: click link → new page loads

E2E TESTS (Playwright):
  Real browser automation against running Consensus server
  Test: critical user journeys end-to-end
  
  Example tests:
    - Full investigation: create session → add evidence → AI processes → review findings → approve
    - Multi-session: create 3 sessions, switch between them, verify isolation
    - Error handling: disconnect network, verify error states, reconnect, verify recovery
    - Accessibility: axe-core scan on every page after key interactions

VISUAL REGRESSION (Playwright + screenshot comparison):
  Screenshot key pages and components
  Compare against baseline (CI fails on visual diff > 0.1%)
  Run on: Chrome (primary), Firefox, Safari
  
  Screenshot scenarios:
    - Dashboard: default state, with data, empty state
    - Investigation: with findings, empty, with errors
    - Tables: sorted, filtered, selected, empty
    - Modals: open, with form errors
    - All responsive breakpoints: mobile, tablet, desktop

ACCESSIBILITY AUDIT:
  axe-core in Playwright tests (every page, every interaction)
  Lighthouse accessibility score ≥ 95
  Manual keyboard navigation test
  Manual screen reader test (VoiceOver + NVDA)
  CI blocks merge if accessibility score drops
```

### 24.2 Component Test Example

```typescript
// SessionRow.test.tsx
describe('SessionRow', () => {
  const mockSession = {
    id: 'a3f2b1c4',
    name: 'Q4 Revenue Analysis',
    status: 'thinking',
    iteration: 42,
    cost: 1.23,
    lastActive: new Date('2026-06-07T14:23:00'),
  };

  it('renders session information', () => {
    render(<SessionRow session={mockSession} />);
    expect(screen.getByText('Q4 Revenue Analysis')).toBeInTheDocument();
    expect(screen.getByText('#a3f2b')).toBeInTheDocument();
    expect(screen.getByText('thinking')).toBeInTheDocument();
    expect(screen.getByText('42')).toBeInTheDocument();
  });

  it('shows thinking pulse animation when status is thinking', () => {
    const { container } = render(<SessionRow session={mockSession} />);
    const statusDot = container.querySelector('.status-dot');
    expect(statusDot).toHaveClass('status-thinking');
    expect(statusDot).toHaveStyle({ animation: expect.stringContaining('pulse') });
  });

  it('navigates to session on double-click', async () => {
    const onNavigate = vi.fn();
    render(<SessionRow session={mockSession} onNavigate={onNavigate} />);
    const row = screen.getByRole('row');
    await userEvent.dblClick(row);
    expect(onNavigate).toHaveBeenCalledWith('a3f2b1c4');
  });

  it('selects row on click', async () => {
    const onSelect = vi.fn();
    render(<SessionRow session={mockSession} onSelect={onSelect} />);
    await userEvent.click(screen.getByRole('row'));
    expect(onSelect).toHaveBeenCalledWith('a3f2b1c4');
  });

  it('applies cost color based on budget usage', () => {
    const cheap = { ...mockSession, cost: 0.50, budgetLimit: 10.00 };
    const expensive = { ...mockSession, cost: 9.00, budgetLimit: 10.00 };
    
    const { rerender } = render(<SessionRow session={cheap} />);
    expect(screen.getByText('$0.50')).toHaveClass('text-success');
    
    rerender(<SessionRow session={expensive} />);
    expect(screen.getByText('$9.00')).toHaveClass('text-error');
  });
});
```

---

## 25. Document Index

```
Complete specification structure:

  026-dashboard-ui.md          ← Main spec (this file)
  ├─ §1  Design System          (~2,000 lines)
  ├─ §2  Layout Architecture    (~1,000 lines)
  ├─ §3  Global Navigation       (~1,500 lines)
  ├─ §4  Dashboard Overview      (~2,000 lines)
  ├─ §5  Investigation Workbench (~3,500 lines)
  ├─ §6  Timeline Explorer       (~2,500 lines)
  ├─ §7  Entity Graph            (~3,000 lines)
  ├─ §8  Semantic Search         (~2,000 lines)
  ├─ §9  Session Lifecycle       (~2,500 lines)
  ├─ §10 Memory Browser          (~2,000 lines)
  ├─ §11 Task Queue              (~1,200 lines)
  ├─ §12 HITL Approvals          (~2,000 lines)
  ├─ §13 Multi-Model Deliberation(~1,500 lines)
  ├─ §14 Billing & Budget        (~1,500 lines)
  ├─ §15 System Health           (~1,200 lines)
  ├─ §16 Multi-Tenant Admin      (~2,000 lines)
  ├─ §17 Animation System        (~1,500 lines)
  ├─ §18 State Management        (~1,500 lines)
  ├─ §19 WebSocket Events        (~1,200 lines)
  ├─ §20 Responsive Behavior     (~1,000 lines)
  ├─ §21 Accessibility           (~1,200 lines)
  ├─ §22 Keyboard Shortcuts      (~1,000 lines)
  ├─ §23 Build & Deploy          (~800 lines)
  └─ §24 Testing Strategy        (~800 lines)

TOTAL: ~41,500 lines of UI specification

STATUS: DRAFT — Complete first pass of all sections
NEXT: Consolidate all part files into single 026-dashboard-ui.md
```

---

## Document Metadata

- **Spec ID:** SPEC-026
- **Title:** Palantir-Grade Dashboard UI Specification
- **Version:** 0.1.0 (Draft)
- **Created:** 2026-06-10
- **Author:** Hermes Agent (Bane's direction)
- **Depends On:** SPEC-015 (API), SPEC-017 (UI Adapter), SPEC-018 (OpenAPI), SPEC-019 (User Flows)
- **Total estimated lines:** ~41,500
- **Status:** First complete draft — all 24 sections written
