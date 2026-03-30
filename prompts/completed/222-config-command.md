---
status: completed
summary: Added `dark-factory config` command that prints effective configuration as YAML to stdout
container: dark-factory-222-config-command
dark-factory-version: v0.69.0
created: "2026-03-30T16:03:00Z"
queued: "2026-03-30T16:03:00Z"
started: "2026-03-30T16:03:02Z"
completed: "2026-03-30T16:10:50Z"
---

<summary>
- New `dark-factory config` command prints the effective configuration
- Shows merged result of defaults + `.dark-factory.yaml` as YAML
- Reuses existing config loading and validation
- No subcommands — just `dark-factory config`
- Helps users verify what values dark-factory actually uses
</summary>

<objective>
Add a `dark-factory config` command that loads the configuration (defaults merged with `.dark-factory.yaml`) and prints the effective result as YAML to stdout.
</objective>

<context>
Read CLAUDE.md for project conventions.

Key files to read before making changes:
- `main.go` — CLI dispatch: `ParseArgs()` (~line 172), switch on `command` (~line 70), `printHelp()` (~line 138)
- `pkg/config/config.go` — `Config` struct (~line 66)
- `pkg/config/loader.go` — `Loader` interface, `Load()` method (~line 73)
</context>

<requirements>
### 1. Add `config` command to CLI dispatch

In `main.go`, add a case in the top-level switch (~line 70):
```go
case "config":
    return printConfig(cfg)
```

Add the `printConfig` function:
```go
func printConfig(cfg config.Config) error {
    enc := yaml.NewEncoder(os.Stdout)
    enc.SetIndent(2)
    defer enc.Close()
    return enc.Encode(cfg)
}
```

Add `"gopkg.in/yaml.v3"` to imports.

### 2. Register `config` as a known command in ParseArgs

In `ParseArgs()` (~line 199), add `"config"` to the top-level commands:
```go
case "run", "daemon", "status", "list", "config":
```

### 3. Update help text

In `printHelp()` (~line 138), add after the `list` line:
```
"  config                 Show effective configuration (defaults + .dark-factory.yaml)\n"+
```

### 4. Add test for ParseArgs

In `parse_args_test.go`, add a test case for the `config` command matching the existing pattern for top-level commands like `status` or `list`.

### 5. Update CHANGELOG.md

Add to the `## Unreleased` section (create if missing):
```
- feat: Add `config` command to show effective configuration
```
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Output is YAML only — matches `.dark-factory.yaml` input format
- No subcommands — just `dark-factory config`
- Config is loaded and validated before printing (reuses existing `loader.Load()` in main.go)
- Sensitive fields (github.token) are printed as-is (they contain `${VAR}` references, not resolved values)
</constraints>

<verification>
```bash
make precommit
```
Must exit 0.
</verification>
