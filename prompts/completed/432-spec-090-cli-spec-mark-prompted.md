---
status: completed
spec: ["090"]
summary: Added `dark-factory spec mark-prompted <id>` CLI subcommand that transitions approved/generating specs to prompted state via the existing SetStatus API
container: dark-factory-exec-432-spec-090-cli-spec-mark-prompted
dark-factory-version: v0.171.1-3-gd94f1fa
created: "2026-05-25T20:30:00Z"
queued: "2026-05-25T20:21:28Z"
started: "2026-05-25T20:21:29Z"
completed: "2026-05-25T20:32:44Z"
---

<summary>
- Operators get a new CLI subcommand `dark-factory spec mark-prompted <id>` that completes the manual prompt-generation lifecycle.
- The subcommand transitions an approved (or generating) spec to `prompted` using the existing state-machine API — no parallel frontmatter writes, no new transition edges.
- Re-running on an already-prompted spec is a no-op exit-zero (idempotent), so the command is safe to invoke from automation.
- Specs in other states (idea/draft/verifying/completed/rejected/hold) are rejected with a clear error naming the current status.
- Help text (`dark-factory --help` and `dark-factory spec`) lists the new subcommand alongside `approve` / `complete`.
- Test coverage mirrors `spec_approve` / `spec_complete` patterns: ginkgo specs covering happy path, idempotent path, and each rejection path.
- The existing auto-generation code path and the spec state machine are NOT modified — the new subcommand reuses the existing transition path.
</summary>

<objective>
Add a `dark-factory spec mark-prompted <id-or-name>` CLI subcommand that performs the same `approved → generating → prompted` lifecycle transition the auto generator already performs, using the existing `spec.File.SetStatus` API, so the manual prompt-generation flow can finish in the same observable state as the auto flow.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-cli-guide.md` for CLI subcommand structure.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` for the ginkgo/gomega test pattern this repo uses.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md` for the factory wiring pattern.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` for `errors.Wrap` / `errors.Errorf` usage.

Files to read before making changes:
- `specs/in-progress/090-cli-spec-mark-prompted.md` — full spec, especially Acceptance Criteria and Failure Modes
- `pkg/spec/spec.go` — `Status` constants (`StatusApproved`, `StatusGenerating`, `StatusPrompted`, etc.), `specTransitions` map (DO NOT MODIFY), `SetStatus`, `Save`, `Load`
- `pkg/generator/generator.go` lines ~180-280 — the existing `markSpecGenerating` (line 180), `executeAndFinalize` (line 191), and `finalizePrompted` (line 250) functions; the new subcommand must produce byte-identical frontmatter for the same starting state
- `pkg/cmd/spec_approve.go` — reference shape for a `Spec<Verb>Command` (interface, struct, constructor, `Run(ctx, args)` method)
- `pkg/cmd/spec_complete.go` — reference for searching all three spec dirs via `FindSpecFileInDirs`
- `pkg/cmd/spec_finder.go` — `FindSpecFileInDirs(ctx, id, dirs...)` resolver used by spec subcommands
- `pkg/cmd/spec_approve_test.go` and `pkg/cmd/spec_complete_test.go` — ginkgo test patterns to mirror
- `pkg/factory/factory.go` lines ~1281-1372 — `CreateSpecApproveCommand` / `CreateSpecCompleteCommand` reference shapes
- `main.go` — `runSpecCommand` switch (~line 361), `printHelp` (~line 940), `printSpecHelp` (~line 1078)
</context>

<requirements>

