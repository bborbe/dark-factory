---
status: completed
summary: Added LogEffectiveConfig startup log to daemon and one-shot run, surfacing maxContainers source (project/global/default), container image, model, workflow flags, commands, and prompt dirs in a single slog.Info line after lock acquisition
container: dark-factory-290-log-effective-config-at-startup
dark-factory-version: v0.110.2
created: "2026-04-16T09:30:00Z"
queued: "2026-04-16T10:28:35Z"
started: "2026-04-16T10:28:50Z"
completed: "2026-04-16T10:47:55Z"
---

<summary>
- Daemon and `run` (one-shot) mode log the effective configuration as a single structured info line at startup
- Users see at a glance which `maxContainers` value is actually being used and whether it came from the project file, the global file, or the built-in default
- The log also surfaces the other common knobs: container image, model, workflow flags (worktree/pr/autoMerge/autoRelease/verificationGate), validation/test commands, debounce, and prompt lifecycle directories
- No secrets, env maps, or file contents are logged
- Configuration loading, defaults, and the `dark-factory config` command are unchanged
- Table-driven test asserts the log line and its key/value pairs for three scenarios: defaults-only, project override, global override
</summary>

<objective>
Surface the effective dark-factory configuration at process startup so users can diagnose "which config won?" without running a second command. Specifically, make `maxContainers` and its source (project / global / default) visible in the daemon and one-shot run logs, alongside the other settings that most often affect behavior. This is strictly a logging addition — no behavior, defaults, or loading logic changes.
</objective>

<context>
Read `/workspace/CLAUDE.md` for project conventions (errors wrapping with `github.com/bborbe/errors`, Ginkgo/Gomega tests, no `fmt.Errorf`, no bare `return err`).

Read these source files before making changes:

- `main.go` — the `run(ctx)` function loads the project config via `config.NewLoader().Load(ctx)` at approximately the line where it calls `slog.Info("dark-factory starting", ...)`. Note that `main.go` does NOT currently load the global config; that happens inside `pkg/factory/factory.go`. You will need to load the global config once in `main.go` so the startup log can report the effective `maxContainers` and its source.
- `pkg/config/config.go` — `Config` struct. Relevant fields: `MaxContainers`, `ContainerImage`, `Model`, `Worktree`, `PR`, `AutoRelease`, `AutoMerge`, `VerificationGate`, `ValidationCommand`, `TestCommand`, `DebounceMs`, `Prompts.InboxDir`, `Prompts.InProgressDir`, `Prompts.CompletedDir`, `Prompts.LogDir`.
- `pkg/globalconfig/globalconfig.go` — `GlobalConfig` with `MaxContainers int`. `DefaultMaxContainers = 3`. `NewLoader().Load(ctx)` returns defaults when `~/.dark-factory/config.yaml` is missing.
- `pkg/factory/factory.go` — `EffectiveMaxContainers(projectMax, globalMax int) int`. Rule: `projectMax > 0` wins, else global wins. The logging helper in this prompt must mirror this rule AND additionally distinguish "default" (global file absent or unset → `globalconfig.DefaultMaxContainers`) from "global" (global file present with an explicit `maxContainers` value).
- `pkg/runner/runner.go` — the `Run(ctx)` method on `*runner`. The daemon startup sequence is: `Acquire lock` → `slog.Info("acquired lock", ...)` → signal setup → migrate/create dirs → `slog.Info("watching for queued prompts", ...)`. The new "effective config" log MUST be emitted BEFORE "watching for queued prompts" so it appears in the daemon's startup banner rather than buried later.
- `pkg/runner/oneshot.go` — the `Run(ctx)` method on `*oneShotRunner`. Same pattern: lock → `slog.Info("acquired lock", ...)` → dirs → processing. Emit the same log line there too.
- `pkg/processor/processor_test.go` around the `bytes.Buffer` + `slog.NewTextHandler` + `slog.SetDefault` BeforeEach/AfterEach pattern for capturing log output in Ginkgo tests. Reuse this exact pattern for the new tests.

Concrete bug this fixes: in a recent debugging session the project `.dark-factory.yaml` had `maxContainers: 5` and the global `~/.dark-factory/config.yaml` had `maxContainers: 3`. The user could not tell at runtime which one was in effect without running `dark-factory config` separately, and wasted roughly 10 minutes before the effective value (5, from project) was confirmed.
</context>

