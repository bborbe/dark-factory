---
status: completed
spec: [091-doctor-command]
container: dark-factory-doctor-exec-435-spec-091-add-doctor-detection-package
dark-factory-version: v0.173.0
created: "2026-06-01T22:00:00Z"
queued: "2026-06-01T22:42:15Z"
started: "2026-06-01T22:42:16Z"
completed: "2026-06-01T23:07:58Z"
---

<summary>
- New `pkg/doctor` package detects seven categories of state anomaly in spec/prompt frontmatter and filesystem layout, returning findings without ever writing
- Each category is a self-contained detector function: duplicate numeric prefixes, prompted-but-not-swept specs, stale verifying specs, orphan prompt-spec links, failed-but-merged prompts, orphan in-progress prompts, and status/directory mismatches
- Findings are typed structs (category, target paths, suggested fix command line, evidence fields) — formatting lives in the cmd layer, not the doctor
- Pure read-only package with no daemon or CLI dependencies; depends only on `pkg/spec`, `pkg/prompt`, `pkg/specnum`, `pkg/reindex`, and a small set of stdlib helpers
- A Ginkgo v2 test suite exercises every category against golden fixture projects with at least three cases per category (≥21 `Describe`/`Context`/`It` total)
</summary>

<objective>
Introduce the read-only detection layer for the `dark-factory doctor` command: a new `pkg/doctor` package that scans the spec and prompt lifecycles and returns a slice of typed findings, one per detected anomaly. The package must not perform any writes — that responsibility belongs to a follow-up prompt. The detectors must reuse the existing scanners in `pkg/spec`, `pkg/prompt`, `pkg/specnum`, and `pkg/reindex` rather than introducing fresh filesystem-walking code.
</objective>

<context>
Read `/workspace/CLAUDE.md` first for project conventions.

Read these files end-to-end before writing code:
- `/workspace/pkg/spec/spec.go` — `Status` enum, `Frontmatter` struct, `SpecFile`, `Load`, `Save`, `SetStatus`, `MarkVerifying`, lifecycle methods (around line 88 transitions table, line 135 frontmatter, line 185 Load, line 220 SetStatus, line 297 MarkVerifying)
- `/workspace/pkg/spec/lister.go` — `Lister` interface, `NewLister`, `List`, `Summary`
- `/workspace/pkg/prompt/prompt.go` — `PromptStatus` enum (line 50), `Frontmatter` struct (line 245), `HasSpec` (line 268), `AvailablePromptStatuses`
- `/workspace/pkg/reindex/reindex.go` — `Rename` struct, `FileMover` interface, `NewReindexer`, the `validPatternRegexp` (line 26: `^(\d{3})-(.+)\.md$`)
- `/workspace/pkg/reindex/specref.go` — `UpdateSpecRefs`, `specFilenamePatternRegexp` (line 22: `spec-(\d{3})`)
- `/workspace/pkg/specnum/specnum.go` — `Parse(s string) int`
- `/workspace/pkg/config/config.go` — `SpecsConfig` and `PromptsConfig` (line 52–67) for the directory fields the doctor will be configured with
- `/workspace/mocks/` — counterfeiter output directory, naming convention `<package>-<interface-kebab>.go` with `//counterfeiter:generate -o ../../mocks/<file> --fake-name <Fake> . <Interface>` on the interface declaration
- `/workspace/pkg/cmd/cmd_suite_test.go` — Ginkgo v2 + Gomega test boilerplate (package_test external test package, `time.Local = time.UTC`, `format.TruncatedDiff = false`)

