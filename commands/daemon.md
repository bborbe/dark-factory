---
description: Start dark-factory daemon to watch and process prompts continuously
allowed-tools: [Read, Bash, Glob, AskUserQuestion]
---

Start dark-factory daemon in background for the current project.

1. Verify `.dark-factory.yaml` exists in current directory
   - If not found, tell user to `cd` to the project root or run `/dark-factory:init-project`
2. Check if a daemon is already running: `cat .dark-factory.lock 2>/dev/null`
   - If lock exists, check if PID is alive: `kill -0 $(cat .dark-factory.lock) 2>/dev/null`
   - If alive, inform user daemon is already running and show `dark-factory status`
   - Ask user if they want to stop it first: `kill $(cat .dark-factory.lock)`
3. Show current queue status: `dark-factory status`
4. Ask user for confirmation before starting
5. Start daemon via Bash tool with `run_in_background: true` and `timeout: 600000`:
   ```
   dark-factory daemon
   ```
6. Confirm daemon started successfully
7. Show how to monitor:
   - `dark-factory status` — check prompt/spec status
   - `dark-factory prompt list` — list all prompts
   - `docker logs --tail 30 <container-name>` — check container output
   - `kill $(cat .dark-factory.lock)` — stop the daemon

Never use `pkill -f dark-factory` — it kills ALL instances across all projects.
