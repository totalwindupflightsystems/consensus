# Directory Index System - Reference Documentation

## Overview
The **index** skill implements a systematic approach to directory organization using index files (`_index.md`, `_index.yaml`, `_index.json`) and consistency prompts (`_prompt.md`). This system preserves context, guides file creation, and improves navigation through directory structures.

## Core Concepts

### 1. Index Files (`_index.*`)
Index files serve as directory-level documentation that:
- **Documents purpose** - Why the directory exists
- **Lists contents** - What files and subdirectories are present
- **Explains conventions** - How files in the directory should be structured
- **Provides context** - Relationships to other directories
- **Tracks changes** - When and why the directory evolves

### 2. Prompt Files (`_prompt.md`)
Prompt files establish consistency standards for:
- **File creation** - Guidelines for new files in the directory
- **Naming conventions** - How to name files and components
- **Content patterns** - Expected structure and organization
- **Quality standards** - Testing, documentation, and style requirements
- **Examples and templates** - Reference implementations

### 3. Context Preservation
The system addresses the "context switching" problem by:
- **Reducing cognitive load** - Providing quick directory overviews
- **Accelerating onboarding** - Helping new contributors understand structure
- **Maintaining standards** - Ensuring consistency over time and across contributors
- **Supporting AI/ML models** - Giving structured context for better file generation

## Implementation Guidelines

### When to Create Index Files
- **Project initialization** - Set up index files for major directories
- **Directory creation** - Create index when adding new significant directories
- **Major refactoring** - Update indexes when directory structure changes
- **Onboarding preparation** - Ensure indexes are current before new team members join
- **Documentation reviews** - Periodically review and update indexes

### What to Include in Index Files
- **Directory purpose** - 1-2 sentences explaining the directory's role
- **Contents list** - Files and subdirectories with brief descriptions (not detailed contents)
- **Conventions** - Important patterns or standards used in the directory
- **Relationships** - Links to related directories
- **Last updated** - Date and brief reason for last update

### What NOT to Include
- **Detailed file contents** - Indexes should not duplicate file documentation
- **Implementation details** - Focus on what, not how
- **Frequent updates** - Update when structure changes, not for every file change
- **Redundant information** - Don't repeat what's obvious from filenames
- **Deep nesting details** - Only immediate children, not grandchildren

### Prompt File Best Practices
- **Be specific but flexible** - Provide guidelines, not rigid rules
- **Include examples** - Show good patterns, not just describe them
- **Update with patterns** - Evolve prompts as best practices develop
- **Balance detail** - Enough guidance to be helpful, not so much it's ignored
- **Reference existing files** - Point to good examples in the codebase

## Integration with Development Workflow

### Version Control
```bash
# Typical .gitignore patterns for index files
# NOT recommended - index files should be version controlled
# They are documentation that should evolve with the codebase

# Recommended approach: commit index files with related changes
git add _index.md _prompt.md
git commit -m "docs: update directory index and prompts"
```

### CI/CD Integration
```yaml
# Example GitHub Actions workflow
- name: Validate directory indexes
  run: |
    npm run index:validate -- --directory .
    npm run index:check -- --directory .
```

### Development Tools Integration
- **IDE/Editor plugins** - Display index files when navigating directories
- **CLI tools** - Quick index viewing and updating
- **Documentation generators** - Include indexes in generated documentation
- **Project onboarding** - Use indexes as part of new contributor guides

## Use Cases

### For AI/ML Models
```bash
# Provide context to models
npm run index:context -- --directory src/components/ --format json
# Outputs structured context for model consumption
```

### For Development Teams
```bash
# Team onboarding checklist
npm run index:onboarding -- --new-contributor

# Directory standards audit  
npm run index:audit -- --directory . --report compliance
```

### For Project Maintenance
```bash
# Find directories needing updates
npm run index:stale -- --days 30

# Generate project structure documentation
npm run index:documentation -- --output PROJECT_STRUCTURE.md
```

## File Format Comparison

| Format | Pros | Cons | Best For |
|--------|------|------|----------|
| **Markdown** | Human-readable, easy to edit, supports rich formatting | Not easily machine-parsed, no strict schema | Most projects, teams preferring documentation |
| **YAML** | Machine-readable, structured, supports comments | Less human-friendly for non-technical users | Projects with automation needs, CI/CD integration |
| **JSON** | Universal compatibility, easily parsed by tools | No comments, verbose syntax | Tool integration, API contexts, machine consumption |

### Conversion Between Formats
```bash
# Convert existing index files
npm run index:convert -- --from markdown --to yaml --directory src/
npm run index:convert -- --from yaml --to json --recursive
```

## Advanced Features

### Dynamic Index Generation
```bash
# Generate index from directory analysis
npm run index:generate -- --analyze --template custom-template.md

# Update indexes based on file changes
npm run index:sync -- --watch --directory src/
```

### Integration with Other Tools
```bash
# Generate TypeScript types from index structure
npm run index:types -- --directory src/components/ --output types/directory.ts

# Create navigation for documentation sites
npm run index:nav -- --directory . --output docs/navigation.json
```

### Quality Gates
```bash
# Enforce index completeness
npm run index:enforce -- --require-index --require-prompt

# Check index freshness
npm run index:freshness -- --max-age-days 30
```

## Common Patterns

### Monorepo Structure
```
packages/
  _index.md           # Monorepo packages overview
  web-app/
    _index.md         # Web application package
    _prompt.md        # Web app file standards
    src/
      _index.md       # Source code structure
  api-server/
    _index.md         # API server package
    _prompt.md        # API server standards
```

### Microservices Architecture
```
services/
  _index.md           # Microservices overview
  auth-service/
    _index.md         # Authentication service
    _prompt.md        # Service development standards
  payment-service/
    _index.md         # Payment processing service
    _prompt.md        # Payment service patterns
```

### Component Library
```
components/
  _index.md           # Component library overview
  _prompt.md          # Component creation guidelines
  atoms/
    _index.md         # Atomic components
    _prompt.md        # Atom component standards
  molecules/
    _index.md         # Molecular components
    _prompt.md        # Molecule composition guidelines
```

## Limitations and Considerations

### Performance
- Large projects with many directories may have many index files
- Recursive operations can be slow on deep directory trees
- Consider caching or incremental updates for performance

### Maintenance Burden
- Index files require periodic updates
- Teams must buy into the system for it to be effective
- Balance between helpful documentation and maintenance overhead

### Tool Compatibility
- Some tools may ignore files starting with underscore
- IDE/editor support varies for custom index files
- Consider team tooling when choosing format

### Adoption Strategy
- Start with critical directories first
- Demonstrate value with onboarding improvements
- Integrate into existing workflows gradually
- Provide templates and automation to reduce friction

## Related Skills
- [git-release](../git-release/references/README.md) - Release management and changelogs
- [skill-builder](../skill-builder/references/README.md) - Skill creation and validation
- [testing-ecosystem](../testing-ecosystem/references/README.md) - Testing strategy and organization
- [test-planning](../test-planning/references/README.md) - Test strategy development

## References
- [Agent Skills Specification](https://agentskills.io/specification)
- [Documentation-Driven Development](https://en.wikipedia.org/wiki/Documentation-driven_development)
- [Project Structure Best Practices](https://docs.python-guide.org/writing/structure/)
- [Keep a Changelog](https://keepachangelog.com/) - Similar philosophy for project history
