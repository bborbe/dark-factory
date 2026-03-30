---
status: completed
spec: ["037"]
summary: Added UpdateSpecRefs to pkg/reindex for spec cross-reference propagation, added reindexAll helper to pkg/runner/lifecycle.go, wired full reindex sequence into both runner and oneshot runner startup, and updated factory.go and all tests accordingly.
container: dark-factory-227-spec-037-spec-refs-and-wire
dark-factory-version: v0.69.0
created: "2026-03-30T17:01:00Z"
queued: "2026-03-30T17:29:26Z"
started: "2026-03-30T18:55:36Z"
completed: "2026-03-30T19:15:59Z"
branch: dark-factory/self-healing-number-conflicts
---

<summary>
- When spec files are renumbered by the reindexer, all prompt files referencing the old spec number are automatically updated
- Prompt frontmatter `spec:` array entries matching the old spec number are rewritten to the new number (e.g., `["035"]` → `["043"]`)
- Prompt filenames containing `spec-NNN` where NNN matches the old spec number are renamed to use the new number (e.g., `050-spec-035-foo.md` → `050-spec-043-foo.md`)
- The daemon watch loop (`runner.Run`) runs full reindex (spec dirs, then spec-ref propagation, then prompt dirs) before processing begins
- The oneshot runner (`oneShotRunner.Run`) also runs full reindex at startup before processing
- Reindex hooks into the existing startup sequence in `pkg/runner/lifecycle.go` alongside `normalizeFilenames`
- All spec and prompt lifecycle dirs (inbox, in-progress, completed, log) are scanned for conflicts
</summary>

<objective>
Extend `pkg/reindex` with spec cross-reference propagation and wire the full reindex sequence into both runner modes. After this prompt, dark-factory self-heals number conflicts on every startup: spec conflicts are resolved first, then prompt cross-references are updated to match renamed specs, then prompt conflicts are resolved.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `/home/node/.claude/docs/go-patterns.md` — interface→constructor→struct, counterfeiter annotations.
Read `/home/node/.claude/docs/go-testing.md` — Ginkgo/Gomega, external test packages, ≥80% coverage.
Read `pkg/reindex/reindex.go` — `Reindexer`, `FileMover`, `Rename` types created in the previous prompt (spec-037 prompt 1).
Read `pkg/prompt/prompt.go` lines 154–200 — `SpecList` type, `Frontmatter.Specs` field, `PromptFile` load/save pattern.
Read `pkg/prompt/prompt.go` lines 476–525 — `FileMover` interface, `Manager` interface, `NewManager`.
Read `pkg/runner/lifecycle.go` — existing `normalizeFilenames`, `migrateQueueDir`, `createDirectories` helper functions.
Read `pkg/runner/runner.go` — `Run` method call sequence (where to insert `reindexAll`).
Read `pkg/runner/oneshot.go` — `Run` method call sequence (where to insert `reindexAll`).
Read `pkg/factory/factory.go` lines 39–55 — `createPromptManager` and how `git.Releaser` (which implements `prompt.FileMover`) is created.
</context>

