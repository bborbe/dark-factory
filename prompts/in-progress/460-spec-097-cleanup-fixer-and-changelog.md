---
status: approved
spec: [097-parent-dir-flock-locking]
created: "2026-06-18T12:13:00Z"
queued: "2026-06-18T12:10:24Z"
branch: dark-factory/parent-dir-flock-locking
---

<summary>

- `dark-factory doctor` gains a new check that detects leftover `*.lock` sidecar files (the litter from the old per-file locking scheme) anywhere under the prompt and spec status directories.
- `dark-factory doctor --fix` removes those leftover `.lock` files. The fix is idempotent — a second run finds nothing and is a no-op.
- The cleanup writes an audit-log entry per removed file, consistent with every other doctor fixer.
- This is the only code site in the whole project that may legitimately reference the `.lock` suffix after this spec — it exists solely to clean up the legacy artifacts.
- A `## Unreleased` CHANGELOG entry is added describing the parent-directory flock migration and the cleanup fixer, referencing spec 097.
- After this prompt, a full prompt + spec lifecycle leaves zero `*.lock` files under `prompts/` or `specs/`, and any pre-existing litter is removed on operator-invoked `doctor --fix`.

</summary>

<objective>
Add a new `dark-factory doctor` check + fixer that detects and (on `--fix`) removes pre-existing `*.lock` sidecar files left by the abandoned per-file locking scheme, anywhere under the prompt and spec status directories. The fixer is idempotent and audited. Add a `## Unreleased` CHANGELOG entry referencing spec 097.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` first, then `/workspace/CLAUDE.md`.

Read these coding-plugin docs:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — error wrapping
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo/Gomega, coverage >=80%
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — `## Unreleased` entry format, prefix required
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-security-linting.md` — gosec file-perms and `#nosec` with reasons (the fixer does `os.Remove`; reading dirs)

Read these source files end-to-end — they ARE the pattern to copy:
- `/workspace/pkg/doctor/doctor.go` — the `Category` constants, the `Finding` struct, the `Deps` struct (directory fields), `checker`, and the `Check(ctx)` method that runs each detector and concatenates findings. A NEW detector method + a NEW category constant + a registration line in `Check` are required.
- `/workspace/pkg/doctor/parse_errors.go` — `scanDirsForSpecs` / `scanDirsForPrompts` helpers (skip non-dirs, `os.IsNotExist` -> continue, only `.md`). The new detector mirrors this but globs for `*.lock` instead of `.md`.
- `/workspace/pkg/doctor/fixer.go` — `applyFinding`'s `switch finding.Category` dispatch (a new `case` is required), `ApplyOptions`, `AppliedFix`/`FailedFix`, the `FixerDeps`. NOTE: the fixer's `--yes`/confirm gate runs in `applyFinding` BEFORE the switch.
- `/workspace/pkg/doctor/fix_orphan_in_progress.go` — the cleanest fixer template: per-path loop, `WriteAuditEntry` BEFORE the mutation, returns `[]AppliedFix` / `[]FailedFix`. The cleanup fixer follows this shape but the mutation is `os.Remove(path)` and there is NO lock acquire (removing a stale empty sidecar needs no directory lock — the spec permits "a standalone scan before any locking").
- `/workspace/pkg/doctor/audit.go` — `AuditEntry{Timestamp, Category, Action, TargetPaths, Before, After}` and `WriteAuditEntry(ctx, path, entry)`. Use `Action: "applied"`, `Before: filepath.Base(path)`, `After: "removed"`.
- `/workspace/pkg/doctor/orphan_prompt_link_test.go` and `/workspace/pkg/doctor/fixer_test.go` — test patterns for a detector + a fixer (temp dirs, `mocks.LockDirLock`, `FixerDeps` construction).
- `/workspace/pkg/cmd/doctor.go` — the doctor command; no change needed (it already iterates all categories and prints/fixes them generically). Confirm the new category flows through `groupFindingsByCategory` and `applyFinding` without a special case in the command.
- `/workspace/CHANGELOG.md` — top of file; add a `## Unreleased` section above the latest `## vX.Y.Z` section (there is no existing `## Unreleased` — create it).
- `/workspace/specs/in-progress/097-parent-dir-flock-locking.md` — `Desired Behavior` 7 & 8, `Failure Modes` row "Pre-existing *.md.lock litter", `Acceptance Criteria` (cleanup AC, CHANGELOG AC, lifecycle AC), `Constraints` (CHANGELOG one-bullet referencing spec).

