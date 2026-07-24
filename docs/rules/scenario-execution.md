# Scenario Execution Guide

How to actually WALK a scenario as the operator. Paired with `scenario-writing.md` (which is for authors).

When you need this doc: you are about to walk one or more scenarios — release-gate, regression check, or one-off verification — and you want a procedure, not improvisation.

When you do NOT need this doc: you are writing or modifying a scenario file. That is `scenario-writing.md`.

## Preflight (before the first scenario)

These must all hold before walking anything. Failing here is cheap; failing mid-walk is expensive.

```bash
# 1. Docker daemon up
docker info >/dev/null 2>&1 || { echo "docker daemon required"; exit 1; }

# 2. gh CLI authed (scenarios 002, 015, 018 hit GitHub)
gh auth status >/dev/null 2>&1 || gh auth login

# 3. Fresh binary built from CURRENT HEAD (not the installed one)
rm -rf /tmp/new-dark-factory                  # may exist as a directory from a prior failure
go build -C ~/Documents/workspaces/dark-factory -o /tmp/new-dark-factory .

# 4. Binary reports the unreleased version (not a tag)
/tmp/new-dark-factory --version   # expect "dark-factory dev"

# 5. Sandbox staging dir not full
df -h "$(dirname "$(mktemp -u)")" | tail -1
# expect plenty of free space; sandboxes can be 100s of MB each
# Note: macOS `mktemp -d` returns paths under `/var/folders/<hash>/T/`, NOT `/tmp`.
# Use the helper above to check the right filesystem.
```

If `/tmp/new-dark-factory` already exists as a directory (left over from a previous failed scenario), `go build -o` fails with "Is a directory". `rm -rf` first.

Leave the live dark-factory daemon running if it is. Scenarios spawn into per-run sandbox dirs with `maxContainers: 999`, which bypasses the system-wide cap.

### Expected noise in logs (not regressions)

Modern dark-factory binaries emit these on every run. Don't flag them as scenario failures:

| Log line | Cause | Action |
|---|---|---|
| `level=WARN ... config file is world-readable ... chmod 600` | Sandbox `.dark-factory.yaml` written without restrictive perms | Ignore. Optional: chmod the file post-write in your scratch copy of the scenario |
| `level=INFO ... prompt_load legacy_container_alias=true file=prompts/completed/...` | Spec 102 compat shim reading legacy `container:` frontmatter on the canonical sandbox's completed prompts | Ignore. Floods the log; not a regression |
| `level=INFO ... 'worktree' is deprecated in .dark-factory.yaml; use 'workflow' instead` | Some scenarios still use the legacy `worktree:` yaml key | Ignore. Scenarios will migrate over time |
| `level=INFO ... acquired lock file=.dark-factory.lock` | Daemon's normal startup | Ignore |

If you see one of these, do NOT debug it. If a scenario assertion grep fails because of one of these, the assertion is poorly written — file a fix to the scenario, not the binary.

### Shell CWD resets between commands

Every Bash invocation in this guide / a manual walk runs in a fresh shell. CWD does NOT persist between commands. You must `cd "$WORK_DIR/<sandbox>"` at the start of every Action and Expected step, even if a previous step already `cd`'d there.

The canonical pattern in scenario files is to print `WORK_DIR=...` at the end of Setup so the next step can recover it.

## Walking a scenario

Each active scenario file has four sections: **Setup → Action → Expected → Cleanup**. Each section is a checkbox list. Walk top-to-bottom, flipping `[ ]` → `[x]` as you complete each item, in a personal scratch copy of the file (do NOT modify the canonical file in `scenarios/`).

| Phase | What it does | What "done" means |
|---|---|---|
| **Setup** | Builds sandbox, primes initial state | Every `[ ]` flipped, no errors. Helper `lib.sh` does most of this. |
| **Action** | Runs the binary against the sandbox | Commands return expected exit code (usually 0). |
| **Expected** | Observes the resulting state | Every assertion holds. This is the actual test. |
| **Cleanup** | Removes sandbox + side-effects | `WORK_DIR` removed, GH PRs/branches deleted, daemon stopped. |

If the scenario uses a helper script (only 013 today via `scenarios/helper/run-013-all.sh`), the script flips everything for you and reports `Result: N passed, M failed`. Otherwise: walk by hand.

## Helper batches vs manual walk

| Scenario | Runner | Notes |
|---|---|---|
| 013 | `bash scenarios/helper/run-013-all.sh` | 42 sub-scenarios A–M, fully automated, ≈30 s |
| All others (active) | manual | Open the file, follow Setup → Action → Expected, observe output |

Run the helper batch first — fastest signal that the binary boots, config layering works, and the sandbox primitives in `lib.sh` are intact. If 013 fails, do NOT continue to manual scenarios; fix the regression first.

## Reading the result

A scenario PASSES when every `[ ]` in Setup AND Action AND Expected becomes `[x]` with the observed behavior matching the assertion text. There is no partial pass — Expected is binary by section.

