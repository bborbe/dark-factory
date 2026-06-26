---
status: completed
spec: [099-correlation-ids-structured-logging]
summary: Created pkg/log context-logger helpers (NewContext/From), docs/rules/logging-conventions.md convention doc, scripts/hotpath-logcheck.sh, and make hotpath-logcheck target (warn mode) as the foundation for spec-099 per-prompt correlation-id structured logging.
container: dark-factory-exec-466-spec-099-log-context-helpers
dark-factory-version: v0.183.0
created: "2026-06-26T05:42:49Z"
queued: "2026-06-26T06:13:15Z"
started: "2026-06-26T06:13:17Z"
completed: "2026-06-26T06:18:18Z"
branch: dark-factory/correlation-ids-structured-logging
---

<summary>

- Creates a new `pkg/log` package with two helpers that let any code carry a per-prompt logger through a `context.Context`: one to attach a logger, one to pull it back out.
- The retrieval helper never returns nil — when no logger was attached (boot code, tests), it falls back to the process default logger, so a missing logger can never panic.
- Documents the project's structured-logging convention in a new `docs/rules/logging-conventions.md`: the fixed canonical attribute-key set, the snake_case rule, the context-threading rule, and a table of removed synonyms (`file`/`path`/`prompt` all collapse to `prompt_id`).
- Adds a new `make hotpath-logcheck` Makefile target that scans the six hot-path packages for bare package-level `slog.Info/Warn/Error` calls. In this prompt it runs in WARN mode: it prints offenders but always exits 0, so the tree stays green while later prompts migrate.
- Establishes the contract every later migration prompt targets — helpers, doc, and check infrastructure — without changing any existing log call yet.
- No existing behavior changes; this prompt is purely additive.

</summary>

<objective>
Create the `pkg/log` context-logger helpers (`NewContext`, `From`), the durable `docs/rules/logging-conventions.md` convention doc, and a `make hotpath-logcheck` Makefile target in warn mode (prints offenders, always exits 0). This establishes the contract that prompts 2-5 migrate against. No hot-path log call is changed in this prompt.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` first, then `/workspace/CLAUDE.md`.

Read these coding-plugin docs:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-patterns.md` — interface/constructor/struct conventions, error wrapping
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo/Gomega suite files, external test packages (`package_test`), coverage >= 80%
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-logging-guide.md` — slog conventions used in this repo
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — changelog entry format

Read the parent spec end-to-end:
- `/workspace/specs/in-progress/099-correlation-ids-structured-logging.md` — especially Desired Behavior items 1, 5, 6, 7; Constraints; Failure Modes rows 1, 3, 4, 7; Acceptance Criteria 1, 2, 7.

Read these source files before editing:
- `/workspace/main.go` lines 120-130 — how the default slog handler is installed (`slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, ...)))`). This is the fallback `From` must return when no logger is bound.
- `/workspace/pkg/runner/runner.go` around line 165 — the daemon also calls `slog.SetDefault(...)`. Confirms `slog.Default()` is always a real handler at runtime.
- `/workspace/Makefile` — the `precommit` target (line 16) and target style. Do NOT wire `hotpath-logcheck` into `precommit` in THIS prompt (that happens in prompt 5).
- `/workspace/docs/rules/` — existing rule docs (`prompt-writing.md`, `scenario-writing.md`, `spec-writing.md`) for the doc style/location.

IMPORTANT FACTS verified against the codebase (do not re-derive):
- There is NO existing `pkg/log` package. The repo's structured logging uses stdlib `log/slog` directly. `github.com/bborbe/log` (imported as `liblog` in `pkg/factory/factory.go`) is a SEPARATE external package providing samplers — it is NOT the package this prompt creates and must NOT be confused with it.
- The new `pkg/log` package imports ONLY stdlib (`context`, `log/slog`). It imports NO dark-factory package. This guarantees no import cycle: hot-path packages import `pkg/log`, never the reverse (spec Failure Mode row 8 — the cycle case does not arise because the package is a pure leaf).
- The canonical attribute-key set (spec Desired Behavior item 5) is exactly: `prompt_id`, `spec_id`, `container`, `workflow_type`, `error`, `file`, `dir`, `branch`, `workflow_step`.

</context>

<requirements>

## 1. Create `pkg/log/context.go`

Create `/workspace/pkg/log/context.go` with the BSD license header (copy the 3-line header verbatim from any existing `.go` file, e.g. `pkg/processor/processor.go` lines 1-3), package `log`, and exactly these two helpers plus one private context-key sentinel:

```go
package log