<requirements>

## 1. Add `LogEffectiveConfig` helper in `pkg/factory/factory.go`

Add an exported function adjacent to `EffectiveMaxContainers` with this exact signature and behavior:

```go
// LogEffectiveConfig emits a single slog.Info "effective config" line describing
// the resolved settings that drive daemon/run behavior. This is purely diagnostic;
// no value is mutated.
//
// maxContainersSource is one of:
//   - "project" when cfg.MaxContainers > 0 (project file override wins)
//   - "global"  when cfg.MaxContainers <= 0 AND globalFilePresent is true
//                (global file present with an explicit value)
//   - "default" when cfg.MaxContainers <= 0 AND globalFilePresent is false
//                (no global file / no explicit value → globalconfig.DefaultMaxContainers)
//
// The caller is responsible for determining globalFilePresent (see requirement 2).
// No secrets, env maps, or file contents are logged. No error is returned.
func LogEffectiveConfig(cfg config.Config, globalCfg globalconfig.GlobalConfig, globalFilePresent bool) {
    // Implementation: compute effective maxContainers + source, then call slog.Info exactly once.
}
```

Rules:
- Use `EffectiveMaxContainers(cfg.MaxContainers, globalCfg.MaxContainers)` to compute the effective value — do not re-implement the rule.
- The source determination must match the doc comment exactly. In particular: if `cfg.MaxContainers > 0`, source is always `"project"` regardless of `globalFilePresent`.
- The log call MUST use `slog.Info` with literal message `"effective config"` and the following key/value pairs in this order:

  ```
  maxContainers        = <int, effective>
  maxContainersSource  = <string: "project" | "global" | "default">
  containerImage       = cfg.ContainerImage
  model                = cfg.Model
  worktree             = cfg.Worktree
  pr                   = cfg.PR
  autoRelease          = cfg.AutoRelease
  autoMerge            = cfg.AutoMerge
  verificationGate     = cfg.VerificationGate
  validationCommand    = cfg.ValidationCommand
  testCommand          = cfg.TestCommand
  debounceMs           = cfg.DebounceMs
  promptsInboxDir      = cfg.Prompts.InboxDir
  promptsInProgressDir = cfg.Prompts.InProgressDir
  promptsCompletedDir  = cfg.Prompts.CompletedDir
  promptsLogDir        = cfg.Prompts.LogDir
  ```

- Do NOT log `cfg.Env`, `cfg.ExtraMounts`, `cfg.GitHub`, `cfg.Notifications`, `cfg.Bitbucket`, `cfg.NetrcFile`, `cfg.GitconfigFile`, `cfg.AdditionalInstructions`, `cfg.ClaudeDir`, or any secret/env-var reference. (These may contain tokens or personal paths.)
- Do NOT call `slog.Info` more than once. One line, all fields, so it is easy to grep and parse.
- The function must be pure with respect to the process: no `os.Stat`, no `os.ReadFile`, no `os.Getenv`. It only takes its three parameters and logs.

## 2. Detect whether the global config file is present

Add an exported helper in `pkg/globalconfig/globalconfig.go`:

```go
// FileExists reports whether the global config file (~/.dark-factory/config.yaml) exists.
// Callers use this only to distinguish "global file present" from "using built-in defaults"
// in diagnostic logs. Error semantics:
//   - Config file missing → (false, nil)
//   - Home dir lookup fails → (false, wrapped error)
//   - Any other stat error → (false, wrapped error)
//   - File present (any size) → (true, nil)
func FileExists(ctx context.Context) (bool, error)
```

Rules:
- Resolve the home dir via the existing package-level `userHomeDir` variable (so tests can override).
- Path: `filepath.Join(home, ".dark-factory", "config.yaml")`.
- Use `os.Stat`. On `os.IsNotExist(err)` → return `(false, nil)`. On any other I/O error → return `(false, errors.Wrap(ctx, err, "globalconfig: stat config file"))`. On success → `(true, nil)`.
- An **empty but existing** file counts as `true` here — we only check existence. This is intentional: the user created the file, so source is "global" even if they explicitly set 0 / empty fields (and the loader would still fill defaults).

