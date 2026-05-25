---
status: failed
container: dark-factory-exec-425-review-dark-factory-4-fix-shell-script-cd
dark-factory-version: v0.171.1-3-gd94f1fa
created: "2026-05-24T00:00:00Z"
queued: "2026-05-25T14:51:20Z"
started: "2026-05-25T19:16:05Z"
completed: "2026-05-25T19:17:37Z"
lastFailReason: 'validate completion report: completion report status: partial'
---

<summary>
- Added `|| exit 1` to both `cd` calls in scenarios/helper/lib.sh
- Prevents silent failure where script continues in wrong directory if cd fails
- Two instances fixed: init_sandbox (line 54) and setup_sandbox_copy (line 85)
</summary>

<objective>
Fix shellcheck SC2164 warnings in scenarios/helper/lib.sh by adding exit checks after cd commands.
</objective>

<context>
Files to read before making changes:
- `scenarios/helper/lib.sh` — line 54 (`cd "$WORK_DIR"` in init_sandbox), line 85 (`cd "$WORK_DIR/$subdir"` in setup_sandbox_copy)
</context>

<requirements>
1. In `scenarios/helper/lib.sh`, line 54, change:
   ```
   cd "$WORK_DIR"
   ```
   to:
   ```
   cd "$WORK_DIR" || exit 1
   ```

2. In `scenarios/helper/lib.sh`, line 85, change:
   ```
   cd "$WORK_DIR/$subdir"
   ```
   to:
   ```
   cd "$WORK_DIR/$subdir" || exit 1
   ```
</requirements>

<constraints>
- Only change files in this repo
- Do NOT commit — dark-factory handles git
</constraints>

<verification>
shellcheck scenarios/helper/lib.sh
</verification>
