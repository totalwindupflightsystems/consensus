# Distributed Tracing with Logs Reference

## Overview
Distributed tracing with logs enables end-to-end request tracking across microservices and distributed systems by propagating trace context through logs. This reference covers implementation patterns, standards, and best practices.

## Core Concepts

### Trace Context Propagation
Trace context carries trace and span identifiers across service boundaries.

#### W3C Trace Context Standard
```
traceparent: 00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01
tracestate: congo=t61rcWkgMzE
```

**Header Format:**
- `traceparent`: `00-{trace-id}-{span-id}-{trace-flags}`
  - Version: `00` (current version)
  - Trace ID: 32 hex characters (16 bytes)
  - Span ID: 16 hex characters (8 bytes)
  - Trace Flags: 2 hex characters (sampling flag, etc.)

#### Propagation Methods:

**HTTP Headers:**
```http
GET /api/users HTTP/1.1
Host: api.example.com
Traceparent: 00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01
X-Correlation-Id: tx-123456
X-Request-Id: req-789012
```

**Message Queues (Kafka, RabbitMQ):**
```json
{
  "headers": {
    "traceparent": "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
    "correlation_id": "tx-123456"
  },
  "body": {
    "order_id": "ord-789",
    "amount": 99.99
  }
}
```

**Database Operations:**
```sql
-- Include trace context in audit fields
INSERT INTO orders (id, amount, trace_id, span_id, created_at)
VALUES ('ord-789', 99.99, '0af7651916cd43dd8448eb211c80319c', 'b7ad6b7169203331', NOW());
```

### Span Logging
Logs should include trace context for correlation.

#### Structured Log Format:
```json
{
  "timestamp": "2026-02-26T18:00:00Z",
  "level": "INFO",
  "trace_id": "0af7651916cd43dd8448eb211c80319c",
  "span_id": "b7ad6b7169203331",
  "parent_span_id": "c8be7c825a934b7d",
  "span_name": "process_payment",
  "span_kind": "SERVER",
  "event": "span_start",
  "duration_ms": 0,
  "attributes": {
    "order_id": "ord-789",
    "payment_method": "credit_card",
    "amount": 99.99
  }
}
```

## Implementation Patterns

### 1. Middleware/Interceptor Pattern
Automatically inject and extract trace context.

#### Node.js Express Example:
```javascript
const { trace, context } = require('@opentelemetry/api');

function tracingMiddleware(req, res, next) {
  // Extract trace context from headers
  const carrier = { traceparent: req.headers.traceparent };
  const extractedContext = trace.extract(context.active(), carrier);
  
  // Start new span
  const span = trace.getTracer('http').startSpan(
    `${req.method} ${req.path}`,
    {
      kind: trace.SpanKind.SERVER,
      attributes: {
        'http.method': req.method,
        'http.path': req.path,
        'http.route': req.route?.path
      }
    },
    extractedContext
  );
  
  // Store span in request context
  req.span = span;
  
  // Add trace context to response headers
  res.setHeader('traceparent', trace.getTraceParent(span));
  
  // End span when response finishes
  res.on('finish', () => {
    span.setAttribute('http.status_code', res.statusCode);
    span.end();
  });
  
  next();
}
```

#### Python Flask Example:
```python
from opentelemetry import trace
from opentelemetry.trace.propagation.tracecontext import TraceContextTextMapPropagator
from opentelemetry.instrumentation.flask import FlaskInstrumentor

propagator = TraceContextTextMapPropagator()

@app.before_request
def start_trace():
    # Extract trace context from headers
    carrier = {'traceparent': request.headers.get('traceparent')}
    ctx = propagator.extract(carrier)
    
    # Start span
    tracer = trace.get_tracer(__name__)
    span = tracer.start_span(
        f"{request.method} {request.path}",
        context=ctx,
        kind=trace.SpanKind.SERVER
    )
    
    # Add to request context
    g.span = span
    
    # Set attributes
    span.set_attribute("http.method", request.method)
    span.set_attribute("http.path", request.path)

@app.after_request
def end_trace(response):
    if hasattr(g, 'span'):
        g.span.set_attribute("http.status_code", response.status_code)
        g.span.end()
    return response
```