Also export a test hook — the existing `userHomeDir` variable can already be overridden by tests in the same package, so no additional hook is needed. If writing a test in an external `_test` package, write the test in `globalconfig_internal_test.go` (which is in-package).

## 3. Call the helper at daemon startup

In `pkg/runner/runner.go`, modify `Run(ctx)` on `*runner`. The log line must fire AFTER the lock is acquired and BEFORE `slog.Info("watching for queued prompts", ...)` — ideally directly after the existing `slog.Info("acquired lock", ...)` line.

Because `pkg/runner` must not import `pkg/factory` (factory already imports runner; a reverse import would cycle), the **log call itself must happen in the wiring layer**, not inside the runner. Approach:

- Add a new optional field `startupLogger func()` on the `runner` struct (unexported).
- Extend `NewRunner(...)` with one additional trailing parameter `startupLogger func()` (nilable). All existing call sites must pass this.
- In `Run(ctx)`, after `slog.Info("acquired lock", ...)` and before `slog.Info("watching for queued prompts", ...)`, call `if r.startupLogger != nil { r.startupLogger() }`.
- Update `pkg/factory/factory.go` `CreateRunner` to construct the closure:

  ```go
  globalFilePresent, _ := globalconfig.FileExists(ctx)
  startupLogger := func() {
      LogEffectiveConfig(cfg, globalCfg, globalFilePresent)
  }
  ```

  and pass `startupLogger` into `NewRunner`. Swallow the error from `FileExists` — logging must never block daemon startup.

## 4. Call the helper at `run` (one-shot) startup

Mirror requirement 3 for `pkg/runner/oneshot.go`:

- Add `startupLogger func()` field on `*oneShotRunner`.
- Extend `NewOneShotRunner(...)` with a trailing `startupLogger func()` parameter.
- In its `Run(ctx)`, call `if r.startupLogger != nil { r.startupLogger() }` immediately after `slog.Info("acquired lock", ...)`.
- Update `pkg/factory/factory.go` `CreateOneShotRunner` to construct the same closure and pass it in.

Both runners should log the **same** line — identical fields, identical ordering, identical message.

## 5. Do NOT modify the following

- `pkg/config/loader.go` — loading logic stays identical.
- `Defaults()` in `pkg/config/config.go` — no new defaults.
- `printConfig` in `main.go` — the `dark-factory config` command output stays byte-for-byte identical.
- `EffectiveMaxContainers` — logic unchanged.
- No new CLI flags. No new env vars.

## 6. Tests — `pkg/factory/factory_test.go`

Add a new `Describe("LogEffectiveConfig", ...)` block next to the existing `Describe("EffectiveMaxContainers", ...)`. Use the same `bytes.Buffer` + `slog.SetDefault` / `AfterEach` restore pattern as `pkg/processor/processor_test.go` (see the `BeforeEach` around the block that creates `logBuf` and sets `slog.NewTextHandler(&logBuf, ...)`).

Write a **table-driven** test (Ginkgo `DescribeTable` / `Entry`) covering these cases. In each case, after calling `LogEffectiveConfig`, assert that the captured buffer contains the literal `msg="effective config"` and the expected `maxContainers=…` and `maxContainersSource=…` pairs.

| Case | cfg.MaxContainers | globalCfg.MaxContainers | globalFilePresent | Expected maxContainers | Expected maxContainersSource |
|---|---|---|---|---|---|
| defaults-only (no project, no global file) | 0 | 3 (= `globalconfig.DefaultMaxContainers`) | false | 3 | default |
| global-only override | 0 | 7 | true | 7 | global |
| project override beats global | 5 | 3 | true | 5 | project |
| project override when global is default | 5 | 3 | false | 5 | project |
| project override equals global (still "project") | 3 | 3 | true | 3 | project |

Additional assertions that must hold in ALL five cases:
- Exactly one `msg="effective config"` substring in the buffer.
- The output contains `containerImage=…`, `model=…`, `worktree=…`, `pr=…`, `autoRelease=…`, `autoMerge=…`, `verificationGate=…`, `validationCommand=…`, `testCommand=…`, `debounceMs=…`, `promptsInboxDir=…`, `promptsInProgressDir=…`, `promptsCompletedDir=…`, `promptsLogDir=…`.
- The output does NOT contain the substrings `env=`, `extraMounts=`, `github=`, `netrcFile=`, `gitconfigFile=`, `notifications=`, `bitbucket=`.

