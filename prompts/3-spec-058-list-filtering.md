---
status: draft
spec: [058-reject-spec-and-prompt]
created: "2026-04-25T10:30:00Z"
---

<summary>
- `dark-factory prompt list` scans `prompts/rejected/` and hides rejected items by default; `--all` shows them
- `dark-factory spec list` hides rejected specs by default; `--all` shows them; the spec lister is extended to also scan `specs/rejected/`
- `pkg/spec/lister.go` `Summary()` counts rejected specs correctly (it already gained the `Rejected int` field from prompt 1)
- The daemon (specwatcher + processor) naturally ignores rejected files because it only scans `specs/in-progress/` and `prompts/in-progress/` — no code change needed, but this prompt documents and tests the invariant
- All existing `--all` flag handling is preserved; reject filtering stacks on top of existing completed filtering
- New Ginkgo tests cover: rejected items hidden by default, shown with --all, rejected dir not present (graceful), daemon dir invariant
</summary>

<objective>
Wire the `rejected` status into the list display layer so that rejected items are hidden by default (matching the existing treatment of `completed`) and visible with `--all`. Update the spec lister and prompt list command to scan their respective `rejected/` directories, and confirm the daemon requires no changes.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`

**Prerequisite**: This prompt depends on prompts 1 and 2 (`1-spec-058-model.md`, `2-spec-058-commands.md`) having been applied first.
The following must already exist:
- `prompt.RejectedPromptStatus`, `spec.StatusRejected`
- `cfg.Prompts.RejectedDir`, `cfg.Specs.RejectedDir` in `pkg/config/config.go`

Key files to read before editing:
- `pkg/cmd/list.go` — current `listCommand`, `scanDir`, `excludePromptStatus`, `--all` handling
- `pkg/cmd/spec_list.go` — current `specListCommand.Run`, how `spec.NewLister` is called with dirs, `--all` handling
- `pkg/spec/lister.go` — `Lister` interface, `lister` struct, `List()`, `Summary()` and `Summary` struct
- `pkg/factory/factory.go` — `CreateListCommand`, `CreateSpecListCommand` factory functions
- `pkg/cmd/list_test.go` — existing test patterns
- `pkg/cmd/spec_list_test.go` — existing test patterns
- `pkg/specwatcher/watcher.go` — only reads `inProgressDir`; no rejected dir scanning needed
- `pkg/processor/processor.go` — only reads from `InProgressDir` queue; no rejected dir scanning needed
</context>

<requirements>

## 1. Update `pkg/cmd/list.go` — prompt list command

### 1a. Add `rejectedDir` to `listCommand`

In `pkg/cmd/list.go`, add `rejectedDir string` to the `listCommand` struct and `NewListCommand` constructor.

Current constructor signature:
```go
func NewListCommand(
    inboxDir string,
    queueDir string,
    completedDir string,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
) ListCommand
```

New signature:
```go
func NewListCommand(
    inboxDir string,
    queueDir string,
    completedDir string,
    rejectedDir string,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
) ListCommand
```

### 1b. Scan rejected dir in `Run`

In `Run`, after the `completedEntries` scan block, add:

```go
rejectedEntries, err := l.scanDir(ctx, l.rejectedDir)
if err != nil {
    return errors.Wrap(ctx, err, "scan rejected")
}
entries = append(entries, rejectedEntries...)
```

### 1c. Exclude rejected by default

In the `switch` block that applies status filters, the existing default (non-`showAll`) case currently excludes `completed`:

```go
case !showAll:
    entries = excludePromptStatus(entries, string(prompt.CompletedPromptStatus))
```

Update it to also exclude `rejected`:

```go
case !showAll:
    entries = excludePromptStatus(entries, string(prompt.CompletedPromptStatus))
    entries = excludePromptStatus(entries, string(prompt.RejectedPromptStatus))
