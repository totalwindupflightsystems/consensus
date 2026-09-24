# Observability Logging Reference

## Overview
Observability logging integrates logs with metrics, traces, alerts, and dashboards to create comprehensive system understanding. This reference covers patterns, tools, and practices for building observable systems using logs as a core component.

## The Three Pillars of Observability

### 1. Logs
**What happened**: Discrete events with context and timestamps.

#### Log Types for Observability:
- **Application Logs**: Business logic, errors, user actions
- **Infrastructure Logs**: System metrics, resource usage, health checks
- **Audit Logs**: Security events, access patterns, compliance data
- **Performance Logs**: Timing information, bottlenecks, slow operations

### 2. Metrics
**How much/how often**: Numeric measurements over time.

#### Log-Driven Metrics:
```python
def extract_metrics_from_logs(logs):
    metrics = {
        'error_rate': 0,
        'request_count': 0,
        'latency_p95': 0,
        'throughput': 0
    }
    
    error_count = sum(1 for log in logs if log['level'] == 'ERROR')
    metrics['error_rate'] = error_count / len(logs) if logs else 0
    
    # Extract latency from structured logs
    latencies = [log.get('duration_ms', 0) for log in logs if 'duration_ms' in log]
    if latencies:
        metrics['latency_p95'] = np.percentile(latencies, 95)
    
    return metrics
```

### 3. Traces
**Where in the flow**: Request flow across service boundaries.

#### Log-Enhanced Tracing:
```json
{
  "timestamp": "2026-02-26T18:00:00Z",
  "level": "INFO",
  "trace_id": "0af7651916cd43dd8448eb211c80319c",
  "span_id": "b7ad6b7169203331",
  "span_name": "process_payment",
  "event": "span_start",
  "attributes": {
    "order_id": "ord-789",
    "payment_method": "credit_card",
    "amount": 99.99
  },
  "metrics": {
    "start_time": 1677427200000,
    "cpu_usage_before": 45.2,
    "memory_before_mb": 123.4
  }
}
```

## Integration Patterns

### 1. Correlation Across Pillars

#### Correlation Fields:
```yaml
correlation_fields:
  # Trace context
  trace_id: ["trace_id", "trace.id", "X-Trace-Id"]
  span_id: ["span_id", "span.id"]
  
  # Service context
  service: ["service", "service.name", "component"]
  environment: ["env", "environment", "deployment.env"]
  
  # Business context
  user_id: ["user_id", "user.id", "userId"]
  session_id: ["session_id", "session.id"]
  request_id: ["request_id", "req.id", "X-Request-Id"]
  
  # Temporal context
  timestamp: ["@timestamp", "timestamp", "time"]
```

#### Correlation Implementation:
```python
class ObservabilityCorrelator:
    def correlate_data(self, logs, metrics, traces):
        """Correlate data across observability pillars."""
        correlated = []
        
        # Create time windows for correlation
        time_windows = self.create_time_windows(logs, metrics, traces)
        
        for window in time_windows:
            # Find related data
            window_logs = self.filter_by_time(logs, window)
            window_metrics = self.filter_by_time(metrics, window)
            window_traces = self.filter_by_time(traces, window)
            
            # Correlate by trace_id
            for trace in window_traces:
                trace_id = trace.get('trace_id')
                related_logs = [log for log in window_logs 
                              if log.get('trace_id') == trace_id]
                related_metrics = self.find_metrics_for_trace(trace_id, window_metrics)
                
                correlated.append({
                    'trace': trace,
                    'logs': related_logs,
                    'metrics': related_metrics,
                    'window': window
                })
        
        return correlated
```

### 2. Log-Driven Metrics Generation

#### Real-time Metric Extraction:
```python
from prometheus_client import Counter, Histogram, Gauge

# Define metrics
ERROR_COUNTER = Counter('app_errors_total', 'Total application errors', ['service', 'error_type'])
REQUEST_DURATION = Histogram('request_duration_seconds', 'Request duration', ['service', 'endpoint'])
THROUGHPUT_GAUGE = Gauge('requests_per_second', 'Request throughput', ['service'])

def extract_metrics_from_log(log_entry):
    """Extract metrics from log entries in real-time."""
    if log_entry.get('level') == 'ERROR':
        ERROR_COUNTER.labels(
            service=log_entry.get('service', 'unknown'),
            error_type=log_entry.get('error_type', 'unknown')
        ).inc()
    
    if 'duration_ms' in log_entry:
        REQUEST_DURATION.labels(
            service=log_entry.get('service', 'unknown'),
            endpoint=log_entry.get('endpoint', 'unknown')
        ).observe(log_entry['duration_ms'] / 1000)
    
    if log_entry.get('event') == 'request_completed':
        THROUGHPUT_GAUGE.labels(
            service=log_entry.get('service', 'unknown')
        ).set(self.calculate_throughput())
```

