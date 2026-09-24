# Assumption Testing - Reference Documentation

## Overview
The **assumption-testing** skill identifies, documents, and explicitly tests implicit assumptions in code, tests, design, and documentation. It prevents failures caused by untested beliefs about how the system should work by making assumptions explicit and validating them systematically.

## Core Methodology

### 1. Assumption Discovery
- **Code Analysis**: Scanning for implicit assumptions about data, dependencies, performance, and error handling
- **Test Review**: Analyzing tests for hidden assumptions about data, environments, and execution
- **Design Examination**: Reviewing architecture and documentation for unstated assumptions
- **Stakeholder Interview**: Gathering assumptions from domain experts and users

### 2. Assumption Documentation
- **Assumption Registry**: Creating a catalog of identified assumptions
- **Categorization**: By type (technical, business, user experience, operational)
- **Risk Assessment**: Prioritizing based on impact and likelihood of being wrong
- **Evidence Tracking**: Documenting sources and supporting evidence for each assumption

### 3. Assumption Validation
- **Explicit Testing**: Designing tests that directly validate assumptions
- **Boundary Testing**: Testing at assumption boundaries and beyond
- **Negation Testing**: Testing what happens when assumptions are false
- **Reality Comparison**: Comparing assumptions against real-world data and behavior

### 4. Assumption Management
- **Status Tracking**: Monitoring validation status and staleness
- **Communication**: Sharing assumption catalog and validation results
- **Continuous Review**: Periodically re-evaluating assumptions as systems evolve

## Common Assumption Types

### Technical Assumptions
- **Availability Assumptions**: "The database will always be available"
- **Performance Assumptions**: "Network latency will never exceed 100ms"
- **Data Assumptions**: "API responses will match the documented schema"
- **Resource Assumptions**: "Memory will always be sufficient for operations"

### Business Logic Assumptions
- **User Behavior**: "Users will always enter valid data"
- **Business Rules**: "Tax calculations follow current regulations"
- **Market Conditions**: "Currency conversion rates will be available"
- **Operational Constraints**: "Business hours are 9-5 Monday-Friday"

### User Experience Assumptions
- **User Understanding**: "Users understand how to use this feature"
- **Access Needs**: "Accessibility requirements are being met"
- **Workflow Preferences**: "Users prefer this workflow over alternatives"
- **Device Capabilities**: "Mobile users have stable internet connections"

### Operational Assumptions
- **Monitoring Coverage**: "Monitoring will alert us of failures"
- **Recovery Capabilities**: "Backups are complete and restorable"
- **Scaling Assumptions**: "Scaling will handle peak traffic"
- **Security Assumptions**: "Security patches are applied promptly"

## Integration with Other Testing Skills

### Complementary Skills
- **trust-but-verify**: Uses assumption testing as foundation for skeptical verification
- **reality-validation**: Compares assumptions against real-world expectations
- **testing-ecosystem**: Provides context for assumption testing within overall strategy

### Coordination Points
   1. Use `test-planning` to include assumption testing in test strategies
   2. Leverage `test-dependency-mapper` to understand assumption dependencies
   3. Integrate with `test-coverage` to measure assumption validation completeness
   4. Coordinate with `test-orchestrator` to schedule assumption testing appropriately

## Implementation Examples

### Basic Assumption Testing Workflow
```bash
# Discover assumptions in payment processing module
npm run assumption:discover -- --path src/payment/ \
  --types technical,business-logic \
  --output assumptions-payment.json

# Test high-risk assumptions
npm run assumption:test -- --priority high \
  --methods boundary-testing,negation-testing \
  --environment staging
```

### Assumption Registry Management
```yaml
# Example assumption registry entry
- assumption: "payment-gateway-api-always-returns-within-2-seconds"
  location: "src/payment/gateway.js:45-78"
  type: "technical"
  risk: "critical"
  source: "initial-implementation"
  last_validated: "2024-01-15"
  validation_status: "failed"
  impact: "payment-timeouts-cause-failed-transactions"
```

## Best Practices

### When to Apply Assumption Testing
- **New feature development** with uncertain requirements
- **Legacy system maintenance** with undocumented behavior
- **Third-party integration** with limited documentation
- **Complex algorithm implementation** with many edge cases
- **Production deployment preparation** for critical systems

### Assumption Documentation Standards
- Use consistent format for assumption registry entries
- Link assumptions to specific code locations
- Document sources and rationale for each assumption
- Track validation history and evidence
- Share assumption catalog across development team

### Risk-Based Prioritization
- Start with assumptions that have highest impact if wrong
- Focus on assumptions that are least supported by evidence
- Prioritize assumptions that are foundational to system design
- Revisit assumptions periodically as context changes

## Limitations and Considerations

### Discovery Challenges
- Some assumptions are deeply implicit and difficult to identify
- Assumptions may be cultural or domain-specific and not obvious to outsiders
- Complete assumption discovery is theoretically impossible
- Assumptions evolve over time and require ongoing maintenance

### Validation Complexity
- Some assumptions are difficult or expensive to test directly
- Validation may require domain expertise beyond technical testing capabilities
- Testing assumptions about user behavior requires user research
- Some operational assumptions can only be validated in production

### Resource Management
- Assumption testing adds overhead to development process
- Balance thoroughness with practical constraints
- Use automation for repeatable assumption validation
- Focus manual effort on highest-risk assumptions

## Related Skills
- [trust-but-verify](../trust-but-verify/references/README.md) - Skeptical verification of claims
- [reality-validation](../reality-validation/references/README.md) - Real-world behavior validation
- [testing-ecosystem](../testing-ecosystem/references/README.md) - Complete testing landscape understanding
- [test-planning](../test-planning/references/README.md) - Comprehensive test strategy development

## References
- [Agent Skills Specification](https://agentskills.io/specification)
- [Implicit Assumptions in Software Development](https://en.wikipedia.org/wiki/Assumption_(software))
- [Risk-Based Testing Approaches](https://www.istqb.org/)
- [Domain-Driven Design and Assumptions](https://domainlanguage.com/ddd/)
