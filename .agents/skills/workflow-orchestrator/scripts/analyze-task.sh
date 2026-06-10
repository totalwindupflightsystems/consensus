#!/bin/bash
set -e

echo "Workflow Orchestrator: Task Analysis" >&2
echo "====================================" >&2

TASK="${1:-Add new feature}"
OUTPUT_FILE="${2:-task-analysis.json}"
TIMESTAMP=$(date +%Y-%m-%dT%H:%M:%S)

echo "Analyzing task: $TASK" >&2
echo "Output: $OUTPUT_FILE" >&2
echo "" >&2

# Simple analysis based on keywords (in a real implementation, this would use NLP)
TASK_LOWER=$(echo "$TASK" | tr '[:upper:]' '[:lower:]')

# Initialize variables
COMPLEXITY="medium"
PATTERN="build-verify"
REASON="Standard task requiring quality verification"
AGENTS=2
ESTIMATED_COST="10-20"
ESTIMATED_TIME="30-45 minutes"
SUCCESS_PROBABILITY=80

# Analyze task complexity
if [[ "$TASK_LOWER" =~ (bug|fix|small|simple|minor|typo) ]]; then
    COMPLEXITY="simple"
    PATTERN="builder-only"
    REASON="Simple task doesn't require verification"
    AGENTS=1
    ESTIMATED_COST="5-10"
    ESTIMATED_TIME="15-30 minutes"
    SUCCESS_PROBABILITY=90
elif [[ "$TASK_LOWER" =~ (complex|architecture|refactor|redesign|major) ]]; then
    COMPLEXITY="complex"
    PATTERN="build-verify-plan"
    REASON="Complex task requires planning"
    AGENTS=3
    ESTIMATED_COST="20-40"
    ESTIMATED_TIME="60-90 minutes"
    SUCCESS_PROBABILITY=70
elif [[ "$TASK_LOWER" =~ (multi|parallel|several|multiple|comprehensive) ]]; then
    COMPLEXITY="very complex"
    PATTERN="multi-agent"
    REASON="Task has multiple aspects that can be parallelized"
    AGENTS=4
    ESTIMATED_COST="30-60"
    ESTIMATED_TIME="45-75 minutes"
    SUCCESS_PROBABILITY=75
fi

# Check for security/quality keywords
if [[ "$TASK_LOWER" =~ (auth|security|password|encrypt|secure|payment) ]]; then
    if [ "$PATTERN" = "builder-only" ]; then
        PATTERN="build-verify"
        REASON="Security-related tasks require verification"
        AGENTS=2
        ESTIMATED_COST="15-25"
        SUCCESS_PROBABILITY=85
    fi
fi

# Check for testing keywords
if [[ "$TASK_LOWER" =~ (test|testing|coverage|spec|verify) ]]; then
    PATTERN="build-verify"
    REASON="Testing tasks require verification"
    AGENTS=2
fi

echo "Analysis Results:" >&2
echo "-----------------" >&2
echo "Task: $TASK" >&2
echo "Complexity: $COMPLEXITY" >&2
echo "Recommended Pattern: $PATTERN" >&2
echo "Reason: $REASON" >&2
echo "" >&2
echo "Agents Required: $AGENTS" >&2
echo "Estimated Cost: \$"$ESTIMATED_COST >&2
echo "Estimated Time: $ESTIMATED_TIME" >&2
echo "Success Probability: ${SUCCESS_PROBABILITY}%" >&2
echo "" >&2

# Pattern comparison
echo "Pattern Comparison:" >&2
echo "┌─────────────────┬─────────┬─────────┬─────────┬──────────┐" >&2
echo "│ Pattern         │ Control │ Cost   │ Speed   │ Quality  │" >&2
echo "├─────────────────┼─────────┼─────────┼─────────┼──────────┤" >&2

if [ "$PATTERN" = "builder-only" ]; then
    echo "│ Builder Only    │ Low     │ $      │ Fast    │ Medium   │" >&2
    echo "│ Build + Verify  │ Medium  │ \$\$     │ Medium  │ High     │" >&2
    echo "│ Build+Verify+Plan│ High    │ \$\$\$    │ Slow    │ Highest  │" >&2
    echo "│ Multi-Agent     │ Highest │ \$\$\$\$   │ Medium  │ Highest  │" >&2
