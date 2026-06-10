# Logging Performance Optimization Reference

## Overview
Logging performance optimization focuses on reducing overhead, implementing efficient logging patterns, and managing resource consumption for high-throughput systems. This reference provides techniques, tools, and best practices for optimizing logging performance.

## Performance Impact Areas

### 1. CPU Overhead
Logging operations consume CPU cycles for:
- String formatting and concatenation
- Serialization (JSON, XML, etc.)
- Level filtering and condition evaluation
- Context gathering (stack traces, method names)

### 2. Memory Usage
Logging consumes memory through:
- Log message buffers
- String allocations
- Context objects
- Queue/buffer storage

### 3. I/O Operations
Disk and network I/O includes:
- Writing to log files
- Network transmission for centralized logging
- Flush operations
- Rotation and compression

### 4. Latency Impact
Synchronous logging can block application threads, increasing latency.

## Measurement & Profiling

### Benchmarking Tools

#### Custom Benchmark Script:
```python
import time
import logging
import statistics

def benchmark_logging(logger, iterations=10000):
    """Benchmark logging performance."""
    durations = []
    
    for i in range(iterations):
        start = time.perf_counter_ns()
        logger.info("Test log message %d", i)
        end = time.perf_counter_ns()
        durations.append(end - start)
    
    avg_ns = statistics.mean(durations)
    ops_per_second = 1_000_000_000 / avg_ns if avg_ns > 0 else 0
    
    return {
        'iterations': iterations,
        'avg_ns': avg_ns,
        'ops_per_second': ops_per_second,
        'p50_ns': statistics.median(durations),
        'p95_ns': statistics.quantiles(durations, n=20)[18],
        'p99_ns': statistics.quantiles(durations, n=100)[98]
    }
```

#### Using Async Profiler:
```bash
# Profile Java logging with async-profiler
./profiler.sh -d 60 -f logging-profile.html --chunktime 1 <pid>

# Profile CPU usage of logging operations
./profiler.sh -e cpu -d 30 -f cpu-profile.html <pid>
```

### Key Metrics to Monitor:
```prometheus
# Logging performance metrics
logging_cpu_seconds_total
logging_memory_bytes
logging_operations_total
logging_latency_seconds_bucket
logging_queue_size
logging_dropped_messages_total

# Resource usage
logging_buffer_usage_percent
logging_disk_io_bytes_total
logging_network_io_bytes_total
```

## Optimization Techniques

### 1. Async Logging

#### Implementation Patterns:

**Log4j2 AsyncLogger:**
```xml
<Configuration>
  <Appenders>
    <RandomAccessFile name="File" fileName="logs/app.log"
                     immediateFlush="false" bufferSize="8192">
      <PatternLayout pattern="%d{ISO8601} %-5p %c{1}:%L - %m%n"/>
    </RandomAccessFile>
  </Appenders>
  
  <Loggers>
    <AsyncLogger name="com.example" level="INFO" includeLocation="false">
      <AppenderRef ref="File"/>
    </AsyncLogger>
    
    <Root level="WARN">
      <AppenderRef ref="File"/>
    </Root>
  </Loggers>
</Configuration>
```

**Python QueueHandler:**
```python
import logging
import queue
from logging.handlers import QueueHandler, QueueListener

# Create queue
log_queue = queue.Queue(-1)  # Unlimited size

# Set up queue handler
queue_handler = QueueHandler(log_queue)
queue_handler.setLevel(logging.INFO)

# Set up listener with actual handler
file_handler = logging.FileHandler('app.log')
listener = QueueListener(log_queue, file_handler)
listener.start()

# Configure logger
logger = logging.getLogger(__name__)
logger.addHandler(queue_handler)
logger.setLevel(logging.INFO)
```

**Go Async Logger:**
```go
package main

import (
	"log"
	"os"
	"sync"
)

type AsyncLogger struct {
	ch     chan string
	wg     sync.WaitGroup
	writer *os.File
}

func NewAsyncLogger(filename string, bufferSize int) *AsyncLogger {
	f, _ := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	
	al := &AsyncLogger{
		ch:     make(chan string, bufferSize),
		writer: f,
	}
	
	al.wg.Add(1)
	go al.processLogs()
	
	return al
}

func (al *AsyncLogger) Log(msg string) {
	select {
	case al.ch <- msg:
		// Message queued
	default:
		// Buffer full, drop message
		log.Printf("Log buffer full, dropped: %s", msg)
	}
}

func (al *AsyncLogger) processLogs() {
	defer al.wg.Done()
	
	for msg := range al.ch {
		al.writer.WriteString(msg + "\n")
	}
}
```

