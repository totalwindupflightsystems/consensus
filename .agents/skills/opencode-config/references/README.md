# OpenCode Configuration Reference

## Overview

OpenCode uses a hierarchical configuration system that merges settings from multiple sources. Configuration can be expressed in JSON/JSONC files (`opencode.json`) or Markdown files with YAML frontmatter (for modes, agents, commands). Understanding the precedence order is critical for effective configuration management.

## Configuration Formats

### JSON/JSONC
- **Primary format**: `opencode.json` or `opencode.jsonc`
- **Schema**: `https://opencode.ai/config.json` (runtime), `https://opencode.ai/tui.json` (TUI)
- **Features**: Comments supported in JSONC, variable substitution (`{env:VAR}`, `{file:path}`)
- **Use cases**: Main configuration, provider settings, permissions, tools, MCP servers

### Markdown with YAML Frontmatter
- **Location**: `.opencode/` or `~/.config/opencode/` subdirectories (`modes/`, `agents/`, `commands/`, etc.)
- **Structure**: YAML frontmatter block followed by Markdown content
- **Use cases**: Modes, agents, commands, skills where natural language instructions are beneficial

## Locations & Precedence Order

Configuration files are **merged** (not replaced), with later sources overriding earlier ones for conflicting keys:

1. **Remote config** (`.well-known/opencode`) - Organizational defaults
2. **Global config** (`~/.config/opencode/opencode.json`) - User preferences
3. **Custom config** (`OPENCODE_CONFIG` env var) - Custom overrides
4. **Project config** (`opencode.json` in project) - Project-specific settings
5. **`.opencode` directories** - Agents, commands, plugins, modes, skills
6. **Inline config** (`OPENCODE_CONFIG_CONTENT` env var) - Runtime overrides

### Important Notes
- `.opencode` and `~/.config/opencode` directories use **plural names** for subdirectories: `agents/`, `commands/`, `modes/`, `plugins/`, `skills/`, `tools/`, `themes/`
- Singular names (e.g., `agent/`) are also supported for backwards compatibility
- Project config has highest precedence among standard config files
- TUI-specific settings go in `tui.json` (or `tui` key in `opencode.json` - deprecated)

## Key Configuration Areas

### Models & Providers
```json
{
  "model": "anthropic/claude-sonnet-4-5",
  "small_model": "anthropic/claude-haiku-4-5",
  "provider": {
    "anthropic": {
      "options": {
        "timeout": 600000,
        "setCacheKey": true
      }
    }
  }
}
```

### Tools & Permissions
```json
{
  "tools": {
    "write": false,
    "bash": false
  },
  "permission": {
    "edit": "ask",
    "bash": "ask"
  }
}
```

### Modes (via Agent Configuration)
```json
{
  "agent": {
    "review": {
      "description": "Code review mode",
      "model": "anthropic/claude-sonnet-4-5",
      "prompt": "You are a code reviewer...",
      "tools": {
        "write": false,
        "edit": false,
        "bash": false
      }
    }
  }
}
```

### Markdown Mode Example
File: `~/.config/opencode/modes/review.md`
```markdown
---
temperature: 0.1
tools:
  write: false
  edit: false
  bash: false
---

You are in code review mode. Focus on code quality, potential bugs, and security considerations without making direct changes.
```

### Variable Substitution
- `{env:VARIABLE_NAME}` - Environment variable substitution
- `{file:path/to/file}` - File content substitution (relative to config file or absolute)

## Research Protocol

**CRITICAL**: OpenCode configuration evolves rapidly. Always verify details before providing implementation guidance.

### When to Research
1. **Before providing specific config examples** - Check official docs for current schema
2. **When encountering configuration issues** - Verify precedence and merging behavior
3. **When deprecation warnings appear** - Check for migration paths
4. **When new OpenCode version is released** - Review changelog for config changes

### Research Steps
1. **Search broadly**: "OpenCode configuration [specific topic]" using `searxng_searxng_web_search`
2. **Read authoritative sources**: Official docs (`opencode.ai/docs/config/`), GitHub issues, changelogs
3. **Cross-reference**: Confirm from 2+ independent sources when possible
4. **Check dates**: Prioritize recent information (last 3 months)
5. **Verify schema**: Use `$schema` URL for validation

### Search Query Examples
- "OpenCode config precedence order"
- "OpenCode JSON schema latest"
- "OpenCode markdown mode configuration"
- "OpenCode environment variable substitution"
- "OpenCode deprecated configuration options"

## Examples

### Project-Specific Configuration
```json
{
  "$schema": "https://opencode.ai/config.json",
  "model": "anthropic/claude-haiku-4-5",
  "permission": {
    "edit": "ask",
    "bash": "ask"
  },
  "instructions": ["./project-guidelines.md"],
  "watcher": {
    "ignore": ["node_modules/**", "dist/**"]
  }
}
```

### Global Mode Definition
```markdown
---
model: anthropic/claude-sonnet-4-5
temperature: 0.2
tools:
  write: true
  edit: true
  bash: false
---

You are in documentation mode. Focus on creating clear, comprehensive documentation with examples.
```

### Environment-Based Configuration
```json
{
  "model": "{env:OPENCODE_MODEL}",
  "provider": {
    "anthropic": {
      "options": {
        "apiKey": "{env:ANTHROPIC_API_KEY}"
      }
    }
  }
}
```

## Common Pitfalls

1. **Precedence confusion**: Project config overrides global, but `.opencode` directories have special precedence
2. **Deprecated options**: `mode` option is deprecated in favor of `agent`
3. **Path resolution**: Relative paths in config files are resolved relative to the config file location
4. **Merging behavior**: Non-conflicting settings from all configs are preserved
5. **TUI vs runtime**: TUI settings belong in `tui.json`, not `opencode.json`

## Verification Checklist

Before finalizing any configuration guidance:

- [ ] Check official documentation for schema updates
- [ ] Verify precedence order for the specific setting
- [ ] Confirm file locations and naming conventions
- [ ] Test variable substitution patterns
- [ ] Validate JSON with schema URL
- [ ] Review deprecation warnings in recent releases
- [ ] Cross-reference with community examples

## Validation Script

The skill includes `scripts/validate-config.sh` which provides basic validation of OpenCode configuration files:

```bash
# Run from skill directory or project root
./skills/opencode-config/scripts/validate-config.sh
```

**Capabilities:**
- Checks for existence of common config files (global, project, `.opencode` directories)
- Validates JSON syntax (if `jq` is installed)
- Outputs configuration precedence order
- Reports environment variables (`OPENCODE_CONFIG`, `OPENCODE_CONFIG_CONTENT`)
- Provides machine-readable JSON output for programmatic use

**Example output:**
```
Checking configuration files...
📄 File exists: /Users/lexykwaii/.config/opencode/opencode.json
✅ Valid JSON: /Users/lexykwaii/.config/opencode/opencode.json
📄 File exists: ./opencode.json
✅ Valid JSON: ./opencode.json
```

## Sources & References

- Official Documentation: https://opencode.ai/docs/config/
- Configuration Schema: https://opencode.ai/config.json
- TUI Schema: https://opencode.ai/tui.json
- GitHub Repository: https://github.com/anomalyco/opencode
- Changelog: https://github.com/anomalyco/opencode/releases

*Last researched: 2026-02-26*