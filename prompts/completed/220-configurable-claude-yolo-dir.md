---
status: completed
summary: Replaced DARK_FACTORY_CLAUDE_CONFIG_DIR env var with claudeDir config field in .dark-factory.yaml, defaulting to ~/.claude-yolo, with ResolvedClaudeDir() helper and updated executor, factory, and tests
container: dark-factory-220-configurable-claude-yolo-dir
dark-factory-version: v0.69.0
created: "2026-03-30T15:06:52Z"
queued: "2026-03-30T15:06:52Z"
started: "2026-03-30T15:27:13Z"
completed: "2026-03-30T15:35:29Z"
---

<summary>
- Projects can configure which host directory holds Claude authentication via `.dark-factory.yaml`
- Replaces `DARK_FACTORY_CLAUDE_CONFIG_DIR` environment variable with a config-file setting
- Default changes from `~/.claude` to `~/.claude-yolo` — matches documented setup instructions
- Existing users who already set the env var to `~/.claude-yolo` can remove it
</summary>

<objective>
Replace the `DARK_FACTORY_CLAUDE_CONFIG_DIR` environment variable with a `claudeDir` config field in `.dark-factory.yaml`. This lets each project specify its own claude-yolo config directory. The env var is removed — all config goes through `.dark-factory.yaml`.
</objective>

<context>
Read CLAUDE.md for project conventions.

Key files to read before making changes:
- `pkg/config/config.go` — `Config` struct (~line 66), `Defaults()` (~line 98), `resolveFilePath()` helper (~line 313)
- `pkg/executor/executor.go` — `dockerExecutor` struct (~line 60), `NewDockerExecutor()` (~line 40), `Execute()` (~line 73, uses `resolveClaudeConfigDir` at line 120), `buildDockerCommand()` (~line 306, receives `claudeConfigDir` param)
- `pkg/executor/executor.go` — `resolveClaudeConfigDir()` (~line 412) — reads `DARK_FACTORY_CLAUDE_CONFIG_DIR`, to be removed
- `pkg/factory/factory.go` — two call sites for `NewDockerExecutor`: `CreateSpecGenerator()` (~line 318) and `CreateProcessor()` (~line 405)
- `pkg/executor/executor_internal_test.go` — tests for `resolveClaudeConfigDir` (~line 908) and Execute with `DARK_FACTORY_CLAUDE_CONFIG_DIR` (~line 704)
- `pkg/executor/executor_test.go` — external test creating `NewDockerExecutor` (~line 29)
</context>

<requirements>
### 1. Add `ClaudeDir` to Config

In `pkg/config/config.go`:

Add field to `Config` struct:
```go
ClaudeDir string `yaml:"claudeDir"`
```

Add default in `Defaults()`:
```go
ClaudeDir: "~/.claude-yolo",
```

Add resolver method:
```go
// ResolvedClaudeDir returns the claude-yolo config directory with ~ expanded.
func (c Config) ResolvedClaudeDir() string {
    return resolveFilePath(c.ClaudeDir)
}
```

`resolveFilePath` already handles `~/` expansion and `${VAR}` references — reuse it.

### 2. Add `claudeDir` to executor

In `pkg/executor/executor.go`:

Add field to `dockerExecutor` struct:
```go
claudeDir string
```

Add parameter to `NewDockerExecutor`:
```go
func NewDockerExecutor(
    containerImage string,
    projectName string,
    model string,
    netrcFile string,
    gitconfigFile string,
    env map[string]string,
    claudeDir string,
) Executor {
```

Store it: `claudeDir: claudeDir,`

In `Execute()`, replace:
```go
claudeConfigDir := resolveClaudeConfigDir(home)
```
with:
```go
claudeConfigDir := e.claudeDir
```

### 3. Remove `resolveClaudeConfigDir`

Delete the entire `resolveClaudeConfigDir` function (~lines 409-426) from `pkg/executor/executor.go`. Remove unused imports if any (`os` may still be needed by other code — check before removing).

### 4. Update factory call sites

In `pkg/factory/factory.go`, add `cfg.ResolvedClaudeDir()` as the last argument to both `NewDockerExecutor` calls:

`CreateSpecGenerator()` (~line 318):
```go
executor.NewDockerExecutor(
    containerImage,
    project.Name(cfg.ProjectName),
    cfg.Model,
    cfg.NetrcFile,
    cfg.GitconfigFile,
    cfg.Env,
    cfg.ResolvedClaudeDir(),
),
```

`CreateProcessor()` receives individual fields, not `cfg`. Thread a new `claudeDir string` parameter through `CreateProcessor` and its callers (`CreateRunner`, `CreateOneShotRunner`), matching the existing pattern for `netrcFile`/`gitconfigFile`/`env`. Pass `cfg.ResolvedClaudeDir()` at each call site.

### 5. Update tests

In `pkg/executor/executor_internal_test.go`:

- Remove the `Describe("resolveClaudeConfigDir", ...)` block (~lines 908-939)
- Update `Context("with DARK_FACTORY_CLAUDE_CONFIG_DIR set", ...)` (~line 704): remove env var, instead set `exec.claudeDir = "/custom/claude-config"` and verify volume mount
- Update `Context("with DARK_FACTORY_CLAUDE_CONFIG_DIR unset", ...)` (~line 717): remove env var, instead set `exec.claudeDir` to the expected default path and verify volume mount
- Update `BeforeEach` at ~line 597: add `claudeDir` field to `dockerExecutor` struct literal (use a test path like `"/tmp/test-claude-yolo"`)

In `pkg/executor/executor_test.go`:
- Update `NewDockerExecutor` call (~line 29) to include the new `claudeDir` parameter

### 6. Update CHANGELOG.md

Add `## Unreleased` section above `## v0.69.1`:
```
- feat: Add `claudeDir` config field to set claude-yolo config directory per project (replaces DARK_FACTORY_CLAUDE_CONFIG_DIR env var)
```

### 7. Update README.md

- Remove references to `DARK_FACTORY_CLAUDE_CONFIG_DIR` env var
- Add `claudeDir` to the config example (default: `~/.claude-yolo`)
- Update the "YOLO Container Setup" section: remove `export DARK_FACTORY_CLAUDE_CONFIG_DIR=~/.claude-yolo` instruction
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Default MUST be `~/.claude-yolo` — intentional change from old default `~/.claude`, matches documented setup in README
- `DARK_FACTORY_CLAUDE_CONFIG_DIR` env var is fully removed, not deprecated
- The container-side path `/home/node/.claude` is unchanged — only the host-side source is configurable
- Reuse `resolveFilePath()` in config.go — do NOT duplicate path resolution logic
- Do NOT modify the `Execute()` signature or the `Executor` interface
</constraints>

<verification>
```bash
make precommit
```
Must exit 0 (tests pass, lint clean).
</verification>