### 2. Buffering Strategies

#### Buffer Types:

**Ring Buffer (Circular Buffer):**
```python
import threading
from collections import deque

class RingBuffer:
    def __init__(self, capacity):
        self.capacity = capacity
        self.buffer = deque(maxlen=capacity)
        self.lock = threading.Lock()
    
    def append(self, item):
        with self.lock:
            if len(self.buffer) == self.capacity:
                # Buffer full, drop oldest
                self.buffer.popleft()
            self.buffer.append(item)
    
    def flush(self):
        with self.lock:
            items = list(self.buffer)
            self.buffer.clear()
            return items
```

**Memory-Mapped File Buffer:**
```python
import mmap
import os

class MappedFileBuffer:
    def __init__(self, filename, size_mb=100):
        self.filename = filename
        self.size = size_mb * 1024 * 1024
        
        # Create or open file
        with open(filename, 'a+b') as f:
            f.truncate(self.size)
        
        self.file = open(filename, 'r+b')
        self.mmap = mmap.mmap(self.file.fileno(), self.size)
        self.position = 0
    
    def write(self, data):
        data_bytes = data.encode('utf-8')
        if self.position + len(data_bytes) > self.size:
            self.position = 0  # Wrap around
        
        self.mmap.seek(self.position)
        self.mmap.write(data_bytes)
        self.position += len(data_bytes)
    
    def close(self):
        self.mmap.close()
        self.file.close()
```

### 3. Sampling Strategies

#### Probabilistic Sampling:
```python
import random
import hashlib

class ProbabilisticSampler:
    def __init__(self, rate):
        """
        Args:
            rate: Sampling rate (0.0 to 1.0)
        """
        self.rate = rate
    
    def should_sample(self, log_entry):
        # Deterministic sampling based on trace_id or other stable field
        if 'trace_id' in log_entry:
            # Use hash of trace_id for deterministic sampling
            hash_val = int(hashlib.md5(log_entry['trace_id'].encode()).hexdigest(), 16)
            return (hash_val % 1000) < (self.rate * 1000)
        else:
            # Random sampling
            return random.random() < self.rate
```

#### Rate-Based Sampling:
```python
from collections import defaultdict
import time

class RateLimitingSampler:
    def __init__(self, max_per_second):
        self.max_per_second = max_per_second
        self.counts = defaultdict(int)
        self.last_reset = time.time()
    
    def should_sample(self, key):
        current_time = time.time()
        
        # Reset counts if new second
        if current_time - self.last_reset >= 1.0:
            self.counts.clear()
            self.last_reset = current_time
        
        # Check if we've exceeded rate limit
        if self.counts[key] >= self.max_per_second:
            return False
        
        self.counts[key] += 1
        return True
```

#### Level-Based Sampling:
```yaml
sampling:
  # Different sampling rates per log level
  DEBUG:
    rate: 0.01  # 1% of DEBUG logs
    min_per_second: 10  # Keep at least 10 per second
    
  INFO:
    rate: 0.1   # 10% of INFO logs
    min_per_second: 100
    
  WARN:
    rate: 0.5   # 50% of WARN logs
    min_per_second: 1000
    
  ERROR:
    rate: 1.0   # 100% of ERROR logs
    adaptive: false  # Never sample errors
```

### 4. Efficient Serialization

#### Binary Formats:
```python
import json
import msgpack
import pickle
import time

def benchmark_serialization(data, iterations=10000):
    """Benchmark different serialization formats."""
    formats = {
        'json': lambda d: json.dumps(d).encode('utf-8'),
        'msgpack': lambda d: msgpack.packb(d),
        'pickle': lambda d: pickle.dumps(d),
        'protobuf': lambda d: d.SerializeToString()
    }
    
    results = {}
    for name, serialize in formats.items():
        start = time.perf_counter()
        size = 0
        for _ in range(iterations):
            serialized = serialize(data)
            size = len(serialized)
        end = time.perf_counter()
        
        results[name] = {
            'time_ms': (end - start) * 1000,
            'size_bytes': size,
            'ops_per_second': iterations / (end - start)
        }
    
    return results
```

