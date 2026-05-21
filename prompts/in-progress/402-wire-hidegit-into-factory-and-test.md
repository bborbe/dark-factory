---
status: committing
spec: [085-auto-inject-hidegit-guidance]
summary: Wired resolved hideGit expression to promptenricher.NewEnricher in factory.CreateProcessor, added integration tests verifying fragment emission, and documented the behavior in troubleshooting.md
container: dark-factory-exec-402-wire-hidegit-into-factory-and-test
dark-factory-version: v0.164.0
created: "2026-05-21T21:45:00Z"
queued: "2026-05-21T21:43:03Z"
started: "2026-05-21T22:06:23Z"
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
Read `pkg/factory/factory.go` — the `CreateProcessor` function, particularly the `promptenricher.NewEnricher` call site at line 954 (sole site) and the `executor.NewDockerExecutor` call site at line 879-892 where `workflow == config.WorkflowWorktree || hideGit` is passed as the `hideGit` parameter at line 891. Both `workflow` and `hideGit` are local parameters of `CreateProcessor` and are in scope at line 954.
Read `pkg/config/config.go` — the `HideGit bool` field and the `Workflow` type with `WorkflowWorktree` value.
Read `pkg/promptenricher/enricher.go` — the `NewEnricher` signature (with `hideGit bool` as last parameter — the sibling prompt `085-add-hidegit-guidance-to-enricher.md` adds this parameter and updates all other call sites to pass `false`; this prompt replaces the `false` at `factory.go:954` with the resolved expression).
Read `pkg/promptenricher/enricher_test.go` — the existing test patterns.
Read `docs/troubleshooting.md` — existing structure for adding a new subsection.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — integration test patterns. Use the in-container absolute path; do NOT use `~/.claude/...` (host-relative paths do not resolve inside the YOLO container).
</context>

<requirements>
1. In `pkg/factory/factory.go`, update the `promptenricher.NewEnricher` call at line 954:
   - Replace the literal `false` (added by the sibling prompt as a placeholder) with the resolved expression `workflow == config.WorkflowWorktree || hideGit`
   - The expression must be observably identical to the one already passed to `executor.NewDockerExecutor` at line 891 — they are sourced from the same local variables in `CreateProcessor`'s scope

2. In `pkg/factory/factory_test.go`, add a Ginkgo `Describe("hideGit fragment wiring", ...)` block with TWO tests (both deterministic and container-free):

   a) **Wiring contract test (source-code grep)** — proves the enricher and executor receive the SAME resolved expression in factory.go. Read `pkg/factory/factory.go` as a string at test time and assert: `strings.Count(factoryGoContent, "workflow == config.WorkflowWorktree || hideGit")` is exactly 2. One occurrence is the executor wiring at line 891, the other is the enricher wiring at line 954. If a future refactor diverges them, this test fails. This is the cheapest defense against the failure-modes table rows 4-5 (wiring mismatch).

   b) **Behavior contract test (direct enricher construction)** — constructs `promptenricher.NewEnricher` directly in the test with the same resolved expression and asserts:
   - With `hideGit=true`: `Enrich(ctx, "PROMPT_BODY")` output contains the substring `character device` AND the substring `hideGit=true active`.
   - With `hideGit=false`: `Enrich(ctx, "PROMPT_BODY")` output does NOT contain `hideGit=true active`.
   - This test duplicates the resolved expression (intentional — the wiring test above catches drift between factory.go's two call sites).

   Do NOT try to instantiate `factory.CreateProcessor` and inspect its emitted enriched prompt — the `Processor` interface does not expose the enricher's output. The wiring test + behavior test above together satisfy spec AC line 106 ("integration test asserts both branches; test does NOT require running a real container").

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
go test ./pkg/factory/... -v
make precommit
grep -nE 'hideGit.*guidance|guidance.*hideGit|character device' docs/troubleshooting.md
grep -c 'workflow == config.WorkflowWorktree || hideGit' pkg/factory/factory.go  # must be 2
```
Note: do NOT use `-run "TestHideGit|TestHideFragment"` — the package uses Ginkgo. Top-level `Test*` is `TestFactory` (the Ginkgo suite entry). A `-run` regex that doesn't match `TestFactory` silently runs zero tests and exits 0 (false green). Run the full suite with `-v` and rely on the new `Describe` block's name being present in output.
</verification>
