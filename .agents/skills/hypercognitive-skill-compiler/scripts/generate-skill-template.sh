#!/bin/bash
set -e

# Skill Template Generator
# Example script showing proper structure for skills

echo "Skill Template Generator" >&2
echo "=======================" >&2

# Check arguments
if [ $# -lt 2 ]; then
    echo "Usage: $0 <skill-name> <description>" >&2
    echo "Example: $0 my-skill 'Does amazing things'" >&2
    exit 1
fi

skill_name="$1"
description="$2"
category="${3:-development}"

# Validate inputs
if [[ ! "$skill_name" =~ ^[a-z0-9]+(-[a-z0-9]+)*$ ]]; then
    echo "❌ Error: Skill name must be kebab-case" >&2
    echo "  Valid: lowercase alphanumeric with single hyphens" >&2
    echo "  Example: api-documentation, git-release, react-review" >&2
    exit 1
fi

desc_length=${#description}
if [ $desc_length -lt 1 ] || [ $desc_length -gt 1024 ]; then
    echo "❌ Error: Description must be 1-1024 characters" >&2
    echo "  Current length: $desc_length" >&2
    exit 1
fi

echo "Generating skill template: $skill_name" >&2
echo "Description: $description" >&2
echo "Category: $category" >&2

# Generate SKILL.md template
cat << EOF
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
# Example command or usage pattern
\`\`\`

## Output format

\`\`\`
Example output from the skill
\`\`\`

## Notes

- Implementation details
- Dependencies or requirements
- Limitations or constraints

## Scripts

This skill includes the following scripts:

### \`scripts/example.sh\`
\`\`\`bash
#!/bin/bash
set -e

echo "Example script for $skill_name"
echo "Add your implementation here"
\`\`\`
EOF

echo "" >&2
echo "✅ Template generated successfully" >&2
echo "Next steps:" >&2
echo "  1. Save this template to SKILL.md" >&2
echo "  2. Create scripts/ directory with executable scripts" >&2
echo "  3. Add reference documentation" >&2
echo "  4. Test with: npx skills add . --skill $skill_name --list" >&2