If an assertion does NOT hold:

- Capture the actual output (paste into a scratch note; you will lose it otherwise)
- Classify the failure (see next section)
- Stop walking. Do not try to "fix forward" by skipping the failing assertion.

## Fail classification (different debug paths)

| Phase that failed | What it tells you | First place to look |
|---|---|---|
| **Setup** | Sandbox primitives broken — `lib.sh`, missing `dark-factory-sandbox` source, `/tmp` full, helper script bug. NOT a binary regression. | `scenarios/helper/lib.sh`; the canonical sandbox dir; `df -h /tmp` |
| **Action** | The binary failed to execute the command. Could be: bad CLI flag handling, missing subcommand, container fails to start, panic. | The binary's own output; `/tmp/new-dark-factory --help`; recent commits touching `cmd/` or `pkg/runner/` |
| **Expected** | The binary ran but the outcome is wrong. This is the regression. | `pkg/` matching the symptom (see table below); the prompt log under `$WORK_DIR/<sandbox>/prompts/log/`; `$WORK_DIR/<sandbox>/.dark-factory.log` |

| Expected symptom | Most likely surface |
|---|---|
| YOLO container fails to start, log mentions `root/sudo privileges` | `pkg/runner/` UID remapping, or container image bump |
| Log truncated after `Starting headless session...` | `pkg/executor/streamfmt/`, container image |
| Git push fails, error swallowed | `pkg/git/` — verify stderr captured into the wrapped error |
| Spec stuck in `prompted` after prompts done | `pkg/processor/` workflow-executor phase ordering or sweep ticker |
| Container name collision | `pkg/generator/`, `pkg/processor/` spawn sites |
| `prompt_id=<id>` not in `.dark-factory.log` | `pkg/log/` context binding (spec 099) |

## Mid-scenario abort + cleanup

Sometimes you need to bail mid-walk — wrong scenario, environment problem, your own error. Leftover state will pollute the next scenario.

Always clean these:

```bash
# Sandbox dir (helper's EXIT trap usually removes this, but not if you SIGKILL'd)
echo "$WORK_DIR"                     # confirm path
rm -rf "$WORK_DIR"

# Containers left running
docker ps --filter label=dark-factory.project --format '{{.Names}}'
docker stop $(docker ps -q --filter label=dark-factory.project) 2>/dev/null

# GH-touching scenarios (002, 015, 018) — real PRs and branches on dark-factory-sandbox repo
gh pr list --repo bborbe/dark-factory-sandbox --state open
gh pr close <num> --delete-branch       # for each you opened

# Daemon you started by hand (helper scripts use foreground binaries; manual walks may not)
pgrep -f "/tmp/new-dark-factory" && pkill -f "/tmp/new-dark-factory"
```

If you abort scenario 002 mid-walk and don't clean the GH PR, the NEXT walk of 002 may find an unexpected open PR and the Setup assertion fails.

## Time budgets + babysit cadence

| Class | Examples | Wall time | Attendance |
|---|---|---|---|
| Helper batch | 013 (42 sub-scenarios) | ≈30 s | Run-and-glance; report line is enough |
| Pure CLI | 011 | < 1 min | Light babysit; assertions are mechanical |
| Single YOLO + CLI | 010, 012 | 1–2 min | Watch container start; otherwise hands-off |
| LLM-driving (execute only) | 001, 003, 006 | 25 s – 2.5 min each | Watch the prompt log scroll; intervene only on hang |
| LLM-driving (generate + audit) | 019 | **up to ~8 min** | See the 019 note below — do NOT size a wait loop off the older "≈2.5 min" figure |
| GitHub-touching | 002 | 1–2 min + manual GH cleanup | Watch + remember to clean up the PR after |
| Worktree gates | 021, 022 | 30 s – 1 min | Light babysit |
| backend:local | 024 | < 1 min | No container spawns at all — that IS the assertion |

Total full gate: ~45–60 min wall, ~$1 LLM cost (measured 2026-07-24 on claude-yolo v0.14.0). Plan accordingly — do not start the gate when you have 10 min before another commitment.

**019 runs far longer than the rest.** Its generate+audit phase alone was observed at ~6 min. If your wait loop expires just as `auto-approve: approved generated prompt` is logged, the daemon dies with the prompt queued-but-unexecuted — which looks exactly like the historical "executor never picked up the prompt" bug. It is not. Re-run `dark-factory run` in the same sandbox and the queued prompt completes normally.

Babysitting is not optional for LLM-driving scenarios: a stuck Claude run will sit at full token cost until you notice and `Ctrl-C` it. If you cannot babysit, do not start.

## Common pitfalls

