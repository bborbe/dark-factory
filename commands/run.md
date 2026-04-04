---
description: One-shot dark-factory run — generates prompts from approved specs, then executes all queued prompts
allowed-tools: [Read, Bash, Glob]
---

Run dark-factory in one-shot mode from the current project root.

`dark-factory run` does two things in order:
1. **Generates prompts** from all approved specs (no manual `/create-prompt` needed)
2. **Executes all queued prompts** (both newly generated and previously queued)

Steps:

1. Verify `.dark-factory.yaml` exists in current directory
   - If not found, tell user to `cd` to the project root or run `/dark-factory:init-project`
2. Check if a daemon is already running: `cat .dark-factory.lock 2>/dev/null`
   - If running, warn user (run will fail if lock held) and stop
3. Show current queue status: `dark-factory status`
   - Note: "Queue: 0 prompts" is normal if only specs are approved — prompts will be generated automatically
4. Run `dark-factory run` via Bash tool with `run_in_background: true` and `timeout: 600000`
5. After completion, show results:
   - `dark-factory status`
   - `dark-factory prompt list`
   - Check for failed prompts in `prompts/in-progress/` with `status: failed`
6. If any prompts failed:
   - Show log: `cat prompts/log/<name>.log | tail -50`
   - Suggest: fix prompt or code, then `dark-factory prompt retry`