Reference docs (in-container path applies — the prompt runs inside a YOLO container):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-patterns.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-mocking-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-enum-type-pattern.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-time-injection.md`
</context>

<requirements>
1. Create the package directory `/workspace/pkg/doctor/` with a new `package doctor` doc.go containing the standard BSD license header (copy the header verbatim from any existing `pkg/<x>/doc.go` such as `/workspace/pkg/reindex/doc.go`) and a one-line package comment of the form `// Package doctor detects state anomalies in spec and prompt files.`
2. Define the public API in `pkg/doctor/doctor.go`. Use the Interface → Constructor → Struct pattern. Required symbols (all names anchored to existing codebase conventions; verify with grep before inventing):
   - Type `Category` as `string` with the six constants below — anchor to the spec's detection-categories list (DB #3 through DB #8):
     - `CategoryDuplicateSpecNumbers Category = "duplicate-spec-numbers"`
     - `CategoryPromptedNotSwept Category = "prompted-but-not-swept"`
     - `CategoryVerifyingStale Category = "verifying-stale"`
     - `CategoryOrphanPromptLink Category = "orphan-prompt-link"`
     - `CategoryOrphanInProgressPrompt Category = "orphan-in-progress-prompt"`
     - `CategoryStatusDirMismatch Category = "status-dir-mismatch"`
     - Plus a parse-error sentinel `CategoryParseError Category = "parse-errors"` for files that fail YAML parse. Not produced by any of the six detectors — it is emitted by `Check` itself for per-file parse failures (see step 7). Document in a GoDoc comment on the type.
     - **NOTE: there is NO `CategoryFailedButMerged`.** Earlier drafts listed it; the category has been deferred to a follow-up spec because `prompt.Frontmatter` has no `commit` SHA field. See spec § Non-goals.
   - Type `Finding` as a struct with exported fields `Category Category`, `TargetPaths []string` (always sorted lexicographically, ascending), `SpecID string` (empty when the finding is not associated with a spec), `Detail string` (human-readable one-line description, fixed template per category), `FixCommand string` (the copy-paste `dark-factory <subcommand> <id>` line). Add a GoDoc comment.
   - Interface `Checker` with one method `Check(ctx context.Context) ([]Finding, error)`. Counterfeiter annotation immediately above the interface declaration: `//counterfeiter:generate -o ../../mocks/doctor-checker.go --fake-name DoctorChecker . Checker`.
   - Constructor `NewChecker(deps Deps) Checker` and private struct `checker` holding deps. No business logic in the constructor.
   - Type `Deps` as a struct exporting: `SpecsInboxDir string`, `SpecsInProgressDir string`, `SpecsCompletedDir string`, `SpecsRejectedDir string`, `PromptsInboxDir string`, `PromptsInProgressDir string`, `PromptsCompletedDir string`, `PromptsCancelledDir string`, `SpecLister spec.Lister`, `PromptManager *prompt.Manager`, `CurrentDateTimeGetter libtime.CurrentDateTimeGetter`, `VerifyingStaleHours int` (zero treated as default 24). Add a GoDoc comment. **Do NOT add `SpecsIdeasDir`** — that directory does not exist in `pkg/config/config.go:SpecsConfig` (verified: the struct has only `InboxDir`, `InProgressDir`, `CompletedDir`, `RejectedDir`, `LogDir`).
3. Split detection into one file per category, all under `pkg/doctor/`, each implementing a private `detect*(ctx, deps) ([]Finding, error)` method on `*checker`. The `Check` method invokes all six in order and concatenates, then appends parse-error findings (see step 7). Each detector file contains ONLY its detector plus any small helpers it needs — no shared util sprawl. Required files:
   - `pkg/doctor/duplicate_spec_numbers.go`
   - `pkg/doctor/prompted_not_swept.go`
   - `pkg/doctor/verifying_stale.go`
   - `pkg/doctor/orphan_prompt_link.go`
   - `pkg/doctor/orphan_in_progress_prompt.go`
   - `pkg/doctor/status_dir_mismatch.go`
   - **Do NOT create `pkg/doctor/failed_but_merged.go`.** Deferred per spec § Non-goals.
