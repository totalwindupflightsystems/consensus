#!/bin/bash
set -e

echo "Observability Logging: Dashboard" >&2
echo "================================" >&2

# Default values
DASHBOARD_TYPE="${1:-grafana}"
SERVICES="${2:-api-service,auth-service,db-service}"
OUTPUT_FILE="${3:-observability-dashboard.json}"
TIMESTAMP=$(date +%Y-%m-%dT%H:%M:%S)

echo "Creating $DASHBOARD_TYPE observability dashboard" >&2
echo "Services: $SERVICES" >&2
echo "Output: $OUTPUT_FILE" >&2
echo "" >&2

# Split services into array
IFS=',' read -ra SERVICE_ARRAY <<< "$SERVICES"

# Generate dashboard based on type
case "$DASHBOARD_TYPE" in
    grafana)
        cat << EOF > "$OUTPUT_FILE"
{
  "dashboard": {
    "title": "Observability Dashboard",
    "description": "Comprehensive observability dashboard combining logs, metrics, and traces",
    "tags": ["observability", "logs", "metrics", "traces"],
    "timezone": "browser",
    "panels": [
      {
        "id": 1,
        "title": "Error Rate (Logs)",
        "type": "timeseries",
        "targets": [
          {
            "expr": "rate(error_logs_total[5m])",
            "legendFormat": "{{service}}"
          }
        ],
        "gridPos": {"x": 0, "y": 0, "w": 12, "h": 8}
      },
      {
        "id": 2,
        "title": "Request Latency (Metrics)",
        "type": "timeseries",
        "targets": [
          {
            "expr": "histogram_quantile(0.95, rate(request_duration_seconds_bucket[5m]))",
            "legendFormat": "P95 {{service}}"
          },
          {
            "expr": "histogram_quantile(0.99, rate(request_duration_seconds_bucket[5m]))",
            "legendFormat": "P99 {{service}}"
          }
        ],
        "gridPos": {"x": 12, "y": 0, "w": 12, "h": 8}
      },
      {
        "id": 3,
        "title": "Recent Logs",
        "type": "logs",
        "targets": [
          {
            "query": "level:ERROR",
            "limit": 100
          }
        ],
        "gridPos": {"x": 0, "y": 8, "w": 24, "h": 8}
      },
      {
        "id": 4,
        "title": "Service Map (Traces)",
        "type": "nodeGraph",
        "targets": [
          {
            "query": "topology(service_name)"
          }
        ],
        "gridPos": {"x": 0, "y": 16, "w": 12, "h": 8}
      },
      {
        "id": 5,
        "title": "SLO Burn Rate",
        "type": "gauge",
        "targets": [
          {
            "expr": "1 - (error_budget_remaining)",
            "legendFormat": "Error Budget Consumed"
          }
        ],
        "gridPos": {"x": 12, "y": 16, "w": 6, "h": 8}
      },
      {
        "id": 6,
        "title": "Top Errors",
        "type": "table",
        "targets": [
          {
            "expr": "topk(5, count_over_time(error_logs_total[1h]))",
            "legendFormat": "{{error_type}}"
          }
        ],
        "gridPos": {"x": 18, "y": 16, "w": 6, "h": 8}
      }
    ],
    "templating": {
      "list": [
        {
          "name": "service",
          "label": "Service",
          "type": "query",
          "query": "label_values(service_name)",
          "multi": true,
          "includeAll": true
        },
        {
          "name": "environment",
          "label": "Environment",
          "type": "query",
          "query": "label_values(environment)",
          "multi": false,
          "includeAll": false
        }
      ]
    },
    "time": {
      "from": "now-1h",
      "to": "now"
    },
    "refresh": "30s"
  },
  "message": "Dashboard created",
  "timestamp": "$TIMESTAMP"
}
EOF
        echo "✅ Generated Grafana dashboard: $OUTPUT_FILE" >&2
        ;;
    
    datadog)
        cat << EOF > "$OUTPUT_FILE"
{
  "dashboard": {
    "title": "Observability Dashboard",
    "description": "Comprehensive observability dashboard",
    "layout_type": "ordered",
    "widgets": [
      {
        "definition": {
          "title": "Error Rate",
          "type": "timeseries",
          "requests": [
            {
              "q": "avg:error_rate{*}.as_rate()"
            }
          ]
        }
      },
      {
        "definition": {
          "title": "Log Stream",
          "type": "log_stream",
          "indexes": ["*"],
          "query": "status:error"
        }
      },
      {
        "definition": {
          "title": "Service Map",
          "type": "topology_map",
          "requests": [
            {
              "query": {
                "data_source": "service_map",
                "service": "",
                "env": ""
              }
            }
          ]
        }
      }
    ]
  }
}
EOF
        echo "✅ Generated Datadog dashboard: $OUTPUT_FILE" >&2
        ;;
    
    config)
        # Generate observability configuration
        cat << EOF > "observability-config.yaml"