### 2. Logging Integration
Inject trace context into log statements.

#### Log4j2 with OpenTelemetry:
```xml
<Configuration>
  <Appenders>
    <Console name="Console" target="SYSTEM_OUT">
      <JsonLayout complete="true" compact="false">
        <KeyValuePair key="trace_id" value="$${ctx:traceId}"/>
        <KeyValuePair key="span_id" value="$${ctx:spanId}"/>
      </JsonLayout>
    </Console>
  </Appenders>
  
  <Loggers>
    <Root level="INFO">
      <AppenderRef ref="Console"/>
    </Root>
  </Loggers>
</Configuration>
```

#### Python Logging with Context:
```python
import logging
from opentelemetry import trace

class TracingLogHandler(logging.Handler):
    def emit(self, record):
        # Get current span
        current_span = trace.get_current_span()
        
        if current_span.is_recording():
            # Add trace context to log record
            record.trace_id = format_trace_id(current_span.get_span_context().trace_id)
            record.span_id = format_span_id(current_span.get_span_context().span_id)
        
        # Call parent emit
        super().emit(record)

# Configure logging
logger = logging.getLogger(__name__)
handler = TracingLogHandler()
logger.addHandler(handler)
```

### 3. Async Operations
Propagate trace context through async code.

#### JavaScript Async/Await:
```javascript
async function processOrder(orderId) {
  const tracer = trace.getTracer('order-service');
  const span = tracer.startSpan('processOrder');
  
  try {
    // Propagate context through async operations
    await context.with(trace.setSpan(context.active(), span), async () => {
      await validateOrder(orderId);
      await chargePayment(orderId);
      await sendConfirmation(orderId);
    });
    
    span.setStatus({ code: SpanStatusCode.OK });
  } catch (error) {
    span.recordException(error);
    span.setStatus({ code: SpanStatusCode.ERROR, message: error.message });
    throw error;
  } finally {
    span.end();
  }
}
```

#### Python Async:
```python
import asyncio
from opentelemetry import trace
from opentelemetry.trace import Status, StatusCode

async def process_order(order_id):
    tracer = trace.get_tracer(__name__)
    
    with tracer.start_as_current_span("process_order") as span:
        span.set_attribute("order_id", order_id)
        
        try:
            # Async operations maintain context
            await validate_order(order_id)
            await charge_payment(order_id)
            await send_confirmation(order_id)
            
            span.set_status(Status(StatusCode.OK))
        except Exception as e:
            span.record_exception(e)
            span.set_status(Status(StatusCode.ERROR, str(e)))
            raise
```

## OpenTelemetry Integration

### Configuration

#### Basic OpenTelemetry Setup:
```yaml
# otel-config.yaml
opentelemetry:
  service:
    name: "payment-service"
    version: "1.0.0"
  
  tracing:
    enabled: true
    sampler:
      type: "parentbased_traceidratio"
      ratio: 0.1  # Sample 10% of traces
    
    exporters:
      - type: "otlp"
        endpoint: "http://otel-collector:4317"
      - type: "logging"
        level: "debug"
    
    propagators:
      - "tracecontext"
      - "baggage"
  
  resource:
    attributes:
      service.name: "payment-service"
      service.version: "1.0.0"
      deployment.environment: "production"
```

#### Auto-instrumentation:
```bash
# Node.js
NODE_OPTIONS="--require @opentelemetry/auto-instrumentations/register" node app.js

# Python
opentelemetry-instrument python app.py

# Java
java -javaagent:opentelemetry-javaagent.jar -jar app.jar
```

### Manual Instrumentation Examples

#### Go:
```go
package main

import (
	"context"
	"log"
	
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func processPayment(ctx context.Context, orderID string, amount float64) error {
	tracer := otel.Tracer("payment-service")
	
	ctx, span := tracer.Start(ctx, "processPayment")
	defer span.End()
	
	span.SetAttributes(
		attribute.String("order.id", orderID),
		attribute.Float64("amount", amount),
	)
	
	// Business logic
	if err := chargeCard(orderID, amount); err != nil {
		span.RecordError(err)
		return err
	}
	
	return nil
}
```

