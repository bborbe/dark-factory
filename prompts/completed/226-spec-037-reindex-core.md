---
status: completed
spec: ["037"]
summary: Created pkg/reindex package with Reindexer interface and implementation that detects and resolves duplicate numeric prefixes across lifecycle directories using created frontmatter date as tie-breaker, plus counterfeiter mocks and 92.2% test coverage
container: dark-factory-226-spec-037-reindex-core
dark-factory-version: v0.69.0
created: "2026-03-30T17:00:00Z"
queued: "2026-03-30T17:29:26Z"
started: "2026-03-30T18:31:15Z"
completed: "2026-03-30T18:55:33Z"
branch: dark-factory/self-healing-number-conflicts
---

<summary>
- New `pkg/reindex` package detects files sharing the same numeric prefix across all lifecycle directories (inbox, in-progress, completed, log) for a single file type (prompts or specs)
- Cross-directory conflicts are detected: a file numbered 220 in `completed/` and another 220 in `in-progress/` are a conflict
- Conflicts are resolved deterministically: the file with the earliest `created` frontmatter date keeps its number; alphabetical filename breaks ties when dates are equal or missing
- Losing files are renamed to the next available gap-filling number (same `findNextAvailableNumber` approach as existing prompt normalization)
- Each rename is logged at Info level with old and new filename
- Running reindex when no duplicates exist produces no changes (idempotent)
- New `Reindexer` interface and `NewReindexer` constructor follow project interface→constructor→struct pattern
- Tests use real temp-dir file operations; test suite reaches ≥80% statement coverage
</summary>

<objective>
Create `pkg/reindex` — a new package that detects and resolves duplicate numeric prefixes across multiple lifecycle directories. The package is type-agnostic (handles both prompt files and spec files) and uses `created` frontmatter date as the primary tie-breaker, falling back to alphabetical filename order.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `/home/node/.claude/docs/go-patterns.md` — interface→constructor→struct, counterfeiter annotations, error wrapping.
Read `/home/node/.claude/docs/go-testing.md` — Ginkgo/Gomega, suite setup, external test packages, ≥80% coverage.
Read `pkg/specnum/specnum.go` — `specnum.Parse(s string) int` which this package uses to extract numeric prefixes.
Read `pkg/prompt/prompt.go` lines 1042–1160 — `scanPromptFiles`, `findNextAvailableNumber`, `renameInvalidFiles` for the numbering logic pattern to follow.
Read `pkg/spec/spec.go` lines 1–30 — to understand how `github.com/adrg/frontmatter` is used to parse frontmatter.
</context>

