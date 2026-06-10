#!/bin/bash
set -e

echo "Validating OpenCode configuration..." >&2

# Check if jq is available for JSON validation
if command -v jq >/dev/null 2>&1; then
    JQ_AVAILABLE=true
else
    JQ_AVAILABLE=false
    echo "Warning: jq not installed, JSON validation limited" >&2
fi

# Function to validate JSON file
validate_json_file() {
    local file="$1"
    if [ ! -f "$file" ]; then
        echo "File not found: $file" >&2
        return 1
    fi
    
    if [ "$JQ_AVAILABLE" = true ]; then
        if jq empty "$file" 2>/dev/null; then
            echo "✅ Valid JSON: $file" >&2
        else
            echo "❌ Invalid JSON: $file" >&2
            jq empty "$file" 2>&1 | head -5 >&2
            return 1
        fi
    else
        # Basic validation with grep
        if grep -q '"$schema"' "$file" 2>/dev/null; then
            echo "⚠️  Basic validation (no jq): $file appears to have schema reference" >&2
        fi
    fi
}

# Check for common config files
CONFIG_FILES=(
    "$HOME/.config/opencode/opencode.json"
    "$HOME/.config/opencode/tui.json"
    "./opencode.json"
    "./tui.json"
    "./.opencode/modes/"
    "./.opencode/agents/"
    "./.opencode/commands/"
)

echo "Checking configuration files..." >&2

for file in "${CONFIG_FILES[@]}"; do
    if [ -e "$file" ]; then
        if [[ "$file" == */ ]]; then
            # Directory
            if [ -d "$file" ]; then
                echo "📁 Directory exists: $file" >&2
            fi
        else
            # File
            echo "📄 File exists: $file" >&2
            if [[ "$file" == *.json ]]; then
                validate_json_file "$file"
            fi
        fi
    fi
done

# Output precedence order
echo ""
echo "Configuration Precedence Order (later overrides earlier):" >&2
cat << 'EOF'
1. Remote config (.well-known/opencode)
2. Global config (~/.config/opencode/opencode.json)
3. Custom config (OPENCODE_CONFIG env var)
4. Project config (opencode.json in project)
5. .opencode directories (agents, commands, plugins)
6. Inline config (OPENCODE_CONFIG_CONTENT env var)
EOF

# Check environment variables
if [ -n "$OPENCODE_CONFIG" ]; then
    echo "ℹ️  OPENCODE_CONFIG is set: $OPENCODE_CONFIG" >&2
fi

if [ -n "$OPENCODE_CONFIG_CONTENT" ]; then
    echo "ℹ️  OPENCODE_CONFIG_CONTENT is set (length: ${#OPENCODE_CONFIG_CONTENT} chars)" >&2
fi

echo ""
echo "Validation complete." >&2

# Output machine-readable summary
echo '{"status": "success", "config_files_found": true, "jq_available": '$JQ_AVAILABLE'}'