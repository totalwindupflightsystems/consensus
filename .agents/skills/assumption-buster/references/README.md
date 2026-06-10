# Assumption Buster Reference

## Overview

Assumption busting is an aggressive form of critical thinking that actively seeks to disprove assumptions through counterexamples, edge cases, and stress testing. Unlike assumption testing (which seeks to validate), assumption busting tries to falsify assumptions to discover their breaking points and limitations.

## Philosophical Foundation

### Falsification Principle (Karl Popper)
- Scientific theories can never be proven true, only falsified
- The value of a theory is in its ability to withstand attempts to falsify it
- Assumption busting applies this principle to practical assumptions

### Black Swan Theory (Nassim Taleb)
- Rare, unpredictable events with massive impact
- Assumptions often fail to account for black swans
- Assumption busting seeks these catastrophic failure modes

### Murphy's Law
- "Anything that can go wrong, will go wrong"
- Assumption busting operationalizes this by asking "what can go wrong?"
- Focus on worst-case scenarios, not just probable ones

## Methodology

### 1. Identify Assumptions
- **Explicit assumptions**: Stated beliefs about how things work
- **Implicit assumptions**: Unstated beliefs embedded in decisions
- **Meta-assumptions**: Assumptions about assumptions

### 2. Attempt Falsification
- **Counterexample generation**: Find cases where assumption fails
- **Boundary testing**: Push assumptions to their limits
- **Stress testing**: Apply extreme conditions
- **Adversarial thinking**: Actively try to break the assumption

### 3. Document Failure Modes
- **Breaking point**: Where/when assumption fails
- **Failure consequences**: What happens when assumption fails
- **Recovery requirements**: How to recover from assumption failure
- **Mitigation strategies**: How to reduce impact of assumption failure

### 4. Update Understanding
- **Revised assumptions**: Assumptions with known limits
- **Known unknowns**: Areas where assumptions don't apply
- **Risk boundaries**: Understanding of where risks exist
- **Contingency plans**: Prepared responses for assumption failure

## Busting Techniques

### Technical Assumptions:
- **Load to failure**: What load breaks the system?
- **Resource starvation**: What happens with minimal resources?
- **Dependency failure**: What if all dependencies fail?
- **Version conflicts**: What if versions are incompatible?
- **Data corruption**: What if data is maliciously malformed?

### Business Assumptions:
- **Market reversal**: What if customer preferences change overnight?
- **Competitor innovation**: What if competitors leapfrog us?
- **Regulatory shock**: What if regulations completely change?
- **Economic collapse**: What if funding disappears?
- **Team dissolution**: What if key people leave?

### Process Assumptions:
- **Communication breakdown**: What if all communication fails?
- **Tool failure**: What if primary tools become unavailable?
- **Knowledge loss**: What if institutional knowledge disappears?
- **Timeline compression**: What if deadlines move up 90%?
- **Quality erosion**: What if quality standards decline steadily?

## Implementation Process

### Step 1: Assumption Inventory
1. **Brainstorm assumptions**: List all assumptions about the system/decision
2. **Categorize assumptions**: Technical, business, process, etc.
3. **Prioritize assumptions**: By impact if wrong and confidence level

### Step 2: Busting Session
1. **Select assumption**: Start with high-impact, high-confidence assumptions
2. **Generate counterexamples**: "What would make this assumption false?"
3. **Explore edge cases**: "What unusual situations break this?"
4. **Stress test limits**: "What extreme conditions reveal weaknesses?"
5. **Document findings**: Record busted assumptions and failure modes

### Step 3: Analysis and Action
1. **Evaluate impact**: How serious is assumption failure?
2. **Develop mitigations**: How to reduce impact if assumption fails?
3. **Update plans**: Revise plans based on busted assumptions
4. **Communicate findings**: Share results with stakeholders
5. **Schedule re-busting**: When to re-test assumptions

## Common Assumption Categories to Bust

### Technology Stack Assumptions:
- "This framework will be maintained for 5+ years"
- "This database can handle our scale"
- "This cloud provider won't have major outages"
- "This library has no critical security vulnerabilities"
- "This API won't change breakingly"

