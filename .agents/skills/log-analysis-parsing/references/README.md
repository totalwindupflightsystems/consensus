# Log Analysis & Parsing Reference

## Overview
Log analysis and parsing transforms raw log data into structured, actionable insights. This reference covers techniques, tools, and patterns for effective log parsing and analysis.

## Parsing Techniques

### 1. Regular Expression (Regex) Parsing
Most common method for unstructured logs.

#### Common Log Patterns:

**Apache/Nginx Access Logs:**
```regex
^(?P<ip>\S+) \S+ (?P<user>\S+) \[(?P<timestamp>[^\]]+)\] "(?P<method>\S+) (?P<path>\S+) (?P<protocol>\S+)" (?P<status>\d+) (?P<size>\d+)
```

**Syslog Format:**
```regex
^<(?P<priority>\d+)>(?P<timestamp>\w{3}\s+\d+\s+\d+:\d+:\d+)\s+(?P<hostname>\S+)\s+(?P<process>\S+)(?:\[(?P<pid>\d+)\])?:\s+(?P<message>.*)$
```

**Key-Value Logs:**
```regex
(\w+)=("[^"]*"|\S+)
```

#### Performance Tips:
- Use non-capturing groups `(?:...)` when capture not needed
- Avoid greedy quantifiers `.*` when possible
- Compile regex once and reuse
- Consider regex engine differences (PCRE vs RE2)

### 2. Grok Pattern Parsing
Grok uses named regex patterns for reusable components.

#### Common Grok Patterns:
```grok
# Timestamps
%{TIMESTAMP_ISO8601:timestamp}
%{HTTPDATE:timestamp}

# IP Addresses
%{IP:client_ip}
%{IPV6:client_ip}

# Numbers
%{NUMBER:response_time}
%{BASE10NUM:bytes}

# Data
%{DATA:message}
%{GREEDYDATA:message}

# Paths
%{URIPATH:request_path}
%{URIPATHPARAM:request_path}
```

#### Example Grok Pattern for Apache Logs:
```grok
%{IP:client_ip} %{USER:ident} %{USER:auth} \[%{HTTPDATE:timestamp}\] "%{WORD:method} %{URIPATHPARAM:request} HTTP/%{NUMBER:http_version}" %{NUMBER:response_status} %{NUMBER:bytes_sent}
```

### 3. JSON Parsing
For structured JSON logs.

#### Tools:
- **jq**: Command-line JSON processor
- **Python json module**: Built-in JSON parsing
- **Logstash json filter**: JSON parsing in pipelines

#### jq Examples:
```bash
# Extract specific fields
cat app.log | jq -r '.timestamp, .level, .message'

# Filter by level
cat app.log | jq -r 'select(.level == "ERROR") | .message'

# Group by field
cat app.log | jq -r 'group_by(.service)[] | {service: .[0].service, count: length}'

# Calculate statistics
cat app.log | jq -r 'select(.response_time) | .response_time' | stats.py
```

### 4. Multi-line Log Parsing
For stack traces and multi-line events.

#### Logstash Configuration:
```ruby
filter {
  multiline {
    pattern => "^%{TIMESTAMP_ISO8601}"
    negate => true
    what => "previous"
  }
}
```

#### Fluentd Configuration:
```xml
<source>
  @type tail
  format multiline
  format_firstline /^\d{4}-\d{2}-\d{2}/
  format1 /^(?<time>\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}) \[(?<level>.*)\] (?<message>.*)/
</source>
```

## Analysis Techniques

### 1. Pattern Frequency Analysis
Identify most common log messages, errors, or patterns.

#### Command-line Examples:
```bash
# Count occurrences of each error message
grep -o "ERROR.*" app.log | sort | uniq -c | sort -rn

# Top 10 error patterns
awk '/ERROR/ {print $5, $6, $7}' app.log | sort | uniq -c | sort -rn | head -10

# Using awk for field-based analysis
awk '{count[$7]++} END {for (word in count) print word, count[word]}' access.log
```

### 2. Time-Series Analysis
Analyze log patterns over time.

