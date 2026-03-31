---
status: completed
spec: [024-config-restructure]
summary: Implemented startup directory creation for all 8 lifecycle dirs, queue→in-progress migration, and inbox number scanning in NormalizeFilenames
container: dark-factory-121-spec-024-4-migration-startup-numbering
dark-factory-version: v0.20.2
created: "2026-03-06T00:00:00Z"
queued: "2026-03-06T19:57:43Z"
started: "2026-03-06T19:57:44Z"
completed: "2026-03-06T20:15:09Z"
---
<summary>
- On startup, creates all required directories if they don't exist: prompts.inboxDir, prompts.inProgressDir, prompts.completedDir, prompts.logDir, specs.inboxDir, specs.inProgressDir, specs.completedDir, specs.logDir
- On startup, migrates `prompts/queue/` → `prompts/in-progress/` by renaming the directory if the old path exists and the new path does not
- Updates `NormalizeFilenames` so number assignment scans all three prompt dirs (inbox + in-progress + completed) to find the highest used number before assigning a new one
- Updates runner.go `createDirectories` to create all eight dirs (not just three)
- Migration is one-time and idempotent: if old dir doesn't exist, skip; if new dir already exists, skip
- All existing tests must still pass
</summary>

<objective>
On every startup, dark-factory ensures all eight lifecycle directories exist and migrates the old `prompts/queue/` directory to `prompts/in-progress/` if it exists. Number assignment for new prompt files scans all three prompt directories (inbox + in-progress + completed) so no number is ever reused across directories.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `/home/node/.claude/docs/go-patterns.md` and `go-testing.md`.
Read `pkg/runner/runner.go` — `createDirectories` and `Run` method to update.
Read `pkg/runner/runner_test.go` — tests to update.
Read `pkg/prompt/prompt.go` — `NormalizeFilenames` and `scanPromptFiles` functions to update.
Read `pkg/prompt/prompt_test.go` — tests to update.
Read `pkg/factory/factory.go` — `CreateRunner` to check how dirs are passed to runner.
NOTE: This prompt depends on prompts 118, 119, and 120. All config fields and
dir lifecycle changes from those prompts are in place.
</context>

<requirements>
1. In `pkg/runner/runner.go`, update `createDirectories` to create all eight lifecycle directories:

   ```go
   func (r *runner) createDirectories(ctx context.Context) error {
       dirs := []string{
           r.inboxDir,
           r.inProgressDir,
           r.completedDir,
           r.logDir,
           r.specsInboxDir,
           r.specsInProgressDir,
           r.specsCompletedDir,
           r.specsLogDir,
       }
       for _, dir := range dirs {
           if err := os.MkdirAll(dir, 0750); err != nil {
               return errors.Wrap(ctx, err, fmt.Sprintf("create directory %s", dir))
           }
       }
       return nil
   }
   ```

   Add the new fields to the `runner` struct:
   ```go
   type runner struct {
       inboxDir           string
       inProgressDir      string
       completedDir       string
       logDir             string
       specsInboxDir      string
       specsInProgressDir string
       specsCompletedDir  string
       specsLogDir        string
       promptManager      prompt.Manager
       locker             lock.Locker
       watcher            watcher.Watcher
       processor          processor.Processor
       server             server.Server
       reviewPoller       review.ReviewPoller
       specWatcher        specwatcher.SpecWatcher
   }
   ```

   Update `NewRunner` to accept the new parameters:
   ```go
   func NewRunner(
       inboxDir string,
       inProgressDir string,
       completedDir string,
       logDir string,
       specsInboxDir string,
       specsInProgressDir string,
       specsCompletedDir string,
       specsLogDir string,
       promptManager prompt.Manager,
       locker lock.Locker,
       watcher watcher.Watcher,
       processor processor.Processor,
       server server.Server,
       reviewPoller review.ReviewPoller,
       specWatcher specwatcher.SpecWatcher,
   ) Runner
   ```

