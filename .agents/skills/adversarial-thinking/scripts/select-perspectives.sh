#!/bin/bash
set -e

echo "Adversarial Thinking Perspective Selector" >&2
echo "=========================================" >&2

# Check for required arguments
if [ -z "$1" ]; then
    echo "Usage: $0 <context> [specifics...]" >&2
    echo "Example: $0 'technical-decision' 'microservices-architecture' 'high-stakes'" >&2
    echo "Contexts: technical-decision, product-decision, security-assessment, process-design, risk-assessment, general" >&2
    exit 1
fi

CONTEXT="$1"
SPECIFICS="${2:-}"
STAKES="${3:-medium}"
TIMESTAMP=$(date +%Y-%m-%dT%H:%M:%S)

echo "Context: $CONTEXT" >&2
echo "Specifics: $SPECIFICS" >&2
echo "Stakes: $STAKES" >&2
echo "Timestamp: $TIMESTAMP" >&2
echo "" >&2

# Generate perspective selection
echo "Generating adversarial perspective selection..." >&2
echo "" >&2

cat << EOF
Adversarial Thinking Perspective Selection
──────────────────────────────────────────
Context: $CONTEXT
Specifics: $SPECIFICS
Stakes: $STAKES
Date: $TIMESTAMP

Available Adversarial Perspectives:
1. Devil's Advocate (@skills/devils-advocate)
2. Assumption Buster (@skills/assumption-buster)
3. Red Team (@skills/redteam)
4. White Hat (@skills/white-hat)
5. Trust But Verify (@skills/trust-but-verify)

Recommended Perspective Mix:

EOF

case "$CONTEXT" in
    "technical-decision"|"technical"|"architecture")
        echo "Technical Decision Context:" >&2
        cat << EOF
Primary Perspectives (HIGH PRIORITY):
1. Devil's Advocate - Challenge technology choices and architecture decisions
2. Assumption Buster - Test technical assumptions (scalability, performance, dependencies)

Secondary Perspectives (MEDIUM PRIORITY):
3. Red Team - Evaluate security implications and attack surface
4. Trust But Verify - Validate performance claims and technical benchmarks

Tertiary Perspective (CONTEXT DEPENDENT):
5. White Hat - Design secure implementation (if security-relevant)

Rationale:
- Technical decisions benefit most from logical challenge (devil's advocate)
- Technical assumptions often have hidden failure modes (assumption buster)
- Security implications important for internet-facing systems (red team)
- Technical claims often overoptimistic (trust but verify)

Time Allocation (based on stakes):
EOF
        case "$STAKES" in
            "low")
                echo "- Devil's Advocate: 60%" >&2
                echo "- Assumption Buster: 40%" >&2
                echo "- Total time: 30-60 minutes" >&2
                ;;
            "medium")
                echo "- Devil's Advocate: 40%" >&2
                echo "- Assumption Buster: 30%" >&2
                echo "- Red Team: 20%" >&2
                echo "- Trust But Verify: 10%" >&2
                echo "- Total time: 60-120 minutes" >&2
                ;;
            "high"|"high-stakes")
                echo "- Devil's Advocate: 30%" >&2
                echo "- Assumption Buster: 25%" >&2
                echo "- Red Team: 20%" >&2
                echo "- Trust But Verify: 15%" >&2
                echo "- White Hat: 10%" >&2
                echo "- Total time: 120-240 minutes" >&2
                ;;
            *)
                echo "- Using medium stakes allocation by default" >&2
                ;;
        esac
        ;;
    
    "product-decision"|"product"|"feature")
        echo "Product/Feature Decision Context:" >&2
        cat << EOF
Primary Perspectives (HIGH PRIORITY):
1. Devil's Advocate - Challenge user assumptions and market fit
2. Assumption Buster - Test business model and user behavior assumptions

Secondary Perspectives (MEDIUM PRIORITY):
3. Trust But Verify - Validate user research and market data
4. Red Team - Evaluate competitive threats and market vulnerabilities

Tertiary Perspective (CONTEXT DEPENDENT):
5. White Hat - Design secure user experiences (if security-sensitive)

Rationale:
- Product decisions often suffer from confirmation bias (devil's advocate)
- User behavior assumptions frequently wrong (assumption buster)
- Market data often misinterpreted (trust but verify)
- Competitive analysis crucial for product success (red team)

Time Allocation (based on stakes):
EOF
        case "$STAKES" in
            "low")
                echo "- Devil's Advocate: 70%" >&2
                echo "- Assumption Buster: 30%" >&2
                echo "- Total time: 30-60 minutes" >&2
                ;;
            "medium")
                echo "- Devil's Advocate: 40%" >&2
                echo "- Assumption Buster: 30%" >&2
                echo "- Trust But Verify: 20%" >&2
                echo "- Red Team: 10%" >&2
                echo "- Total time: 60-120 minutes" >&2
                ;;
            "high"|"high-stakes")
                echo "- Devil's Advocate: 30%" >&2
                echo "- Assumption Buster: 25%" >&2
                echo "- Trust But Verify: 20%" >&2
                echo "- Red Team: 15%" >&2
                echo "- White Hat: 10%" >&2
                echo "- Total time: 120-240 minutes" >&2
                ;;
            *)
                echo "- Using medium stakes allocation by default" >&2
                ;;
        esac
        ;;
    
    "security-assessment"|"security"|"pentest")
        echo "Security Assessment Context:" >&2
        cat << EOF
