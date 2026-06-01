---
status: completed
spec: [091-doctor-command]
container: dark-factory-doctor-exec-436-spec-091-add-doctor-fixer-and-cli
dark-factory-version: v0.173.0
created: "2026-06-02T00:00:00Z"
queued: "2026-06-01T22:42:15Z"
started: "2026-06-01T23:07:59Z"
completed: "2026-06-01T23:42:21Z"
---

<summary>
- Adds a `Fixer` to the `pkg/doctor` package that, given a slice of `Finding`s from prompt 1, applies the per-category mutation and writes one line to `.dark-factory/doctor.log` per action
- New `pkg/lock/filelock.go` exposes a per-file `flock`-based lock with a configurable acquire timeout (5s default); the fixer wraps every target-file mutation in this lock so a concurrent daemon cannot corrupt mid-write
- Adds `dark-factory doctor [--fix] [--yes] [--verifying-stale-hours=N]` as a top-level CLI command in `pkg/cmd/doctor.go`, with `no findings` on clean projects and exit code 1 when findings exist
- `--fix` prompts `Apply? [y/N]` per finding on stdin; `--yes` auto-accepts. The audit log is append-only with mode 0644; the directory `.dark-factory/` is created with 0755
- The renumber fix writes `previous_id: NNN` (string field) to the renamed spec's frontmatter and rewrites linked prompts' `spec:` field via the existing `reindex.UpdateSpecRefs` helper; prompt filenames are NOT touched
- Wired into the CLI tree via `factory.CreateDoctorCommand` and a new `case "doctor":` in `main.go`; help text in `printHelp` and a new `printDoctorHelp` are updated
- Adds the `PreviousID string` field to `spec.Frontmatter` with yaml tag `previous_id` — purely additive, no existing callers touched
- Ginkgo v2 + Gomega test suite covers: the 6 fix-action categories with golden fixture projects, audit-log append behavior, per-file lock contention (mocked), and CLI exit codes/stderr layout

</summary>

<objective>
Complete the `dark-factory doctor` user-facing surface: build the fixer that applies each finding from the existing `pkg/doctor.Checker` (prompt 1) under a per-file lock with an audit log, expose the whole thing as a top-level `dark-factory doctor [--fix] [--yes] [--verifying-stale-hours=N]` CLI command, and wire it through the existing CLI tree. Read-only detection lives in prompt 1; this prompt adds the writing half without re-introducing any daemon-driven auto-mutation.

</objective>

<context>
Read `/workspace/CLAUDE.md` first for project conventions.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-cli-guide.md` for the top-level CLI subcommand shape (this is a top-level command, not a `prompt` or `spec` subcommand).
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md` for the factory wiring pattern.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` for `errors.Wrap` / `errors.Errorf` usage.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-mocking-guide.md` and `go-testing-guide.md` for counterfeiter + Ginkgo/Gomega.