#### Field Selection:
```python
class OptimizedLogger:
    def __init__(self):
        # Pre-allocate common fields
        self.common_fields = {
            'service': 'payment-service',
            'environment': 'production',
            'version': '1.2.3'
        }
    
    def log(self, level, message, **extra):
        # Start with common fields
        log_entry = self.common_fields.copy()
        
        # Add timestamp
        log_entry['timestamp'] = time.time()
        
        # Add level and message
        log_entry['level'] = level
        log_entry['message'] = message
        
        # Only add extra fields if they're meaningful
        for key, value in extra.items():
            if value is not None and value != '':
                log_entry[key] = value
        
        return log_entry
```

### 5. Lazy Evaluation

#### Lazy Message Formatting:
```java
// Java - Use lambda for lazy evaluation
logger.debug("Complex calculation result: {}", () -> {
    // This only executes if DEBUG is enabled
    return expensiveCalculation();
});

// Traditional approach (always executes expensive calculation)
logger.debug("Complex calculation result: " + expensiveCalculation());
```

#### Python Lazy Evaluation:
```python
import logging

class LazyMessage:
    def __init__(self, func, *args, **kwargs):
        self.func = func
        self.args = args
        self.kwargs = kwargs
    
    def __str__(self):
        return str(self.func(*self.args, **self.kwargs))

def expensive_operation():
    # Simulate expensive operation
    import time
    time.sleep(0.1)
    return "result"

# Lazy evaluation - only computes if needed
logger.debug("Result: %s", LazyMessage(expensive_operation))
```

## Configuration Optimization

### Log Level Optimization

#### Environment-Specific Levels:
```yaml
log_levels:
  development:
    root: DEBUG
    com.example: DEBUG
    org.springframework: INFO
    org.hibernate: WARN
    
  staging:
    root: INFO
    com.example: INFO
    org.springframework: WARN
    org.hibernate: ERROR
    
  production:
    root: WARN
    com.example: INFO  # Business logic at INFO
    org.springframework: ERROR
    org.hibernate: ERROR
    
  performance_testing:
    root: ERROR  # Minimal logging for performance tests
    com.example: ERROR
```

#### Dynamic Log Level Adjustment:
```python
class DynamicLogLevelManager:
    def __init__(self):
        self.levels = {}
        self.metrics = {}
    
    def adjust_levels_based_on_load(self, current_load):
        """Adjust log levels based on system load."""
        if current_load > 0.8:  # High load
            # Reduce logging
            self.set_level('DEBUG', 'INFO')
            self.set_level('INFO', 'WARN')
        elif current_load < 0.3:  # Low load
            # Increase logging for debugging
            self.set_level('INFO', 'DEBUG')
            self.set_level('WARN', 'INFO')
    
    def set_level(self, from_level, to_level):
        for logger_name in self.get_loggers_at_level(from_level):
            logging.getLogger(logger_name).setLevel(to_level)
```

### Buffer Configuration

#### Optimal Buffer Sizing:
```python
def calculate_optimal_buffer_size(throughput_logs_per_second, max_latency_ms):
    """
    Calculate optimal buffer size based on throughput and latency requirements.
    
    Args:
        throughput_logs_per_second: Expected log throughput
        max_latency_ms: Maximum acceptable latency in milliseconds
    
    Returns:
        Optimal buffer size in number of log entries
    """
    # Convert latency to seconds
    max_latency_seconds = max_latency_ms / 1000
    
    # Buffer should hold at least throughput * latency
    min_buffer = throughput_logs_per_second * max_latency_seconds
    
    # Add safety margin
    safety_factor = 2.0
    
    return int(min_buffer * safety_factor)
```

#### Buffer Monitoring:
```python
class BufferMonitor:
    def __init__(self, buffer, alert_threshold=0.8):
        self.buffer = buffer
        self.alert_threshold = alert_threshold
        self.metrics = {
            'usage_percent': [],
            'drop_count': 0,
            'throughput': []
        }
    
    def monitor(self):
        while True:
            # Calculate buffer usage
            usage = self.buffer.size() / self.buffer.capacity()
            self.metrics['usage_percent'].append(usage)
            
            # Alert if buffer is getting full
            if usage > self.alert_threshold:
                self.alert_high_usage(usage)
            
            # Record throughput
            self.metrics['throughput'].append(self.buffer.throughput())
            
            time.sleep(1)  # Monitor every second
```

