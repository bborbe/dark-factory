---
status: approved
spec: [088-disable-auto-prompt-generation]
created: "2026-05-24T00:00:00Z"
queued: "2026-05-23T22:30:51Z"
branch: dark-factory/disable-auto-prompt-generation
---

<summary>
- `README.md` "User-level defaults" paragraph (line 153) updated to include `disableAutoGeneratePrompts` in the supported-keys list
- `docs/configuration.md` gains a new subsection documenting `disableAutoGeneratePrompts`: the flag purpose, the trigger condition, the expected INFO log line, and example manual invocation via `/dark-factory:generate-prompts-for-spec`
- `main.go` `--set` help text (around lines 969 and 991) updated to include `disableAutoGeneratePrompts` in the supported-keys list
- `make precommit` passes
</summary>

<objective>
Document the `disableAutoGeneratePrompts` flag in README, `docs/configuration.md`, and the CLI help text so operators understand the behavior and how to manually trigger generation when the flag is `true`.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Key files to read before making changes:
- `README.md` — lines 148-158, the "User-level defaults" paragraph. Add `disableAutoGeneratePrompts` to the list of supported keys.
- `docs/configuration.md` — full file. Add a new subsection under a relevant heading (after the spec-watcher behavior or near the CLI flags section). The subsection covers: flag purpose, trigger condition, expected log line, manual invocation example.
- `main.go` — `printRunHelp` (lines 957-977) and `printDaemonHelp` (lines 980-999). The `--set` supported-keys list is at lines 969 and 991.

The `docs/configuration.md` section for the `--set` table (around lines 436-450) already lists all `--set` supported keys. Add `disableAutoGeneratePrompts` there as well.
</context>

<requirements>

### 1. Update `README.md` "User-level defaults" paragraph (line 153)

Change from:

```
**User-level defaults** in `~/.dark-factory/config.yaml` apply across every project that doesn't override them. Supports `model`, `hideGit`, `autoRelease`, `dirtyFileThreshold`, `maxContainers`. Precedence: default ← global ← project ← CLI arg.
```

to:

```
**User-level defaults** in `~/.dark-factory/config.yaml` apply across every project that doesn't override them. Supports `model`, `hideGit`, `autoRelease`, `dirtyFileThreshold`, `maxContainers`, `disableAutoGeneratePrompts`. Precedence: default ← global ← project ← CLI arg.
```

Add `disableAutoGeneratePrompts` at the end of the list (alphabetical order not required — follow existing pattern).

### 2. Add `disableAutoGeneratePrompts` to the `--set` table in `docs/configuration.md`

In the `--set key=value` section (around line 446), add a new row to the table:

```
| `disableAutoGeneratePrompts` | bool (`true` or `false`) | `--set disableAutoGeneratePrompts=true` |
```

Place it after `autoMerge` row. Add a brief note after the table: "When set to `true`, the spec watcher will NOT auto-fire the generator container when a spec is approved. Use `/dark-factory:generate-prompts-for-spec <spec-path>` to trigger generation manually."

### 3. Add a new subsection in `docs/configuration.md` for the disable-auto-generation behavior

Add a new subsection after the `--set key=value` section (the table updated in Requirement 2) and BEFORE the `## Common Patterns` heading. Title: `### Disable Auto Prompt Generation`.

Content:

````markdown
### Disable Auto Prompt Generation

Suppress the automatic spec-to-prompts generation that fires when a spec moves to `status: approved`.

```yaml
disableAutoGeneratePrompts: true
```

| Field | Default | Purpose |
|-------|---------|---------|
| `disableAutoGeneratePrompts` | `false` (enabled) | When `true`, the spec watcher will NOT auto-fire the generator container when a spec is approved. Operators run `/dark-factory:generate-prompts-for-spec <spec-path>` manually to trigger generation. |

**When to use**: You want to approve a spec to lock its contents but defer prompt generation — review the generated prompts before the container runs, run with custom args, or skip generation entirely for spec-only experiments.

**Behavior**:
- `disableAutoGeneratePrompts: false` (default): Approving a spec triggers the generator container. Prompts appear in `prompts/` automatically.
- `disableAutoGeneratePrompts: true`: Approving a spec logs an INFO line and does NOT start the generator. The spec stays at `status: approved` in `specs/in-progress/`.

**Expected log line** (INFO level):
```
spec approved — auto-generation disabled, run /dark-factory:generate-prompts-for-spec <spec-path> manually
```

**Manual invocation** (when flag is `true`):
```bash
# Trigger generation for a specific spec
/dark-factory:generate-prompts-for-spec <spec-path>

# Example: trigger for spec 088
/dark-factory:generate-prompts-for-spec specs/in-progress/088-disable-auto-prompt-generation.md
```

**Per-invocation override** (no yaml editing needed):
```bash
dark-factory daemon --set disableAutoGeneratePrompts=true
dark-factory run --set disableAutoGeneratePrompts=false
```

**Layering precedence**: default (`false`) ← global (`~/.dark-factory/config.yaml`) ← project (`.dark-factory.yaml`) ← CLI (`--set`). Matches `hideGit`, `autoRelease`, and other user-pref fields.

**Note**: This only affects the automatic trigger. The `commands/generate-prompts-for-spec.md` command on the host always works regardless of this flag.
````

### 4. Update `main.go` `printRunHelp` supported-keys list (line 969)

Change from:

```
Supported keys: hideGit, autoRelease, dirtyFileThreshold, model, maxContainers, workflow, pr, autoMerge
```

to:

```
Supported keys: hideGit, autoRelease, dirtyFileThreshold, model, maxContainers, workflow, pr, autoMerge, disableAutoGeneratePrompts
```

### 5. Update `main.go` `printDaemonHelp` supported-keys list (line 991)

Same change as step 4 — add `disableAutoGeneratePrompts` to the supported-keys list in `printDaemonHelp`.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do NOT change any code behavior — only documentation
- The `--set` table entry in `docs/configuration.md` must include the bool type and an example
- The new subsection must mention the exact INFO log line format
- The manual invocation must reference `/dark-factory:generate-prompts-for-spec` with a path argument
</constraints>

<verification>
```bash
# Documentation presence
grep -n 'disableAutoGeneratePrompts' README.md
grep -niE 'auto-generation disabled|disableAutoGeneratePrompts' docs/configuration.md
grep -n 'disableAutoGeneratePrompts' main.go

# Help text includes the new key
grep '"disableAutoGeneratePrompts"' main.go

# Final validation
make precommit
```
</verification>