Files to read end-to-end before writing code:
- `/workspace/prompts/1-spec-091-add-doctor-detection-package.md` — the sibling prompt; the public types it defines (`Checker`, `Finding`, `Category`, `Deps`) are what this prompt consumes
- `/workspace/pkg/cmd/spec_status.go` and `/workspace/pkg/cmd/spec_show.go` — top-level-shaped `Run(ctx, args) error` commands with `--json` style flag parsing; mirror their structure
- `/workspace/pkg/cmd/spec_approve.go` and `/workspace/pkg/cmd/spec_complete.go` — find-then-mutate patterns; the fixer's renumber and orphan-cancel actions mirror these
- `/workspace/pkg/cmd/cmd_suite_test.go` — Ginkgo v2 + Gomega suite (60s timeout, `package_test`, `time.Local = time.UTC`)
- `/workspace/pkg/cmd/cancel.go` — has the closest "operator-confirms-then-mutates" shape; the fixer's interactive prompt borrows its `errors.Errorf` wrapping style
- `/workspace/pkg/lock/locker.go` — the existing project-wide locker pattern using `syscall.Flock` + `LOCK_EX|LOCK_NB`; the new `FileLock` reuses the same syscall primitive
- `/workspace/pkg/reindex/reindex.go` — `NewReindexer(dirs, mover).Reindex(ctx)` returns `[]Rename` (old/new path pairs); the fixer uses this to compute renumber renames then applies them through its own `FileMover`
- `/workspace/pkg/reindex/specref.go` — `UpdateSpecRefs(ctx, specRenames, promptDirs, mover, pm)` rewrites prompt frontmatter `spec:` fields; the fixer uses this for the renumber fix
- `/workspace/pkg/spec/spec.go` line 134–148 — `Frontmatter` struct; add a single field here
- `/workspace/pkg/spec/spec.go` line 301–441 — `AutoCompleter` interface + `CheckAndComplete(ctx, specID) error` method (the per-spec transition the fixer calls for `prompted-but-not-swept`)
- `/workspace/pkg/prompt/prompt.go` line 244–263 — `Frontmatter` struct (no `commit` field); the fixer reads `Frontmatter.Specs` for orphan-link detection and `pf.Save(ctx)` to write back
- `/workspace/pkg/prompt/prompt.go` line 311–345 — `load` + `Save` are the only safe way to read/write prompt files
- `/workspace/pkg/prompt/prompt.go` line 425–470 — `pf.MarkCompleted()`, `pf.MarkCancelled()`, `pf.MarkApproved()` helpers (use these rather than raw frontmatter writes)
- `/workspace/pkg/factory/factory.go` — `CreateStatusCommand` (line 1088), `CreateListCommand` (line 1119), `CreateCombinedStatusCommand` (line 1388) — closest analogues to a new `CreateDoctorCommand` (zero business logic, just constructor + dep injection)
- `/workspace/main.go` `runCommand` (line 146), `printHelp` (line 945), `printCommandHelp` (line 123) — dispatcher and help wiring
- `/workspace/mocks/` — counterfeiter output directory; naming `<package>-<interface-kebab>.go` with `//counterfeiter:generate -o ../../mocks/<file> --fake-name <Fake> . <Interface>` on the interface declaration
- `/workspace/pkg/subproc/subproc.go` — `subproc.NewRunner().RunWithWarnAndTimeout(ctx, op, name, args...)` for bounded subprocess calls; the `failed-but-merged` fix runs `git merge-base --is-ancestor <sha> HEAD` through this
- `/workspace/docs/architecture-flow.md` — current daemon lifecycle context; the fixer's audit log is a NEW file, separate from `.dark-factory.lock` and `.dark-factory.log`
</context>

<requirements>

1. **Extend `pkg/spec/spec.go` Frontmatter** (additive only — DO NOT touch any existing field):
   - Add `PreviousID string yaml:"previous_id,omitempty"` to the `Frontmatter` struct at line 134. Use `string` (NOT `int`) so the YAML form `previous_id: 056` is preserved on round-trip.
   - **CRITICAL: verify the YAML emitter produces UNQUOTED output.** `gopkg.in/yaml.v3` emits `string "056"` as `previous_id: "056"` (quoted with double-quotes) by default to avoid YAML 1.1 octal interpretation — this BREAKS the spec's AC line `grep -c '^previous_id: 056$' specs/in-progress/057-*.md` returns 1. Add a contract test in `pkg/spec/spec_test.go`:
     ```go
     It("PreviousID marshals as bare unquoted YAML (no leading-zero octal quoting)", func() {
         fm := spec.Frontmatter{PreviousID: "056"}
         out, err := yaml.Marshal(fm)
         Expect(err).NotTo(HaveOccurred())
         Expect(string(out)).To(ContainSubstring("previous_id: 056\n"))
         Expect(string(out)).NotTo(ContainSubstring(`previous_id: "056"`))
     })
     ```
     If the test fails (it likely will under default yaml.v3 behavior), implement a custom `MarshalYAML()` on `Frontmatter` that encodes `PreviousID` via `yaml.Node{Kind: yaml.ScalarNode, Style: 0, Value: fm.PreviousID, Tag: "!!str"}` — `Style: 0` (default) plus an explicit `!!str` tag forces yaml.v3 to emit unquoted. Confirm by re-running the test.
   - This is the only frontmatter change. The existing `SetBranchIfEmpty` pattern (line 262) is the closest analogue for "set-once-on-rename" semantics; mirror its GoDoc style.