# Observability configuration
observability:
  pillars:
    logs:
      collection: fluentd
      storage: elasticsearch
      retention_days: 30
      
    metrics:
      collection: prometheus
      storage: prometheus
      retention_days: 15
      
    traces:
      collection: otel-collector
      storage: jaeger
      retention_days: 7
  
  correlation:
    fields:
      trace_id: ["trace_id", "trace.id"]
      service: ["service", "service.name"]
      environment: ["env", "environment"]
    
    rules:
      - when: "log.level == 'ERROR'"
        then: "increment_metric('errors_total')"
      - when: "trace.duration > 1000"
        then: "log.warning('slow_trace')"
  
  dashboards:
    - name: "Service Health"
      type: "grafana"
      refresh: "30s"
      panels:
        - error_rate
        - latency
        - throughput
        - error_logs
  
  alerting:
    rules:
      - alert: "HighErrorRate"
        condition: "error_rate > 0.05"
        duration: "5m"
        severity: "critical"
        channels: ["slack", "pagerduty"]
      
      - alert: "SLOViolationImminent"
        condition: "error_budget_remaining < 0.1"
        duration: "1h"
        severity: "warning"
        channels: ["slack"]
  
  slo:
    targets:
      - service: "api-service"
        slo: 0.999
        sli: "error_rate"
        alert_burn_rate: 10
EOF
        echo "✅ Generated observability configuration: observability-config.yaml" >&2
        OUTPUT_FILE="observability-config.yaml"
        ;;
    
    *)
        echo "Unknown dashboard type: $DASHBOARD_TYPE. Supported: grafana, datadog, config" >&2
        exit 1
        ;;
esac

echo "" >&2
echo "Observability Dashboard Overview:" >&2
echo "-------------------------------" >&2
echo "Combines three pillars of observability:" >&2
echo "1. Logs: Error logs stream, error rate analysis" >&2
echo "2. Metrics: Latency, throughput, SLO burn rate" >&2
echo "3. Traces: Service map, trace waterfall views" >&2
echo "" >&2
echo "Generated files:" >&2
echo "  - $OUTPUT_FILE (dashboard configuration)" >&2
if [ "$DASHBOARD_TYPE" = "grafana" ]; then
    echo "  - observability-config.yaml (optional configuration)" >&2
fi
echo "" >&2
echo "Next steps:" >&2
echo "1. Import dashboard into your observability platform" >&2
echo "2. Configure data sources for logs, metrics, and traces" >&2
echo "3. Customize panels for your specific services" >&2
echo "4. Set up alerting based on dashboard metrics" >&2

# Output JSON status
echo '{"status": "created", "service": "observability-logging", "timestamp": "'"$TIMESTAMP"'", "dashboard_type": "'"$DASHBOARD_TYPE"'", "output_file": "'"$OUTPUT_FILE"'", "services": "'"$SERVICES"'"}'