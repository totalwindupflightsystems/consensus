#!/bin/bash
set -e

echo "Starting security scan analysis..." >&2

# Function to display usage
usage() {
    echo "Usage: $0 [OPTIONS]" >&2
    echo "Options:" >&2
    echo "  --target URL          Target URL to scan" >&2
    echo "  --container IMAGE    Docker container image to scan" >&2
    echo "  --iac PATH           Infrastructure as code path to scan" >&2
    echo "  --compliance NAME    Compliance framework (soc2, iso27001, hipaa, gdpr)" >&2
    echo "  --llm-analysis       Enable LLM-powered security analysis" >&2
    echo "  --context TEXT       Additional context for LLM analysis" >&2
    echo "  --help               Show this help message" >&2
    exit 1
}

# Parse command line arguments
TARGET=""
CONTAINER=""
IAC_PATH=""
COMPLIANCE=""
LLM_ANALYSIS=false
CONTEXT=""

while [[ $# -gt 0 ]]; do
    case $1 in
        --target)
            TARGET="$2"
            shift 2
            ;;
        --container)
            CONTAINER="$2"
            shift 2
            ;;
        --iac)
            IAC_PATH="$2"
            shift 2
            ;;
        --compliance)
            COMPLIANCE="$2"
            shift 2
            ;;
        --llm-analysis)
            LLM_ANALYSIS=true
            shift
            ;;
        --context)
            CONTEXT="$2"
            shift 2
            ;;
        --help)
            usage
            ;;
        *)
            echo "Error: Unknown option $1" >&2
            usage
            ;;
    esac
done

# Validate at least one scanning option is provided
if [ -z "$TARGET" ] && [ -z "$CONTAINER" ] && [ -z "$IAC_PATH" ]; then
    echo "Error: At least one scanning option must be provided (--target, --container, or --iac)" >&2
    usage
fi

# Function to check if command exists
check_command() {
    if ! command -v "$1" &> /dev/null; then
        echo "Warning: $1 not found. Some security scanning capabilities may be limited." >&2
        return 1
    fi
    return 0
}

# Check for common security scanning tools
check_command "npm"
check_command "snyk"
check_command "trivy"
check_command "zap"
check_command "bandit"
check_command "gosec"
check_command "semgrep"
check_command "tfsec"
check_command "checkov"
check_command "trufflehog"

# Analyze based on input parameters
echo "Analyzing security requirements..." >&2

SCAN_TYPE=""
SCAN_TARGET=""

if [ -n "$TARGET" ]; then
    SCAN_TYPE="web_application"
    SCAN_TARGET="$TARGET"
    echo "• Web application scanning: $TARGET" >&2
fi

if [ -n "$CONTAINER" ]; then
    SCAN_TYPE="container"
    SCAN_TARGET="$CONTAINER"
    echo "• Container scanning: $CONTAINER" >&2
fi

if [ -n "$IAC_PATH" ]; then
    SCAN_TYPE="infrastructure"
    SCAN_TARGET="$IAC_PATH"
    echo "• Infrastructure as code scanning: $IAC_PATH" >&2
fi

if [ -n "$COMPLIANCE" ]; then
    echo "• Compliance framework: $COMPLIANCE" >&2
fi

if [ "$LLM_ANALYSIS" = true ]; then
    echo "• LLM-powered analysis: Enabled" >&2
    if [ -n "$CONTEXT" ]; then
        echo "• Additional context: $CONTEXT" >&2
    fi
fi

# Generate scan plan
echo "Generating security scan plan..." >&2

# Output JSON with scan plan
cat <<EOF
{
  "scan_type": "$SCAN_TYPE",
  "scan_target": "$SCAN_TARGET",
  "compliance_framework": "$COMPLIANCE",
  "llm_analysis": $LLM_ANALYSIS,
  "scan_context": "$CONTEXT",
  "recommended_tools": [
    "OWASP ZAP",
    "Snyk",
    "Trivy",
    "Nessus",
    "Semgrep",
    "Bandit",
    "Gosec",
    "TfSec",
    "Checkov",
    "TruffleHog"
  ],
  "scan_steps": [
    "1. Dependency vulnerability scanning",
    "2. Static application security testing (SAST)",
    "3. Dynamic application security testing (DAST)",
    "4. Container vulnerability scanning",
    "5. Infrastructure as code scanning",
    "6. Secrets detection",
    "7. Compliance mapping",
    "8. Risk assessment and prioritization"
  ],
  "estimated_duration": "10-30 minutes",
  "critical_actions": [
    "Run dependency scan with 'npm audit' or 'snyk test'",
    "Perform SAST with 'semgrep scan --config auto'",
    "Run DAST with 'zap-baseline.py -t $TARGET' (if web target)",
    "Scan container with 'trivy image $CONTAINER' (if container)",
    "Scan IaC with 'tfsec $IAC_PATH' (if IaC)",
    "Check for secrets with 'trufflehog --regex --entropy'"
  ]
}
EOF

echo "Security scan analysis complete. Run the recommended critical actions above." >&2