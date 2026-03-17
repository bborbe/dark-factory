#!/usr/bin/env bash
set -euo pipefail

# Watch dark-factory execution with sound alerts.
# Polls every 60s, plays macOS sounds on state changes.
#
# Sounds:
#   3x Sosumi = prompt failed — check log, fix, retry
#   Basso     = stuck >15min — may need intervention
#   Glass     = all prompts complete

# Auto-detect project directory
if [ -n "${1:-}" ]; then
  PROJECT_DIR="$1"
elif [ -f ".dark-factory.yaml" ]; then
  PROJECT_DIR="."
else
  # Search for running daemon via lock files
  PROJECT_DIR=""
  for lock in $(find ~/Documents/workspaces -name ".dark-factory.lock" -type f 2>/dev/null); do
    dir=$(dirname "$lock")
    pid=$(cat "$lock" 2>/dev/null)
    if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
      PROJECT_DIR="$dir"
      break
    fi
  done
  if [ -z "$PROJECT_DIR" ]; then
    echo "ERROR: No dark-factory project found. Pass project dir as argument or cd to project root."
    exit 1
  fi
fi

cd "$PROJECT_DIR"
echo "Watching: $(pwd)"

# Verify .dark-factory.yaml exists
if [ ! -f ".dark-factory.yaml" ]; then
  echo "ERROR: .dark-factory.yaml not found in $PROJECT_DIR"
  exit 1
fi

# Check daemon is running
if ! dark-factory status 2>&1 | grep -q "running\|idle\|executing"; then
  echo "ERROR: daemon not running. Start it first with /dark-factory:daemon"
  exit 1
fi

echo "Watching dark-factory... (Ctrl+C to stop)"

while true; do
  STATUS=$(dark-factory status 2>&1)
  CURRENT=$(echo "$STATUS" | grep "Current:" | perl -pe 's/.*Current:\s*//')
  QUEUE=$(echo "$STATUS" | grep "Queue:" | perl -pe 's/.*Queue:\s*//')
  COMPLETED=$(echo "$STATUS" | grep "Completed:" | perl -pe 's/.*Completed:\s*//')
  echo "$(date +%H:%M:%S) | Queue: $QUEUE | Completed: $COMPLETED | Current: $CURRENT"

  # Alert: prompt failed
  if echo "$STATUS" | grep -q "failed"; then
    echo "ALERT: PROMPT FAILED!"
    afplay /System/Library/Sounds/Sosumi.aiff
    afplay /System/Library/Sounds/Sosumi.aiff
    afplay /System/Library/Sounds/Sosumi.aiff
    break
  fi

  # Done: queue empty, daemon idle
  if echo "$STATUS" | grep -q "idle" && echo "$STATUS" | grep -qE "Queue:[[:space:]]+0"; then
    echo "ALL DONE!"
    afplay /System/Library/Sounds/Glass.aiff
    break
  fi

  # Alert: stuck >15 minutes
  MINS=$(echo "$STATUS" | grep "executing since" | grep -o '[0-9]*m' | tr -d 'm')
  if [ -n "$MINS" ] && [ "$MINS" -ge 15 ]; then
    echo "ALERT: STUCK >15min on $CURRENT"
    afplay /System/Library/Sounds/Basso.aiff
  fi

  sleep 60
done
