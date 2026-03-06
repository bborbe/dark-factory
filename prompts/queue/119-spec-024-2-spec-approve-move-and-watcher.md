---
spec: ["024"]
status: created
created: "2026-03-06T00:00:00Z"
---
<summary>
- Changes `spec approve` to move the spec file from `specs.inboxDir` to `specs.inProgressDir` after setting status to approved
- Updates `NewSpecApproveCommand` to accept both `inboxDir` and `inProgressDir`
- Changes `SpecWatcher` to watch `specs.inProgressDir` for Create events only — no frontmatter polling
- Removes the startup `scanExistingApproved` scan (no longer needed: files in inProgressDir are already approved by definition)
- Updates `CreateSpecWatcher` in factory.go to pass `cfg.Specs.InProgressDir`
- Updates `CreateSpecGenerator` in factory.go to log to `cfg.Specs.LogDir`
- Generator already uses `specs.logDir` for the log file path — verify this is passed correctly
- All existing tests must still pass
</summary>

<objective>
After prompt 118 restructures the config, change the spec approval flow so that the file move itself is the signal: `spec approve` sets `status: approved` and moves the file from `specs/` (inboxDir) to `specs/in-progress/` (inProgressDir). The SpecWatcher watches `specs/in-progress/` for Create events — when a file appears there, it triggers generation immediately without reading frontmatter status.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `/home/node/.claude/docs/go-patterns.md` and `go-testing.md`.
Read `pkg/cmd/spec_approve.go` — current implementation to change.
Read `pkg/cmd/spec_approve_test.go` — tests to update.
Read `pkg/cmd/spec_finder.go` — FindSpecFile used by approve.
Read `pkg/specwatcher/watcher.go` — current implementation to change.
Read `pkg/specwatcher/watcher_test.go` — tests to update.
Read `pkg/factory/factory.go` — CreateSpecWatcher and CreateSpecGenerator to update.
Read `pkg/generator/generator.go` — verify logDir usage.
Read `mocks/spec-watcher.go` — counterfeiter mock (regenerate if interface changes).
NOTE: This prompt depends on prompt 118 (config restructure). cfg.Specs.InboxDir,
cfg.Specs.InProgressDir, and cfg.Specs.LogDir are available after prompt 118.
</context>

<requirements>
1. In `pkg/cmd/spec_approve.go`, update `specApproveCommand` to accept both dirs and move the file:

   ```go
   type specApproveCommand struct {
       inboxDir      string
       inProgressDir string
   }

   func NewSpecApproveCommand(inboxDir string, inProgressDir string) SpecApproveCommand {
       return &specApproveCommand{
           inboxDir:      inboxDir,
           inProgressDir: inProgressDir,
       }
   }

   func (s *specApproveCommand) Run(ctx context.Context, args []string) error {
       if len(args) == 0 {
           return errors.Errorf(ctx, "spec identifier required")
       }

       id := args[0]
       path, err := FindSpecFile(ctx, s.inboxDir, id)
       if err != nil {
           return err
       }

       sf, err := spec.Load(ctx, path)
       if err != nil {
           return errors.Wrap(ctx, err, "load spec")
       }

       if sf.Frontmatter.Status == string(spec.StatusApproved) {
           return errors.Errorf(ctx, "spec is already approved")
       }

       sf.SetStatus(string(spec.StatusApproved))
       if err := sf.Save(ctx); err != nil {
           return errors.Wrap(ctx, err, "save spec")
       }

       // Ensure inProgressDir exists
       if err := os.MkdirAll(s.inProgressDir, 0750); err != nil {
           return errors.Wrap(ctx, err, "create in-progress dir")
       }

       // Move file to inProgressDir — the file move is the signal to SpecWatcher
       dest := filepath.Join(s.inProgressDir, filepath.Base(path))
       if err := os.Rename(path, dest); err != nil {
           return errors.Wrap(ctx, err, "move spec to in-progress")
       }

       fmt.Printf("approved: %s\n", filepath.Base(dest))
       return nil
   }
   ```

   Add `"os"` to imports if not already present.

2. In `pkg/factory/factory.go`, update `CreateSpecApproveCommand`:

   ```go
   func CreateSpecApproveCommand(cfg config.Config) cmd.SpecApproveCommand {
       return cmd.NewSpecApproveCommand(cfg.Specs.InboxDir, cfg.Specs.InProgressDir)
   }
   ```