import (
	"context"
	"log/slog"
)

// contextKey is an unexported type for the logger context key.
// Using a private named type prevents collisions with keys from other packages.
type contextKey struct{}

// loggerKey is the sentinel value used to store/retrieve the bound *slog.Logger.
var loggerKey = contextKey{}

// NewContext returns a copy of ctx carrying logger. Downstream code retrieves it
// via From. Re-binding (e.g. after a container is assigned) is done by calling
// NewContext again with a logger derived via logger.With(...).
func NewContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// From returns the *slog.Logger bound to ctx by NewContext. When no logger is
// bound (boot path, tests, pre-bind hot-path lines), it returns slog.Default()
// — never nil. Callers can therefore always call log.From(ctx).Info(...) without
// a nil check. The fallback emits to whatever handler slog.SetDefault installed
// (see main.go / pkg/runner/runner.go).
func From(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey).(*slog.Logger); ok && logger != nil {
		return logger
	}
	return slog.Default()
}
```

Do NOT add any other exported symbol to this package. Add a GoDoc package comment (a `// Package log ...` sentence) either at the top of `context.go` or in a `pkg/log/doc.go` — follow the repo's existing `doc.go` style (e.g. `pkg/processor/doc.go`).

## 2. Create `pkg/log/context_test.go`

Create `/workspace/pkg/log/context_test.go` as an EXTERNAL test package (`package log_test`) using Ginkgo/Gomega — match the suite style in `pkg/processor/processor_suite_test.go` (you will also need a suite runner file; see step 3).

The tests MUST cover (spec AC 2 and Failure Modes rows 1, 7). Name the Ginkgo `Describe`/`It` blocks so that `grep -E 'func Test(From|NewContext|With)' pkg/log/context_test.go` returns >= 3 lines is NOT how Ginkgo works — instead satisfy the intent with these THREE explicit test functions written in plain `testing` style ALONGSIDE the Ginkgo suite (Ginkgo and plain `Test*` functions coexist in the same package). Add these three plain-`testing` functions so the spec's evidence grep passes:

```go
func TestFromReturnsBoundLogger(t *testing.T) {
	var buf bytes.Buffer
	l := slog.New(slog.NewTextHandler(&buf, nil))
	ctx := log.NewContext(context.Background(), l)
	if got := log.From(ctx); got != l {
		t.Fatalf("From did not return the bound logger")
	}
}

func TestFromFallbackNeverNil(t *testing.T) {
	if got := log.From(context.Background()); got == nil {
		t.Fatal("From returned nil for an unbound context")
	}
}

func TestWithAttrsPropagate(t *testing.T) {
	var buf bytes.Buffer
	base := slog.New(slog.NewTextHandler(&buf, nil))
	bound := base.With("prompt_id", "042-foo")
	ctx := log.NewContext(context.Background(), bound)
	log.From(ctx).Info("hello")
	if !strings.Contains(buf.String(), "prompt_id=042-foo") {
		t.Fatalf("expected prompt_id attr in output, got: %s", buf.String())
	}
}
```

These three functions satisfy AC 2 (a), (b), (c) respectively:
- (a) `From(NewContext(ctx, L))` returns `L` — `TestFromReturnsBoundLogger`.
- (b) `From(context.Background())` returns a non-nil fallback — `TestFromFallbackNeverNil`.
- (c) attrs added via `With` are present on downstream emissions — `TestWithAttrsPropagate`.

Also add a Ginkgo `Describe` block (any focused name) that asserts concurrent isolation per spec Failure Mode row 7: two goroutines each bind a different `prompt_id` and emit one line; assert each goroutine's captured output contains only its own `prompt_id`. Use a per-goroutine `bytes.Buffer` (each goroutine gets its own logger/handler) so there is no shared-writer data race. Run with `run.CancelOnFirstErrorWait` if convenient, or a `sync.WaitGroup`.

Required imports in the test file: `bytes`, `context`, `log/slog`, `strings`, `testing`, plus the Ginkgo/Gomega imports and `log "github.com/bborbe/dark-factory/pkg/log"` (alias to avoid clashing with the test package name; verify the exact module path is `github.com/bborbe/dark-factory/pkg/log`).

