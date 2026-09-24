# Reality Validation - Reference Documentation

## Overview
The **reality-validation** skill compares system behavior against real-world expectations, domain knowledge, and practical constraints rather than just technical specifications. It ensures software makes sense in actual usage contexts and solves real problems effectively.

## Core Methodology

### 1. Domain Understanding
- **Context Research**: Studying domain terminology, concepts, and relationships
- **Process Analysis**: Understanding actual business processes and workflows
- **User Research**: Learning about real user roles, responsibilities, and goals
- **Constraint Identification**: Discovering practical, regulatory, and industry constraints

### 2. Reality Gap Analysis
- **Data Comparison**: Does system data match real-world data patterns?
- **Process Validation**: Do automated workflows match manual processes?
- **Calculation Verification**: Do calculations produce realistic, useful results?
- **Timing Analysis**: Do time-based behaviors match real-world timing?
- **Scale Assessment**: Does system scale match real-world volumes and patterns?

### 3. Reality-Based Testing
- **Real-World Data Testing**: Using production data samples and historical data
- **Expert Validation**: Review by domain experts and subject matter experts
- **User Acceptance Testing**: Validation by actual users in real contexts
- **Context-Aware Validation**: Testing under realistic conditions and constraints

### 4. Practical Correctness Assessment
- **Usefulness Evaluation**: Are results practically useful for decision-making?
- **Business Sense Assessment**: Do outputs make business sense?
- **Efficiency Analysis**: Are workflows efficient in practice?
- **Impact Measurement**: Does system deliver real business value?

## Key Validation Areas

### Data & Model Validation
- **Data Patterns**: Do system data distributions match real-world distributions?
- **Entity Relationships**: Do modeled relationships reflect real relationships?
- **Data Quality**: Is data accurate, complete, and timely enough for real use?
- **Temporal Patterns**: Do time-based behaviors match real-world timing?
- **Geographic Patterns**: Do location-based behaviors make geographic sense?

### Process & Workflow Validation
- **Business Processes**: Do automated processes match manual processes in efficiency and outcome?
- **User Workflows**: Do system workflows support actual user tasks effectively?
- **Decision Points**: Do system decisions reflect real decision criteria and quality?
- **Approval Flows**: Do approval processes match organizational reality and efficiency?
- **Exception Handling**: Do exceptions reflect real exceptional situations and handling?

### Calculation & Logic Validation
- **Business Calculations**: Do calculations produce business-valid results?
- **Pricing Logic**: Does pricing match market reality and business strategy?
- **Risk Assessments**: Do risk calculations reflect actual risk factors and outcomes?
- **Forecasting**: Do forecasts align with historical patterns and expert expectations?
- **Optimization**: Do optimization results provide practical improvements?

### User Experience Validation
- **User Mental Models**: Does UI match user understanding of the domain?
- **Workflow Efficiency**: Do interfaces support efficient real work?
- **Information Presentation**: Is information presented for practical use and decision-making?
- **Decision Support**: Does system help users make better real decisions?
- **Learning Curve**: Can users apply domain knowledge to system use effectively?

## Integration with Other Testing Skills

### Complementary Skills
- **trust-but-verify**: Uses reality validation as ultimate truth benchmark
- **assumption-testing**: Validates assumptions against real-world evidence
- **testing-ecosystem**: Provides context for reality validation within overall testing

### Coordination Points
   1. Use `test-planning` to include reality validation in test strategies
   2. Leverage `test-dependency-mapper` to understand reality validation prerequisites
   3. Integrate with `testing-usability` for user-centered reality checks
   4. Coordinate with `testing-performance` for real-world performance validation

## Implementation Examples

### Domain-Specific Validation
```bash
# Healthcare domain validation
npm run reality:validate -- --domain healthcare \
  --system "patient-scheduling" \
  --data-source "production-samples" \
  --expert "clinical-director"

# Financial domain validation  
npm run reality:validate -- --domain finance \
  --system "portfolio-management" \
  --compare "historical-performance" \
  --expert "finance-director"
```

### Context-Aware Validation
```bash
# Seasonal variation testing
npm run reality:validate:context -- --season "holiday-peak" \
  --data "historical-holiday-data" \
  --workload "peak-load-patterns"

# Regulatory environment testing
npm run reality:validate:context -- --regulatory-environment "gdpr" \
  --requirements "compliance-checklist" \
  --expert "compliance-officer"
```

## Best Practices

### When to Apply Reality Validation
- **Complex real-world domains** (finance, healthcare, logistics, manufacturing)
- **Mission-critical systems** where failure has serious consequences
- **Systems replacing manual processes** to ensure automation maintains quality
- **Regulated industries** with strict compliance requirements
- **Customer-facing applications** where user satisfaction is critical

### Domain Expert Engagement
- Involve subject matter experts early and continuously
- Establish clear validation criteria based on domain knowledge
- Use real-world cases and scenarios for testing
- Document expert feedback and incorporate into validation
- Establish ongoing expert review processes

### Real-World Data Usage
- Use sanitized production data for testing when possible
- Create realistic test data based on actual patterns
- Test with historical data to validate against known outcomes
- Use real-time data feeds for dynamic validation
- Ensure data privacy and security when using real data

## Limitations and Considerations

### Domain Expertise Dependency
- Requires access to domain experts and subject matter experts
- Different experts may have different perspectives on "reality"
- Domain knowledge may be tacit and difficult to formalize
- Reality changes over time requiring ongoing validation

### Data Challenges
- Real-world data may be incomplete, inconsistent, or biased
- Production data usage requires careful privacy and security handling
- Historical data may not reflect current or future reality
- Data access may be limited by regulatory or organizational constraints

### Practical Constraints
- Reality validation can be time-consuming and resource-intensive
- Some aspects of reality are difficult to measure or test
- Balance between theoretical correctness and practical usefulness is subjective
- Different stakeholders may have different definitions of "reality"

## Related Skills
- [trust-but-verify](../trust-but-verify/references/README.md) - Skeptical verification of claims
- [assumption-testing](../assumption-testing/references/README.md) - Testing implicit assumptions
- [testing-usability](../testing-usability/references/README.md) - User experience validation
- [testing-ecosystem](../testing-ecosystem/references/README.md) - Complete testing landscape understanding

## References
- [Agent Skills Specification](https://agentskills.io/specification)
- [Domain-Driven Design](https://domainlanguage.com/ddd/)
- [Real-World Software Validation](https://en.wikipedia.org/wiki/Verification_and_validation)
- [Business Process Validation](https://www.bpmn.org/)
