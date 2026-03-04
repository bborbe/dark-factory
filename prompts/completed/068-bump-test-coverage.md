---
status: completed
summary: Bumped test coverage for pkg/executor (46.9% → 80.0%) and pkg/factory (41.7% → 97.3%)
container: dark-factory-068-bump-test-coverage
dark-factory-version: v0.16.0
created: "2026-03-04T23:31:45Z"
queued: "2026-03-04T23:31:45Z"
started: "2026-03-04T23:31:45Z"
completed: "2026-03-04T23:38:31Z"
---
# Bump test coverage to ≥80% for executor and factory packages

## Goal

Increase test coverage from 46.9% → ≥80% for `pkg/executor` and from 41.7% → ≥80% for `pkg/factory`.

## Package: `pkg/factory` (41.7% → ≥80%)

### Easy wins — test all `Create*` functions

Add tests to `pkg/factory/factory_test.go` for:

- `CreateWatcher` — returns non-nil
- `CreateProcessor` — returns non-nil
- `CreateLocker` — returns non-nil
- `CreateServer` — returns non-nil
- `CreateStatusCommand` — returns non-nil
- `CreateQueueCommand` — returns non-nil

Each just needs a valid `config.Defaults()` input and verify non-nil return. Same pattern as the existing `CreateRunner` test.

## Package: `pkg/executor` (46.9% → ≥80%)

### 1. Add `extractPromptBaseName` tests

Pure function, no dependencies. Add to `executor_internal_test.go`:

```go
Describe("extractPromptBaseName", func() {
    It("extracts basename when prefix matches", func() {
        Expect(extractPromptBaseName("myproject-042-fix-bug", "myproject")).To(Equal("042-fix-bug"))
    })
    It("returns full name when prefix does not match", func() {
        Expect(extractPromptBaseName("other-042-fix-bug", "myproject")).To(Equal("other-042-fix-bug"))
    })
    It("returns full name when containerName equals prefix", func() {
        Expect(extractPromptBaseName("myproject-", "myproject")).To(Equal(""))
    })
    It("returns full name when containerName is shorter than prefix", func() {
        Expect(extractPromptBaseName("my", "myproject")).To(Equal("my"))
    })
})
```

### 2. Make `Execute` testable via `CommandRunner` interface

The `Execute` method calls `exec.CommandContext` → `cmd.Run()` which requires Docker. Extract a `CommandRunner` interface so tests can mock it.

Create `pkg/executor/command.go`:

```go
// CommandRunner runs an external command.
type CommandRunner interface {
    Run(ctx context.Context, cmd *exec.Cmd) error
}

// defaultCommandRunner runs commands directly via cmd.Run().
type defaultCommandRunner struct{}

func (r *defaultCommandRunner) Run(ctx context.Context, cmd *exec.Cmd) error {
    return cmd.Run()
}
```

Add `commandRunner CommandRunner` field to `dockerExecutor`. Default to `&defaultCommandRunner{}` in `NewDockerExecutor`. Replace `cmd.Run()` call in `Execute` with `e.commandRunner.Run(ctx, cmd)`.

**Important:** `CommandRunner` is internal to `pkg/executor` — not exported. No need for a shared models package. The `config` import in tests is one-directional (`executor_test` → `config`), no circular dependency.

### 3. Add `Execute` tests with mock runner

Add to `executor_internal_test.go`:

```go
Describe("Execute", func() {
    var (
        exec    *dockerExecutor
        tempDir string
        logFile string
    )

    BeforeEach(func() {
        exec = &dockerExecutor{
            containerImage: config.Defaults().ContainerImage,
            projectName:    "test-project",
            commandRunner:  &fakeCommandRunner{},
        }
        // setup tempDir, logFile...
    })

    It("creates log file and temp prompt file", func() { ... })
    It("returns error when log dir creation fails", func() { ... })
    It("returns error when command runner fails", func() { ... })
    It("cleans up temp file after execution", func() { ... })
})
```

Where `fakeCommandRunner` is a simple test double defined in the test file:

```go
type fakeCommandRunner struct {
    err error
}

func (f *fakeCommandRunner) Run(ctx context.Context, cmd *exec.Cmd) error {
    return f.err
}
```

## Constraints

- Run `make precommit` for validation only
- Do NOT commit, tag, or push (dark-factory handles git)
- Coverage ≥80% for both `pkg/executor` and `pkg/factory`
- Do NOT create a shared models package — no circular dependency exists
- Do NOT change any public API signatures
- `CommandRunner` interface stays unexported (internal to executor package)
- Follow existing Ginkgo/Gomega test patterns exactly