</context>

<requirements>

## 1. Add the new category constant

In `/workspace/pkg/doctor/doctor.go`, add a category constant alongside the existing ones:
```go
// CategoryLegacyLockFile indicates a leftover *.lock sidecar from the abandoned
// per-file locking scheme (spec 097). These empty files are removed by doctor --fix.
const CategoryLegacyLockFile Category = "legacy-lock-file"
```

## 2. Add the detector

Create `/workspace/pkg/doctor/legacy_lock_file.go` (BSD license header; package `doctor`). Add a checker method:
```go
func (c *checker) detectLegacyLockFiles(ctx context.Context) ([]Finding, error)
```
2.1. Scan EVERY prompt and spec status directory for `*.lock` files. Reuse the directory list pattern from `scanParseErrors`:
- prompt dirs: `c.deps.PromptsInboxDir`, `PromptsInProgressDir`, `PromptsCompletedDir`, `PromptsCancelledDir`
- spec dirs: `c.deps.SpecsInboxDir`, `SpecsInProgressDir`, `SpecsCompletedDir`, `SpecsRejectedDir`

2.2. For each directory: `os.ReadDir(dir)`; on `os.IsNotExist` -> `continue`; on other error -> `return nil, errors.Wrap(ctx, err, "read directory for lock-file scan: "+dir)`. For each entry that is NOT a dir and whose name ends with `.lock`, emit ONE finding:
```go
findings = append(findings, Finding{
	Category:    CategoryLegacyLockFile,
	TargetPaths: []string{filepath.Join(dir, e.Name())},
	Detail:      "legacy lock-file sidecar from old per-file locking scheme",
	FixCommand:  "dark-factory doctor --fix",
})
```
Use `strings.HasSuffix(e.Name(), ".lock")` for the match. Return the accumulated findings.

2.3. Register the detector in `Check` (`/workspace/pkg/doctor/doctor.go`): add a block AFTER `detectStatusDirMismatches` and BEFORE `scanParseErrors` (parse-errors must stay last per the existing comment):
```go
legacyLocks, err := c.detectLegacyLockFiles(ctx)
if err != nil {
	return nil, errors.Wrap(ctx, err, "detect legacy lock files")
}
all = append(all, legacyLocks...)
```

## 3. Add the fixer

Create `/workspace/pkg/doctor/fix_legacy_lock_file.go` (BSD header; package `doctor`). Mirror `fix_orphan_prompt_link.go` structure but with `os.Remove` and no lock acquire:
```go
func (f *fixer) fixLegacyLockFile(
	ctx context.Context,
	finding Finding,
	opts ApplyOptions,
) (applied []AppliedFix, failed []FailedFix) {
	for _, path := range finding.TargetPaths {
		af, ff := f.applyLegacyLockFilePath(ctx, path, finding, opts)
		if af != nil {
			applied = append(applied, *af)
		}
		if ff != nil {
			failed = append(failed, *ff)
		}
	}
	return
}

func (f *fixer) applyLegacyLockFilePath(
	ctx context.Context,
	path string,
	finding Finding,
	opts ApplyOptions,
) (applied *AppliedFix, failed *FailedFix) {
	// Audit BEFORE the mutation so a remove failure still leaves a record of intent.
	if err := WriteAuditEntry(ctx, opts.AuditLogPath, AuditEntry{
		Timestamp:   time.Time(f.deps.CurrentDateTimeGetter.Now()),
		Category:    finding.Category,
		Action:      "applied",
		TargetPaths: []string{path},
		Before:      filepath.Base(path),
		After:       "removed",
	}); err != nil {
		return nil, &FailedFix{
			Category:    finding.Category,
			TargetPaths: []string{path},
			Detail:      "audit log write failed: " + err.Error(),
		}
	}

	if err := os.Remove(path); err != nil {
		// Idempotent: a file already gone (concurrent doctor run / re-run) is success, not failure.
		if os.IsNotExist(err) {
			return &AppliedFix{
				Category:    finding.Category,
				TargetPaths: []string{path},
				FixCommand:  finding.FixCommand,
			}, nil
		}
		return nil, &FailedFix{
			Category:    finding.Category,
			TargetPaths: []string{path},
			Detail:      "remove failed: " + err.Error(),
		}
	}

	return &AppliedFix{
		Category:    finding.Category,
		TargetPaths: []string{path},
		FixCommand:  finding.FixCommand,
	}, nil
}
```
Imports: `context`, `os`, `path/filepath`, `time`. Confirm `f.deps.CurrentDateTimeGetter` is available on `FixerDeps` (it is — embedded `Deps`).

