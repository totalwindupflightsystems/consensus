#!/bin/bash
set -e

# Skill Template Generator
# Part of skill-builder skill - generates skill templates

echo "Skill Template Generator" >&2
echo "======================" >&2

# Check arguments
if [ $# -lt 2 ]; then
    echo "Usage: $0 <skill-name> <description> [category]" >&2
    echo "Example: $0 my-skill 'Does amazing things' development" >&2
    exit 1
fi

skill_name="$1"
description="$2"
category="${3:-development}"

# Validate skill name
if [[ ! "$skill_name" =~ ^[a-z0-9]+(-[a-z0-9]+)*$ ]]; then
    echo "❌ Error: Skill name must be kebab-case" >&2
    echo "  Valid: lowercase alphanumeric with hyphens" >&2
    echo "  Invalid characters: uppercase, spaces, underscores, special chars" >&2
    exit 1
fi

# Validate description length
desc_length=${#description}
if [ $desc_length -lt 1 ] || [ $desc_length -gt 1024 ]; then
    echo "❌ Error: Description must be 1-1024 characters" >&2
    echo "  Current length: $desc_length" >&2
    exit 1
fi

# Generate skill template
echo "Generating skill template: $skill_name" >&2

# Create directory structure
mkdir -p "$skill_name"
mkdir -p "$skill_name/scripts"
mkdir -p "$skill_name/references"
mkdir -p "$skill_name/assets"

# Create SKILL.md
cat > "$skill_name/SKILL.md" << EOF
---
name: $skill_name
description: $description
license: MIT
compatibility: opencode
metadata:
  audience: developers
  category: $category
---

# $(echo "$skill_name" | sed 's/-/ /g' | awk '{for(i=1;i<=NF;i++) $i=toupper(substr($i,1,1)) substr($i,2)} 1')

$description

## When to use me

Use this skill when you need to...

## What I do

- First capability
- Second capability  
- Third capability

## Examples

\`\`\`bash
# Example usage
\`\`\`

## Output format

\`\`\`
Example output format
\`\`\`

## Notes

- Important implementation details
- Any limitations or requirements
EOF

# Create example script
cat > "$skill_name/scripts/example.sh" << 'EOF'
#!/bin/bash
set -e

# Example script for skill
echo "Running $0"
echo "Skill: $(basename $(dirname $(dirname "$0")))"
echo "Arguments: $@"

# Add your script logic here
EOF

chmod +x "$skill_name/scripts/example.sh"

# Create reference documentation
cat > "$skill_name/references/README.md" << EOF
# $skill_name - Reference Documentation

Add detailed documentation, API references, or implementation notes here.

This file can be referenced from SKILL.md using links.
EOF

# Create validation report
cat > "$skill_name/VALIDATION.md" << EOF
# Validation Report: $skill_name

## Validation Results
- ✅ Name format validation passed
- ✅ Description length validation passed ($desc_length characters)
- ✅ Directory structure created
- ✅ SKILL.md template generated
- ✅ Script template generated
- ✅ Reference template generated

## Next Steps
1. Edit \`$skill_name/SKILL.md\` with detailed instructions
2. Implement scripts in \`$skill_name/scripts/\`
3. Add reference documentation
4. Test skill locally: \`npx skills add . --skill $skill_name --list\`
5. Create symlink: \`ln -s ../../$skill_name .opencode/skills/$skill_name\`

## Compliance Check
- [x] Name follows kebab-case format
- [x] Description 1-1024 characters
- [x] Frontmatter includes required fields
- [ ] Skill content implemented
- [ ] Scripts functional
- [ ] Examples provided
- [ ] Edge cases handled
EOF

echo "✅ Skill template created: $skill_name/" >&2
echo "  - SKILL.md (template)" >&2
echo "  - scripts/example.sh" >&2
echo "  - references/README.md" >&2
echo "  - VALIDATION.md (report)" >&2
echo "" >&2
echo "Next: Edit SKILL.md with detailed instructions and implement functionality" >&2