```

`--all` already shows everything (the `showAll` case has no filter applied), so no change needed there.

## 2. Update `pkg/cmd/spec_list.go` — spec list command

### 2a. Extend the lister to include `rejectedDir`

In `spec_list.go`, the `SpecListCommand` currently receives a `specpkg.Lister` constructed in the factory. The lister's dir list must include the rejected dir.

The `specListCommand.Run` already filters out `completed` by default:

```go
if !showAll && sf.Frontmatter.Status == string(specpkg.StatusCompleted) {
    continue
}
```

Update this to also filter out `rejected`:

```go
if !showAll && (sf.Frontmatter.Status == string(specpkg.StatusCompleted) ||
    sf.Frontmatter.Status == string(specpkg.StatusRejected)) {
    continue
}
```

No structural change to `SpecListCommand` is needed — the rejected specs appear in the `List()` results if the lister is given the rejected dir.

## 3. Update `pkg/factory/factory.go` — factory functions

### 3a. Update `CreateListCommand`

Find `CreateListCommand` in `pkg/factory/factory.go`. It currently passes three dirs to `cmd.NewListCommand`. Add the rejected dir as the fourth positional argument:

```go
return cmd.NewListCommand(
    cfg.Prompts.InboxDir,
    cfg.Prompts.InProgressDir,
    cfg.Prompts.CompletedDir,
    cfg.Prompts.RejectedDir,   // ADD
    currentDateTimeGetter,
)
```

### 3b. Update ALL `spec.NewLister` call sites in `pkg/factory/factory.go`

Find every `spec.NewLister(...)` call in `pkg/factory/factory.go`. There are five known sites at approximately lines **752, 979, 999, 1072, 1141**. Run this grep first to enumerate them precisely:

```bash
grep -n "spec.NewLister(" pkg/factory/factory.go
```

For **each** site, add `cfg.Specs.RejectedDir` as a new positional dir argument to the variadic `dirs ...string` parameter. The `NewLister` constructor takes a variadic dirs list, so adding the dir is mechanical:

```go
// Before
spec.NewLister(
    currentDateTimeGetter,
    cfg.Specs.InboxDir,
    cfg.Specs.InProgressDir,
    cfg.Specs.CompletedDir,
)

// After
spec.NewLister(
    currentDateTimeGetter,
    cfg.Specs.InboxDir,
    cfg.Specs.InProgressDir,
    cfg.Specs.CompletedDir,
    cfg.Specs.RejectedDir,    // ADD
)
```

All five sites must be updated — otherwise commands like processor, scenario, and status views will silently miss rejected specs in their lookups.

After updating, re-grep:

```bash
# Every spec.NewLister call should now include RejectedDir
grep -A4 "spec.NewLister(" pkg/factory/factory.go | grep -c "RejectedDir"
# Output must equal the number of NewLister sites (5)
```

### 3c. Update `CreateCombinedListCommand` and `combined_list.go`

The bare `dark-factory list` command (combined view of specs + prompts) is implemented in `pkg/cmd/combined_list.go`. Without changes here, it will still show rejected items — inconsistent with the per-entity `prompt list` and `spec list`.

#### 3c.i. `pkg/cmd/combined_list.go` — accept `rejectedDir`

Find `NewCombinedListCommand` (line ~46). Currently it builds:

```go
dirs := []string{c.inboxDir, c.queueDir, c.completedDir}
```

Extend the constructor signature to accept `rejectedDir`, store it on the struct, and add it to the `dirs` slice. Then in the `Run` filter block (the equivalent of `spec_list.go`'s `!showAll` filter), exclude both `completed` and `rejected` by default:

```go
if !showAll {
    if sf.Frontmatter.Status == string(spec.StatusCompleted) {
        continue
    }
    if sf.Frontmatter.Status == string(spec.StatusRejected) {
        continue
    }
}
```

Use typed constants — no literal `"rejected"` string comparisons (per spec 057's grep gate; verified in §10).

#### 3c.ii. `pkg/factory/factory.go` — wire the new dir

Find `CreateCombinedListCommand` (line ~1137) and pass `cfg.Specs.RejectedDir` (or whichever rejected dir the combined list aggregates — pick the spec rejected dir if combined list is spec-centric, both dirs if it aggregates both).

Update the `cmd.NewCombinedListCommand` call accordingly.

## 4. Confirm daemon requires no changes

Read `pkg/specwatcher/watcher.go` and `pkg/processor/processor.go`. Verify:

1. `specwatcher` watches `inProgressDir` (e.g., `specs/in-progress/`) — it does NOT scan `specs/rejected/`. No change needed.
2. `processor` reads from its queue dir (e.g., `prompts/in-progress/`) — it does NOT scan `prompts/rejected/`. No change needed.

Do not modify either file. The daemon's natural scoping to `in-progress/` directories means rejected items in `rejected/` directories are already ignored — this is the design intent.

Write a brief comment in the relevant test (step 5d) documenting this invariant rather than adding dead code.

## 5. Write tests

### 5a. Update `pkg/cmd/list_test.go`

Add a new `Describe("list command with rejected dir", ...)` block in `pkg/cmd/list_test.go` (external `package cmd_test`). Do NOT modify existing tests.

Test cases (use `GinkgoT().TempDir()` for all dirs; write real files):

1. **Rejected hidden by default**: create a rejected prompt file with `status: rejected` in the rejectedDir → call `Run(ctx, []string{})` → output does NOT contain the rejected filename

2. **Rejected shown with --all**: same setup → call `Run(ctx, []string{"--all"})` → output DOES contain the rejected filename

3. **Rejected dir missing is OK**: construct `listCommand` with a non-existent rejectedDir → `Run(ctx, []string{})` → no error (graceful handling via `os.IsNotExist` in `scanDir`)

4. **Non-rejected items still shown by default**: create a draft prompt in inbox → call `Run(ctx, []string{})` → output contains it

### 5b. Update `pkg/cmd/spec_list_test.go`

Add a new `Describe("spec list command with rejected", ...)` block. Do NOT modify existing tests.

Test cases:

1. **Rejected spec hidden by default**: create a spec file with `status: rejected` in the rejectedDir (passed to the lister) → `Run(ctx, []string{})` → output does NOT contain the spec filename

2. **Rejected spec shown with --all**: same setup → `Run(ctx, []string{"--all"})` → output DOES contain the spec filename

3. **Daemon invariant documented**: add a comment in the test file (not as a test case, just as a `// NOTE:` comment) stating that the daemon's `specwatcher` and `processor` only scan `in-progress/` directories and therefore naturally exclude `rejected/` items — no code change needed.

