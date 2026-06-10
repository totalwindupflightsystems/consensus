#!/bin/bash
set -e

echo "Assumption Buster" >&2
echo "==================" >&2

# Check for required arguments
if [ -z "$1" ]; then
    echo "Usage: $0 <assumption> [context] [intensity]" >&2
    echo "Example: $0 'database-always-available' 'payment-system' aggressive" >&2
    echo "Intensity levels: light, medium, aggressive, extreme" >&2
    exit 1
fi

ASSUMPTION="$1"
CONTEXT="${2:-general}"
INTENSITY="${3:-medium}"
TIMESTAMP=$(date +%Y-%m-%dT%H:%M:%S)

echo "Assumption: $ASSUMPTION" >&2
echo "Context: $CONTEXT" >&2
echo "Intensity: $INTENSITY" >&2
echo "Timestamp: $TIMESTAMP" >&2
echo "" >&2

# Generate assumption busting challenges based on intensity
echo "Generating assumption busting challenges..." >&2
echo "" >&2

cat << EOF
Assumption Busting Analysis
──────────────────────────────
Assumption: $ASSUMPTION
Context: $CONTEXT
Intensity: $INTENSITY
Analysis Date: $TIMESTAMP

Busting Methodology:

1. Counterexample Generation:
   - What specific cases prove this assumption false?
   - What data contradicts this assumption?
   - What historical examples show this failing?
   - What expert opinions challenge this?

2. Edge Case Exploration:
   - What boundary conditions break this assumption?
   - What unusual situations violate this assumption?
   - What extreme values make this assumption fail?
   - What rare but catastrophic scenarios disprove this?

3. Stress Testing:
   - What load/pressure causes this assumption to break?
   - What resource constraints invalidate this assumption?
   - What environmental conditions make this assumption false?
   - What time pressures reveal this assumption's weakness?

4. Adversarial Thinking:
   - How could someone deliberately make this assumption fail?
   - What attack vectors target this assumption?
   - How could competitors exploit this assumption's weakness?
   - What malicious inputs would break this assumption?

5. Murphy's Law Application:
   - What can go wrong with this assumption?
   - What will go wrong with this assumption (eventually)?
   - What worst-case scenarios exist if this assumption fails?
   - What cascading failures might result from this assumption failing?

Intensity-Specific Challenges:

EOF

case "$INTENSITY" in
    "light")
        echo "Light intensity (10-15 minutes):" >&2
        cat << EOF
- Quick counterexample brainstorming
- Obvious edge cases only
- Surface-level stress testing
- Basic adversarial thinking
EOF
        ;;
    "medium")
        echo "Medium intensity (20-30 minutes):" >&2
        cat << EOF
- Systematic counterexample generation
- Common edge cases exploration
- Moderate stress testing
- Competitor perspective thinking
EOF
        ;;
    "aggressive")
        echo "Aggressive intensity (45-60 minutes):" >&2
        cat << EOF
- Exhaustive counterexample search
- Rare edge case investigation
- Extreme stress testing
- Adversary mindset adoption
- Black swan scenario generation
EOF
        ;;
    "extreme")
        echo "Extreme intensity (90+ minutes):" >&2
        cat << EOF
- Deliberate falsification attempt
- Catastrophic edge case exploration
- Breaking point determination
- Nation-state adversary perspective
- Complete assumption destruction attempt
EOF
        ;;
    *)
        echo "Unknown intensity: $INTENSITY" >&2
        cat << EOF
- Using medium intensity by default
EOF
        ;;
esac

cat << EOF

Busting Process:
1. Gather team (3-5 people recommended)
2. Present assumption clearly
3. Set timer based on intensity level
4. Generate busting ideas (no criticism during brainstorming)
5. Categorize findings (counterexamples, edge cases, stress failures)
6. Prioritize by impact and likelihood
7. Develop mitigation strategies
8. Document and assign action items

Expected Outcomes:
- Busted: Assumption proven false (document counterexample)
- Weakened: Assumption has known failure conditions (document limits)
- Strengthened: Assumption withstands busting attempts (document confidence)
- Unchanged: Insufficient information to bust (document testing needed)

Risk Assessment:
- Impact if assumption fails: [Low/Medium/High/Catastrophic]
- Likelihood of assumption failing: [Rare/Unlikely/Possible/Likely/Certain]
- Detection difficulty if assumption fails: [Easy/Medium/Hard/Impossible]
- Recovery difficulty if assumption fails: [Trivial/Moderate/Difficult/Impossible]

Documentation Template:
1. Assumption: [Clear statement]
2. Context: [Where this assumption applies]
3. Busting date: [$TIMESTAMP]
4. Busting intensity: [$INTENSITY]
5. Findings: [Counterexamples, edge cases, stress failures]
6. Status: [Busted/Weakened/Strengthened/Unchanged]
7. Impact if failed: [Assessment]
8. Mitigations: [Actions to reduce risk]
9. Next busting date: [When to re-test]

Assumption Registry Integration:
- Add to assumption registry with busting status
- Link to related assumptions and decisions
- Update risk register based on findings
- Share with relevant stakeholders

EOF

echo '{"status": "generated", "assumption": "'"$ASSUMPTION"'", "context": "'"$CONTEXT"'", "intensity": "'"$INTENSITY"'", "timestamp": "'"$TIMESTAMP"'", "methodology": "assumption-busting"}'