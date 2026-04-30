---
status: completed
spec: [059-skip-preflight-cli-flag]
summary: Added --skip-preflight CLI flag to dark-factory run and daemon commands, threading it through ParseArgs, runCommand, runRunCommand, runDaemonCommand, and factory CreateRunner/CreateOneShotRunner with conditional preflight checker suppression
container: dark-factory-354-spec-059-flag-and-factory
dark-factory-version: v0.137.0-1-g310a15c6
created: "2026-04-30T19:30:00Z"
queued: "2026-04-30T19:31:53Z"
started: "2026-04-30T19:32:45Z"
completed: "2026-04-30T19:39:15Z"
branch: dark-factory/skip-preflight-cli-flag
---

<summary>
- `dark-factory run --skip-preflight` and `dark-factory daemon --skip-preflight` are valid — accepted by the CLI
- Flag is position-agnostic: `dark-factory --skip-preflight run` and `dark-factory run --skip-preflight` both work
- When the flag is set, the preflight checker is never constructed — no baseline command runs, no cache is read or written, no baseline-failure report is emitted
- A startup log line (`slog.Info`) records that preflight was skipped so operators reviewing logs can detect it
- Passing `--skip-preflight` to commands other than `run` and `daemon` (e.g. `status`, `list`, `prompt`) is rejected with an error
- `--help` for `run` and `daemon` lists `--skip-preflight` with a safety note
- Flag is not persisted into config; it is purely per-invocation
- All existing tests pass; new tests cover flag parsing, the unsupported-subcommand rejection path, and factory behavior when skip flag is set
</summary>

<objective>
Add a `--skip-preflight` CLI flag to `dark-factory run` and `dark-factory daemon` that bypasses the preflight baseline check for the duration of that process. When set, the daemon never invokes the preflight command, never reads or writes the preflight cache, and never emits baseline-failure reports. Prompts proceed directly to normal execution. Default behavior is completely unchanged.
</objective>

<context>
Read `CLAUDE.md` for project conventions (errors, Ginkgo/Gomega, Counterfeiter, no commits).
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-factory-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`

Key files to read before editing:
- `main.go` — `ParseArgs` function (~line 661), `runCommand` (~line 113), `runRunCommand` (~line 173), `runDaemonCommand` (~line 200), `printRunHelp` (~line 548), `printDaemonHelp` (~line 560), `printHelp` (~line 513), main `ParseArgs` call (~line 40)
- `parse_args_test.go` — `parseArgsResult` struct, `assertParseArgs`, all test functions; must be updated to handle 6th return value
- `main_internal_test.go` — the `ParseArgs` Describe block (~line 66) with local `result` struct and `parse` func; must be updated
- `pkg/factory/factory.go` — `CreateRunner` (~line 288) and `CreateOneShotRunner` (~line 428); both need `skipPreflight bool` param and conditional skip of preflight checker creation
- `pkg/factory/factory_test.go` — three call sites that invoke `CreateRunner`/`CreateOneShotRunner` and must receive `false` for the new `skipPreflight` param; add new skip-preflight test case

The existing preflight spec (spec 055) and its implementation are in `pkg/preflight/` and `pkg/preflightconditions/`. The preflight checker is only created when `cfg.PreflightCommand != ""`. Conditionally nilling it out (`skipPreflight=true` → never create the checker) is the entire mechanism needed.

Reference: `specs/completed/055-preflight-baseline-check.md` for the existing preflight contract being bypassed.
</context>

<requirements>

## 1. Update `ParseArgs` in `main.go`

`ParseArgs` currently returns `(bool, string, string, []string, bool)`. Change it to return `(bool, string, string, []string, bool, bool)` where the sixth value is `skipPreflight bool`.

Add `--skip-preflight` extraction inside the range loop in `ParseArgs`, immediately after the `"--auto-approve"` case:

```go
case "--skip-preflight":
    skipPreflight = true