## 3. Add the Ginkgo suite runner

Create `/workspace/pkg/log/log_suite_test.go` mirroring `pkg/processor/processor_suite_test.go` (BSD header, `package log_test`, a `TestLog(t *testing.T)` that calls `RegisterFailHandler(Fail)` and `RunSpecs(t, "Log Suite")`). This is the standard suite runner pattern in this repo.

## 4. Create `docs/rules/logging-conventions.md`

Create `/workspace/docs/rules/logging-conventions.md`. It MUST contain these three `##` headings EXACTLY so `grep -nE '^## (Canonical Keys|Threading|Removed Synonyms)' docs/rules/logging-conventions.md` returns >= 3 lines (spec AC 7):

- `## Canonical Keys` — a table/list of the fixed key set: `prompt_id`, `spec_id`, `container`, `workflow_type`, `error`, `file`, `dir`, `branch`, `workflow_step`, with a one-line meaning each. State that introducing a new key requires editing BOTH this doc AND the `hotpath-logcheck` allow-list in the same PR (spec Failure Mode row 4).
- `## Threading` — the snake_case rule and the context-threading rule. Name `log.From(ctx)` explicitly as the only correct way hot-path packages emit logs, and `log.NewContext(ctx, logger)` as the binding primitive. State that bare package-level `slog.Info/Warn/Error` is rejected by `make hotpath-logcheck` in hot-path packages.
- `## Removed Synonyms` — a table mapping removed synonyms to their canonical key: `file` → `prompt_id`, `path` → `prompt_id`, `prompt` → `prompt_id`. State these were removed at migration time (not deprecated). NOTE: in this codebase `file` is currently used as a basename attr; the migration (prompts 2-4) renames the prompt-identity uses of `file`/`path`/`prompt` to `prompt_id`. Document the canonical target as `prompt_id`.

Also list the six hot-path packages the check covers: `pkg/processor`, `pkg/executor`, `pkg/promptresumer`, `pkg/committingrecoverer`, `pkg/cancellationwatcher`, `pkg/queuescanner`. And state the documented exclusion: `pkg/executor/launch.go`'s pure argv-builder functions (`BuildDockerRunArgs` and helpers) are EXCLUDED because they are shared boot/probe launch-shape builders with no per-prompt `ctx` — they are not per-prompt hot-path code (spec Non-goal "Does NOT migrate boot-time logs").

OPEN QUESTION surfaced for the human reviewer (leave as an HTML comment in the doc): the spec's AC 5/6 evidence greps target `slog.String(...)`/`slog.Int(...)` call forms, but this codebase emits attrs exclusively via the inline key-value form `slog.Info(msg, "key", val)`. Those ACs are therefore vacuously satisfied by the grep but the real key-hygiene work is renaming inline kv keys (done in prompts 2-4). This doc is the durable record of the canonical set regardless of call form.

## 5. Add `make hotpath-logcheck` target (WARN mode)

Add a new `hotpath-logcheck` target to `/workspace/Makefile`. In THIS prompt it runs in WARN mode: it prints offending `file:line` for any bare package-level `slog.Info(`, `slog.Warn(`, or `slog.Error(` in the six hot-path packages, but ALWAYS exits 0. Prompt 5 flips it to strict (non-zero on offenders) and wires it into `precommit`.

Implement the verifier as a sibling shell script `/workspace/scripts/hotpath-logcheck.sh` (mirrors the repo pattern of `scripts/check-changelog.sh`, `scripts/check-versions.sh`) so the package allow-list is checked into the repo (spec Constraint "allow-list is checked into the repo"). The script:

- Defines an allow-list of hot-path package dirs at the top:
  ```sh
  PACKAGES="pkg/processor pkg/executor pkg/promptresumer pkg/committingrecoverer pkg/cancellationwatcher pkg/queuescanner"
  ```
- Uses POSIX `grep`/`awk` only (no GNU-specific flags) so it runs on macOS zsh AND Linux bash (spec Constraint).
- Scans `*.go` files in those packages, EXCLUDING:
  - `*_test.go` files
  - counterfeiter-generated files (those under `mocks/` are not in these dirs anyway, but exclude any file whose first lines contain `// Code generated by counterfeiter` to be safe)
  - `pkg/executor/launch.go` (the documented shared-builder exclusion from step 4)
