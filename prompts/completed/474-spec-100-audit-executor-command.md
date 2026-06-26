---
status: completed
spec: [100-centralize-subprocess-runner]
summary: Added GoDoc comment to commandRunner interface in pkg/executor/command.go documenting why it is deliberately not routed through pkg/subproc.Runner — it owns the docker SIGINT-then-SIGKILL escalation protocol; confirmed 0 exec.Command calls in command.go and correct allow-list state; CHANGELOG updated.
container: dark-factory-exec-474-spec-100-audit-executor-command
dark-factory-version: v0.183.0
created: "2026-06-26T07:30:00Z"
queued: "2026-06-26T07:57:18Z"
started: "2026-06-26T08:39:53Z"
completed: "2026-06-26T08:43:33Z"
branch: dark-factory/centralize-subprocess-runner
---

<summary>

- Audits the docker executor's command-running seam to decide whether it can route through the central subprocess runner without breaking docker's stop/kill signal protocol.
- Confirms the executor's command seam itself spawns no process — it only runs a pre-built docker command and manages the SIGINT-then-SIGKILL escalation that docker stop/kill depends on.
- Concludes the seam stays as-is (it is the docker signal protocol the spec explicitly keeps out of scope) and records that conclusion as an inline comment so future readers understand why it is exempt.
- Confirms the actual docker-spawning calls already live in allow-listed files, so the lint gate is unaffected.
- No behavior change — this prompt is a documented audit plus a one-line clarifying comment.

</summary>

