#!/bin/bash
set -e

echo "Workflow Orchestrator: Available Patterns" >&2
echo "=========================================" >&2

PATTERNS_DIR="${1:-./references}"
TIMESTAMP=$(date +%Y-%m-%dT%H:%M:%S)

echo "Available workflow patterns:" >&2
echo "" >&2

# Check if patterns exist
if [ ! -d "$PATTERNS_DIR" ]; then
    echo "Patterns directory not found: $PATTERNS_DIR" >&2
    echo "Using built-in pattern descriptions instead." >&2
    echo "" >&2
fi

# Pattern descriptions
cat << EOF
1. Builder Only Pattern (builder-only.yaml)
   ---------------------------------------
   Description: Simple single-agent loop that retries until success
   Best for: Simple tasks, bug fixes, small features
   Agents: 1 (builder)
   Control: Low, Cost: Low, Speed: High
   Use when: You just need it done, quality is secondary to completion

2. Build + Verify Pattern (build-verify.yaml)
   -------------------------------------------
   Description: Two-agent sequential workflow with quality verification
   Best for: Production code, quality-critical work
   Agents: 2 (builder, verifier)
   Control: Medium, Cost: Medium, Speed: Medium
   Use when: Quality matters, need verification before deployment

3. Build + Verify + Plan Pattern (build-verify-plan.yaml)
   -------------------------------------------------------
   Description: Three-agent workflow with planning, building, and verification
   Best for: Complex features, architectural changes
   Agents: 3+ (planner, builder, verifier)
   Control: High, Cost: High, Speed: Low
   Use when: Task is complex, requires planning and quality assurance

4. Multi-Agent Pipeline Pattern (multi-agent-pipeline.yaml)
   --------------------------------------------------------
   Description: Parallel agent execution with result merging
   Best for: Multiple perspectives, comprehensive solutions
   Agents: 3+ parallel agents
   Control: High, Cost: High, Speed: Medium
   Use when: Need multiple expert perspectives simultaneously

5. Custom Pattern
   --------------
   Description: User-defined sequence of agents with custom routing
   Best for: Unique requirements, specialized workflows
   Agents: Variable
   Control: Full, Cost: Variable, Speed: Variable
   Use when: Existing patterns don't fit your specific needs
EOF

echo "" >&2
echo "Pattern Selection Matrix:" >&2
echo "┌─────────────────────┬─────────────┬────────────┬────────────┬──────────────┐" >&2
echo "│ Task Complexity     │ Simple      │ Medium     │ Complex    │ Very Complex │" >&2
echo "├─────────────────────┼─────────────┼────────────┼────────────┼──────────────┤" >&2
echo "│ Recommended Pattern │ Builder Only│ Build+Verify│ B+V+Plan   │ Multi-Agent  │" >&2
echo "├─────────────────────┼─────────────┼────────────┼────────────┼──────────────┤" >&2
echo "│ Control Needed      │ Low         │ Medium     │ High      │ Highest     │" >&2
echo "│ Cost                │ $           │ $$         │ $$$       │ $$$$        │" >&2
echo "│ Speed               │ Fast        │ Medium     │ Slow      │ Medium      │" >&2
echo "│ Quality             │ Medium      │ High       │ Highest   │ Highest      │" >&2
echo "└─────────────────────┴─────────────┴────────────┴────────────┴──────────────┘" >&2

echo "" >&2
echo "Usage Examples:" >&2
echo "  # List patterns with descriptions" >&2
echo "  npm run workflow:patterns" >&2
echo "" >&2
echo "  # Generate specific pattern" >&2
echo "  npm run workflow:generate -- --pattern build-verify --output my-workflow.yaml" >&2
echo "" >&2
echo "  # Analyze task and suggest pattern" >&2
echo "  npm run workflow:analyze -- --task \"Add authentication system\"" >&2

# Output JSON for machine parsing
cat << EOF
{
  "patterns": [
    {
      "name": "builder-only",
      "description": "Simple single-agent loop",
      "agents": 1,
      "control": "low",
      "cost": "low",
      "speed": "high",
      "file": "builder-only.yaml"
    },
    {
      "name": "build-verify",
      "description": "Two-agent sequential workflow with verification",
      "agents": 2,
      "control": "medium",
      "cost": "medium",
      "speed": "medium",
      "file": "build-verify.yaml"
    },
    {
      "name": "build-verify-plan",
      "description": "Three-agent workflow with planning",
      "agents": 3,
      "control": "high",
      "cost": "high",
      "speed": "low",
      "file": "build-verify-plan.yaml"
    },
    {
      "name": "multi-agent-pipeline",
      "description": "Parallel agent execution",
      "agents": 3,
      "control": "highest",
      "cost": "highest",
      "speed": "medium",
      "file": "multi-agent-pipeline.yaml"
    },
    {
      "name": "custom",
      "description": "User-defined workflow",
      "agents": "variable",
      "control": "full",
      "cost": "variable",
      "speed": "variable",
      "file": null
    }
  ],
  "timestamp": "$TIMESTAMP",
  "service": "workflow-orchestrator"
}
EOF