#### Python Example:
```python
import pandas as pd
from datetime import datetime

# Parse logs into time series
logs = pd.read_csv('app.log', parse_dates=['timestamp'])
logs.set_index('timestamp', inplace=True)

# Resample by hour
hourly_errors = logs[logs['level'] == 'ERROR'].resample('H').size()
hourly_volume = logs.resample('H').size()

# Calculate error rate
error_rate = hourly_errors / hourly_volume
```

### 3. Correlation Analysis
Link related events across log sources.

#### Correlation Strategies:
1. **Trace IDs**: Use distributed tracing IDs
2. **Timestamps**: Correlate by time windows
3. **User IDs**: Follow user journeys
4. **Request IDs**: Track request flow

#### Example Correlation Script:
```python
def correlate_logs(logs1, logs2, time_window=5):
    """Correlate logs from two sources within time window."""
    correlated = []
    
    for log1 in logs1:
        # Find logs in second source within time window
        matching = [
            log2 for log2 in logs2 
            if abs((log1['timestamp'] - log2['timestamp']).total_seconds()) <= time_window
            and have_common_fields(log1, log2)
        ]
        
        if matching:
            correlated.append({
                'primary': log1,
                'related': matching
            })
    
    return correlated
```

### 4. Anomaly Detection
Identify unusual patterns.

#### Statistical Methods:
- **Z-score**: Detect deviations from mean
- **IQR**: Identify outliers using interquartile range
- **Seasonal decomposition**: Remove seasonality to find anomalies
- **Machine learning**: Unsupervised anomaly detection

#### Simple Anomaly Detection:
```python
def detect_anomalies(log_volume, window=24, threshold=3):
    """Detect anomalies in log volume using moving average."""
    moving_avg = log_volume.rolling(window=window).mean()
    moving_std = log_volume.rolling(window=window).std()
    
    # Calculate z-score
    zscore = (log_volume - moving_avg) / moving_std
    
    # Flag anomalies where z-score > threshold
    anomalies = zscore.abs() > threshold
    
    return anomalies
```

## Tools & Libraries

### Command-line Tools
| Tool | Purpose | Example |
|------|---------|---------|
| `grep` | Pattern matching | `grep -E "ERROR|WARN" app.log` |
| `awk` | Field processing | `awk '{print $1, $7}' access.log` |
| `sed` | Text transformation | `sed 's/ERROR/CRITICAL/g' app.log` |
| `jq` | JSON processing | `jq '.timestamp, .message' app.log` |
| `rg` (ripgrep) | Fast grep | `rg -N "error.*timeout" app.log` |
| `mlr` (miller) | Log processing | `mlr --csv cut -f timestamp,level app.log` |

### Programming Libraries
- **Python**: `re`, `json`, `pandas`, `numpy`, `scikit-learn`
- **Go**: `regexp`, `encoding/json`, `go-flags`
- **Java**: `java.util.regex`, `Jackson`, `LogParser`
- **Node.js**: `regex`, `JSON.parse`, `bunyan`, `winston`

### Specialized Tools
- **Logstash**: Pipeline-based log processing
- **Fluentd**: Unified logging layer
- **Vector**: High-performance observability pipeline
- **Splunk**: Enterprise log analysis
- **Elasticsearch + Kibana**: Search and visualization

## Performance Optimization

### Parsing Performance
1. **Compile regex once**: Reuse compiled patterns
2. **Use streaming parsing**: Don't load entire files into memory
3. **Parallel processing**: Use multiple cores for large files
4. **Indexing**: Create indexes for frequent queries

### Memory Management
```python
# Stream large log files
def parse_large_file(file_path):
    with open(file_path, 'r') as f:
        for line in f:
            parsed = parse_log_line(line)
            yield parsed
            # Process line by line to avoid memory issues
```

## Common Parsing Challenges & Solutions

