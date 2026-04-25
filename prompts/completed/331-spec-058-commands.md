---
status: completed
spec: [058-reject-spec-and-prompt]
summary: Implemented prompt reject and spec reject CLI commands with --reason flag, cascade, preflight, RejectedDir config fields, FindPromptFileInDirs helper, factory wiring, main.go dispatch, Ginkgo tests, and counterfeiter mocks.
container: dark-factory-331-spec-058-commands
dark-factory-version: v0.132.0
created: "2026-04-25T10:30:00Z"
queued: "2026-04-25T10:49:09Z"
started: "2026-04-25T11:03:52Z"
completed: "2026-04-25T11:16:09Z"
---

<summary>
- `dark-factory prompt reject <name> --reason "<text>"` command is added and works end-to-end
- `dark-factory spec reject <name> --reason "<text>"` command is added, including spec-level cascade to all linked prompts
- `--reason` flag is required for both commands; missing flag produces a clear error
- Rejected prompts are stamped with `status: rejected`, `rejected: <timestamp>`, `rejected_reason: <text>` and moved to `prompts/rejected/` (directory auto-created)
- Rejected specs are stamped identically and moved to `specs/rejected/` (directory auto-created)
- Spec cascade preflight catches any linked prompt that is not rejectable (executing, completed, etc.) before mutating any file
- Attempting to reject an already-rejected or non-rejectable item produces a clear error naming the current status
- Both commands are wired into the CLI (main.go) and factory (pkg/factory/factory.go)
- New Ginkgo tests cover all rejection paths, error cases, and cascade behavior
</summary>

<objective>
Implement `prompt reject` and `spec reject` as first-class CLI commands that move work items to terminal `rejected` state with audit metadata. The spec reject command cascades to linked prompts with a two-phase (pre-flight + commit) approach. Both commands are wired into the factory and main.go following existing patterns for `unapprove` and `cancel`.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-factory-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/`

**Prerequisite**: This prompt depends on prompt 1 (`1-spec-058-model.md`) having been applied first.
The following must already exist:
- `spec.StatusRejected`, `spec.Status.IsRejectable()`, `spec.SpecFile.StampRejected(reason)`, `spec.Frontmatter.Rejected/RejectedReason`
- `prompt.RejectedPromptStatus`, `prompt.PromptStatus.IsRejectable()`, `prompt.PromptFile.StampRejected(reason)`, `prompt.Frontmatter.Rejected/RejectedReason`

Key files to read before editing:
- `pkg/cmd/unapprove.go` — pattern for `prompt unapprove` command (struct, interface, constructor, Run)
- `pkg/cmd/spec_unapprove.go` — pattern for `spec unapprove` command including linked-prompt scanning
- `pkg/cmd/prompt_finder.go` — `FindPromptFile` (single dir); need to add multi-dir variant
- `pkg/cmd/spec_finder.go` — `FindSpecFileInDirs` (multi-dir); model the prompt multi-dir finder on this
- `pkg/factory/factory.go` — `CreateUnapproveCommand`, `CreateSpecUnapproveCommand` as factory patterns
- `pkg/config/config.go` — `PromptsConfig`, `SpecsConfig` structs and defaults
- `main.go` — where prompt subcommands and spec subcommands are dispatched (look for `case "unapprove"`)
- `pkg/prompt/prompt.go` — `PromptFile.StampRejected()`, `Frontmatter.HasSpec()`, `StripNumberPrefix()`
- `pkg/spec/spec.go` — `SpecFile.StampRejected()`, `SpecFile.Name`, `SpecFile.Frontmatter`
</context>

<requirements>

## 1. Add `RejectedDir` to config

In `pkg/config/config.go`, add `RejectedDir string` to both `PromptsConfig` and `SpecsConfig`:

```go
type PromptsConfig struct {
    InboxDir      string `yaml:"inboxDir"`
    InProgressDir string `yaml:"inProgressDir"`
    CompletedDir  string `yaml:"completedDir"`
    RejectedDir   string `yaml:"rejectedDir"`  // ADD THIS
}

type SpecsConfig struct {
    InboxDir      string `yaml:"inboxDir"`
    InProgressDir string `yaml:"inProgressDir"`
    CompletedDir  string `yaml:"completedDir"`
    RejectedDir   string `yaml:"rejectedDir"`  // ADD THIS
}
```

In the `DefaultConfig()` function (or wherever defaults are set), add:

```go
// Prompts defaults
RejectedDir: "prompts/rejected",

