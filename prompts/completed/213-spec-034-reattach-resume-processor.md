---
status: completed
spec: ["034"]
container: dark-factory-213-spec-034-reattach-resume-processor
dark-factory-version: v0.63.0
created: "2026-03-21T18:00:00Z"
queued: "2026-03-21T19:11:50Z"
started: "2026-03-21T19:36:12Z"
completed: "2026-03-21T19:59:23Z"
branch: dark-factory/resume-executing-on-restart
---

<summary>
- The Executor interface gains a Reattach method that connects to a running container's log stream and waits for it to exit — no new container is created
- Reattach opens the log file in overwrite mode (docker logs --follow replays all container output from the start), runs the stuck-container watcher in parallel, and returns when the container exits
- The Processor interface gains a ResumeExecuting method that handles the startup resume flow: it scans for executing prompts, reconstructs execution context from frontmatter, and drives them through to completion using Reattach
- For PR workflow: if the clone directory no longer exists, the prompt is reset to approved instead of resuming
- The runner calls ResumeExecuting once before the main event loop starts, completing any interrupted executions
- Resumed executions produce the same completion flow (report parsing, git ops, status update) as fresh executions
- Counterfeiter mocks are regenerated for both updated interfaces
</summary>

<objective>
Add re-attach capability to the executor (so it can monitor an already-running container) and a resume startup method to the processor (so it can drive interrupted executions to completion after dark-factory restarts). Wire both into runner.Run() so the resume happens before the normal event loop begins.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read all `/home/node/.claude/docs/go-*.md` docs before starting.

**This prompt builds on prompt 1** (1-spec-034-container-liveness-selective-reset), which added `resumeOrResetExecuting` to the runner. After that method runs, prompts with still-running containers are left in "executing" state. This prompt picks them up and drives them to completion.

Read `pkg/executor/executor.go` — `Executor` interface (line ~29), `Execute` method, `watchForCompletionReport` function (line ~162), `prepareLogFile` (line ~206), `commandRunner` interface, `buildDockerCommand`.
Read `pkg/executor/checker.go` — `ContainerChecker` interface (created in prompt 1).
Read `pkg/processor/processor.go`:
  - `Processor` interface (line ~34) and `NewProcessor` (line ~40)
  - `processPrompt` method — the normal execution flow this resume mirrors
  - `handlePostExecution` method — called after execution completes; resume must call it too
  - `workflowState` struct (line ~532) — must reconstruct for resume
  - `preparePromptForExecution` (line ~1107) — derives baseName and containerName; resume reads these from frontmatter instead
  - `setupCloneWorkflowState` (line ~730) — clone path: `filepath.Join(os.TempDir(), "dark-factory", p.projectName+"-"+baseName)` (line ~742)
  - `sanitizeContainerName` (line ~1200) — used to reconstruct baseName from filename
Read `pkg/runner/runner.go` — `Run()` method, where `resumeOrResetExecuting` is called (from prompt 1).
Read `pkg/prompt/prompt.go` — `Frontmatter.Container`, `Frontmatter.Branch`, `ExecutingPromptStatus`, `MarkApproved`.
Read `mocks/executor.go` and `mocks/processor.go` — existing mock patterns.
Read `pkg/report/suffix.go` — `MarkerEnd` constant used by `watchForCompletionReport`.
</context>

<requirements>
1. **Add `Reattach` to the `Executor` interface** in `pkg/executor/executor.go`:
   ```go
   // Reattach connects to a running container's output stream and waits for it to exit.
   // It does not create a new container. The log file is overwritten from the beginning
   // of the container's output (docker logs replays all output from container start).
   // Returns nil when the container exits successfully.
   Reattach(ctx context.Context, logFile string, containerName string) error
   ```
   Place the new method after `Execute` in the interface definition.

