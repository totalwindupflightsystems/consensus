#!/bin/bash
set -e

# Skill Requirements Analyzer
# Part of skill-builder skill - analyzes skill requirements

echo "Skill Requirements Analyzer" >&2
echo "==========================" >&2

# Check if requirements file was provided
if [ $# -eq 0 ]; then
    echo "Usage: $0 <requirements-file.json>" >&2
    echo "Or pipe JSON: echo '{\"name\":\"test\"}' | $0 -" >&2
    exit 1
fi

input="$1"

# Read JSON input
if [ "$input" = "-" ]; then
    json_input=$(cat)
else
    if [ ! -f "$input" ]; then
        echo "❌ Error: Requirements file not found: $input" >&2
        exit 1
    fi
    json_input=$(cat "$input")
fi

# Parse JSON using jq if available, otherwise basic parsing
if command -v jq >/dev/null 2>&1; then
    echo "Using jq for JSON parsing" >&2
    
    # Extract fields
    name=$(echo "$json_input" | jq -r '.skill_requirements.name // empty')
    description=$(echo "$json_input" | jq -r '.skill_requirements.description // empty')
    category=$(echo "$json_input" | jq -r '.skill_requirements.category // "development"')
    
    core_functions=$(echo "$json_input" | jq -r '.capabilities.core_functions // [] | join(", ")')
    scripts_needed=$(echo "$json_input" | jq -r '.capabilities.scripts_needed // [] | join(", ")')
    
    compatibility=$(echo "$json_input" | jq -r '.constraints.compatibility_requirements // [] | join(", ")')
    security_constraints=$(echo "$json_input" | jq -r '.constraints.security_constraints // [] | join(", ")')
    
else
    echo "⚠️  jq not available, using basic parsing" >&2
    
    # Basic JSON parsing (very simplified)
    name=$(echo "$json_input" | grep -o '"name"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | cut -d'"' -f4)
    description=$(echo "$json_input" | grep -o '"description"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | cut -d'"' -f4)
    category=$(echo "$json_input" | grep -o '"category"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | cut -d'"' -f4)
    
    if [ -z "$category" ]; then
        category="development"
    fi
fi

echo "" >&2
echo "## Requirements Analysis Report" >&2
echo "================================" >&2

# Check for critical information
critical_issues=0

if [ -z "$name" ]; then
    echo "❌ CRITICAL: Skill name missing" >&2
    critical_issues=$((critical_issues + 1))
else
    echo "✅ Name: $name" >&2
    
    # Validate name format
    if [[ ! "$name" =~ ^[a-z0-9]+(-[a-z0-9]+)*$ ]]; then
        echo "❌ Name format invalid: must be kebab-case" >&2
        echo "   Suggested: $(echo "$name" | tr '[:upper:]' '[:lower:]' | tr -s ' ' | tr ' ' '-' | sed 's/[^a-z0-9-]//g')" >&2
    else
        echo "✅ Name format valid (kebab-case)" >&2
    fi
fi

if [ -z "$description" ]; then
    echo "❌ CRITICAL: Skill description missing" >&2
    critical_issues=$((critical_issues + 1))
else
    desc_length=${#description}
    echo "✅ Description: $description" >&2
    echo "   Length: $desc_length characters" >&2
    
    if [ $desc_length -lt 1 ] || [ $desc_length -gt 1024 ]; then
        echo "❌ Description length invalid: must be 1-1024 characters" >&2
    else
        echo "✅ Description length valid" >&2
    fi
fi

echo "" >&2
echo "## Capabilities Analysis" >&2
echo "========================" >&2

if [ -n "$core_functions" ] && [ "$core_functions" != "null" ]; then
    echo "Core functions:" >&2
    echo "$core_functions" | tr ',' '\n' | sed 's/^/  • /' >&2
else
    echo "⚠️ No core functions specified" >&2
fi

if [ -n "$scripts_needed" ] && [ "$scripts_needed" != "null" ] && [ "$scripts_needed" != "" ]; then
    echo "" >&2
    echo "Scripts needed:" >&2
    echo "$scripts_needed" | tr ',' '\n' | sed 's/^/  • /' >&2
else
    echo "" >&2
    echo "✅ No scripts specified (optional)" >&2
fi

echo "" >&2
echo "## Constraints Analysis" >&2
echo "========================" >&2

if [ -n "$compatibility" ] && [ "$compatibility" != "null" ] && [ "$compatibility" != "" ]; then
    echo "Compatibility requirements:" >&2
    echo "$compatibility" | tr ',' '\n' | sed 's/^/  • /' >&2
else
    echo "⚠️ No compatibility requirements specified" >&2
fi

if [ -n "$security_constraints" ] && [ "$security_constraints" != "null" ] && [ "$security_constraints" != "" ]; then
    echo "" >&2
    echo "Security constraints:" >&2
    echo "$security_constraints" | tr ',' '\n' | sed 's/^/  • /' >&2
else
    echo "" >&2
    echo "⚠️ No security constraints specified" >&2
fi

echo "" >&2
echo "## Assessment Summary" >&2
echo "=====================" >&2

if [ $critical_issues -gt 0 ]; then
    echo "❌ $critical_issues CRITICAL issues found" >&2
    echo "   Skill compilation cannot proceed" >&2
    echo "" >&2
    echo "Required fixes:" >&2
    [ -z "$name" ] && echo "  • Provide skill name" >&2
    [ -z "$description" ] && echo "  • Provide skill description" >&2
    exit 1
else
    echo "✅ All critical requirements present" >&2
    echo "✅ Ready for skill compilation" >&2
    
    # Output JSON for machine parsing
    echo '{"status":"ready","skill_name":"'"$name"'","description_length":'"${#description}"',"category":"'"$category"'","critical_issues":0}' >&2
fi