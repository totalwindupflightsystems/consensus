#!/bin/bash
set -e

echo "Starting API documentation analysis..." >&2

# Function to display usage
usage() {
    echo "Usage: $0 [OPTIONS]" >&2
    echo "Options:" >&2
    echo "  --source PATH        Source directory or file to analyze" >&2
    echo "  --format FORMAT      API format (openapi, grpc, graphql, rpc, auto)" >&2
    echo "  --portal             Generate API portal documentation" >&2
    echo "  --output PATH        Output directory for generated documentation" >&2
    echo "  --consistency-check  Check consistency between implementation and docs" >&2
    echo "  --api PATH           API implementation path for consistency checking" >&2
    echo "  --help               Show this help message" >&2
    exit 1
}

# Parse command line arguments
SOURCE=""
FORMAT="auto"
PORTAL=false
OUTPUT=""
CONSISTENCY_CHECK=false
API_PATH=""

while [[ $# -gt 0 ]]; do
    case $1 in
        --source)
            SOURCE="$2"
            shift 2
            ;;
        --format)
            FORMAT="$2"
            shift 2
            ;;
        --portal)
            PORTAL=true
            shift
            ;;
        --output)
            OUTPUT="$2"
            shift 2
            ;;
        --consistency-check)
            CONSISTENCY_CHECK=true
            shift
            ;;
        --api)
            API_PATH="$2"
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

# Validate source is provided
if [ -z "$SOURCE" ]; then
    echo "Error: --source must be provided" >&2
    usage
fi

# Check if source exists
if [ ! -e "$SOURCE" ]; then
    echo "Error: Source '$SOURCE' does not exist" >&2
    exit 1
fi

# Function to check if command exists
check_command() {
    if ! command -v "$1" &> /dev/null; then
        echo "Warning: $1 not found. Some documentation capabilities may be limited." >&2
        return 1
    fi
    return 0
}

# Check for common API documentation tools
check_command "swagger-cli"
check_command "redoc-cli"
check_command "protoc"
check_command "graphql"
check_command "spectral"

# Detect API format if auto
if [ "$FORMAT" = "auto" ]; then
    echo "Detecting API format from source..." >&2
    if [[ "$SOURCE" =~ \.(proto|pb)$ ]] || find "$SOURCE" -name "*.proto" -o -name "*.pb" | grep -q .; then
        FORMAT="grpc"
        echo "Detected gRPC/Protobuf format" >&2
    elif [[ "$SOURCE" =~ \.(graphql|gql)$ ]] || find "$SOURCE" -name "*.graphql" -o -name "*.gql" | grep -q .; then
        FORMAT="graphql"
        echo "Detected GraphQL format" >&2
    elif [[ "$SOURCE" =~ \.(yaml|yml|json)$ ]] || find "$SOURCE" -name "*.yaml" -o -name "*.yml" -o -name "*.json" | grep -q .; then
        # Check if it's OpenAPI
        if grep -q "openapi:" "$SOURCE" 2>/dev/null || grep -q '"openapi":' "$SOURCE" 2>/dev/null; then
            FORMAT="openapi"
            echo "Detected OpenAPI format" >&2
        else
            FORMAT="rest"
            echo "Detected REST/HTTP format (assuming OpenAPI)" >&2
        fi
    else
        # Assume REST/HTTP by default
        FORMAT="rest"
        echo "Assuming REST/HTTP format" >&2
    fi
fi

echo "Analyzing API documentation for format: $FORMAT" >&2
echo "Source: $SOURCE" >&2

if [ "$PORTAL" = true ]; then
    echo "API portal generation: Enabled" >&2
    if [ -z "$OUTPUT" ]; then
        OUTPUT="./api-portal"
        echo "Output directory: $OUTPUT (default)" >&2
    else
        echo "Output directory: $OUTPUT" >&2
    fi
fi

if [ "$CONSISTENCY_CHECK" = true ]; then
    echo "Consistency checking: Enabled" >&2
    if [ -n "$API_PATH" ]; then
        echo "API implementation path: $API_PATH" >&2
    else
        echo "Warning: API implementation path not provided for consistency checking" >&2
    fi
fi

# Generate analysis based on format
case $FORMAT in
    openapi|rest)
        API_TYPE="REST/HTTP"
        TOOLS=("Swagger UI" "Redoc" "Stoplight" "OpenAPI Generator")
        ;;
    grpc)
        API_TYPE="gRPC"
        TOOLS=("protoc with doc plugins" "grpc-doc" "DocFX" "Protobuf documentation generator")
        ;;
    graphql)
        API_TYPE="GraphQL"
        TOOLS=("GraphQL Doc" "SpectaQL" "GraphQL Markdown" "GraphQL Voyager")
        ;;
    rpc)
        API_TYPE="RPC"
        TOOLS=("JSON-RPC documentation tools" "XML-RPC documentation" "Custom RPC documentation")
        ;;
    *)
        API_TYPE="Unknown"
        TOOLS=("General API documentation tools")
        ;;
esac

# Count endpoints/files (simplified)
ENDPOINT_COUNT=0
FILE_COUNT=0

if [ -d "$SOURCE" ]; then
    FILE_COUNT=$(find "$SOURCE" -type f \( -name "*.js" -o -name "*.ts" -o -name "*.py" -o -name "*.go" -o -name "*.java" -o -name "*.rs" \) | wc -l)
    ENDPOINT_COUNT=$((FILE_COUNT * 3))  # Rough estimate
else
    FILE_COUNT=1
    ENDPOINT_COUNT=5  # Rough estimate
fi

# Output JSON with analysis
cat <<EOF
{
  "api_type": "$API_TYPE",
  "source": "$SOURCE",
  "file_count": $FILE_COUNT,
  "estimated_endpoints": $ENDPOINT_COUNT,
  "portal_generation": $PORTAL,
  "consistency_checking": $CONSISTENCY_CHECK,
  "recommended_tools": [
    ${TOOLS[@]/#/\"/}
    ${TOOLS[@]/%/\"}
  ],
  "documentation_workflow": [
    "1. Extract API definitions from source code",
    "2. Generate API specification (OpenAPI, Protobuf, GraphQL schema)",
    "3. Create reference documentation",
    "4. Add examples and tutorials",
    "5. Implement interactive testing",
    "6. Generate client SDKs",
    "7. Deploy documentation portal",
    "8. Set up documentation updates pipeline"
  ],
  "quality_metrics_to_check": [
    "Completeness (all endpoints documented)",
    "Accuracy (matches implementation)",
    "Clarity (clear descriptions and examples)",
    "Consistency (consistent formatting and structure)",
    "Discoverability (searchable, well-organized)",
    "Interactivity (testable examples)",
    "Versioning (multiple versions documented)",
    "Authentication (auth methods documented)"
  ],
  "next_steps": [
    "Run 'swagger-cli bundle $SOURCE -o openapi.yaml' (for OpenAPI)",
    "Run 'protoc --doc_out=docs --doc_opt=markdown $SOURCE/*.proto' (for gRPC)",
    "Run 'graphql-doc generate $SOURCE' (for GraphQL)",
    "Review documentation for missing endpoints and parameters",
    "Add authentication and error handling documentation",
    "Generate interactive API console",
    "Set up automated documentation updates"
  ]
}
EOF

echo "API documentation analysis complete. Follow the next steps above." >&2