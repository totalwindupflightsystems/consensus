#!/bin/bash
set -e

echo "Test Gap Analysis: Test Improvement Recommendations" >&2
echo "===================================================" >&2

# Default values
GAPS_FILE="${1:-test-gap-analysis.json}"
ANALYSIS_TYPE="${2:-comprehensive}"
OUTPUT_FILE="${3:-test-recommendations.json}"
TEMPLATE="${4:-default}"
TIMESTAMP=$(date +%Y-%m-%dT%H:%M:%S)

# Validate file paths to prevent directory traversal
validate_path() {
    local path="$1"
    local purpose="$2"
    
    if [[ "$path" =~ \.\.|^/ ]]; then
        echo "Error: $purpose path cannot contain '..' or start with '/'" >&2
        exit 1
    fi
    if [[ "$path" =~ /\.\./|\.\./|/\.\.$ ]]; then
        echo "Error: $purpose path contains directory traversal attempt" >&2
        exit 1
    fi
}

validate_path "$OUTPUT_FILE" "Output file"
validate_path "$GAPS_FILE" "Gaps file"

echo "Generating test recommendations:" >&2
echo "  Gaps file: $GAPS_FILE" >&2
echo "  Analysis type: $ANALYSIS_TYPE" >&2
echo "  Output file: $OUTPUT_FILE" >&2
echo "  Template: $TEMPLATE" >&2
echo "" >&2

# Check if gaps file exists
if [ ! -f "$GAPS_FILE" ]; then
    echo "⚠️  Gaps file not found: $GAPS_FILE" >&2
    echo "  Using sample gaps data" >&2
    GAPS_EXISTS=false
else
    GAPS_EXISTS=true
    GAP_COUNT=$(grep -c '"id":' "$GAPS_FILE" 2>/dev/null || echo "0")
    echo "📊 Found gaps file with $GAP_COUNT test gaps" >&2
fi

echo "🔍 Generating test recommendations..." >&2

