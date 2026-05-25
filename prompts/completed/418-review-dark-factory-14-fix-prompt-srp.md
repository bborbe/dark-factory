---
status: completed
summary: 'Split oversized Manager in pkg/prompt/prompt.go into focused types: PromptStatusManager (status mutations), PromptScanner (directory queries), PromptMover (file operations), PromptFileLoader (file I/O); split RollbackMoveToCompleted into PrepareRollback (state prep) and RollbackMove (I/O)'
container: dark-factory-exec-418-review-dark-factory-14-fix-prompt-srp
dark-factory-version: v0.171.1-3-gd94f1fa
created: "2026-05-24T00:00:00Z"
queued: "2026-05-25T14:51:20Z"
started: "2026-05-25T15:04:16Z"
completed: "2026-05-25T15:10:17Z"
---

<summary>
- Split the oversized Manager in pkg/prompt/prompt.go into focused types
- Extracted PromptStatusManager for all status mutations and side effects
- Extracted PromptScanner for directory scanning and queries
- Extracted PromptMover for file movement operations
- Fixed RollbackMoveToCompleted to separate state prep from I/O
</summary>

<objective>
Split the Manager in pkg/prompt/prompt.go into focused types following Single Responsibility Principle.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes:
- `pkg/prompt/prompt.go` — 1330 lines, Manager struct with 19 methods across 5 concerns
- `pkg/prompt/prompt_test.go` — existing test patterns
</context>

<requirements>
1. In `pkg/prompt/prompt.go`, extract focused types from Manager:

   a) `PromptStatusManager` — handles all status mutations: SetStatus, SetContainer, SetVersion, SetPRURL, SetBranch, IncrementRetryCount, MarkApproved, MarkCompleted, MarkFailed, MarkCancelled, MarkCommitting, MarkPendingVerification, StampRejected, SetLastFailReason, CanTransitionTo, IsTerminal

   b) `PromptScanner` — handles directory queries: ListQueued, HasExecuting, FindCommitting, FindPromptStatusInProgress, AllPreviousCompleted, FindMissingCompleted

   c) `PromptMover` — handles file movement: MoveToCompleted, MoveToCancelled, NormalizeFilenames, RollbackMoveToCompleted (split to separate PrepareRollback from I/O)

   d) `PromptFileLoader` — handles file I/O: Load, Content, Title, ReadFrontmatter

2. The existing Manager can remain as a thin facade composing these types, or be replaced entirely.

3. Extract `setStatus` logic into `PromptStatusManager` — it mixes validation (load), state machine (status write), and side effects (timestamps) all in one function.

4. For `RollbackMoveToCompleted`, split into:
   - `PrepareRollback(ctx, completedPath)` — load, set status to Committing, Save
   - `RollbackMove(ctx, completedPath, originalPath)` — MoveFile
</requirements>

<constraints>
- Only change files in this repo
- Do NOT commit — dark-factory handles git
- Keep existing exported interface if other packages depend on Manager
- Use `errors.Wrap`/`errors.Errorf` from `github.com/bborbe/errors` — never `fmt.Errorf` or bare `return err`
</constraints>

<verification>
make precommit
</verification>
