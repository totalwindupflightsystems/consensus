#!/bin/bash
set -e

# Skill Validation Helper Script
# Part of skill-builder skill - validates skill structure and compliance

echo "Skill Builder Validation Helper" >&2
echo "==============================" >&2

# Check if skill directory was provided
if [ $# -eq 0 ]; then
    echo "Usage: $0 <skill-directory>" >&2
    echo "Example: $0 skills/git-release" >&2
    exit 1
fi

skill_dir="$1"
skill_name=$(basename "$skill_dir")

echo "Validating skill: $skill_name" >&2

# Check directory exists
if [ ! -d "$skill_dir" ]; then
    echo "❌ Error: Skill directory not found: $skill_dir" >&2
    exit 1
fi

# Check SKILL.md exists
if [ ! -f "$skill_dir/SKILL.md" ]; then
    echo "❌ Error: SKILL.md not found in $skill_dir" >&2
    exit 1
fi

# Check YAML frontmatter
if ! head -20 "$skill_dir/SKILL.md" | grep -q "^---$"; then
    echo "❌ Error: SKILL.md missing YAML frontmatter (---)" >&2
    exit 1
fi

# Extract frontmatter between --- markers
frontmatter=$(awk '/^---$/ {if (++count == 2) exit; next} count == 1' "$skill_dir/SKILL.md")

# Check required fields
if ! echo "$frontmatter" | grep -q "name:"; then
    echo "❌ Error: SKILL.md missing 'name' field" >&2
    exit 1
fi

if ! echo "$frontmatter" | grep -q "description:"; then
    echo "❌ Error: SKILL.md missing 'description' field" >&2
    exit 1
fi

# Extract and validate name
name=$(echo "$frontmatter" | grep "name:" | cut -d: -f2 | tr -d ' ' | tr -d '"' | tr -d "'")
if [[ ! "$name" =~ ^[a-z0-9]+(-[a-z0-9]+)*$ ]]; then
    echo "❌ Error: Skill name '$name' must be lowercase alphanumeric with hyphens" >&2
    echo "  Valid format: kebab-case, e.g., git-release, react-review" >&2
    exit 1
fi

# Check name matches directory
if [ "$name" != "$skill_name" ]; then
    echo "⚠️  Warning: Skill name '$name' does not match directory name '$skill_name'" >&2
fi

# Check description length
desc=$(echo "$frontmatter" | grep "description:" | cut -d: -f2- | sed 's/^ *//')
desc_length=${#desc}
if [ $desc_length -lt 1 ] || [ $desc_length -gt 1024 ]; then
    echo "❌ Error: Description length $desc_length chars, must be 1-1024 characters" >&2
    exit 1
fi

# Check for scripts directory
if [ -d "$skill_dir/scripts" ]; then
    echo "Checking scripts directory..." >&2
    for script in "$skill_dir/scripts"/*.sh; do
        if [ -f "$script" ]; then
            if [ ! -x "$script" ]; then
                echo "⚠️  Warning: Script not executable: $(basename "$script")" >&2
            fi
            # Check for shebang
            if ! head -1 "$script" | grep -q "^#!/bin/bash"; then
                echo "⚠️  Warning: Script missing shebang or wrong shebang: $(basename "$script")" >&2
            fi
            # Check for set -e
            if ! head -5 "$script" | grep -q "set -e"; then
                echo "⚠️  Warning: Script missing 'set -e' for fail-fast behavior: $(basename "$script")" >&2
            fi
        fi
    done
fi

echo "✅ Skill '$skill_name' validation passed" >&2
echo "Name: $name" >&2
echo "Description length: $desc_length characters" >&2

# Output JSON for machine parsing
echo '{"status":"success","skill":"'"$skill_name"'","name":"'"$name"'","description_length":'"$desc_length"'}'
