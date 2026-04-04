---
description: Start dark-factory daemon to watch and process prompts continuously
allowed-tools: [Read, Bash, Glob]
---

Start dark-factory daemon in background for the current project.

1. Verify `.dark-factory.yaml` exists in current directory
   - If not found, tell user to `cd` to the project root or run `/dark-factory:init-project`
2. Check if a daemon is already running: `cat .dark-factory.lock 2>/dev/null`
   - If lock exists, check if PID is alive: `kill -0 $(cat .dark-factory.lock) 2>/dev/null`
   - If alive, inform user daemon is already running and show `dark-factory status`, then stop
3. Start daemon via Bash tool with `run_in_background: true` and `timeout: 600000`:
   ```
   dark-factory daemon
   ```
4. Confirm daemon started and show monitor commands:
   - `dark-factory status` — check prompt/spec status
   - `dark-factory prompt list` — list all prompts
   - `docker logs --tail 30 <container-name>` — check container output
   - `kill $(cat .dark-factory.lock)` — stop the daemon

Never use `pkill -f dark-factory` — it kills ALL instances across all projects.