2. **Add `pkg/lock/filelock.go`** (NEW file in the existing `pkg/lock` package — counterfeiter annotation required):
   - `FileLock` interface with two methods: `Acquire(ctx context.Context, timeout time.Duration) error` and `Release(ctx context.Context) error`.
   - `NewFileLock(path string) FileLock` constructor; the implementation stores `path+".lock"` as the lock file and reuses `syscall.Flock` with `LOCK_EX|LOCK_NB` (the same pattern as the existing `Locker.Acquire` at line 57).
   - `Acquire` polls every 100ms until either (a) the lock is acquired, (b) `timeout` elapses, or (c) `ctx` is cancelled. Return a wrapped error naming the path and the elapsed timeout on (b)/(c). This satisfies the spec § Failure Mode "On lock-acquire timeout (5s), print `skipped: <path> locked by another process`" — the CLI layer translates the error.
   - `Release` unlocks, closes the fd, and removes the lock file (matching the existing `Locker.Release` at line 83). On already-released, return nil (idempotent).
   - `//counterfeiter:generate -o ../../mocks/lock-file-lock.go --fake-name LockFileLock . FileLock` on the interface declaration. Generated `mocks/lock-file-lock.go` MUST appear in `/workspace/mocks/` after `go generate`.

3. **Add `pkg/doctor/fixer.go`** (NEW file):
   - `Fixer` interface with one method: `Apply(ctx context.Context, findings []Finding, opts ApplyOptions) (ApplyResult, error)`.
   - `ApplyOptions` struct with exported fields: `Yes bool` (skip confirmations), `Stdin io.Reader` (defaults to `os.Stdin` when nil), `Stdout io.Writer` (defaults to `os.Stdout`), `Stderr io.Writer` (defaults to `os.Stderr`), `AuditLogPath string` (defaults to `.dark-factory/doctor.log` relative to the project root resolved by `project.FindRoot`), `FileLockTimeout time.Duration` (defaults to `5*time.Second`).
   - `ApplyResult` struct with three slices: `Applied []AppliedFix`, `Skipped []SkippedFix`, `Failed []FailedFix`. Each entry has `Category Category`, `TargetPaths []string`, `Detail string`, plus a category-specific payload field (`FixCommand` mirrors the `Finding.FixCommand`).
   - `AppliedFix` adds `AuditLine string` (the exact line written to the audit log, so tests can assert on it without re-reading the file).
   - Counterfeiter annotation: `//counterfeiter:generate -o ../../mocks/doctor-fixer.go --fake-name DoctorFixer . Fixer`.
   - `NewFixer(deps FixerDeps) Fixer` constructor (no business logic). `FixerDeps` reuses the `Deps` struct from prompt 1 (since the fixer needs the same scanners) plus: `AutoCompleter spec.AutoCompleter`, `Mover prompt.FileMover`, `FileLockFactory func(path string) lock.FileLock` (defaults to `lock.NewFileLock`), `GitRunner subproc.Runner`, `CurrentDateTimeGetter libtime.CurrentDateTimeGetter`.
   - Private `fixer` struct holds deps. `Apply` iterates findings, dispatches each by `Category` to a private `fix*` method, prints `Apply? [y/N] ` to Stdout and reads one line from Stdin (unless `opts.Yes`), and writes the audit-log line on every action.