// Specs defaults
RejectedDir: "specs/rejected",
```

Do NOT add `RejectedDir` to the validation block (it is auto-created on demand; requiring it to exist would break fresh repos).

## 2. Add `FindPromptFileInDirs` to `pkg/cmd/prompt_finder.go`

`FindPromptFile` currently searches a single directory. Add a multi-dir variant modeled exactly on `FindSpecFileInDirs`:

```go
// FindPromptFileInDirs searches dirs in order and returns the first match.
func FindPromptFileInDirs(ctx context.Context, id string, dirs ...string) (string, error) {
    // Try as a direct path first (absolute or relative with directory component)
    if filepath.IsAbs(id) || strings.ContainsRune(id, '/') {
        if _, err := os.Stat(id); err == nil {
            return id, nil
        }
    }

    for _, dir := range dirs {
        path, err := FindPromptFile(ctx, dir, id)
        if err == nil {
            return path, nil
        }
    }
    return "", errors.Errorf(ctx, "prompt not found: %s", id)
}
```

This function searches inbox first, then in-progress. The caller determines the dir order.

## 3. Create `pkg/cmd/reject.go` — `prompt reject` command

Create the file `pkg/cmd/reject.go`:

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd
```

Add:

### 3a. Interface and struct

```go
//counterfeiter:generate -o ../../mocks/reject-command.go --fake-name RejectCommand . RejectCommand

// RejectCommand executes the prompt reject subcommand.
type RejectCommand interface {
    Run(ctx context.Context, args []string) error
}

// rejectCommand implements RejectCommand.
type rejectCommand struct {
    inboxDir              string
    inProgressDir         string
    rejectedDir           string
    currentDateTimeGetter libtime.CurrentDateTimeGetter
}
```

### 3b. Constructor

```go
// NewRejectCommand creates a new RejectCommand.
func NewRejectCommand(
    inboxDir string,
    inProgressDir string,
    rejectedDir string,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
) RejectCommand {
    return &rejectCommand{
        inboxDir:              inboxDir,
        inProgressDir:         inProgressDir,
        rejectedDir:           rejectedDir,
        currentDateTimeGetter: currentDateTimeGetter,
    }
}
```

### 3c. `Run` method

```go
// Run executes the prompt reject command.
func (r *rejectCommand) Run(ctx context.Context, args []string) error {
    // Parse --reason flag (required)
    reason, remaining, err := parseReasonFlag(args)
    if err != nil {
        return err
    }
    if len(remaining) == 0 {
        return errors.Errorf(ctx, "usage: dark-factory prompt reject <file> --reason <text>")
    }
    id := remaining[0]
    return r.rejectByID(ctx, id, reason)
}
```

### 3d. Helper methods

```go
func (r *rejectCommand) rejectByID(ctx context.Context, id, reason string) error {
    path, err := FindPromptFileInDirs(ctx, id, r.inboxDir, r.inProgressDir)
    if err != nil {
        return errors.Errorf(ctx, "prompt not found: %s", id)
    }

    pf, err := prompt.Load(ctx, path, r.currentDateTimeGetter)
    if err != nil {
        return errors.Wrap(ctx, err, "load prompt")
    }

    status := prompt.PromptStatus(pf.Frontmatter.Status)
    if status == prompt.RejectedPromptStatus {
        return errors.Errorf(ctx, "%s is already rejected", filepath.Base(path))
    }
    if !status.IsRejectable() {
        return errors.Errorf(
            ctx,
            "cannot reject prompt with status %q — pre-execution states only (idea, draft, approved)",
            pf.Frontmatter.Status,
        )
    }

    pf.StampRejected(reason)
    if err := pf.Save(ctx); err != nil {
        return errors.Wrap(ctx, err, "save prompt")
    }

    if err := os.MkdirAll(r.rejectedDir, 0750); err != nil {
        return errors.Wrap(ctx, err, "create rejected dir")
    }

    // Preserve numeric prefix when moving
    dest := filepath.Join(r.rejectedDir, filepath.Base(path))
    if err := os.Rename(path, dest); err != nil {
        return errors.Wrap(ctx, err, "move prompt to rejected")
    }

    fmt.Printf("rejected: %s\n", filepath.Base(path))
    return nil
}
```

