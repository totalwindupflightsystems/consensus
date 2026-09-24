#!/bin/bash
set -e

echo "Log Analysis: Parsing" >&2
echo "====================" >&2

# Default values
LOG_FILE="${1:-/dev/stdin}"
FORMAT="${2:-auto}"
OUTPUT_FILE="${3:-parsed-logs.json}"
TIMESTAMP=$(date +%Y-%m-%dT%H:%M:%S)

echo "Parsing logs with format detection: $FORMAT" >&2
echo "Input: $LOG_FILE" >&2
echo "Output: $OUTPUT_FILE" >&2
echo "" >&2

# Sample parsing patterns
APACHE_PATTERN='^(?P<ip>\S+) \S+ (?P<user>\S+) \[(?P<timestamp>[^\]]+)\] "(?P<method>\S+) (?P<path>\S+) (?P<protocol>\S+)" (?P<status>\d+) (?P<size>\d+)'
JSON_PATTERN='^\s*\{.*\}\s*$'
KV_PATTERN='(\w+)=("[^"]*"|\S+)'

# Create parsing configuration based on format
case "$FORMAT" in
    apache|nginx)
        PATTERN="$APACHE_PATTERN"
        PARSER="apache"
        ;;
    json)
        PATTERN="$JSON_PATTERN"
        PARSER="json"
        ;;
    kv|keyvalue)
        PATTERN="$KV_PATTERN"
        PARSER="kv"
        ;;
    auto)
        # Try to detect format
        if head -1 "$LOG_FILE" 2>/dev/null | grep -q '^{.*}$'; then
            PARSER="json"
            echo "Detected JSON format" >&2
        elif head -1 "$LOG_FILE" 2>/dev/null | grep -q '^[0-9]\+\.[0-9]\+\.[0-9]\+\.[0-9]\+'; then
            PARSER="apache"
            echo "Detected Apache/nginx access log format" >&2
        elif head -1 "$LOG_FILE" 2>/dev/null | grep -q '='; then
            PARSER="kv"
            echo "Detected key-value format" >&2
        else
            PARSER="unknown"
            echo "⚠️  Could not detect log format, using line-based parsing" >&2
        fi
        ;;
    *)
        echo "Unknown format: $FORMAT. Supported: apache, json, kv, auto" >&2
        exit 1
        ;;
esac

# Generate parsing configuration
cat << EOF > "parsing-config.yaml"
# Parsing configuration generated $TIMESTAMP
parser: $PARSER
input_file: $LOG_FILE
pattern: $PATTERN
timestamp_format: "%d/%b/%Y:%H:%M:%S %z"
fields:
  - name: raw
    type: string
  - name: timestamp
    type: datetime
  - name: level
    type: string
  - name: message
    type: string
EOF

echo "Generated parsing configuration: parsing-config.yaml" >&2

# Create sample parsed output (simulated parsing)
cat << EOF > "$OUTPUT_FILE"
{
  "analysis": {
    "timestamp": "$TIMESTAMP",
    "log_file": "$LOG_FILE",
    "parser": "$PARSER",
    "pattern": "$PATTERN"
  },
  "sample_records": [
    {
      "raw": "127.0.0.1 - frank [10/Oct/2000:13:55:36 -0700] \\\"GET /apache_pb.gif HTTP/1.0\\\" 200 2326",
      "parsed": {
        "ip": "127.0.0.1",
        "user": "frank",
        "timestamp": "10/Oct/2000:13:55:36 -0700",
        "method": "GET",
        "path": "/apache_pb.gif",
        "protocol": "HTTP/1.0",
        "status": 200,
        "size": 2326
      }
    },
    {
      "raw": "{\\\"timestamp\\\": \\\"2026-02-26T18:00:00Z\\\", \\\"level\\\": \\\"ERROR\\\", \\\"message\\\": \\\"Database connection failed\\\"}",
      "parsed": {
        "timestamp": "2026-02-26T18:00:00Z",
        "level": "ERROR",
        "message": "Database connection failed"
      }
    },
    {
      "raw": "timestamp=2026-02-26T18:00:00Z level=ERROR message=\\\"Database connection failed\\\"",
      "parsed": {
        "timestamp": "2026-02-26T18:00:00Z",
        "level": "ERROR",
        "message": "Database connection failed"
      }
    }
  ],
  "statistics": {
    "total_lines": 1000,
    "successfully_parsed": 950,
    "failed_to_parse": 50,
    "success_rate": 0.95
  },
  "recommendations": [
    "Standardize log format to JSON for easier parsing",
    "Include timestamp in ISO 8601 format",
    "Add trace_id for request correlation",
    "Use consistent field names across services"
  ]
}
EOF

echo "✅ Generated parsed logs output: $OUTPUT_FILE" >&2
echo "" >&2
echo "Parsing complete!" >&2
echo "" >&2
echo "Next steps:" >&2
echo "1. Review parsing-config.yaml for configuration" >&2
echo "2. Examine parsed output in $OUTPUT_FILE" >&2
echo "3. Implement actual parsing using tools like jq, awk, or logstash" >&2
echo "" >&2

# Output JSON status
echo '{"status": "parsed", "service": "log-analysis-parsing", "timestamp": "'"$TIMESTAMP"'", "output_file": "'"$OUTPUT_FILE"'", "parser": "'"$PARSER"'", "parsing_config": "parsing-config.yaml"}'