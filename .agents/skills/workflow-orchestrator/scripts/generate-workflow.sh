#!/bin/bash
set -e

echo "Workflow Orchestrator: Generate Configuration" >&2
echo "============================================" >&2

# Default values
PATTERN="${1:-builder-only}"
OUTPUT_FILE="${2:-workflow-config.yaml}"
TASK_NAME="${3:-untitled-task}"
MAX_ITERATIONS="${4:-20}"
TIMESTAMP=$(date +%Y-%m-%dT%H:%M:%S)

echo "Generating workflow configuration:" >&2
echo "  Pattern: $PATTERN" >&2
echo "  Output: $OUTPUT_FILE" >&2
echo "  Task: $TASK_NAME" >&2
echo "  Max iterations: $MAX_ITERATIONS" >&2
echo "" >&2

# Check if pattern template exists
PATTERN_FILE="./references/$PATTERN.yaml"
if [ -f "$PATTERN_FILE" ]; then
    echo "Using pattern template: $PATTERN_FILE" >&2
    # Create a customized version
    sed "s/untitled-task/$TASK_NAME/g; s/max_iterations: 20/max_iterations: $MAX_ITERATIONS/g" "$PATTERN_FILE" > "$OUTPUT_FILE"
    echo "✅ Generated workflow configuration: $OUTPUT_FILE" >&2
    echo "" >&2
    echo "To use this configuration:" >&2
    echo "1. Review the generated YAML file" >&2
    echo "2. Customize agent roles and tasks as needed" >&2
    echo "3. Execute with: npm run workflow:execute -- --config $OUTPUT_FILE" >&2
else
    echo "⚠️  Pattern template not found: $PATTERN_FILE" >&2
    echo "Generating basic configuration from template..." >&2
    
    # Generate basic configuration
    case "$PATTERN" in
        builder-only|simple)
            cat << EOF > "$OUTPUT_FILE"
# Workflow Configuration: $TASK_NAME
# Generated: $TIMESTAMP
# Pattern: builder-only

workflow:
  name: "$TASK_NAME"
  version: "1.0"
  
  settings:
    max_iterations: $MAX_ITERATIONS
    stop_on_success: true
    escape_hatch: true
    
  agents:
    builder:
      role: "Build $TASK_NAME"
      model: "anthropic/claude-sonnet-4-20250514"
      temperature: 0.7
      tools: ["read", "write", "edit", "bash"]
      stop_condition: "<promise>TASK_COMPLETE</promise>"
      
  sequence:
    - agent: builder
      
  success_criteria:
    - "Task completed successfully"
    - "All requirements met"
    
  telemetry:
    track_costs: true
    track_iterations: true
EOF
            ;;
            
        build-verify|verified)
            cat << EOF > "$OUTPUT_FILE"
# Workflow Configuration: $TASK_NAME
# Generated: $TIMESTAMP
# Pattern: build-verify

workflow:
  name: "$TASK_NAME"
  version: "1.0"
  
  settings:
    max_iterations: $MAX_ITERATIONS
    stop_on_success: true
    escape_hatch: true
    
  agents:
    builder:
      role: "Build $TASK_NAME"
      model: "anthropic/claude-sonnet-4-20250514"
      temperature: 0.7
      tools: ["read", "write", "edit", "bash"]
      stop_condition: "<promise>BUILT</promise>"
      
    verifier:
      role: "Verify quality and correctness"
      model: "anthropic/claude-sonnet-4-20250514"
      temperature: 0.3
      tools: ["read", "bash", "question"]
      stop_condition: "<promise>VERIFIED</promise>"
      
  sequence:
    - agent: builder
    - agent: verifier
      condition: "builder.stop_condition met"
      
  feedback:
    verifier_failed:
      target: builder
      message: "Verification failed, fix issues"
      
  success_criteria:
    - "Built successfully"
    - "Verified for quality"
    
  telemetry:
    track_costs: true
    track_iterations: true
EOF
            ;;
            
        build-verify-plan|planned)
            cat << EOF > "$OUTPUT_FILE"
# Workflow Configuration: $TASK_NAME
# Generated: $TIMESTAMP
# Pattern: build-verify-plan