### 3e. `parseReasonFlag` helper (package-level, reused by spec reject)

```go
// parseReasonFlag extracts --reason <text> from args.
// Returns the reason string, remaining args (without --reason and its value), and an error if
// --reason is missing or has no value.
func parseReasonFlag(args []string) (string, []string, error) {
    var reason string
    var remaining []string
    for i := 0; i < len(args); i++ {
        if args[i] == "--reason" {
            if i+1 >= len(args) {
                return "", nil, fmt.Errorf("--reason requires a value")
            }
            reason = args[i+1]
            i++ // skip the value
            continue
        }
        remaining = append(remaining, args[i])
    }
    if reason == "" {
        return "", nil, fmt.Errorf("--reason is required")
    }
    return reason, remaining, nil
}
```

Note: `parseReasonFlag` returns `fmt.Errorf` (no ctx — it is a pure parsing helper, not a business operation). The caller wraps the error with context if needed, but since the flag parse errors are returned directly to the user via `Run`, this is correct per the error-wrapping guide.

## 4. Create `pkg/cmd/spec_reject.go` — `spec reject` command

Create `pkg/cmd/spec_reject.go`.

### 4a. Interface and struct

```go
//counterfeiter:generate -o ../../mocks/spec-reject-command.go --fake-name SpecRejectCommand . SpecRejectCommand

// SpecRejectCommand executes the spec reject subcommand.
type SpecRejectCommand interface {
    Run(ctx context.Context, args []string) error
}

// specRejectCommand implements SpecRejectCommand.
type specRejectCommand struct {
    specsInboxDir         string
    specsInProgressDir    string
    specsRejectedDir      string
    promptsInboxDir       string
    promptsInProgressDir  string
    promptsCompletedDir   string
    promptsRejectedDir    string
    currentDateTimeGetter libtime.CurrentDateTimeGetter
}
```

### 4b. Constructor

```go
// NewSpecRejectCommand creates a new SpecRejectCommand.
func NewSpecRejectCommand(
    specsInboxDir string,
    specsInProgressDir string,
    specsRejectedDir string,
    promptsInboxDir string,
    promptsInProgressDir string,
    promptsCompletedDir string,
    promptsRejectedDir string,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
) SpecRejectCommand {
    return &specRejectCommand{
        specsInboxDir:         specsInboxDir,
        specsInProgressDir:    specsInProgressDir,
        specsRejectedDir:      specsRejectedDir,
        promptsInboxDir:       promptsInboxDir,
        promptsInProgressDir:  promptsInProgressDir,
        promptsCompletedDir:   promptsCompletedDir,
        promptsRejectedDir:    promptsRejectedDir,
        currentDateTimeGetter: currentDateTimeGetter,
    }
}
```

### 4c. `Run` method

```go
// Run executes the spec reject command.
func (s *specRejectCommand) Run(ctx context.Context, args []string) error {
    reason, remaining, err := parseReasonFlag(args)
    if err != nil {
        return errors.Errorf(ctx, "%v", err)
    }
    if len(remaining) == 0 {
        return errors.Errorf(ctx, "usage: dark-factory spec reject <name> --reason <text>")
    }
    return s.rejectSpec(ctx, remaining[0], reason)
}
```

### 4d. `rejectSpec` method — state check, preflight, cascade, commit

