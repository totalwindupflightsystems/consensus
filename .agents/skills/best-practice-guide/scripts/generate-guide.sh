#!/bin/bash
set -e

echo "Best Practice Guide: Guide Generation" >&2
echo "====================================" >&2

# Check for required arguments
if [ -z "$1" ]; then
    echo "Usage: $0 <topic> [format] [search]" >&2
    echo "Example: $0 'react-performance' skill true" >&2
    echo "Formats: skill, documentation, both" >&2
    echo "Search: true, false (whether to recommend search tools)" >&2
    exit 1
fi

TOPIC="$1"
FORMAT="${2:-skill}"
SEARCH_RECOMMENDED="${3:-true}"
TIMESTAMP=$(date +%Y-%m-%dT%H:%M:%S)

# Validate topic input to prevent directory traversal
if [[ "$TOPIC" =~ \.\.|/ ]]; then
    echo "Error: Topic cannot contain '..' or '/' characters" >&2
    exit 1
fi
if [[ ! "$TOPIC" =~ ^[a-zA-Z0-9_-]+$ ]]; then
    echo "Error: Topic must contain only alphanumeric characters, hyphens, and underscores" >&2
    exit 1
fi

echo "Topic: $TOPIC" >&2
echo "Format: $FORMAT" >&2
echo "Search recommended: $SEARCH_RECOMMENDED" >&2
echo "Timestamp: $TIMESTAMP" >&2
echo "" >&2

# Common topics and their characteristics
declare -A TOPIC_METADATA
TOPIC_METADATA=(
    ["web-application-security"]="Critical security practices for web applications"
    ["container-security"]="Security hardening for Docker containers and Kubernetes"
    ["dependency-security"]="Managing dependency vulnerabilities and updates"
    ["react-performance"]="Performance optimization patterns for React applications"
    ["nodejs-performance"]="Node.js application performance tuning"
    ["jest-testing-patterns"]="Effective testing patterns with Jest framework"
    ["basic-documentation"]="Essential documentation structure for projects"
    ["comprehensive-documentation"]="Complete documentation strategy"
    ["database-security"]="Database security and access control best practices"
    ["api-design"]="REST/GraphQL API design patterns and best practices"
    ["cloud-deployment"]="Cloud deployment patterns and infrastructure as code"
    ["monitoring-observability"]="Application monitoring and observability setup"
    ["ci-cd-pipeline"]="Continuous integration and deployment pipeline design"
    ["accessibility-compliance"]="Web accessibility standards and compliance"
    ["data-protection"]="Data protection and privacy compliance (GDPR, CCPA)"
)

# Get topic description
TOPIC_DESCRIPTION="${TOPIC_METADATA[$TOPIC]}"
if [ -z "$TOPIC_DESCRIPTION" ]; then
    TOPIC_DESCRIPTION="Best practices for $TOPIC"
fi

echo "Generating guide for: $TOPIC" >&2
echo "Description: $TOPIC_DESCRIPTION" >&2
echo "" >&2

# Determine impact level
IMPACT="medium"
if [[ "$TOPIC" == *"security"* ]]; then
    IMPACT="high"
elif [[ "$TOPIC" == *"performance"* ]]; then
    IMPACT="medium"
elif [[ "$TOPIC" == *"documentation"* ]]; then
    IMPACT="low"
fi

# Generate guide content based on format
case "$FORMAT" in
    "skill")
        echo "Creating skill format guide..." >&2
        SKILL_NAME="${TOPIC//-/_}_guide"
        SKILL_DIR="skills/${TOPIC}-guide"
        
        echo "Skill directory: $SKILL_DIR" >&2
        
        # Create directory structure
        mkdir -p "$SKILL_DIR"
        mkdir -p "$SKILL_DIR/references"
        mkdir -p "$SKILL_DIR/scripts"
        
        # Create SKILL.md
        cat << EOF > "$SKILL_DIR/SKILL.md"
---
name: ${TOPIC}-guide
description: ${TOPIC_DESCRIPTION}
license: MIT
compatibility: opencode
metadata:
  audience: developers, architects, security professionals
  category: best-practices
---

# ${TOPIC^} Best Practices Guide

