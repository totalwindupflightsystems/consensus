---
name: migration-guide-generator-axiom
description: >
  Version-to-version upgrade guides, breaking change detection and documentation, deprecation
  path planning, rollback procedure documentation, and automated migration script generation.
  Load this skill when preparing version upgrades, documenting breaking changes, or planning
  deprecation paths.
license: MIT
compatibility: opencode
metadata:
  version: "1.0"
  created: "2026-02-27"
  primary_spec: specs/00-PRD.md
  secondary_specs:
    - specs/README.md
tags:
  vertical: [coding, writing]
  category: development
  core: false
---

# Migration Guide Generator Skill (Portable)

> **"Never publish a migration guide without a tested rollback."**
>
> **"Every breaking change needs a migration path. Every migration path needs a rollback."**

This skill provides portable, production-grade guidance for creating version-to-version
migration guides, detecting breaking changes, planning deprecation paths, and documenting
rollback procedures. It ensures that every upgrade path is safe, tested, and reversible.

---

## Activation

Load this skill when:
- Preparing a major or minor version release with breaking changes
- Documenting how to upgrade between Axiom versions
- Planning a deprecation path for an API, config field, or data format
- Writing rollback procedures for a migration
- Detecting breaking changes between two versions of a spec or API
- Generating automated migration scripts for config or data format changes
- Reviewing a PR that introduces breaking changes

---

## Non-Negotiables

1. **Never publish a migration guide without a tested rollback.** Every migration step must
   have a documented, tested rollback procedure. "Restore from backup" is not a rollback plan.

2. **Never remove a deprecation warning before the documented timeline.** If you said
   "deprecated in v2.0, removed in v3.0," you must honor that timeline.

3. **Every breaking change must be documented.** No silent breaking changes. If it breaks
   existing behavior, it goes in the migration guide with a clear "before/after" example.

4. **Semantic versioning alignment is mandatory.** Major = breaking, minor = additive,
   patch = fix. No breaking changes in minor or patch releases.

5. **Migration guides must be testable.** Every step in the guide must be verifiable.
   Include verification commands for each step.

---

## Migration Guide Template

### File Location

```
docs/migrations/v<FROM>-to-v<TO>.md
```

Or for Axiom-specific migrations:

```
docs/migrations/axiom-v<FROM>-to-v<TO>.md
```

### Template

```markdown
# Migration Guide: v<FROM> to v<TO>

## Overview

**From**: v<FROM>
**To**: v<TO>
**Release Date**: <ISO 8601>
**Migration Difficulty**: Easy | Medium | Hard
**Estimated Time**: <duration>
**Rollback Available**: Yes | Partial | No

## Prerequisites

- [ ] Current version is v<FROM> (verify: `<command to check version>`)
- [ ] Backup completed (verify: `<command to verify backup>`)
- [ ] All tests pass on current version (verify: `<test command>`)
- [ ] Read this entire guide before starting

## Breaking Changes

### 1. <Breaking Change Title>

**What changed**: <Clear description of the change>
**Why**: <Rationale for the change>
**Impact**: <Who/what is affected>

**Before (v<FROM>)**:
```<language>
<old code/config/behavior>
```

**After (v<TO>)**:
```<language>
<new code/config/behavior>
```

**Migration steps**:
1. <Step 1>
2. <Step 2>

**Verification**: `<command to verify this change was applied correctly>`

### 2. <Next Breaking Change>
...

## Step-by-Step Migration

### Step 1: <Title>

```bash
<commands>
```

**Verification**:
```bash
<verification command>
# Expected output: <expected>
```

### Step 2: <Title>
...

### Step N: Final Verification

```bash
<full test suite command>
# All tests must pass
```

## Rollback Procedure

### Full Rollback (revert entire migration)

```bash
<rollback commands>
```

**Verification after rollback**:
```bash
<verification command>
# Expected: system is back to v<FROM> behavior
```

### Partial Rollback (revert specific changes)

#### Rollback Change 1: <Title>
```bash
<rollback commands for this specific change>
```

## Deprecation Notices

| Item | Deprecated In | Removal Target | Replacement |
|------|--------------|----------------|-------------|
| <item> | v<FROM> | v<FUTURE> | <replacement> |

## Known Issues

- <Any known issues with this migration>

## Trace

axiom:trace work_item=<ID> spec=<spec-ref> doc=docs/migrations/v<FROM>-to-v<TO>.md
```

---

## Breaking Change Detection

### What Constitutes a Breaking Change

| Category | Breaking | Non-Breaking |
|----------|----------|-------------|
| **API** | Remove endpoint, change required field, narrow response | Add endpoint, add optional field, widen response |
| **CLI** | Remove flag, change flag behavior, change exit codes | Add flag, add subcommand |
| **Config** | Remove field, change field type, change default behavior | Add optional field, add section |
| **Data Model** | Remove column, change type, change constraint | Add nullable column, add index |
| **Memory Bank** | Change required file format, rename required files | Add optional files, add sections |
| **Spec** | Remove requirement, change invariant | Add requirement, add section |

### Detection Methods

