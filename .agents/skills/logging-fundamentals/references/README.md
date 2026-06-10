# Logging Fundamentals Reference

## Overview

Logging is the process of recording events, errors, and informational messages from software applications and systems. Proper logging is critical for debugging, monitoring, auditing, and understanding system behavior. This reference covers fundamental logging concepts, best practices, and implementation patterns.

## Core Concepts

### Log Levels
Log levels indicate the severity or importance of log messages. Standard levels include:

#### DEBUG
- **Purpose**: Detailed information for diagnosing problems during development
- **When to use**: Tracing variable values, function entry/exit, detailed flow
- **Production**: Typically disabled or sampled (1-10% of messages)
- **Example**: `"Processing user request, userId: 123, parameters: {...}"`

#### INFO
- **Purpose**: Confirmation that things are working as expected
- **When to use**: Service startup, configuration loaded, user actions completed
- **Production**: Enabled for operational monitoring
- **Example**: `"User login successful, userId: 123"`

#### WARN
- **Purpose**: Indication that something unexpected happened but application continues
- **When to use**: Deprecated API usage, slow queries, approaching limits
- **Production**: Enabled for alerting on trends
- **Example**: `"Database query took 2000ms, threshold: 1000ms"`

#### ERROR
- **Purpose**: Error events that affect specific operation but not whole system
- **When to use**: Failed API calls, validation errors, external service failures
- **Production**: Enabled for immediate investigation
- **Example**: `"Payment gateway timeout after 5000ms"`

#### FATAL/CRITICAL
- **Purpose**: Severe errors causing application termination
- **When to use**: Unrecoverable system errors, data corruption, critical failures
- **Production**: Enabled for immediate alerting and response
- **Example**: `"Database connection pool exhausted, shutting down"`

### Structured Logging
Structured logging outputs logs in a machine-readable format (typically JSON) with consistent fields rather than plain text messages.

#### Benefits:
1. **Machine parsability**: Easy to process, filter, and analyze
2. **Consistent schema**: Predictable field names and types
3. **Rich context**: Ability to include complex nested data
4. **Correlation**: Built-in fields for tracing and correlation
5. **Queryability**: Easy to search and aggregate by specific fields

#### Required Fields:
```json
{
  "timestamp": "ISO 8601 format with timezone",
  "level": "DEBUG|INFO|WARN|ERROR|FATAL",
  "service": "Application/service name",
  "component": "Module/component name",
  "message": "Human-readable message"
}
```

#### Recommended Additional Fields:
- `trace_id`: Distributed tracing identifier
- `span_id`: Individual operation identifier
- `user_id`: User identifier (if applicable)
- `session_id`: User session identifier
- `request_id`: HTTP request identifier
- `correlation_id`: Business transaction identifier
- `duration_ms`: Operation duration in milliseconds
- `environment`: Deployment environment
- `hostname`: Server hostname
- `version`: Application version
- `thread_id`: Execution thread identifier

### Context Propagation
Context propagation ensures relevant identifiers and metadata flow through distributed systems and async operations.

#### Propagation Methods:
1. **HTTP Headers**:
   ```http
   X-Trace-Id: abc123-def456
   X-Span-Id: def456
   X-Correlation-Id: business-tx-789
   X-User-Id: user-123
   ```

2. **Thread-local Storage**: For synchronous operations within a process
3. **Async Context**: For promises, callbacks, and background jobs
4. **Message Queues**: Include context in message metadata
5. **Database Transactions**: Store context in audit fields

#### Context Loss Prevention:
- **Middleware/AOP**: Automatically inject/extract context
- **Framework integration**: Use framework-provided context propagation
- **Manual propagation**: Explicitly pass context through function calls
- **Context managers**: Use context managers/scoped contexts

## Implementation Patterns

### Pattern 1: Logger Factory
```typescript
// Create logger with context
const logger = LoggerFactory.createLogger({
  service: 'payment-service',
  component: 'payment-processor',
  version: process.env.APP_VERSION,
  environment: process.env.NODE_ENV
});

// Usage with additional context
logger.info('Processing payment', {
  order_id: order.id,
  amount: order.amount,
  currency: order.currency
});
```

