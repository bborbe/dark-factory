---
status: committing
summary: Dropped 4 noise typed primitives from pkg/processor/types.go, moved ~190 lines of filename normalization out of prompt.go into pkg/prompt/normalize.go, and injected currentDateTimeGetter once at run() in main.go threading through all 21 factory functions instead of instantiating libtime.NewCurrentDateTime() 21 times
container: dark-factory-352-architecture-factory-polish
dark-factory-version: v0.135.3-1-gf3b7a3f
created: "2026-04-25T23:45:00Z"
queued: "2026-04-25T22:09:46Z"
started: "2026-04-25T22:13:49Z"
---

<summary>
- Drop the four typed primitives in `pkg/processor/types.go` (`MaxContainers`, `VerificationGate`, `DirtyFileThreshold`, `AutoRetryLimit`) that have no methods or validation — replace with plain `int` / `bool` at the boundary
- Move ~200 lines of filename-normalization logic out of `pkg/prompt/prompt.go` into a new file `pkg/prompt/normalize.go` (same package, just file organisation)
- Inject `currentDateTimeGetter` once at `CreateRunner` / `CreateOneShotRunner` entry instead of constructing `libtime.NewCurrentDateTime()` 21 times throughout `pkg/factory/factory.go`
- Pure refactor — no behaviour change; all existing tests pass unchanged
</summary>