Primary Perspectives (HIGH PRIORITY):
1. Red Team - Attack simulation and vulnerability finding
2. White Hat - Defense design and security control implementation

Secondary Perspectives (MEDIUM PRIORITY):
3. Assumption Buster - Test security assumption limits
4. Trust But Verify - Validate security control effectiveness

Tertiary Perspective (CONTEXT DEPENDENT):
5. Devil's Advocate - Challenge security architecture decisions

Rationale:
- Security requires both attack and defense perspectives (red team + white hat)
- Security assumptions often dangerously optimistic (assumption buster)
- Security control effectiveness must be verified (trust but verify)
- Security architecture decisions need logical challenge (devil's advocate)

Time Allocation (based on stakes):
EOF
        case "$STAKES" in
            "low")
                echo "- Red Team: 60%" >&2
                echo "- White Hat: 40%" >&2
                echo "- Total time: 60-120 minutes" >&2
                ;;
            "medium")
                echo "- Red Team: 40%" >&2
                echo "- White Hat: 30%" >&2
                echo "- Assumption Buster: 20%" >&2
                echo "- Trust But Verify: 10%" >&2
                echo "- Total time: 120-180 minutes" >&2
                ;;
            "high"|"high-stakes")
                echo "- Red Team: 30%" >&2
                echo "- White Hat: 25%" >&2
                echo "- Assumption Buster: 20%" >&2
                echo "- Trust But Verify: 15%" >&2
                echo "- Devil's Advocate: 10%" >&2
                echo "- Total time: 180-360 minutes" >&2
                ;;
            *)
                echo "- Using medium stakes allocation by default" >&2
                ;;
        esac
        ;;
    
    "process-design"|"process"|"operational")
        echo "Process/Operational Design Context:" >&2
        cat << EOF
Primary Perspectives (HIGH PRIORITY):
1. Devil's Advocate - Challenge efficiency and effectiveness claims
2. Assumption Buster - Test process reliability and resilience assumptions

Secondary Perspectives (MEDIUM PRIORITY):
3. Trust But Verify - Validate process metrics and outcomes
4. Red Team - Evaluate process security and abuse potential

Tertiary Perspective (CONTEXT DEPENDENT):
5. White Hat - Design secure and resilient processes

Rationale:
- Process designs often have hidden inefficiencies (devil's advocate)
- Process assumptions fail under stress (assumption buster)
- Process metrics often misleading (trust but verify)
- Processes can be abused or attacked (red team)

Time Allocation (based on stakes):
EOF
        case "$STAKES" in
            "low")
                echo "- Devil's Advocate: 80%" >&2
                echo "- Assumption Buster: 20%" >&2
                echo "- Total time: 30-60 minutes" >&2
                ;;
            "medium")
                echo "- Devil's Advocate: 50%" >&2
                echo "- Assumption Buster: 30%" >&2
                echo "- Trust But Verify: 20%" >&2
                echo "- Total time: 60-90 minutes" >&2
                ;;
            "high"|"high-stakes")
                echo "- Devil's Advocate: 40%" >&2
                echo "- Assumption Buster: 25%" >&2
                echo "- Trust But Verify: 20%" >&2
                echo "- Red Team: 10%" >&2
                echo "- White Hat: 5%" >&2
                echo "- Total time: 90-150 minutes" >&2
                ;;
            *)
                echo "- Using medium stakes allocation by default" >&2
                ;;
        esac
        ;;
    
    "risk-assessment"|"risk"|"risk-analysis")
        echo "Risk Assessment Context:" >&2
        cat << EOF
Primary Perspectives (HIGH PRIORITY):
1. Assumption Buster - Test risk assumption limits and failure modes
2. Trust But Verify - Validate risk data and probability estimates

Secondary Perspectives (MEDIUM PRIORITY):
3. Devil's Advocate - Challenge risk prioritization and mitigation strategies
4. Red Team - Evaluate attack-based risks (if security-related)

Tertiary Perspective (CONTEXT DEPENDENT):
5. White Hat - Design risk mitigation controls

