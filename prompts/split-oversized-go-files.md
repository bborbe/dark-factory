---
status: idea
created: "2026-04-25T16:25:00Z"
---

<summary>
- Identify every Go file in the repo over 2,000 lines (the smell threshold from the YOLO CLAUDE.md "File size" rule)
- For each offender, split into multiple focused files in the same package — one per concern (e.g. `processor_queue_test.go`, `processor_sweep_test.go`, `processor_recovery_test.go`)
- Generated mocks (`mocks/*.go`), protobuf, and other code-generated files are exempt
- Each output file targets < 1,500 lines (ideally 500–1,000)
- All existing tests pass unchanged after the split — no behavior changes, only relocations
- Update `CHANGELOG.md` with one entry per package split
</summary>

<objective>
Reduce reading friction and structural smell across the repo by splitting oversized Go files into focused per-concern files. Currently four test files exceed 2,000 lines, the largest at 7,372 lines, which causes Read-tool failures, slow IDE navigation, and frequent merge conflicts. After this prompt, no hand-written Go file in the repo exceeds 2,000 lines.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-architecture-patterns.md` from the coding plugin docs.
Read `go-testing-guide.md` from the coding plugin docs.

The YOLO CLAUDE.md "File size" rule (line ~34 of `/home/node/.claude/CLAUDE.md`) defines the thresholds:

> | Lines | Status | Action |
> |---|---|---|
> | < 1,500 | ✅ healthy | none |
> | 1,500–2,000 | ⚠️ borderline | watch |
> | > 2,000 | ❌ smell | split |
> | > 3,000 | ❌❌ structural problem | split mandatory |

Survey before starting:

```bash
cd /workspace
find . -path ./vendor -prune -o -path ./mocks -prune -o -name '*.go' -print 2>/dev/null \
  | xargs wc -l 2>/dev/null \
  | sort -rn \
  | head -15
```

Known offenders at the time of writing (verify with the survey — line counts may have shifted):

| File | Lines |
|------|-------|
| `pkg/processor/processor_test.go` | 7,372 |
| `pkg/prompt/prompt_test.go` | 3,067 |
| `pkg/processor/processor_internal_test.go` | 2,934 |
| `pkg/config/config_test.go` | 2,648 |

These four are the primary targets. If the survey reveals additional files > 2,000 lines (e.g. a new file shipped between drafting and execution), include them.

**Generated code is exempt:**
- Anything in `mocks/` (counterfeiter-generated)
- Anything in `vendor/`
- Files with `// Code generated` headers
- Protobuf `.pb.go` files
</context>

<requirements>

## 1. Survey

Run the find command above. Compile the list of files > 2,000 lines (excluding generated code). Verify against the known offenders. If any new file appeared, add it. If a known offender has shrunk below 2,000 lines (e.g. recent edits already split it), drop it from the target list.

## 2. Plan splits per file

For each target file, identify the natural concerns. Read the file in chunks (offset/limit) and group `Describe` / `It` blocks (test files) or top-level `func` / `type` declarations (implementation files) by concern.

Examples of concerns for `processor_test.go`:

| Concern | Suggested filename |
|---------|-------------------|
| Queue tick + processExistingQueued | `processor_queue_test.go` |
| Sweep tick + checkPromptedSpecs | `processor_sweep_test.go` |
| Committing recovery | `processor_recovery_test.go` |
| Container lifecycle / executor wiring | `processor_executor_test.go` |
| Shared setup, helpers, fakes | `processor_helpers_test.go` |

Aim: each output file 500–1,500 lines. If a single concern exceeds 1,500 lines on its own, split it further (e.g. `processor_queue_basic_test.go`, `processor_queue_blocked_test.go`).

For `prompt_test.go`, split likely along `Frontmatter` / `PromptFile` / `Manager` / `ListQueued` lines.

For `config_test.go`, split along `partialConfig` / `Validate` / `Round-trip` / `Defaults` lines.

For `processor_internal_test.go`, split similarly to `processor_test.go` but for unexported tests.

Pick filenames that match the existing file naming convention (`<pkg>_<concern>_test.go` if the package uses that, otherwise the dominant pattern).