## Tool-Specific Optimizations

### Log4j2 Optimization

#### Configuration:
```xml
<Configuration status="WARN">
  <!-- Use asynchronous loggers -->
  <AsyncLogger name="com.example" level="INFO" includeLocation="false">
    <AppenderRef ref="File"/>
  </AsyncLogger>
  
  <Appenders>
    <!-- Use RandomAccessFile for better performance -->
    <RandomAccessFile name="File" fileName="logs/app.log"
                     immediateFlush="false" bufferSize="8192">
      <PatternLayout pattern="%d{ISO8601} %-5p %c{1}:%L - %m%n"/>
    </RandomAccessFile>
    
    <!-- Async appender with ring buffer -->
    <Routing name="Async">
      <Routes pattern="$${sd:type}">
        <Route>
          <RollingFile name="Rolling-${sd:type}" 
                      fileName="logs/${sd:type}.log"
                      filePattern="logs/${sd:type}-%d{yyyy-MM-dd}.log.gz">
            <PatternLayout pattern="%m%n"/>
            <Policies>
              <TimeBasedTriggeringPolicy/>
            </Policies>
          </RollingFile>
        </Route>
      </Routes>
    </Routing>
  </Appenders>
</Configuration>
```

### Python Logging Optimization

#### Efficient Configuration:
```python
import logging
import logging.config

LOGGING_CONFIG = {
    'version': 1,
    'disable_existing_loggers': False,
    
    'formatters': {
        'simple': {
            'format': '%(asctime)s %(levelname)s %(name)s: %(message)s'
        },
        'json': {
            '()': 'pythonjsonlogger.jsonlogger.JsonFormatter',
            'fmt': '%(asctime)s %(levelname)s %(name)s %(message)s'
        }
    },
    
    'handlers': {
        'console': {
            'class': 'logging.StreamHandler',
            'level': 'INFO',
            'formatter': 'simple',
            'stream': 'ext://sys.stdout'
        },
        'file': {
            'class': 'logging.handlers.RotatingFileHandler',
            'level': 'DEBUG',
            'formatter': 'json',
            'filename': 'app.log',
            'maxBytes': 10485760,  # 10MB
            'backupCount': 5,
            'encoding': 'utf8'
        },
        'async_file': {
            '()': 'concurrent_log_handler.ConcurrentRotatingFileHandler',
            'level': 'INFO',
            'formatter': 'json',
            'filename': 'app-async.log',
            'maxBytes': 10485760,
            'backupCount': 5,
            'use_gzip': True
        }
    },
    
    'loggers': {
        '': {  # Root logger
            'level': 'INFO',
            'handlers': ['async_file']
        },
        'app.core': {
            'level': 'DEBUG',
            'handlers': ['async_file'],
            'propagate': False
        }
    }
}

logging.config.dictConfig(LOGGING_CONFIG)
```

### Go Zap Optimization

#### High-Performance Configuration:
```go
package main

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func NewProductionLogger() *zap.Logger {
	// Encoder config
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "timestamp"
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.StacktraceKey = "stacktrace"
	
	// Core with sampling
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zapcore.Lock(os.Stdout),
		zap.InfoLevel,
	)
	
	// Add sampling
	sampledCore := zapcore.NewSamplerWithOptions(
		core,
		time.Second,       // Check interval
		100,              // Initial logs per interval
		100,              // Burst allowance
	)
	
	// Build logger
	logger := zap.New(sampledCore, zap.AddCaller())
	
	return logger
}

// Usage
logger := NewProductionLogger()
defer logger.Sync()

// Structured logging with fields
logger.Info("Request processed",
	zap.String("method", "GET"),
	zap.String("path", "/api/users"),
	zap.Int("status", 200),
	zap.Duration("duration", 150*time.Millisecond),
)
```

## Monitoring & Alerting

### Key Performance Indicators:
```yaml
monitoring:
  cpu:
    threshold: 5%  # Max CPU usage by logging
    window: 5m
    
  memory:
    threshold: 100MB  # Max memory for logging buffers
    window: 1m
    
  latency:
    threshold: 10ms  # Max logging latency
    window: 1m
    
  throughput:
    threshold: 10000  # Max logs per second
    window: 1m
    
  buffer_usage:
    threshold: 80%  # Alert when buffer > 80% full
    window: 1m
    
  drop_rate:
    threshold: 1%  # Alert when > 1% logs dropped
    window: 5m
```