Rationale:
- Risk assessments rely heavily on assumptions (assumption buster)
- Risk data often incomplete or biased (trust but verify)
- Risk prioritization needs logical challenge (devil's advocate)
- Security risks require attack perspective (red team)

Time Allocation (based on stakes):
EOF
        case "$STAKES" in
            "low")
                echo "- Assumption Buster: 60%" >&2
                echo "- Trust But Verify: 40%" >&2
                echo "- Total time: 30-60 minutes" >&2
                ;;
            "medium")
                echo "- Assumption Buster: 40%" >&2
                echo "- Trust But Verify: 30%" >&2
                echo "- Devil's Advocate: 20%" >&2
                echo "- Red Team: 10%" >&2
                echo "- Total time: 60-120 minutes" >&2
                ;;
            "high"|"high-stakes")
                echo "- Assumption Buster: 30%" >&2
                echo "- Trust But Verify: 25%" >&2
                echo "- Devil's Advocate: 20%" >&2
                echo "- Red Team: 15%" >&2
                echo "- White Hat: 10%" >&2
                echo "- Total time: 120-240 minutes" >&2
                ;;
            *)
                echo "- Using medium stakes allocation by default" >&2
                ;;
        esac
        ;;
    
    "general"|"default")
        echo "General Adversarial Thinking Context:" >&2
        cat << EOF
Balanced Perspective Mix (ALL PERSPECTIVES):
1. Devil's Advocate - 25% (Logical challenge and alternatives)
2. Assumption Buster - 25% (Assumption testing and failure modes)
3. Red Team - 20% (Attack simulation and vulnerability finding)
4. White Hat - 15% (Defense design and security controls)
5. Trust But Verify - 15% (Evidence validation and reality checking)

Rationale:
- General context requires balanced adversarial coverage
- Each perspective addresses different types of weaknesses
- Overemphasis on one perspective creates blind spots
- Balanced approach most robust for unknown contexts

Time Allocation (based on stakes):
EOF
        case "$STAKES" in
            "low")
                echo "- All perspectives: Equal short sessions" >&2
                echo "- Total time: 45-75 minutes" >&2
                ;;
            "medium")
                echo "- Balanced mix as above" >&2
                echo "- Total time: 90-150 minutes" >&2
                ;;
            "high"|"high-stakes")
                echo "- Balanced mix with deeper each perspective" >&2
                echo "- Total time: 180-300 minutes" >&2
                ;;
            *)
                echo "- Using medium stakes allocation by default" >&2
                ;;
        esac
        ;;
    
    *)
        echo "Unknown context: $CONTEXT" >&2
        cat << EOF
- Using general context by default
EOF
        ;;
esac

cat << EOF

Implementation Guide:

1. Team Composition:
   - Different people for different adversarial roles
   - Rotate roles to build diverse adversarial skills
   - Include outsiders for fresh perspectives

2. Session Structure:
   - Briefing: Present subject for adversarial review
   - Role assignment: Assign specific adversarial perspectives
   - Time-boxing: Strict time limits per perspective
   - Documentation: Record challenges and responses
   - Synthesis: Combine findings into integrated view

3. Output Format:
   - Executive summary: Key adversarial findings
   - By perspective: Challenges from each adversarial role
   - Integrated analysis: Combined risk assessment
   - Recommendations: Specific improvements based on findings
   - Next steps: Actions, owners, timelines

4. Success Metrics:
   - Blind spots identified: Number of significant issues uncovered
   - Decision quality improvement: Post-hoc evaluation
   - Risk reduction: Measurable risk reduction from findings
   - Team learning: Knowledge gained from adversarial process

Common Pitfalls to Avoid:
- Adversarial overload: Too much challenge, not enough building
- Perspective imbalance: Over-reliance on favorite adversarial style
- Ritualistic challenge: Going through motions without engagement
- Personality conflict: Adversarial roles becoming personal
- Analysis paralysis: Endless challenge without decision

Adaptation Guidelines:
- Adjust time allocation based on findings during session
- Re-prioritize perspectives if initial assumptions prove wrong
- Invite additional perspectives if unexpected issues emerge
- Shorten or extend sessions based on value being generated

Documentation Template:
1. Context: $CONTEXT
2. Specifics: $SPECIFICS
3. Stakes: $STAKES
4. Date: $TIMESTAMP
5. Selected perspectives: [List with time allocation]
6. Team members: [Names and assigned perspectives]
7. Findings by perspective: [Challenges, weaknesses, alternatives]
8. Integrated risk assessment: [Combined risk view]
9. Action items: [Specific improvements with owners]
10. Follow-up date: [When to review implementation]

EOF

echo '{"status": "generated", "context": "'"$CONTEXT"'", "specifics": "'"$SPECIFICS"'", "stakes": "'"$STAKES"'", "timestamp": "'"$TIMESTAMP"'", "perspectives_selected": true}'