---
status: draft
spec: [085-auto-inject-hidegit-guidance]
created: "2026-05-21T21:45:00Z"
branch: dark-factory/auto-inject-hidegit-guidance
---

<summary>
- `pkg/factory/factory.go` passes the resolved `hideGit` value to `promptenricher.NewEnricher` at the single call site (line ~954)
- The resolved expression `workflow == config.WorkflowWorktree || hideGit` is passed to the enricher, matching the expression already used for the docker executor at line ~891
- A factory integration test asserts that when `hideGit=true` the emitted prompt contains the fragment, and when `hideGit=false` it does not
- A unit test asserts the enricher and executor receive the same `hideGit` value in both daemon and run modes
- `docs/troubleshooting.md` (or `docs/configuration.md`) gains a short subsection naming the fragment and the `hideGit=true` trigger condition
</summary>

<objective>
Wire the resolved `hideGit` value from `pkg/factory/factory.go:891` into `promptenricher.NewEnricher` so the enricher receives the same `hideGit` value as the docker executor. Add integration tests verifying the fragment appears in emitted prompts when `hideGit=true` and does not appear when `hideGit=false`. Add documentation.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/factory/factory.go` — the `CreateProcessor` function, particularly the `promptenricher.NewEnricher` call site at line ~954 and the `executor.NewDockerExecutor` call site at line ~879-892 where `workflow == config.WorkflowWorktree || hideGit` is passed as the `hideGit` parameter.
Read `pkg/config/config.go` — the `HideGit bool` field and the `Workflow` type with `WorkflowWorktree` value.
Read `pkg/promptenricher/enricher.go` — the `NewEnricher` signature (now with `hideGit bool` as last parameter after this change is deployed).
Read `pkg/promptenricher/enricher_test.go` — the existing test patterns.
Read `docs/troubleshooting.md` — existing structure for adding a new subsection.
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — integration test patterns.
</context>

<requirements>
1. In `pkg/factory/factory.go`, update the `promptenricher.NewEnricher` call at line ~954:
   - Add `workflow == config.WorkflowWorktree || hideGit` as the last argument (matching the expression passed to `executor.NewDockerExecutor` at line ~891)
   - Verify this is the only call site for `promptenricher.NewEnricher` in the file

2. In `pkg/factory/factory_test.go`, add integration tests:
   - `It("emits prompt with hideGit fragment when hideGit=true", ...)` — construct a factory in a mode that sets `hideGit=true`, call the method that produces an enriched prompt, assert the result contains the fragment marker and the instruction to run `make precommit`
   - `It("emits prompt without hideGit fragment when hideGit=false", ...)` — construct a factory with `hideGit=false`, assert the result does not contain the fragment marker
   - `It("enricher and executor receive the same hideGit value in daemon mode", ...)` — construct the factory in daemon mode, assert the `hideGit` value passed to the enricher equals the value passed to the executor
   - `It("enricher and executor receive the same hideGit value in run mode", ...)` — same for run mode

   The integration tests do NOT require running a real Docker container — they test the string output of the enricher as wired into the factory. Use the same mock patterns as existing factory tests.

3. In `docs/troubleshooting.md`, add a new subsection:
   - Section title: `### hideGit guidance fragment`
   - Content: Brief description (2-3 sentences) that when `hideGit=true` is configured, every prompt includes a guidance fragment explaining that `/workspace/.git` appears as a character device by design and instructing the agent to run `make precommit` regardless. State that this is intentional dark-factory behavior and the fragment cannot be disabled separately from `hideGit`.
</requirements>

<constraints>
- The factory must pass the same resolved `hideGit` expression to both `promptenricher.NewEnricher` and `executor.NewDockerExecutor` — both use `workflow == config.WorkflowWorktree || hideGit`
- There is exactly one call site for `promptenricher.NewEnricher` in `pkg/factory/factory.go` — the update must cover it completely
- All existing factory tests must still pass
- Do NOT commit — dark-factory handles git
- Use `errors.Wrapf(ctx, err, "message")` for error wrapping — never `fmt.Errorf`
- The integration test does not require a real Docker container — it tests the string emitted by the wired enricher
</constraints>

<verification>
```bash
go test ./pkg/factory/... -v -run "TestHideGit|TestHideFragment"
make precommit
grep -nE 'hideGit.*guidance|guidance.*hideGit|character device' docs/troubleshooting.md docs/configuration.md
```
</verification>