3.1. Register the fixer in `/workspace/pkg/doctor/fixer.go`'s `applyFinding` switch, adding a case:
```go
case CategoryLegacyLockFile:
	af, ff := f.fixLegacyLockFile(ctx, finding, opts)
	return af, nil, ff
```

## 4. Tests

4.1. Create `/workspace/pkg/doctor/legacy_lock_file_test.go` (package `doctor_test`; BSD header). Add a Ginkgo `Describe` covering the DETECTOR:
- Setup: temp dirs for all prompt + spec status dirs; construct a `Deps` and `NewChecker(deps)`.
- "detects a .lock file in a prompt directory": write `prompts/in-progress/002-x.md.lock` (empty), run `Check`, assert a finding with `Category == CategoryLegacyLockFile` and the right `TargetPaths`.
- "detects .lock files across multiple status dirs": write `.lock` files in a prompt dir AND a spec dir; assert two findings.
- "returns no legacy-lock findings when the tree is clean": no `.lock` files; assert zero `CategoryLegacyLockFile` findings.
- "ignores .md files and does not flag them": write a normal `.md` file; assert it is NOT flagged as a legacy lock.
Note: `Check` short-circuits with "not a dark-factory project" if `SpecsInProgressDir` does not exist — ensure the test creates that dir (mirror existing detector tests).

4.2. Add a fixer test (in `legacy_lock_file_test.go` or `fixer_test.go`) covering REMOVAL + IDEMPOTENCE:
- "removes a legacy .lock file": construct `NewFixer(FixerDeps{...})` with `FileLockFactory` set to a fake `func(string) lock.DirLock { return &mocks.LockDirLock{} }` (the cleanup fixer does not call it, but `NewFixer` defaults it anyway; passing a fake keeps the test hermetic), write a `.lock` file, build the finding (or run `Check` then `Apply`), call `Apply(ctx, findings, ApplyOptions{Yes: true})`, assert the file is gone and the result has one `Applied` and zero `Failed`.
- "is idempotent — re-running on an already-clean tree applies nothing": after removal, run `Check` + `Apply` again; assert zero findings / zero applied / zero failed. Also assert `os.Remove` on a missing file is treated as success (the `os.IsNotExist` branch) by removing the file out-of-band between detect and apply if convenient, OR by a direct unit call to `applyLegacyLockFilePath` on a non-existent path asserting an `AppliedFix` is returned.
- Use `mocks.LockDirLock` (the mock renamed in prompt 1) and `lock.DirLock` for the `FileLockFactory` field type.

4.3. Coverage for the new detector + fixer paths (audit-write-failure path optional but preferred) must keep `pkg/doctor` >= 80%.

## 5. CHANGELOG

In `/workspace/CHANGELOG.md`, add a `## Unreleased` section at the very top of the version list (above `## v0.181.0`). Per `changelog-guide.md`, prefix is required. Add these bullets:
```
## Unreleased

- feat: replace per-file `*.lock` sidecar locking with a flock on the parent status directory (spec 097); no `.lock` files are created during normal operation, crash-release is automatic via kernel flock drop
- feat: lock spec mutation commands (`spec approve`/`reject`/`complete`/`unapprove`/`mark-prompted`) on their source status directory, matching the prompt-mutation locking protocol (spec 097)
- feat: add `dark-factory doctor` `legacy-lock-file` check that removes pre-existing `*.md.lock` litter from the old locking scheme (spec 097)
```
If a `## Unreleased` section already exists by the time this runs, APPEND these bullets to it instead of creating a duplicate section.

## 6. Verify the lifecycle + grep ACs