4. **Add per-category fixer files** in `pkg/doctor/`, one method per category. All file mutations MUST acquire the per-file lock first. Each `fix*` method signature: `func (f *fixer) fixX(ctx context.Context, finding Finding) (AppliedFix, error)`. Required files and behavior:

   - `pkg/doctor/fix_renumber.go` — `CategoryDuplicateSpecNumbers`. For each pair in `finding.TargetPaths`, compute the next free spec number. Approach: call `reindex.NewReindexer(specDirs, f.mover).Reindex(ctx)` to get a `[]Rename`, filter to renames whose `OldPath` is in `finding.TargetPaths`, then for each rename: acquire per-file lock on the OLD path, load the spec via `spec.Load`, set `Frontmatter.PreviousID = fmt.Sprintf("%03d", oldNum)` (use the existing `specnum.Parse`-extracted value, formatted as 3-digit to match the `^previous_id: 056` AC), `Save`, then call `f.mover.MoveFile`. After all renames, call `reindex.UpdateSpecRefs(ctx, renames, promptDirs, f.mover, f.promptManager)` to rewrite prompt frontmatter. If a slot is now taken (slot-churn case from spec § Failure Modes), recompute; if 3 attempts fail, log `failed: slot churn` and continue with the next finding.
   - `pkg/doctor/fix_sweep.go` — `CategoryPromptedNotSwept`. Call `f.autoCompleter.CheckAndComplete(ctx, specID)` where `specID` is extracted from the finding's `TargetPaths` via `filepath.Base` + `strings.TrimSuffix(..., ".md")`. The existing `AutoCompleter` transitions the spec from `prompted` to `verifying` when all linked prompts are completed; if `CheckAndComplete` returns no error, log success. If it returns the "spec not transitioning" silent-no-op behavior, log `skipped: linked prompts not all complete` and continue.
   - `pkg/doctor/fix_verifying_stale.go` — `CategoryVerifyingStale`. NO-OP: this finding is informational only. Log `skipped: verifying-stale is informational; run \`dark-factory spec verify <id>\` manually`. Return a `SkippedFix` with `Detail` set to that message.
   - `pkg/doctor/fix_unlink.go` — `CategoryOrphanPromptLink`. For each path in `finding.TargetPaths`: acquire per-file lock, load via `f.promptManager.Load`, find the orphan spec id in `Frontmatter.Specs` (it's the spec id named in the finding's `Detail`), rewrite the slice to remove the orphan (preserving any other specs), call `pf.Save(ctx)`. The `FixCommand` is `dark-factory prompt unlink <id>` for the audit log line; the relink alternative is provided as text in the finding's `Detail` (per spec DB #6) but the fixer does NOT apply relink automatically (relink requires operator judgment on the new spec id; only unlink is safe to auto-apply).
   - **No `fix_complete_merged.go`** — `CategoryFailedButMerged` is deferred per spec § Non-goals; do NOT create this file.
   - `pkg/doctor/fix_orphan_in_progress.go` — `CategoryOrphanInProgressPrompt`. For each path: acquire per-file lock, load via `f.promptManager.Load`. The fixer applies ONLY `cancel` (the safe, reversible default per spec DB #7) — NEVER `complete`. After `MarkCancelled` + `MoveToCancelled`, the file lives at a new path under `prompts/cancelled/`; do NOT then attempt a second mutation. Emit ONE `AppliedFix` entry per orphan. For `cancel`: call `pf.MarkCancelled()` and `f.promptManager.MoveToCancelled(ctx, path)`. If the prompt's status is not in the cancellable set (e.g., already `completed`), emit a `SkippedFix` with `Detail: "prompt no longer cancellable; current status=<status>"`.
   - `pkg/doctor/fix_status_dir_mismatch.go` — `CategoryStatusDirMismatch`. For each path: acquire per-file lock, compute the expected directory from the `Detail` (which names the contradiction), call `os.Rename(path, expectedDir/filepath.Base(path))`. The `os.MkdirAll(expectedDir, 0750)` call must precede the rename if the expected dir does not exist. The fixer handles BOTH spec and prompt variants — read the `FixCommand` prefix (`dark-factory spec move` vs `dark-factory prompt move`) to decide which subdir tree the path belongs to.
   - `parse-errors` (`CategoryParseError`) has NO fixer — log `skipped: parse-errors require manual YAML fix`.

5. **Add `pkg/doctor/audit.go`** (NEW file):
   - `WriteAuditEntry(ctx, path, entry AuditEntry) error` — appends one line to the file at `path` with mode 0644 (create if not exist, append otherwise). The directory containing `path` is created with mode 0755 if missing.
   - `AuditEntry` struct with exported fields: `Timestamp time.Time` (RFC3339 in the file), `Category Category`, `Action string` (`applied`, `skipped`, `failed`), `TargetPaths []string` (joined by space), `Before string` (one-line description of the prior state), `After string` (one-line description of the new state). Render format: `<rfc3339>\t<Category>\t<action>\t<targets>\t<before>\t<after>\n` (tab-separated for grep-ability).
   - The `AppliedFix.AuditLine` field in the result struct MUST be the exact string written, so tests can assert without re-reading the file.

6. **Add `pkg/cmd/doctor.go`** (NEW file):
   - `DoctorCommand` interface with `Run(ctx context.Context, args []string) error` (same shape as `SpecStatusCommand` at line 19 of `spec_status.go`).
   - `doctorCommand` struct holds the doctor `Checker`, `Fixer`, and the project root path (for the audit log location).
   - `NewDoctorCommand(checker Checker, fixer Fixer, projectRoot string) DoctorCommand` constructor (no business logic).
   - Counterfeiter: `//counterfeiter:generate -o ../../mocks/doctor-command.go --fake-name DoctorCommand . DoctorCommand`.
   - `Run` parses `--fix`, `--yes`, and `--verifying-stale-hours=N` (default 24) from `args`. Reject unknown flags with `errors.Errorf(ctx, "unknown flag: %q", arg)` and a stderr hint pointing to `dark-factory doctor --help`.
   - Call `checker.Check(ctx)`. If `len(findings) == 0`, print exactly `no findings\n` to stdout and return nil (exit code 0 — this contract is from the spec's Acceptance Criteria; the `\n` makes `diff <(dark-factory doctor) <(echo 'no findings')` empty).
   - Otherwise, group findings by `Category` and print one section per category that has findings. Each section starts with the category name (verbatim, e.g. `duplicate-spec-numbers`), then a one-line description, then one line per finding: `<target-paths>  <fix-command>`. Print to stdout. After all sections, call `fixer.Apply(ctx, findings, opts)` IFF `--fix` was set; if not `--fix`, return `errors.Errorf(ctx, "doctor found %d finding(s); re-run with --fix to apply", len(findings))` to drive exit code 1. (The `main.go` layer translates this error to exit code 1 — see step 8.)
   - Use the existing `errors.Errorf` / `errors.Wrap` style from `spec_status.go` line 46. No `fmt.Errorf`, no `context.Background()`.

7. **Wire the factory** (`pkg/factory/factory.go`):
   - Add `CreateDoctorCommand` (model on `CreateStatusCommand` at line 1088). It needs: all 8 spec/prompt lifecycle dirs from `cfg`, the spec `Lister`, the `prompt.Manager`, the `spec.AutoCompleter`, a `FileMover`, a `subproc.Runner`, the `CurrentDateTimeGetter`, and the `verifyingStaleHours` int from the CLI flag (passed through the dispatcher in main.go). Construct the `doctor.NewChecker` + `doctor.NewFixer` and pass to `cmd.NewDoctorCommand`.
   - The `VerifyingStaleHours` value comes from the CLI flag, NOT from config — pass it through the dispatcher in main.go (step 8) and into the factory via a new `CreateDoctorCommand(ctx, cfg, verifyingStaleHours, currentDateTimeGetter)` signature.

8. **Wire `main.go` dispatch**:
   - In `runCommand` (line 146), add a new top-level case (BEFORE `default:`):
     ```go
     case "doctor":
         if err := validateDoctorArgs(ctx, args, printDoctorHelp); err != nil {
             return err
         }
         hours, remaining, err := extractVerifyingStaleHours(ctx, args)
         if err != nil {
             return err
         }
         return factory.CreateDoctorCommand(ctx, cfg, hours, currentDateTimeGetter).Run(ctx, remaining)
     ```
   - `extractVerifyingStaleHours` mirrors `extractMaxContainers` at line 642 (parses `--verifying-stale-hours=N` and returns the int + remaining args). Reject non-integer values and values < 1 with `errors.Errorf(ctx, "--verifying-stale-hours value must be a positive integer, got %q", value)`.
   - `validateDoctorArgs` only accepts `--fix`, `--yes`, and `--verifying-stale-hours=N`; reject anything else with `errors.Errorf(ctx, "unknown flag: %q", arg)`.
   - In `printCommandHelp` (line 123), add `case "doctor": printDoctorHelp()`.
   - Add `printDoctorHelp` (model on `printStatusHelp` at line 1036): `Usage: dark-factory doctor [--fix] [--yes] [--verifying-stale-hours=N]` + 3-line description of what each flag does.
   - In `printHelp` (line 945), add a top-level entry in the Commands block (after the `status` row):
     ```
     "  doctor [--fix] [--yes] [--verifying-stale-hours=N]  Detect state anomalies (and optionally fix them)\n"+
     ```
   - In `ParseArgs` (line 1123), add `"doctor"` to the list of top-level commands at line 1171 (`case "run", "daemon", "kill", "status", "list", "config":`).
   - Confirm `runCommand` returns the error from `Run` and `main` exits 1 on non-nil error (already true at line 33–36). The spec's exit-code-1 contract is satisfied by the CLI's own `errors.Errorf` when findings exist AND `--fix` is NOT set.

9. **Update `pkg/cmd/cmd_suite_test.go`** if needed (it already covers all `pkg/cmd` tests; new tests go in `pkg/cmd/doctor_test.go` and inherit the suite).

10. **Generate mocks**: after creating each interface (`Fixer`, `DoctorCommand`, `FileLock`), run `cd /workspace && go generate -mod=mod ./pkg/doctor/... ./pkg/cmd/... ./pkg/lock/...`. Output files MUST appear at `mocks/doctor-fixer.go`, `mocks/doctor-command.go`, `mocks/lock-file-lock.go`. Do NOT hand-write mocks.

11. **Add Ginkgo tests** for the new code:
   - `pkg/cmd/doctor_test.go` (`package cmd_test`): minimum 6 `It` blocks: (1) zero findings → stdout `no findings` + nil error; (2) one finding in a category → category section header + fix line printed + non-nil error (so main.go exits 1); (3) `--fix` without `--yes` with a `DoctorFixer` fake that records `Apply` was called with the findings; (4) `--fix --yes` with the same fake — confirms `opts.Yes == true` is passed; (5) `--verifying-stale-hours=12` extracts the int; (6) unknown flag returns error.
   - `pkg/doctor/fixer_test.go` (`package doctor_test`): minimum 6 `It` blocks, one per fixable category. Each builds a golden fixture under `os.MkdirTemp("", "doctor-fix-<category>-*")`, calls `Apply` with `opts.Yes = true`, asserts the file was mutated as expected AND the audit-log line is well-formed AND `result.Applied` contains the expected entry. Use a fixed-clock `CurrentDateTimeGetter` (see prompt 1's instruction at step 5 for the pattern). Use the `mocks.LockFileLock` for the file-lock contention test: configure it to return an error on first `Acquire`, assert `result.Skipped` is populated and the audit log records `skipped`.
   - `pkg/doctor/audit_test.go` (`package doctor_test`): 3 `It` blocks — (1) `WriteAuditEntry` creates the file with mode 0644 if missing; (2) `WriteAuditEntry` appends to an existing file (assert second line is preceded by a newline); (3) directory creation uses mode 0755.
   - `pkg/lock/filelock_test.go` (`package lock_test`): 3 `It` blocks — (1) `Acquire` + `Release` round-trip; (2) contention: first lock holds, second `Acquire` with 200ms timeout returns error after ~200ms; (3) `Release` is idempotent (second call returns nil).
   - Per AC line: the 7-category minimum is `5 fix-action categories × ≥3 It each + 1 verifying-stale + 1 parse-error skip = 17+ It blocks` in `pkg/doctor/`. Combined with the 6 in `pkg/cmd/doctor_test.go`, total ≥ 26 `It` blocks added. The audit + filelock tests add ~6 more. AC line: `grep -c 'Describe\|Context\|It' /workspace/pkg/cmd/doctor_test.go /workspace/pkg/doctor/fixer_test.go /workspace/pkg/doctor/audit_test.go /workspace/pkg/lock/filelock_test.go` returns ≥ 30.

12. **Run `cd /workspace && make precommit`** — must pass. Fix any lint/format issues that appear (most likely: gofumpt formatting, godoc on exported identifiers, gosec file-mode annotations on the new `os.OpenFile` calls in `filelock.go`).

</requirements>

<constraints>
- DO NOT touch `pkg/runner/`, `pkg/factory/`'s `CreateRunner`/`CreateOneShotRunner`, `pkg/specwatcher/`, `pkg/processor/`, `main.go`'s `runRunCommand`/`runDaemonCommand` dispatch, or any daemon-tick code. That is the scope of prompt 3 (silent reconciliation removal). The factory's only edit in THIS prompt is the NEW `CreateDoctorCommand` function.
- DO NOT touch the read-only detection layer in `pkg/doctor/` (the `Checker` interface, the `Deps` struct, the 7 detector files, the test suite). That is prompt 1.
- DO NOT add new config keys to `.dark-factory.yaml`. The `--verifying-stale-hours` flag is a CLI-only arg (per spec § Constraints: "only `--verifying-stale-hours` is exposed as a CLI flag").
- DO NOT add an "auto-fix on startup" config flag (spec § Non-goals: "An escape hatch on the Goal is itself a regression").
- DO NOT add Prometheus metrics (spec does not call for them).
- DO NOT invoke the doctor from the daemon. Operators run `dark-factory doctor` on demand. The spec § Non-goals: "Do NOT auto-fix on startup".
- DO NOT cross project boundaries. `doctor` reads only the project rooted at `project.FindRoot` (spec § Non-goals).
- DO NOT introduce new file-walking code. Reuse the `reindex` package's unexported `scanByNumberPrefix` helper (added in prompt 1), `spec.Lister`, `prompt.Manager`, and `prompt.Counter` from the existing codebase.
- DO NOT modify the existing `lock.Locker` interface (project-wide lock). The new `lock.FileLock` is a separate type in the same package.
- All error wrapping via `github.com/bborbe/errors` (`errors.Wrap`, `errors.Errorf`). Never `fmt.Errorf`. Never `context.Background()` in pkg/ code.
- Counterfeiter mocks for new public interfaces (`Fixer`, `DoctorCommand`, `FileLock`). Reuse existing mocks (`spec.Lister`, `prompt.Manager`, `spec.AutoCompleter`, `subproc.Runner`) — do NOT re-mock them.
- External test packages (`package <name>_test`), Ginkgo v2 + Gomega, ≥80% statement coverage on the new code in `pkg/doctor/`, `pkg/cmd/`, `pkg/lock/`.
- File mode `0644` for audit log; `0755` for `.dark-factory/` directory; `0600` for the lock file (matching the existing `lock.Locker.Acquire` line 51). The gosec `#nosec` annotations must include a one-line reason (e.g. `// #nosec G304 -- path is operator-controlled .lock file`).
- Use `libtime.CurrentDateTimeGetter` for time injection — do NOT call `time.Now()` directly in detection or audit code.
- Do NOT commit. dark-factory handles git.
- Existing tests in `pkg/spec/`, `pkg/prompt/`, `pkg/reindex/`, `pkg/specsweeper/`, `pkg/specwatcher/`, `pkg/lock/` must still pass.
- The `dark-factory prompt complete` command's signature is NOT changed. The `dark-factory prompt complete <id> --reason=merged-externally` line in the spec's copy-paste output is for the operator to use the `prompt_complete` command — but the `prompt_complete` command does NOT support `--reason` today. The doctor fixer applies the completion directly (see requirement 4 `fix_complete_merged.go`) and does NOT shell out to `prompt_complete`. Document this divergence in the fixer's GoDoc on the constant for `FixCommand` template for the `failed-but-merged` category.

</constraints>

<verification>
- `cd /workspace && make precommit` exits 0.
- `cd /workspace && go test -count=1 -coverprofile=/tmp/doctor-fix.cover.out ./pkg/doctor/... ./pkg/cmd/... ./pkg/lock/...` exits 0 and `go tool cover -func=/tmp/doctor-fix.cover.out | tail -1` reports ≥80.0% on the changed packages.
- `ls /workspace/pkg/doctor/fixer.go /workspace/pkg/doctor/audit.go /workspace/pkg/doctor/fix_renumber.go /workspace/pkg/doctor/fix_sweep.go /workspace/pkg/doctor/fix_verifying_stale.go /workspace/pkg/doctor/fix_unlink.go /workspace/pkg/doctor/fix_complete_merged.go /workspace/pkg/doctor/fix_orphan_in_progress.go /workspace/pkg/doctor/fix_status_dir_mismatch.go /workspace/pkg/cmd/doctor.go /workspace/pkg/lock/filelock.go` all return 0 (all files exist).
- `ls /workspace/mocks/doctor-fixer.go /workspace/mocks/doctor-command.go /workspace/mocks/lock-file-lock.go` all return 0 (counterfeiter generated).
- `grep -c 'Describe\|Context\|It' /workspace/pkg/cmd/doctor_test.go /workspace/pkg/doctor/fixer_test.go /workspace/pkg/doctor/audit_test.go /workspace/pkg/lock/filelock_test.go` returns ≥ 30.
- `cd /workspace && grep -n 'PreviousID' pkg/spec/spec.go` returns at least 1 line (the new field is present).
- `cd /workspace && grep -n '"doctor"' main.go` returns at least 1 line (the dispatcher case is wired).
- `cd /workspace && grep -rn 'os.ReadDir\|filepath.Walk' pkg/doctor/*.go pkg/cmd/doctor.go pkg/lock/filelock.go` returns 0 lines (no new file-walking code outside what prompt 1 already added).
- `cd /workspace && git diff --name-only HEAD` shows ONLY files under `pkg/doctor/`, `pkg/cmd/`, `pkg/lock/`, `pkg/factory/`, `pkg/spec/`, `mocks/`, and `main.go`. No changes to `pkg/runner/`, `pkg/specwatcher/`, `pkg/processor/`, or any other package — those are prompt 3's scope.
- Manual end-to-end (inside YOLO container, against a throwaway fixture):
  ```
  cd /tmp && rm -rf df-doctor && mkdir -p df-doctor/specs/in-progress df-doctor/prompts/in-progress && cd df-doctor
  cat > specs/in-progress/056-foo.md <<EOF
  ---
  status: approved
  ---
  # Foo
  EOF
  cat > specs/in-progress/056-bar.md <<EOF
  ---
  status: approved
  ---
  # Bar
  EOF
  /tmp/dark-factory doctor                       # exit 1; prints "duplicate-spec-numbers" section
  /tmp/dark-factory doctor --fix --yes           # renames 056-bar to next-free number; audit log created
  cat .dark-factory/doctor.log                   # 1+ line
  /tmp/dark-factory doctor                       # exit 0; prints "no findings"
  ```
- `cd /workspace && go generate -mod=mod ./pkg/doctor/... ./pkg/cmd/... ./pkg/lock/...` exits 0 and `git status` shows the three new mock files (no other diff).

</verification>