#### Batch Metric Processing:
```bash
# Use Fluentd to extract metrics from logs
<source>
  @type tail
  path /var/log/app/*.log
  tag app.logs
</source>

<filter app.logs>
  @type prometheus
  <metric>
    name app_errors_total
    type counter
    desc Total application errors
    key level
    <labels>
      service ${service}
      error_type ${error_type}
    </labels>
  </metric>
</filter>
```

### 3. Unified Dashboards

#### Grafana Dashboard Configuration:
```yaml
dashboard:
  title: "Service Observability"
  panels:
    - title: "Error Rate (Logs + Metrics)"
      type: "timeseries"
      targets:
        - expr: "rate(app_errors_total[5m])"
          legendFormat: "{{service}}"
        - expr: "error_rate_from_logs"
          legendFormat: "Log-based"
    
    - title: "Request Flow (Traces)"
      type: "nodegraph"
      targets:
        - query: "traces_by_service"
    
    - title: "Recent Errors (Logs)"
      type: "logs"
      targets:
        - query: "level:ERROR"
          limit: 50
    
    - title: "Resource Usage (Metrics)"
      type: "timeseries"
      targets:
        - expr: "container_memory_usage_bytes"
        - expr: "container_cpu_usage_seconds_total"
```

#### Datadog Integration:
```yaml
logs:
  - type: file
    path: /var/log/app/*.log
    service: app-service
    source: python
    
metrics:
  - type: log_derived
    logs_query: "service:app-service level:ERROR"
    name: app.errors
    type: count
    
dashboards:
  - name: "Full Observability"
    layout_type: "ordered"
    widgets:
      - type: "timeseries"
        title: "Cross-pillar View"
        requests:
          - q: "logs('service:app-service').rollup('count').as_count()"
            display_type: "bars"
          - q: "avg:app.errors{*}.rollup('avg')"
            display_type: "line"
```

## SLO Monitoring with Logs

### Error Budget Calculation

#### From Logs:
```python
class SLOCalculator:
    def calculate_error_budget(self, logs, slo_target, window_hours):
        """
        Calculate error budget from log data.
        
        Args:
            logs: List of log entries with success/failure indicators
            slo_target: Target success rate (e.g., 0.999 for 99.9%)
            window_hours: Time window in hours
        
        Returns:
            Error budget consumption and remaining
        """
        total_requests = len(logs)
        successful = sum(1 for log in logs if self.is_successful(log))
        
        success_rate = successful / total_requests if total_requests > 0 else 1.0
        error_rate = 1.0 - success_rate
        
        # Error budget calculation
        allowed_errors = (1.0 - slo_target) * total_requests
        actual_errors = total_requests - successful
        budget_consumed = actual_errors / allowed_errors if allowed_errors > 0 else float('inf')
        budget_remaining = max(0, 1.0 - budget_consumed)
        
        return {
            'total_requests': total_requests,
            'successful': successful,
            'success_rate': success_rate,
            'error_rate': error_rate,
            'slo_target': slo_target,
            'allowed_errors': allowed_errors,
            'actual_errors': actual_errors,
            'budget_consumed': budget_consumed,
            'budget_remaining': budget_remaining,
            'burn_rate': budget_consumed / (window_hours / 24 * 7)  # Weekly burn rate
        }
```

#### Burn Rate Alerting:
```yaml
alerting:
  error_budget:
    rules:
      - alert: "HighErrorBudgetBurnRate"
        expr: "error_budget_burn_rate > 10"
        for: "1h"
        labels:
          severity: "critical"
          team: "sre"
        annotations:
          summary: "Error budget burning too fast"
          description: |
            Error budget burn rate is {{ $value }}x faster than allowed.
            At this rate, error budget will be exhausted in {{ $value }} hours.
          
      - alert: "SLOViolationImminent"
        expr: "error_budget_remaining < 0.1"
        for: "6h"
        labels:
          severity: "warning"
        annotations:
          summary: "SLO violation imminent"
          description: "Only {{ $value * 100 }}% error budget remaining"
```

### Automated Remediation