Provide a small helper `fullTestConfig()` that returns a `config.Config` with every logged field populated with a distinct non-zero value (e.g. `ContainerImage: "ghcr.io/bborbe/yolo:test"`, `Model: "claude-sonnet-4-6"`, `ValidationCommand: "make precommit"`, `TestCommand: "make test"`, `DebounceMs: 500`, `Prompts: config.PromptsConfig{InboxDir: "p", InProgressDir: "p/ip", CompletedDir: "p/c", LogDir: "p/l"}`) so redaction assertions are meaningful.

## 7. Tests — `pkg/globalconfig/globalconfig_internal_test.go`

Extend the existing suite (or add a new `Describe("FileExists", ...)`). Cover:
- Home dir does not exist / override `userHomeDir` to return an empty temp dir → `(false, nil)`.
- Home dir exists, config file missing → `(false, nil)`.
- Home dir exists, config file is a zero-byte file → `(true, nil)`.
- Home dir exists, config file contains valid YAML → `(true, nil)`.
- Home dir lookup itself fails (override `userHomeDir` to return `("", errors.New(...))` or equivalent using the existing error-wrapping convention) → `(false, non-nil error)`.

Use `GinkgoT().TempDir()` for the fake home dir.

## 8. Tests — factory wiring (lightweight)

Do NOT write a full integration test through `CreateRunner` / `CreateOneShotRunner` — those pull in Docker, gh CLI, etc. Instead, if either constructor in `pkg/factory/factory.go` already has a test that can be extended cheaply, add a single assertion that the closure passed to `NewRunner` / `NewOneShotRunner`, when invoked, emits the `effective config` line. If that is not cheap, skip this step — requirement 6's direct tests on `LogEffectiveConfig` are the authoritative coverage.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Do NOT change config loading, defaults, validation, or the `dark-factory config` output.
- Do NOT add new CLI flags or env vars.
- Do NOT log secrets: no `cfg.Env`, no `cfg.ExtraMounts`, no `cfg.GitHub`, no `cfg.Notifications`, no `cfg.Bitbucket`, no `cfg.NetrcFile`, no `cfg.GitconfigFile`, no `cfg.AdditionalInstructions`, no `cfg.ClaudeDir`.
- Exactly ONE `slog.Info("effective config", ...)` call per startup — one for daemon, one for run. No duplicates.
- Wrap all non-nil errors with `errors.Wrap` / `errors.Errorf` from `github.com/bborbe/errors`. Never `fmt.Errorf`, never bare `return err`.
- Keep error messages lowercase, no file paths in the message.
- Respect the existing import direction: `pkg/factory` imports `pkg/runner`; the reverse is forbidden. That is why the helper lives in `pkg/factory` and is invoked via a closure the factory passes into the runner.
- Existing tests must still pass.
- No `fmt.Errorf`, no `errors.New`, no `pkg/errors` in changed files.
- Do not touch `go.mod` / `go.sum` / `vendor/`.
</constraints>

<verification>
Run `make precommit` in the repo root — must exit 0.

Manual spot-check (document in the prompt log, not automated):
1. In a project with `.dark-factory.yaml: maxContainers: 5` and global `~/.dark-factory/config.yaml: maxContainers: 3`, `dark-factory daemon` must emit a line containing `msg="effective config"`, `maxContainers=5`, `maxContainersSource=project`.
2. In a project with no `.dark-factory.yaml` but global `~/.dark-factory/config.yaml: maxContainers: 3`, the same daemon start must emit `maxContainers=3 maxContainersSource=global`.
3. With neither file present, `maxContainers=3 maxContainersSource=default`.
4. `dark-factory run` (one-shot) must emit the identical line in the same three scenarios.
5. Verify no secrets appear: run `dark-factory daemon` in the repo and confirm the startup log does not contain `env=`, `extraMounts=`, `github=`, `netrcFile=`, `gitconfigFile=`.
</verification>
