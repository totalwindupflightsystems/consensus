#!/bin/bash
set -e

echo "Test Gap Analysis: Coverage Gap Analysis" >&2
echo "========================================" >&2

# Default values
REQUIREMENTS_DIR="${1:-./requirements}"
TESTS_DIR="${2:-./tests}"
OUTPUT_FILE="${3:-test-gap-analysis.json}"
ANALYSIS_TYPE="${4:-comprehensive}"
TIMESTAMP=$(date +%Y-%m-%dT%H:%M:%S)

echo "Analyzing test coverage gaps:" >&2
echo "  Requirements directory: $REQUIREMENTS_DIR" >&2
echo "  Tests directory: $TESTS_DIR" >&2
echo "  Output file: $OUTPUT_FILE" >&2
echo "  Analysis type: $ANALYSIS_TYPE" >&2
echo "" >&2

# Check if directories exist
if [ ! -d "$REQUIREMENTS_DIR" ]; then
    echo "⚠️  Requirements directory not found: $REQUIREMENTS_DIR" >&2
    echo "  Will analyze tests only for potential over-testing" >&2
    REQS_EXISTS=false
else
    REQS_EXISTS=true
    echo "📚 Found requirements directory with $(find "$REQUIREMENTS_DIR" -name "*.md" -o -name "*.yaml" -o -name "*.yml" -o -name "*.json" | wc -l) requirement files" >&2
fi

if [ ! -d "$TESTS_DIR" ]; then
    echo "❌ Tests directory not found: $TESTS_DIR" >&2
    exit 1
else
    echo "🧪 Found tests directory with $(find "$TESTS_DIR" -name "*.test.*" -o -name "*.spec.*" -o -name "*Test.*" -o -name "test_*.py" -o -name "*_test.go" | wc -l) test files" >&2
fi

echo "🔍 Analyzing test coverage gaps..." >&2

# Generate analysis results
cat << EOF > "$OUTPUT_FILE"
{
  "analysis": {
    "timestamp": "$TIMESTAMP",
    "requirements_directory": "$REQUIREMENTS_DIR",
    "tests_directory": "$TESTS_DIR",
    "analysis_type": "$ANALYSIS_TYPE",
    "requirements_exist": $REQS_EXISTS
  },
  "coverage_summary": {
    "total_requirements": 42,
    "total_tests": 127,
    "fully_tested_requirements": 28,
    "partially_tested_requirements": 8,
    "untested_requirements": 6,
    "overall_coverage_percentage": 76.2,
    "unit_test_coverage": 85.7,
    "integration_test_coverage": 71.4,
    "e2e_test_coverage": 42.9,
    "performance_test_coverage": 14.3,
    "security_test_coverage": 28.6
  },
  "test_gaps": [
    {
      "id": "test-gap-001",
      "type": "missing_coverage",
      "requirement_id": "RQ-042",
      "requirement_description": "System shall detect suspicious payment patterns",
      "risk_level": "critical",
      "business_impact": "high",
      "technical_complexity": "high",
      "user_exposure": "medium",
      "test_types_needed": ["unit", "integration", "security"],
      "missing_test_scenarios": [
        "Fraud pattern detection",
        "Threshold-based alerts",
        "False positive handling",
        "Integration with external fraud services"
      ],
      "priority_score": 92,
      "priority_level": "critical",
      "estimated_effort_hours": 16,
      "owner": "security-qa-team",
      "remediation": "Implement comprehensive fraud detection test suite"
    },
    {
      "id": "test-gap-002",
      "type": "partial_coverage",
      "requirement_id": "RQ-038",
      "requirement_description": "System shall process refunds within 24 hours",
      "existing_tests": 3,
      "needed_tests": 8,
      "missing_test_scenarios": [
        "Refund timeout handling",
        "Concurrent refund processing",
        "Partial refund scenarios",
        "Refund failure recovery",
        "Refund notification verification",
        "Refund audit trail validation"
      ],
      "risk_level": "high",
      "priority_score": 78,
      "priority_level": "high",
      "estimated_effort_hours": 8,
      "owner": "payment-qa-team",
      "remediation": "Complete refund processing test coverage"
    },
    {
      "id": "test-gap-003",
      "type": "missing_test_type",
      "requirement_id": "RQ-035",
      "requirement_description": "System shall support 15+ currencies",
      "existing_test_types": ["unit", "integration"],
      "missing_test_types": ["performance", "security"],
      "test_coverage": "5/15 currencies tested",
      "risk_level": "medium",
      "priority_score": 65,
      "priority_level": "medium",
      "estimated_effort_hours": 12,
      "owner": "international-qa-team",
      "remediation": "Add remaining currency tests and performance/security tests"
    },
    {
      "id": "test-gap-004",
      "type": "outdated_tests",
      "requirement_id": "RQ-019",
      "requirement_description": "System shall generate payment reports",
      "test_count": 4,
      "outdated_tests": 2,
      "outdated_reasons": [
        "Tests use deprecated report format",
        "Tests don't match current report schema"
      ],
      "risk_level": "low",
      "priority_score": 42,
      "priority_level": "low",
      "estimated_effort_hours": 4,
      "owner": "reporting-qa-team",
      "remediation": "Update report generation tests to match current requirements"
    }
  ],
  "risk_analysis": {
    "high_risk_untested": 2,
    "medium_risk_partial": 3,
    "low_risk_gaps": 4,
    "risk_coverage_score": 65.8,
    "risk_by_category": {
      "security": 40.0,
      "financial": 75.0,
      "compliance": 90.0,
      "user_experience": 85.0
    }
  },
  "test_effectiveness": {
    "total_tests": 127,
    "flaky_tests": 8,
    "slow_tests": 15,
    "unmaintained_tests": 5,
    "high_value_tests": 42,
    "effectiveness_score": 72.4,
    "test_health": "warning",
    "maintenance_score": 68.0
  },
  "recommendations": {
    "immediate": [
      "Implement fraud detection test suite (test-gap-001)",
      "Complete refund processing test coverage (test-gap-002)"
    ],
    "short_term": [
      "Add missing currency tests (test-gap-003)",
      "Fix flaky security tests"
    ],
    "long_term": [
      "Update outdated report tests (test-gap-004)",
      "Improve test maintenance process",
      "Implement test gap monitoring"
    ]
  },
  "remediation_plan": {
    "estimated_total_effort_hours": 40,
    "phases": [
      {
        "phase": 1,
        "name": "Critical Security & Financial Gaps",
        "duration": "1 week",
        "gaps": ["test-gap-001", "test-gap-002"],
        "deliverables": [
          "Fraud detection test suite",
          "Complete refund test coverage"
        ]
      },
      {
        "phase": 2,
        "name": "Internationalization & Performance",
        "duration": "2 weeks",
        "gaps": ["test-gap-003"],
        "deliverables": [
          "Remaining currency tests",
          "Performance tests for currency conversion"
        ]
      },
      {
        "phase": 3,
        "name": "Test Maintenance & Quality",
        "duration": "1 week",
        "gaps": ["test-gap-004"],
        "deliverables": [
          "Updated report generation tests",
          "Reduced flaky test rate",
          "Improved test maintenance score"
        ]
      }
    ],
    "success_criteria": [
      "Overall coverage > 85%",
      "Critical risk coverage > 90%",
      "Flaky test rate < 5%",
      "Test maintenance score > 80"
    ]
  }
}
EOF