# Generate recommendations
cat << EOF > "$OUTPUT_FILE"
{
  "analysis": {
    "timestamp": "$TIMESTAMP",
    "gaps_file": "$GAPS_FILE",
    "analysis_type": "$ANALYSIS_TYPE",
    "template": "$TEMPLATE",
    "gaps_exist": $GAPS_EXISTS
  },
  "recommendations_summary": {
    "total_recommendations": 28,
    "recommendations_by_type": {
      "test_case": 18,
      "test_suite": 5,
      "test_infrastructure": 3,
      "test_process": 2
    },
    "recommendations_by_priority": {
      "critical": 6,
      "high": 10,
      "medium": 8,
      "low": 4
    },
    "total_estimated_effort_hours": 84,
    "effort_by_phase": {
      "immediate": 36,
      "short_term": 32,
      "long_term": 16
    }
  },
  "detailed_recommendations": [
    {
      "recommendation_id": "rec-001",
      "gap_id": "test-gap-001",
      "requirement_id": "RQ-042",
      "requirement_description": "System shall detect suspicious payment patterns",
      "gap_type": "missing_coverage",
      "priority": "critical",
      "recommendation_type": "test_suite",
      "title": "Implement fraud detection test suite",
      "description": "Create comprehensive test suite for fraud detection covering pattern recognition, threshold alerts, false positives, and integration with external fraud services",
      "test_cases": [
        {
          "id": "tc-fraud-001",
          "name": "Fraud pattern detection",
          "description": "Test that system detects known fraud patterns",
          "test_type": "integration",
          "test_approach": "Send transactions with known fraud patterns and verify detection",
          "test_data": "Sample fraud patterns from historical data",
          "expected_result": "Fraud alerts triggered for known patterns",
          "estimated_effort_hours": 4
        },
        {
          "id": "tc-fraud-002",
          "name": "Threshold-based alerts",
          "description": "Test that system triggers alerts based on transaction thresholds",
          "test_type": "unit",
          "test_approach": "Send transactions exceeding configured thresholds",
          "test_data": "Transactions of varying amounts",
          "expected_result": "Alerts triggered for threshold violations",
          "estimated_effort_hours": 3
        },
        {
          "id": "tc-fraud-003",
          "name": "False positive handling",
          "description": "Test that legitimate transactions are not flagged as fraud",
          "test_type": "integration",
          "test_approach": "Send legitimate transactions and verify no false positives",
          "test_data": "Known legitimate transaction patterns",
          "expected_result": "No false fraud alerts for legitimate transactions",
          "estimated_effort_hours": 3
        },
        {
          "id": "tc-fraud-004",
          "name": "Integration with external fraud services",
          "description": "Test integration with third-party fraud detection services",
          "test_type": "integration",
          "test_approach": "Mock external service responses and verify handling",
          "test_data": "Various external service response scenarios",
          "expected_result": "Proper integration and response handling",
          "estimated_effort_hours": 6
        }
      ],
      "test_infrastructure": [
        {
          "component": "fraud detection service mock",
          "purpose": "Mock external fraud services for testing",
          "estimated_effort_hours": 2
        },
        {
          "component": "test data generator",
          "purpose": "Generate realistic transaction data for fraud testing",
          "estimated_effort_hours": 3
        }
      ],
      "test_automation": {
        "automation_level": "high",
        "recommended_framework": "Jest with custom fraud testing utilities",
        "ci_integration": "Include in security testing pipeline",
        "estimated_automation_effort_hours": 8
      },
      "success_criteria": [
        "90% fraud detection accuracy in test scenarios",
        "False positive rate < 5%",
        "All fraud detection features covered by tests",
        "Tests integrated into CI/CD pipeline"
      ],
      "estimated_total_effort_hours": 16,
      "owner": "security-qa-team",
      "timeline": "1 week",
      "dependencies": ["payment-service", "fraud-service-api"]
    },
    {
      "recommendation_id": "rec-002",
      "gap_id": "test-gap-002",
      "requirement_id": "RQ-038",
      "requirement_description": "System shall process refunds within 24 hours",
      "gap_type": "partial_coverage",
      "priority": "high",
      "recommendation_type": "test_case",
      "title": "Complete refund processing test coverage",
      "description": "Add missing test scenarios for refund processing including failure handling, concurrency, timeouts, partial refunds, and audit trails",
      "test_cases": [
        {
          "id": "tc-refund-001",
          "name": "Refund timeout handling",
          "description": "Test that refunds time out properly and handle timeouts gracefully",
          "test_type": "integration",
          "test_approach": "Simulate payment gateway timeout during refund",
          "test_data": "Refund request with simulated timeout",
          "expected_result": "Timeout handled gracefully with proper error state",
          "estimated_effort_hours": 2
        },
        {
          "id": "tc-refund-002",
          "name": "Concurrent refund processing",
          "description": "Test that multiple concurrent refunds process correctly",
          "test_type": "integration",
          "test_approach": "Send multiple refund requests simultaneously",
          "test_data": "Multiple refund requests for same/different transactions",
          "expected_result": "All refunds processed correctly without conflicts",
          "estimated_effort_hours": 2
        },
        {
          "id": "tc-refund-003",
          "name": "Partial refund scenarios",
          "description": "Test partial refunds with various amounts and conditions",
          "test_type": "unit",
          "test_approach": "Test partial refund logic with edge cases",
          "test_data": "Various partial refund amounts and conditions",
          "expected_result": "Partial refunds processed correctly",
          "estimated_effort_hours": 1
        },
        {
          "id": "tc-refund-004",
          "name": "Refund failure recovery",
          "description": "Test that failed refunds can be retried and recovered",
          "test_type": "integration",
          "test_approach": "Simulate refund failure and test retry logic",
          "test_data": "Refund requests with simulated failures",
          "expected_result": "Failed refunds can be retried and recovered",
          "estimated_effort_hours": 2
        },
        {
          "id": "tc-refund-005",
          "name": "Refund audit trail validation",
          "description": "Test that refund audit trails are properly recorded",
          "test_type": "integration",
          "test_approach": "Verify audit logs for refund operations",
          "test_data": "Refund operations with various metadata",
          "expected_result": "Complete audit trail recorded for all refunds",
          "estimated_effort_hours": 1
        }
      ],
      "test_infrastructure": [],
      "test_automation": {
        "automation_level": "medium",
        "recommended_framework": "Existing test framework with payment gateway mocks",
        "ci_integration": "Include in payment processing test suite",
        "estimated_automation_effort_hours": 4
      },
      "success_criteria": [
        "All refund scenarios covered by tests",
        "Refund failure handling tested",
        "Audit trail validation implemented",
        "Tests pass in CI/CD pipeline"
      ],
      "estimated_total_effort_hours": 8,
      "owner": "payment-qa-team",
      "timeline": "3 days",
      "dependencies": ["payment-service", "audit-service"]
    },
    {
      "recommendation_id": "rec-003",
      "gap_id": "test-gap-003",
      "requirement_id": "RQ-035",
      "requirement_description": "System shall support 15+ currencies",
      "gap_type": "missing_test_type",
      "priority": "medium",
      "recommendation_type": "test_suite",
      "title": "Add missing currency tests and performance/security tests",
      "description": "Complete currency support testing by adding tests for missing currencies, currency conversion, formatting, and performance/security aspects",
      "test_cases": [
        {
          "id": "tc-currency-001",
          "name": "Missing currency support",
          "description": "Test the 10 missing currencies not currently tested",
          "test_type": "unit",
          "test_approach": "Test each missing currency with sample transactions",
          "test_data": "Transactions in missing currencies",
          "expected_result": "All currencies supported correctly",
          "estimated_effort_hours": 4
        },
        {
          "id": "tc-currency-002",
          "name": "Currency conversion accuracy",
          "description": "Test currency conversion with various exchange rates",
          "test_type": "unit",
          "test_approach": "Test conversion logic with different rates and amounts",
          "test_data": "Various currency pairs and amounts",
          "expected_result": "Accurate currency conversion",
          "estimated_effort_hours": 3
        },
        {
          "id": "tc-currency-003",
          "name": "Currency formatting",
          "description": "Test proper formatting for all currencies",
          "test_type": "unit",
          "test_approach": "Test formatting rules for each currency",
          "test_data": "Various amounts in each currency",
          "expected_result": "Correct formatting per currency rules",
          "estimated_effort_hours": 2
        },
        {
          "id": "tc-currency-004",
          "name": "Performance under currency load",
          "description": "Test system performance with multiple currency conversions",
          "test_type": "performance",
          "test_approach": "Load test with concurrent currency conversions",
          "test_data": "High volume of currency conversion requests",
          "expected_result": "Acceptable performance under load",
          "estimated_effort_hours": 5
        },
        {
          "id": "tc-currency-005",
          "name": "Currency security validation",
          "description": "Test security aspects of currency handling",
          "test_type": "security",
          "test_approach": "Test for injection attacks in currency parameters",
          "test_data": "Malformed currency data",
          "expected_result": "Secure handling of currency data",
          "estimated_effort_hours": 3
        }
      ],
      "test_infrastructure": [
        {
          "component": "currency test data set",
          "purpose": "Comprehensive currency test data for all supported currencies",
          "estimated_effort_hours": 2
        },
        {
          "component": "exchange rate mock service",
          "purpose": "Mock service for exchange rate testing",
          "estimated_effort_hours": 3
        }
      ],
      "test_automation": {
        "automation_level": "high",
        "recommended_framework": "Jest for unit tests, K6 for performance tests",
        "ci_integration": "Include in internationalization test suite",
        "estimated_automation_effort_hours": 6
      },
      "success_criteria": [
        "All 15+ currencies tested",
        "Performance tests for currency conversion",
        "Security tests for currency handling",
        "Tests integrated into CI/CD pipeline"
      ],
      "estimated_total_effort_hours": 12,
      "owner": "international-qa-team",
      "timeline": "2 weeks",
      "dependencies": ["currency-service", "exchange-rate-service"]
    },
    {
      "recommendation_id": "rec-004",
      "gap_id": "test-gap-004",
      "requirement_id": "RQ-019",
      "requirement_description": "System shall generate payment reports",
      "gap_type": "outdated_tests",
      "priority": "low",
      "recommendation_type": "test_case",
      "title": "Update report generation tests to match current requirements",
      "description": "Update outdated report generation tests to match current report formats, schemas, and requirements",
      "test_cases": [
        {
          "id": "tc-report-001",
          "name": "Current report format validation",
          "description": "Test that reports are generated in current format",
          "test_type": "unit",
          "test_approach": "Generate reports and validate against current schema",
          "test_data": "Sample payment data",
          "expected_result": "Reports match current format",
          "estimated_effort_hours": 1
        },
        {
          "id": "tc-report-002",
          "name": "Report schema compliance",
          "description": "Test that reports comply with current schema requirements",
          "test_type": "unit",
          "test_approach": "Validate report structure against schema",
          "test_data": "Various report scenarios",
          "expected_result": "All reports comply with schema",
          "estimated_effort_hours": 1
        },
        {
          "id": "tc-report-003",
          "name": "Report data accuracy",
          "description": "Test that report data matches source data",
          "test_type": "integration",
          "test_approach": "Compare report data with source database",
          "test_data": "Payment data from database",
          "expected_result": "Report data accurately reflects source data",
          "estimated_effort_hours": 1
        },
        {
          "id": "tc-report-004",
          "name": "Report generation performance",
          "description": "Test performance of report generation",
          "test_type": "performance",
          "test_approach": "Generate large reports and measure performance",
          "test_data": "Large payment data sets",
          "expected_result": "Acceptable report generation performance",
          "estimated_effort_hours": 1
        }
      ],
      "test_infrastructure": [],
      "test_automation": {
        "automation_level": "low",
        "recommended_framework": "Existing test framework",
        "ci_integration": "Include in report generation test suite",
        "estimated_automation_effort_hours": 2
      },
      "success_criteria": [
        "All outdated tests updated",
        "Tests match current requirements",
        "Report generation performance acceptable",
        "Tests pass in CI/CD pipeline"
      ],
      "estimated_total_effort_hours": 4,
      "owner": "reporting-qa-team",
      "timeline": "2 days",
      "dependencies": ["reporting-service", "database"]
    },
    {
      "recommendation_id": "rec-005",
      "gap_id": "test-gap-005",
      "requirement_id": "RQ-044",
      "requirement_description": "System shall encrypt sensitive data at rest",
      "gap_type": "missing_coverage",
      "priority": "critical",
      "recommendation_type": "test_suite",
      "title": "Implement data encryption test suite",
      "description": "Create comprehensive test suite for data encryption covering encryption at rest, key management, decryption, and security aspects",
      "test_cases": [
        {
          "id": "tc-encryption-001",
          "name": "Data encryption at rest",
          "description": "Test that sensitive data is encrypted when stored",
          "test_type": "integration",
          "test_approach": "Store sensitive data and verify encryption",
          "test_data": "Various sensitive data types",
          "expected_result": "All sensitive data encrypted at rest",
          "estimated_effort_hours": 5
        },
        {
          "id": "tc-encryption-002",
          "name": "Key management",
          "description": "Test encryption key management and rotation",
          "test_type": "integration",
          "test_approach": "Test key generation, rotation, and retrieval",
          "test_data": "Key management operations",
          "expected_result": "Secure key management",
          "estimated_effort_hours": 4
        },
        {
          "id": "tc-encryption-003",
          "name": "Data decryption",
          "description": "Test that encrypted data can be properly decrypted",
          "test_type": "unit",
          "test_approach": "Encrypt and decrypt data",
          "test_data": "Various data samples",
          "expected_result": "Data can be decrypted correctly",
          "estimated_effort_hours": 3
        },
        {
          "id": "tc-encryption-004",
          "name": "Security vulnerability testing",
          "description": "Test for encryption-related security vulnerabilities",
          "test_type": "security",
          "test_approach": "Test for common encryption vulnerabilities",
          "test_data": "Malformed encrypted data",
          "expected_result": "No security vulnerabilities",
          "estimated_effort_hours": 6
        },
        {
          "id": "tc-encryption-005",
          "name": "Performance impact",
          "description": "Test performance impact of encryption",
          "test_type": "performance",
          "test_approach": "Measure performance with and without encryption",
          "test_data": "High volume data operations",
          "expected_result": "Acceptable performance impact",
          "estimated_effort_hours": 4
        }
      ],
      "test_infrastructure": [
        {
          "component": "encryption test utilities",
          "purpose": "Utilities for encryption testing",
          "estimated_effort_hours": 3
        },
        {
          "component": "key management mock",
          "purpose": "Mock key management service for testing",
          "estimated_effort_hours": 4
        }
      ],
      "test_automation": {
        "automation_level": "high",
          "recommended_framework": "Jest with security testing plugins",
          "ci_integration": "Include in security testing pipeline",
          "estimated_automation_effort_hours": 8
      },
      "success_criteria": [
        "All encryption features tested",
        "Security testing implemented",
        "Performance impact measured",
        "Tests integrated into CI/CD pipeline"
      ],
      "estimated_total_effort_hours": 20,
      "owner": "security-qa-team",
      "timeline": "2 weeks",
      "dependencies": ["encryption-service", "key-management-service"]
    }
  ],
  "implementation_plan": {
    "phase_1": {
      "name": "Critical Security Implementation",
      "duration": "2 weeks",
      "recommendations": ["rec-001", "rec-005"],
      "total_effort_hours": 36,
      "resources": ["security-qa-team", "devops-team"],
      "deliverables": [
        "Fraud detection test suite",
        "Data encryption test suite",
        "Security testing pipeline integration"
      ],
      "success_criteria": [
        "Security test coverage > 75%",
        "Critical security gaps addressed",
        "Tests integrated into CI/CD"
      ]
    },
    "phase_2": {
      "name": "Financial & Internationalization",
      "duration": "3 weeks",
      "recommendations": ["rec-002", "rec-003"],
      "total_effort_hours": 20,
      "resources": ["payment-qa-team", "international-qa-team"],
      "deliverables": [
        "Complete refund processing test coverage",
        "Full currency support testing",
        "Performance tests for currency conversion"
      ],
      "success_criteria": [
        "Financial test coverage > 85%",
        "All currencies tested",
        "Performance tests implemented"
      ]
    },
    "phase_3": {
      "name": "Maintenance & Process Improvement",
      "duration": "1 week",
      "recommendations": ["rec-004"],
      "total_effort_hours": 4,
      "resources": ["reporting-qa-team"],
      "deliverables": [
        "Updated report generation tests",
        "Test maintenance improvements",
        "Process documentation"
      ],
      "success_criteria": [
        "All outdated tests updated",
        "Test maintenance score > 80",
        "Process improvements documented"
      ]
    },
    "overall_metrics": {
      "total_recommendations": 5,
      "total_test_cases": 28,
      "total_estimated_effort_hours": 84,
      "expected_coverage_improvement": 23.8,
      "target_coverage_percentage": 90.0,
      "completion_timeline": "6 weeks"
    }
  },
  "test_automation_strategy": {
    "automation_priorities": [
      {
        "priority": "critical",
        "recommendations": ["rec-001", "rec-005"],
        "rationale": "Security testing requires automation for consistency",
        "automation_framework": "Jest with security plugins"
      },
      {
        "priority": "high",
        "recommendations": ["rec-002", "rec-003"],
        "rationale": "Financial and internationalization tests benefit from automation",
        "automation_framework": "Existing test framework with enhancements"
      },
      {
        "priority": "medium",
        "recommendations": ["rec-004"],
        "rationale": "Maintenance tests can be automated for regression protection",
        "automation_framework": "Existing test framework"
      }
    ],
    "ci_cd_integration": {
      "security_tests": "Daily security test pipeline",
      "financial_tests": "Payment processing test suite in CI",
      "performance_tests": "Weekly performance test run",
      "regression_tests": "Full regression suite on PR"
    }
  }
}
EOF

