---
status: completed
summary: 'Refactored factory pattern: extracted WorkflowExecutorProvider interface, created pure notifier sub-helpers, exported provider creation functions, and updated CreateProcessor signature'
container: dark-factory-exec-429-review-dark-factory-8-fix-factory-pattern
dark-factory-version: v0.171.1-3-gd94f1fa
created: "2026-05-24T00:00:00Z"
queued: "2026-05-25T14:51:20Z"
started: "2026-05-25T19:43:33Z"
completed: "2026-05-25T19:56:08Z"
---

<summary>
- Lifted conditional business logic out of factory.go into main.go
- createProviderDeps: if/else routing moved to main.go Run()
- CreateWorkflowExecutor: switch dispatch moved to WorkflowExecutorProvider interface
- CreateNotifier: nested if conditionals moved to main.go Run()
- EffectiveMaxContainers: conditional moved to main.go Run()
</summary>

<objective>
Fix factory pattern violations where business logic (if/else, switch) lives in factory.go instead of main.go.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-factory-pattern.md` for factory pattern rules.

Files to read before making changes:
- `pkg/factory/factory.go` — lines ~221 (createProviderDeps), ~787 (CreateWorkflowExecutor), ~994 (CreateNotifier), ~76 (EffectiveMaxContainers)
- `main.go` — to understand where to move the conditionals
- `pkg/processor/workflow_executor_provider.go` — if it exists, or where to create it
</context>

<requirements>
1. Fix `createProviderDeps` (line ~221): Remove the if/else. Keep two pure sub-helpers `createGitHubProviderDeps` and `createBitbucketProviderDeps`. Move the provider selection switch/if to `main.go` where the factory is called.

2. Fix `CreateWorkflowExecutor` (line ~787): Extract the switch into a `WorkflowExecutorProvider` interface. Create a `workflowExecutorProvider` struct implementing `Get(ctx, workflow)`. The factory returns the provider; callers call `provider.Get(ctx, workflow)`.

3. Fix `CreateNotifier` (line ~994): Remove the nested if conditionals. Keep pure sub-helpers `CreateTelegramNotifier`, `CreateDiscordNotifier`, `CreateNoopNotifier`. Move the conditional logic to `main.go`.

4. Fix `EffectiveMaxContainers` (line ~76): Move the if/else logic to `main.go` at each call site. The factory function can remain as a simple max() if needed, but the business rule about per-project vs global should be in main.go.

5. Update all callers in main.go to use the new approach.
</requirements>

<constraints>
- Only change files in this repo
- Do NOT commit — dark-factory handles git
- Factory functions must have zero business logic (no if/else/switch/loops)
- Constructors return interfaces, not concrete types
</constraints>

<verification>
make precommit
</verification>