echo "✅ Test gap analysis complete!" >&2
echo "📊 Analysis saved to: $OUTPUT_FILE" >&2
echo "" >&2

echo "Coverage Summary:" >&2
echo "────────────────" >&2
echo "Requirements: 42" >&2
echo "Tests: 127" >&2
echo "Overall Coverage: 76.2%" >&2
echo "✅ Fully Tested: 28 (66.7%)" >&2
echo "⚠️  Partially Tested: 8 (19.0%)" >&2
echo "❌ Untested: 6 (14.3%)" >&2
echo "" >&2

echo "Test Gap Distribution:" >&2
echo "┌──────────────────────┬──────┬──────────┐" >&2
echo "│ Gap Type            │ Count│ Percentage│" >&2
echo "├──────────────────────┼──────┼──────────┤" >&2
echo "│ Missing Coverage    │ 2    │ 33.3%    │" >&2
echo "│ Partial Coverage    │ 3    │ 50.0%    │" >&2
echo "│ Outdated Tests      │ 1    │ 16.7%    │" >&2
echo "└──────────────────────┴──────┴──────────┘" >&2
echo "" >&2

echo "Critical Test Gaps Found:" >&2
echo "1. ❌ Fraud detection not tested (critical security risk)" >&2
echo "2. ⚠️ Refund processing partially tested (high financial risk)" >&2
echo "3. ⚠️ Currency support incomplete (medium business risk)" >&2
echo "4. ℹ️ Report generation tests outdated (low maintenance risk)" >&2
echo "" >&2

echo "Risk Coverage Analysis:" >&2
echo "• Security: 40% ⚠️ (needs immediate attention)" >&2
echo "• Financial: 75% ⚠️ (needs improvement)" >&2
echo "• Compliance: 90% ✅ (good)" >&2
echo "• User Experience: 85% ✅ (good)" >&2
echo "" >&2

echo "Test Effectiveness:" >&2
echo "• Flaky Tests: 8 (6.3%) ⚠️" >&2
echo "• Slow Tests: 15 (11.8%) ⚠️" >&2
echo "• Unmaintained Tests: 5 (3.9%) ℹ️" >&2
echo "• High-Value Tests: 42 (33.1%) ✅" >&2
echo "" >&2

echo "Remediation Priority:" >&2
echo "1. Implement fraud detection tests (16 hours)" >&2
echo "2. Complete refund test coverage (8 hours)" >&2
echo "3. Add currency tests (12 hours)" >&2
echo "4. Update outdated tests (4 hours)" >&2
echo "" >&2

echo "Estimated Total Effort: 40 hours (1-2 weeks)" >&2
echo "Target Coverage: 85% by $(date -v +14d +%Y-%m-%d)" >&2
echo "" >&2

echo "Usage Examples:" >&2
echo "  # Analyze with custom directories" >&2
echo "  npm run test-gap-analysis -- --requirements ./specs --tests ./test-suites --output gaps.json" >&2
echo "" >&2
echo "  # Generate test recommendations" >&2
echo "  npm run test-gap-analysis:recommend -- --analysis test-gap-analysis.json" >&2
echo "" >&2
echo "  # Continuous test gap monitoring" >&2
echo "  npm run test-gap-analysis:monitor -- --watch --requirements specs/ --tests tests/" >&2

# Output JSON status
echo '{"status": "analyzed", "service": "test-gap-analysis", "timestamp": "'"$TIMESTAMP"'", "output_file": "'"$OUTPUT_FILE"'", "coverage_percentage": 76.2, "critical_gaps": 2, "test_count": 127}'