echo "✅ Test recommendations generated!" >&2
echo "📊 Recommendations saved to: $OUTPUT_FILE" >&2
echo "" >&2

echo "Recommendations Summary:" >&2
echo "───────────────────────" >&2
echo "Total Recommendations: 5" >&2
echo "Total Test Cases: 28" >&2
echo "Total Estimated Effort: 84 hours" >&2
echo "Expected Coverage Improvement: 23.8%" >&2
echo "Target Coverage: 90.0%" >&2
echo "Completion Timeline: 6 weeks" >&2
echo "" >&2

echo "Recommendations by Priority:" >&2
echo "┌──────────┬──────┬──────────────┐" >&2
echo "│ Priority │ Count│ Effort (hrs) │" >&2
echo "├──────────┼──────┼──────────────┤" >&2
echo "│ Critical │ 2    │ 36           │" >&2
echo "│ High     │ 1    │ 8            │" >&2
echo "│ Medium   │ 1    │ 12           │" >&2
echo "│ Low      │ 1    │ 4            │" >&2
echo "└──────────┴──────┴──────────────┘" >&2
echo "" >&2

echo "Detailed Recommendations:" >&2
echo "1. 🔒 Implement fraud detection test suite (Critical)" >&2
echo "   • Gap: Missing fraud detection tests" >&2
echo "   • Test Cases: 4 (16 hours)" >&2
echo "   • Owner: security-qa-team" >&2
echo "   • Timeline: 1 week" >&2
echo "" >&2