2. In `pkg/runner/runner.go`, add a `migrateQueueDir` method called from `Run` before `createDirectories`:

   ```go
   // migrateQueueDir renames prompts/queue/ → prompts/in-progress/ if the old path
   // exists and the new path does not. This is a one-time migration.
   func (r *runner) migrateQueueDir(ctx context.Context) error {
       oldQueue := filepath.Join(filepath.Dir(r.inProgressDir), "queue")
       // Only migrate if old dir exists
       if _, err := os.Stat(oldQueue); os.IsNotExist(err) {
           return nil
       }
       // Skip if new dir already exists (migration already done or manually created)
       if _, err := os.Stat(r.inProgressDir); err == nil {
           slog.Info("skipping queue migration: in-progress dir already exists",
               "old", oldQueue, "new", r.inProgressDir)
           return nil
       }
       if err := os.Rename(oldQueue, r.inProgressDir); err != nil {
           return errors.Wrap(ctx, err, "migrate queue dir to in-progress")
       }
       slog.Info("migrated queue dir to in-progress", "old", oldQueue, "new", r.inProgressDir)
       return nil
   }
   ```

   Call it from `Run`, before `createDirectories`:
   ```go
   // Migrate old prompts/queue/ → prompts/in-progress/ if needed
   if err := r.migrateQueueDir(ctx); err != nil {
       return errors.Wrap(ctx, err, "migrate queue dir")
   }

   // Create directories if they don't exist
   if err := r.createDirectories(ctx); err != nil {
       return errors.Wrap(ctx, err, "create directories")
   }
   ```

3. In `pkg/factory/factory.go`, update `CreateRunner` to pass the new parameters to `NewRunner`:

   ```go
   return runner.NewRunner(
       inboxDir,
       inProgressDir,
       completedDir,
       cfg.Prompts.LogDir,
       cfg.Specs.InboxDir,
       cfg.Specs.InProgressDir,
       cfg.Specs.CompletedDir,
       cfg.Specs.LogDir,
       promptManager,
       CreateLocker("."),
       CreateWatcher(...),
       CreateProcessor(...),
       srv,
       reviewPoller,
       specWatcher,
   )
   ```

4. In `pkg/prompt/prompt.go`, update `NormalizeFilenames` to scan all three prompt dirs (inbox + in-progress + completed) when collecting used numbers:

   Update the function signature to accept all three dirs:
   ```go
   func NormalizeFilenames(
       ctx context.Context,
       dir string,
       inboxDir string,
       completedDir string,
       mover FileMover,
   ) ([]Rename, error)
   ```

   In the body, collect used numbers from BOTH `inboxDir` and `completedDir` (in addition to `dir`):
   ```go
   files, usedNumbers := scanPromptFiles(entries)

   // Also collect numbers used in inboxDir so we don't reuse numbers from draft prompts.
   inboxEntries, err := os.ReadDir(inboxDir)
   if err != nil && !os.IsNotExist(err) {
       return nil, errors.Wrap(ctx, err, "read inbox directory")
   }
   _, inboxNumbers := scanPromptFiles(inboxEntries)
   for n := range inboxNumbers {
       usedNumbers[n] = true
   }

   // Also collect numbers used in completedDir.
   completedEntries, err := os.ReadDir(completedDir)
   if err != nil && !os.IsNotExist(err) {
       return nil, errors.Wrap(ctx, err, "read completed directory")
   }
   _, completedNumbers := scanPromptFiles(completedEntries)
   for n := range completedNumbers {
       usedNumbers[n] = true
   }
   ```

   Note: the existing code already scans `completedDir`. Add the `inboxDir` scan. Rename the parameter to make it clear both inbox and completed are scanned.