#### Java Spring Boot:
```java
import io.opentelemetry.api.OpenTelemetry;
import io.opentelemetry.api.trace.Span;
import io.opentelemetry.api.trace.Tracer;
import io.opentelemetry.context.Scope;

@Service
public class OrderService {
    private final Tracer tracer;
    
    public OrderService(OpenTelemetry openTelemetry) {
        this.tracer = openTelemetry.getTracer("order-service");
    }
    
    public Order processOrder(OrderRequest request) {
        Span span = tracer.spanBuilder("processOrder")
            .setAttribute("order.id", request.getId())
            .setAttribute("order.amount", request.getAmount())
            .startSpan();
        
        try (Scope scope = span.makeCurrent()) {
            // Business logic
            Order order = createOrder(request);
            chargePayment(order);
            sendConfirmation(order);
            
            span.addEvent("order.completed");
            return order;
        } catch (Exception e) {
            span.recordException(e);
            span.setStatus(Status.ERROR);
            throw e;
        } finally {
            span.end();
        }
    }
}
```

## Log-Trace Correlation

### Correlation Strategies

1. **Direct Inclusion**: Include trace_id in every log entry
2. **Mapped Diagnostic Context (MDC)**: Thread-local storage for trace context
3. **Structured Logging**: JSON logs with trace fields
4. **Log Enrichment**: Post-processing to add trace context

#### Log Enrichment Example:
```python
def enrich_logs_with_trace(logs, trace_data):
    """Enhance logs with trace context from tracing system."""
    enriched_logs = []
    
    for log in logs:
        if 'trace_id' in log:
            trace_id = log['trace_id']
            trace_info = trace_data.get(trace_id, {})
            
            # Add trace context to log
            log['trace_context'] = {
                'trace_id': trace_id,
                'span_id': trace_info.get('span_id'),
                'parent_span_id': trace_info.get('parent_span_id'),
                'service_name': trace_info.get('service_name'),
                'duration_ms': trace_info.get('duration_ms')
            }
        
        enriched_logs.append(log)
    
    return enriched_logs
```

### Querying Correlated Logs

#### Elasticsearch Query:
```json
{
  "query": {
    "bool": {
      "must": [
        { "term": { "trace_id": "0af7651916cd43dd8448eb211c80319c" } },
        { "range": { "@timestamp": { "gte": "now-1h" } } }
      ]
    }
  },
  "sort": [{ "@timestamp": "asc" }]
}
```

#### SQL Query for Database Logs:
```sql
SELECT 
  l.timestamp,
  l.level,
  l.message,
  t.trace_id,
  t.span_id,
  t.service_name
FROM logs l
JOIN traces t ON l.trace_id = t.trace_id
WHERE t.trace_id = '0af7651916cd43dd8448eb211c80319c'
ORDER BY l.timestamp;
```

## Sampling Strategies

### Sampling Types:
1. **Always On**: Sample all traces (high volume)
2. **Probability**: Sample X% of traces
3. **Rate Limiting**: Sample up to N traces per second
4. **Parent-Based**: Respect parent's sampling decision
5. **Adaptive**: Adjust sampling based on system load

#### OpenTelemetry Sampling Configuration:
```yaml
sampling:
  # Parent-based probability sampling
  parentbased_traceidratio:
    root: 0.01  # 1% of root spans
    remote_parent_sampled: 1.0  # 100% if parent sampled
    remote_parent_not_sampled: 0.0  # 0% if parent not sampled
    local_parent_sampled: 1.0
    local_parent_not_sampled: 0.0
  
  # Custom sampling logic
  custom:
    rules:
      - condition: "attributes['http.path'] == '/health'"
        sample: false  # Don't sample health checks
      - condition: "attributes['http.status_code'] >= 500"
        sample: true   # Always sample errors
      - condition: "attributes['user.id'] == 'admin'"
        sample: true   # Always sample admin actions
```