#### Pattern-Based Remediation:
```python
class LogDrivenRemediation:
    def analyze_and_remediate(self, log_patterns):
        """Analyze log patterns and trigger remediation."""
        for pattern in log_patterns:
            if pattern['type'] == 'high_error_rate':
                if self.should_scale_out(pattern):
                    self.scale_out_service(pattern['service'])
                    
            elif pattern['type'] == 'database_slow_queries':
                self.add_database_index(pattern['query_pattern'])
                
            elif pattern['type'] == 'memory_leak':
                self.restart_service(pattern['service'], graceful=True)
                
            elif pattern['type'] == 'dependency_down':
                self.enable_circuit_breaker(pattern['dependency'])
                self.failover_to_backup(pattern['dependency'])
```

## Tools & Platforms

### Open Source Stack:
| Component | Options | Integration Method |
|-----------|---------|-------------------|
| **Log Collection** | Fluentd, Filebeat, Vector | Correlate via trace_id |
| **Metrics** | Prometheus, VictoriaMetrics | Extract from logs |
| **Traces** | Jaeger, Tempo, OpenTelemetry | Include in logs |
| **Visualization** | Grafana, Kibana | Unified dashboards |
| **Alerting** | Alertmanager, Grafana Alerts | Log pattern detection |

### Commercial Platforms:
- **Datadog**: Unified observability platform
- **New Relic**: Full-stack observability
- **Splunk**: Log-focused with metrics and traces
- **Dynatrace**: AI-powered observability
- **AppDynamics**: Application performance monitoring

### Hybrid Approach Example:
```yaml
observability_stack:
  logs:
    collector: fluentd
    storage: elasticsearch
    retention: 30 days
    
  metrics:
    collector: prometheus
    storage: thanos
    retention: 90 days
    
  traces:
    collector: otel-collector
    storage: tempo
    retention: 7 days
    
  correlation:
    enabled: true
    fields: ["trace_id", "service", "environment"]
    
  dashboards:
    provider: grafana
    refresh: 30s
    unified: true
```

## Implementation Patterns

### 1. Centralized Correlation Service
```python
class ObservabilityCorrelationService:
    def __init__(self):
        self.log_store = LogStore()
        self.metric_store = MetricStore()
        self.trace_store = TraceStore()
    
    def correlate_request(self, request_id):
        """Correlate all observability data for a request."""
        # Get logs for request
        logs = self.log_store.get_by_request_id(request_id)
        
        # Get metrics for the time period
        time_range = self.get_time_range_from_logs(logs)
        metrics = self.metric_store.get_by_time_range(time_range)
        
        # Get traces
        trace_id = self.extract_trace_id_from_logs(logs)
        traces = self.trace_store.get_by_trace_id(trace_id) if trace_id else []
        
        # Correlate and return unified view
        return self.create_unified_view(logs, metrics, traces)
    
    def create_unified_view(self, logs, metrics, traces):
        return {
            'request_timeline': self.create_timeline(logs, traces),
            'performance_metrics': self.extract_performance_metrics(logs, metrics),
            'error_analysis': self.analyze_errors(logs),
            'dependency_graph': self.create_dependency_graph(traces),
            'recommendations': self.generate_recommendations(logs, metrics, traces)
        }
```

### 2. Real-time Observability Pipeline
```
Application → Logs → [Fluentd] → [Elasticsearch]
                    ↓ Extract    ↓ Correlate
                Metrics → [Prometheus] → [Grafana]
                    ↑                  ↗
                Traces → [Jaeger] → [Tempo]
```

#### Pipeline Configuration:
```yaml
pipeline:
  stages:
    - name: "collection"
      components:
        - type: "fluentd"
          config:
            sources: ["app", "infra", "audit"]
            buffer_size: "100MB"
            
    - name: "processing"
      components:
        - type: "log_enricher"
          adds: ["trace_id", "service_name", "environment"]
        - type: "metric_extractor"
          extracts: ["error_rate", "latency", "throughput"]
        - type: "correlator"
          correlation_fields: ["trace_id", "request_id"]
          
    - name: "storage"
      components:
        - type: "elasticsearch"
          index: "logs-observability"
        - type: "prometheus"
          retention: "90d"
        - type: "jaeger"
          retention: "7d"
          
    - name: "visualization"
      components:
        - type: "grafana"
          dashboards: ["unified-observability"]
        - type: "kibana"
          saved_objects: ["correlation-analysis"]
```

## Best Practices