<requirements>
1. Create `pkg/reindex/doc.go`:
   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   // Package reindex detects and resolves duplicate numeric prefixes across
   // multiple lifecycle directories for prompt and spec files.
   package reindex
   ```

2. Create `pkg/reindex/reindex.go` with the following types and implementation:

   **Types:**

   ```go
   // Rename represents a file rename performed by the reindexer.
   type Rename struct {
       OldPath string
       NewPath string
   }

   //counterfeiter:generate -o ../../mocks/reindex-file-mover.go --fake-name ReindexFileMover . FileMover

   // FileMover handles file move operations with git awareness.
   type FileMover interface {
       MoveFile(ctx context.Context, oldPath string, newPath string) error
   }

   //counterfeiter:generate -o ../../mocks/reindexer.go --fake-name Reindexer . Reindexer

   // Reindexer detects and resolves duplicate numeric prefixes across multiple directories.
   type Reindexer interface {
       Reindex(ctx context.Context) ([]Rename, error)
   }
   ```

   **Constructor:**

   ```go
   // NewReindexer creates a Reindexer that scans the given dirs for NNN-slug.md files
   // with duplicate numeric prefixes and renames conflicting files.
   func NewReindexer(dirs []string, mover FileMover) Reindexer {
       return &reindexer{dirs: dirs, mover: mover}
   }

   type reindexer struct {
       dirs  []string
       mover FileMover
   }
   ```

   **fileEntry type (unexported):**

   ```go
   type fileEntry struct {
       dir     string  // the directory containing the file
       name    string  // filename only (e.g. "035-foo.md")
       number  int     // numeric prefix (e.g. 35), or -1 if absent
       slug    string  // slug part without number and extension (e.g. "foo")
       created string  // raw "created" value from frontmatter, or ""
   }
   ```

   **Algorithm — `Reindex` method:**

   ```go
   func (r *reindexer) Reindex(ctx context.Context) ([]Rename, error) {
       // Step 1: Collect all .md files from all dirs
       // Step 2: Collect all currently-used numbers (for gap-filling later)
       // Step 3: Group entries by number; only entries matching ^\d{3}-.+\.md$ are candidates
       // Step 4: For each group with >1 entry:
       //   - Read created from frontmatter for each entry
       //   - Sort: earliest created first, then alphabetical name
       //   - First entry keeps its number; remaining entries need renaming
       // Step 5: For each loser: find next available number, rename, log, add to usedNumbers
       // Return accumulated renames
   }
   ```

   **Detailed steps:**

   a. `collectEntries(dirs []string) ([]fileEntry, error)`:
      - For each dir in dirs, call `os.ReadDir`; skip if dir does not exist (`os.IsNotExist`)
      - For each entry: skip dirs, skip non-`.md` files
      - Parse number from `strings.TrimSuffix(entry.Name(), ".md")` using `specnum.Parse`
      - Parse slug: if name matches `^\d{3}-(.+)\.md$`, slug = capture group; else slug = stem
      - Do NOT read `created` yet (deferred to when needed in step 4)
      - Append `fileEntry{dir, name, number, slug, ""}` to results
      - Return all entries

   b. Collect all used numbers: iterate all entries; if `validPattern.MatchString(entry.name)` (i.e., `^\d{3}-.+\.md$`) then `usedNumbers[entry.number] = true`

   c. Group entries by number: `groups := make(map[int][]fileEntry)` — only include entries where `validPattern.MatchString(entry.name)` is true

   d. For each group in `groups` where `len(group) > 1`:
      - Read `created` from frontmatter for each entry using the helper `readCreated(path string) string`
        - `readCreated` opens the file, calls `frontmatter.Parse` with a `struct{ Created string \`yaml:"created"\` }`, returns the Created value or `""` on error
      - Sort the group slice:
        - Parse time with `parseCreated(s string) (time.Time, bool)` — tries `time.RFC3339` then `"2006-01-02"`; returns `(t, true)` if parseable, `(time.Time{}, false)` if not
        - Sorting rule: if both have valid times → earliest first; if both valid and equal → alphabetical name; if i has valid time and j does not → i before j; if j has valid time and i does not → j before i; if both missing → alphabetical name
      - First entry in sorted group: keeper (no rename)
      - Remaining entries: losers (need renaming)

   e. For each loser:
      - Find next available: `newNum := findNextAvailableNumber(usedNumbers)` (same logic as `prompt.findNextAvailableNumber`: iterate from 1 upward until `!usedNumbers[n]`)
      - `usedNumbers[newNum] = true` (claim it)
      - New name: `fmt.Sprintf("%03d-%s.md", newNum, loser.slug)`
      - Old path: `filepath.Join(loser.dir, loser.name)`
      - New path: `filepath.Join(loser.dir, newName)`
      - Call `r.mover.MoveFile(ctx, oldPath, newPath)` — if it fails, return immediately with `errors.Wrapf(ctx, err, "reindex rename %s → %s", loser.name, newName)`
      - Log: `slog.Info("reindex: renamed file", "from", loser.name, "to", newName)`
      - Append `Rename{OldPath: oldPath, NewPath: newPath}` to results

   **Helper: `validPatternRegexp`:**
   ```go
   var validPatternRegexp = regexp.MustCompile(`^(\d{3})-(.+)\.md$`)
   ```
   Use this to check valid format AND extract slug in one shot.

   **Helper: `findNextAvailableNumber`:**
   ```go
   func findNextAvailableNumber(usedNumbers map[int]bool) int {
       for i := 1; ; i++ {
           if !usedNumbers[i] {
               return i
           }
       }
   }
   ```

   **Helper: `readCreated(path string) string`:**
   ```go
   func readCreated(path string) string {
       data, err := os.ReadFile(path)
       if err != nil {
           return ""
       }
       var fm struct {
           Created string `yaml:"created"`
       }
       _, _ = frontmatter.Parse(bytes.NewReader(data), &fm)
       return fm.Created
   }
   ```

   **Helper: `parseCreated(s string) (time.Time, bool)`:**
   ```go
   func parseCreated(s string) (time.Time, bool) {
       if s == "" {
           return time.Time{}, false
       }
       if t, err := time.Parse(time.RFC3339, s); err == nil {
           return t, true
       }
       if t, err := time.Parse("2006-01-02", s); err == nil {
           return t, true
       }
       return time.Time{}, false
   }
   ```

3. Create `pkg/reindex/reindex_suite_test.go` (Ginkgo suite):
   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package reindex_test

   import (
       "testing"

       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"
   )

   func TestReindex(t *testing.T) {
       RegisterFailHandler(Fail)
       RunSpecs(t, "Reindex Suite")
   }
   ```

4. Create `pkg/reindex/reindex_test.go` with the following test cases. Use real temp dirs and `os.Rename`-based `FileMover`. Do NOT use counterfeiter mocks for FileMover in this file (the package doesn't exist yet); use an inline implementation:

   ```go
   type osFileMover struct{}
   func (m *osFileMover) MoveFile(_ context.Context, oldPath, newPath string) error {
       return os.Rename(oldPath, newPath)
   }
   ```

   Helper to write a .md file with optional `created` field:
   ```go
   func writeFile(t GinkgoTInterface, path string, created string) {
       content := "---\n"
       if created != "" {
           content += fmt.Sprintf("created: %q\n", created)
       }
       content += "---\n\nbody\n"
       Expect(os.WriteFile(path, []byte(content), 0600)).To(Succeed())
   }
   ```

   **Tests to implement:**

   - `no duplicates` — two files with different numbers in one dir → `Reindex` returns empty slice, files unchanged
   - `duplicate in same dir, earlier created keeps number` — two files `035-foo.md` and `035-bar.md` in same dir with different `created` timestamps → the one with earlier `created` keeps `035`, the other gets a new number
   - `duplicate across dirs, earlier created keeps number` — `035-foo.md` in `dir1` (created `2026-01-01`) and `035-bar.md` in `dir2` (created `2026-02-01`) → `dir1/035-foo.md` unchanged, `dir2/035-bar.md` renamed to next available
   - `alphabetical tiebreak when created dates are equal` — two files with same `created` timestamp → file with earlier alphabetical name keeps the number
   - `alphabetical tiebreak when created is missing` — two files without `created` field → earlier alphabetical name keeps the number
   - `one has created, one does not` — file with valid `created` keeps the number over file without `created`
   - `three-way conflict` — three files with same number → oldest keeps number, next two get sequential new numbers; result is deterministic
   - `idempotent` — run Reindex twice on same dirs → second call returns empty slice
   - `no-op when dirs do not exist` — Reindex with nonexistent dirs returns empty slice without error
   - `gap-filling` — when numbers 1–5 are used and a conflict assigns a new number, it uses 6 (next available)
   - `rename error is returned` — FileMover returns error → Reindex propagates the error

5. After writing the files, run `make generate` to create the counterfeiter mocks:
   ```bash
   cd /workspace && make generate
   ```
   Verify `mocks/reindexer.go` and `mocks/reindex-file-mover.go` are created.
</requirements>

<constraints>
- Existing prompt and spec file formats must not change
- `pkg/reindex` must NOT import `pkg/prompt`, `pkg/spec`, or `pkg/runner` — only `pkg/specnum` and standard/vendor packages
- New numbers assigned during conflict resolution use the same gap-filling approach as `prompt.findNextAvailableNumber` (iterate from 1, find first gap)
- Specs and prompts both use 3-digit zero-padded numbers; filenames follow `NNN-slug.md` convention
- Spec number extraction uses `specnum.Parse` (no new number-parsing paths)
- All existing tests must continue to pass
- The `created` frontmatter field format is ISO datetime (`2006-01-02T15:04:05Z`) or ISO date (`2006-01-02`)
- Files without a valid 3-digit prefix (`^\d{3}-.+\.md$`) are ignored by the reindexer (handled by NormalizeFilenames separately)
- Do NOT commit — dark-factory handles git
- Error wrapping must use `errors.Wrapf(ctx, err, ...)` from `github.com/bborbe/errors` — never `fmt.Errorf`
</constraints>

<verification>
Run `make precommit` — must pass.

Additional checks:
```bash
# Confirm new package exists
ls pkg/reindex/reindex.go pkg/reindex/doc.go

# Confirm no imports of pkg/prompt or pkg/spec from pkg/reindex
grep -n "dark-factory/pkg/prompt\|dark-factory/pkg/spec\|dark-factory/pkg/runner" pkg/reindex/reindex.go
# Expected: no output

# Confirm counterfeiter mocks generated
ls mocks/reindexer.go mocks/reindex-file-mover.go

# Run package tests with coverage
go test -coverprofile=/tmp/cover.out -mod=vendor ./pkg/reindex/...
go tool cover -func=/tmp/cover.out | grep -E "total|reindex"
# Expected: total coverage ≥80%

# Confirm specnum.Parse is used (not a new parsing function)
grep -n "specnum.Parse" pkg/reindex/reindex.go
# Expected: at least one match
```
</verification>