```

Declare `skipPreflight := false` at the top of `ParseArgs` alongside `autoApprove := false`.

Return `skipPreflight` as the 6th value in all `return` statements inside `ParseArgs`. There are 5 return statements — update each:

```go
return debug, "help", "", []string{}, autoApprove, skipPreflight
// ... etc for each return
```

Update the doc comment for `ParseArgs` to include `skipPreflight` in the tuple description.

## 2. Update the `ParseArgs` call site in `main()`

`main.go` line ~40 currently reads:
```go
debug, command, subcommand, args, autoApprove := ParseArgs(os.Args[1:])
```

Change to:
```go
debug, command, subcommand, args, autoApprove, skipPreflight := ParseArgs(os.Args[1:])
```

## 3. Thread `skipPreflight` through `runCommand`

### 3a. Update the `runCommand` call (~line 87)

```go
return runCommand(ctx, cfg, command, subcommand, args, autoApprove, skipPreflight, currentDateTimeGetter)
```

### 3b. Update the `runCommand` signature and body (~line 113)

Add `skipPreflight bool` after `autoApprove bool`:

```go
func runCommand(
    ctx context.Context,
    cfg config.Config,
    command, subcommand string,
    args []string,
    autoApprove bool,
    skipPreflight bool,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
```

Add validation at the TOP of `runCommand`, before the switch statement, to reject `--skip-preflight` on unsupported commands:

```go
if skipPreflight {
    switch command {
    case "run", "daemon":
        // valid
    default:
        return errors.Errorf(ctx, "unknown flag: --skip-preflight")
    }
}
```

Update the `case "run":` and `case "daemon":` lines to pass `skipPreflight`:
```go
case "run":
    return runRunCommand(ctx, cfg, args, autoApprove, skipPreflight, currentDateTimeGetter)
case "daemon":
    return runDaemonCommand(ctx, cfg, args, skipPreflight, currentDateTimeGetter)
```

## 4. Update `runRunCommand`

Add `skipPreflight bool` after `autoApprove bool`:

```go
func runRunCommand(
    ctx context.Context,
    cfg config.Config,
    args []string,
    autoApprove bool,
    skipPreflight bool,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
```

Add a startup log **before** calling the factory, inside the function body after argument validation:

```go
if skipPreflight {
    slog.Info("preflight: baseline check disabled for this invocation (--skip-preflight flag)")
}
```

Update the factory call to pass `skipPreflight`:
```go
runErr := factory.CreateOneShotRunner(ctx, cfg, version.Version, autoApprove, skipPreflight, currentDateTimeGetter).
    Run(ctx)
```

## 5. Update `runDaemonCommand`

Add `skipPreflight bool` after `args []string`:

```go
func runDaemonCommand(
    ctx context.Context,
    cfg config.Config,
    args []string,
    skipPreflight bool,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
```

Add the same startup log after argument validation:

```go
if skipPreflight {
    slog.Info("preflight: baseline check disabled for this invocation (--skip-preflight flag)")
}
```

Update the factory call:
```go
runErr := factory.CreateRunner(ctx, cfg, version.Version, skipPreflight, currentDateTimeGetter).Run(ctx)
```

## 6. Update help text for `run` and `daemon`

### 6a. `printRunHelp`

Change the usage line and add `--skip-preflight` to the flags:

```go
func printRunHelp() {
    fmt.Fprintf(
        os.Stdout,
        "Usage: dark-factory run [--max-containers N] [--auto-approve] [--skip-preflight]\n\n"+
            "Process all queued prompts and exit.\n\n"+
            "Flags:\n"+
            "  --max-containers N  Override the container limit for this run\n"+
            "  --auto-approve      Automatically approve new prompts found during run\n"+
            "  --skip-preflight    Skip preflight baseline check for this invocation.\n"+
            "                      Prompts may run on a broken baseline — use with caution.\n"+
            "  --help, -h          Show this help\n",
    )
}
```

### 6b. `printDaemonHelp`

```go
func printDaemonHelp() {
    fmt.Fprintf(
        os.Stdout,
        "Usage: dark-factory daemon [--max-containers N] [--skip-preflight]\n\n"+
            "Watch for queued prompts and execute them (long-running).\n\n"+
            "Flags:\n"+
            "  --max-containers N  Override the container limit for this run\n"+
            "  --skip-preflight    Skip preflight baseline check for this invocation.\n"+
            "                      Prompts may run on a broken baseline — use with caution.\n"+
            "  --help, -h          Show this help\n",
    )
}
```

### 6c. `printHelp` (~line 513)

Find the lines mentioning `run` and `daemon` in the commands table and add `[--skip-preflight]` to both:

```go
"  run [--max-containers N] [--skip-preflight]    Process all queued prompts and exit\n"+
"  daemon [--max-containers N] [--skip-preflight] Watch for queued prompts and execute them (long-running)\n"+
```

## 7. Update `CreateRunner` in `pkg/factory/factory.go`

Add `skipPreflight bool` as a new parameter after `ver string`:

```go
func CreateRunner(
    ctx context.Context,
    cfg config.Config,
    ver string,
    skipPreflight bool,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
) runner.Runner {
```

Find the preflight checker creation block (the `if cfg.PreflightCommand != ""` block, around line 354). Change the condition to also check `!skipPreflight`:

```go
if cfg.PreflightCommand != "" && !skipPreflight {
    projectRoot, rootErr := os.Getwd()
    // ... rest of block unchanged ...
}
```

No other changes to `CreateRunner`.

## 8. Update `CreateOneShotRunner` in `pkg/factory/factory.go`

Add `skipPreflight bool` after `autoApprove bool`:

```go
func CreateOneShotRunner(
    ctx context.Context,
    cfg config.Config,
    ver string,
    autoApprove bool,
    skipPreflight bool,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
) runner.OneShotRunner {
```

Find the preflight checker creation block (`if cfg.PreflightCommand != ""`, around line 460). Change condition:

```go
if cfg.PreflightCommand != "" && !skipPreflight {
    projectRoot, rootErr := os.Getwd()
    // ... rest of block unchanged ...
}
```

No other changes to `CreateOneShotRunner`.

## 9. Update `parse_args_test.go`

### 9a. Add `skipPreflight` to `parseArgsResult`

```go
type parseArgsResult struct {
    debug        bool
    command      string
    subcommand   string
    args         []string
    autoApprove  bool
    skipPreflight bool
}
```

### 9b. Update `assertParseArgs` to receive and check 6th return value

```go
func assertParseArgs(t *testing.T, input []string, want parseArgsResult) {
    t.Helper()
    debug, command, subcommand, args, autoApprove, skipPreflight := ParseArgs(input)
    if debug != want.debug {
        t.Errorf("debug: got %v, want %v", debug, want.debug)
    }
    if command != want.command {
        t.Errorf("command: got %q, want %q", command, want.command)
    }
    if subcommand != want.subcommand {
        t.Errorf("subcommand: got %q, want %q", subcommand, want.subcommand)
    }
    if len(args) != len(want.args) {
        t.Errorf("args: got %v, want %v", args, want.args)
        return
    }
    for i := range args {
        if args[i] != want.args[i] {
            t.Errorf("args[%d]: got %q, want %q", i, args[i], want.args[i])
        }
    }
    if autoApprove != want.autoApprove {
        t.Errorf("autoApprove: got %v, want %v", autoApprove, want.autoApprove)
    }
    if skipPreflight != want.skipPreflight {
        t.Errorf("skipPreflight: got %v, want %v", skipPreflight, want.skipPreflight)
    }
}
```

### 9c. Add `TestParseArgsSkipPreflight`

```go
func TestParseArgsSkipPreflight(t *testing.T) {
    t.Parallel()
    // flag after command
    assertParseArgs(t,
        []string{"run", "--skip-preflight"},
        parseArgsResult{command: "run", args: []string{}, skipPreflight: true},
    )
    // flag before command (position-agnostic)
    assertParseArgs(t,
        []string{"--skip-preflight", "run"},
        parseArgsResult{command: "run", args: []string{}, skipPreflight: true},
    )
    // flag for daemon
    assertParseArgs(t,
        []string{"daemon", "--skip-preflight"},
        parseArgsResult{command: "daemon", args: []string{}, skipPreflight: true},
    )
    // without flag, skipPreflight defaults to false
    assertParseArgs(t,
        []string{"run"},
        parseArgsResult{command: "run", args: []string{}, skipPreflight: false},
    )
    // combined with other flags
    assertParseArgs(t,
        []string{"-debug", "run", "--auto-approve", "--skip-preflight"},
        parseArgsResult{debug: true, command: "run", args: []string{}, autoApprove: true, skipPreflight: true},
    )
    // idempotent: flag passed twice
    assertParseArgs(t,
        []string{"run", "--skip-preflight", "--skip-preflight"},
        parseArgsResult{command: "run", args: []string{}, skipPreflight: true},
    )
}
```

## 10. Update `main_internal_test.go`

The `ParseArgs` Describe block (~line 66) defines a local `result` struct and `parse` func. Update both to handle the 6th return value:

```go
var _ = Describe("ParseArgs", func() {
    type result struct {
        debug        bool
        command      string
        subcommand   string
        args         []string
        autoApprove  bool
        skipPreflight bool
    }
    parse := func(rawArgs []string) result {
        debug, command, subcommand, args, autoApprove, skipPreflight := ParseArgs(rawArgs)
        return result{debug, command, subcommand, args, autoApprove, skipPreflight}
    }
    // ... existing It blocks unchanged ...
```

The existing `It` blocks do not assert `skipPreflight` (they only assert command/args), so no changes to the It bodies are needed — just the struct and parse func.

### 10b. Add `runCommand` rejection tests for unsupported subcommands

Add a new top-level `Describe` block in `main_internal_test.go` (after the `ParseArgs` Describe). Use `libtime.NewCurrentDateTime()` (already imported transitively; add the import if missing) and `config.Config{}` zero-value — the rejection happens before any config field is read.

```go
var _ = Describe("runCommand --skip-preflight rejection", func() {
    ctx := context.Background()
    dt := libtime.NewCurrentDateTime()

    DescribeTable("rejects --skip-preflight on unsupported commands",
        func(command string) {
            err := runCommand(ctx, config.Config{}, command, "", []string{}, false, true, dt)
            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring("unknown flag: --skip-preflight"))
        },
        Entry("status", "status"),
        Entry("list", "list"),
        Entry("prompt", "prompt"),
        Entry("spec", "spec"),
        Entry("scenario", "scenario"),
        Entry("config", "config"),
    )
})
```

Add the necessary imports to `main_internal_test.go`:
```go
"github.com/bborbe/dark-factory/pkg/config"
libtime "github.com/bborbe/time"
```

## 11. Update `pkg/factory/factory_test.go`

### 11a. Update existing `CreateRunner` call sites to pass `false` for `skipPreflight`

There are **two** call sites in factory_test.go. Find them with:
```bash
grep -n "factory.CreateRunner" pkg/factory/factory_test.go
```

Add `false` as the new 4th argument (after `"v0.0.1"`, before `libtime.NewCurrentDateTime()`):

```go
factory.CreateRunner(context.Background(), cfg, "v0.0.1", false, libtime.NewCurrentDateTime())
```

```go
factory.CreateRunner(ctx, c, "v0.0.1", false, libtime.NewCurrentDateTime()).Run(ctx)
```

### 11b. Update existing `CreateOneShotRunner` call site to pass `false` for `skipPreflight`

There is **one** call site. Find it with:
```bash
grep -n "factory.CreateOneShotRunner" pkg/factory/factory_test.go
```

Add `false` as the 5th argument (after `autoApprove false`, before `libtime.NewCurrentDateTime()`):

```go
factory.CreateOneShotRunner(ctx, c, "v0.0.1", false, false, libtime.NewCurrentDateTime()).Run(ctx)
```

### 11c. Add skip-preflight test for `CreateOneShotRunner`

Find the Describe block that contains the "CreateOneShotRunner.Run returns ErrPreflightFailed" test (around line 392). Add a new `It` block immediately after it:

```go
It("CreateOneShotRunner.Run returns nil when skip-preflight bypasses failing preflight", func() {
    c := buildPreflightConfig()
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    // skipPreflight=true — preflight checker not created, queue is empty → exits with nil
    err := factory.CreateOneShotRunner(ctx, c, "v0.0.1", false, true, libtime.NewCurrentDateTime()).Run(ctx)
    Expect(err).NotTo(HaveOccurred())
})
```

This test verifies that when `skipPreflight=true`, the one-shot runner does NOT return `ErrPreflightFailed` (it exits cleanly with nil because the queue is empty).

## 12. Write CHANGELOG entry

Add or extend `## Unreleased` at the top of `CHANGELOG.md`:

```
- feat: add --skip-preflight CLI flag to run and daemon commands to bypass preflight baseline check for a single invocation
```

## 13. Run `make test`

```bash
cd /workspace && make test
```

All tests must pass before running `make precommit`.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do NOT touch `go.mod` / `go.sum` / `vendor/`
- The flag must NOT be persisted into `config.Config` — it is per-invocation only; do NOT add any field to `Config` or `Defaults()` or `Validate()`
- Existing CLI behavior must not change for invocations that do not pass the flag — all existing tests continue to pass unchanged
- Preflight cache state from previous non-skip runs must not be affected — the simplest correct implementation is "never construct the checker when skipPreflight=true", which means no cache reads or writes can occur
- The flag applies uniformly to `run` (one-shot) and `daemon` modes
- When `--skip-preflight` is passed to commands other than `run` and `daemon`, return an error via `errors.Errorf` (no new `os.Exit` calls)
- Use `errors.Errorf(ctx, ...)` from `github.com/bborbe/errors` for any new error creation — never `fmt.Errorf`
- Use `slog.Info(...)` for the startup log, consistent with other startup log lines in `main.go`
- The `//nolint:funlen` annotation on `CreateRunner` and `CreateOneShotRunner` in `factory.go` must be preserved
- All test files use Ginkgo/Gomega in external `_test` packages (package `main_test`, `factory_test`) or the standard `testing` package for table-driven tests — follow existing file patterns
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Spot checks:
1. `grep -n "skip-preflight\|skipPreflight\|SkipPreflight" main.go` — should find: extraction in ParseArgs, validation in runCommand, log in runRunCommand/runDaemonCommand, factory calls, help text
2. `grep -n "skipPreflight\|skip-preflight" pkg/factory/factory.go` — should find: parameter in CreateRunner and CreateOneShotRunner, condition in preflight checker creation block (both functions)
3. `grep -n "skipPreflight\|SkipPreflight" parse_args_test.go` — should find: struct field, assertParseArgs check, TestParseArgsSkipPreflight test cases
4. `grep -n "skipPreflight\|false.*false\|v0.0.1.*false" pkg/factory/factory_test.go` — should find 3 updated call sites and the new skip test
5. `go build ./...` — no compile errors
6. `go test ./...` — all tests pass
</verification>