2. **Implement `Reattach` on `dockerExecutor`** in `pkg/executor/executor.go`:
   ```go
   func (e *dockerExecutor) Reattach(ctx context.Context, logFile string, containerName string) error {
       logFileHandle, err := prepareLogFile(ctx, logFile)
       if err != nil {
           return errors.Wrap(ctx, err, "prepare log file for reattach")
       }
       defer logFileHandle.Close()

       // docker logs --follow replays all output from container start and blocks until exit
       // #nosec G204 -- containerName is generated internally from prompt filename
       cmd := exec.CommandContext(ctx, "docker", "logs", "--follow", containerName)
       cmd.Stdout = io.MultiWriter(os.Stdout, logFileHandle)
       cmd.Stderr = io.MultiWriter(os.Stderr, logFileHandle)

       slog.Info("reattaching to running container", "containerName", containerName)

       if err := run.CancelOnFirstFinish(ctx,
           func(ctx context.Context) error {
               return e.commandRunner.Run(ctx, cmd)
           },
           func(ctx context.Context) error {
               return watchForCompletionReport(ctx, logFile, containerName, 2*time.Minute, 10*time.Second, e.commandRunner)
           },
       ); err != nil {
           return errors.Wrap(ctx, err, "reattach failed")
       }
       return nil
   }
   ```
   - Reuses `prepareLogFile` (truncates/creates the log file, then writes docker logs output into it).
   - Reuses `watchForCompletionReport` exactly as `Execute` does.
   - `prepareLogFile` opens with `O_TRUNC` — this is correct: `docker logs --follow` replays all output from the start, so we get the full log from scratch.

3. **Regenerate the Executor mock**:
   Run `go generate ./pkg/executor/...` to regenerate `mocks/executor.go` with the new `Reattach` method.

4. **Add `ResumeExecuting` to the `Processor` interface** in `pkg/processor/processor.go`:
   ```go
   // ResumeExecuting resumes any prompts still in "executing" state on startup.
   // Called once by the runner before the normal event loop begins.
   // For each executing prompt, it reattaches to the running container and drives
   // the prompt to completion through the normal post-execution flow.
   ResumeExecuting(ctx context.Context) error
   ```

5. **Implement `ResumeExecuting` on `processor`** in `pkg/processor/processor.go`:
   ```go
   func (p *processor) ResumeExecuting(ctx context.Context) error {
       entries, err := os.ReadDir(p.queueDir)
       if err != nil {
           if os.IsNotExist(err) {
               return nil
           }
           return errors.Wrap(ctx, err, "read queue dir for resume")
       }
       for _, entry := range entries {
           if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
               continue
           }
           promptPath := filepath.Join(p.queueDir, entry.Name())
           if err := p.resumePrompt(ctx, promptPath); err != nil {
               return errors.Wrap(ctx, err, "resume prompt")
           }
       }
       return nil
   }
   ```

6. **Implement `resumePrompt(ctx, promptPath) error`** (private method on `processor`):
   ```go
   func (p *processor) resumePrompt(ctx context.Context, promptPath string) error {
       pf, err := p.promptManager.Load(ctx, promptPath)
       if err != nil {
           return errors.Wrap(ctx, err, "load prompt for resume")
       }
       if prompt.PromptStatus(pf.Frontmatter.Status) != prompt.ExecutingPromptStatus {
           return nil // not executing — skip
       }

       containerName := pf.Frontmatter.Container
       if containerName == "" {
           // No container name in frontmatter — cannot resume; reset to approved
           slog.Warn("cannot resume prompt: no container name in frontmatter; resetting to approved",
               "file", filepath.Base(promptPath))
           pf.MarkApproved()
           if err := pf.Save(ctx); err != nil {
               return errors.Wrap(ctx, err, "save prompt after failed resume")
           }
           return nil
       }

       baseName := strings.TrimSuffix(filepath.Base(promptPath), ".md")
       baseName = sanitizeContainerName(baseName)

       logFile, err := filepath.Abs(filepath.Join(p.logDir, baseName+".log"))
       if err != nil {
           return errors.Wrap(ctx, err, "resolve log file path for resume")
       }

       title := pf.Title()
       if title == "" {
           title = baseName
       }

       // Reconstruct workflowState from frontmatter and filesystem
       workflowState, ok, err := p.reconstructWorkflowState(ctx, baseName, pf, promptPath)
       if err != nil {
           return errors.Wrap(ctx, err, "reconstruct workflow state")
       }
       if !ok {
           // Clone missing for PR workflow — reset to approved
           slog.Info("resetting prompt: clone directory missing for PR workflow",
               "file", filepath.Base(promptPath))
           pf.MarkApproved()
           if err := pf.Save(ctx); err != nil {
               return errors.Wrap(ctx, err, "save prompt after clone missing")
           }
           return nil
       }

       slog.Info("resuming executing prompt", "file", filepath.Base(promptPath), "container", containerName)

       if err := p.executor.Reattach(ctx, logFile, containerName); err != nil {
           return errors.Wrap(ctx, err, "reattach to container")
       }

       slog.Info("reattached container exited", "file", filepath.Base(promptPath))

       // Reload prompt file (state may have changed)
       pf, err = p.promptManager.Load(ctx, promptPath)
       if err != nil {
           return errors.Wrap(ctx, err, "reload prompt after reattach")
       }

       return p.handlePostExecution(ctx, pf, promptPath, title, logFile, workflowState)
   }
   ```

