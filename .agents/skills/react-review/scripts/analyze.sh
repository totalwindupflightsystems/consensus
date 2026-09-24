#!/bin/bash
set -e

# React Component Analysis Script
# This script can be called by agents to analyze React components

echo "React Component Analysis" >&2
echo "======================" >&2

# Check if file was provided
if [ $# -eq 0 ]; then
    echo "Usage: $0 <react-component-file> [additional-files...]" >&2
    exit 1
fi

for file in "$@"; do
    if [ ! -f "$file" ]; then
        echo "Warning: File not found: $file" >&2
        continue
    fi
    
    echo "Analyzing: $file" >&2
    
    # Basic React component analysis
    echo "=== Analysis for $file ==="
    
    # Check for React import
    if grep -q "from 'react'" "$file" || grep -q 'from "react"' "$file" || grep -q "import React" "$file"; then
        echo "✓ React import detected"
    else
        echo "✗ No React import found (might be JSX file or missing import)"
    fi
    
    # Check for default export (common for components)
    if grep -q "export default" "$file"; then
        echo "✓ Default export found"
    fi
    
    # Check for TypeScript
    if [[ "$file" == *.tsx ]] || [[ "$file" == *.ts ]]; then
        echo "✓ TypeScript component"
    fi
    
    # Count components in file
    component_count=$(grep -c "function [A-Z]" "$file" || true)
    component_count=$((component_count + $(grep -c "const [A-Z]" "$file" || true)))
    component_count=$((component_count + $(grep -c "class [A-Z]" "$file" || true)))
    
    if [ "$component_count" -gt 0 ]; then
        echo "✓ Found $component_count component(s)"
    fi
    
    echo ""
done

echo "Analysis complete" >&2
