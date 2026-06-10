#!/bin/bash
set -e

echo "Distributed Tracing: Generate Trace" >&2
echo "==================================" >&2

# Default values
NUM_SPANS="${1:-5}"
OUTPUT_FILE="${2:-trace-data.json}"
SERVICE_NAME="${3:-api-service}"
TIMESTAMP=$(date +%Y-%m-%dT%H:%M:%S)

# Generate trace ID (32 hex characters)
TRACE_ID=$(openssl rand -hex 16 2>/dev/null || echo "0af7651916cd43dd8448eb211c80319c")

echo "Generating trace with $NUM_SPANS spans" >&2
echo "Trace ID: $TRACE_ID" >&2
echo "Service: $SERVICE_NAME" >&2
echo "Output: $OUTPUT_FILE" >&2
echo "" >&2

# Generate W3C traceparent header
TRACEPARENT="00-$TRACE_ID-$(openssl rand -hex 8)-01"
echo "Traceparent header: $TRACEPARENT" >&2

# Create trace data with spans
cat << EOF > "$OUTPUT_FILE"
{
  "trace": {
    "trace_id": "$TRACE_ID",
    "service_name": "$SERVICE_NAME",
    "timestamp": "$TIMESTAMP",
    "traceparent": "$TRACEPARENT",
    "span_count": $NUM_SPANS
  },
  "spans": [
EOF

# Generate spans
for i in $(seq 1 $NUM_SPANS); do
    SPAN_ID=$(openssl rand -hex 8 2>/dev/null || echo "b7ad6b716920333$i")
    PARENT_SPAN_ID=""
    if [ $i -gt 1 ]; then
        PARENT_SPAN_ID="b7ad6b716920333$((i-1))"
    fi
    
    SPAN_NAME="span-$i"
    DURATION=$((RANDOM % 1000 + 50))
    START_OFFSET=$((RANDOM % 100))
    STATUS="OK"
    if [ $((RANDOM % 10)) -eq 0 ]; then
        STATUS="ERROR"
    fi
    
    cat << EOF >> "$OUTPUT_FILE"
    {
      "span_id": "$SPAN_ID",
      "parent_span_id": "$PARENT_SPAN_ID",
      "name": "$SPAN_NAME",
      "kind": "SERVER",
      "start_time_offset_ms": $START_OFFSET,
      "duration_ms": $DURATION,
      "status": "$STATUS",
      "attributes": {
        "service": "$SERVICE_NAME",
        "operation": "operation-$i",
        "http.method": "GET",
        "http.path": "/api/resource/$i"
      },
      "events": [
        {
          "name": "span_start",
          "timestamp_offset_ms": 0
        },
        {
          "name": "span_end",
          "timestamp_offset_ms": $DURATION
        }
      ],
      "logs": [
        {
          "timestamp": "$TIMESTAMP",
          "fields": {
            "level": "INFO",
            "message": "Span $i started",
            "trace_id": "$TRACE_ID",
            "span_id": "$SPAN_ID"
          }
        },
        {
          "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ -d "+${DURATION}ms")",
          "fields": {
            "level": "INFO",
            "message": "Span $i completed",
            "trace_id": "$TRACE_ID",
            "span_id": "$SPAN_ID",
            "duration_ms": $DURATION,
            "status": "$STATUS"
          }
        }
      ]
    }
EOF
    
    if [ $i -lt $NUM_SPANS ]; then
        echo "    ," >> "$OUTPUT_FILE"
    fi
done

cat << EOF >> "$OUTPUT_FILE"
  ],
  "propagation_headers": {
    "traceparent": "$TRACEPARENT",
    "tracestate": "",
    "X-Correlation-Id": "corr-$TRACE_ID",
    "X-Request-Id": "req-$(openssl rand -hex 8)"
  },
  "recommendations": [
    "Include trace_id in all log entries",
    "Propagate traceparent header across HTTP calls",
    "Use consistent span naming conventions",
    "Add business context to span attributes",
    "Implement sampling to reduce volume in production"
  ]
}
EOF

echo "✅ Generated trace data: $OUTPUT_FILE" >&2
echo "" >&2
echo "Trace Context Headers:" >&2
echo "  traceparent: $TRACEPARENT" >&2
echo "  X-Correlation-Id: corr-$TRACE_ID" >&2
echo "" >&2
echo "Sample log entries with trace context:" >&2
echo '  {"timestamp": "'"$TIMESTAMP"'", "level": "INFO", "trace_id": "'"$TRACE_ID"'", "span_id": "b7ad6b7169203331", "message": "Request processed"}' >&2
echo "" >&2

# Generate OpenTelemetry configuration
cat << EOF > "otel-config.yaml"
# OpenTelemetry configuration for $SERVICE_NAME
opentelemetry:
  service:
    name: "$SERVICE_NAME"
    version: "1.0.0"
  
  tracing:
    enabled: true
    sampler:
      type: "parentbased_traceidratio"
      ratio: 0.1
    
    exporters:
      - type: "otlp"
        endpoint: "http://otel-collector:4317"
      - type: "logging"
        level: "info"
    
    propagators:
      - "tracecontext"
      - "baggage"
  
  logging:
    include_trace_context: true
    fields:
      - "trace_id"
      - "span_id"
      - "trace_flags"
    format: "json"
EOF

echo "Generated OpenTelemetry configuration: otel-config.yaml" >&2
echo "" >&2
echo "Next steps:" >&2
echo "1. Review trace data in $OUTPUT_FILE" >&2
echo "2. Use propagation headers in HTTP requests" >&2
echo "3. Configure OpenTelemetry with otel-config.yaml" >&2
echo "4. Add trace context to your application logs" >&2

# Output JSON status
echo '{"status": "generated", "service": "distributed-tracing-logs", "timestamp": "'"$TIMESTAMP"'", "trace_id": "'"$TRACE_ID"'", "output_file": "'"$OUTPUT_FILE"'", "otel_config": "otel-config.yaml"}'