7. **Implement `reconstructWorkflowState(ctx, baseName, pf, promptPath) (*workflowState, bool, error)`** (private method on `processor`):
   - If `p.worktree` is false (direct or in-place workflow): return `&workflowState{}, true, nil` (no clone needed)
   - If `p.worktree` is true (PR/clone workflow):
     - Reconstruct `clonePath := filepath.Join(os.TempDir(), "dark-factory", p.projectName+"-"+baseName)`
     - Check if clonePath exists: `os.Stat(clonePath)` — if error/missing → return `nil, false, nil` (signals reset-to-approved)
     - Reconstruct `branchName`: use `pf.Branch()` if non-empty, else `"dark-factory/" + baseName`
     - Reconstruct `originalDir`: use `os.Getwd()` (dark-factory restarted fresh, so CWD is the original project dir)
     - Return `&workflowState{clonePath: clonePath, branchName: branchName, originalDir: originalDir}, true, nil`
   - Add GoDoc comment.

8. **Regenerate the Processor mock**:
   Run `go generate ./pkg/processor/...` to regenerate `mocks/processor.go` with `ResumeExecuting`.

9. **Wire `ResumeExecuting` into `runner.Run()`** in `pkg/runner/runner.go`:
   After the `resumeOrResetExecuting` call (from prompt 1) and before the parallel `run.CancelOnFirstError` block, add:
   ```go
   // Resume any prompts still in executing state (container was still running on restart)
   if err := r.processor.ResumeExecuting(ctx); err != nil {
       return errors.Wrap(ctx, err, "resume executing prompts")
   }
   ```

10. **Tests in `pkg/executor/executor_test.go` or `executor_internal_test.go`**:
    Add tests for `Reattach`:
    - **Case: container outputs completion report, exits normally** → `Reattach` returns nil, log file contains output
    - **Case: context cancelled** → `Reattach` returns nil (not an error)
    - **Case: docker logs command returns non-zero (container not found)** → `Reattach` returns an error
    Use the existing `commandRunner` mock pattern (already used by `watchForCompletionReport` tests).

11. **Tests in `pkg/processor/processor_internal_test.go`** or `pkg/processor/processor_test.go`:
    Add tests for `ResumeExecuting`:
    - **Case: no executing prompts in queueDir** → returns nil, no executor calls
    - **Case: one executing prompt, container name in frontmatter** → `Reattach` called, `handlePostExecution` called
    - **Case: executing prompt with empty container name** → prompt reset to approved, no Reattach call
    - **Case: PR workflow, clone dir missing** → prompt reset to approved, no Reattach call
    Use counterfeiter mocks for `Executor` and `PromptManager`.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- `Reattach` must never create a new Docker container — it only reads logs from an existing one
- `Reattach` uses `docker logs --follow` which naturally exits when the container stops — this is the primary exit path
- `#nosec G204` comment required on any `exec.CommandContext` with a variable argument
- The `reconstructWorkflowState` method must not do any git operations — purely filesystem and frontmatter inspection
- If `p.worktree` is false, assume the working directory is correct — no clone path check needed
- Re-attach must not cause duplicate git operations: `handlePostExecution` is idempotent for already-completed prompts (it checks log file for report)
- The `Reattach` method on the interface must appear after `Execute` and before `StopAndRemoveContainer`
- Follow existing error-wrapping style: `errors.Wrap(ctx, err, ...)` — never `fmt.Errorf`, never `context.Background()`
- Test coverage ≥80% for changed packages
</constraints>

<verification>
Run `make precommit` — must pass with exit code 0.
Spot-check: `grep -r "Reattach" pkg/executor/` shows interface + implementation.
Spot-check: `grep -r "ResumeExecuting" pkg/processor/` shows interface + implementation.
Spot-check: `grep -r "ResumeExecuting" pkg/runner/` shows the call in Run().
Spot-check: `grep -r "Reattach" mocks/executor.go` shows mock method.
</verification>