<objective>
Audit `pkg/executor/command.go` against `pkg/subproc.Runner` (spec Desired Behavior 6). Determine whether its `commandRunner` seam can spawn through the runner without breaking the docker SIGINT-then-SIGKILL escalation protocol. Document the outcome with an inline comment. If migration is impossible without churn, leave it as-is — and confirm the docker SPAWN sites it serves are already allow-listed (they are, from prompt 1), so no allow-list edit is needed.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` first, then `/workspace/CLAUDE.md`.

Read the parent spec end-to-end:
- `/workspace/specs/in-progress/100-centralize-subprocess-runner.md` — Desired Behavior 6 and 8; Acceptance Criteria 9 (allow-list path); Non-goal "Do NOT replace the docker CLI executor"; Failure Modes row 2.

Read these source files before editing:
- `/workspace/pkg/executor/command.go` — the `commandRunner` interface (`Run(ctx, cmd *exec.Cmd) error`), `defaultCommandRunner`, and its `Run` method. KEY OBSERVATION (verify by reading): `command.go` contains NO `exec.Command(...)` / `exec.CommandContext(...)` CALL. It only accepts an already-constructed `*exec.Cmd` and runs it via `cmd.Start()` + `cmd.Wait()`, with a goroutine that sends `os.Interrupt` on `ctx.Done()` and escalates to `cmd.Process.Kill()` after 10 s. That escalation is the docker SIGINT-then-SIGKILL protocol the spec keeps out of scope.
- `/workspace/pkg/executor/executor.go` — where the docker `*exec.Cmd`s are actually CONSTRUCTED: lines 224, 278, 283, 333, 479, 562, 573 (`exec.CommandContext(ctx, "docker", ...)`). These are the real spawn sites, and `pkg/executor/executor.go` is ALREADY in the execcheck allow-list (added in prompt 1). `command.go` merely RUNS the cmd handed to it via `e.commandRunner.Run(ctx, cmd)`.
- `/workspace/scripts/hotpath-execcheck.sh` (created in prompt 1) — confirm the allow-list contains `pkg/executor/{checker,stopper,executor,launch}.go` but NOT `pkg/executor/command.go`.

Decision is already determined by the code (do NOT re-deliberate): `command.go` spawns nothing, so it does NOT need migration AND does NOT need an allow-list entry (the gate only flags `exec.Command(Context)?\(` CALLS, and `command.go` has none). Its `Run` method legitimately holds a `*exec.Cmd` parameter and the signal-escalation logic — that is the docker protocol, exempt by spec Non-goal. This prompt records that conclusion; it does NOT migrate anything.
</context>

<requirements>

## 1. Confirm the audit facts

1.1. Run `grep -nE 'exec\.Command(Context)?\(' pkg/executor/command.go` — it MUST return 0 matches. (If it returns any match, the audit assumption is wrong: STOP and report `Status: failed` with the offending line, because the spec's Desired Behavior 6 analysis would need revisiting — do NOT improvise a migration.)

1.2. Confirm `pkg/executor/command.go` is NOT in the execcheck allow-list in `scripts/hotpath-execcheck.sh` and does NOT need to be (it has no spawn call). Confirm `pkg/executor/executor.go` (which DOES construct the docker cmds) IS allow-listed.

## 2. Record the audit conclusion as an inline comment

In `/workspace/pkg/executor/command.go`, add a doc comment above the `commandRunner` interface (or extend the existing `// commandRunner runs an external command.` comment) explaining the audit outcome, so a future reader knows this seam was deliberately NOT routed through `subproc.Runner`:

```go
// commandRunner runs an already-constructed *exec.Cmd, managing the docker
// SIGINT-then-SIGKILL escalation on context cancellation (cmd.Process.Signal
// then cmd.Process.Kill after a grace period). This is the docker stop/kill
// signal protocol and is deliberately NOT routed through pkg/subproc.Runner:
// the runner offers warn+timeout semantics over cmd.Output() but no signal
// escalation, and the docker *exec.Cmd construction lives in executor.go
// (allow-listed in scripts/hotpath-execcheck.sh). See spec 100 Desired
// Behavior 6 / Non-goal "Do NOT replace the docker CLI executor".
type commandRunner interface {
	Run(ctx context.Context, cmd *exec.Cmd) error
}
```

Do NOT change the interface, the `defaultCommandRunner` implementation, or any signal-handling logic — behavior is unchanged. This is a comment-only edit.

## 3. No code migration

3.1. Do NOT add `command.go` to the allow-list (it has no spawn to exempt).
3.2. Do NOT change any docker spawn site in `executor.go` (out of scope — spec Non-goal).
3.3. Do NOT alter the `commandRunner` / `CommandRunner` mock (`mocks/command-runner.go`).

## 4. CHANGELOG

Append to `## Unreleased` in `/workspace/CHANGELOG.md`:
```
- docs: document why pkg/executor/command.go's commandRunner seam stays outside pkg/subproc.Runner — it owns the docker SIGINT-then-SIGKILL escalation protocol; docker *exec.Cmd construction remains in the allow-listed executor.go (spec 100 prompt 4)
```

</requirements>

<constraints>

- Do NOT migrate `command.go` or any `pkg/executor` file to `subproc.Runner` — the docker signal protocol is out of scope (spec Non-goal "Do NOT replace the docker CLI executor").
- Do NOT add `pkg/executor/command.go` to the execcheck allow-list — it spawns nothing, so there is nothing to exempt. Adding it would be a meaningless allow-list entry.
- Do NOT change the `commandRunner` interface signature, the `defaultCommandRunner.Run` logic, or the signal-escalation timing (10 s grace) — behavior must be byte-identical.
- If requirement 1.1 finds a real `exec.Command(Context)?` CALL in `command.go` (contradicting the audit assumption), STOP and report `Status: failed` — do NOT improvise a migration that could break the SIGINT protocol.
- No new Go dependencies (spec Constraint).
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.

</constraints>

<verification>

```bash
cd /workspace

# Audit fact: command.go spawns nothing
grep -nE 'exec\.Command(Context)?\(' pkg/executor/command.go    # expected: 0 matches

# allow-list: command.go NOT present; the real spawn file (executor.go) IS present
grep -n 'pkg/executor/command.go' scripts/hotpath-execcheck.sh  # expected: 0 matches
grep -n 'pkg/executor/executor.go' scripts/hotpath-execcheck.sh # expected: 1 match

# behavior unchanged — executor tests still pass
go test -mod=mod ./pkg/executor/...

# the audit comment landed
grep -n 'Do NOT route\|deliberately NOT routed\|SIGINT-then-SIGKILL' pkg/executor/command.go  # expected: >= 1 line

# Full precommit (execcheck still warn — green)
make precommit                                                  # expected: exit 0
```

</verification>
