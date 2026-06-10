#!/bin/bash
set -e

# Skill Bundle Validator
# Part of hypercognitive-skill-compiler - validates complete skill bundles

echo "Skill Bundle Validator" >&2
echo "=====================" >&2

# Check if skill directory was provided
if [ $# -eq 0 ]; then
    echo "Usage: $0 <skill-directory>" >&2
    echo "Example: $0 skills/git-release" >&2
    exit 1
fi

skill_dir="$1"
skill_name=$(basename "$skill_dir")

echo "Validating skill bundle: $skill_name" >&2
echo "Directory: $skill_dir" >&2

# Initialize validation report
validation_passed=true
issues=()

# Check 1: SKILL.md exists
if [ ! -f "$skill_dir/SKILL.md" ]; then
    issues+=("❌ SKILL.md not found")
    validation_passed=false
else
    echo "✅ SKILL.md found" >&2
    
    # Check frontmatter
    if ! head -20 "$skill_dir/SKILL.md" | grep -q "^---$"; then
        issues+=("❌ SKILL.md missing YAML frontmatter (---)")
        validation_passed=false
    else
        echo "✅ YAML frontmatter detected" >&2
        
        # Extract frontmatter
        frontmatter=$(awk '/^---$/ {if (++count == 2) exit; next} count == 1' "$skill_dir/SKILL.md" 2>/dev/null || true)
        
        # Check required fields
        if ! echo "$frontmatter" | grep -q "name:"; then
            issues+=("❌ SKILL.md missing 'name' field")
            validation_passed=false
        fi
        
        if ! echo "$frontmatter" | grep -q "description:"; then
            issues+=("❌ SKILL.md missing 'description' field")
            validation_passed=false
        fi
    fi
fi

# Check 2: Directory structure (optional directories)
echo "" >&2
echo "Checking directory structure..." >&2

if [ -d "$skill_dir/scripts" ]; then
    echo "✅ scripts/ directory found" >&2
    script_count=$(find "$skill_dir/scripts" -name "*.sh" -type f | wc -l)
    echo "   Found $script_count .sh script(s)" >&2
    
    # Check script executability
    for script in "$skill_dir/scripts"/*.sh; do
        if [ -f "$script" ]; then
            if [ ! -x "$script" ]; then
                issues+=("⚠️  Script not executable: $(basename "$script")")
            fi
            # Check shebang
            if ! head -1 "$script" | grep -q "^#!/bin/bash"; then
                issues+=("⚠️  Script missing shebang: $(basename "$script")")
            fi
        fi
    done
else
    echo "ℹ️  scripts/ directory not found (optional)" >&2
fi

if [ -d "$skill_dir/references" ]; then
    echo "✅ references/ directory found" >&2
    ref_count=$(find "$skill_dir/references" -name "*.md" -type f | wc -l)
    echo "   Found $ref_count reference file(s)" >&2
else
    echo "ℹ️  references/ directory not found (optional)" >&2
fi

if [ -d "$skill_dir/assets" ]; then
    echo "✅ assets/ directory found" >&2
    asset_count=$(find "$skill_dir/assets" -type f | wc -l)
    echo "   Found $asset_count asset file(s)" >&2
else
    echo "ℹ️  assets/ directory not found (optional)" >&2
fi

# Check 3: Overall structure completeness
echo "" >&2
echo "## Validation Summary" >&2
echo "====================" >&2

if [ "$validation_passed" = true ]; then
    echo "✅ Skill bundle validation PASSED" >&2
    echo "" >&2
    echo "Skill '$skill_name' appears to be a valid Agent Skill bundle." >&2
    echo "Ready for installation via: npx skills add . --skill $skill_name" >&2
    
    # Output JSON for machine parsing
    echo '{"status":"success","skill":"'"$skill_name"'","valid":true,"issues":[]}'
else
    echo "❌ Skill bundle validation FAILED" >&2
    echo "" >&2
    echo "Issues found:" >&2
    for issue in "${issues[@]}"; do
        echo "  $issue" >&2
    done
    echo "" >&2
    echo "Required fixes:" >&2
    echo "  1. Ensure SKILL.md exists with proper YAML frontmatter" >&2
    echo "  2. Include 'name' and 'description' fields in frontmatter" >&2
    echo "  3. Make scripts executable (chmod +x)" >&2
    echo "  4. Add shebang to scripts (#!/bin/bash)" >&2
    
    # Output JSON for machine parsing
    issues_json=$(printf '%s\n' "${issues[@]}" | jq -R . | jq -s . 2>/dev/null || echo '[]')
    echo '{"status":"failed","skill":"'"$skill_name"'","valid":false,"issues":'"$issues_json"'}' >&2
    exit 1
fi