4. For each detector, produce a `Finding` whose `FixCommand` matches the spec verbatim. Required strings (anchored to spec sections 3–9):
   - duplicate-spec-numbers: `dark-factory spec renumber <id-to-move>` where `<id-to-move>` is the filename without `.md` of the LATER (by filename lex order) colliding file. The other colliding files are reported as `TargetPaths`. `Detail` must mention all colliding paths and each one's status + linked-prompts count. Linked-prompt counts come from `prompt.NewCounter(deps.CurrentDateTimeGetter, deps.PromptsInboxDir, deps.PromptsInProgressDir, deps.PromptsCompletedDir).CountBySpec(ctx, id)` — use the existing `pkg/prompt` Counter, do not re-implement counting.
   - prompted-but-not-swept: `dark-factory spec sweep <spec-id>`. The detector iterates `specsInboxDir ∪ specsInProgressDir ∪ specsCompletedDir` and fires for each spec with `Status == "prompted"` whose linked prompts are ALL in a terminal state (i.e. zero prompts have status ∈ {idea, draft, approved, executing, failed, in_review, pending_verification, committing}). For "all linked prompts", reuse `spec.NewAutoCompleter(...).allLinkedPromptsCompleted` indirectly by calling `spec.Load` and scanning the prompt directories the same way (or extract a small helper — your choice, but no new file-walking). The first linked-prompt count comes from the same Counter used by category 1.
   - verifying-stale: `dark-factory spec verify <spec-id>`. Fire when `Status == "verifying"` AND `Verifying` is empty OR more than `verifyingStaleHours` (default 24) before now. Parse `Verifying` as RFC3339 with `time.Parse(time.RFC3339, fm.Verifying)`. On parse failure, fire with `Detail` mentioning "Verifying timestamp unparseable".
   - orphan-prompt-link: `dark-factory prompt unlink <prompt-id>` (primary fix-line shown in CLI output). Emit **ONE** finding per missing link with the relink alternative `dark-factory prompt relink <prompt-id> <new-spec-id>` provided as a secondary line in the `Detail` field — NOT as a second `Finding`. `TargetPaths` is `[promptFilePath]`. The detector iterates all `.md` files in `prompts/InboxDir ∪ Prompts/InProgressDir ∪ Prompts/CompletedDir ∪ Prompts/CancelledDir`, loads each via `deps.PromptManager.Load`, and for each `Frontmatter.Specs` entry checks that a spec file with that numeric prefix exists in any of the four spec directories. Use `specnum.Parse` (do not write a new regex).
   - orphan-in-progress-prompt: `dark-factory prompt cancel <prompt-id>` (the safe, reversible default — `complete` was considered but is unsafe post-move). Emit **ONE** finding per orphan. Fire when a `.md` file exists in `PromptsInProgressDir` AND the prompt's `Frontmatter.Specs` references a spec that resolves to a file in `SpecsCompletedDir` or `SpecsRejectedDir`. The resolution uses `specnum.Parse` to compare numeric prefixes; if any of the prompt's linked spec numbers resolve to a completed/rejected spec, fire.
   - status-dir-mismatch: `dark-factory spec move <spec-id>` (and the prompt equivalent `dark-factory prompt move <prompt-id>` if a prompt directory mismatch is detected). The spec's own mismatch table:
     - specs/in-progress/ must contain ONLY specs with status ∈ {idea, draft, approved, generating, prompted, verifying}
     - specs/completed/ must contain ONLY specs with status `completed`
     - specs/rejected/ must contain ONLY specs with status `rejected`
     - prompts/in-progress/ must contain ONLY prompts with status ∈ {idea, draft, approved, executing, failed, in_review, pending_verification, committing}
     - prompts/completed/ must contain ONLY prompts with status `completed` or `rejected` (rejected is allowed in completed for legacy compatibility — confirm by grep of existing files in `prompts/completed/`)
     - prompts/cancelled/ must contain ONLY prompts with status `cancelled`
   Each mismatch produces one finding with the actual status, actual directory, and the expected directory. Use `FixCommand` for the spec variants; for prompt directory mismatches use `dark-factory prompt move <prompt-id>`.
