---
spec: ["024"]
status: created
created: "2026-03-06T00:00:00Z"
---
<summary>
- Changes `spec verify` to move the spec file to `specs.completedDir` after marking it completed
- Updates `NewSpecVerifyCommand` to accept `inProgressDir` and `completedDir`
- Updates `spec verify` to search all three spec dirs (inbox + in-progress + completed) via FindSpecFile
- Updates `spec list` to list specs from all three spec dirs (Lister scans all three)
- Updates `NewLister` to accept multiple directories
- Updates `AutoCompleter` to look for spec files in all three spec dirs when marking verifying
- Updates `CreateSpecVerifyCommand`, `CreateSpecListCommand`, `CreateSpecStatusCommand`, `CreateCombinedListCommand`, `CreateCombinedStatusCommand` in factory.go to pass all three spec dirs
- Updates `spec approve` search to only use `specs.inboxDir` (already done in prompt 119 — verify)
- All existing tests must still pass
</summary>

<objective>
Complete the spec directory lifecycle: when `spec verify` is called, it marks the spec completed and moves the file from `specs/in-progress/` to `specs/completed/`. The `spec list` and `spec status` commands aggregate specs across all three directories. The `AutoCompleter` in `pkg/spec/spec.go` locates the spec file across all three dirs when transitioning to verifying.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `/home/node/.claude/docs/go-patterns.md` and `go-testing.md`.
Read `pkg/cmd/spec_verify.go` — current implementation to change.
Read `pkg/cmd/spec_verify_test.go` — tests to update.
Read `pkg/cmd/spec_finder.go` — FindSpecFile; needs to search multiple dirs.
Read `pkg/cmd/spec_finder_test.go` — tests to update.
Read `pkg/spec/lister.go` — Lister interface and implementation; needs multi-dir support.
Read `pkg/spec/spec.go` — AutoCompleter.CheckAndComplete; needs multi-dir spec lookup.
Read `pkg/spec/spec_test.go` — AutoCompleter tests to update.
Read `pkg/factory/factory.go` — Create* functions to update.
NOTE: This prompt depends on prompts 118 and 119. cfg.Specs.InboxDir,
cfg.Specs.InProgressDir, and cfg.Specs.CompletedDir are available.
</context>

<requirements>
1. In `pkg/cmd/spec_verify.go`, update to accept three dirs and move the file after completing:

   ```go
   type specVerifyCommand struct {
       inboxDir      string
       inProgressDir string
       completedDir  string
   }

   func NewSpecVerifyCommand(inboxDir, inProgressDir, completedDir string) SpecVerifyCommand {
       return &specVerifyCommand{
           inboxDir:      inboxDir,
           inProgressDir: inProgressDir,
           completedDir:  completedDir,
       }
   }

   func (s *specVerifyCommand) Run(ctx context.Context, args []string) error {
       if len(args) == 0 {
           return errors.Errorf(ctx, "spec identifier required")
       }

       id := args[0]
       // Search all three dirs — a verifying spec is in inProgressDir, but allow
       // searching all dirs for convenience
       path, err := FindSpecFileInDirs(ctx, id, s.inboxDir, s.inProgressDir, s.completedDir)
       if err != nil {
           return err
       }

       sf, err := spec.Load(ctx, path)
       if err != nil {
           return errors.Wrap(ctx, err, "load spec")
       }

       if sf.Frontmatter.Status != string(spec.StatusVerifying) {
           return errors.Errorf(
               ctx,
               "spec is not in verifying state (current: %s)",
               sf.Frontmatter.Status,
           )
       }

       sf.MarkCompleted()
       if err := sf.Save(ctx); err != nil {
           return errors.Wrap(ctx, err, "save spec")
       }

       // Ensure completedDir exists
       if err := os.MkdirAll(s.completedDir, 0750); err != nil {
           return errors.Wrap(ctx, err, "create completed dir")
       }

       // Move file to completedDir
       dest := filepath.Join(s.completedDir, filepath.Base(path))
       if err := os.Rename(path, dest); err != nil {
           return errors.Wrap(ctx, err, "move spec to completed")
       }

       fmt.Printf("verified: %s\n", filepath.Base(dest))
       return nil
   }
   ```

   Add `"os"` to imports.