#### API Breaking Changes

```bash
# OpenAPI diff for breaking changes
oasdiff breaking openapi-v1.json openapi-v2.json

# Or with openapi-diff
openapi-diff openapi-v1.json openapi-v2.json --fail-on-incompatible
```

#### CLI Breaking Changes

```bash
# Compare help output between versions
diff <(v1/axiom run --help) <(v2/axiom run --help)

# Check exit codes
v1/axiom run --invalid-flag 2>/dev/null; echo "v1 exit: $?"
v2/axiom run --invalid-flag 2>/dev/null; echo "v2 exit: $?"
```

#### Config Schema Breaking Changes

```bash
# Compare config schemas
diff v1/.axiom/axiom.config.yaml.schema v2/.axiom/axiom.config.yaml.schema

# Validate old config against new schema
python3 -c "
import yaml, jsonschema
config = yaml.safe_load(open('v1-config.yaml'))
schema = yaml.safe_load(open('v2-schema.yaml'))
try:
    jsonschema.validate(config, schema)
    print('COMPATIBLE: Old config works with new schema')
except jsonschema.ValidationError as e:
    print(f'BREAKING: {e.message}')
"
```

#### Memory Bank Format Changes

```bash
# Check for required file renames
diff <(ls v1/.memory-bank/) <(ls v2/.memory-bank/)

# Validate old memory bank against new expectations
python3 -c "
import pathlib
required_v2 = ['_index.md', '_prompt.md']
mb = pathlib.Path('.memory-bank')
missing = [f for f in required_v2 if not (mb / f).exists()]
if missing:
    print(f'BREAKING: Missing required files: {missing}')
else:
    print('COMPATIBLE')
"
```

---

## Deprecation Path Planning

### Deprecation Lifecycle

```
Active --> Deprecated (with warning) --> Removed
  |            |                           |
  |            +-- minimum 2 minor         +-- only in major version
  |                versions or 3 months
  |
  +-- (no deprecation needed for internal-only changes)
```

### Deprecation Rules

1. **Announce**: Mark as deprecated in code, docs, and changelog
2. **Warn**: Emit runtime warnings when deprecated feature is used
3. **Document**: Provide replacement and migration path
4. **Timeline**: Maintain deprecated feature for at least 2 minor versions or 3 months
5. **Remove**: Only remove in a major version release after the timeline expires

### Deprecation Warning Template

```python
# In code
import warnings

def old_function():
    warnings.warn(
        "old_function() is deprecated since v2.1 and will be removed in v3.0. "
        "Use new_function() instead. "
        "Migration guide: docs/migrations/v2.0-to-v3.0.md#old-function-removal",
        DeprecationWarning,
        stacklevel=2,
    )
    return new_function()
```

```yaml
# In config (deprecated field with compatibility)
# .axiom/axiom.config.yaml
api:
  compat_alias_until: "2026-06-01T00:00:00Z"  # Remove aliases after this date
```

### Deprecation Registry

Maintain a deprecation registry in the migration docs:

```markdown
# Deprecation Registry

| Item | Type | Deprecated In | Warning Added | Removal Target | Replacement | Migration Guide |
|------|------|--------------|---------------|----------------|-------------|-----------------|
| `status` field alias | API response | v1.2 | v1.2 | v2.0 | `run_status` | [v1-to-v2](v1-to-v2.md) |
| `--in-process` flag | CLI | v1.5 | v1.5 | v2.0 | `--local` | [v1-to-v2](v1-to-v2.md) |
```

---

## Rollback Procedure Documentation

### Rollback Requirements

Every migration step MUST have a rollback that:

1. **Is specific**: Not "restore from backup" but exact commands to reverse the change
2. **Is tested**: The rollback procedure has been executed at least once
3. **Preserves data**: No data loss during rollback (or data loss is explicitly documented)
4. **Is ordered**: Rollback steps are in reverse order of migration steps
5. **Has verification**: Each rollback step has a verification command

### Rollback Template

```markdown
## Rollback: <Migration Step Title>

**Reverses**: Step N of the migration guide
**Data loss**: None | <description of what is lost>
**Estimated time**: <duration>

### Steps

1. <Rollback step 1>
   ```bash
   <command>
   ```
   Verify: `<verification command>`

2. <Rollback step 2>
   ...

### Post-Rollback Verification

```bash
<full verification command>
# Expected: system behaves as v<FROM>
```
```

---

## Axiom-Specific Migrations

### Config Schema Changes

```bash
# Generate migration script for config changes
python3 << 'EOF'
import yaml

def migrate_config(old_config_path, new_config_path):
    """Migrate .axiom/axiom.config.yaml from v1 to v2 format."""
    with open(old_config_path) as f:
        config = yaml.safe_load(f)

    # Example: rename field
    if "old_field" in config:
        config["new_field"] = config.pop("old_field")

    # Example: restructure section
    if "flat_key" in config:
        config.setdefault("nested", {})["key"] = config.pop("flat_key")

    with open(new_config_path, "w") as f:
        yaml.dump(config, f, default_flow_style=False)

    print(f"Migrated {old_config_path} -> {new_config_path}")

migrate_config(".axiom/axiom.config.yaml", ".axiom/axiom.config.yaml.new")
EOF
```

