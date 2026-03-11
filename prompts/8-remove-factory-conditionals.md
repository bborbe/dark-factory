---
status: created
created: "2026-03-11T16:45:24Z"
---

<summary>
- Factory helper functions no longer contain conditional branching (if statements)
- The decision about whether to create optional components is made inline in `CreateRunner`
- Two private factory helpers (`createOptionalServer`, `createOptionalReviewPoller`) are removed
- Factory functions are pure composition with zero business logic
</summary>

<objective>
Remove the `createOptionalServer` and `createOptionalReviewPoller` private helper functions from the factory. These contain `if` conditionals which are runtime decisions (business logic) that violate the zero-business-logic factory rule. Inline the conditional logic into `CreateRunner` where it is acceptable as part of the top-level composition root.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/factory/factory.go` — find `createOptionalServer` (~line 55) and `createOptionalReviewPoller` (~line 74). Both are private helpers that wrap a single `if` conditional around a `Create*` call.
Read `CreateRunner` (~line 86) which calls both helpers.
</context>

<requirements>
1. In `pkg/factory/factory.go`, remove the `createOptionalServer` function entirely (~lines 55-72).

2. In `pkg/factory/factory.go`, remove the `createOptionalReviewPoller` function entirely (~lines 74-83).

3. In `CreateRunner`, replace the call to `createOptionalServer(...)` with the inline equivalent:
   ```go
   var srv server.Server
   if cfg.ServerPort > 0 {
       srv = CreateServer(cfg.ServerPort, inboxDir, inProgressDir, completedDir, cfg.Prompts.LogDir, promptManager)
   }
   ```

4. In `CreateRunner`, replace the call to `createOptionalReviewPoller(...)` with the inline equivalent:
   ```go
   var poller review.ReviewPoller
   if cfg.AutoReview {
       poller = CreateReviewPoller(cfg, promptManager)
   }
   ```

5. Pass `srv` and `poller` to `runner.NewRunner(...)` in place of the previous helper calls.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
- Do not change any other factory functions.
- `runner.NewRunner` already handles nil values for `server` and `reviewPoller` — no changes needed there.
</constraints>

<verification>
Run `make precommit` -- must pass.
</verification>
