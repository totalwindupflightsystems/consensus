# Trust But Verify - Reference Documentation

## Overview
The **trust-but-verify** skill implements skeptical verification of system claims, test results, and assumptions through independent validation rather than blind acceptance of defaults. This skill prevents false confidence by actively challenging claims and accelerates issue detection by finding discrepancies early.

## Core Methodology

### 1. Claim Identification
- **Sources**: Test results, documentation, memory systems, stakeholder reports, deployment logs
- **Categorization**: By criticality, verifiability, source credibility, and time since last verification
- **Prioritization**: Risk-based approach focusing on high-impact claims first

### 2. Verification Strategy Design
- **Independent Validation**: Using different methods, data, and environments than original tests
- **Multi-Method Approach**: Combining technical testing with expert review and real-world validation
- **Context Variation**: Testing in different environments, with different data sets, and under different conditions

### 3. Skeptical Verification Execution
- **Assumption Challenging**: Deliberately questioning what could be wrong with the claim
- **Boundary Testing**: Pushing limits and testing edge cases
- **Reality Checking**: Comparing against domain knowledge and practical constraints

### 4. Discrepancy Detection & Reporting
- **Comparison Analysis**: Identifying false positives and false negatives
- **Impact Assessment**: Evaluating business and technical impact of discrepancies
- **Actionable Insights**: Providing specific recommendations for improvement

## Integration with Other Testing Skills

### Complementary Skills
- **assumption-testing**: Tests implicit assumptions that underlie claims
- **reality-validation**: Compares system behavior against real-world expectations
- **testing-ecosystem**: Provides context for how verification fits into overall testing strategy

### Coordination Points
   1. Use `test-orchestrator` to schedule verification after initial testing
   2. Leverage `test-dependency-mapper` to understand verification prerequisites
   3. Integrate with `test-coverage` to measure verification completeness
   4. Coordinate with `test-planning` to include verification in test strategies

## Implementation Examples

### Basic Verification Workflow
```bash
# Verify specific claim about payment processing
npm run verify:claim -- --claim "payment-processing-tests-pass" \
  --source "ci-report" \
  --verification-methods "independent-testing,load-testing,expert-review"

# Comprehensive verification of critical claims
npm run verify:critical-claims -- --environment staging \
  --data-source production-samples \
  --expert finance-director
```

### CI/CD Integration
```yaml
# Example GitHub Actions workflow
- name: Trust but verify
  run: |
    npm run verify:test-results -- --test-suite "${{ github.event_name }}"
    npm run verify:deployment -- --environment "${{ env.ENVIRONMENT }}"
```

## Best Practices

### When to Apply Trust But Verify
- **Before production releases** of critical features
- **After major refactoring** or architectural changes
- **When integrating** with new third-party services
- **For mission-critical systems** where failure has high impact
- **When onboarding** to unfamiliar codebases or systems

### Verification Frequency
- **High-risk claims**: Verify before each deployment
- **Medium-risk claims**: Verify weekly or per sprint
- **Low-risk claims**: Verify monthly or per release
- **Stale claims**: Verify if not validated within defined time period

### Resource Allocation
- Allocate 10-20% of testing budget to verification activities
- Balance verification effort with claim criticality
- Use automation for repeatable verification, manual for exploratory

## Limitations and Considerations

### Technical Limitations
- Verification cannot prove complete absence of issues
- Some claims are inherently difficult to verify independently
- Verification adds time to development and deployment cycles
- Resource-intensive for comprehensive verification

### Practical Considerations
- Balance skepticism with practicality
- Document verification methods and results for auditability
- Share verification findings transparently with stakeholders
- Use verification insights to improve original testing processes

### Risk Management
- Focus verification on highest-risk areas first
- Implement progressive verification based on system maturity
- Use verification to build confidence, not just find faults
- Consider business impact when prioritizing verification efforts

## Related Skills
- [assumption-testing](../assumption-testing/references/README.md) - Testing implicit assumptions
- [reality-validation](../reality-validation/references/README.md) - Real-world behavior validation
- [testing-ecosystem](../testing-ecosystem/references/README.md) - Complete testing landscape understanding
- [test-orchestrator](../test-orchestrator/references/README.md) - Test execution coordination

## References
- [Agent Skills Specification](https://agentskills.io/specification)
- [Skeptical Verification Patterns](https://en.wikipedia.org/wiki/Trust,_but_verify)
- [Testing Pyramid and Verification](https://martinfowler.com/articles/practical-test-pyramid.html)
