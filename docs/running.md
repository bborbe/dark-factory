# Running Dark Factory

How to start dark-factory, monitor execution, handle failures, and complete specs.

## Starting the Daemon

Run from the project root (where `.dark-factory.yaml` lives):

```bash
dark-factory daemon
```

- **`daemon`** — watches for new prompts, processes continuously
- **`run`** — one-shot mode, processes all queued prompts and exits

### When to use daemon vs run

| Scenario | Command | Why |
|----------|---------|-----|
| Multiple prompts queued | `daemon` | Processes all sequentially, watches for more |
| Iterating on failures (retry → fix → retry) | `daemon` | Auto-picks up retried prompts without restarting |
| Single prompt, want quick feedback | `run` | Exits when done — no cleanup needed |
| CI/automation | `run` | Predictable lifecycle, exits with status code |
| Spec-based flow (auto-generated prompts) | `daemon` | Watches for new prompts as spec generates them (see [Two ways to generate prompts](#two-ways-to-generate-prompts-from-an-approved-spec)) |

**Rule of thumb:** Use `daemon` when you'll be iterating or have multiple prompts. Use `run` for a single known prompt where you want it to finish and exit.

**From Claude Code:** Use Bash tool with `run_in_background: true`:

```bash
# run_in_background: true
dark-factory daemon
```

Don't run in the foreground (blocks your session) or detached with `&` (loses lifecycle tracking).

**Multiple projects:** Each project has its own lock file — you can run one daemon per project simultaneously. **After a spec renumber or external-reference drift**, run `dark-factory doctor` from the project root before resuming the daemon — it surfaces stale external references the daemon can no longer auto-fix. See [Detecting State Anomalies](#detecting-state-anomalies).

## Two ways to generate prompts from an approved spec

When `dark-factory spec approve <name>` moves a spec to `status: approved`, prompts can be generated two ways. Both produce the same artifacts in `prompts/in-progress/`; they differ only in *who decides when generation runs*.

| | Auto (default) | Manual |
|---|----------------|--------|
| **Trigger** | Daemon's spec watcher fires the generator container as soon as the spec hits `approved` | Operator invokes `/dark-factory:generate-prompts-for-spec <spec-path>` from a Claude Code session |
| **Config** | `autoGeneratePrompts: true` in `.dark-factory.yaml`, `~/.dark-factory/config.yaml`, or `--set autoGeneratePrompts=true` | `autoGeneratePrompts: false` (default) |
| **Daemon role** | Generator + auditor + approver + executor all in one continuous loop | Daemon only executes prompts after the operator queues them; the manual command transitions the spec to `status: prompted` on success — same lifecycle outcome as the auto path |
| **Latency** | Seconds after `spec approve` | Whenever the operator runs the command |
| **LLM cost timing** | Spent immediately on approve | Deferred until operator decides |

### When to pick auto

- **Hands-off batch work** — multiple approved specs land overnight; daemon chews through them without intervention.
- **You trust the spec already** — audit happened during `/dark-factory:audit-spec` and you don't expect the generator output to need a pre-execution review.
- **You want the daemon never idle** — fastest path from "spec approved" to "PR open".

Cost: if the spec turns out to be subtly wrong, you pay the generation tokens before noticing. Interrupting mid-generation is also more disruptive (cancel the container, reset the spec).

### When to pick manual

- **You want to re-read the spec one more time** before paying generation cost.
- **You're approving specs in bulk but only want to generate one at a time** (e.g., staggered review).
- **You're experimenting** — approve a spec to lock its contents, but defer or skip generation entirely.
- **The generator is being upgraded** (new model, new prompt template) and you want to pick the moment to switch.

Cost: more steps — you must remember to run the manual command, and until you do, the spec sits at `status: approved`. Once the manual command runs, the spec transitions to `status: prompted` automatically via `dark-factory spec mark-prompted` — same final state as the auto path.

### Switching modes

Per-project (persistent):

```yaml
# .dark-factory.yaml
autoGeneratePrompts: true   # or false
```

Per-invocation (no yaml editing):

```bash
dark-factory daemon --set autoGeneratePrompts=true
dark-factory run    --set autoGeneratePrompts=false
```

Manual command (when auto is disabled):

```bash
/dark-factory:generate-prompts-for-spec specs/in-progress/088-disable-auto-prompt-generation.md
```

On success this command also transitions the spec from `approved` to `prompted` (via `dark-factory spec mark-prompted`), so the spec's lifecycle status matches the auto path.

Field reference and layering precedence: [configuration.md § Disable Auto Prompt Generation](configuration.md#disable-auto-prompt-generation).

## Monitoring

### Watch with sound alerts

```bash
/dark-factory:watch
```

Auto-detects the project directory (via `.dark-factory.lock`), polls every 60s, and plays macOS sounds:
- 3x Sosumi = prompt failed — check log, fix, retry
- Basso = stuck >15min — may need intervention
- Glass = all prompts complete

Works from any directory — no need to `cd` to the project root first.

### Check status

```bash
dark-factory status          # combined status of prompts and specs
dark-factory prompt list     # list all prompts with status
dark-factory spec list       # list all specs with status
```

### Check container logs

```bash
docker logs --tail 30 dark-factory-NNN-prompt-name
```

### Check execution logs

```bash
cat prompts/log/NNN-prompt-name.log
```

## Handling Failures

When a prompt fails (`status: failed`):

1. **Check the log:** `prompts/log/NNN-name.log`
2. **Look for the completion report** at end of log (blockers field explains why)
3. **Fix the issue** — either the prompt (clarify instructions, reduce scope) or the project code
4. **Retry:**

```bash
dark-factory prompt retry    # re-queues all failed prompts
```

The daemon picks up retried prompts automatically.

## Stopping the Daemon

```bash
# Check if running
cat .dark-factory.lock

# Stop a specific instance
kill $(cat .dark-factory.lock)
```

**Never use `pkill -f dark-factory`** — this kills ALL instances across all projects.

**Don't delete `.dark-factory.lock`** — the lock is flock-based. A new instance acquires it automatically when the old process exits.

## Completing Specs

When all prompts for a spec are completed, dark-factory auto-transitions the spec to `verifying`.

Verify the acceptance criteria pass, then mark complete:

```bash
dark-factory spec complete <spec-id>
```

See [spec-verification.md](spec-verification.md) for the full verification procedure.

## Workflow: Direct

`workflow: direct` (default). Dark-factory commits to the current branch. Push and tag are gated by `autoRelease` and `CHANGELOG.md` (see [Versioning](#versioning) below and [configuration.md](configuration.md)).

```
approve prompt → daemon executes → commit → (push if autoRelease) → (tag if CHANGELOG.md) → done
```

- Queue multiple prompts at once — they execute sequentially
- Review with `git log -1` and `git diff HEAD~1`

## Workflow: Branch / Worktree / Clone

`workflow: branch | worktree | clone` puts work on a feature branch. With `pr: true`, dark-factory pushes and opens a PR; with `pr: false`, the branch stays local (or pushed without a PR for `clone`). The isolation mode (`branch`, `worktree`, `clone`) controls where the working tree lives — see [workflows.md](workflows.md) for the matrix.

```
approve prompt → daemon executes → feature branch + PR → review → merge
```

- Queue one prompt at a time — next depends on previous being merged
- After merge, update CHANGELOG and release on master

## Versioning

Two orthogonal switches:

| `autoRelease` | `CHANGELOG.md` | Behavior |
|---------------|----------------|----------|
| `false` (default) | * | commits stay local — no push, no tag |
| `true` | absent | commit + push (no version, no tag) |
| `true` | present | commit + push + bump `## Unreleased` → `## vX.Y.Z` + create tag + push tag |

When both `autoRelease: true` and `CHANGELOG.md` are set, dark-factory automatically:
- Determines version bump (patch/minor) from the changelog content
- Renames `## Unreleased` → `## vX.Y.Z`
- Creates a git tag (e.g., `v0.3.4`)
- Pushes both commit and tag

See [configuration.md](configuration.md) for the field reference and [release-process.md](release-process.md) for the full release procedure (including the pre-release scenario gate).

## Retrospective

After each successful prompt, spend 2 minutes:

| Question | If wrong... |
|----------|-------------|
| Wrong code pattern? | Update YOLO docs |
| Wrong test pattern? | Update YOLO docs |
| Vague prompt → wrong output? | More specificity next time |
| Missing project convention? | Update project CLAUDE.md |

## Detecting State Anomalies

`dark-factory doctor` is a read-only diagnostic that scans your `specs/` and `prompts/` trees for state anomalies — duplicate spec numbers, stale `verifying` timestamps, orphan spec/prompt links, and more. Run on demand. It exits 0 when the project is clean, 1 with a categorized report when findings exist. Each finding names the affected file paths and a copy-paste command line that an operator can run to fix it manually.

### Usage

- `dark-factory doctor` — scan and report; exit 0 when clean, 1 with findings
- `dark-factory doctor --fix` — scan, prompt `Apply? [y/N]` per finding, apply safe fixes
- `dark-factory doctor --fix --yes` — same as `--fix` but auto-accepts all confirmations (for scripted cleanup)
- `dark-factory doctor --verifying-stale-hours=48` — override the default 24h stale-`verifying` threshold

### Detection categories

| Category | What it catches | Copy-paste fix |
|---|---|---|
| `duplicate-spec-numbers` | Two `.md` files in the same lifecycle dir share a `NNN-` prefix | `dark-factory spec renumber <id-to-move>` |
| `prompted-but-not-swept` | A spec is in `prompted` state but all its prompts are already `completed`/`rejected`/`cancelled` and it hasn't transitioned to `verifying` | `dark-factory spec sweep <spec-id>` |
| `verifying-stale` | A spec is in `verifying` with no progress in the last 24h (configurable via `--verifying-stale-hours`) | `dark-factory spec verify <spec-id>` (informational — no auto-fix) |
| `orphan-prompt-link` | A prompt's `spec: [NNN]` references a spec id with no `.md` file in any `specs/*/` dir | `dark-factory prompt unlink <prompt-id>` (relink alternative provided in finding `Detail`) |
| `orphan-in-progress-prompt` | A prompt lives in `prompts/in-progress/` but its parent spec is already `completed` or `rejected` | `dark-factory prompt cancel <prompt-id>` |
| `status-dir-mismatch` | A spec or prompt's `status:` field contradicts the lifecycle directory it lives in (e.g. `status: completed` inside `specs/in-progress/`) | `dark-factory spec move <spec-id>` (or the prompt equivalent) |

### Audit log

`dark-factory doctor --fix` appends one line per action to `.dark-factory/doctor.log` (mode 0644) for traceability. Each line contains timestamp (RFC3339), finding category, target file path(s), action taken, before-state, and after-state. When a spec is renumbered, the `previous_id` frontmatter field on the renamed spec records the prior number so the rename is reversible by reading the frontmatter.

### Read-only by default

`dark-factory doctor` (without `--fix`) never writes to `specs/` or `prompts/`. Safe to run from CI, from a script, or from a Claude Code session — the worst case is a non-zero exit code.

## Troubleshooting

| Problem | Fix |
|---------|-----|
| Lock error on start | Another instance running — check `cat .dark-factory.lock` |
| Stale external references after a spec renumber (PR description, commit message, vault task) | Run `dark-factory doctor` to see affected files; the daemon no longer silently renumbers specs on startup — see [Detecting State Anomalies](#detecting-state-anomalies) |
| Prompt not picked up | Must be in `prompts/in-progress/`, use `dark-factory prompt approve` |
| Failed prompt blocks queue | Fix prompt/code, then `dark-factory prompt retry` |
| Container not found | Ensure claude-yolo image is pulled |
| YOLO stuck or slow | Simplify prompt, add more specificity |
| Container running for hours | Check `docker logs` — may be in a retry loop. Stop container, fix prompt |
| `go mod download` fails | Check `netrcFile` and `GOPRIVATE` in config |

## ID Formats

`dark-factory spec` and `dark-factory prompt` subcommands taking an `<id>` argument accept four equivalent formats:

| Format | Example | Notes |
|--------|---------|-------|
| Padded number | `063` | Matches the format shown in `spec list` / `prompt list` output |
| Unpadded number | `63` | Quick typing |
| Full basename | `063-bug-foo-bar` | Tab-completion friendly |
| With `.md` extension | `063-bug-foo-bar.md` | Convenient when copy-pasting from `ls` |

When two specs share a number (defensive case — the daemon assigns unique numbers, so this should never occur in practice), the CLI errors with `ambiguous spec id <input>: <list-of-paths>` and exits non-zero.

## Project Detection

When run outside a project root (no `.dark-factory.yaml` in the current directory), `dark-factory` walks up the directory tree to find one — same convention as `git`. The walk stops at `$HOME`. If no `.dark-factory.yaml` is found, the CLI exits non-zero with `not a dark-factory project: no .dark-factory.yaml in <cwd> or any parent directory`.

This means you can run `dark-factory spec list` from any subdirectory of a project — no need to `cd` to the root first.

## CLI Reference

| Command | Purpose |
|---------|---------|
| `dark-factory daemon` | Watch and process continuously |
| `dark-factory run` | One-shot: process queue and exit |
| `dark-factory status` | Combined status overview |
| `dark-factory prompt list` | List prompts with status |
| `dark-factory prompt approve <name>` | Queue a prompt |
| `dark-factory prompt retry` | Re-queue failed prompts |
| `dark-factory spec list` | List specs with status |
| `dark-factory spec approve <name>` | Approve a spec |
| `dark-factory spec complete <name>` | Mark verified spec as done |
| `dark-factory spec mark-prompted <name>` | Transition a spec to `prompted` (used by the manual generation flow) |
| `dark-factory doctor [--fix] [--yes] [--verifying-stale-hours=N]` | Detect (and optionally fix) state anomalies in `specs/` and `prompts/` |