workflow:
  name: "$TASK_NAME"
  version: "1.0"
  
  settings:
    max_iterations: $MAX_ITERATIONS
    stop_on_success: true
    escape_hatch: true
    
  agents:
    planner:
      role: "Plan approach for $TASK_NAME"
      model: "anthropic/claude-sonnet-4-20250514"
      temperature: 0.3
      tools: ["read", "write", "question"]
      stop_condition: "<promise>PLANNED</promise>"
      
    builder:
      role: "Build $TASK_NAME"
      model: "anthropic/claude-sonnet-4-20250514"
      temperature: 0.7
      tools: ["read", "write", "edit", "bash"]
      stop_condition: "<promise>BUILT</promise>"
      
    verifier:
      role: "Verify quality and correctness"
      model: "anthropic/claude-sonnet-4-20250514"
      temperature: 0.3
      tools: ["read", "bash", "question"]
      stop_condition: "<promise>VERIFIED</promise>"
      
  sequence:
    - agent: planner
    - agent: builder
      condition: "planner.stop_condition met"
    - agent: verifier
      condition: "builder.stop_condition met"
      
  feedback:
    verifier_failed:
      target: planner
      message: "Verification failed, replan needed"
      
    builder_stuck:
      target: planner
      message: "Builder stuck, needs guidance"
      
  success_criteria:
    - "Well-planned approach"
    - "Built successfully"
    - "Verified for quality"
    
  telemetry:
    track_costs: true
    track_iterations: true
EOF
            ;;
            
        multi-agent|parallel)
            cat << EOF > "$OUTPUT_FILE"
# Workflow Configuration: $TASK_NAME
# Generated: $TIMESTAMP
# Pattern: multi-agent

workflow:
  name: "$TASK_NAME"
  version: "1.0"
  
  settings:
    max_iterations: $MAX_ITERATIONS
    stop_on_success: true
    escape_hatch: true
    
  agents:
    specialist_a:
      role: "Specialist for aspect A of $TASK_NAME"
      model: "anthropic/claude-sonnet-4-20250514"
      temperature: 0.7
      tools: ["read", "write", "edit"]
      stop_condition: "<promise>ASPECT_A_COMPLETE</promise>"
      
    specialist_b:
      role: "Specialist for aspect B of $TASK_NAME"
      model: "anthropic/claude-sonnet-4-20250514"
      temperature: 0.7
      tools: ["read", "write", "edit"]
      stop_condition: "<promise>ASPECT_B_COMPLETE</promise>"
      
    merger:
      role: "Merge parallel work into cohesive solution"
      model: "anthropic/claude-sonnet-4-20250514"
      temperature: 0.5
      tools: ["read", "write", "edit", "bash"]
      stop_condition: "<promise>MERGED</promise>"
      
  parallel:
    - specialist_a
    - specialist_b
    
  sequence:
    - parallel: [specialist_a, specialist_b]
    - agent: merger
      condition: "specialist_a.stop_condition met AND specialist_b.stop_condition met"
      
  success_criteria:
    - "All aspects completed"
    - "Work merged successfully"
    
  telemetry:
    track_costs: true
    track_iterations: true
EOF
            ;;
            
        *)
            echo "Unknown pattern: $PATTERN. Using builder-only as default." >&2
            PATTERN="builder-only"
            # Recursive call with default
            "$0" "builder-only" "$OUTPUT_FILE" "$TASK_NAME" "$MAX_ITERATIONS"
            exit 0
            ;;
    esac
    
    echo "✅ Generated workflow configuration: $OUTPUT_FILE" >&2
fi

echo "" >&2
echo "Next steps:" >&2
echo "1. Customize agent roles and specific tasks" >&2
echo "2. Adjust tools and permissions as needed" >&2
echo "3. Set appropriate stop conditions" >&2
echo "4. Define success criteria clearly" >&2
echo "" >&2

# Output JSON status
echo '{"status": "generated", "service": "workflow-orchestrator", "timestamp": "'"$TIMESTAMP"'", "output_file": "'"$OUTPUT_FILE"'", "pattern": "'"$PATTERN"'", "task_name": "'"$TASK_NAME"'", "max_iterations": "'"$MAX_ITERATIONS"'"}'