2. In `pkg/cmd/spec_finder.go`, add a new function `FindSpecFileInDirs` that searches multiple directories in order:

   ```go
   // FindSpecFileInDirs searches dirs in order and returns the first match.
   // Falls back to the existing FindSpecFile logic for each dir.
   func FindSpecFileInDirs(ctx context.Context, id string, dirs ...string) (string, error) {
       // Try as a direct path first (absolute or relative with directory component)
       if filepath.IsAbs(id) || strings.ContainsRune(id, '/') {
           if _, err := os.Stat(id); err == nil {
               return id, nil
           }
       }

       for _, dir := range dirs {
           path, err := findInDir(ctx, dir, id)
           if err == nil {
               return path, nil
           }
       }
       return "", errors.Errorf(ctx, "spec not found: %s", id)
   }
   ```

   Extract the per-directory search logic from `FindSpecFile` into a private `findInDir(ctx, dir, id)` helper, then rewrite `FindSpecFile` to call `FindSpecFileInDirs(ctx, id, specsDir)` — preserving backward compatibility. The `findInDir` helper contains the exact/prefix match logic currently in `FindSpecFile`.

3. In `pkg/spec/lister.go`, update `Lister` to scan multiple directories:

   ```go
   type lister struct {
       dirs []string
   }

   func NewLister(dirs ...string) Lister {
       return &lister{dirs: dirs}
   }
   ```

   Update `List()` to scan all dirs and aggregate:
   ```go
   func (l *lister) List(ctx context.Context) ([]*SpecFile, error) {
       var all []*SpecFile
       for _, dir := range l.dirs {
           entries, err := os.ReadDir(dir)
           if err != nil {
               if os.IsNotExist(err) {
                   continue
               }
               return nil, errors.Wrap(ctx, err, "read specs directory")
           }
           for _, entry := range entries {
               if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
                   continue
               }
               path := filepath.Join(dir, entry.Name())
               sf, err := Load(ctx, path)
               if err != nil {
                   return nil, errors.Wrap(ctx, err, "load spec file")
               }
               all = append(all, sf)
           }
       }
       return all, nil
   }
   ```

   `Summary()` is unchanged — it calls `List()` which now aggregates.

4. In `pkg/spec/spec.go`, update `AutoCompleter` to search all three spec dirs when looking up the spec file:

   Update `autoCompleter` struct:
   ```go
   type autoCompleter struct {
       queueDir         string
       completedDir     string
       specsInboxDir    string
       specsInProgressDir string
       specsCompletedDir  string
   }

   func NewAutoCompleter(queueDir, completedDir, specsInboxDir, specsInProgressDir, specsCompletedDir string) AutoCompleter {
       return &autoCompleter{
           queueDir:           queueDir,
           completedDir:       completedDir,
           specsInboxDir:      specsInboxDir,
           specsInProgressDir: specsInProgressDir,
           specsCompletedDir:  specsCompletedDir,
       }
   }
   ```

   Update `CheckAndComplete` to find the spec file across all three spec dirs:
   ```go
   // Find the spec file in any of the three spec dirs
   var specPath string
   for _, dir := range []string{a.specsInboxDir, a.specsInProgressDir, a.specsCompletedDir} {
       candidate := filepath.Join(dir, specID+".md")
       if _, err := os.Stat(candidate); err == nil {
           specPath = candidate
           break
       }
   }
   if specPath == "" {
       slog.Warn("spec file not found in any spec dir", "specID", specID)
       return nil
   }
   ```

   Replace the existing `specPath := filepath.Join(a.specsDir, specID+".md")` line with the above block.

5. In `pkg/factory/factory.go`, update all Create* functions that use spec dirs:

   `CreateSpecListCommand`:
   ```go
   func CreateSpecListCommand(cfg config.Config) cmd.SpecListCommand {
       counter := prompt.NewCounter(cfg.Prompts.InboxDir, cfg.Prompts.InProgressDir, cfg.Prompts.CompletedDir)
       return cmd.NewSpecListCommand(
           spec.NewLister(cfg.Specs.InboxDir, cfg.Specs.InProgressDir, cfg.Specs.CompletedDir),
           counter,
       )
   }
   ```

   `CreateSpecStatusCommand`:
   ```go
   func CreateSpecStatusCommand(cfg config.Config) cmd.SpecStatusCommand {
       counter := prompt.NewCounter(cfg.Prompts.InboxDir, cfg.Prompts.InProgressDir, cfg.Prompts.CompletedDir)
       return cmd.NewSpecStatusCommand(
           spec.NewLister(cfg.Specs.InboxDir, cfg.Specs.InProgressDir, cfg.Specs.CompletedDir),
           counter,
       )
   }
   ```

   `CreateSpecVerifyCommand`:
   ```go
   func CreateSpecVerifyCommand(cfg config.Config) cmd.SpecVerifyCommand {
       return cmd.NewSpecVerifyCommand(cfg.Specs.InboxDir, cfg.Specs.InProgressDir, cfg.Specs.CompletedDir)
   }
   ```

   `CreateCombinedStatusCommand`:
   ```go
   spec.NewLister(cfg.Specs.InboxDir, cfg.Specs.InProgressDir, cfg.Specs.CompletedDir)
   ```

   `CreateCombinedListCommand`:
   ```go
   spec.NewLister(cfg.Specs.InboxDir, cfg.Specs.InProgressDir, cfg.Specs.CompletedDir)
   ```

   `CreateProcessor` — update the `NewAutoCompleter` call:
   ```go
   spec.NewAutoCompleter(
       inProgressDir,
       completedDir,
       cfg.Specs.InboxDir,
       cfg.Specs.InProgressDir,
       cfg.Specs.CompletedDir,
   ),
   ```
   Note: `CreateProcessor` must receive `cfg config.Config` (or the individual spec dir strings) so it can pass them to `NewAutoCompleter`. Currently `CreateProcessor` takes individual string parameters. Add `specsInboxDir`, `specsInProgressDir`, `specsCompletedDir` parameters to `CreateProcessor`, or pass `cfg` directly. The simplest change: add three new string parameters at the end of `CreateProcessor` and update the call in `CreateRunner`.

