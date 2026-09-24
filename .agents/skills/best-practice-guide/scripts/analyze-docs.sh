#!/bin/bash
set -e

echo "Best Practice Guide: Documentation Analysis" >&2
echo "==========================================" >&2

# Check for project directory
PROJECT_DIR="${1:-.}"
OUTPUT_FILE="${2:-gaps.json}"
TIMESTAMP=$(date +%Y-%m-%dT%H:%M:%S)

echo "Project directory: $PROJECT_DIR" >&2
echo "Output file: $OUTPUT_FILE" >&2
echo "Timestamp: $TIMESTAMP" >&2
echo "" >&2

# Common documentation files to analyze
DOC_FILES=(
  "AGENTS.md"
  "CLAUDE.md"
  "README.md"
  "CONTRIBUTING.md"
  "DEVELOPMENT.md"
  "SPECS.md"
  "docs/README.md"
  ".github/README.md"
)

# Technology indicators to look for
TECH_INDICATORS=(
  "react" "vue" "angular" "svelte"
  "node" "python" "java" "go" "rust"
  "docker" "kubernetes" "terraform"
  "aws" "azure" "gcp" "cloud"
  "postgresql" "mysql" "mongodb" "redis"
  "graphql" "rest" "api"
  "typescript" "javascript"
  "jest" "cypress" "pytest" "mocha"
  "webpack" "vite" "esbuild"
  "tailwind" "bootstrap" "css"
)

# Common gap categories
GAP_CATEGORIES=(
  "security"
  "performance"
  "testing"
  "deployment"
  "monitoring"
  "documentation"
  "architecture"
  "scalability"
  "maintenance"
  "onboarding"
)

echo "Scanning documentation files..." >&2

# Check which documentation files exist
EXISTING_DOCS=()
for doc in "${DOC_FILES[@]}"; do
  if [ -f "$PROJECT_DIR/$doc" ]; then
    EXISTING_DOCS+=("$doc")
    echo "✅ Found: $doc" >&2
  fi
done

echo "" >&2
echo "Analyzing technology stack..." >&2

# Check for package.json, pyproject.toml, etc. to detect technologies
TECHNOLOGIES=()
if [ -f "$PROJECT_DIR/package.json" ]; then
  echo "📦 Found package.json - Node.js project detected" >&2
  TECHNOLOGIES+=("nodejs" "javascript/typescript")
  
  # Quick scan for common dependencies
  if grep -q "react" "$PROJECT_DIR/package.json" 2>/dev/null; then
    TECHNOLOGIES+=("react")
  fi
  if grep -q "vue" "$PROJECT_DIR/package.json" 2>/dev/null; then
    TECHNOLOGIES+=("vue")
  fi
  if grep -q "express" "$PROJECT_DIR/package.json" 2>/dev/null; then
    TECHNOLOGIES+=("express")
  fi
  if grep -q "jest" "$PROJECT_DIR/package.json" 2>/dev/null; then
    TECHNOLOGIES+=("jest")
  fi
fi

if [ -f "$PROJECT_DIR/pyproject.toml" ]; then
  echo "🐍 Found pyproject.toml - Python project detected" >&2
  TECHNOLOGIES+=("python")
fi

if [ -f "$PROJECT_DIR/Cargo.toml" ]; then
  echo "🦀 Found Cargo.toml - Rust project detected" >&2
  TECHNOLOGIES+=("rust")
fi

if [ -f "$PROJECT_DIR/go.mod" ]; then
  echo "🐹 Found go.mod - Go project detected" >&2
  TECHNOLOGIES+=("go")
fi

# Check for Docker
if [ -f "$PROJECT_DIR/Dockerfile" ] || [ -f "$PROJECT_DIR/docker-compose.yml" ]; then
  echo "🐳 Docker detected" >&2
  TECHNOLOGIES+=("docker")
fi

# Check for cloud configuration
if [ -f "$PROJECT_DIR/terraform.tf" ] || [ -f "$PROJECT_DIR/.tf" ]; then
  echo "🏗️ Terraform detected" >&2
  TECHNOLOGIES+=("terraform")
fi

echo "" >&2
echo "Identifying potential gaps..." >&2

# Generate gap analysis based on technologies and documentation
POTENTIAL_GAPS=()

# Security gaps
if [[ " ${TECHNOLOGIES[@]} " =~ " react " ]] || [[ " ${TECHNOLOGIES[@]} " =~ " nodejs " ]]; then
  POTENTIAL_GAPS+=("web-application-security")
fi

if [[ " ${TECHNOLOGIES[@]} " =~ " docker " ]]; then
  POTENTIAL_GAPS+=("container-security")
fi

if [[ " ${TECHNOLOGIES[@]} " =~ " python " ]] || [[ " ${TECHNOLOGIES[@]} " =~ " nodejs " ]]; then
  POTENTIAL_GAPS+=("dependency-security")
fi