1. **Create `pkg/cmd/spec_mark_prompted.go`** modelled on `pkg/cmd/spec_approve.go` and `pkg/cmd/spec_complete.go`. Required structure:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package cmd

   import (
       "context"
       "fmt"

       "github.com/bborbe/errors"
       libtime "github.com/bborbe/time"

       "github.com/bborbe/dark-factory/pkg/spec"
   )

   //counterfeiter:generate -o ../../mocks/spec-mark-prompted-command.go --fake-name SpecMarkPromptedCommand . SpecMarkPromptedCommand

   // SpecMarkPromptedCommand executes the spec mark-prompted subcommand.
   type SpecMarkPromptedCommand interface {
       Run(ctx context.Context, args []string) error
   }

   // specMarkPromptedCommand implements SpecMarkPromptedCommand.
   type specMarkPromptedCommand struct {
       inboxDir              string
       inProgressDir         string
       completedDir          string
       currentDateTimeGetter libtime.CurrentDateTimeGetter
   }

   // NewSpecMarkPromptedCommand creates a new SpecMarkPromptedCommand.
   func NewSpecMarkPromptedCommand(
       inboxDir string,
       inProgressDir string,
       completedDir string,
       currentDateTimeGetter libtime.CurrentDateTimeGetter,
   ) SpecMarkPromptedCommand {
       return &specMarkPromptedCommand{
           inboxDir:              inboxDir,
           inProgressDir:         inProgressDir,
           completedDir:          completedDir,
           currentDateTimeGetter: currentDateTimeGetter,
       }
   }
   ```

2. **Implement `Run(ctx, args)`** with this exact behavior (see spec § Desired Behavior 2-6 and § Failure Modes):

   a. If `len(args) == 0`, return `errors.Errorf(ctx, "spec identifier required")`.

   b. Resolve the spec id via `FindSpecFileInDirs(ctx, args[0], s.inboxDir, s.inProgressDir, s.completedDir)` (same dirs as `spec_complete.go` — a spec being marked prompted can be in `inProgressDir` after approval). Wrap errors with `errors.Wrap(ctx, err, "find spec file")`.

   c. Load the spec via `spec.Load(ctx, path, s.currentDateTimeGetter)`. Wrap with `"load spec"`.

   d. Read `current := spec.Status(sf.Frontmatter.Status)`. Branch:
      - If `current == spec.StatusPrompted`: print `"already prompted: <basename>\n"` to stdout and return nil. Do NOT call `Save`. (Idempotent — see spec AC: "exit 0 without modifying the file" and "stdout contains the literal string `already prompted`".)
      - If `current == spec.StatusApproved`: perform the two-step transition. Call `sf.SetStatus(string(spec.StatusGenerating))` then `sf.SetStatus(string(spec.StatusPrompted))`. This stamps both the `generating:` and `prompted:` timestamps via the existing `stampOnce` logic — matching what the auto path's `markSpecGenerating` + `finalizePrompted` produces.
      - If `current == spec.StatusGenerating`: call `sf.SetStatus(string(spec.StatusPrompted))` once.
      - Otherwise (idea/draft/verifying/completed/rejected/empty): return `errors.Errorf(ctx, "spec cannot be marked prompted from status %q (expected approved, generating, or prompted)", current)`. Do NOT call `CanTransitionTo` here — the error message must name the current status for the operator (spec AC: "stderr contains the current status").

   e. Call `sf.Save(ctx)`. Wrap with `"save spec"`.

   f. Print `"prompted: <basename>\n"` to stdout and return nil. Use `filepath.Base(path)` for the basename — add `"path/filepath"` to imports.

3. **Frontmatter byte-identity invariant**: Step 2d's `approved → generating → prompted` two-step MUST produce frontmatter byte-identical to what `pkg/generator/generator.go` writes when it calls `markSpecGenerating` followed by `finalizePrompted` on the same starting spec (modulo the timestamp values themselves, which depend on `currentDateTimeGetter`). Both code paths must call `SetStatus` exactly twice with the same arguments in the same order. Do NOT introduce a helper that calls `SetStatus("prompted")` directly when starting from `approved` — the `generating:` timestamp must be stamped.

4. **Create `pkg/cmd/spec_mark_prompted_test.go`** modelled on `pkg/cmd/spec_complete_test.go` (use the same `tempDir / inboxDir / inProgressDir / completedDir` layout — see `spec_complete_test.go` lines 20-55). Required test cases (each as a separate `It(...)` block inside `Describe("SpecMarkPromptedCommand", ...)` → `Describe("Run", ...)`):

   a. Returns error when no identifier given (assert error message contains `"spec identifier required"`).

   b. Marks an `approved` spec as `prompted`, moving status forward. Write a fixture `001-my-spec.md` with `---\nstatus: approved\napproved: "2026-01-01T00:00:00Z"\n---\n# My Spec` in `inProgressDir`. Run with `["001-my-spec.md"]`. Assert exit error is nil, file content contains `status: prompted`, contains `generating:`, contains `prompted:`.

   c. Marks a `generating` spec as `prompted`. Fixture with `status: generating` in `inProgressDir`. Assert content contains `status: prompted` and `prompted:`.

   d. Idempotent on already-prompted spec. Fixture with `status: prompted` and `prompted: "2026-01-01T00:00:00Z"`. Capture file mtime before, run, assert nil error, assert mtime UNCHANGED (use `os.Stat` before and after; compare `ModTime()`), assert original `prompted:` timestamp is unchanged (read file content; the captured timestamp string must still be present).

   e. Rejects `draft` status with non-zero error whose message contains the literal substring `draft`.

   f. Rejects `completed` status with non-zero error whose message contains the literal substring `completed`.

   g. Rejects `verifying` status with non-zero error whose message contains the literal substring `verifying`.

   h. Rejects `rejected` status with non-zero error whose message contains the literal substring `rejected`.

   i. Rejects `idea` status with non-zero error whose message contains the literal substring `idea`.

   j. Returns error when spec not found (use unknown id like `"999-nonexistent.md"`, assert error contains `"spec not found"`).

   k. Resolves by numeric prefix (write `001-my-spec.md` with `status: approved`, run with `["001"]`, assert success and `status: prompted`).

   l. Resolves by basename without `.md` (run with `["001-my-spec"]`, assert success).

   m. **Byte-identity test** vs. the generator's `SetStatus` sequence. In the test, given an `approved` fixture, perform the equivalent two `SetStatus` calls directly on a separately-loaded `spec.SpecFile` using the same `currentDateTimeGetter`, `Save` it to a sibling path, and assert the two files are byte-identical. Use a fixed-clock `currentDateTimeGetter` (see step 5) so timestamps are deterministic.