### Pattern 2: Context Middleware
```javascript
// Express middleware example
app.use((req, res, next) => {
  // Extract or generate context
  const traceId = req.headers['x-trace-id'] || generateTraceId();
  const userId = req.user?.id || 'anonymous';
  
  // Create request-scoped logger
  req.logger = logger.withContext({
    trace_id: traceId,
    user_id: userId,
    request_id: generateRequestId(),
    path: req.path,
    method: req.method
  });
  
  // Propagate to downstream services
  res.setHeader('X-Trace-Id', traceId);
  
  next();
});
```

### Pattern 3: Structured Error Logging
```python
try:
    process_payment(order)
except PaymentGatewayError as e:
    logger.error(
        "Payment gateway error",
        exc_info=e,
        extra={
            "order_id": order.id,
            "gateway": "stripe",
            "error_code": e.code,
            "retry_count": retry_count,
            "duration_ms": duration_ms
        }
    )
    raise
```

### Pattern 4: Log Sampling
```go
// Sample debug logs at 10% rate in production
func shouldLogDebug() bool {
    if level != DEBUG {
        return true
    }
    if environment == "production" {
        return rand.Float64() < 0.10 // 10% sampling
    }
    return true
}
```

## Configuration Examples

### Python (Structlog)
```python
import structlog

structlog.configure(
    processors=[
        structlog.processors.TimeStamper(fmt="iso"),
        structlog.processors.JSONRenderer(),
    ],
    context_class=dict,
    logger_factory=structlog.PrintLoggerFactory(),
    wrapper_class=structlog.BoundLogger,
    cache_logger_on_first_use=True,
)

log = structlog.get_logger()
log.info("payment.processed", order_id="ord_123", amount=99.99)
```

### Node.js (Winston/Pino)
```javascript
const pino = require('pino');

const logger = pino({
  level: process.env.LOG_LEVEL || 'info',
  timestamp: pino.stdTimeFunctions.isoTime,
  formatters: {
    level: (label) => ({ level: label }),
    bindings: () => ({
      service: 'payment-service',
      hostname: require('os').hostname(),
      version: process.env.npm_package_version
    })
  },
  serializers: {
    err: pino.stdSerializers.err,
    req: pino.stdSerializers.req,
    res: pino.stdSerializers.res
  }
});
```

### Java (Logback with Logstash)
```xml
<configuration>
  <appender name="JSON" class="ch.qos.logback.core.ConsoleAppender">
    <encoder class="net.logstash.logback.encoder.LogstashEncoder">
      <timestampPattern>ISO8601</timestampPattern>
      <fieldNames>
        <timestamp>timestamp</timestamp>
        <level>level</level>
        <logger>service</logger>
        <thread>thread</thread>
        <message>message</message>
        <stackTrace>stack_trace</stackTrace>
      </fieldNames>
      <customFields>{"service":"payment-service","environment":"production"}</customFields>
    </encoder>
  </appender>
  
  <root level="INFO">
    <appender-ref ref="JSON" />
  </root>
</configuration>
```

## Best Practices

### Do's:
1. **Use structured logging** for production systems
2. **Include correlation IDs** in all logs
3. **Log at appropriate levels** based on operational importance
4. **Include sufficient context** for debugging without external systems
5. **Use consistent field names** across the codebase
6. **Propagate context** through async operations and service boundaries
7. **Sample high-volume logs** (debug, trace) in production
8. **Monitor log volume and growth**
9. **Test logging configuration** in all environments
10. **Document logging standards** for the team

### Don'ts:
1. **Don't log sensitive information** (passwords, tokens, PII)
2. **Don't use string concatenation** for log messages (use structured fields)
3. **Don't enable debug logging** in production by default
4. **Don't ignore log performance** (async logging for high volume)
5. **Don't create log-only databases** (use proper log aggregation)
6. **Don't rely on console.log** for production logging
7. **Don't log the same error multiple times** (implement deduplication)
8. **Don't forget timezones** in timestamps (use UTC)
9. **Don't mix human and machine readable formats**
10. **Don't log without purpose** (every log should have a consumer)

## Performance Considerations