5. (Deferred — `failed-but-merged` detector removed from this prompt per spec § Non-goals. No file, no detector, no helper, no test.)
6. Sort `TargetPaths` lexicographically ascending in every finding. Sort findings within each category the same way. Do NOT sort findings across categories — preserve category order so output is stable.
7. Parse-error detection requires a **thin per-file parse scanner** in `pkg/doctor/` — this is the **one exception** to the "do not introduce new file-walking code" constraint. Rationale: `pkg/spec/spec.go:Load` (lines 200–208, verified) silently swallows YAML parse errors and returns an empty-Frontmatter `*SpecFile{}` with `nil` error; `pkg/spec/lister.go:List` (lines 50–74) therefore cannot surface parse failures. The constraint "use `spec.Lister`, don't re-walk" applies to the SIX detectors above; for parse-error detection, add a thin helper `scanParseErrors(ctx, dirs []string) []Finding` in a new file `pkg/doctor/parse_errors.go` that:
   - Walks the spec lifecycle dirs (`os.ReadDir` is allowed in THIS file only) and prompt lifecycle dirs
   - For each `.md` file, calls `frontmatter.Parse(content)` directly (the same parser `spec.Load`/`prompt.load` use internally — find the exported parser via `grep -rn "frontmatter.Parse" pkg/`)
   - Emits a `Finding` with `Category == CategoryParseError`, `TargetPaths == [filePath]`, `Detail` containing the wrapped error message, `FixCommand` set to the literal `Fix the YAML by hand, then re-run \`dark-factory doctor\``
   - Continues scanning on per-file parse failure
   - Returns `(nil, error)` only if a directory read itself fails AND `!os.IsNotExist(err)` — missing directories are project-not-initialized signals handled by the cmd layer (prompt 2), not parse errors.
   - The `<verification>` block's `grep -rn 'os.ReadDir|filepath.Walk' pkg/doctor/` check is updated to allow ONLY `parse_errors.go` to match.
8. For the duplicate-spec-numbers detector, re-use `reindex.NewReindexer` and call its public API. But `reindex.NewReindexer` performs mutations via `FileMover` — the doctor must NOT mutate. Approach: introduce a small private helper in `pkg/reindex` (NOT a new exported symbol) that scans a directory and groups files by numeric prefix, returning `map[int][]string` (number → list of file basenames). The doctor's duplicate detector calls this helper. The helper lives in `pkg/reindex/specref.go` as `scanByNumberPrefix(ctx, dirs []string) (map[int][]string, error)` (unexported). Existing tests in `pkg/reindex` MUST still pass.
9. (See step 7 — parse-error scanner lives in `pkg/doctor/parse_errors.go` and is the **only** file in this package permitted to call `os.ReadDir`. `Check` invokes it last so parse-error findings come after the six category findings in the output stream.)
10. Add counterfeiter annotations on the new public interfaces and struct literals that need mocks for downstream testing. Required: only the `Checker` interface per step 2; do NOT introduce additional public interfaces in this prompt (the `prompt.Manager` and `spec.Lister` already have their own interfaces and counterfeiter mocks at `mocks/spec-lister.go`, `mocks/prompt-counter.go`, etc. — reuse them).
11. Generate mocks: run `cd /workspace && go generate -mod=mod ./pkg/doctor/...`. The output `mocks/doctor-checker.go` MUST appear. Do NOT hand-write the mock file.
12. Create `pkg/doctor/doctor_suite_test.go` modeled on `pkg/cmd/cmd_suite_test.go`:
   - `package doctor_test` (external test package)
   - The standard header (`//go:generate go run -mod=mod github.com/maxbrunsfeld/counterfeiter/v6 -generate`)
   - Standard `time.Local = time.UTC` and `format.TruncatedDiff = false`
   - `RunSpecs(t, "Doctor Suite", ...)` with `suiteConfig.Timeout = 60 * time.Second`
13. Create per-category Ginkgo test files in `pkg/doctor/`, each in the `doctor_test` external package. Use `os.MkdirTemp("", "doctor-<category>-*")` for golden fixture projects. Each file has at least three `It` blocks per spec AC line (≥18 `Describe`/`Context`/`It` blocks total across the SIX detector files, +3 for `parse_errors_test.go`). Each `It` block:
   - Builds a minimal fixture directory tree under the temp dir
   - Constructs a `doctor.Deps` with the temp dirs as the lifecycle directories
   - Invokes `doctor.NewChecker(deps).Check(ctx)`
   - Asserts the expected finding is present (by `Category`, by `TargetPaths` membership, by `FixCommand` substring)
   - Asserts the fixture tree is unchanged (snapshot file count + key file contents before and after — proves read-only)
   - Cleans up with `defer os.RemoveAll(tempDir)`