### Challenge: Inconsistent Timestamp Formats
**Solution**: Normalize timestamps
```python
def normalize_timestamp(timestamp_str):
    formats = [
        '%Y-%m-%dT%H:%M:%S.%fZ',
        '%Y-%m-%d %H:%M:%S',
        '%d/%b/%Y:%H:%M:%S %z',
        '%b %d %H:%M:%S'
    ]
    
    for fmt in formats:
        try:
            return datetime.strptime(timestamp_str, fmt)
        except ValueError:
            continue
    
    raise ValueError(f"Unknown timestamp format: {timestamp_str}")
```

### Challenge: Multi-line Stack Traces
**Solution**: Use stateful parsing
```python
class StackTraceParser:
    def __init__(self):
        self.in_stack_trace = False
        self.current_trace = []
    
    def parse_line(self, line):
        if line.startswith('Exception') or line.startswith('at '):
            self.in_stack_trace = True
            self.current_trace.append(line)
            return None
        elif self.in_stack_trace:
            if line.strip() == '':
                # End of stack trace
                trace = '\n'.join(self.current_trace)
                self.current_trace = []
                self.in_stack_trace = False
                return {'type': 'stack_trace', 'content': trace}
            else:
                self.current_trace.append(line)
                return None
        else:
            return parse_normal_line(line)
```

### Challenge: Missing or Malformed Fields
**Solution**: Handle gracefully
```python
def safe_parse(log_line, parser):
    try:
        return parser.parse(log_line)
    except ParseError as e:
        # Log parsing failure but continue
        logging.warning(f"Failed to parse line: {log_line[:100]}...")
        return {
            'raw': log_line,
            'parse_error': str(e),
            'timestamp': datetime.now()
        }
```

## Real-world Examples

### Example 1: Web Server Log Analysis
```bash
# Analyze Apache access logs
cat access.log | awk '
{
    # Count by status code
    status[$9]++
    
    # Track response sizes
    total_size += $10
    count++
}
END {
    print "Status Code Distribution:"
    for (s in status) {
        printf "  %s: %d (%.1f%%)\n", s, status[s], (status[s]/count)*100
    }
    
    print "\nAverage Response Size:", total_size/count
}'
```

### Example 2: Application Error Trend Analysis
```python
import pandas as pd
import matplotlib.pyplot as plt

# Load and parse logs
logs = pd.read_csv('app.log', parse_dates=['timestamp'])
logs['hour'] = logs['timestamp'].dt.floor('H')

# Calculate hourly error rate
hourly_stats = logs.groupby('hour').agg({
    'level': lambda x: (x == 'ERROR').sum(),
    'timestamp': 'count'
}).rename(columns={'level': 'errors', 'timestamp': 'total'})

hourly_stats['error_rate'] = hourly_stats['errors'] / hourly_stats['total']

# Plot
hourly_stats['error_rate'].plot()
plt.title('Hourly Error Rate')
plt.ylabel('Error Rate')
plt.xlabel('Hour')
plt.show()
```

### Example 3: Security Log Analysis
```bash
# Detect failed login attempts
grep "Failed password" auth.log | awk '
{
    # Extract IP address
    match($0, /[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+/)
    ip = substr($0, RSTART, RLENGTH)
    
    # Count attempts per IP
    attempts[ip]++
}
END {
    print "Failed Login Attempts by IP:"
    for (ip in attempts) {
        if (attempts[ip] > 5) {
            print "  " ip ": " attempts[ip] " attempts"
        }
    }
}'
```

## Implementation Checklist

- [ ] Identify log formats and sources
- [ ] Create parsing patterns for each format
- [ ] Implement error handling for parsing failures
- [ ] Set up analysis pipelines
- [ ] Create dashboards for key metrics
- [ ] Implement alerting for anomalies
- [ ] Optimize parsing performance
- [ ] Document parsing rules and patterns
- [ ] Test with sample data from all sources
- [ ] Monitor parsing success rates

## Further Reading
- [Regular Expressions Mastery](https://www.example.com/regex-mastery)
- [Log Parsing Patterns](https://www.example.com/log-parsing-patterns)
- [Time Series Analysis of Logs](https://www.example.com/log-time-series)
- [Anomaly Detection in Logs](https://www.example.com/log-anomaly-detection)
- [Grok Pattern Reference](https://www.example.com/grok-patterns)