### User Behavior Assumptions:
- "Users will read instructions/documentation"
- "Users will behave rationally"
- "Users have stable internet connections"
- "Users will update their apps/browsers"
- "Users will provide accurate information"

### Market Assumptions:
- "Our competitive advantage will last X years"
- "Customer demand will grow at Y rate"
- "Market conditions will remain stable"
- "Regulatory environment won't change significantly"
- "Economic conditions will support our business model"

### Team/Process Assumptions:
- "We'll have the same team throughout the project"
- "Team velocity will remain constant"
- "Processes will be followed consistently"
- "Communication will be effective"
- "Priorities won't change dramatically"

## Tools and Techniques

### Brainstorming Techniques:
- **Pre-mortem**: Imagine the project has failed, work backward
- **Red teaming**: Think like an adversary trying to break assumptions
- **Six thinking hats**: Use different perspectives to challenge assumptions
- **Five whys**: Keep asking "why" to uncover root assumptions

### Analysis Techniques:
- **Fault tree analysis**: Map how assumptions can fail
- **Failure mode and effects analysis (FMEA)**: Systematic failure analysis
- **Scenario planning**: Develop multiple future scenarios that break assumptions
- **Monte Carlo simulation**: Model assumption failure probabilistically

### Testing Techniques:
- **Chaos engineering**: Deliberately inject failures to test assumptions
- **Load testing**: Push systems beyond assumed limits
- **Security testing**: Attempt to break security assumptions
- **Usability testing**: Observe real users to validate behavior assumptions

## Measurement and Metrics

### Assumption Health Metrics:
- **Assumption coverage**: Percentage of critical assumptions busted
- **Busting effectiveness**: Number of assumptions successfully busted
- **Failure mode discovery**: New failure modes identified per session
- **Impact reduction**: Estimated risk reduction from busting

### Process Metrics:
- **Time investment**: Time spent busting vs value gained
- **Team participation**: Breadth of team involvement in busting
- **Action follow-through**: Percentage of busting findings acted upon
- **Re-busting frequency**: How often assumptions are re-tested

## Integration with Other Skills

### With Devil's Advocate:
- **Devil's advocate**: Challenges reasoning and logic
- **Assumption buster**: Tries to disprove foundational assumptions
- **Combination**: Both logical challenge and empirical testing

### With Red Team:
- **Red team**: Security-focused attack simulation
- **Assumption buster**: General assumption falsification
- **Combination**: Security assumptions + general assumptions

### With Trust But Verify:
- **Trust but verify**: Independently tests claims
- **Assumption buster**: Aggressively tries to disprove assumptions
- **Combination**: Verification + falsification for comprehensive testing

### With White Hat:
- **White hat**: Builds defensive capabilities
- **Assumption buster**: Identifies where defenses might fail
- **Combination**: Defense building informed by failure understanding

## Common Pitfalls

### Over-busting:
- **Problem**: Spending excessive time on low-impact assumptions
- **Solution**: Prioritize by impact and confidence
- **Guideline**: 80/20 rule - focus on 20% of assumptions that cause 80% of risk

### Analysis Paralysis:
- **Problem**: Getting stuck in endless "what if" scenarios
- **Solution**: Time-box busting sessions
- **Guideline**: 60-90 minutes per session, with clear next actions

### Negative Culture:
- **Problem**: Busting seen as criticism rather than improvement
- **Solution**: Frame as "making our ideas stronger"
- **Guideline**: Celebrate busted assumptions as learning opportunities

### Lack of Follow-through:
- **Problem**: Findings documented but not acted upon
- **Solution**: Assign owners and deadlines for each finding
- **Guideline**: Each busting session ends with action items

## Resources

### Further Reading:
- "The Black Swan" by Nassim Taleb (rare events)
- "Thinking in Bets" by Annie Duke (decision-making under uncertainty)
- "Antifragile" by Nassim Taleb (systems that gain from disorder)
- "The Logic of Scientific Discovery" by Karl Popper (falsification)

### Tools and Templates:
- Assumption busting checklist (systematic challenge questions)
- Pre-mortem template (imagining failure to identify assumptions)
- Failure mode catalog (common assumption failure patterns)
- Risk register template (tracking busted assumptions and mitigations)

*Last updated: 2026-02-26*