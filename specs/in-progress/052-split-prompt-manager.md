---
status: verifying
approved: "2026-04-16T19:30:10Z"
generating: "2026-04-16T19:46:40Z"
prompted: "2026-04-16T19:56:38Z"
verifying: "2026-04-16T23:09:23Z"
branch: dark-factory/split-prompt-manager
---

## Summary

Every consumer of `pkg/prompt` today depends on the full 19-method `Manager` interface, even when it calls only one or two methods. This spec replaces that single wide interface with small interfaces defined at each point of use. The `prompt` package exports a concrete `Manager` struct that satisfies every consumer's interface implicitly through Go structural typing. Counterfeiter generates a per-consumer fake, so tests mock only what they exercise.

## Problem

`prompt.Manager` is a textbook God Interface: 19 methods covering prompt loading, frontmatter parsing, queue ordering, filename normalization, branch queries, and retry-count bookkeeping. Seven consumer packages depend on it:

| Consumer | Methods actually called | Count |
|---|---|---|
| processor | `ListQueued`, `Load`, `AllPreviousCompleted`, `SetStatus`, `MoveToCompleted`, `HasQueuedPromptsOnBranch`, `SetPRURL` | 7 |
| runner | `ResetExecuting`, `NormalizeFilenames` | 2 |
| runner (oneshot) | `ResetExecuting`, `ListQueued`, `NormalizeFilenames` | 3 |
| server | `NormalizeFilenames` | 1 |
| status | `ListQueued`, `Title`, `ReadFrontmatter`, `HasExecuting` | 4 |
| review | `ReadFrontmatter`, `Load`, `MoveToCompleted`, `SetStatus`, `IncrementRetryCount` | 5 |
| watcher | `NormalizeFilenames` | 1 |

No consumer uses more than seven methods, but every consumer's counterfeiter fake carries all 19. Tests wire 19-method mocks even to exercise one call. Real dependencies are hidden behind the god interface, so it's impossible to tell at a glance what the server actually needs from `prompt` (it's one method). Several methods (`SetContainer`, `SetVersion`, `SetBranch`, `Content`, `ResetFailed`) are only called inside `pkg/prompt` itself and should not appear on any exported interface at all.

## Goal

After this work, each consumer's test file wires a small fake that contains only the methods the consumer calls. Reading a consumer package tells the reader immediately what it needs from `prompt`. The `prompt` package exposes a concrete `*Manager` that the factory constructs once and passes around; every consumer accepts its own narrow interface, which `*Manager` satisfies by structural typing.

## Assumptions

- Go's structural typing makes interface-at-point-of-use a zero-cost pattern — the concrete `*Manager` automatically satisfies every consumer interface that is a subset of its methods.
- Counterfeiter generates fakes per interface without conflicts, using the package-prefix naming convention the project already uses.
- The factory currently constructs exactly one `Manager` and passes it to every consumer; this does not change.
- No consumer relies on runtime interface assertion (e.g., `m.(interface{ Foo() })`) on the prompt manager. Verified by grep for `prompt.Manager` type assertions.
- The five internal-only methods (`SetContainer`, `SetVersion`, `SetBranch`, `Content`, `ResetFailed`) have no consumers outside `pkg/prompt` itself. Verified by grep of each method across the repo.

## Non-Goals

- Changing any prompt lifecycle behavior.
- Splitting `pkg/prompt` into multiple packages.
- Changing how the factory constructs the manager.
- Introducing DI frameworks or runtime interface assembly.
- Renaming `Manager` or changing its concrete method signatures.

## Desired Behavior

1. `pkg/prompt` exports a concrete `*Manager` struct (constructed via an existing `NewManager` factory) whose public methods are exactly today's 19 methods minus any moved to unexported receivers in step 3.
2. The `pkg/prompt` package does NOT export a single `Manager` interface covering all 19 methods. (A small `Manager` interface for the factory is acceptable only if it is the same interface one consumer uses — not a god interface preserved under another name.)
3. Every consumer package defines its own interface declaring only the methods it calls. The interface lives where the consumer uses it (e.g., `pkg/processor/prompt_manager.go`), not in `pkg/prompt`.
4. Each consumer's tests use a counterfeiter fake generated from that consumer's interface, sized to match.
5. Methods called only from within `pkg/prompt` itself (`SetContainer`, `SetVersion`, `SetBranch`, `Content`, `ResetFailed`) become unexported or remain as struct methods not part of any consumer interface.
6. The factory passes `*Manager` to each consumer. Each consumer's constructor accepts its own narrow interface type; `*Manager` satisfies it implicitly.

## Constraints

- No behavioral changes. Every existing test passes with at most mechanical changes (imports, mock types).
- Factory wiring continues to construct one `Manager` per project.
- No new packages.
- New code follows project conventions: `github.com/bborbe/errors` wrapping, Ginkgo/Gomega, Counterfeiter fakes with package prefix (e.g., `processor_prompt_manager.go`), `libtime`.
- `pkg/prompt`'s public API does not shrink in a way that breaks any current caller — methods stay accessible on the concrete struct even if the god interface is gone.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Consumer calls a method not declared on its local interface | Compile error | Add the method to the consumer's interface |
| Two consumers need overlapping method sets | Each defines its own interface (overlap is fine) | No action — duplication of method signatures across consumer interfaces is intentional |
| A method turns out to be called from outside `pkg/prompt` but was marked internal-only | Compile error at the external caller | Restore the method to exported status and add it to the caller's consumer interface |
| Counterfeiter fake name collides with existing fake | Build fails | Use package-prefixed `--fake-name` (e.g., `ProcessorPromptManager`) per project convention |
| Factory wiring compiles but at runtime a consumer receives a `nil` manager | Existing nil-check or panic on first call | Covered by existing factory tests asserting constructor arguments |

## Do-Nothing Option

Every new consumer of `prompt` gets handed a 19-method mock and has to understand all 19 to write a test. New features that need one new method bloat the single interface by one more. The server package (1 method) looks identical to the processor package (7 methods) at the type level, so reviewers and static analysis can't tell which is actually coupled to what. `pkg/prompt/prompt.go` keeps growing as the single home for every concern.

## Acceptance Criteria

- [ ] `pkg/prompt` exports no single interface covering all 19 methods. If any `Manager` interface remains in `pkg/prompt`, it is used by at most one consumer and is that consumer's narrow interface (not a god interface under a new name).
- [ ] Each of the seven consumer packages defines a local interface matching its column in the Problem table. No consumer imports a 19-method god interface.
- [ ] Each consumer's tests use a counterfeiter fake of its local interface, with method count equal to that consumer's column.
- [ ] `SetContainer`, `SetVersion`, `SetBranch`, `Content`, `ResetFailed` are either unexported or not part of any consumer interface.
- [ ] Factory wiring is unchanged at the call-site level: one `Manager` constructed, passed to each consumer.
- [ ] `make precommit` passes.
- [ ] All existing tests pass. Test setup code for each consumer references that consumer's small fake instead of the 19-method fake.

## Verification

```
cd ~/Documents/workspaces/dark-factory
make precommit
```

Sanity check the interface shape:

```
grep -rh "type.*interface {" pkg/prompt | wc -l      # should be small; no 19-method interface
grep -rh "counterfeiter:generate" pkg | wc -l         # consumer fakes exist per narrow interface
```