5. **Fixed-clock setup for the byte-identity test**: `github.com/bborbe/time` exposes `CurrentDateTimeGetterFunc` (a func-type adapter for the `CurrentDateTimeGetter` interface) at `time_current-datetime.go:16`. Use it to build a deterministic getter for the byte-identity test:

   ```go
   import (
       stdtime "time"
       libtime "github.com/bborbe/time"
   )

   fixed := libtime.NewDateTime(2026, stdtime.January, 1, 12, 0, 0, 0, stdtime.UTC)
   fixedClock := libtime.CurrentDateTimeGetterFunc(func() libtime.DateTime { return fixed })
   ```

   Inject `fixedClock` into BOTH the CLI command under test AND the parallel manual `SetStatus` sequence in the test, so all timestamps are identical by construction. Do NOT use `libtime.NewCurrentDateTime()` and rely on sub-second windows — that is flaky under CI load. Other tests in the file (status-mismatch, not-found, idempotent) can use `libtime.NewCurrentDateTime()` because they do not assert timestamp equality.

   If `libtime.NewDateTime` does not exist with this exact arity, check the module source at `$GOPATH/pkg/mod/github.com/bborbe/time@*/time_date-time.go` (or run `go doc github.com/bborbe/time.NewDateTime`) for the current constructor signature. Note: `libtime.ParseDateTime(ctx, "2026-01-01T12:00:00Z")` is an alternative but returns `(*DateTime, error)` — if you use it, deref the pointer and assert no error before passing to the closure. The contract is "build one fixed `libtime.DateTime`, wrap it in `CurrentDateTimeGetterFunc`"; the exact constructor call is implementation detail. This project has NO `vendor/` directory — do not grep there.

6. **Wire the factory**: in `pkg/factory/factory.go`, immediately after `CreateSpecCompleteCommand` (line ~1361-1372), add:

   ```go
   // CreateSpecMarkPromptedCommand creates a SpecMarkPromptedCommand.
   func CreateSpecMarkPromptedCommand(
       cfg config.Config,
       currentDateTimeGetter libtime.CurrentDateTimeGetter,
   ) cmd.SpecMarkPromptedCommand {
       return cmd.NewSpecMarkPromptedCommand(
           cfg.Specs.InboxDir,
           cfg.Specs.InProgressDir,
           cfg.Specs.CompletedDir,
           currentDateTimeGetter,
       )
   }
   ```

7. **Wire `main.go` dispatch**: in `runSpecCommand` (line ~361), add a new case before `default:`:

   ```go
   case "mark-prompted":
       if err := validateOneArg(ctx, args, printSpecHelp); err != nil {
           return err
       }
       return factory.CreateSpecMarkPromptedCommand(cfg, currentDateTimeGetter).Run(ctx, args)
   ```

   Place it after the `case "complete":` block.

