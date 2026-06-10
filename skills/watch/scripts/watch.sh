#!/usr/bin/env bash
set -euo pipefail

# Watch dark-factory execution with sound alerts.
# Polls every 60s, plays macOS sounds on state changes.
#
# Sounds:
#   3x Sosumi = prompt failed (or silent + stuck) — check log, fix, retry
#   Basso     = stuck — may need intervention; fired by either
#               (a) elapsed-time threshold (>=15min on "executing since"), or
#               (b) container-log quietness (>=10min elapsed AND <5 log lines
#                   in the last 3min — covers BOTH gen and exec modes)
#   Glass     = all prompts complete
#
# Liveness probe rationale: elapsed-time alone produces false positives on
# legitimately slow specs (a heavy gen container can run 12-18min on a 10-DB
# spec) and misses stuck gen containers entirely (the old "executing since"
# grep matches only exec mode, never gen). We use docker logs activity as
# the primary liveness signal; elapsed time is just the gate that decides
# when to check.

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
  # Use `prompt list` as the source of truth — string-matching against
  # `dark-factory status` output false-positives on filenames containing
  # "failed" (e.g. a prompt named `widen-reject-accept-failed.md`).
  if dark-factory prompt list 2>/dev/null | grep -qE "^\s*[0-9]+.*\s+failed\s*$"; then
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

  # At most one Basso per poll cycle: at >=15min stuck in exec mode the
  # elapsed-time branch below AND the liveness branch both qualify — without
  # this guard the sound plays twice in quick succession.
  BASSO_FIRED=0

  # Alert: stuck >15 minutes (exec mode, elapsed-time fallback).
  # `|| true` swallows grep's exit 1 when "executing since" is absent
  # (e.g. daemon is in spec-generation mode, not prompt-execution mode);
  # without it `set -euo pipefail` kills the watcher on every poll.
  MINS=$(echo "$STATUS" | grep "executing since" | grep -o '[0-9]*m' | tr -d 'm' || true)
  if [ -n "$MINS" ] && [ "$MINS" -ge 15 ]; then
    echo "ALERT: STUCK >15min on $CURRENT"
    afplay /System/Library/Sounds/Basso.aiff
    BASSO_FIRED=1
  fi

  # Liveness probe: covers BOTH gen and exec modes.
  # Resolve the active container from `Container: <name> (running)` in the
  # `dark-factory status` output. Skip cleanly if docker is unavailable or
  # the container line is absent (daemon idle). `head -n1` hardens against
  # a future formatter emitting multiple Container: lines.
  CONTAINER=$(echo "$STATUS" | grep -E '^[[:space:]]*Container:' | head -n1 | perl -pe 's/.*Container:\s*([^[:space:]]+).*/$1/' || true)
  # Defense in depth: only accept docker-legal container names so a future
  # `dark-factory status` formatter change cannot smuggle regex metacharacters
  # into the docker filter below.
  if [[ ! "$CONTAINER" =~ ^[a-zA-Z0-9][a-zA-Z0-9_.-]*$ ]]; then
    CONTAINER=""
  fi
  if [ -n "$CONTAINER" ] && command -v docker >/dev/null 2>&1; then
    # `docker ps` reports container age in a human-readable form ("14 minutes",
    # "About a minute", "About an hour", "2 hours", "3 days"). We parse the
    # leading integer and unit; "About an hour" (docker's literal output for
    # the 45-90min window) maps to 60 so the probe still fires there; any
    # day/week/month/year unit is far beyond the 10min gate, so it maps to a
    # flat 1440. Anything shorter ("seconds", "About a minute") rounds to 0 —
    # no probe. Name filter is anchored (^/name$) — unanchored is substring
    # match and can return another container's uptime.
    RUNNING_FOR=$(docker ps --filter "name=^/${CONTAINER}\$" --format '{{.RunningFor}}' 2>/dev/null || true)
    ELAPSED_MIN=0
    if [[ "$RUNNING_FOR" =~ ^([0-9]+)[[:space:]]minute ]]; then
      ELAPSED_MIN="${BASH_REMATCH[1]}"
    elif [[ "$RUNNING_FOR" =~ ^([0-9]+)[[:space:]]hour ]]; then
      ELAPSED_MIN=$(( BASH_REMATCH[1] * 60 ))
    elif [[ "$RUNNING_FOR" =~ ^About[[:space:]]an[[:space:]]hour ]]; then
      ELAPSED_MIN=60
    elif [[ "$RUNNING_FOR" =~ (day|week|month|year) ]]; then
      ELAPSED_MIN=1440
    fi
    if [ "$ELAPSED_MIN" -ge 10 ]; then
      # Count log lines in the last 3 minutes. Healthy containers emit
      # tool-use / tool-result JSON every few seconds → dozens of lines.
      # Stuck containers go silent → 0-4 lines.
      # A failing `docker logs` (container gone between ps and logs, daemon
      # hung, permissions) must NOT be conflated with "container is silent" —
      # that would escalate a docker problem to Sosumi+break. On failure we
      # skip the probe for this cycle instead.
      if DOCKER_LOG_OUT=$(docker logs --since=3m "$CONTAINER" 2>/dev/null); then
        LOG_LINES=$(printf '%s' "$DOCKER_LOG_OUT" | grep -c . || true)
      else
        echo "WARN: docker logs failed for $CONTAINER — skipping liveness probe this cycle"
        LOG_LINES=99
      fi
      if [ "$LOG_LINES" -lt 5 ]; then
        if [ "$ELAPSED_MIN" -ge 15 ] && [ "$LOG_LINES" -eq 0 ]; then
          echo "ALERT: SILENT + STUCK — $CONTAINER quiet for >=3min at ${ELAPSED_MIN}m (0 log lines)"
          afplay /System/Library/Sounds/Sosumi.aiff
          afplay /System/Library/Sounds/Sosumi.aiff
          afplay /System/Library/Sounds/Sosumi.aiff
          break
        fi
        # Deliberate three-band behavior at >=15min: 0 lines = silent+stuck →
        # break above; 1-4 lines = low-traffic, alert (Basso) but KEEP
        # watching; 5+ lines = healthy, no alert. Do not merge the 0 and 1-4
        # bands — a trickle of output means the container is alive.
        echo "ALERT: QUIET — $CONTAINER only $LOG_LINES log lines in last 3min at ${ELAPSED_MIN}m"
        if [ "$BASSO_FIRED" -eq 0 ]; then
          afplay /System/Library/Sounds/Basso.aiff
        fi
      fi
    fi
  fi

  sleep 60
done
