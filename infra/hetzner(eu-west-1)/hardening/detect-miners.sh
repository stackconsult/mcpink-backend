#!/bin/bash
# Crypto miner detection script
# Run via cron every 5 minutes: */5 * * * * /root/detect-miners.sh
set -e

LOG_FILE="/var/log/miner-detection.log"
CPU_THRESHOLD=90
AUTO_KILL=${AUTO_KILL:-false}

# Ensure log file exists
touch "$LOG_FILE"

# Get container stats (non-streaming for cron)
docker stats --no-stream --format "{{.Name}}|{{.CPUPerc}}" 2>/dev/null | while read line; do
    container=$(echo "$line" | cut -d'|' -f1)
    cpu_raw=$(echo "$line" | cut -d'|' -f2)

    # Extract numeric CPU value (handles "123.45%" format)
    cpu=$(echo "$cpu_raw" | grep -oP '[\d.]+' | head -1)

    # Skip if no CPU value found
    if [ -z "$cpu" ]; then
        continue
    fi

    # Skip system containers
    case "$container" in
        traefik*|coolify*|cadvisor*|alloy*|grafana*)
            continue
            ;;
    esac

    # Check if CPU exceeds threshold
    if (( $(echo "$cpu > $CPU_THRESHOLD" | bc -l) )); then
        timestamp=$(date '+%Y-%m-%d %H:%M:%S')
        echo "$timestamp: HIGH CPU detected in $container (${cpu}%)" >> "$LOG_FILE"

        # Log container info for investigation
        docker inspect --format '{{.Config.Image}}' "$container" 2>/dev/null >> "$LOG_FILE" || true

        if [ "$AUTO_KILL" = "true" ]; then
            echo "$timestamp: Auto-killing $container" >> "$LOG_FILE"
            docker kill "$container" 2>/dev/null || true
        fi
    fi
done

# Rotate log if too large (>10MB)
if [ -f "$LOG_FILE" ]; then
    size=$(stat -f%z "$LOG_FILE" 2>/dev/null || stat -c%s "$LOG_FILE" 2>/dev/null || echo 0)
    if [ "$size" -gt 10485760 ]; then
        mv "$LOG_FILE" "${LOG_FILE}.old"
        touch "$LOG_FILE"
    fi
fi
