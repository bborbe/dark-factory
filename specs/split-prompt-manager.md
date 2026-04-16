---
status: draft
---

## Summary

- The 19-method `prompt.Manager` interface is split into small, focused interfaces
- Each consumer depends only on the methods it needs
- Counterfeiter mocks are generated per interface, not one giant fake
- No behavioral changes — pure structural refactor
- `prompt.go` (1,229 lines) is split into focused files within the same package

## Problem

`prompt.Manager` has 19 methods — a textbook God Interface. Every consumer depends on the full interface even though most use only 2-3 methods. This makes testing harder (giant mock), hides actual dependencies, and violates the Interface Segregation Principle.

Current consumers and their actual needs:

| Package | Methods Used | Count |
|---------|-------------|-------|
| processor | ListQueued, Load, AllPreviousCompleted, SetStatus, MoveToCompleted, HasQueuedPromptsOnBranch, SetPRURL | 7 |
| runner | ResetExecuting, NormalizeFilenames | 2 |
| runner (oneshot) | ResetExecuting, ListQueued, NormalizeFilenames | 3 |
| server | NormalizeFilenames | 1 |
| status | ListQueued, Title, ReadFrontmatter, HasExecuting | 4 |
| review | ReadFrontmatter, Load, MoveToCompleted, SetStatus, IncrementRetryCount | 5 |
| watcher | NormalizeFilenames | 1 |

Several methods (`SetContainer`, `SetVersion`, `SetBranch`, `Content`, `ResetFailed`) are only called internally within `pkg/prompt` itself and should not be on any exported interface.

## Goal

The finished system has:

- Small, focused interfaces defined at point of use in each consumer package
- The `prompt` package exports a concrete `Manager` struct (or keeps one composed interface for the factory)
- `prompt.go` is split into: `prompt.go` (model), `manager.go` (lifecycle ops), `normalizer.go` (filename logic), `ordering.go` (queue ordering)
- Each consumer's tests use a small, focused Counterfeiter mock

## Non-Goals

- Changing any behavior — this is a pure refactor
- Splitting the `prompt` package into multiple packages
- Changing the factory wiring (factory can still create one Manager and pass it around)

## Desired Behavior

1. Each consumer package defines its own interface with only the methods it uses
2. The `prompt.Manager` struct satisfies all consumer interfaces implicitly (Go structural typing)
3. Counterfeiter generates mocks for each consumer's interface in the consumer's package (or `mocks/`)
4. `pkg/prompt/prompt.go` contains only: `PromptFile`, `Frontmatter`, `PromptStatus`, `SpecList`, `Prompt`, `Load`, `Save`
5. `pkg/prompt/manager.go` contains: `Manager` struct, `NewManager`, lifecycle methods (`SetStatus`, `MoveToCompleted`, `SetPRURL`, etc.)
6. `pkg/prompt/normalizer.go` contains: `NormalizeFilenames`, `scanPromptFiles`, `parseFilename`, `renameInvalidFiles`, `determineRename`, `findNextAvailableNumber`, `performRename`
7. `pkg/prompt/ordering.go` contains: `AllPreviousCompleted`, `HasQueuedPromptsOnBranch`
8. All existing tests pass without modification (or with minimal import changes)

## Constraints

- No behavioral changes — every test must pass
- Factory can still create a single Manager and pass it to multiple consumers
- Keep everything in the `prompt` package — no new packages
- Methods only used internally (`SetContainer`, `SetVersion`, `SetBranch`, `Content`, `ResetFailed`) become unexported or stay on the struct without being on an exported interface

## Failure Modes

| Trigger | Expected Behavior | Recovery |
|---------|-------------------|----------|
| Consumer uses a method not on its interface | Compile error | Add method to the consumer's interface |
| Factory can't pass Manager to consumer | Compile error — type mismatch | Consumer interface must be a subset of Manager's methods |
| Circular import from interface definition | Won't happen — interfaces defined at consumer, not in prompt pkg | N/A |

## Do-Nothing Option

The 19-method Manager continues to work. Cost: every new consumer gets a 19-method mock, tests are harder to write, actual dependencies are hidden, and the 1,229-line file grows further.

## Acceptance Criteria

- [ ] No exported interface in `pkg/prompt` has more than 5 methods
- [ ] Each consumer package defines its own interface with only the methods it calls
- [ ] `pkg/prompt/prompt.go` is under 300 lines
- [ ] `pkg/prompt/manager.go` exists with lifecycle operations
- [ ] `pkg/prompt/normalizer.go` exists with filename logic
- [ ] `make precommit` passes
- [ ] No behavioral changes — all existing tests pass

## Verification

Run `make precommit` — must pass.
