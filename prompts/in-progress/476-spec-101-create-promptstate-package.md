---
status: approved
spec: ["101"]
created: "2026-06-26T08:00:00Z"
queued: "2026-06-26T08:00:41Z"
---

<summary>

- Introduces a single new package that becomes the one place in the codebase allowed to decide "what state is this prompt actually in?" from its observable inputs (where the file lives, its frontmatter status, its container name, and whether Docker says the container is alive).
- Declares the seven canonical prompt states (plus one error-only sentinel for unrecognised inputs) as in-memory values, separate from the on-disk status strings — the on-disk format is untouched.
- Encodes the allowed transitions between those states in one transition table, with a query that answers "is this move legal?" and returns false for anything not in the table.
- Captures the four hard-won recovery rules from recent bug fixes (resume keeps an alive prompt executing, a gone container aborts it, committing advances to completed, and the half-state where the file is already in the completed dir wins as completed) as locked regression tests.
- Pure, dependency-free decision logic: the same inputs always produce the same state, safe to call from many places at once.
- Ships only the contract and its tests — no existing code is rewired yet (that is later prompts).

</summary>

<objective>
Create a new leaf package `pkg/promptstate` that owns the interpretation of a prompt's four observable inputs `(filesystem location, frontmatter status, container field, docker state)` and decides the authoritative current `State`. It exposes `State` (seven canonical values plus a `StateUnknown` sentinel), `IsValidTransition(from, to State) bool` backed by a transition table, and `InterpretTuple(...) State` as a pure function. It ships with regression tests locking the four recovery edges and the full transition table. No existing consumer is rewired in this prompt — this prompt only defines the contract that prompts 2-5 build on.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` first, then `/workspace/CLAUDE.md`.