## 3. Execute splits

For each target file:

1. Create the new files (empty initially) with the correct `package` declaration
2. Move whole `var _ = Describe(...)` blocks (test files) or whole `func` / `type` declarations (implementation files) from the source file into the appropriate target file
3. Move shared `BeforeEach` setup, helper functions, fakes into a `<pkg>_helpers_test.go` if used by multiple split files
4. Keep the original file's name even if it shrinks — its remaining content (high-level orchestration tests, top-level setup) lives there
5. Verify imports: each new file needs the imports its content uses; remove unused imports from the source

Ginkgo's `var _ = Describe(...)` registrations are package-global — order across files doesn't matter and Ginkgo discovers all of them. No `init()` games.

If a helper function is only used by one new file, move it there (don't put everything in `_helpers_test.go`).

## 4. Verify after each file

After splitting one file, run `make test` for the affected package:

```bash
cd /workspace && go test ./pkg/processor/...
```

If anything fails (compile error, missing import, missing helper), fix immediately before moving on. Do not batch all four splits then run tests at the end — debugging a multi-file failure is much harder.

## 5. Final verification

After all splits:

```bash
cd /workspace && make precommit
```

Must exit 0.

Re-run the survey to confirm no remaining file > 2,000 lines:

```bash
find . -path ./vendor -prune -o -path ./mocks -prune -o -name '*.go' -print 2>/dev/null \
  | xargs wc -l 2>/dev/null \
  | awk '$1 > 2000 { print }' \
  | sort -rn
```

The output should list zero files (empty), or only generated files if any slipped through the prune filter.

## 6. CHANGELOG entry

Append to `## Unreleased` in `CHANGELOG.md`:

```
- refactor: split oversized Go test files (`processor_test.go` 7372→<1500/file, `prompt_test.go` 3067→<1500/file, `processor_internal_test.go` 2934→<1500/file, `config_test.go` 2648→<1500/file) into per-concern files; no behavior changes, all tests pass; eliminates Read-tool token-limit friction and reduces merge-conflict surface
```

Adjust the file list to match what was actually split.

## 7. Run final verification

```bash
cd /workspace && make precommit
```

Must exit 0.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do not touch `go.mod` / `go.sum` / `vendor/`
- This is a **pure refactor** — no test additions, no test deletions, no behavior changes. Only file relocations.
- All existing tests must pass unchanged. If a test breaks after the split, the split was incorrect — fix the split, don't modify the test.
- Each new file uses the same `package` declaration as the source
- Imports are minimized per file (only what each file needs)
- Generated code is OUT OF SCOPE — `mocks/`, `vendor/`, `// Code generated` files
- After this prompt, no hand-written `*.go` file in the repo exceeds 2,000 lines (verified by the final grep)
- Use `errors.Wrap` / `errors.Wrapf` from `github.com/bborbe/errors` for any error construction (unlikely needed in a relocation refactor)
- External test packages (`package foo_test`) — keep the existing package layout per file
- The `<pkg>_test_helpers_test.go` convention follows project's existing patterns; if the project uses a different convention (e.g. `<pkg>_test_helpers.go` or `helpers_test.go`), match that
- Coverage stays unchanged (the same test code runs, just from different files)
- See `go-architecture-patterns.md` for the file-size rationale
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Spot checks:

```bash
cd /workspace

# No hand-written Go file > 2000 lines remains
find . -path ./vendor -prune -o -path ./mocks -prune -o -name '*.go' -print 2>/dev/null \
  | xargs wc -l 2>/dev/null \
  | awk '$1 > 2000 && $2 != "total" { print }' \
  | sort -rn

# All four known offenders shrunk
wc -l pkg/processor/processor_test.go pkg/prompt/prompt_test.go \
      pkg/processor/processor_internal_test.go pkg/config/config_test.go

# Each new file should be < 1500 lines
ls pkg/processor/*_test.go pkg/prompt/*_test.go pkg/config/*_test.go \
  | xargs wc -l \
  | awk '$1 > 1500 && $2 != "total" { print "still too big:", $0 }'
```
</verification>
