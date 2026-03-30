---
status: executing
spec: ["036"]
container: dark-factory-224-spec-036-slug-migrator
dark-factory-version: v0.69.0
created: "2026-03-30T17:00:00Z"
queued: "2026-03-30T17:29:26Z"
started: "2026-03-30T17:29:28Z"
branch: dark-factory/full-slug-spec-references
---
<summary>
- Bare spec number references in prompts are automatically expanded to human-readable full slugs
- References like `["036"]` become `["036-full-slug-spec-references"]` by looking up actual spec files
- Unresolvable or ambiguous references are left unchanged with a warning
- Already-expanded references are not modified (idempotent)
- Prompts without spec references are skipped
</summary>

<objective>
Create `pkg/slugmigrator` with a `Migrator` interface and implementation that scans prompt files and replaces bare spec number references (e.g. `"036"`) with full spec slugs (e.g. `"036-full-slug-spec-references"`). This is the core reusable migration logic used in both startup migration and generator post-processing.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read pkg/specnum/specnum.go — `specnum.Parse` is the single source of truth for extracting numbers; use it, do not duplicate.
Read pkg/prompt/prompt.go — `Frontmatter.Specs` (`SpecList`), `HasSpec`, `Load`, `Save`, and `NewPromptFile` are the prompt primitives.
Read pkg/spec/lister.go and pkg/spec/spec.go — understand how spec files are stored across lifecycle dirs.
Read /home/node/.claude/docs/ for Go conventions (patterns, testing, factory pattern).
</context>

<requirements>
1. Create `pkg/slugmigrator/migrator.go` with:

   **Interface** (with counterfeiter annotation):
   ```go
   //counterfeiter:generate -o ../../mocks/spec-slug-migrator.go --fake-name SpecSlugMigrator . Migrator
   type Migrator interface {
       // MigrateDirs scans all .md files in each dir and replaces bare spec number
       // references with full slugs. Skips files that cannot be parsed.
       MigrateDirs(ctx context.Context, promptDirs []string) error
   }
   ```

   **Implementation** `specSlugMigrator`:
   - Constructor:
     ```go
     func NewMigrator(
         specsDirs []string,
         currentDateTimeGetter libtime.CurrentDateTimeGetter,
     ) Migrator
     ```
   - `specsDirs` is the list of all spec lifecycle directories to scan for spec files
     (inbox, in-progress, completed). Pass all three so specs at any stage are found.

   **buildSlugMap(ctx, specsDirs)** — private helper:
   - Scans each dir in `specsDirs` for `.md` files
   - For each file, runs `specnum.Parse(strings.TrimSuffix(name, ".md"))` to extract the number
   - If number < 0, skip (no numeric prefix)
   - Key = number (int), Value = full slug without extension (e.g. `"036-full-slug-spec-references"`)
   - If the same number appears in multiple dirs (across all scanned dirs), log a warning and
     do NOT add to the map (or mark as ambiguous). Skip all ambiguous numbers during migration.
   - Returns `map[int]string`

   **MigrateDirs** implementation:
   - For each dir in `promptDirs`:
     - Read directory; skip if not exists
     - For each `.md` file, call `migrateFile(ctx, path, slugMap)`
   - Returns nil (individual file failures are logged and skipped, not fatal)

   **migrateFile(ctx, path, slugMap)** — private helper:
   - Load prompt via `prompt.Load`
   - If `len(fm.Specs) == 0`, return nil (nothing to do)
   - For each spec ref in `fm.Specs`:
     - Run `specnum.Parse(ref)` — if number < 0 OR the ref already has a non-numeric slug
       suffix (i.e. `strings.Contains(ref, "-")`), treat it as already a full slug → keep as-is
     - If it is a bare number (no `-` in ref, just digits): look up `slugMap[number]`
       - If not found in map: `slog.Warn("spec slug not found, leaving bare ref", "ref", ref, "file", path)` and keep as-is
       - If found: replace with the full slug string from the map
   - If no refs changed, return nil (don't write)
   - Save the updated prompt: `pf.Save(ctx)`
   - Log: `slog.Info("migrated spec refs to full slugs", "file", filepath.Base(path), "updated", changedCount)`

2. Create `pkg/slugmigrator/migrator_test.go` using Ginkgo/Gomega covering:
   - `MigrateDirs` on a directory with a bare-number ref → ref updated to full slug
   - `MigrateDirs` on a file already using full slug → unchanged (idempotent)
   - `MigrateDirs` on a file with no `spec:` field → unchanged, no error
   - Bare number with no matching spec file → unchanged, no error (warning only)
   - Two spec files with the same number (ambiguous) → bare ref left unchanged, no error
   - Empty directory → no error
   - Non-existent directory → no error

3. Run `make generate` to produce the counterfeiter mock at `mocks/spec-slug-migrator.go`.
</requirements>

<constraints>
- Use `github.com/bborbe/errors` for error wrapping (not fmt.Errorf)
- Use `github.com/bborbe/dark-factory/pkg/specnum` for number extraction — do NOT duplicate logic
- Use `github.com/bborbe/dark-factory/pkg/prompt` for Load/Save/Frontmatter access
- A bare spec ref is identified by: `specnum.Parse(ref) >= 0` AND `!strings.Contains(ref, "-")`
  (i.e. it parses as a number and has no slug suffix — "036" is bare, "036-foo" is full)
- Prompt files that fail to load must be skipped with a `slog.Warn` (not returned as errors)
- The `specsDirs` ambiguity check is global: if number N appears in ANY two spec files across
  all scanned spec dirs, N is ambiguous and bare refs to N are left unchanged
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
</constraints>

<verification>
Run `make precommit` — must pass.
Check coverage: `go test -coverprofile=/tmp/cover.out -mod=vendor ./pkg/slugmigrator/... && go tool cover -func=/tmp/cover.out | grep -E "^total|slugmigrator"`
Coverage must be ≥80%.
</verification>