### Memory Bank Format Changes

```bash
# Migrate memory bank structure
python3 << 'EOF'
import pathlib, shutil

mb = pathlib.Path(".memory-bank")

# Example: rename required file
old = mb / "old-name.md"
new = mb / "new-name.md"
if old.exists() and not new.exists():
    shutil.move(str(old), str(new))
    # Leave redirect stub
    old.write_text(f"# Moved\n\nThis file has been moved to [{new.name}]({new.name}).\n")
    print(f"Migrated {old} -> {new}")

# Example: add required file
required = mb / "_prompt.md"
if not required.exists():
    required.write_text("# Memory Bank Rules\n\n(Generated by migration script)\n")
    print(f"Created {required}")
EOF
```

### Spec Format Changes

When spec format changes between versions:

1. Document the format change in the migration guide
2. Provide a script to update existing spec files
3. Validate all specs after migration

---

## Automated Migration Script Generation

### When to Generate Scripts

Generate automated migration scripts when:
- Config schema changes can be mechanically applied
- File renames or restructuring can be scripted
- Data format changes have a deterministic transformation
- The migration affects many files with the same pattern

### Script Requirements

Every migration script MUST:

1. Be idempotent (safe to run multiple times)
2. Create backups before modifying files
3. Validate inputs before making changes
4. Report what was changed
5. Exit with non-zero code on failure
6. Include a `--dry-run` flag

### Script Template

```python
#!/usr/bin/env python3
"""Migration script: v<FROM> to v<TO>."""
import argparse
import pathlib
import shutil
import sys

def main():
    parser = argparse.ArgumentParser(description="Migrate from v<FROM> to v<TO>")
    parser.add_argument("--dry-run", action="store_true", help="Show what would change")
    parser.add_argument("--backup-dir", default="/tmp/migration-backup",
                        help="Backup directory")
    args = parser.parse_args()

    changes = []

    # Step 1: Detect what needs to change
    # ...

    # Step 2: Create backup
    if not args.dry_run:
        backup = pathlib.Path(args.backup_dir)
        backup.mkdir(parents=True, exist_ok=True)
        # ... copy files to backup ...

    # Step 3: Apply changes
    for change in changes:
        if args.dry_run:
            print(f"[DRY RUN] Would {change['action']}: {change['path']}")
        else:
            # ... apply change ...
            print(f"[APPLIED] {change['action']}: {change['path']}")

    # Step 4: Verify
    if not args.dry_run:
        # ... run verification ...
        print("Migration complete. Run tests to verify.")

    return 0

if __name__ == "__main__":
    sys.exit(main())
```

---

## Verification

### How to Verify a Migration Guide

1. **Fresh environment test**: Apply the migration to a clean copy of the old version
2. **Rollback test**: After migration, execute the rollback and verify old behavior
3. **Idempotency test**: Run the migration twice; second run should be a no-op
4. **Partial failure test**: Interrupt the migration midway; verify rollback works
5. **Test suite**: Run the full test suite after migration

### Evidence Template

```markdown
## Migration Verification Evidence

**Migration**: v<FROM> to v<TO>
**Date**: <ISO 8601>
**Environment**: <description>

### Forward Migration
- [ ] All steps completed successfully
- [ ] Verification commands pass for each step
- [ ] Full test suite passes after migration

### Rollback Test
- [ ] Rollback steps completed successfully
- [ ] System behaves as v<FROM> after rollback
- [ ] Full test suite passes after rollback

### Idempotency Test
- [ ] Second migration run is a no-op
- [ ] No errors on second run

### Evidence Files
- Migration output: <path>
- Rollback output: <path>
- Test results: <path>
```

---

## Integration

### Works With

| Skill/Agent | Integration Point |
|-------------|-------------------|
| `api-contract-validator-axiom` | API breaking change detection feeds migration guides |
| `adr-manager-axiom` | Breaking change decisions recorded as ADRs |
| `enterprise-release-quality` | Migration guide is a release gate artifact |
| `@sre-ops-axiom` | Rollback procedures feed runbooks |
| `prd-spec-merge-axiom` | PRD changes may trigger migration guides |

---

## AI-Assisted Development Risks (2026)

| Risk | Mitigation |
|------|------------|
| AI generates migration guide without testing rollback | Non-negotiable: tested rollback required |
| AI removes deprecated features before timeline | Non-negotiable: honor deprecation timeline |
| AI introduces breaking changes in minor versions | Automated breaking change detection in CI |
| AI generates migration scripts that aren't idempotent | Template enforces idempotency |
| AI skips "before/after" examples for breaking changes | Template requires examples |
| AI claims migration is "simple" without evidence | Require verification evidence |

---

## Trace

`axiom:trace work_item=migration-guide-generator-axiom spec=specs/00-PRD.md plan= prompt=.opencode/skills/migration-guide-generator-axiom/SKILL.md evidence= doc= test= commit=`