6. In `pkg/factory/factory.go`, update `CreateProcessor` signature:
   ```go
   func CreateProcessor(
       inProgressDir string,
       completedDir string,
       logDir string,
       projectName string,
       promptManager prompt.Manager,
       releaser git.Releaser,
       versionGetter version.Getter,
       ready <-chan struct{},
       containerImage string,
       model string,
       workflow config.Workflow,
       ghToken string,
       autoMerge bool,
       autoRelease bool,
       autoReview bool,
       specsInboxDir string,
       specsInProgressDir string,
       specsCompletedDir string,
   ) processor.Processor
   ```
   And update the call in `CreateRunner` to pass `cfg.Specs.InboxDir`, `cfg.Specs.InProgressDir`, `cfg.Specs.CompletedDir`.

7. Update `pkg/cmd/spec_verify_test.go`:
   - Update `NewSpecVerifyCommand` calls to pass three dirs
   - Add test: after `Run`, file is moved to `completedDir`
   - Add test: file is not found in `inProgressDir` after verify
   - Keep existing tests: error on wrong status, error on not found

8. Update `pkg/cmd/spec_finder_test.go`:
   - Add test for `FindSpecFileInDirs` with multiple dirs
   - Test: id found in second dir when not in first
   - Test: error when not found in any dir

9. Update `pkg/spec/lister_test.go` (or `spec_test.go`):
   - Add test for `NewLister` with multiple dirs
   - Test: specs from both dirs are returned
   - Test: missing dir is silently skipped (IsNotExist returns empty, not error)

10. Update `pkg/spec/spec_test.go` for `NewAutoCompleter`:
    - Update constructor calls to pass five args
    - Tests for `CheckAndComplete` still pass

11. Run `make generate` if any counterfeiter-annotated interfaces changed. Check `Lister` interface — it is unchanged (same `List` and `Summary` methods), so mocks likely don't need regeneration. Check `AutoCompleter` interface — unchanged. Check `SpecVerifyCommand` — unchanged.
</requirements>

<constraints>
- `spec approve` already searches only `specs.inboxDir` (done in prompt 119) — do not change that
- `spec verify` searches all three dirs — verifying specs are typically in inProgressDir but searching all is safe
- `spec list` must aggregate specs from all three dirs and preserve the existing output format
- Use `os.Rename` for the file move in spec_verify (same as spec_approve)
- The `Lister` interface methods (`List`, `Summary`) are unchanged — only `NewLister` changes to variadic
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- `make precommit` must pass
</constraints>

<verification>
Run `make precommit` — must pass.

Additional checks:
```bash
# Confirm spec verify moves file
grep -n 'os.Rename\|completedDir' pkg/cmd/spec_verify.go

# Confirm Lister accepts multiple dirs
grep -n 'func NewLister' pkg/spec/lister.go

# Confirm AutoCompleter searches all three spec dirs
grep -n 'specsInboxDir\|specsInProgressDir\|specsCompletedDir' pkg/spec/spec.go

# Confirm FindSpecFileInDirs exists
grep -n 'FindSpecFileInDirs' pkg/cmd/spec_finder.go

go test ./pkg/spec/... -v
go test ./pkg/cmd/... -v -run TestSpecVerify
go test ./pkg/cmd/... -v -run TestSpecList
```
</verification>
