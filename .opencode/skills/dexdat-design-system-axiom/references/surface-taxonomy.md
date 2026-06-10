# Surface Taxonomy

axiom:trace work_item=dexdat-design-system-01 spec=specs/README.md plan=.memory-bank/work-items/dexdat-design-system-01/plan.md doc=.opencode/skills/dexdat-design-system-axiom/references/surface-taxonomy.md evidence=.memory-bank/work-items/dexdat-design-system-01/verification.md commit=

## Purpose

This file is the planning map for which surfaces the Dexdat design doctrine must
be able to cover.

## Surface Families

### Web Marketing
- landing pages
- brand storytelling pages
- campaign pages
- docs-like public pages

### Web App
- authenticated dashboards
- tables and dense data views
- forms and workflows
- settings and admin surfaces
- command/control interfaces

### Mobile App
- handheld-first navigation
- touch interaction constraints
- reduced viewport information density
- gesture and haptic expectations

### Tablet App
- split-pane and multi-column opportunities
- touch plus keyboard accessory support
- higher-density layout than mobile, lower than desktop

### Desktop App
- pointer-heavy interaction patterns
- larger canvases and multitasking assumptions
- keyboard-shortcut expectations

## Cross-Cutting Domains

Every major surface may need doctrine for:
- onboarding
- navigation
- data entry
- dense information display
- search and filtering
- notifications and alerts
- loading states
- empty states
- failure recovery states
- accessibility accommodations
- motion and transitions

## Project-Specific Mapping Questions

When generating a new design spec, answer at least:

1. Which surface families are in scope?
2. Which surfaces share the same token system?
3. Which rules are inherited unchanged?
4. Which rules diverge by surface and why?
5. Which constraints are hardware/platform-specific?