5. Update `Manager.NormalizeFilenames` in `pkg/prompt/prompt.go`:
   - The `manager` struct currently stores `queueDir` (now `inProgressDir`) and `completedDir`
   - Add `inboxDir` field to `manager` and update `NewManager`:
     ```go
     type manager struct {
         inboxDir     string
         inProgressDir string // previously queueDir
         completedDir string
         mover        FileMover
     }

     func NewManager(inboxDir string, inProgressDir string, completedDir string, mover FileMover) Manager {
         return &manager{
             inboxDir:      inboxDir,
             inProgressDir: inProgressDir,
             completedDir:  completedDir,
             mover:         mover,
         }
     }
     ```
   - Update `manager.NormalizeFilenames`:
     ```go
     func (pm *manager) NormalizeFilenames(ctx context.Context, dir string) ([]Rename, error) {
         return NormalizeFilenames(ctx, dir, pm.inboxDir, pm.completedDir, pm.mover)
     }
     ```
   - Update `manager.ResetExecuting`, `manager.ResetFailed`, `manager.HasExecuting`, `manager.ListQueued` to use `pm.inProgressDir` instead of `pm.queueDir`
   - Update `manager.MoveToCompleted` — unchanged (uses `pm.completedDir`)
   - Update `manager.AllPreviousCompleted` — unchanged (uses `pm.completedDir`)

6. Update `pkg/factory/factory.go` wherever `NewManager` and `NewPromptManager` (the private helper) are called:
   ```go
   func createPromptManager(inboxDir string, inProgressDir string, completedDir string) (prompt.Manager, git.Releaser) {
       releaser := git.NewReleaser()
       promptManager := prompt.NewManager(inboxDir, inProgressDir, completedDir, releaser)
       return promptManager, releaser
   }
   ```
   Update all callers of `createPromptManager` to pass `cfg.Prompts.InboxDir`, `cfg.Prompts.InProgressDir`, `cfg.Prompts.CompletedDir`.

7. Update `pkg/runner/runner_test.go`:
   - Update `NewRunner` calls to pass the new parameters
   - Add test: `migrateQueueDir` renames `prompts/queue` → `prompts/in-progress` when old dir exists
   - Add test: `migrateQueueDir` is a no-op when old dir does not exist
   - Add test: `migrateQueueDir` skips when `prompts/in-progress` already exists
   - Update `createDirectories` test to verify all eight dirs are created

8. Update `pkg/prompt/prompt_test.go`:
   - Update `NormalizeFilenames` calls to pass the extra `inboxDir` parameter
   - Add test: number assignment does not reuse numbers from `inboxDir`
   - Add test: number assignment does not reuse numbers from `completedDir`
   - Update `NewManager` calls to pass `inboxDir`

9. Run `make generate` if any counterfeiter-annotated interfaces changed. The `Manager` interface adds no new methods, `Runner` interface is unchanged — mocks should not need regeneration. Verify by checking `mocks/prompt-manager.go`.
</requirements>

<constraints>
- Migration is done with `os.Rename` (directory rename), not file-by-file copy
- Migration only runs if `prompts/queue/` exists AND `prompts/in-progress/` does not exist
- The old `prompts/queue/` path is derived as `filepath.Join(filepath.Dir(r.inProgressDir), "queue")` — do NOT hardcode it
- If `inProgressDir` is `prompts/in-progress`, then the old dir is `prompts/queue`
- All eight dirs are created on every startup (idempotent via `os.MkdirAll`)
- Existing prompts in `prompts/queue/` must be picked up after migration (they are moved to `prompts/in-progress/`)
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- `make precommit` must pass
</constraints>

<verification>
Run `make precommit` — must pass.

Additional checks:
```bash
# Confirm migration logic exists in runner
grep -n 'migrateQueueDir\|queue.*in-progress\|in-progress.*queue' pkg/runner/runner.go

# Confirm all 8 dirs in createDirectories
grep -A 20 'func.*createDirectories' pkg/runner/runner.go

# Confirm NormalizeFilenames scans inbox too
grep -n 'inboxDir\|inboxEntries\|inboxNumbers' pkg/prompt/prompt.go

# Confirm NewManager takes inboxDir
grep -n 'func NewManager' pkg/prompt/prompt.go

go test ./pkg/runner/... -v
go test ./pkg/prompt/... -v
```
</verification>