```go
func (s *specRejectCommand) rejectSpec(ctx context.Context, id, reason string) error {
    // 1. Find and load the spec
    path, err := FindSpecFileInDirs(ctx, id, s.specsInboxDir, s.specsInProgressDir)
    if err != nil {
        return errors.Errorf(ctx, "spec not found: %s", id)
    }

    sf, err := spec.Load(ctx, path, s.currentDateTimeGetter)
    if err != nil {
        return errors.Wrap(ctx, err, "load spec")
    }

    // 2. State check
    status := spec.Status(sf.Frontmatter.Status)
    if status == spec.StatusRejected {
        return errors.Errorf(ctx, "%s is already rejected", filepath.Base(path))
    }
    if !status.IsRejectable() {
        return errors.Errorf(
            ctx,
            "cannot reject spec with status %q — rejectable states: idea, draft, approved, generating, prompted",
            sf.Frontmatter.Status,
        )
    }

    // 3. Cross-entity pre-flight (only needed when status == prompted, but run always to be safe)
    // Linked prompts are discovered in inbox, in-progress, and completed dirs.
    // Prompts already in rejected/ are NOT scanned (they are already done).
    linkedPaths, err := s.findLinkedPrompts(ctx, sf.Name)
    if err != nil {
        return errors.Wrap(ctx, err, "find linked prompts")
    }

    if err := s.preflight(ctx, linkedPaths); err != nil {
        return err
    }

    // 4. Cascade: reject each linked prompt
    for _, ppath := range linkedPaths {
        if err := s.rejectLinkedPrompt(ctx, ppath, reason); err != nil {
            return errors.Wrapf(ctx, err, "reject linked prompt %s", filepath.Base(ppath))
        }
    }

    // 5. Reject the spec itself
    sf.StampRejected(reason)
    if err := sf.Save(ctx); err != nil {
        return errors.Wrap(ctx, err, "save spec")
    }

    if err := os.MkdirAll(s.specsRejectedDir, 0750); err != nil {
        return errors.Wrap(ctx, err, "create specs rejected dir")
    }

    dest := filepath.Join(s.specsRejectedDir, filepath.Base(path))
    if err := os.Rename(path, dest); err != nil {
        return errors.Wrap(ctx, err, "move spec to rejected")
    }

    fmt.Printf("rejected: %s\n", filepath.Base(path))
    return nil
}
```

### 4e. `findLinkedPrompts` — scan dirs for prompts referencing this spec

```go
// findLinkedPrompts scans inbox, in-progress, and completed prompt directories
// for any prompt whose spec: frontmatter array references this spec by name.
// Prompts already in prompts/rejected/ are intentionally excluded.
func (s *specRejectCommand) findLinkedPrompts(ctx context.Context, specName string) ([]string, error) {
    var paths []string
    for _, dir := range []string{s.promptsInboxDir, s.promptsInProgressDir, s.promptsCompletedDir} {
        entries, err := os.ReadDir(dir)
        if err != nil {
            if os.IsNotExist(err) {
                continue
            }
            return nil, errors.Wrap(ctx, err, "read prompt dir")
        }
        for _, entry := range entries {
            if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
                continue
            }
            ppath := filepath.Join(dir, entry.Name())
            pf, err := prompt.Load(ctx, ppath, s.currentDateTimeGetter)
            if err != nil {
                continue
            }
            if pf.Frontmatter.HasSpec(specName) {
                paths = append(paths, ppath)
            }
        }
    }
    return paths, nil
}
```

### 4f. `preflight` — verify all linked prompts are rejectable

```go
// preflight verifies every linked prompt is in a rejectable state.
// Returns an error listing all offending prompts if any are not rejectable.
// No files are mutated by this method.
func (s *specRejectCommand) preflight(ctx context.Context, linkedPaths []string) error {
    var offenders []string
    for _, ppath := range linkedPaths {
        pf, err := prompt.Load(ctx, ppath, s.currentDateTimeGetter)
        if err != nil {
            return errors.Wrapf(ctx, err, "load prompt %s for preflight", filepath.Base(ppath))
        }
        status := prompt.PromptStatus(pf.Frontmatter.Status)
        if !status.IsRejectable() {
            offenders = append(offenders, fmt.Sprintf("%s (status: %s)", filepath.Base(ppath), pf.Frontmatter.Status))
        }
    }
    if len(offenders) > 0 {
        return errors.Errorf(
            ctx,
            "cannot reject spec: linked prompts are not in a rejectable state:\n  %s\nCancel or wait for them to complete first",
            strings.Join(offenders, "\n  "),
        )
    }
    return nil
}
```

### 4g. `rejectLinkedPrompt` — stamp and move a single linked prompt

```go
func (s *specRejectCommand) rejectLinkedPrompt(ctx context.Context, ppath, reason string) error {
    pf, err := prompt.Load(ctx, ppath, s.currentDateTimeGetter)
    if err != nil {
        return errors.Wrap(ctx, err, "load prompt")
    }

    pf.StampRejected(reason)
    if err := pf.Save(ctx); err != nil {
        return errors.Wrap(ctx, err, "save prompt")
    }

    if err := os.MkdirAll(s.promptsRejectedDir, 0750); err != nil {
        return errors.Wrap(ctx, err, "create prompts rejected dir")
    }

    dest := filepath.Join(s.promptsRejectedDir, filepath.Base(ppath))
    if err := os.Rename(ppath, dest); err != nil {
        return errors.Wrap(ctx, err, "move prompt to rejected")
    }

    fmt.Printf("  rejected prompt: %s\n", filepath.Base(ppath))
    return nil
}
```