| Pitfall | Symptom | Fix |
|---|---|---|
| `/tmp/new-dark-factory` is a directory from a prior failed `cp` | `go build -o`: "Is a directory" | `rm -rf /tmp/new-dark-factory` before `go build` |
| Dirty working tree blocks `setup_sandbox_copy` | helper aborts early; sandbox empty | `git stash -u` before the scenario, restore after |
| `gh auth` expired mid-walk (scenario 002 only) | `gh pr create` 401 | `gh auth login` and re-walk the Action phase |
| Sandbox dir leaks because helper EXIT trap was bypassed | parent dir of `mktemp -d` shows shrinkage | `ls /var/folders/.../T/tmp.*` (macOS) or `/tmp/tmp.*` (Linux) and `rm -rf` orphans |
| Stale daemon from previous walk still binds container slot | new scenario can't spawn; "no free slot" | `pgrep -f /tmp/new-dark-factory` and kill |
| Running the gate against the INSTALLED binary instead of `/tmp/new-dark-factory` | Scenario passes but you haven't tested the new code | Always `build_binary` or run preflight step 3 |
| Backgrounded daemon (e.g. scenario 022's `--set hideGit=true` check) leaves zombie process | `wait $PID` hangs in next command | Use `kill $PID 2>/dev/null; wait $PID 2>/dev/null` after the `sleep` |
| Scenario assertion looks for original prompt filename, but `dark-factory prompt approve` renumbered it | `ls prompts/in-progress/<original-slug>.md` fails | `find prompts/in-progress -name '*<slug>*'` or `grep -l <slug> prompts/in-progress/*.md` |
| Chained `&& echo OK || echo FAIL` skips middle commands when one early link fails | Later checks silently absent from output | Use one Bash invocation per assertion when capturing pass/fail per-check |
| Sandbox vanishes the instant Setup returns | `fatal: Unable to read current working directory: No such file or directory`, then every assertion fails at once | Was a `trap ... EXIT` registered *inside* `setup_sandbox_copy` — in **zsh** an in-function trap is function-local and fires on function return, not shell exit. Fixed 2026-07-24 by registering the trap once at source time. If you see this on an older checkout, update `lib.sh` or set the sandbox up by hand |
| Assertion greps for wording the binary no longer emits | Scenario reports FAIL while the behavior is provably correct | Read the actual output before believing a FAIL. Fix the scenario, not the binary (see "expected noise" above — same principle). Two were found this way on 2026-07-24: 013-G (`invalid` vs `does not match required pattern`) and 011 (unquoted-timestamp regex vs YAML-quoted `rejected: "…"`) |

## Per-scenario quick reference

Compact map. Each row is "what does this scenario actually exercise + watch-for".

| # | Title | Helper used | Watch for | Cleanup specific |
|---|---|---|---|---|
| 001 | direct workflow commits + tags + pushes | `setup_sandbox_copy` | new commit on master, version tag bumped, push to local bare remote | sandbox EXIT trap |
| 002 | PR workflow opens real PR | `setup_sandbox_copy` | real GH PR created on dark-factory-sandbox | `gh pr close --delete-branch` |
| 003 | container smoke test | `setup_sandbox_copy` | YOLO container starts, runs, exits clean | sandbox EXIT trap |
| 006 | full spec lifecycle | `setup_sandbox_copy` | spec moves through `approved → prompted → verifying` | sandbox EXIT trap |
| 010 | preflight baseline gate | `setup_sandbox_copy` | preflight runs `make precommit` before first prompt | sandbox EXIT trap |
| 011 | reject-spec cascade (pure CLI) | `scenario_setup` | `dark-factory spec reject` cascades to linked prompts | sandbox EXIT trap |
| 012 | `--skip-preflight` flag | `setup_sandbox_copy` | preflight skipped, first prompt runs immediately | sandbox EXIT trap |
| 013 | config layering (default<global<project<arg) | `run-013-all.sh` | "Result: 42 passed, 0 failed" | helper handles |
| 019 | spec-078 auto-approve happy path | `setup_sandbox_copy` | generate → audit → auto-approve → execute; **up to ~8 min**, most of it before the first container edit | sandbox EXIT trap |
| 021 | spec-084 daemon worktree gate | `setup_sandbox_copy` | worktree workflow refuses unsafe configs at daemon start | sandbox EXIT trap |
| 022 | spec-084 run worktree gate | `setup_sandbox_copy` | same gate enforced on `dark-factory run` (one-shot) | sandbox EXIT trap |
| 024 | spec-104 backend:local fails closed | manual | `claude` removed from PATH → `claude not found on PATH`, and **zero** docker containers spawned | sandbox EXIT trap |

## When the gate is the wrong tool

Scenarios cover the surfaces they exist for; they cannot exercise a brand-new subcommand introduced in the same release. For new user-facing subcommands, see `releasing-dark-factory.md` § "new-feature live-smoke" — every new subcommand needs a real-target run before `make install`, separate from the scenario walk.

## References

- `scenario-writing.md` — how to author new scenarios (paired with this doc)
- `releasing-dark-factory.md` — release gate that drives most scenario walks
- `scenarios/helper/lib.sh` — shared helper primitives every scenario uses
- `CLAUDE.md` § "Before `make install`" — the policy that mandates this walk
