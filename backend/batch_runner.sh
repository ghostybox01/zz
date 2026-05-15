#!/bin/bash
# Raven Batch Runner
# Processes batch files one at a time

cd /root/python_job

echo "$(date '+%Y-%m-%d %H:%M:%S') Starting batch runner..."
echo "$(date '+%Y-%m-%d %H:%M:%S') Starting batch runner..." >> output.log

# Check if raven-scanner exists
if [ ! -f "raven-scanner" ]; then
    echo "ERROR: raven-scanner binary not found!"
    echo "ERROR: raven-scanner binary not found!" >> output.log
    exit 1
fi

chmod +x raven-scanner

# Process each batch
for batch in batch_*.txt; do
    # Skip if already done or failed
    [ -f "${batch}.done" ] && continue
    [ -f "${batch}.failed" ] && continue
    
    # Skip if no batches
    [ "$batch" = "batch_*.txt" ] && break
    
    echo "$(date '+%Y-%m-%d %H:%M:%S') Processing: $batch"
    echo "$(date '+%Y-%m-%d %H:%M:%S') Processing: $batch" >> output.log
    
    # Copy batch to targets.txt (what the scanner expects)
    cp "$batch" targets.txt
    
    # Run the scanner
    if ./raven-scanner targets.txt >> output.log 2>&1; then
        mv "$batch" "${batch}.done"
        echo "$(date '+%Y-%m-%d %H:%M:%S') DONE: $batch"
        echo "$(date '+%Y-%m-%d %H:%M:%S') DONE: $batch" >> output.log
    else
        mv "$batch" "${batch}.failed"
        echo "$(date '+%Y-%m-%d %H:%M:%S') FAILED: $batch"
        echo "$(date '+%Y-%m-%d %H:%M:%S') FAILED: $batch" >> output.log
    fi
done

echo "$(date '+%Y-%m-%d %H:%M:%S') ALL COMPLETE"
echo "$(date '+%Y-%m-%d %H:%M:%S') ALL COMPLETE" >> output.log