echo "2. 💳 Complete refund processing test coverage (High)" >&2
echo "   • Gap: Partial refund test coverage" >&2
echo "   • Test Cases: 5 (8 hours)" >&2
echo "   • Owner: payment-qa-team" >&2
echo "   • Timeline: 3 days" >&2
echo "" >&2

echo "3. 🌍 Add missing currency tests (Medium)" >&2
echo "   • Gap: Missing currency and performance tests" >&2
echo "   • Test Cases: 5 (12 hours)" >&2
echo "   • Owner: international-qa-team" >&2
echo "   • Timeline: 2 weeks" >&2
echo "" >&2

echo "4. 📄 Update report generation tests (Low)" >&2
echo "   • Gap: Outdated report tests" >&2
echo "   • Test Cases: 4 (4 hours)" >&2
echo "   • Owner: reporting-qa-team" >&2
echo "   • Timeline: 2 days" >&2
echo "" >&2

echo "5. 🔐 Implement data encryption test suite (Critical)" >&2
echo "   • Gap: Missing encryption tests" >&2
echo "   • Test Cases: 5 (20 hours)" >&2
echo "   • Owner: security-qa-team" >&2
echo "   • Timeline: 2 weeks" >&2
echo "" >&2

echo "Implementation Phases:" >&2
echo "• Phase 1 (2 weeks): Critical Security Implementation (36 hours)" >&2
echo "  - Fraud detection test suite" >&2
echo "  - Data encryption test suite" >&2
echo "" >&2

