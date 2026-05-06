---
description: Create, reconfigure, or auto-migrate .dark-factory.yaml
allowed-tools: [Read, Write, Edit, Bash, Glob, AskUserQuestion]
---

Manage `.dark-factory.yaml` for the current project. Routes by project state: greenfield → delegate to `init-project`; valid existing config → reconfigure menu; invalid config (legacy fields, schema mismatch) → auto-migrate flow.

Read `docs/configuration.md` from the dark-factory installation first for the field reference, then execute:

## Step 1: Detect state

```bash
test -f .dark-factory.yaml && echo "EXISTS" || echo "MISSING"
```

If `MISSING` → go to Step 2 (greenfield).

If `EXISTS` → run validation:

```bash
dark-factory config >/dev/null 2>&1 && echo "VALID" || echo "INVALID"
```

- `VALID` → go to Step 3 (reconfigure).
- `INVALID` → capture the error and go to Step 4 (auto-migrate):
  ```bash
  dark-factory config 2>&1 | tail -5
  ```

## Step 2: Greenfield (no .dark-factory.yaml)

Tell the user no config exists, then delegate to `init-project`:

> No `.dark-factory.yaml` found. Running `/dark-factory:init-project` for greenfield setup.

Invoke `/dark-factory:init-project` (let the user pass an argument or ask them to pick a workflow). STOP after init-project completes.

## Step 3: Reconfigure (valid existing config)

Show the current effective config:

```bash
dark-factory config
```

Then ask via AskUserQuestion (single question, numbered options):

> What would you like to change?
> 1. Switch workflow (direct / branch / worktree / clone)
> 2. Toggle a boolean (autoMerge / autoRelease / pr / hideGit)
> 3. Edit a specific field by name (advanced)
> 4. Run a known migration (e.g. drop legacy autoReview fields)
> 5. No changes — exit

Branch on the answer:

- **1. Switch workflow** — ask which workflow (single question, 4 options). Update `workflow:` in `.dark-factory.yaml`. If switching FROM `direct` TO `branch`/`worktree`/`clone` and `pr:` is unset, propose adding `pr: true` (most common combo).

- **2. Toggle boolean** — ask which field, ask new value (true/false). Update the YAML.

- **3. Edit by name** — ask which field name, ask new value. Read `pkg/config/config.go` `Defaults()` if needed to confirm valid field names. (Or grep the user's `.dark-factory.yaml` to list current fields.)

- **4. Migration** — go to Step 4 logic.

- **5. Exit** — stop with no write.

After ANY change, go to Step 5 (write + validate + commit).

## Step 4: Auto-migrate (invalid config)

Read the current `.dark-factory.yaml` and the captured `dark-factory config` error from Step 1. Match against known legacy patterns:

**Spec 073 — removed fields** (`autoReview`, `allowedReviewers`, `useCollaborators`, `maxReviewRetries`, `pollIntervalSec`):

If the error mentions any of these, propose dropping the offending field(s):

> Detected legacy field(s): `<list>`. These were removed in v0.151.0 — review enforcement now lives in GitHub branch protection. Remove from `.dark-factory.yaml`?

If yes → strip the named lines from the YAML, preserve everything else (including comments and key order).

**Other / unknown errors:**

Show the error and ask the user to describe the intended change. Edit accordingly.

After the proposed edit, go to Step 5.

## Step 5: Write, validate, commit

1. **Backup** the current file:
   ```bash
   cp .dark-factory.yaml .dark-factory.yaml.bak
   ```

2. **Show the diff** before writing:
   ```bash
   diff -u .dark-factory.yaml.bak <new-content> || true
   ```

3. **Confirm** via AskUserQuestion (single yes/no): "Write this change?"
   - No → restore (`mv .dark-factory.yaml.bak .dark-factory.yaml`), exit.

4. **Write** the new content via Edit/Write.

5. **Validate**:
   ```bash
   dark-factory config >/dev/null 2>&1
   ```
   - Exit 0 → success.
   - Non-zero → revert (`mv .dark-factory.yaml.bak .dark-factory.yaml`), report the error, stop.

6. **Clean up** backup on success:
   ```bash
   rm .dark-factory.yaml.bak
   ```

7. **Commit** (ask user first):
   ```bash
   git add .dark-factory.yaml
   git commit -m "configure: <summary of change>"
   ```

## Step 6: Summary

Print one-line summary of what changed (e.g. `workflow: direct → worktree, pr: true added`), or `no changes — operator cancelled`.

Remind the user to restart the daemon if it was running:

```bash
kill $(cat .dark-factory.lock 2>/dev/null) 2>/dev/null && dark-factory daemon
```

## Constraints

- Never modify any file outside the project root.
- Always backup to `.dark-factory.yaml.bak` before any write; remove on success, restore on failure.
- Always show the diff and require explicit confirmation before writing.
- Preserve YAML comments and key order — use Edit (line-targeted) rather than Write (full overwrite) when changing a subset of fields.
- If `dark-factory` binary is not on PATH, warn but allow the user to write the YAML anyway with a "validate yourself with the next dark-factory invocation" hint.
- Never invent field names — confirm against `pkg/config/config.go` `Defaults()` or the user's existing YAML.
