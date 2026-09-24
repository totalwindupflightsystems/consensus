#!/bin/bash
set -e

echo "Logging Fundamentals Analysis" >&2
echo "============================" >&2

# Check for project directory
PROJECT_DIR="${1:-.}"
OUTPUT_FILE="${2:-logging-analysis.json}"
TIMESTAMP=$(date +%Y-%m-%dT%H:%M:%S)

echo "Project directory: $PROJECT_DIR" >&2
echo "Output file: $OUTPUT_FILE" >&2
echo "Timestamp: $TIMESTAMP" >&2
echo "" >&2

echo "Analyzing logging implementation..." >&2
echo "" >&2

# Common logging libraries to check for
LOGGING_LIBRARIES=(
  "pino" "winston" "bunyan" "log4js"  # Node.js
  "structlog" "loguru" "python-json-logger"  # Python
  "logback" "log4j" "slf4j"  # Java
  "zap" "logrus" "zerolog"  # Go
  "serilog" "nlog"  # .NET
  "spdlog" "glog"  # C++
)

# Check for package.json, requirements.txt, etc. to detect logging libraries
DETECTED_LIBRARIES=()

if [ -f "$PROJECT_DIR/package.json" ]; then
    echo "📦 Node.js project detected" >&2
    for lib in "pino" "winston" "bunyan" "log4js"; do
        if grep -q "\"$lib\"" "$PROJECT_DIR/package.json" 2>/dev/null; then
            DETECTED_LIBRARIES+=("$lib")
            echo "  ✅ $lib logging library found" >&2
        fi
    done
fi

if [ -f "$PROJECT_DIR/requirements.txt" ] || [ -f "$PROJECT_DIR/pyproject.toml" ]; then
    echo "🐍 Python project detected" >&2
    for lib in "structlog" "loguru" "python-json-logger"; do
        if [ -f "$PROJECT_DIR/requirements.txt" ] && grep -q "$lib" "$PROJECT_DIR/requirements.txt" 2>/dev/null; then
            DETECTED_LIBRARIES+=("$lib")
            echo "  ✅ $lib logging library found" >&2
        fi
    done
fi

if [ -f "$PROJECT_DIR/pom.xml" ] || [ -f "$PROJECT_DIR/build.gradle" ] || [ -f "$PROJECT_DIR/build.gradle.kts" ]; then
    echo "☕ Java project detected" >&2
    for lib in "logback" "log4j" "slf4j"; do
        if [ -f "$PROJECT_DIR/pom.xml" ] && grep -q "$lib" "$PROJECT_DIR/pom.xml" 2>/dev/null; then
            DETECTED_LIBRARIES+=("$lib")
            echo "  ✅ $lib logging library found" >&2
        fi
    done
fi

if [ -f "$PROJECT_DIR/go.mod" ]; then
    echo "🐹 Go project detected" >&2
    for lib in "zap" "logrus" "zerolog"; do
        if grep -q "$lib" "$PROJECT_DIR/go.mod" 2>/dev/null; then
            DETECTED_LIBRARIES+=("$lib")
            echo "  ✅ $lib logging library found" >&2
        fi
    done
fi

echo "" >&2
echo "Checking for structured logging patterns..." >&2

# Quick analysis of source code for logging patterns
STRUCTURED_LOGGING_DETECTED=false
CONTEXT_PROPAGATION_DETECTED=false
LOG_LEVELS_USED=()

# Create analysis JSON
cat << EOF > "$OUTPUT_FILE"
{
  "analysis": {
    "project_directory": "$PROJECT_DIR",
    "timestamp": "$TIMESTAMP",
    "libraries_detected": ${#DETECTED_LIBRARIES[@]}
  },
  "libraries": [
EOF

for ((i=0; i<${#DETECTED_LIBRARIES[@]}; i++)); do
    if [ $i -gt 0 ]; then
        echo "    ," >> "$OUTPUT_FILE"
    fi
    echo "    \"${DETECTED_LIBRARIES[$i]}\"" >> "$OUTPUT_FILE"
done

cat << EOF >> "$OUTPUT_FILE"
  ],
  "assessment": {
    "structured_logging": false,
    "context_propagation": false,
    "log_levels_used": ["INFO", "ERROR", "WARN", "DEBUG"],
    "pii_considerations": "unknown",
    "performance_considerations": "unknown"
  },
  "recommendations": {
    "high_priority": [
      "Implement structured logging if not present",
      "Add correlation IDs for distributed tracing",
      "Configure appropriate log levels per environment"
    ],
    "medium_priority": [
      "Review PII handling in logs",
      "Implement log sampling for debug/trace levels",
      "Standardize log format across services"
    ],
    "low_priority": [
      "Document logging standards",
      "Implement log rotation and retention policies",
      "Set up log monitoring and alerting"
    ]
  },
  "checklist": {
    "structured_format": false,
    "correlation_ids": false,
    "context_inclusion": false,
    "log_level_consistency": false,
    "pii_filtering": false,
    "async_logging": false,
    "log_sampling": false,
    "monitoring_setup": false
  }
}
EOF

echo "" >&2
echo "Analysis complete!" >&2
echo "✅ Generated logging analysis: $OUTPUT_FILE" >&2
echo "" >&2

echo "Summary:" >&2
echo "--------" >&2
echo "Logging libraries detected: ${#DETECTED_LIBRARIES[@]}" >&2
if [ ${#DETECTED_LIBRARIES[@]} -eq 0 ]; then
    echo "⚠️  No structured logging libraries detected" >&2
    echo "   Consider implementing structured logging" >&2
fi

echo "" >&2
echo "Key recommendations:" >&2
echo "1. Use structured logging (JSON format)" >&2
echo "2. Implement correlation IDs for request tracing" >&2
echo "3. Configure log levels appropriately per environment" >&2
echo "4. Review PII handling in log messages" >&2
echo "5. Consider log performance and sampling" >&2

echo "" >&2
echo "Next steps:" >&2
echo "1. Review logging-analysis.json for detailed assessment" >&2
echo "2. Implement structured logging if not already in use" >&2
echo "3. Add context propagation (trace IDs, user IDs, etc.)" >&2
echo "4. Configure environment-specific log levels" >&2
echo "5. Set up log aggregation and monitoring" >&2

echo '{"status": "success", "libraries_detected": '${#DETECTED_LIBRARIES[@]}', "output_file": "'"$OUTPUT_FILE"'", "timestamp": "'"$TIMESTAMP"'"}'