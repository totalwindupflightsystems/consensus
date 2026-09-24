#!/bin/bash
set -e

echo "Devil's Advocate Challenge" >&2
echo "==========================" >&2

# Check for required arguments
if [ -z "$1" ]; then
    echo "Usage: $0 <decision|assumption|proposal> [context]" >&2
    echo "Example: $0 'use-microservices' 'ecommerce-platform'" >&2
    exit 1
fi

TARGET="$1"
CONTEXT="${2:-general}"
TIMESTAMP=$(date +%Y-%m-%dT%H:%M:%S)

echo "Target: $TARGET" >&2
echo "Context: $CONTEXT" >&2
echo "Timestamp: $TIMESTAMP" >&2
echo "" >&2

# Generate devil's advocate challenges
echo "Generating devil's advocate challenges..." >&2
echo "" >&2

cat << EOF
Devil's Advocate Analysis
──────────────────────────────
Target: $TARGET
Context: $CONTEXT
Analysis Date: $TIMESTAMP

Core Questions to Challenge:

1. What if the opposite approach is actually better?
   - What evidence supports alternative approaches?
   - What are we assuming that might be wrong?
   - What would a competitor do differently?

2. What are the hidden costs and trade-offs?
   - What maintenance burden does this create?
   - What learning curve is required?
   - What dependencies does this create?
   - What flexibility does this sacrifice?

3. What failure modes are we ignoring?
   - What happens when this scales 10x?
   - What breaks under stress or load?
   - How does this fail gracefully (or not)?
   - What recovery procedures are needed?

4. What assumptions need explicit testing?
   - What user behavior are we assuming?
   - What technical capabilities are we assuming?
   - What market conditions are we assuming?
   - What team capabilities are we assuming?

5. What alternative perspectives should we consider?
   - How would a new team member view this?
   - How would a customer view this?
   - How would a regulator view this?
   - How would a competitor view this?

6. What evidence contradicts our position?
   - What data suggests alternative approaches?
   - What historical precedents suggest caution?
   - What expert opinions differ from ours?
   - What user feedback suggests different needs?

7. What are the second-order consequences?
   - What ripple effects might this create?
   - How might this decision constrain future choices?
   - What unintended consequences might emerge?
   - How might this interact with other systems/decisions?

8. What if our key assumptions are wrong?
   - What if the problem we're solving isn't the real problem?
   - What if user needs are different than we think?
   - What if technology constraints change unexpectedly?
   - What if market conditions shift dramatically?

Recommended Challenge Process:
1. Assign someone explicitly to play devil's advocate
2. Time-box the challenge session (15-30 minutes)
3. Document all challenges and responses
4. Identify which challenges require further investigation
5. Update plans based on valid challenges

Challenge Intensity: Medium (adjust based on decision criticality)
Time Investment: 10-30 minutes recommended
Expected Value: Higher-quality decision with fewer blind spots

EOF

echo '{"status": "generated", "target": "'"$TARGET"'", "context": "'"$CONTEXT"'", "timestamp": "'"$TIMESTAMP"'", "challenges_generated": 8, "format": "devils-advocate"}'