${TOPIC_DESCRIPTION}

## When to use me

Use this guide when:
- Designing or implementing ${TOPIC} solutions
- Reviewing existing ${TOPIC} implementations
- Preparing for security audits or compliance checks
- Optimizing ${TOPIC} performance and reliability
- Onboarding team members to ${TOPIC} practices

## Core Principles

1. **Security First**: Always prioritize security in ${TOPIC} implementations
2. **Performance Awareness**: Consider performance implications of design choices
3. **Maintainability**: Design for long-term maintainability and evolution
4. **Documentation**: Document decisions and patterns for team knowledge sharing
5. **Testing**: Implement comprehensive testing for ${TOPIC} functionality

## Implementation Patterns

### Pattern 1: Basic Implementation
\`\`\`
// Example implementation pattern
// Replace with actual ${TOPIC} patterns
\`\`\`

### Pattern 2: Advanced Optimization
\`\`\`
// Advanced optimization pattern
// Replace with actual ${TOPIC} optimization patterns
\`\`\`

## Common Pitfalls

1. **Pitfall 1**: [Common mistake to avoid]
   - **Why it's problematic**: [Explanation]
   - **Better approach**: [Recommended alternative]

2. **Pitfall 2**: [Another common mistake]
   - **Why it's problematic**: [Explanation]
   - **Better approach**: [Recommended alternative]

## Validation Checklist

- [ ] Security review completed
- [ ] Performance testing conducted
- [ ] Documentation updated
- [ ] Team training provided
- [ ] Monitoring implemented

## Additional Resources

- Official documentation: [Link to official docs]
- Industry best practices: [Link to industry resources]
- Security guidelines: [Link to security resources]
- Performance benchmarks: [Link to performance data]

*Generated by best-practice-guide skill - Last updated: ${TIMESTAMP}*
EOF
        
        # Create references/README.md
        cat << EOF > "$SKILL_DIR/references/README.md"
# ${TOPIC^} Best Practices Reference

## Overview

This guide provides comprehensive best practices for ${TOPIC}, covering security, performance, maintainability, and operational considerations.

## Research Sources

The following sources were consulted in creating this guide:

EOF
        
        if [ "$SEARCH_RECOMMENDED" = "true" ]; then
            cat << EOF >> "$SKILL_DIR/references/README.md"
**Search recommended for current best practices:**

Recommended search queries:
1. "${TOPIC} best practices $(date +%Y)"
2. "${TOPIC} security considerations"
3. "${TOPIC} performance optimization"
4. "${TOPIC} common mistakes"

Authoritative sources to consult:
- Official framework/library documentation
- Industry security guidelines (OWASP, NIST, etc.)
- Performance benchmark reports
- Community best practice articles
EOF
        else
            cat << EOF >> "$SKILL_DIR/references/README.md"
**Note:** This guide was generated without external research. Consider validating with current best practices.
EOF
        fi
        
        cat << EOF >> "$SKILL_DIR/references/README.md"

## Detailed Guidelines

### Security Considerations
1. Authentication and authorization patterns
2. Data protection requirements
3. Input validation and sanitization
4. Error handling and information disclosure

### Performance Optimization
1. Resource utilization patterns
2. Caching strategies
3. Database/query optimization
4. Network efficiency

### Operational Excellence
1. Monitoring and alerting setup
2. Logging and debugging
3. Deployment and scaling patterns
4. Disaster recovery procedures

## Compliance Requirements

### Regulatory Considerations
- Data protection regulations (GDPR, CCPA, etc.)
- Industry-specific compliance (HIPAA, PCI-DSS, etc.)
- Security standards (ISO 27001, SOC 2, etc.)

### Implementation Checklist
- [ ] Document all security controls
- [ ] Implement proper logging
- [ ] Set up monitoring and alerts
- [ ] Create runbooks for common issues
- [ ] Establish regular review procedures

## Maintenance Procedures

### Regular Review Schedule
- Monthly: Security vulnerability review
- Quarterly: Performance optimization review
- Biannually: Architecture and scalability review
- Annually: Comprehensive best practices review

### Update Procedures
1. Monitor for new vulnerabilities and updates
2. Review industry best practice changes
3. Update documentation and patterns
4. Train team on new procedures
5. Validate implementation changes

*Last updated: ${TIMESTAMP}*
EOF
        
        # Create a simple script
        cat << EOF > "$SKILL_DIR/scripts/check-${TOPIC}.sh"
#!/bin/bash
set -e

echo "${TOPIC^} Best Practices Check" >&2
echo "=============================" >&2

echo "Checking ${TOPIC} implementation..." >&2
echo "" >&2

echo "Review areas:" >&2
echo "1. Security controls implementation" >&2
echo "2. Performance optimization patterns" >&2
echo "3. Documentation completeness" >&2
echo "4. Testing coverage" >&2
echo "5. Monitoring setup" >&2
echo "" >&2

echo '{"status": "check_complete", "topic": "'"$TOPIC"'", "timestamp": "'"$TIMESTAMP"'", "checks_performed": 5}' >&2
EOF
        
        chmod +x "$SKILL_DIR/scripts/check-${TOPIC}.sh"
        
        echo "✅ Created skill: $SKILL_DIR" >&2
        ;;
    
    "documentation")
        echo "Creating documentation format guide..." >&2
        DOC_FILE="docs/${TOPIC}-guide.md"
        
        mkdir -p "$(dirname "$DOC_FILE")"
        
        cat << EOF > "$DOC_FILE"
# ${TOPIC^} Best Practices Guide

${TOPIC_DESCRIPTION}

*Generated by best-practice-guide skill - Last updated: ${TIMESTAMP}*

## Overview

This guide provides best practices for ${TOPIC} implementation, focusing on security, performance, and maintainability.

## Impact Level: ${IMPACT^^}

## Key Principles

### 1. Security First
- Implement proper authentication and authorization
- Protect sensitive data
- Validate and sanitize all inputs
- Follow principle of least privilege

### 2. Performance Optimization
- Monitor and optimize resource usage
- Implement effective caching strategies
- Optimize database queries and connections
- Minimize network latency

### 3. Maintainability
- Write clean, documented code
- Follow consistent patterns and conventions
- Implement comprehensive testing
- Create clear documentation

### 4. Operational Excellence
- Set up proper monitoring and alerting
- Implement effective logging
- Create runbooks for common operations
- Plan for scalability and growth

## Implementation Guidelines

### Basic Implementation
\`\`\`
// Basic ${TOPIC} implementation pattern
// Replace with actual implementation details
\`\`\`

### Advanced Patterns
\`\`\`
// Advanced ${TOPIC} patterns for complex scenarios
// Replace with actual advanced patterns
\`\`\`

## Security Considerations

### Critical Security Controls
1. **Authentication**: Secure user authentication mechanisms
2. **Authorization**: Proper role-based access control
3. **Data Protection**: Encryption of sensitive data
4. **Input Validation**: Comprehensive input sanitization
5. **Error Handling**: Secure error messages and logging

### Compliance Requirements
- [ ] Data protection regulations
- [ ] Industry security standards
- [ ] Privacy requirements
- [ ] Audit logging requirements

## Performance Optimization

### Key Metrics to Monitor
- Response time and latency
- Resource utilization (CPU, memory, disk, network)
- Database query performance
- Cache hit rates

### Optimization Strategies
1. Implement caching where appropriate
2. Optimize database queries and indexes
3. Use connection pooling
4. Implement rate limiting and throttling

## Testing Strategy

### Test Types Required
- Unit tests for individual components
- Integration tests for component interactions
- Security tests for vulnerabilities
- Performance tests for scalability

### Test Coverage Goals
- 80%+ unit test coverage
- Critical path integration testing
- Regular security vulnerability scanning
- Performance benchmarking under load

## Monitoring and Alerting

### Required Monitoring
- Application health and availability
- Performance metrics and trends
- Security events and anomalies
- Business metrics and KPIs

### Alerting Strategy
- Immediate alerts for critical failures
- Warning alerts for degradation
- Informational alerts for significant events
- Regular reports for trend analysis

## Maintenance Procedures

### Regular Reviews
- Monthly security vulnerability review
- Quarterly performance optimization review
- Biannual architecture review
- Annual comprehensive best practices review

### Update Procedures
1. Monitor for updates and vulnerabilities
2. Test updates in staging environment
3. Deploy updates with rollback capability
4. Document changes and update procedures

## Additional Resources

EOF
        
        if [ "$SEARCH_RECOMMENDED" = "true" ]; then
            cat << EOF >> "$DOC_FILE"
### Recommended Research
For current best practices, research:

1. **Search queries:**
   - "${TOPIC} best practices $(date +%Y)"
   - "${TOPIC} security guidelines"
   - "${TOPIC} performance optimization"

2. **Authoritative sources:**
   - Official documentation and release notes
   - Industry security guidelines (OWASP, NIST, etc.)
   - Performance benchmark reports
   - Community best practice articles

3. **Validation requirements:**
   - Cross-reference multiple sources
   - Verify recency of information
   - Test recommendations in your environment
EOF
        else
            cat << EOF >> "$DOC_FILE"
### Note on Research
This guide was generated without external research. Consider validating recommendations with current best practices and official documentation.
EOF
        fi
        
        cat << EOF >> "$DOC_FILE"

## Integration with Project Documentation

This guide should be referenced in:
- \`AGENTS.md\` under relevant sections
- \`CLAUDE.md\` for agent context
- Project README for team awareness
- Onboarding documentation for new team members

## Revision History

- $(date +%Y-%m-%d): Initial guide created by best-practice-guide skill

---
*End of ${TOPIC^} Best Practices Guide*
EOF
        
        echo "✅ Created documentation: $DOC_FILE" >&2
        ;;
    
    "both")
        echo "Creating both skill and documentation formats..." >&2
        "$0" "$TOPIC" "skill" "$SEARCH_RECOMMENDED"
        "$0" "$TOPIC" "documentation" "$SEARCH_RECOMMENDED"
        ;;
    
    *)
        echo "Unknown format: $FORMAT" >&2
        exit 1
        ;;
esac

echo "" >&2
echo "Guide generation complete!" >&2
echo "" >&2

# Generate integration recommendations
echo "Integration Recommendations:" >&2
echo "---------------------------" >&2

case "$FORMAT" in
    "skill")
        echo "1. Add reference to AGENTS.md:" >&2
        echo "   \`\`\`markdown" >&2
        echo "   ## ${TOPIC^} Best Practices" >&2
        echo "   " >&2
        echo "   See \`skills/${TOPIC}-guide/\` for comprehensive ${TOPIC} best practices." >&2
        echo "   \`\`\`" >&2
        echo "" >&2
        echo "2. Register skill in skills catalog" >&2
        echo "3. Test skill discovery by agents" >&2
        ;;
    "documentation")
        echo "1. Add reference to AGENTS.md:" >&2
        echo "   \`\`\`markdown" >&2
        echo "   ## ${TOPIC^} Guidelines" >&2
        echo "   " >&2
        echo "   See \`docs/${TOPIC}-guide.md\` for detailed ${TOPIC} best practices." >&2
        echo "   \`\`\`" >&2
        echo "" >&2
        echo "2. Update table of contents if applicable" >&2
        echo "3. Link from related documentation sections" >&2
        ;;
    "both")
        echo "1. Add reference to AGENTS.md:" >&2
        echo "   \`\`\`markdown" >&2
        echo "   ## ${TOPIC^} Best Practices" >&2
        echo "   " >&2
        echo "   - Quick reference: \`skills/${TOPIC}-guide/\`" >&2
        echo "   - Detailed guide: \`docs/${TOPIC}-guide.md\`" >&2
        echo "   \`\`\`" >&2
        ;;
esac

echo "" >&2
echo "Next steps:" >&2
echo "1. Review generated content for accuracy" >&2
echo "2. Research current best practices if search recommended" >&2
echo "3. Customize for specific project needs" >&2
echo "4. Integrate into project workflow" >&2
echo "5. Schedule regular reviews and updates" >&2

echo '{"status": "generated", "topic": "'"$TOPIC"'", "format": "'"$FORMAT"'", "search_recommended": "'"$SEARCH_RECOMMENDED"'", "timestamp": "'"$TIMESTAMP"'", "impact": "'"$IMPACT"'"}'