### Logging Overhead:
- **Synchronous logging**: Blocks execution until log is written
- **Asynchronous logging**: Non-blocking but uses memory buffers
- **Formatting overhead**: JSON serialization, timestamp formatting
- **I/O overhead**: File system or network writes

### Optimization Strategies:
1. **Async appenders**: Use async loggers for high-volume applications
2. **Buffering**: Buffer logs and write in batches
3. **Sampling**: Sample debug/trace logs in production
4. **Conditional logging**: Check log level before expensive operations
   ```python
   if logger.isEnabledFor(DEBUG):
       logger.debug(f"Expensive debug: {expensive_operation()}")
   ```
5. **Lazy evaluation**: Use lambda/callbacks for expensive message generation
6. **Rate limiting**: Limit log volume per component/time period

## Security Considerations

### PII (Personally Identifiable Information):
- **Never log**: Passwords, API keys, tokens, credit card numbers
- **Mask/redact**: Email addresses, phone numbers, IP addresses (partial)
- **Hash/obfuscate**: User IDs, session IDs (use one-way hashes)
- **Compliance**: Consider GDPR, CCPA, HIPAA requirements

### Secret Detection:
- Implement automated scanning for secrets in logs
- Use regex patterns for common secret formats
- Scan both development and production logs
- Alert on potential secret leakage

### Log Access Control:
- Restrict access to production logs
- Implement log encryption for sensitive data
- Use secure transport for log forwarding
- Regular audit of log access

## Environment-Specific Configuration

### Development:
```yaml
level: DEBUG
format: pretty (colored, human-readable)
output: console
context: full (including stack traces)
sampling: none
```

### Staging:
```yaml
level: INFO
format: structured (JSON)
output: console + file
context: standard (trace_id, user_id, etc.)
sampling: 100% debug, 10% trace
```

### Production:
```yaml
level: WARN (errors and warnings only)
format: structured (JSON)
output: file + aggregation service
context: essential (trace_id, error details)
sampling: 1% debug, 0.1% trace
```

## Monitoring and Maintenance

### Key Metrics to Monitor:
1. **Log volume**: Messages per second/minute
2. **Log level distribution**: Ratio of ERROR/WARN/INFO/DEBUG
3. **Log size growth**: Storage consumption over time
4. **Log latency**: Time from log call to persistence
5. **Error rate**: ERROR logs per time period
6. **Unique trace coverage**: Percentage of requests with trace logs

### Alerting Rules:
1. **Error spike**: >10 ERROR logs in 1 minute
2. **Warning trend**: Increasing WARN logs over 1 hour
3. **Debug enabled**: DEBUG logs in production environment
4. **Log storage full**: >90% storage capacity used
5. **Log pipeline failure**: No logs received for 5 minutes

### Regular Maintenance:
1. **Monthly**: Review log retention policies
2. **Quarterly**: Update log parsing rules for new fields
3. **Biannually**: Review and update PII detection patterns
4. **Annually**: Audit logging standards and compliance

## Tools and Libraries

### Popular Logging Libraries:
- **Python**: structlog, python-json-logger, loguru
- **Node.js**: pino, winston, bunyan
- **Java**: Logback, Log4j2, SLF4J
- **Go**: zap, logrus, zerolog
- **.NET**: Serilog, NLog, Microsoft.Extensions.Logging

### Structured Logging Encoders:
- **logstash-logback-encoder** (Java)
- **structlog** (Python)
- **pino** (Node.js)
- **zap** (Go)

### Context Propagation:
- **OpenTelemetry**: Distributed tracing context
- **Spring Cloud Sleuth** (Java)
- **cls-hooked/async_hooks** (Node.js)
- **contextvars** (Python 3.7+)

## Further Reading

### Books:
- "Observability Engineering" by Charity Majors, Liz Fong-Jones, George Miranda
- "Distributed Systems Observability" by Cindy Sridharan
- "Site Reliability Engineering" by Google SRE Team

### Articles:
- "Structured Logging" (Martin Fowler)
- "The 12-Factor App: Logs"
- "Distributed Tracing in Practice"

### Standards:
- OpenTelemetry Logging Specification
- RFC 5424: The Syslog Protocol
- ISO 8601: Date and time format

*Last updated: 2026-02-26*