Required imports for `pkg/cmd/spec_reject.go`: `"context"`, `"fmt"`, `"os"`, `"path/filepath"`, `"strings"`, `"github.com/bborbe/errors"`, `libtime "github.com/bborbe/time"`, `"github.com/bborbe/dark-factory/pkg/prompt"`, `"github.com/bborbe/dark-factory/pkg/spec"`.

## 5. Factory functions in `pkg/factory/factory.go`

Add two new factory functions following the `CreateUnapproveCommand` and `CreateSpecUnapproveCommand` patterns.

### 5a. `CreateRejectCommand`

```go
// CreateRejectCommand creates a RejectCommand.
func CreateRejectCommand(cfg config.Config) cmd.RejectCommand {
    return cmd.NewRejectCommand(
        cfg.Prompts.InboxDir,
        cfg.Prompts.InProgressDir,
        cfg.Prompts.RejectedDir,
        libtime.NewCurrentDateTime(),
    )
}
```

### 5b. `CreateSpecRejectCommand`

```go
// CreateSpecRejectCommand creates a SpecRejectCommand.
func CreateSpecRejectCommand(cfg config.Config) cmd.SpecRejectCommand {
    return cmd.NewSpecRejectCommand(
        cfg.Specs.InboxDir,
        cfg.Specs.InProgressDir,
        cfg.Specs.RejectedDir,
        cfg.Prompts.InboxDir,
        cfg.Prompts.InProgressDir,
        cfg.Prompts.CompletedDir,
        cfg.Prompts.RejectedDir,
        libtime.NewCurrentDateTime(),
    )
}
```

## 6. Wire into `main.go`

In `main.go`, find the prompt subcommand dispatch block (where `case "unapprove"` appears for prompts) and add:

```go
case "reject":
    return factory.CreateRejectCommand(cfg).Run(ctx, args)
```

Find the spec subcommand dispatch block (where `case "unapprove"` appears for specs) and add:

```go
case "reject":
    return factory.CreateSpecRejectCommand(cfg).Run(ctx, args)
```

Update the help text blocks that describe prompt subcommands and spec subcommands. Find the existing help strings (search for `"  prompt unapprove"` and `"  spec unapprove"`) and add reject entries in the same style:

```
  prompt reject <id> --reason <text>  Reject a prompt (move to rejected/, terminal state)
  spec reject <id> --reason <text>    Reject a spec and all linked prompts (move to rejected/, terminal state)
```

## 7. Write tests

### 7a. `pkg/cmd/reject_test.go`

Use external `package cmd_test`. Follow patterns from `pkg/cmd/unapprove_test.go`. Create a Ginkgo test suite (if `pkg/cmd/cmd_suite_test.go` already exists, do NOT create a new one).

Test cases:
1. **Missing reason flag** — `Run(ctx, []string{"some-prompt.md"})` → error contains "--reason is required"
2. **Prompt not found** — `Run(ctx, []string{"999-missing.md", "--reason", "x"})` → error
3. **Reject from draft (inbox)** — create a temp prompt file with `status: draft` in inbox dir → `Run` → file moves to rejectedDir with `status: rejected`, `rejected_reason: x`, `rejected` timestamp set; `os.Stat` on original path errors; `os.Stat` on dest path succeeds
4. **Reject from approved (in-progress)** — same but file in inProgressDir
5. **Reject already-rejected item** — file with `status: rejected` → error contains "already rejected"
6. **Reject executing prompt** — file with `status: executing` → error contains "cannot reject"
7. **Reject completed prompt** — file with `status: completed` → error contains "cannot reject"

For each test, use a `GinkgoT().TempDir()` for the directories and write real files.

### 7b. `pkg/cmd/spec_reject_test.go`