Read these coding-plugin docs (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-enum-type-pattern.md` — string-enum convention: plural collection type `States`, `Available*`, `Validate(ctx)`, `Contains()`, `String()`. Apply this to the new `State` type.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-patterns.md` — interface/constructor/struct conventions, error wrapping with `bborbe/errors`, GoDoc style.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo/Gomega suite files, external test packages (`package_test`), coverage >= 80%, table-driven tests.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-licensing-guide.md` — BSD header requirement.
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — changelog entry format.

Read the parent spec end-to-end:
- `/workspace/specs/in-progress/101-extract-unified-prompt-state-machine.md` — especially Desired Behavior items 1, 2, 7; Constraints; Failure Modes rows 2, 3, 4, 5, 7; Acceptance Criteria 1, 2, 11, 13.

Read these source files END-TO-END before writing (full reads, not skims) so the new package mirrors existing tuple-reading rules exactly:
- `/workspace/pkg/prompt/prompt.go` lines 48-175 — the on-disk `PromptStatus` enum (11 values), `AvailablePromptStatuses`, `promptTransitions` table, `CanTransitionTo`, `Validate`, `IsTerminal`, `IsPreExecution`, `IsActive`. The new `State` is an IN-MEMORY type DISTINCT from `PromptStatus`; `PromptStatus` stays the on-disk storage type (spec Non-goal: "Do NOT change the public surface of `pkg/prompt.PromptFile`").
- `/workspace/pkg/runner/lifecycle.go` `resumeOrResetExecutingEntry` (around line 245) — the canonical resume/reset rule: if `frontmatter.Status == executing` and the container `IsRunning`, the prompt STAYS executing; if the container is GONE, it is reset to approved (this reset is the conceptual `StateAborted`). On `IsRunning` error (docker unavailable) it REFUSES to reset and propagates the error.
- `/workspace/pkg/committingrecoverer/recoverer.go` `Recover` (around line 94, esp. lines 131-145) — the half-state rule: when `frontmatter.Status == committing` BUT the file is already inside the completed dir (`filepath.Dir(promptPath) == completedDir`), the prompt is treated as COMPLETED (location wins; this is the PR #30 fix the spec references). Otherwise committing advances to completed normally.
- `/workspace/pkg/cancellationwatcher/watcher.go` around line 99 — `frontmatter.Status == cancelled` → `StateCancelled`. This consumer also tolerates docker being unavailable (it does not block on docker).
- `/workspace/pkg/queuescanner/scanner.go` around line 296 and 481 — `status.IsPreExecution()` gate and the `pending_verification` check.
- `/workspace/pkg/prompt/prompt_test.go` lines 28-90 — the Ginkgo `Describe`/`It` style this repo uses for enum tests; mirror it.
- `/workspace/pkg/prompt/prompt_suite_test.go` — the suite-runner pattern (`TestX` + `RunSpecs`).

VERIFIED FACTS (do not re-derive):
- Module path is `github.com/bborbe/dark-factory` (from `go.mod` line 1). The new package import path is `github.com/bborbe/dark-factory/pkg/promptstate`.
- The on-disk `PromptStatus` constants and their string values (from `pkg/prompt/prompt.go`): `IdeaPromptStatus="idea"`, `DraftPromptStatus="draft"`, `ApprovedPromptStatus="approved"`, `ExecutingPromptStatus="executing"`, `CompletedPromptStatus="completed"`, `FailedPromptStatus="failed"`, `InReviewPromptStatus="in_review"`, `PendingVerificationPromptStatus="pending_verification"`, `CancelledPromptStatus="cancelled"`, `CommittingPromptStatus="committing"`, `RejectedPromptStatus="rejected"`.
- The existing on-disk transition table `promptTransitions` (pkg/prompt/prompt.go ~line 117) is the authority for which status→status moves are legal on disk. The new in-memory `State` transition table must be CONSISTENT with it for the seven canonical states (it may be a focused projection; it must NOT contradict it).
- BSD license header is the 3-line block at `pkg/prompt/prompt.go` lines 1-3 — copy it verbatim onto every new `.go` file.
- `pkg/prompt.PromptStatus` already implements `CanTransitionTo(ctx, target)` (line 140) and `Validate(ctx)` (line 109) — the new package may IMPORT `pkg/prompt` to map `PromptStatus → State`. Importing `pkg/prompt` does NOT create a cycle: `pkg/prompt` does not import `pkg/promptstate`.

OPEN QUESTION for the human reviewer (leave as a Go doc comment near `StateAborted`): the spec lists seven canonical states including `Aborted`, but there is NO on-disk `aborted` status value. `StateAborted` is the INTERPRETED state for "frontmatter says `executing` but the container is gone" — the case `resumeOrResetExecutingEntry` handles by resetting to `approved`. It has no 1:1 on-disk string; it is an interpreted/transient state. Document this explicitly so a future reader does not look for an `aborted` frontmatter value.

</context>

<requirements>

## 1. Create `pkg/promptstate/state.go` — the `State` enum

Create `/workspace/pkg/promptstate/state.go` with the BSD header (copy verbatim from `pkg/prompt/prompt.go` lines 1-3), `package promptstate`, and the `State` string enum. Declare EXACTLY these constants so the spec AC `grep -cE '^\s*State(Approved|Executing|Committing|Completed|Cancelled|PendingVerification|Aborted)\b'` returns >= 7, plus the error-only sentinel:

```go
package promptstate

// State is the in-memory authoritative state of a prompt, derived by InterpretTuple
// from the four observable inputs. It is DISTINCT from pkg/prompt.PromptStatus, which
// is the on-disk storage type; the 1:1 mapping between them lives in this package.
type State string

const (
	// StateApproved — prompt is queued, not yet executing.
	StateApproved State = "approved"
	// StateExecuting — prompt is running in a container (or being resumed into one).
	StateExecuting State = "executing"
	// StateCommitting — container succeeded; git commit pending.
	StateCommitting State = "committing"
	// StateCompleted — prompt finished and (re)located in the completed dir.
	StateCompleted State = "completed"
	// StateCancelled — prompt was cancelled before or during execution.
	StateCancelled State = "cancelled"
	// StatePendingVerification — prompt awaits post-review verification.
	StatePendingVerification State = "pending_verification"
	// StateAborted is the INTERPRETED state for "frontmatter says executing but the
	// container is gone" — the case the daemon resolves by resetting to approved.
	// It has NO on-disk status string; it is a transient/interpreted state only.
	// (See OPEN QUESTION in the prompt context.)
	StateAborted State = "aborted"
	// StateUnknown is the error-only sentinel returned by InterpretTuple when the
	// frontmatter status string is not one InterpretTuple recognises
	// (spec Failure Mode row 2). Callers log ERROR unknown_prompt_status and surface
	// the prompt as "unknown"; the daemon does NOT silently coerce.
	StateUnknown State = "unknown"
)
```

Add, following `go-enum-type-pattern.md`:
- `type States []State`
- `var AvailableStates = States{StateApproved, StateExecuting, StateCommitting, StateCompleted, StateCancelled, StatePendingVerification, StateAborted}` (the seven CANONICAL states; `StateUnknown` is the sentinel and is deliberately NOT in `AvailableStates`).
- `func (s State) String() string { return string(s) }`
- `func (ss States) Contains(s State) bool` — linear membership check.
- `func (s State) Validate(ctx context.Context) error` — returns a `bborbe/errors`-wrapped `validation.Error` (mirror `pkg/prompt/prompt.go` `Validate` at line 109; import `github.com/bborbe/validation` and `github.com/bborbe/errors`) when `s` is not in `AvailableStates`. `StateUnknown` is INVALID per `Validate` (it is a sentinel, not a canonical state).

## 2. Create `pkg/promptstate/transitions.go` — the transition table + `IsValidTransition`

Create `/workspace/pkg/promptstate/transitions.go` with the BSD header, `package promptstate`, and a transition table consistent with the on-disk `promptTransitions` in `pkg/prompt/prompt.go`. Encode (do NOT contradict the on-disk table; this is a projection onto the seven canonical states plus the `StateAborted` recovery sink):

```go
// stateTransitions is the single source of truth for allowed in-memory state moves.
// It is consistent with pkg/prompt.promptTransitions; add one row here to enable a
// new transition. Every State in AvailableStates MUST appear as a source key or as a
// sink in at least one row (enforced by TestTransitionTableCoversAllStates).
var stateTransitions = map[State][]State{
	StateApproved:            {StateExecuting, StateCancelled},
	StateExecuting:           {StateCommitting, StateCancelled, StateAborted},
	StateCommitting:          {StateCompleted},
	StateAborted:             {StateApproved}, // recovery: container gone → reset to approved
	StatePendingVerification: {StateCompleted},
	// StateCompleted and StateCancelled are terminal sinks: no outgoing rows.
}
```

Add:
```go
// IsValidTransition reports whether moving from -> to is an allowed state transition.
// Transitions not in the table (including any involving StateUnknown) return false.
func IsValidTransition(from, to State) bool {
	for _, allowed := range stateTransitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}
```

Decision already made — do NOT add an `InReview` source row: `in_review → pending_verification` lives on the on-disk table but `in_review` is NOT one of the seven canonical `State` values the spec enumerates, so it stays out of this in-memory projection. `StatePendingVerification` is reached via the on-disk path, not modeled as an in-memory transition source here.

## 3. Create `pkg/promptstate/interpret.go` — `InterpretTuple`

Create `/workspace/pkg/promptstate/interpret.go` with the BSD header, `package promptstate`. Define the input types and the pure function:

```go
// Location is where the prompt file currently lives — the half-state discriminator.
type Location string

const (
	LocationInProgress Location = "in_progress" // prompts/in-progress/
	LocationCompleted  Location = "completed"   // prompts/completed/
)

// DockerState is the liveness of the prompt's container as reported by Docker.
type DockerState string

const (
	DockerStateRunning     DockerState = "running"
	DockerStateStopped     DockerState = "stopped"
	DockerStateUnavailable DockerState = "unavailable" // daemon unreachable / probe errored
)
```

Then `InterpretTuple`:

```go
// InterpretTuple is the ONLY function in the codebase allowed to decide the
// authoritative current State from the four observable inputs. It is pure: same
// inputs always yield the same State; it has no shared mutable state and never
// blocks on docker. status is the raw on-disk frontmatter status string.
//
// Rules (each locked by a regression test in interpret_test.go):
//   - executing + container running         -> StateExecuting   (resume keeps it executing)
//   - executing + container gone (stopped)  -> StateAborted     (reset-to-approved path)
//   - executing + docker unavailable        -> StateExecuting   (refuse to coerce; file truth wins)
//   - committing + file in completed dir    -> StateCompleted   (location wins; PR #30 half-state)
//   - committing + file in in-progress dir  -> StateCommitting
//   - cancelled                              -> StateCancelled
//   - pending_verification                   -> StatePendingVerification
//   - approved                               -> StateApproved
//   - completed                              -> StateCompleted
//   - any unrecognised status string         -> StateUnknown
func InterpretTuple(location Location, status prompt.PromptStatus, container string, dockerState DockerState) State
```

Implementation notes:
- Import `prompt "github.com/bborbe/dark-factory/pkg/prompt"` for the `PromptStatus` constants.
- Switch on `status`:
  - `prompt.ApprovedPromptStatus` -> `StateApproved`
  - `prompt.ExecutingPromptStatus` -> if `dockerState == DockerStateStopped` return `StateAborted`; otherwise (`DockerStateRunning` OR `DockerStateUnavailable`) return `StateExecuting` (docker-unavailable must NOT coerce to aborted — spec Failure Mode row 7 / "Docker daemon unavailable" row: `InterpretTuple` returns the state computed from the other inputs and never blocks).
  - `prompt.CommittingPromptStatus` -> if `location == LocationCompleted` return `StateCompleted` (half-state, location wins); else `StateCommitting`.
  - `prompt.CompletedPromptStatus` -> `StateCompleted`
  - `prompt.CancelledPromptStatus` -> `StateCancelled`
  - `prompt.PendingVerificationPromptStatus` -> `StatePendingVerification`
  - `default` (any other string, including `idea/draft/failed/in_review/rejected/unknown`) -> `StateUnknown`. (The spec's canonical set is the seven execution-lifecycle states; pre-execution and review states that are not in the seven map to `StateUnknown` from this function's perspective. Document this in the GoDoc so a reader knows `draft`/`failed` are intentionally `StateUnknown` here, not a bug.)
- `container` is accepted for signature completeness and future use (the half-state and resume rules key on `location`+`dockerState`, not the container string itself). Keep the parameter; reference it in a one-line comment so the linter does not flag it (e.g. `_ = container // reserved: callers pass the frontmatter container name`). Decision already made — keep the parameter (the spec's tuple is four-wide); do NOT drop it.

## 4. Create `pkg/promptstate/doc.go`

Create `/workspace/pkg/promptstate/doc.go` with the BSD header and a `// Package promptstate ...` GoDoc comment describing the package as the single owner of prompt-state interpretation. Mirror `pkg/prompt/doc.go` style.

## 5. Create the test suite and regression tests

Create `/workspace/pkg/promptstate/promptstate_suite_test.go` mirroring `pkg/prompt/prompt_suite_test.go` (BSD header, `package promptstate_test`, `func TestPromptstate(t *testing.T)` calling `RegisterFailHandler(Fail)` and `RunSpecs(t, "Promptstate Suite")`).

Create `/workspace/pkg/promptstate/interpret_test.go` and `/workspace/pkg/promptstate/transitions_test.go` as EXTERNAL test packages (`package promptstate_test`), importing `promptstate "github.com/bborbe/dark-factory/pkg/promptstate"` and `prompt "github.com/bborbe/dark-factory/pkg/prompt"`.

5.1. The spec AC-11 requires `go test ./pkg/promptstate/... -run 'Recover|Resume|HalfState|Cancel' -v` to list >= 4 test functions, all PASS. Ginkgo `It` blocks are NOT discoverable by `-run`. Therefore add FOUR plain-`testing` functions (they coexist with Ginkgo in the same package) named so the `-run` regex matches each — locking the four recovery edges:

```go
func TestResumeExecutingStaysExecuting(t *testing.T) {
	// executing + container running -> StateExecuting
	got := promptstate.InterpretTuple(promptstate.LocationInProgress, prompt.ExecutingPromptStatus, "c1", promptstate.DockerStateRunning)
	if got != promptstate.StateExecuting {
		t.Fatalf("want StateExecuting, got %s", got)
	}
}

func TestRecoverExecutingToAborted(t *testing.T) {
	// executing + container gone -> StateAborted (reset-to-approved path)
	got := promptstate.InterpretTuple(promptstate.LocationInProgress, prompt.ExecutingPromptStatus, "c1", promptstate.DockerStateStopped)
	if got != promptstate.StateAborted {
		t.Fatalf("want StateAborted, got %s", got)
	}
}

func TestRecoverCommittingToCompleted(t *testing.T) {
	// committing + file in in-progress dir -> StateCommitting (normal commit pending)
	got := promptstate.InterpretTuple(promptstate.LocationInProgress, prompt.CommittingPromptStatus, "c1", promptstate.DockerStateStopped)
	if got != promptstate.StateCommitting {
		t.Fatalf("want StateCommitting, got %s", got)
	}
}

func TestHalfStateCommittingInCompletedDir(t *testing.T) {
	// committing + file ALREADY in completed dir -> StateCompleted (location wins, PR #30)
	got := promptstate.InterpretTuple(promptstate.LocationCompleted, prompt.CommittingPromptStatus, "c1", promptstate.DockerStateStopped)
	if got != promptstate.StateCompleted {
		t.Fatalf("want StateCompleted, got %s", got)
	}
}

func TestCancelInterpretsCancelled(t *testing.T) {
	got := promptstate.InterpretTuple(promptstate.LocationInProgress, prompt.CancelledPromptStatus, "", promptstate.DockerStateUnavailable)
	if got != promptstate.StateCancelled {
		t.Fatalf("want StateCancelled, got %s", got)
	}
}
```

5.2. Add a plain-`testing` function `TestTransitionTableCoversAllStates(t *testing.T)` (spec Failure Mode row: "New state added to State without a corresponding transition table entry"): assert every `State` in `promptstate.AvailableStates` appears as a source key OR as a sink in `stateTransitions`. Because `stateTransitions` is unexported, drive this through the EXPORTED surface: build the expected-edge set from `AvailableStates` and assert `IsValidTransition` returns true for each known-good edge and false for a representative bad edge. Specifically assert: `IsValidTransition(StateApproved, StateExecuting)` is true; `IsValidTransition(StateCommitting, StateCompleted)` is true; `IsValidTransition(StateExecuting, StateAborted)` is true; `IsValidTransition(StateAborted, StateApproved)` is true; and a representative DISALLOWED edge `IsValidTransition(StateCompleted, StateExecuting)` is false; `IsValidTransition(StateUnknown, StateApproved)` is false.

5.3. Add a plain-`testing` function `TestInterpretDockerUnavailableNeverCoerces(t *testing.T)` (spec Failure Mode "Docker daemon unavailable"): assert `InterpretTuple(LocationInProgress, ExecutingPromptStatus, "c1", DockerStateUnavailable) == StateExecuting` (NOT `StateAborted` — docker-unavailable must not coerce).

5.4. Add a plain-`testing` function `TestInterpretUnknownStatus(t *testing.T)` (spec Failure Mode row 2): assert `InterpretTuple(LocationInProgress, prompt.PromptStatus("bogus-status"), "", DockerStateUnavailable) == StateUnknown`. Also assert `prompt.DraftPromptStatus` and `prompt.FailedPromptStatus` map to `StateUnknown` from this function (they are outside the seven canonical execution states).

5.5. Add a plain-`testing` function `TestInterpretConcurrent(t *testing.T)` (spec Failure Mode "Concurrent calls"): launch N goroutines each calling `InterpretTuple` with a different input and asserting the expected result; use a `sync.WaitGroup`. The `-race` flag (when enabled) must report no data race since the function is pure.

5.6. Add Ginkgo `Describe` blocks (any focused names) that table-test `InterpretTuple` across ALL rules listed in requirement 3's GoDoc (one `It` per row) and `IsValidTransition` across the full table (every allowed edge true, a representative disallowed edge false). Use Gomega `Expect(...).To(Equal(...))`. These provide the readable coverage; the plain `Test*` functions above satisfy the AC `-run` greps.

Coverage for `pkg/promptstate` MUST be >= 80% (trivial given the package is pure logic).

## 6. CHANGELOG

Append to the `## Unreleased` section of `/workspace/CHANGELOG.md` (create the section under the intro block if absent; do NOT disturb existing version sections) ONE bullet:

```
- feat: add pkg/promptstate — single owner of prompt-state interpretation with State enum, IsValidTransition transition table, and pure InterpretTuple(location, status, container, dockerState) (spec 101 prompt 1)
```

</requirements>

<constraints>

- Frontmatter values on disk MUST NOT change. `InterpretTuple` is a READER of the raw status string; it writes nothing and changes no on-disk format (spec Constraint).
- `pkg/prompt.PromptFile.Status` keeps its current type and on-disk YAML tag. The 1:1 mapping `PromptStatus ↔ State` lives INSIDE `pkg/promptstate` (spec Constraint). Do NOT add a `State` field to `PromptFile` or `Frontmatter`.
- `pkg/promptstate` imports `pkg/prompt` (for `PromptStatus` constants), `bborbe/errors`, `bborbe/validation`, and stdlib only. NO new third-party dependencies (spec Constraint). It must NOT import any of the five consumer packages (that would invite a cycle later).
- `InterpretTuple` is PURE — no globals, no I/O, no docker calls, no logging. It NEVER blocks on docker; the caller resolves `DockerState` and passes it in (spec Failure Mode "Docker daemon unavailable").
- BSD-style license header on every new `.go` file (spec Constraint).
- Errors wrapped with `bborbe/errors` (`errors.Wrapf(ctx, ...)`) — never `fmt.Errorf`, never `context.Background()` in pkg/ non-test code (project rule). Only `Validate` has an error path here.
- Counterfeiter mocks regenerate cleanly: this package adds NO new interface in this prompt (it is pure functions + an enum), so `make generate` produces no new mocks. If a later reviewer expects an interface, that is prompt 2's concern, not this one.
- Do NOT rewire any existing consumer in this prompt — this prompt is purely additive (the five consumers migrate in prompts 2 and 3).
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.

</constraints>

<verification>

```bash
cd /workspace

# AC 1 — required symbols exist
grep -nE '^(type State|func IsValidTransition|func InterpretTuple)' pkg/promptstate/*.go
# expected: >= 3 lines

# AC 2 — all seven canonical state constants declared
grep -cE '^\s*State(Approved|Executing|Committing|Completed|Cancelled|PendingVerification|Aborted)\b' pkg/promptstate/*.go
# expected: >= 7

# AC 11 — the four recovery-edge tests are discoverable and pass
go test ./pkg/promptstate/... -run 'Recover|Resume|HalfState|Cancel' -v 2>&1 | grep -E '^(=== RUN|--- PASS|PASS|ok)'
# expected: >= 4 RUN/PASS lines, all PASS

# full package test + coverage
go test -coverprofile=/tmp/cover.out ./pkg/promptstate/... && go tool cover -func=/tmp/cover.out | tail -1
# expected: PASS; total coverage >= 80%

# leaf check — promptstate must not import any of the five consumer packages
grep -nE 'bborbe/dark-factory/pkg/(runner|promptresumer|committingrecoverer|queuescanner|cancellationwatcher)' pkg/promptstate/*.go
# expected: 0 lines

# build + generate clean (no new mocks expected)
go build ./... && go generate ./... && git status --porcelain pkg/ mocks/
# expected: build exit 0; no untracked/modified mock files

# CHANGELOG entry present
grep -n 'spec 101 prompt 1' CHANGELOG.md
# expected: >= 1 line

# full precommit
make precommit
# expected: exit 0
```

</verification>