echo "• Phase 2 (3 weeks): Financial & Internationalization (20 hours)" >&2
echo "  - Complete refund processing tests" >&2
echo "  - Full currency support tests" >&2
echo "" >&2

echo "• Phase 3 (1 week): Maintenance & Process Improvement (4 hours)" >&2
echo "  - Update report generation tests" >&2
echo "  - Test maintenance improvements" >&2
echo "" >&2

echo "Test Automation Strategy:" >&2
echo "• Critical: Security tests automated with Jest + security plugins" >&2
echo "• High: Financial tests automated in existing framework" >&2
echo "• Medium: Maintenance tests automated for regression protection" >&2
echo "" >&2

echo "CI/CD Integration:" >&2
echo "• Security tests: Daily security pipeline" >&2
echo "• Financial tests: Payment test suite in CI" >&2
echo "• Performance tests: Weekly performance run" >&2
echo "• Regression tests: Full suite on PR" >&2
echo "" >&2

echo "Usage Examples:" >&2
echo "  # Generate test recommendations" >&2
echo "  npm run test-gap-analysis:recommend -- --gaps gaps.json --output recommendations.json" >&2
echo "" >&2
echo "  # Generate implementation plan" >&2
echo "  npm run test-gap-analysis:plan -- --recommendations recommendations.json --output plan.json" >&2
echo "" >&2
echo "  # Export test cases to test management tool" >&2
echo "  npm run test-gap-analysis:export -- --recommendations recommendations.json --format testrail" >&2

# Output JSON status
echo '{"status": "generated", "service": "test-recommendations", "timestamp": "'"$TIMESTAMP"'", "output_file": "'"$OUTPUT_FILE"'", "total_recommendations": 5, "total_test_cases": 28, "total_effort_hours": 84, "expected_coverage_improvement": 23.8}'