Test cases:
1. **Missing reason flag** → error contains "--reason is required"
2. **Spec not found** → error
3. **Reject spec from draft, no linked prompts** — spec with `status: draft` in specs inbox → moves to specs rejected with correct frontmatter
4. **Reject spec from approved, no linked prompts** — spec with `status: approved` in specs in-progress → moves to specs rejected
5. **Reject spec with cascade** — spec with `status: approved` in inbox + one linked prompt with `status: draft` in prompts inbox → both move to their respective rejected dirs; both have correct frontmatter fields
6. **Preflight failure** — spec with `status: approved` in inbox + one linked prompt with `status: executing` in prompts in-progress → error contains prompt filename and its status; neither file is moved
7. **Reject already-rejected spec** — spec with `status: rejected` in inbox → error contains "already rejected"
8. **Reject non-rejectable spec** — spec with `status: verifying` → error contains "cannot reject"
9. **Mid-cascade FS error** — spec with `status: approved` + 2 linked prompts (both rejectable). Make `prompts/rejected/` un-writeable (e.g., create as a regular file at the path `prompts/rejected`, blocking `os.MkdirAll`, OR `os.Chmod(rejectedDir, 0500)` after pre-flight passes). Run reject. Assert:
   - Pre-flight succeeded (validated all prompts) but commit phase failed
   - Error message names which prompt(s) succeeded and which failed (per spec line 94)
   - Partial state on disk is acceptable — operator-recoverable
   - The spec itself was NOT moved (commit ordering: spec moves last; if any prompt move fails, spec stays put)

Use `GinkgoT().TempDir()` for all directories. Write real files with correct YAML frontmatter.

For the cascade test (case 5), verify:
- The linked prompt in prompts inbox is gone (moved to prompts rejected)
- The linked prompt in prompts rejected has `status: rejected` and `rejected_reason` matching the spec's reason
- The spec in specs inbox is gone (moved to specs rejected)

## 8. Write CHANGELOG entry

Append to the existing `## Unreleased` section in `CHANGELOG.md`:

```
- feat: add dark-factory prompt reject and dark-factory spec reject commands with --reason flag, cascade, and preflight
```

## 9. Generate counterfeiter mocks

This project uses counterfeiter. The `//counterfeiter:generate` directives on the new `RejectCommand` and `SpecRejectCommand` interfaces (sections 2 and 3) require regeneration:

```bash
cd /workspace && make generate
```

This produces `mocks/reject-command.go` and `mocks/spec-reject-command.go`. If `make generate` does not exist, fall back to:

```bash
cd /workspace && go generate ./pkg/cmd/...
```

Verify the new mock files exist:

```bash
ls mocks/reject-command.go mocks/spec-reject-command.go
```

## 10. Run `make test`

```bash
cd /workspace && make test
```

Must pass before proceeding to `make precommit`.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do not touch `go.mod` / `go.sum` / `vendor/`
- This prompt depends on prompt 1 (`1-spec-058-model.md`) being applied first
- Rejected files keep their numeric prefix — do NOT strip or renumber; use `filepath.Base(path)` as dest filename
- `rejected/` directories are auto-created on first use with `os.MkdirAll(dir, 0750)` — no setup step
- Mid-cascade FS errors stop the cascade and surface a clear error (no rollback attempted; pre-flight catches the common case)
- `parseReasonFlag` returns `fmt.Errorf` (no ctx) — it's a pure arg-parsing helper; the callers use `errors.Errorf(ctx, ...)` for their own business errors
- Cascade only scans `promptsInboxDir`, `promptsInProgressDir`, `promptsCompletedDir` for linked prompts — NOT `promptsRejectedDir` (already-rejected prompts are excluded from cascade)
- `os.MkdirAll` with `0750` perms (not `0755` — follows go-security-linting guidance for non-world-readable dirs)
- All existing tests must pass unchanged
- External test packages (`package cmd_test`)
- Use `errors.Errorf` / `errors.Wrapf` from `github.com/bborbe/errors` for all errors in command methods
- `parseReasonFlag` is a package-level function in `reject.go`; `spec_reject.go` imports it from the same package (they're both in `package cmd`)
- Coverage ≥80% for new files (`pkg/cmd/reject.go`, `pkg/cmd/spec_reject.go`)
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Spot checks:
```bash
cd /workspace

# Commands exist and are wired
grep -n "reject" main.go | grep -i "case\|factory"

# Factory functions present
grep -n "CreateRejectCommand\|CreateSpecRejectCommand" pkg/factory/factory.go

# Counterfeiter annotations present
grep -n "counterfeiter:generate" pkg/cmd/reject.go pkg/cmd/spec_reject.go

# FindPromptFileInDirs added
grep -n "FindPromptFileInDirs" pkg/cmd/prompt_finder.go

# New config fields
grep -n "RejectedDir" pkg/config/config.go

# Run tests
make test
```
</verification>