### Alert Examples:
```python
class LoggingPerformanceAlerts:
    def check_alerts(self, metrics):
        alerts = []
        
        # CPU usage alert
        if metrics.cpu_percent > 5:
            alerts.append({
                'severity': 'warning',
                'message': f'High CPU usage by logging: {metrics.cpu_percent}%',
                'suggestion': 'Consider reducing log level or implementing sampling'
            })
        
        # Buffer usage alert
        if metrics.buffer_usage_percent > 0.8:
            alerts.append({
                'severity': 'critical',
                'message': f'Log buffer almost full: {metrics.buffer_usage_percent*100}%',
                'suggestion': 'Increase buffer size or improve downstream processing'
            })
        
        # Drop rate alert
        if metrics.drop_rate > 0.01:
            alerts.append({
                'severity': 'warning',
                'message': f'High log drop rate: {metrics.drop_rate*100}%',
                'suggestion': 'Review buffer configuration and downstream capacity'
            })
        
        return alerts
```

## Implementation Checklist

- [ ] Profile current logging performance
- [ ] Implement async logging
- [ ] Configure appropriate buffer sizes
- [ ] Set up log sampling
- [ ] Optimize serialization format
- [ ] Implement lazy evaluation
- [ ] Set log levels per environment
- [ ] Configure monitoring and alerting
- [ ] Test under load
- [ ] Document optimization decisions
- [ ] Establish performance baselines
- [ ] Regular performance review
- [ ] Continuously monitor and optimize

## Common Issues & Solutions

### Issue: High CPU usage during peak load
**Solution**: Implement adaptive sampling
```python
class AdaptiveSampler:
    def __init__(self, base_rate=0.1, max_cpu_threshold=70):
        self.base_rate = base_rate
        self.max_cpu_threshold = max_cpu_threshold
    
    def get_sampling_rate(self, current_cpu_percent):
        if current_cpu_percent > self.max_cpu_threshold:
            # Reduce sampling when CPU is high
            reduction = (current_cpu_percent - self.max_cpu_threshold) / 100
            return max(0.01, self.base_rate * (1 - reduction))
        else:
            return self.base_rate
```

### Issue: Memory leaks in logging buffers
**Solution**: Implement buffer lifecycle management
```python
class ManagedBuffer:
    def __init__(self, max_size, max_age_seconds=300):
        self.max_size = max_size
        self.max_age_seconds = max_age_seconds
        self.buffer = []
        self.timestamps = []
        self.cleanup_counter = 0
    
    def append(self, item):
        self.buffer.append(item)
        self.timestamps.append(time.time())
        
        # Periodic cleanup
        self.cleanup_counter += 1
        if self.cleanup_counter >= 100:
            self.cleanup()
            self.cleanup_counter = 0
        
        # Size management
        if len(self.buffer) > self.max_size:
            self.buffer.pop(0)
            self.timestamps.pop(0)
    
    def cleanup(self):
        current_time = time.time()
        # Remove items older than max_age_seconds
        while self.timestamps and current_time - self.timestamps[0] > self.max_age_seconds:
            self.buffer.pop(0)
            self.timestamps.pop(0)
```

### Issue: Logging blocking application threads
**Solution**: Use non-blocking queues and timeouts
```python
import queue
import threading

class NonBlockingLogger:
    def __init__(self, timeout_ms=10):
        self.queue = queue.Queue(maxsize=10000)
        self.timeout_ms = timeout_ms / 1000
        self.worker = threading.Thread(target=self.process_queue)
        self.worker.daemon = True
        self.worker.start()
    
    def log(self, level, message):
        try:
            # Non-blocking put with timeout
            self.queue.put(
                {'level': level, 'message': message, 'timestamp': time.time()},
                timeout=self.timeout_ms
            )
            return True
        except queue.Full:
            # Log dropped
            return False
    
    def process_queue(self):
        while True:
            try:
                item = self.queue.get(timeout=1)
                self.write_to_destination(item)
            except queue.Empty:
                continue
```

## Further Reading
- [Logging Performance Best Practices](https://www.example.com/logging-performance)
- [Async Logging Patterns](https://www.example.com/async-logging)
- [Buffer Management for Logging](https://www.example.com/logging-buffers)
- [Sampling Strategies](https://www.example.com/log-sampling)
- [Performance Monitoring for Logging](https://www.example.com/logging-monitoring)