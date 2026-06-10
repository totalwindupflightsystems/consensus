# Agent Skills Specification Reference

## Core Requirements for Skill Compiler

### SKILL.md Structure
```
---
name: {skill-name}                    # REQUIRED: kebab-case, 1-64 chars
description: {description}             # REQUIRED: 1-1024 chars
license: {license}                    # OPTIONAL: MIT, Apache-2.0, etc.
compatibility: {agent}                # OPTIONAL: opencode, claude-code, etc.
metadata:                             # OPTIONAL: key-value pairs
  audience: {audience}
  category: {category}
  workflow: {workflow}
---
```

### Name Validation Rules
```regex
^[a-z0-9]+(-[a-z0-9]+)*$
```
- Must be lowercase alphanumeric
- Single hyphens allowed between words
- Cannot start or end with hyphen
- Cannot contain consecutive hyphens
- Must match directory name

### Directory Structure
```
skill-name/
├── SKILL.md          # REQUIRED
├── scripts/          # OPTIONAL
│   └── {script}.sh   # Must be executable
├── references/       # OPTIONAL
│   └── {topic}.md    # Supporting documentation
└── assets/           # OPTIONAL
    └── {files}       # Static files, templates
```

### Script Requirements
1. **Shebang**: `#!/bin/bash`
2. **Fail-fast**: `set -e`
3. **Error messages**: `echo "Message" >&2`
4. **Clean output**: Machine-readable to stdout
5. **Cleanup**: `trap 'cleanup' EXIT`

### Skill Discovery Paths
- `.opencode/skills/{name}/SKILL.md`
- `~/.config/opencode/skills/{name}/SKILL.md`
- `.claude/skills/{name}/SKILL.md`
- `~/.claude/skills/{name}/SKILL.md`

### Compatibility Notes
- Skills follow [Agent Skills](https://agentskills.io) open standard
- Compatible with OpenCode, Claude Code, Cursor, and 37+ agents
- Install via: `npx skills add {owner}/{repo}`

### Best Practices
1. **Progressive Disclosure**: Keep SKILL.md under 500 lines, reference supporting files
2. **Clear Examples**: Include 2-3 usage examples with output
3. **Error Handling**: Every script must handle failures gracefully
4. **Security**: No credentials, API keys, or sensitive data in examples
5. **Validation**: Include validation steps in skill instructions

### Quality Gates for Skill Compiler
#### Pre-compilation
- [ ] Name format validated
- [ ] Description length checked
- [ ] Requirements understood
- [ ] Scope boundaries defined

#### During compilation
- [ ] Frontmatter syntax correct
- [ ] Scripts follow best practices
- [ ] Examples realistic and working
- [ ] Error handling comprehensive

#### Post-compilation
- [ ] Complete bundle validates
- [ ] Edge cases addressed
- [ ] Security constraints satisfied
- [ ] Compatibility verified

### Common Skill Patterns

#### 1. Analysis Skills
- Review code, configuration, or data
- Provide recommendations
- Generate reports

#### 2. Automation Skills
- Execute scripts or commands
- Automate workflows
- Integrate with tools

#### 3. Generation Skills
- Create content, code, or documentation
- Generate templates
- Produce outputs from patterns

#### 4. Validation Skills
- Check compliance with standards
- Validate configurations
- Verify requirements

### Error Categories for Skill Compilation

#### Input Validation Errors
- Missing required fields (name, description)
- Invalid name format
- Description too long/short

#### Implementation Errors
- Script syntax errors
- Missing dependencies
- Security vulnerabilities

#### Output Validation Errors
- Invalid SKILL.md structure
- Missing components
- Compatibility issues

### Recovery Strategies

#### For Input Errors
1. Ask up to 7 precise questions
2. Use conservative defaults with warnings
3. Abort if critical security info missing

#### For Implementation Errors
1. Fix syntax automatically when possible
2. Provide corrected templates
3. Escalate to manual intervention

#### For Output Errors
1. Regenerate failed components
2. Provide partial output with error markers
3. Return validation report with fixes needed