- Matches bare package-level calls `slog.Info(`, `slog.Warn(`, `slog.Error(` (the literal forms; do NOT match `slog.InfoContext(`, `slog.WarnContext(`, `slog.ErrorContext(`, or `log.From(ctx).Info(` — only the bare `slog.X(` form). Use a pattern like `grep -nE 'slog\.(Info|Warn|Error)\('`.
- Accepts a mode argument: `hotpath-logcheck.sh warn` prints offenders to stderr and exits 0; `hotpath-logcheck.sh strict` prints offenders and exits 1 if any are found. THIS PROMPT calls it in `warn` mode.

Makefile target:
```makefile
.PHONY: hotpath-logcheck
hotpath-logcheck:
	@bash scripts/hotpath-logcheck.sh warn
```

Make the script executable (`chmod 0755`). Add the BSD-style intent as a leading comment if the repo's other scripts carry one (check `scripts/check-versions.sh` for the convention; match it).

Verify the warn-mode behavior: `make hotpath-logcheck` on the CURRENT (un-migrated) tree MUST print the existing offenders (there are ~50 bare `slog.Info/Warn/Error` calls across the six packages) AND exit 0.

## 6. CHANGELOG

Add to the `## Unreleased` section of `/workspace/CHANGELOG.md` (create the section right under the intro block if it does not exist; do NOT disturb the existing `## v0.183.0` section) ONE bullet:

```
- feat: add pkg/log context-logger helpers (NewContext/From), docs/rules/logging-conventions.md convention doc, and `make hotpath-logcheck` target (warn mode) — foundation for per-prompt correlation-id structured logging (spec 099 prompt 1)
```

</requirements>

<constraints>

- No new dependencies. `pkg/log` imports ONLY stdlib `context` and `log/slog` (spec Constraint "No new dependencies").
- `pkg/log` MUST be a pure leaf: it imports NO dark-factory package (prevents the import cycle of spec Failure Mode row 8).
- `From` MUST never return nil — always fall back to `slog.Default()` (spec Failure Mode row 1).
- Do NOT wire `hotpath-logcheck` into `make precommit` in this prompt — warn mode only; strict + precommit wiring is prompt 5.
- Do NOT change any existing `slog.X` call site in this prompt — it is purely additive.
- The verifier uses POSIX `grep`/`awk` only, no GNU-specific flags (spec Constraint — runs on macOS zsh and Linux bash).
- The package allow-list lives in `scripts/hotpath-logcheck.sh`, checked into the repo (spec Constraint).
- Errors wrapped with `bborbe/errors` if any error path arises — never `fmt.Errorf`, never `context.Background()` in pkg/ non-test code. (The helpers themselves have no error paths.)
- BSD-style license header on every new `.go` file.
- Coverage for `pkg/log` >= 80% (trivial given the two helpers).
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.

</constraints>

<verification>

```bash
cd /workspace

# AC 1 — both helpers exist
grep -nE 'func (NewContext|From)\(' pkg/log/context.go
# expected: 2 lines

# AC 2 — the three evidence test functions exist and pass
grep -E 'func Test(From|With)' pkg/log/context_test.go
# expected: >= 3 lines (TestFromReturnsBoundLogger, TestFromFallbackNeverNil, TestWithAttrsPropagate)
go test ./pkg/log/...
# expected: exit 0

# AC 7 — doc has the three canonical headings
grep -nE '^## (Canonical Keys|Threading|Removed Synonyms)' docs/rules/logging-conventions.md
# expected: >= 3 lines

# hotpath-logcheck warn mode prints offenders but exits 0 on the un-migrated tree
make hotpath-logcheck; echo "exit=$?"
# expected: prints offending file:line lines to stderr, exit=0

# leaf-package import check — pkg/log must not import any dark-factory package
grep -n 'bborbe/dark-factory' pkg/log/context.go pkg/log/doc.go 2>/dev/null
# expected: 0 lines

# build still green
go build ./...
# expected: exit 0

# CHANGELOG entry present under Unreleased
grep -nE 'correlation|structured log|pkg/log context-logger' CHANGELOG.md
# expected: >= 1 line

# full precommit (hotpath-logcheck NOT yet wired in — precommit unchanged)
make precommit
# expected: exit 0
```

</verification>
