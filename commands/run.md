---
description: One-shot execution of all queued dark-factory prompts (processes queue and exits)
allowed-tools: [Read, Bash, Glob, AskUserQuestion]
---

Run dark-factory in one-shot mode from the current project root.

1. Verify `.dark-factory.yaml` exists in current directory
   - If not found, tell user to `cd` to the project root or run `/dark-factory:init-project`
2. Check if a daemon is already running: `cat .dark-factory.lock 2>/dev/null`
   - If running, warn user and ask whether to proceed (run will fail if lock held)
3. Show current queue status: `dark-factory status`
4. Ask user for confirmation before starting
5. Run `dark-factory run` via Bash tool with `run_in_background: true` and `timeout: 600000`
   - This processes all queued prompts and exits
6. After completion, show results:
   - `dark-factory status`
   - `dark-factory prompt list`
   - Check for failed prompts in `prompts/in-progress/` with `status: failed`
7. If any prompts failed:
   - Show log: `cat prompts/log/<name>.log | tail -50`
   - Suggest: fix prompt or code, then `dark-factory prompt retry`
