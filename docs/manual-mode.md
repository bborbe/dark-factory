# Manual Mode (no Go binary, no daemon)

You don't have to install dark-factory or run the daemon to use the workflow. The slash commands shipped with this plugin form a complete spec → prompt → execute → verify chain you can drive interactively from Claude Code, one step at a time.

This document is for operators who:

- Want to try dark-factory without `go install`-ing anything
- Have one-off work and don't want the daemon's batch overhead
- Are learning the workflow before committing to the autonomous setup
- Run in a constrained env where the binary or Docker daemon isn't an option

## The chain

Six commands, in order. Each step is independent and you decide when to run the next one.

```
/dark-factory:create-spec "<task description>"           # 1. draft a spec
/dark-factory:audit-spec specs/<spec-file>               # 2. quality gate (preflight)
/dark-factory:generate-prompts-for-spec specs/<file>     # 3. spec → prompts
/dark-factory:audit-prompt prompts/<prompt-file>         # 4. quality gate (DoD) — once per prompt
/dark-factory:run-prompt prompts/<prompt-file>           # 5. execute in YOLO container
/dark-factory:verify-spec specs/<file>                   # 6. end-to-end scenario + mark complete
```

| # | Command | What it does | Reads | Writes | Quality gate |
|---|---|---|---|---|---|
| 1 | `create-spec` | Drafts a spec under `specs/ideas/` from a task description | task description | `specs/ideas/<name>.md` (status: idea) | — |
| 2 | `audit-spec` | Checks the spec against the preflight checklist + quality criteria | spec file | audit report (stdout) | gates step 3 |
| 3 | `generate-prompts-for-spec` | Splits the spec into one or more prompts under `prompts/` | spec file | `prompts/<n>-<name>.md` (one per slice) | — |
| 4 | `audit-prompt` | Checks each generated prompt against the Definition of Done | prompt file | audit report (stdout) | gates step 5 |
| 5 | `run-prompt` | Executes the prompt headless in a claude-yolo container; commits + pushes | prompt file | git commits in the target repo | — |
| 6 | `verify-spec` | Walks the spec through end-to-end verification (scenario replay), then marks complete | spec + all its prompts | spec status: completed | — |

Steps 2 and 4 are quality gates — they don't mutate disk, they just report. If they pass you proceed; if they fail you fix the spec/prompt and re-run.

## When to use manual mode

- **Single prompt, want the loop closed before stepping away.** Daemon is overhead — you'd start it, wait, kill it.
- **First-time use.** Run the chain step-by-step to see what each command produces before trusting the daemon to do it unattended.
- **Spec exploration.** You want to draft + audit a spec, then come back later to generate prompts.
- **No Go toolchain.** You only have Claude Code; you don't want `go install` or `make install`.
- **Air-gapped / restricted env.** Docker may be available for `run-prompt`'s YOLO container but the daemon's filesystem watcher isn't.

## When to use the daemon instead

The slash-command chain is operator-paced. The daemon is autonomous. Pick the daemon when:

- **You have ≥2 prompts queued and want them processed sequentially.** The daemon picks the next one up the moment the previous one commits.
- **You're going AFK.** The daemon watches the inbox and processes new prompts whenever you drop them in.
- **You want spec → prompts → execution end-to-end with no operator touches.** Set `autoGeneratePrompts: true` in `.dark-factory.yaml` and the daemon does steps 3 + 5 automatically once a spec hits `approved`.
- **You're iterating on failures.** The daemon picks up retried prompts without you having to remember to invoke `run-prompt` again.

See [running.md](running.md) for daemon setup.

## Trade-offs

| Axis | Manual chain | Daemon |
|---|---|---|
| Setup | Zero — slash commands ship with the plugin | `go install` + `.dark-factory.yaml` + Docker running |
| Concurrency | Sequential, one operator action per step | Sequential per project, one daemon per project |
| Latency between steps | Whatever you choose | Seconds (watcher polls) |
| Unattended runs | No — needs you between steps | Yes — designed for AFK |
| Step skipping | You decide which gates to run | Always runs the configured workflow |
| Cost timing | Pay LLM tokens only on the steps you invoke | Pay as the daemon advances |
| Crash recovery | Re-run the failed step | Daemon re-queues on the next tick |

The two modes share the same artifacts on disk — `specs/`, `prompts/`, the same statuses, the same lifecycle file moves. You can start manual, then switch to the daemon partway through (e.g. `audit-spec` manually, then start the daemon to handle the rest). Or vice versa: kill the daemon, finish the last step by hand.

## Worked example

Say you want a one-line fix in repo `~/Documents/workspaces/foo`:

```bash
# Step 1 — draft the spec
/dark-factory:create-spec "Add --json flag to foo CLI for machine-readable output"
# → produces specs/ideas/add-json-flag-to-foo-cli.md

# Step 2 — audit
/dark-factory:audit-spec specs/ideas/add-json-flag-to-foo-cli.md
# → audit report on stdout. If issues, edit the spec and re-run.

# Approve it (moves to specs/, assigns a number)
dark-factory spec approve add-json-flag-to-foo-cli
# → specs/088-add-json-flag-to-foo-cli.md

# Step 3 — generate prompts
/dark-factory:generate-prompts-for-spec specs/088-add-json-flag-to-foo-cli.md
# → prompts/088-add-json-flag.md (or several, if the spec was sliced)

# Step 4 — audit each prompt
/dark-factory:audit-prompt prompts/088-add-json-flag.md
# → DoD audit report. Fix and re-run if needed.

# Approve (moves to in-progress)
dark-factory prompt approve 088-add-json-flag

# Step 5 — execute
/dark-factory:run-prompt prompts/in-progress/088-add-json-flag.md
# → YOLO container runs, commits the change, exits.

# Step 6 — verify end-to-end
/dark-factory:verify-spec specs/088-add-json-flag-to-foo-cli.md
# → replays the scenario, marks spec status: completed on success.
```

You can stop after any step — the artifacts are durable on disk and the next step picks up where you left off whenever you come back.

## When the chain breaks

- **Step 2 (`audit-spec`) fails** — fix the spec file, re-run. The audit doesn't mutate state; the spec stays at `status: idea`.
- **Step 4 (`audit-prompt`) fails** — same: fix the prompt, re-run. Status stays at `pending`.
- **Step 5 (`run-prompt`) fails inside the container** — `dark-factory prompt list` shows the failure; read container logs (`docker logs --tail 30 dark-factory-NNN-...`), fix the prompt, `dark-factory prompt retry`, and re-run `/dark-factory:run-prompt` (or let the daemon pick it up).
- **Step 6 (`verify-spec`) fails** — scenario replay surfaced a real defect. Treat as a new spec (step 1 with the defect as the task), don't try to re-execute the failed scenario in-place.

For the full failure taxonomy see [troubleshooting.md](troubleshooting.md).

## Related

- [running.md](running.md) — daemon mode, `dark-factory daemon` vs `dark-factory run`
- [spec-verification.md](spec-verification.md) — what step 6 (`verify-spec`) actually walks through
- [configuration.md § Disable Auto Prompt Generation](configuration.md#disable-auto-prompt-generation) — controls whether the daemon does step 3 automatically
- [dod.md](dod.md) — the prompt Definition of Done that step 4 audits against