### 1. Structured Logging for Observability
```json
{
  "timestamp": "2026-02-26T18:00:00Z",
  "level": "INFO",
  "service": "payment-service",
  "component": "payment-processor",
  "trace_id": "0af7651916cd43dd8448eb211c80319c",
  "span_id": "b7ad6b7169203331",
  "user_id": "user-123",
  "session_id": "session-456",
  "request_id": "req-789",
  "event": "payment_processed",
  "metrics": {
    "duration_ms": 123,
    "cpu_usage": 45.2,
    "memory_mb": 123.4
  },
  "context": {
    "order_id": "ord-789",
    "amount": 99.99,
    "currency": "USD"
  },
  "environment": "production",
  "version": "1.2.3"
}
```

### 2. Correlation Field Standardization
```yaml
observability_fields:
  mandatory:
    - timestamp: ISO 8601 format
    - service: service name
    - environment: dev/staging/prod
    - trace_id: W3C Trace Context format
    
  recommended:
    - span_id: current span identifier
    - parent_span_id: parent span identifier
    - user_id: user identifier
    - session_id: session identifier
    - request_id: request identifier
    
  optional:
    - component: sub-component name
    - feature: feature name
    - version: application version
```

### 3. Performance Optimization
- **Sampling**: Sample DEBUG/INFO logs, keep all ERROR logs
- **Async Processing**: Don't block application threads
- **Batch Operations**: Process logs in batches
- **Compression**: Compress old logs
- **Tiered Storage**: Hot/Warm/Cold storage based on access patterns

## Monitoring Observability Itself

### Key Metrics:
```prometheus
# Observability system health
observability_logs_collected_total
observability_logs_processed_total
observability_logs_correlated_total
observability_correlation_latency_seconds
observability_storage_usage_bytes

# Data quality
observability_logs_parsing_success_rate
observability_correlation_success_rate
observability_data_freshness_seconds

# Performance
observability_processing_latency_seconds
observability_queue_size
observability_drop_rate
```

### Alerting on Observability Issues:
```yaml
groups:
  - name: observability_health
    rules:
      - alert: ObservabilityDataStale
        expr: observability_data_freshness_seconds > 300
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Observability data is stale"
          
      - alert: HighObservabilityDropRate
        expr: rate(observability_logs_dropped_total[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High observability data drop rate"
```

## Implementation Checklist

- [ ] Define observability data model
- [ ] Implement structured logging with correlation fields
- [ ] Set up log collection and processing pipeline
- [ ] Configure metric extraction from logs
- [ ] Implement trace context propagation
- [ ] Build correlation service
- [ ] Create unified dashboards
- [ ] Set up SLO monitoring with error budgets
- [ ] Implement alerting and remediation
- [ ] Monitor observability system health
- [ ] Document standards and practices
- [ ] Train teams on observability practices
- [ ] Conduct observability maturity assessment
- [ ] Continuously optimize and improve

## Common Challenges & Solutions

### Challenge: Data silos between logs, metrics, traces
**Solution**: Implement correlation service
```python
correlation_service = CorrelationService()
unified_view = correlation_service.get_unified_view(
    trace_id=trace_id,
    time_window='5m'
)
```

### Challenge: High volume of observability data
**Solution**: Implement sampling and tiered storage
```yaml
sampling:
  logs:
    DEBUG: 0.01  # 1%
    INFO: 0.1    # 10%
    WARN: 0.5    # 50%
    ERROR: 1.0   # 100%
    
storage:
  hot: 7 days
  warm: 30 days
  cold: 365 days
```

### Challenge: Complex correlation queries
**Solution**: Pre-compute correlations
```python
# Pre-compute common correlations
class PrecomputedCorrelations:
    def precompute(self):
        # Correlate errors with traces
        errors = self.get_recent_errors()
        for error in errors:
            trace_id = error.get('trace_id')
            if trace_id:
                trace = self.get_trace(trace_id)
                self.store_correlation(error['id'], 'trace', trace)
        
        # Correlate slow requests with resource metrics
        slow_requests = self.get_slow_requests()
        for request in slow_requests:
            metrics = self.get_metrics_for_time(request['timestamp'])
            self.store_correlation(request['id'], 'metrics', metrics)
```

## Further Reading
- [Observability Engineering](https://www.example.com/observability-engineering)
- [The Three Pillars of Observability](https://www.example.com/three-pillars)
- [Log-Driven Architecture](https://www.example.com/log-driven-architecture)
- [SLO Implementation Guide](https://www.example.com/slo-implementation)
- [Unified Observability Patterns](https://www.example.com/unified-observability)