### 5c. Update existing factory call sites if needed

If any existing test directly constructs `cmd.NewListCommand` with three dirs, it will now need four. Read `pkg/cmd/list_test.go` and `pkg/cmd/combined_list_test.go` to check. Update any broken call sites to add an empty string or a temp dir as the fourth argument.

## 6. Update `pkg/spec/lister.go` `Summary()` — verify rejected is counted

Read `pkg/spec/lister.go`. The `Summary` struct should already have `Rejected int` from prompt 1. Verify the `Summary()` method's switch statement includes `case StatusRejected: s.Rejected++`. If it does not, add it now. If it does, no change needed.

## 7. Write CHANGELOG entry

Append to the existing `## Unreleased` section in `CHANGELOG.md`:

```
- feat: dark-factory prompt list and spec list hide rejected items by default; --all shows them; rejected/ dirs are scanned for display
```

## 8. Run `make test`

```bash
cd /workspace && make test
```

Must pass before proceeding to `make precommit`.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do not touch `go.mod` / `go.sum` / `vendor/`
- This prompt depends on prompts 1 and 2 being applied first
- `scanDir` already handles `os.IsNotExist` gracefully — the rejected dir not existing is a normal state for fresh repos and must not error
- Do NOT modify `pkg/specwatcher/watcher.go` or `pkg/processor/processor.go` — their natural scoping already achieves the daemon-ignores-rejected requirement
- Existing `--all` flag semantics are preserved: `--all` shows everything including completed, rejected, and all other statuses
- All existing tests must pass unchanged — do NOT modify existing test assertions, only add new Describe blocks
- External test packages (`package cmd_test`)
- Coverage ≥80% for changed packages
- **No literal `"rejected"` string comparisons in `pkg/cmd/`** — use `prompt.RejectedPromptStatus` and `spec.StatusRejected` typed constants. Verified by post-prompt grep gate in `<verification>` (matches spec 057's pattern that `! grep -rn 'Status == "' pkg/cmd/` and `! grep -rn '"rejected"' pkg/cmd/` succeed)
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Spot checks:
```bash
cd /workspace

# Prompt list excludes rejected by default
grep -n "RejectedPromptStatus\|rejectedDir\|RejectedDir" pkg/cmd/list.go

# Spec list excludes rejected by default
grep -n "StatusRejected" pkg/cmd/spec_list.go

# Combined list also excludes rejected by default
grep -n "StatusRejected\|RejectedDir" pkg/cmd/combined_list.go

# All spec.NewLister sites updated
SITE_COUNT=$(grep -c "spec.NewLister(" pkg/factory/factory.go)
LISTER_REJECTED=$(grep -A4 "spec.NewLister(" pkg/factory/factory.go | grep -c "RejectedDir")
echo "spec.NewLister sites: $SITE_COUNT  with RejectedDir: $LISTER_REJECTED  (must match)"

# No literal "rejected" string comparisons (spec 057 grep gate extended)
! grep -rn '"rejected"' pkg/cmd/

# Run tests
make test

# End-to-end smoke: create a rejected prompt file and verify list hides it
TMPDIR=$(mktemp -d)
mkdir -p "$TMPDIR/prompts" "$TMPDIR/prompts/rejected"
cat > "$TMPDIR/prompts/rejected/042-test-prompt.md" <<'EOF'
---
status: rejected
rejected: "2026-04-25T10:00:00Z"
rejected_reason: "manual smoke"
---
EOF
echo "List without --all (should not show rejected):"
cd "$TMPDIR" && dark-factory prompt list 2>/dev/null || true
echo "List with --all (should show rejected):"
cd "$TMPDIR" && dark-factory prompt list --all 2>/dev/null || true
rm -rf "$TMPDIR"
```
</verification>