3. In `pkg/specwatcher/watcher.go`, change the watcher to:
   - Watch `specs.inProgressDir` (not `specs.inboxDir`)
   - Trigger generation on **Create events only** (a new file appearing = approved spec moved here)
   - Remove `scanExistingApproved` — files already in inProgressDir were placed there by a previous `spec approve` and have already been processed (or will be picked up by the startup scan, which is now simpler — see requirement 4)
   - Remove the status check (`sf.Frontmatter.Status != string(spec.StatusApproved)`) — any `.md` Create event in inProgressDir triggers generation

   Updated struct and constructor:
   ```go
   type specWatcher struct {
       inProgressDir string
       generator     generator.SpecGenerator
       debounce      time.Duration
       mu            sync.Mutex
   }

   func NewSpecWatcher(
       inProgressDir string,
       generator generator.SpecGenerator,
       debounce time.Duration,
   ) SpecWatcher {
       return &specWatcher{
           inProgressDir: inProgressDir,
           generator:     generator,
           debounce:      debounce,
       }
   }
   ```

   Updated `Watch`:
   - Watch `absInProgressDir` (renamed from `absSpecsDir`)
   - Call `w.scanExistingInProgress(ctx, absInProgressDir)` on startup to handle files already present from before restart
   - Log: `"spec watcher started", "dir", absInProgressDir`

   Updated `handleWatchEvent`:
   - Only trigger on `fsnotify.Create` events (not Write or Chmod):
     ```go
     if event.Op&fsnotify.Create == 0 {
         return
     }
     ```
   - Still filter to `.md` files only

   Updated `handleFileEvent`:
   - Remove the status check entirely — any `.md` file in inProgressDir is approved by definition
   - Call `w.generator.Generate(ctx, specPath)` directly after debounce fires
   - Log: `"spec file created in in-progress, triggering generation", "path", specPath`

   Rename `scanExistingApproved` → `scanExistingInProgress`:
   ```go
   // scanExistingInProgress scans inProgressDir for .md files on startup and triggers
   // generation for each. This handles specs that were moved here before the daemon started.
   func (w *specWatcher) scanExistingInProgress(ctx context.Context, inProgressDir string) {
       entries, err := os.ReadDir(inProgressDir)
       if err != nil {
           slog.Info("failed to read spec in-progress dir on startup", "dir", inProgressDir, "error", err)
           return
       }
       for _, entry := range entries {
           if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
               continue
           }
           w.handleFileEvent(ctx, filepath.Join(inProgressDir, entry.Name()))
       }
   }
   ```

   Rename `getSpecsDir()` → `getInProgressDir()` and update to use `w.inProgressDir`.

4. In `pkg/factory/factory.go`, update `CreateSpecWatcher`:

   ```go
   func CreateSpecWatcher(cfg config.Config, gen generator.SpecGenerator) specwatcher.SpecWatcher {
       return specwatcher.NewSpecWatcher(
           cfg.Specs.InProgressDir,
           gen,
           time.Duration(cfg.DebounceMs)*time.Millisecond,
       )
   }
   ```

5. In `pkg/factory/factory.go`, verify `CreateSpecGenerator` passes `cfg.Specs.LogDir`:

   ```go
   func CreateSpecGenerator(cfg config.Config, containerImage string) generator.SpecGenerator {
       return generator.NewSpecGenerator(
           executor.NewDockerExecutor(containerImage, project.Name(cfg.ProjectName), cfg.Model),
           cfg.Prompts.InboxDir,
           cfg.Prompts.CompletedDir,
           cfg.Specs.InboxDir,
           cfg.Specs.LogDir,
       )
   }
   ```

6. Update `pkg/cmd/spec_approve_test.go`:
   - Update `NewSpecApproveCommand` calls to pass both `inboxDir` and `inProgressDir`
   - Add test: after `Run`, the spec file is no longer in `inboxDir` and is present in `inProgressDir`
   - Add test: the moved file has `status: approved` in its frontmatter
   - Keep existing tests: error on missing args, error on already-approved spec

7. Update `pkg/specwatcher/watcher_test.go`:
   - Update `NewSpecWatcher` calls to pass `inProgressDir` instead of `specsDir`
   - Update test: a `.md` Create event in `inProgressDir` triggers generator (no status check needed)
   - Remove test: "non-approved spec → generator NOT called" (status check removed)
   - Keep test: generator error → logged, watcher continues
   - Add test: Write event (not Create) does NOT trigger generator

8. Run `make generate` to regenerate counterfeiter mocks if the `SpecWatcher` or `SpecApproveCommand` interfaces changed. The interfaces themselves haven't changed (same method signatures), so mocks likely don't need regeneration. Verify by checking if `mocks/spec-watcher.go` and `mocks/spec-approve-command.go` are still valid.
</requirements>

<constraints>
- `spec approve` searches only `specs.inboxDir` — approved specs live in `specs.inboxDir` before approval
- The SpecWatcher triggers on Create events only (not Write/Chmod) — this is the key behavior change
- Use `os.Rename` for the file move in spec_approve (not git mv — spec files are not tracked the same way as prompts)
- The file move in spec_approve does NOT need to be a git operation — the SpecWatcher sees the Create event via fsnotify
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- `make precommit` must pass
</constraints>

<verification>
Run `make precommit` — must pass.

Additional checks:
```bash
# Confirm SpecWatcher only triggers on Create
grep -n 'fsnotify.Create\|fsnotify.Write' pkg/specwatcher/watcher.go

# Confirm spec approve moves the file
grep -n 'os.Rename\|inProgressDir' pkg/cmd/spec_approve.go

# Confirm CreateSpecWatcher uses InProgressDir
grep -n 'InProgressDir' pkg/factory/factory.go

go test ./pkg/specwatcher/... -v
go test ./pkg/cmd/... -v -run TestSpecApprove
```
</verification>