14. Cover the following explicit cases (one `It` each, on top of the 21-category minimum):
   - `Check` on a project with zero anomalies returns `([]Finding{}, nil)` (NOT `nil` slice — empty slice).
   - `Check` on a project with `specs/in-progress/` missing returns `error` whose message contains `not a dark-factory project: missing specs/in-progress/` (this is the uninitialized-project failure mode from the spec's Failure Modes table).
   - `Check` on a project where one spec file has invalid YAML frontmatter emits ONE `CategoryParseError` finding for that file and continues scanning (returns no error).
15. Run `cd /workspace && make precommit` after implementing. The build must pass and all new tests must run.
</requirements>

<constraints>
- Do NOT commit. dark-factory handles git.
- Do NOT touch `pkg/runner/lifecycle.go` or the silent reconciliation path — that is the scope of prompt 3.
- Do NOT add the `doctor` CLI subcommand — that is prompt 2. This prompt is the pure detection library.
- Do NOT introduce new file-walking code outside `pkg/reindex`; reuse `spec.Lister`, `prompt.Manager`, and `reindex.scanByNumberPrefix`.
- Do NOT mutate the prompt or spec `Frontmatter` structs (no `PreviousID`, no `CommitSHA` fields). The data-source gap for `failed-but-merged` is acknowledged with a GoDoc comment only.
- All error wrapping via `github.com/bborbe/errors` (`errors.Wrap(ctx, err, "...")` and `errors.Errorf(ctx, "...")`). Never `fmt.Errorf`. Never `context.Background()` in pkg/ code.
- Counterfeiter mocks for new public interfaces only. Reuse existing mocks for everything else.
- External test packages (`package doctor_test`), Ginkgo v2 + Gomega, ≥80% statement coverage on the new package.
- File-mode `0600` for any test-fixture frontmatter writes; `0750` for any directories the test creates. Existing project conventions.
- Use `libtime.CurrentDateTimeGetter` for time injection — do NOT call `time.Now()` directly in detection code.
</constraints>

<verification>
- `cd /workspace && make precommit` exits 0.
- `cd /workspace && go test -count=1 -coverprofile=/tmp/doctor.cover.out ./pkg/doctor/...` exits 0 and `go tool cover -func=/tmp/doctor.cover.out | tail -1` reports ≥80.0%.
- `ls /workspace/pkg/doctor/*.go` lists at least 9 source files: `doctor.go`, `duplicate_spec_numbers.go`, `prompted_not_swept.go`, `verifying_stale.go`, `orphan_prompt_link.go`, `orphan_in_progress_prompt.go`, `status_dir_mismatch.go`, `parse_errors.go`, plus the suite file. **No `failed_but_merged.go`** — `ls pkg/doctor/failed_but_merged.go` MUST return "No such file" (`grep -l 'failed-but-merged\|CategoryFailedButMerged' pkg/doctor/*.go` returns no matches).
- `ls /workspace/mocks/doctor-checker.go` exists (counterfeiter generated).
- `grep -c 'Describe\|Context\|It' /workspace/pkg/doctor/*_test.go` returns ≥ 18.
- `cd /workspace && grep -rn 'os.ReadDir\|filepath.Walk' /workspace/pkg/doctor/*.go` returns lines ONLY in `pkg/doctor/parse_errors.go` (the carve-out per step 7); zero matches in any other file.
- `cd /workspace && git diff --name-only HEAD` shows ONLY files under `pkg/doctor/`, `mocks/`, and any `pkg/reindex/specref.go` touch (the unexported helper). No changes to `pkg/runner/`, `pkg/cmd/`, or `main.go`.
</verification>
