## Duplicate Work Prevention

Before starting any new work item, feature, or investigation:

1. **Check active work**: Read `.memory-bank/work-items/_current.md` and scan `.memory-bank/TODO.md` for open checkboxes covering the same spec requirement.
2. **Search memory bank**: `grep -r "KEYWORD" .memory-bank/ --include="*.md" -l` to find related work items, plans, or findings.
3. **Check spec realized-by links**: Look for `realized-by` or `DONE` markers in the relevant spec file.
4. **Check git history**: `git log --oneline --since="30 days ago" | grep -i "KEYWORD"` for recent related commits.
5. **Check existing modules**: `find .axiom/plugin/src -name "*.ts" | xargs grep -l "FEATURE"` before creating new files.

**Decision rule**: If an active or completed work item already covers the requirement, route to it — do not create a duplicate. If in doubt, ask the user.

Load `.opencode/skills/duplicate-work-prevention-axiom/SKILL.md` for the full checklist and decision table.

axiom:trace work_item=DEX-205 spec=specs/02-Workflows.md plan=phase-97-2/task-97-2-1/step-97-2-1-1 jira_ref=DEX-205
