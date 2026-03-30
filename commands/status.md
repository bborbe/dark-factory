---
description: Show dark-factory status (daemon, queue, specs)
allowed-tools: [Bash]
---

Run `dark-factory status` in the current directory and show the output.

1. Verify `.dark-factory.yaml` exists in current directory
   - If not found, tell user to `cd` to the project root or run `/dark-factory:init-project`
2. Run `dark-factory status`