# Performance gaps
if [[ " ${TECHNOLOGIES[@]} " =~ " react " ]]; then
  POTENTIAL_GAPS+=("react-performance")
fi

if [[ " ${TECHNOLOGIES[@]} " =~ " nodejs " ]]; then
  POTENTIAL_GAPS+=("nodejs-performance")
fi

# Testing gaps
if [[ " ${TECHNOLOGIES[@]} " =~ " jest " ]]; then
  POTENTIAL_GAPS+=("jest-testing-patterns")
fi

# Documentation gaps
if [ ${#EXISTING_DOCS[@]} -eq 0 ]; then
  POTENTIAL_GAPS+=("basic-documentation")
elif [ ${#EXISTING_DOCS[@]} -lt 3 ]; then
  POTENTIAL_GAPS+=("comprehensive-documentation")
fi

echo "" >&2
echo "Generating gap analysis report..." >&2

# Create JSON output
cat << EOF > "$OUTPUT_FILE"
{
  "analysis": {
    "project_directory": "$PROJECT_DIR",
    "timestamp": "$TIMESTAMP",
    "documents_found": ${#EXISTING_DOCS[@]},
    "technologies_detected": ${#TECHNOLOGIES[@]}
  },
  "documents": [
EOF

# Add document list
for ((i=0; i<${#EXISTING_DOCS[@]}; i++)); do
  if [ $i -gt 0 ]; then
    echo "    ," >> "$OUTPUT_FILE"
  fi
  echo "    \"${EXISTING_DOCS[$i]}\"" >> "$OUTPUT_FILE"
done

cat << EOF >> "$OUTPUT_FILE"
  ],
  "technologies": [
EOF

# Add technologies list
for ((i=0; i<${#TECHNOLOGIES[@]}; i++)); do
  if [ $i -gt 0 ]; then
    echo "    ," >> "$OUTPUT_FILE"
  fi
  echo "    \"${TECHNOLOGIES[$i]}\"" >> "$OUTPUT_FILE"
done

cat << EOF >> "$OUTPUT_FILE"
  ],
  "potential_gaps": [
EOF

# Add gaps list with metadata
for ((i=0; i<${#POTENTIAL_GAPS[@]}; i++)); do
  GAP="${POTENTIAL_GAPS[$i]}"
  IMPACT="medium"
  EVIDENCE="Technology detected but no specific guidance found"
  
  # Set impact based on gap type
  if [[ "$GAP" == *"security"* ]]; then
    IMPACT="high"
    EVIDENCE="Security-critical technology detected without specific security guidance"
  elif [[ "$GAP" == *"performance"* ]]; then
    IMPACT="medium"
    EVIDENCE="Performance-sensitive technology detected without optimization guidance"
  elif [[ "$GAP" == *"documentation"* ]]; then
    IMPACT="low"
    EVIDENCE="Limited documentation coverage for project scope"
  fi
  
  if [ $i -gt 0 ]; then
    echo "    ," >> "$OUTPUT_FILE"
  fi
  
  cat << EOF >> "$OUTPUT_FILE"
    {
      "id": "$GAP",
      "topic": "$GAP",
      "impact": "$IMPACT",
      "evidence": "$EVIDENCE",
      "sources_needed": ["Official documentation", "Industry best practices"],
      "research_required": true,
      "priority": "$IMPACT"
    }
EOF
done

cat << EOF >> "$OUTPUT_FILE"
  ],
  "recommendations": {
    "immediate_actions": [
      "Review high-impact gaps first (security-related)",
      "Create basic documentation structure if missing",
      "Identify critical technology-specific best practices"
    ],
    "research_topics": ${#POTENTIAL_GAPS[@]},
    "search_tools_recommended": true,
    "output_formats_suggested": ["skill", "documentation"]
  }
}
EOF

echo "" >&2
echo "Analysis complete!" >&2
echo "✅ Generated gap analysis: $OUTPUT_FILE" >&2
echo "" >&2

# Summary output
echo "Summary:" >&2
echo "--------" >&2
echo "Documents found: ${#EXISTING_DOCS[@]}" >&2
echo "Technologies detected: ${#TECHNOLOGIES[@]}" >&2
echo "Potential gaps identified: ${#POTENTIAL_GAPS[@]}" >&2
echo "" >&2

echo "High-impact gaps:" >&2
for gap in "${POTENTIAL_GAPS[@]}"; do
  if [[ "$gap" == *"security"* ]]; then
    echo "  ⚠️  $gap (security impact)" >&2
  fi
done

echo "" >&2
echo "Next steps:" >&2
echo "1. Review gaps.json for detailed analysis" >&2
echo "2. Use search tools to research high-impact gaps" >&2
echo "3. Generate guides using appropriate format (skill/documentation)" >&2
echo "4. Integrate guides into project documentation" >&2

echo '{"status": "success", "gaps_identified": '${#POTENTIAL_GAPS[@]}', "output_file": "'"$OUTPUT_FILE"'", "timestamp": "'"$TIMESTAMP"'"}'