8. **Update top-level help**: in `printHelp` (line ~940), in the spec section (lines ~960-966), add a line immediately after the `spec complete` line:

   ```
   "  spec mark-prompted <id> Mark a spec as prompted (transitions approved/generating to prompted)\n"+
   ```

   Match the existing column alignment (two leading spaces, command, padding to align descriptions). If alignment differs by a few spaces from the surrounding lines, match the column of the longest existing command in that block (`spec reject <id> --reason <text>`) by using single-space padding where needed.

9. **Update spec-only help**: in `printSpecHelp` (line ~1078-1090), add a line after the `complete <id>` line:

   ```
   "  mark-prompted <id>  Mark a spec as prompted (transitions approved/generating to prompted)\n"+
   ```

10. **Do NOT modify** `pkg/spec/spec.go`'s `specTransitions` map, `pkg/generator/generator.go`, or any auto-path code. Spec AC explicitly verifies `git diff pkg/generator/generator.go` returns no lines.

11. **Run `go generate ./...`** after creating the file so the counterfeiter mock at `mocks/spec-mark-prompted-command.go` is generated.

12. **Run `make precommit`** — must pass. Fix any lint/format issues that appear (most likely: gofumpt formatting, godoc on exported identifiers).

</requirements>

<constraints>
- The state machine in `pkg/spec/spec.go` (`specTransitions` map) MUST NOT change. The valid edges into `prompted` remain `generating → prompted`. The new subcommand performs the same two-step transition (`approved → generating → prompted`) the auto path performs, NOT a new direct edge.
- The new subcommand MUST use the existing `spec.SpecFile.SetStatus` and `spec.SpecFile.Save` APIs — no parallel frontmatter-writing code.
- The frontmatter written for a fresh `approved → prompted` transition MUST be byte-identical to what the auto path's `finalizePrompted` produces for the same starting state (same status field, same timestamp fields including the intermediate `generating:` timestamp).
- Auto-path behavior (`pkg/generator/generator.go`) MUST NOT change.
- Existing CLI behaviors (`spec approve`, `spec complete`, `spec reject`, `spec unapprove`, `spec show`, `spec list`, `spec status`) MUST continue to work unchanged.
- Do NOT add a `--force` flag — illegal transitions must be rejected with a non-zero exit.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
</constraints>

<verification>
Run `make precommit` — must pass (lint, vet, gofumpt, ginkgo).

Run `go test ./pkg/cmd/... -run SpecMarkPrompted` — all new test cases pass.

Run `go test ./pkg/spec/... ./pkg/generator/... ./pkg/cmd/...` — no existing tests regressed.

Manual sanity check inside the YOLO container (these commands assume a built `dark-factory` binary on `$PATH`; if not yet built, run `go build -o /tmp/dark-factory .` first and use `/tmp/dark-factory`):

```
# Help wiring
dark-factory spec | grep -F 'mark-prompted'
dark-factory --help | grep -F 'spec mark-prompted'
# Both must print a line.

# Round-trip on a throwaway fixture
mkdir -p /tmp/df-test/specs/in-progress
cat > /tmp/df-test/specs/in-progress/001-throwaway.md <<'EOF'
---
status: approved
approved: "2026-01-01T00:00:00Z"
---
# Throwaway
EOF
cd /tmp/df-test && dark-factory spec mark-prompted 001
grep -E '^status: prompted$' /tmp/df-test/specs/in-progress/001-throwaway.md   # one match
grep -E '^prompted:'         /tmp/df-test/specs/in-progress/001-throwaway.md   # one match
grep -E '^generating:'       /tmp/df-test/specs/in-progress/001-throwaway.md   # one match

# Idempotent re-run
dark-factory spec mark-prompted 001 | grep -F 'already prompted'
echo $?   # must be 0

# Rejected status (write a completed fixture)
cat > /tmp/df-test/specs/in-progress/002-done.md <<'EOF'
---
status: completed
---
# Done
EOF
dark-factory spec mark-prompted 002 ; echo "exit=$?"   # exit non-zero, stderr names "completed"
```

Verify auto path untouched: `git diff pkg/generator/generator.go pkg/spec/spec.go` — only changes allowed in `spec.go` are godoc/formatting if your edit accidentally touched them; the `specTransitions` map block must show no diff. The cleanest result is zero lines in `git diff` for both files.
</verification>
