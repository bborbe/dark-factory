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
| Spec-based flow (auto-generated prompts) | `daemon` | Watches for new prompts as spec generates them |

**Rule of thumb:** Use `daemon` when you'll be iterating or have multiple prompts. Use `run` for a single known prompt where you want it to finish and exit.

**From Claude Code:** Use Bash tool with `run_in_background: true`:

```bash
# run_in_background: true
dark-factory daemon
```

Don't run in the foreground (blocks your session) or detached with `&` (loses lifecycle tracking).

**Multiple projects:** Each project has its own lock file — you can run one daemon per project simultaneously.

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

## Workflow: Direct

Default workflow. Dark-factory commits, tags, and pushes to the current branch.

```
approve prompt → daemon executes → commit + tag + push → done
```

- Queue multiple prompts at once — they execute sequentially
- Review with `git log -1` and `git diff HEAD~1`

## Workflow: PR

Dark-factory creates a feature branch and opens a PR.

```
approve prompt → daemon executes → feature branch + PR → review → merge
```

- Queue one prompt at a time — next depends on previous being merged
- After merge, update CHANGELOG and release on master

## Versioning

If your project has a `CHANGELOG.md`, dark-factory automatically:
- Determines version bump (patch/minor) from changes
- Updates CHANGELOG.md with new version
- Creates a git tag (e.g., `v0.3.4`)
- Pushes both commit and tag

Without `CHANGELOG.md`, dark-factory commits and pushes without tagging.

## Retrospective

After each successful prompt, spend 2 minutes:

| Question | If wrong... |
|----------|-------------|
| Wrong code pattern? | Update YOLO docs |
| Wrong test pattern? | Update YOLO docs |
| Vague prompt → wrong output? | More specificity next time |
| Missing project convention? | Update project CLAUDE.md |

## Troubleshooting

| Problem | Fix |
|---------|-----|
| Lock error on start | Another instance running — check `cat .dark-factory.lock` |
| Prompt not picked up | Must be in `prompts/in-progress/`, use `dark-factory prompt approve` |
| Failed prompt blocks queue | Fix prompt/code, then `dark-factory prompt retry` |
| Container not found | Ensure claude-yolo image is pulled |
| YOLO stuck or slow | Simplify prompt, add more specificity |
| Container running for hours | Check `docker logs` — may be in a retry loop. Stop container, fix prompt |
| `go mod download` fails | Check `netrcFile` and `GOPRIVATE` in config |

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