6.1. The grep AC: the `.lock` suffix must appear in production code ONLY inside this cleanup fixer/detector:
```bash
grep -rEn '"\.lock"|\+ *"\.lock"|HasSuffix.*\.lock' /workspace/pkg/lock/ /workspace/pkg/cmd/ /workspace/pkg/queuescanner/ /workspace/pkg/doctor/ --include='*.go' --exclude='*_test.go'
```
Expected: matches ONLY in `/workspace/pkg/doctor/legacy_lock_file.go` (the `strings.HasSuffix(..., ".lock")` match). All other production files: empty.

6.2. CHANGELOG AC:
```bash
grep -n 'spec 097\|parent.*directory\|parent-dir-flock\|legacy-lock-file' /workspace/CHANGELOG.md
```
Expected: >= 1 match under `## Unreleased`.

</requirements>

<constraints>

- The cleanup is operator-invoked via `dark-factory doctor --fix` ONLY — NO startup hook, NO implicit cleanup, NO automatic removal during normal command execution (spec Desired Behavior 8: "runs only on explicit operator invocation").
- The fixer MUST be idempotent — a second run on a clean tree applies nothing; removing an already-missing file is success not failure (spec Failure Mode "Idempotent — re-running cleanup is a no-op").
- The cleanup fixer is the ONLY production code site permitted to reference the `.lock` suffix after this spec (spec AC). Do not reference `.lock` in any other production file.
- The CHANGELOG entry references spec 097 in at least one bullet (spec Constraint).
- The doctor `--yes`/confirm gate, audit logging, and exit-code semantics are unchanged — the new fixer flows through the existing `applyFinding` gate and result aggregation (no special-casing in `pkg/cmd/doctor.go`).
- Errors wrapped with `bborbe/errors` (`errors.Wrap(ctx, err, "...")` / `errors.Errorf(ctx, ...)`) — never `fmt.Errorf`, never `context.Background()` in pkg/.
- `os.Remove` and `os.ReadDir` calls: add `#nosec` with a reason only if gosec flags them; the paths are derived from operator-configured status directories, not untrusted input (per `go-security-linting.md`). Prefer fixing the finding over blanket-suppressing.
- BSD-style license header on every new/modified file.
- Coverage for `pkg/doctor` must stay >= 80%; the detector and fixer error paths must be tested.
- This prompt depends on prompts 1-3: the `mocks.LockDirLock` type (prompt 1) and the migrated callers (prompts 2-3) must already exist. If `mocks.LockDirLock` is absent, STOP and report `status: failed` with "DirLock primitive not yet deployed (prompt 1)".
- Do NOT re-introduce per-file `.lock` creation anywhere (spec Non-goal).
- Do NOT commit — dark-factory handles git.

</constraints>

<verification>

Run from the repo root inside the YOLO container:

```bash
cd /workspace && make test
```

All packages pass. Then the ACs:

```bash
cd /workspace
go test ./pkg/doctor/... -v 2>/dev/null | grep -iE 'legacy|lock' | tail -20
grep -n 'CategoryLegacyLockFile' pkg/doctor/doctor.go pkg/doctor/fixer.go pkg/doctor/legacy_lock_file.go pkg/doctor/fix_legacy_lock_file.go   # constant + detector + registration + fixer + case
grep -rEn '"\.lock"|HasSuffix.*\.lock' pkg/lock/ pkg/cmd/ pkg/queuescanner/ pkg/doctor/ --include='*.go' --exclude='*_test.go'   # ONLY legacy_lock_file.go matches
grep -n 'spec 097\|legacy-lock-file\|parent status directory' CHANGELOG.md   # >=1 under ## Unreleased
grep -n '## Unreleased' CHANGELOG.md   # exactly 1
```

Manual lifecycle smoke (optional, illustrative — the hermetic tests are authoritative): create a temp tree with a `prompts/in-progress/001-x.md.lock` file, run the detector+fixer via the test harness, confirm `find <tree> -name '*.lock'` is empty and a re-run applies nothing.

Then full precommit:

```bash
cd /workspace && make precommit
```

Exit code 0 required. On single-target failure (lint/gosec/errcheck/check-changelog), fix and re-run only that target until green, then re-run `make precommit` once. Note `make precommit` runs `check-changelog` — the `## Unreleased` entry must be present and well-formed for it to pass.

Coverage:
```bash
cd /workspace && go test -coverprofile=/tmp/cover.out ./pkg/doctor/... && go tool cover -func=/tmp/cover.out | tail -1
```
Expect >= 80%.

</verification>