<objective>
Address the 4 MINOR architecture-audit findings (#5–#8) in one focused refactor: drop noise types, split a fat file by concern, and stop instantiating clocks 21 times. None of these change behaviour; they reduce ceremony.

Note: the previously-planned `ProcessorConfig` struct (audit finding #5 about 30 positional `NewProcessor` parameters) is **deferred** — prompt 350 already simplified the signature when it removed `ProjectName`, `BaseName`, `ContainerName` and added scanner injection. Re-evaluate after this prompt lands; if `NewProcessor` still has 25+ params, address in a follow-up.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Current state (post-350+351):
- `pkg/processor/types.go` has four typed primitives without methods or validation: `MaxContainers int`, `VerificationGate bool`, `DirtyFileThreshold int`, `AutoRetryLimit int`. Each is wrapped at the constructor boundary, then immediately unwrapped to its underlying primitive inside the processor.
- `pkg/prompt/prompt.go` is large; ~200 lines of filename-normalization logic (`scanPromptFiles`, `parseFilename`, `renameInvalidFiles`, `determineRename`, `performRename`, `NormalizeFilenames`) sit beside lifecycle and frontmatter logic. Different concerns.
- `pkg/factory/factory.go` calls `libtime.NewCurrentDateTime()` in 8+ places (line 296, 434, 891, 984, 1010, 1028, 1040, 1052, …). Each is harmless in production but means time can't be injected at the factory level for testing.

Files in scope (locate by symbol; line numbers churn):
- `pkg/processor/types.go` — the 4 typed primitives to drop
- `pkg/processor/processor.go` — `NewProcessor` parameter list, `processor` struct fields that use the typed primitives
- `pkg/factory/factory.go` — every constructor call that wraps/unwraps these types, plus `CreateRunner`/`CreateOneShotRunner`/per-command factories using `libtime.NewCurrentDateTime()`
- `pkg/prompt/prompt.go` — ~200 lines of filename normalization to move out
- New file: `pkg/prompt/normalize.go`
</context>

<requirements>

## 1. Drop noise typed primitives

In `pkg/processor/types.go`, delete these four types:

- `MaxContainers int`
- `VerificationGate bool`
- `DirtyFileThreshold int`
- `AutoRetryLimit int`

Update `pkg/processor/processor.go`:
- `NewProcessor` parameter list — replace each typed param with the primitive (`maxContainers int`, `verificationGate bool`, `dirtyFileThreshold int`, `autoRetryLimit int`)
- `processor` struct field types — same swap
- Document constraints in the parameter doc comments where helpful (e.g. "// maxContainers must be ≥ 1")

Update every consumer:

```bash
grep -rn "processor\.MaxContainers\|processor\.VerificationGate\|processor\.DirtyFileThreshold\|processor\.AutoRetryLimit" --include='*.go'
```

Replace each `processor.X(value)` cast with the underlying primitive at the call site (typically `pkg/factory/factory.go`).

`pkg/processor/types.go` may end up empty after this. If so, delete the file.

If any of the 4 types has external value consumers (likely none — these are pure factory-to-processor passes), update them.

## 2. Move filename normalization to `pkg/prompt/normalize.go`

Identify the filename-normalization concern in `pkg/prompt/prompt.go`. Locate all helpers by `grep -n "scanPromptFiles\|parseFilename\|renameInvalidFiles\|determineRename\|performRename\|normalizeFilenames\b" pkg/prompt/prompt.go` (after prompt 351 unexported `normalizeFilenames`).

Move the entire group of helpers into a new file:

```
pkg/prompt/normalize.go
```

Same package (`package prompt`). Cut + paste the function bodies; do NOT modify them. Keep imports up to date — add what `normalize.go` needs, remove what `prompt.go` no longer needs.

The `Manager.NormalizeFilenames` method stays in `prompt.go` (lifecycle layer); it just calls into the unexported helper that now lives in `normalize.go`.

Also relocate any related test cases. If `prompt_test.go` or `prompt_file_test.go` has a `Describe("NormalizeFilenames", ...)` block (or similar), move it to a new `pkg/prompt/normalize_test.go` (external `package prompt_test`) for symmetry. If the tests are interleaved with unrelated cases, leave them — file split is mechanical, not a test rewrite.

## 3. Inject `currentDateTimeGetter` at factory entry

`CreateRunner` and `CreateOneShotRunner` are the entry points called from `main.go`. Add `currentDateTimeGetter libtime.CurrentDateTimeGetter` as a parameter to both:

```go
// before
func CreateRunner(ctx context.Context, cfg config.Config, ver string) runner.Runner {
    currentDateTimeGetter := libtime.NewCurrentDateTime()
    // ...
}

// after
func CreateRunner(
    ctx context.Context,
    cfg config.Config,
    ver string,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
) runner.Runner {
    // pass currentDateTimeGetter into every per-command factory below
}
```

Update `main.go` (or wherever these are called) to construct `libtime.NewCurrentDateTime()` once and pass it in.

Within `factory.go`, every internal `Create*Command` helper that currently calls `libtime.NewCurrentDateTime()` accepts it as a parameter from the calling `CreateRunner`/`CreateOneShotRunner`. Net effect: 21 instantiations → 1 (in `main.go`).

Survey the call graph:

```bash
grep -n "libtime.NewCurrentDateTime" pkg/factory/factory.go
```

Each such site becomes a parameter pass-through. The clock instance threads through the entire factory.

Note: `libtime.NewCurrentDateTime()` appears in two patterns — `x := libtime.NewCurrentDateTime()` (assigned) and as an inline argument to a constructor (e.g. `someCtor(libtime.NewCurrentDateTime(), ...)`). Both patterns must be replaced with the threaded `currentDateTimeGetter` parameter.

## 4. Verify

```bash
cd /workspace
make generate
make precommit
```

Both must exit 0.

## 5. CHANGELOG

Append to `## Unreleased` in `CHANGELOG.md`:

```
- refactor: drop noise typed primitives MaxContainers/VerificationGate/DirtyFileThreshold/AutoRetryLimit (no methods, immediately unwrapped); split filename-normalization logic out of prompt.go into pkg/prompt/normalize.go; inject currentDateTimeGetter once at CreateRunner/CreateOneShotRunner entry instead of 21 separate libtime.NewCurrentDateTime() instantiations
```

</requirements>

<constraints>
- Pure refactor — no behaviour change. All existing tests must pass unchanged.
- Do NOT commit — dark-factory handles git
- Do not touch `go.mod` / `go.sum` / `vendor/`
- Search ALL `processor.NewProcessor(` call sites with `grep -rn "processor\.NewProcessor(" --include='*.go'` — every site needs the primitive type swap (recurring lesson: helpers don't always cover every direct call)
- File split (step 2) is **organisational only** — same package `prompt`, no API change, no test rewrite
- The `currentDateTimeGetter` injection (step 3) flows through factory; do not introduce a new package or public API surface
- Use `errors.Wrap` / `errors.Wrapf` from `github.com/bborbe/errors` for any new errors (unlikely needed)
- Counterfeiter mocks regenerated via `make generate` — do not hand-edit
- `pkg/processor/types.go` may become empty after step 1 — delete the file if so
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Spot checks:

```bash
cd /workspace

# Noise types gone
! grep -rn "type MaxContainers\|type VerificationGate\|type DirtyFileThreshold\|type AutoRetryLimit" pkg/processor/

# No external use of the now-deleted types
! grep -rn "processor\.MaxContainers\|processor\.VerificationGate\|processor\.DirtyFileThreshold\|processor\.AutoRetryLimit" --include='*.go'

# Normalize helpers moved
ls pkg/prompt/normalize.go
! grep -n "func scanPromptFiles\|func parseFilename\|func renameInvalidFiles\|func determineRename\|func performRename" pkg/prompt/prompt.go
grep -n "func scanPromptFiles\|func parseFilename" pkg/prompt/normalize.go

# libtime instantiations consolidated
LIBTIME_COUNT=$(grep -c "libtime.NewCurrentDateTime()" pkg/factory/factory.go)
echo "libtime.NewCurrentDateTime() count in factory.go: $LIBTIME_COUNT (expect 0)"
grep -n "currentDateTimeGetter libtime.CurrentDateTimeGetter" main.go

# All NewProcessor call sites still compile
grep -rn "processor\.NewProcessor(" --include='*.go'

make precommit
```
</verification>
