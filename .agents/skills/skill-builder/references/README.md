# Skill Builder - Reference Documentation

## Agent Skills Specification

### Core Requirements
1. **SKILL.md** with YAML frontmatter:
   - Required: `name` (kebab-case, 1-64 chars)
   - Required: `description` (1-1024 chars)
   - Optional: `license`, `compatibility`, `metadata`

2. **Directory Structure**:
   ```
   skill-name/
     SKILL.md
     scripts/          # Optional
     references/       # Optional
     assets/          # Optional
   ```

3. **Naming Conventions**:
   - Directory: kebab-case (e.g., `git-release`)
   - SKILL.md: Always uppercase
   - Scripts: kebab-case.sh (e.g., `deploy.sh`)

### Validation Rules

#### Name Validation
```regex
^[a-z0-9]+(-[a-z0-9]+)*$
```
- Must be lowercase alphanumeric with single hyphens
- Cannot start or end with hyphen
- Cannot contain consecutive hyphens
- Must match directory name

#### Description Validation
- Length: 1-1024 characters
- Should be specific enough for agent selection
- Include trigger phrases users might say

#### Frontmatter Validation
```yaml
---
name: skill-name
description: One sentence describing when to use this skill
license: MIT  # optional
compatibility: opencode  # optional
metadata:  # optional
  audience: developers
  category: development
---
```

### Skill Discovery Paths

OpenCode searches these locations:
- Project config: `.opencode/skills/<name>/SKILL.md`
- Global config: `~/.config/opencode/skills/<name>/SKILL.md`
- Project Claude-compatible: `.claude/skills/<name>/SKILL.md`
- Global Claude-compatible: `~/.claude/skills/<name>/SKILL.md`

### Script Best Practices

#### Required Elements
```bash
#!/bin/bash
set -e  # Fail fast

# Status messages to stderr
echo "Message" >&2

# Machine-readable output to stdout
echo '{"status": "success", "data": "..."}'

# Cleanup trap
trap 'rm -f "$tmp_file"' EXIT
```

#### Security Rules
1. Never include credentials, API keys, or secrets
2. Validate all inputs
3. Use minimal permissions
4. Clean up temporary files
5. Handle errors gracefully

### Compatibility Matrix

| Agent | Project Path | Global Path |
|-------|-------------|-------------|
| OpenCode | `.agents/skills/` | `~/.config/opencode/skills/` |
| Claude Code | `.claude/skills/` | `~/.claude/skills/` |
| Cursor | `.agents/skills/` | `~/.cursor/skills/` |
| Codex | `.agents/skills/` | `~/.codex/skills/` |

### Quality Gates

#### Pre-Implementation
- [ ] Requirements clearly defined
- [ ] Scope boundaries established
- [ ] Success artifact designed
- [ ] Component plan created

#### During Implementation
- [ ] Frontmatter valid
- [ ] Name format correct
- [ ] Description length OK
- [ ] Scripts executable and secure
- [ ] References complete
- [ ] Structure follows standards

#### Post-Implementation
- [ ] Complete bundle validation passed
- [ ] Examples included and working
- [ ] Edge cases handled
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

### Error Handling Patterns

#### Input Validation Errors
```bash
if [ -z "$required_var" ]; then
    echo "Error: required_var is missing" >&2
    exit 1
fi
```

#### Process Errors
```bash
if ! command_to_run; then
    echo "Error: command failed" >&2
    # Recovery logic
    fallback_command
fi
```

#### Output Validation Errors
```bash
if [ ! -f "$output_file" ]; then
    echo "Error: output file not created" >&2
    exit 1
fi
```

### Related Skills in This Repository

#### For Complex Skill Creation
- **hypercognitive-skill-compiler**: Full hypercognitive compiler preserving all original thinking modes, artifact registry, and compilation passes for exhaustive skill creation with rigorous quality gates.

#### For Basic Skill Creation  
- **skill-builder**: This skill - structured templates and validation tools for basic to intermediate skill creation.

### Resources
- [Agent Skills Specification](https://agentskills.io)
- [OpenCode Skills Docs](https://opencode.ai/docs/skills/)
- [Skills CLI](https://github.com/vercel-labs/skills)
- [Vercel Agent Skills](https://github.com/vercel-labs/agent-skills)