elif [ "$PATTERN" = "build-verify" ]; then
    echo "│ Builder Only    │ Low     │ $      │ Fast    │ Medium   │" >&2
    echo "│ ✓ Build + Verify│ Medium  │ \$\$     │ Medium  │ High     │" >&2
    echo "│ Build+Verify+Plan│ High    │ \$\$\$    │ Slow    │ Highest  │" >&2
    echo "│ Multi-Agent     │ Highest │ \$\$\$\$   │ Medium  │ Highest  │" >&2
elif [ "$PATTERN" = "build-verify-plan" ]; then
    echo "│ Builder Only    │ Low     │ $      │ Fast    │ Medium   │" >&2
    echo "│ Build + Verify  │ Medium  │ \$\$     │ Medium  │ High     │" >&2
    echo "│ ✓ Build+Verify+Plan│ High    │ \$\$\$    │ Slow    │ Highest  │" >&2
    echo "│ Multi-Agent     │ Highest │ \$\$\$\$   │ Medium  │ Highest  │" >&2
else
    echo "│ Builder Only    │ Low     │ $      │ Fast    │ Medium   │" >&2
    echo "│ Build + Verify  │ Medium  │ \$\$     │ Medium  │ High     │" >&2
    echo "│ Build+Verify+Plan│ High    │ \$\$\$    │ Slow    │ Highest  │" >&2
    echo "│ ✓ Multi-Agent   │ Highest │ \$\$\$\$   │ Medium  │ Highest  │" >&2
fi

echo "└─────────────────┴─────────┴─────────┴─────────┴──────────┘" >&2
echo "" >&2

echo "Recommendations:" >&2
echo "1. Use $PATTERN pattern for this task" >&2
echo "2. Start with $AGENTS agents" >&2
echo "3. Budget \$$ESTIMATED_COST and $ESTIMATED_TIME" >&2
echo "4. Monitor progress and be ready to adjust" >&2
echo "" >&2

echo "Next steps:" >&2
echo "  # Generate configuration" >&2
echo "  npm run workflow:generate -- --pattern $PATTERN --task \"$TASK\"" >&2
echo "" >&2
echo "  # Execute workflow" >&2
echo "  npm run workflow:execute -- --pattern $PATTERN --task \"$TASK\"" >&2

# Output JSON for machine parsing
cat << EOF > "$OUTPUT_FILE"
{
  "analysis": {
    "timestamp": "$TIMESTAMP",
    "task": "$TASK",
    "complexity": "$COMPLEXITY",
    "recommended_pattern": "$PATTERN",
    "reason": "$REASON",
    "agents_required": $AGENTS,
    "estimated_cost": "$ESTIMATED_COST",
    "estimated_time": "$ESTIMATED_TIME",
    "success_probability": $SUCCESS_PROBABILITY
  },
  "patterns": {
    "builder-only": {
      "control": "low",
      "cost": "low",
      "speed": "high",
      "quality": "medium",
      "recommended_for": ["simple tasks", "bug fixes", "small features"]
    },
    "build-verify": {
      "control": "medium",
      "cost": "medium",
      "speed": "medium",
      "quality": "high",
      "recommended_for": ["production code", "quality-critical work", "testing tasks"]
    },
    "build-verify-plan": {
      "control": "high",
      "cost": "high",
      "speed": "low",
      "quality": "highest",
      "recommended_for": ["complex features", "architectural changes", "security tasks"]
    },
    "multi-agent": {
      "control": "highest",
      "cost": "highest",
      "speed": "medium",
      "quality": "highest",
      "recommended_for": ["multiple aspects", "parallel work", "comprehensive solutions"]
    }
  },
  "recommendations": [
    "Use $PATTERN pattern for this task",
    "Start with $AGENTS agents",
    "Budget \$$ESTIMATED_COST and $ESTIMATED_TIME",
    "Monitor progress and be ready to adjust"
  ]
}
EOF

echo "✅ Analysis saved to: $OUTPUT_FILE" >&2
echo '{"status": "analyzed", "service": "workflow-orchestrator", "timestamp": "'"$TIMESTAMP"'", "output_file": "'"$OUTPUT_FILE"'", "recommended_pattern": "'"$PATTERN"'"}'