#!/bin/bash
set -e

echo "Log Management: Rotation Configuration" >&2
echo "=====================================" >&2

# Default values
MAX_SIZE="${1:-100MB}"
KEEP_FILES="${2:-10}"
COMPRESS="${3:-gzip}"
ROTATE_INTERVAL="${4:-daily}"
OUTPUT_FILE="${5:-logrotate.conf}"
TIMESTAMP=$(date +%Y-%m-%dT%H:%M:%S)

echo "Configuring log rotation with:" >&2
echo "  Max size: $MAX_SIZE" >&2
echo "  Keep files: $KEEP_FILES" >&2
echo "  Compression: $COMPRESS" >&2
echo "  Rotation interval: $ROTATE_INTERVAL" >&2
echo "  Output file: $OUTPUT_FILE" >&2
echo "" >&2

# Generate logrotate configuration
cat << EOF > "$OUTPUT_FILE"
# Log rotation configuration generated on $TIMESTAMP
# Using parameters: max-size=$MAX_SIZE, keep-files=$KEEP_FILES, compress=$COMPRESS, interval=$ROTATE_INTERVAL

/var/log/app/*.log {
    $ROTATE_INTERVAL
    maxsize $MAX_SIZE
    rotate $KEEP_FILES
    compress
    delaycompress
    missingok
    notifempty
    create 0644 root root
    postrotate
        # Optional: Send signal to application to reopen log files
        # killall -HUP appname
        echo "Log rotated: \$1 at \$(date)" > /var/log/logrotate-status.log
    endscript
}

# Additional configurations for specific log files
/var/log/app/access.log {
    $ROTATE_INTERVAL
    maxsize $MAX_SIZE
    rotate $KEEP_FILES
    compress
    delaycompress
    sharedscripts
    postrotate
        # Example: Reload nginx if rotating nginx logs
        # systemctl reload nginx
        echo "Access log rotated" > /var/log/logrotate-status.log
    endscript
}

/var/log/app/error.log {
    $ROTATE_INTERVAL
    maxsize $MAX_SIZE
    rotate $KEEP_FILES
    compress
    delaycompress
    missingok
    create 0644 root root
    postrotate
        echo "Error log rotated" > /var/log/logrotate-status.log
    endscript
}
EOF

echo "✅ Generated log rotation configuration: $OUTPUT_FILE" >&2
echo "" >&2
echo "To use this configuration:" >&2
echo "1. Copy to /etc/logrotate.d/app" >&2
echo "2. Test with: logrotate -d $OUTPUT_FILE" >&2
echo "3. Run manually: logrotate -v $OUTPUT_FILE" >&2
echo "" >&2

# Generate systemd timer configuration if requested
if [ "$ROTATE_INTERVAL" = "hourly" ]; then
    echo "⚠️  Hourly rotation requires systemd timer or cron configuration" >&2
    cat << EOF > "logrotate-hourly.timer"
[Unit]
Description=Run logrotate hourly
Requires=logrotate.service

[Timer]
OnCalendar=hourly
Persistent=true

[Install]
WantedBy=timers.target
EOF
    echo "Generated systemd timer: logrotate-hourly.timer" >&2
fi

# Output JSON for machine parsing
echo '{"status": "configured", "service": "log-management-system", "timestamp": "'"$TIMESTAMP"'", "output_file": "'"$OUTPUT_FILE"'", "parameters": {"max_size": "'"$MAX_SIZE"'", "keep_files": "'"$KEEP_FILES"'", "compress": "'"$COMPRESS"'", "interval": "'"$ROTATE_INTERVAL"'"}}'