<requirements>
1. Create `pkg/reindex/specref.go` with spec cross-reference update logic:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package reindex
   ```

   Add a regexp for the `spec-NNN` filename pattern:
   ```go
   var specFilenamePatternRegexp = regexp.MustCompile(`spec-(\d{3})`)
   ```

   Add the `UpdateSpecRefs` function:
   ```go
   // UpdateSpecRefs propagates spec file renames to prompt files.
   //
   // For each spec rename (old→new), it:
   //  1. Updates the frontmatter `spec:` field in any prompt file that references the old spec number.
   //  2. Renames any prompt file whose filename contains `spec-NNN` where NNN matches the old spec number.
   //
   // promptDirs are scanned for .md files. mover is used for file renames.
   // currentDateTimeGetter is required to load prompt files.
   // Returns the list of prompt file renames performed.
   func UpdateSpecRefs(
       ctx context.Context,
       specRenames []Rename,
       promptDirs []string,
       mover FileMover,
       currentDateTimeGetter libtime.CurrentDateTimeGetter,
   ) ([]Rename, error)
   ```

   **Algorithm for `UpdateSpecRefs`:**

   a. Build `oldNumToNew map[int]int`:
      - For each rename in `specRenames`:
        - Parse old spec number: `oldNum := specnum.Parse(strings.TrimSuffix(filepath.Base(rename.OldPath), ".md"))`
        - Parse new spec number: `newNum := specnum.Parse(strings.TrimSuffix(filepath.Base(rename.NewPath), ".md"))`
        - If both >= 0: `oldNumToNew[oldNum] = newNum`
      - If `len(oldNumToNew) == 0`, return `nil, nil` (no spec renames to propagate)

   b. Collect all .md files from all `promptDirs`: same `os.ReadDir` pattern as `collectEntries` in reindex.go (skip non-existent dirs, skip dirs entries, skip non-`.md` files)

   c. For each prompt file at `path = filepath.Join(dir, entry.Name())`:

      **Frontmatter update:**
      - Load using `prompt.Load(ctx, path, currentDateTimeGetter)` → `pf, err`; skip on error (log warn)
      - If `pf == nil`, skip
      - Track whether any change was made: `changed := false`
      - For each index `i` and spec string `s` in `pf.Frontmatter.Specs`:
        - Parse spec num: `n := specnum.Parse(s)`
        - If `n >= 0`, look up `newNum, ok := oldNumToNew[n]`
        - If ok: replace `pf.Frontmatter.Specs[i]` with `fmt.Sprintf("%03d", newNum)`; `changed = true`
      - If `changed`: call `pf.Save(ctx)` — on error, return `errors.Wrapf(ctx, err, "save spec refs in %s", filepath.Base(path))`
      - Log: `slog.Info("reindex: updated spec ref in prompt", "file", filepath.Base(path), "old", oldNum, "new", newNum)`

      **Filename update:**
      - Check filename: `name := entry.Name()`
      - Find match: `match := specFilenamePatternRegexp.FindStringSubmatch(name)`
      - If match found: parse `specNum, _ := strconv.Atoi(match[1])`
      - Look up `newNum, ok := oldNumToNew[specNum]`
      - If ok: build new name by replacing `spec-NNN` with `fmt.Sprintf("spec-%03d", newNum)`:
        ```go
        newName := specFilenamePatternRegexp.ReplaceAllStringFunc(name, func(s string) string {
            inner := specFilenamePatternRegexp.FindStringSubmatch(s)
            n, _ := strconv.Atoi(inner[1])
            if nn, exists := oldNumToNew[n]; exists {
                return fmt.Sprintf("spec-%03d", nn)
            }
            return s
        })
        ```
      - If `newName != name`: build `oldPath = filepath.Join(dir, name)`, `newPath = filepath.Join(dir, newName)`
      - Call `mover.MoveFile(ctx, oldPath, newPath)` — on error return
      - Log: `slog.Info("reindex: renamed prompt file with spec ref", "from", name, "to", newName)`
      - Append `Rename{OldPath: oldPath, NewPath: newPath}` to results
      - Update `path` to `newPath` for any subsequent operations on this file

   d. Return accumulated renames

   **Import requirements for `specref.go`:**
   ```go
   import (
       "context"
       "fmt"
       "log/slog"
       "os"
       "path/filepath"
       "regexp"
       "strconv"
       "strings"

       libtime "github.com/bborbe/time"
       "github.com/bborbe/errors"

       "github.com/bborbe/dark-factory/pkg/prompt"
       "github.com/bborbe/dark-factory/pkg/specnum"
   )
   ```

2. Add `reindexAll` to `pkg/runner/lifecycle.go`:

   ```go
   // reindexAll runs the full reindex sequence:
   //  1. Reindex spec dirs (resolve cross-directory spec number conflicts)
   //  2. Update spec cross-references in prompt dirs (propagate spec renames)
   //  3. Reindex prompt dirs (resolve cross-directory prompt number conflicts)
   func reindexAll(
       ctx context.Context,
       specDirs []string,
       promptDirs []string,
       mover prompt.FileMover,
       currentDateTimeGetter libtime.CurrentDateTimeGetter,
   ) error {
       // Step 1: Reindex spec files
       specReindexer := reindex.NewReindexer(specDirs, mover)
       specRenames, err := specReindexer.Reindex(ctx)
       if err != nil {
           return errors.Wrap(ctx, err, "reindex spec files")
       }

       // Step 2: Propagate spec renames to prompt cross-references
       if len(specRenames) > 0 {
           if _, err := reindex.UpdateSpecRefs(ctx, specRenames, promptDirs, mover, currentDateTimeGetter); err != nil {
               return errors.Wrap(ctx, err, "update spec refs")
           }
       }

       // Step 3: Reindex prompt files
       promptReindexer := reindex.NewReindexer(promptDirs, mover)
       if _, err := promptReindexer.Reindex(ctx); err != nil {
           return errors.Wrap(ctx, err, "reindex prompt files")
       }

       return nil
   }
   ```

   Add required imports to `lifecycle.go`:
   ```go
   libtime "github.com/bborbe/time"
   "github.com/bborbe/dark-factory/pkg/reindex"
   ```

3. In `pkg/runner/runner.go`, add `reindexAll` call to the `runner` struct and `Run` method:

   a. Add fields to `runner` struct:
   ```go
   currentDateTimeGetter libtime.CurrentDateTimeGetter
   mover                 prompt.FileMover
   ```

   b. Add parameters to `NewRunner`:
   ```go
   currentDateTimeGetter libtime.CurrentDateTimeGetter,
   mover                 prompt.FileMover,
   ```
   Wire them into the struct.

   c. Add method to `runner`:
   ```go
   func (r *runner) reindexAll(ctx context.Context) error {
       specDirs := []string{r.specsInboxDir, r.specsInProgressDir, r.specsCompletedDir, r.specsLogDir}
       promptDirs := []string{r.inboxDir, r.inProgressDir, r.completedDir, r.logDir}
       return reindexAll(ctx, specDirs, promptDirs, r.mover, r.currentDateTimeGetter)
   }
   ```

   d. In `Run`, insert the call AFTER `r.processor.ResumeExecuting` and BEFORE `normalizeFilenames`:
   ```go
   // Reindex all spec and prompt dirs to resolve cross-directory number conflicts
   if err := r.reindexAll(ctx); err != nil {
       return errors.Wrap(ctx, err, "reindex files")
   }
   ```

4. In `pkg/runner/oneshot.go`, add `reindexAll` call to `oneShotRunner`:

   a. Add fields to `oneShotRunner` struct:
   ```go
   mover prompt.FileMover
   ```
   (`currentDateTimeGetter` already exists on `oneShotRunner`)

   b. Add parameter to `NewOneShotRunner`:
   ```go
   mover prompt.FileMover,
   ```
   Wire into struct.

   c. Add method:
   ```go
   func (r *oneShotRunner) reindexAll(ctx context.Context) error {
       specDirs := []string{r.specsInboxDir, r.specsInProgressDir, r.specsCompletedDir, r.specsLogDir}
       promptDirs := []string{r.inboxDir, r.inProgressDir, r.completedDir, r.logDir}
       return reindexAll(ctx, specDirs, promptDirs, r.mover, r.currentDateTimeGetter)
   }
   ```

   d. In `Run`, insert after `resumeOrResetExecuting` and before `normalizeFilenames`:
   ```go
   // Reindex all spec and prompt dirs to resolve cross-directory number conflicts
   if err := r.reindexAll(ctx); err != nil {
       return errors.Wrap(ctx, err, "reindex files")
   }
   ```

5. Update `pkg/factory/factory.go` to pass `mover` (the `releaser` which implements `prompt.FileMover`) to `NewRunner` and `NewOneShotRunner`:

   a. In `CreateRunner` (around line 233), add `releaser` as the last two new params:
   ```go
   return runner.NewRunner(
       inboxDir, inProgressDir, completedDir, cfg.Prompts.LogDir,
       cfg.Specs.InboxDir, cfg.Specs.InProgressDir, cfg.Specs.CompletedDir, cfg.Specs.LogDir,
       promptManager, CreateLocker("."), watcher, proc, srv, poller,
       CreateSpecWatcher(cfg, specGen, currentDateTimeGetter), projectName,
       executor.NewDockerContainerChecker(), n,
       currentDateTimeGetter,  // new
       releaser,               // new
   )
   ```

   b. In `CreateOneShotRunner` (around line 263), add `releaser` as new param:
   ```go
   return runner.NewOneShotRunner(
       inboxDir, ...,
       promptManager,
       CreateLocker("."),
       CreateProcessor(...),
       specGen,
       currentDateTimeGetter,
       executor.NewDockerContainerChecker(),
       autoApprove,
       releaser,  // new
   )
   ```

   Note: Read the actual `CreateRunner` and `CreateOneShotRunner` function bodies carefully before editing — match the exact current argument list and append the new ones at the end.

6. Update `pkg/runner/runner_test.go` and `pkg/runner/oneshot_test.go`:
   - Add the new `currentDateTimeGetter` and `mover` params to any `NewRunner` / `NewOneShotRunner` calls in tests
   - Use `libtime.NewCurrentDateTime()` for the datetime getter
   - Use a simple `&osFileMover{}` inline implementation (same pattern as the reindex tests) or the existing counterfeiter `FileMover` mock from `mocks/file-mover.go`

7. Create `pkg/reindex/specref_test.go` with tests for `UpdateSpecRefs`:

   ```go
   package reindex_test
   ```

   Use the same `osFileMover` helper from `reindex_test.go`.

   **Tests to implement:**

   - `no spec renames` — empty `specRenames` → `UpdateSpecRefs` returns nil, nil, no files changed
   - `updates frontmatter spec field` — prompt file has `spec: ["035"]`, spec file was renamed 035→043 → after call, prompt has `spec: ["043"]`
   - `updates filename with spec-NNN pattern` — prompt file named `050-spec-035-foo.md`, spec renamed 035→043 → file is renamed to `050-spec-043-foo.md`
   - `updates both frontmatter and filename` — file named `050-spec-035-foo.md` with `spec: ["035"]` → both filename and frontmatter are updated
   - `does not touch unrelated prompts` — prompt with different spec number → unchanged
   - `handles missing promptDirs gracefully` — nonexistent dir → no error
   - `multiple spec renames in one call` — two spec renames (035→043, 020→050) → all matching prompts updated

   Use real temp dirs, write .md files with frontmatter, call `UpdateSpecRefs`, verify file content and names.

   For writing test prompt files with frontmatter, use this helper:
   ```go
   func writePromptFile(path string, spec string, extraFrontmatter string) {
       content := "---\n"
       if spec != "" {
           content += fmt.Sprintf("spec: [\"%s\"]\n", spec)
       }
       content += "status: created\n"
       if extraFrontmatter != "" {
           content += extraFrontmatter + "\n"
       }
       content += "---\n\nbody\n"
       Expect(os.WriteFile(path, []byte(content), 0600)).To(Succeed())
   }
   ```

8. After all code changes, run `make generate` to regenerate any affected counterfeiter mocks:
   ```bash
   cd /workspace && make generate
   ```
</requirements>

<constraints>
- Existing prompt and spec file formats must not change (same frontmatter fields, same `NNN-slug.md` naming)
- `pkg/reindex/specref.go` MAY import `pkg/prompt` (for `prompt.Load`, `prompt.FileMover`, `prompt.SpecList`) — no circular import risk since `pkg/prompt` does not import `pkg/reindex`
- `reindexAll` runs AFTER `r.processor.ResumeExecuting` and BEFORE `normalizeFilenames` in both `runner.Run` and `oneShotRunner.Run`
- The `created` frontmatter field format is ISO datetime or ISO date
- Reindex must not modify file content beyond the frontmatter `spec:` field — only frontmatter is changed, body is preserved
- Error wrapping uses `errors.Wrapf(ctx, err, ...)` from `github.com/bborbe/errors` — never `fmt.Errorf`
- All existing tests must continue to pass
- Do NOT commit — dark-factory handles git
</constraints>

<verification>
Run `make precommit` — must pass.

Additional checks:
```bash
# Confirm specref.go exists
ls pkg/reindex/specref.go

# Confirm reindexAll is called in runner.Run
grep -n "reindexAll\|reindex" pkg/runner/runner.go

# Confirm reindexAll is called in oneShotRunner.Run
grep -n "reindexAll\|reindex" pkg/runner/oneshot.go

# Confirm reindexAll is defined in lifecycle.go
grep -n "func reindexAll" pkg/runner/lifecycle.go

# Run reindex package tests with coverage
go test -coverprofile=/tmp/cover.out -mod=vendor ./pkg/reindex/...
go tool cover -func=/tmp/cover.out | grep "total"
# Expected: ≥80%

# Run runner tests
go test -mod=vendor ./pkg/runner/...

# Run all tests
make test
```
</verification>