## Performance Considerations

### Overhead Management
1. **Sampling**: Reduces volume significantly
2. **Async Export**: Don't block application threads
3. **Batch Export**: Send traces in batches
4. **Buffer Management**: Handle backpressure gracefully

#### Performance Optimization Example:
```java
@Configuration
public class TracingConfig {
    
    @Bean
    public OpenTelemetry openTelemetry() {
        SdkTracerProvider tracerProvider = SdkTracerProvider.builder()
            .addSpanProcessor(
                BatchSpanProcessor.builder(
                    OtlpGrpcSpanExporter.builder()
                        .setEndpoint("http://otel-collector:4317")
                        .build()
                )
                .setScheduleDelay(5, TimeUnit.SECONDS)  // Batch every 5 seconds
                .setMaxExportBatchSize(512)  // Max batch size
                .setMaxQueueSize(2048)  // Buffer size
                .build()
            )
            .setSampler(
                Sampler.parentBased(
                    Sampler.traceIdRatioBased(0.1)  // Sample 10%
                )
            )
            .build();
        
        return OpenTelemetrySdk.builder()
            .setTracerProvider(tracerProvider)
            .build();
    }
}
```

## Monitoring & Alerting

### Key Metrics to Monitor:
1. **Trace collection rate**: traces/second
2. **Trace export success rate**: percentage successful
3. **Trace latency**: time from creation to export
4. **Sampling rate**: actual vs configured
5. **Error rate**: failed traces or exports

#### Prometheus Metrics Example:
```yaml
# otel-collector metrics configuration
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
  
processors:
  batch:
    timeout: 5s
  
exporters:
  prometheus:
    endpoint: "0.0.0.0:8889"
    namespace: "otel"
  
service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [batch]
      exporters: [prometheus]
```

### Alerting Rules:
```yaml
groups:
  - name: tracing_alerts
    rules:
      - alert: HighTraceDropRate
        expr: rate(otel_traces_dropped_total[5m]) > 0.1
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "High trace drop rate"
          description: "{{ $value }}% of traces are being dropped"
      
      - alert: TraceExportFailure
        expr: rate(otel_traces_export_failed_total[5m]) > 0.05
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Trace export failures"
          description: "{{ $value }}% of trace exports are failing"
```

## Implementation Checklist

- [ ] Choose tracing standard (W3C TraceContext, OpenTelemetry)
- [ ] Implement context propagation middleware
- [ ] Integrate trace context with logging
- [ ] Configure sampling strategy
- [ ] Set up trace exporters
- [ ] Implement async/batch processing
- [ ] Add trace context to database operations
- [ ] Propagate context through message queues
- [ ] Set up monitoring and alerting
- [ ] Document tracing standards and practices
- [ ] Train development team
- [ ] Test end-to-end tracing
- [ ] Review and optimize performance

## Common Issues & Solutions

### Issue: Trace context lost in async operations
**Solution**: Use context propagation libraries
```javascript
// Correct: Propagate context through async/await
async function withContext(fn) {
  const context = getCurrentContext();
  return await context.with(context, fn);
}
```

### Issue: High tracing overhead
**Solution**: Implement sampling
```yaml
sampling:
  rate: 0.01  # 1% sampling in production
  adaptive: true  # Adjust based on load
```

### Issue: Trace data too large
**Solution**: Limit span attributes and events
```java
Span span = tracer.spanBuilder("operation")
    .setAttribute("important.key", value)
    // Don't add large values as attributes
    .startSpan();
```

## Further Reading
- [W3C Trace Context Specification](https://www.w3.org/TR/trace-context/)
- [OpenTelemetry Documentation](https://opentelemetry.io/docs/)
- [Distributed Tracing in Practice](https://www.example.com/distributed-tracing-practice)
- [Tracing in Microservices](https://www.example.com/microservices-tracing)
- [Performance Implications of Tracing](https://www.example.com/tracing-performance)