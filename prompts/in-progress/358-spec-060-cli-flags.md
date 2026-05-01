---
status: approved
spec: [060-config-layering-phase-1]
created: "2026-05-01T09:00:00Z"
queued: "2026-05-01T09:19:24Z"
branch: dark-factory/config-layering-phase-1
---

<summary>
- `dark-factory run --hide-git` forces hide-git mode for that invocation regardless of any yaml setting
- `dark-factory run --no-hide-git` forces hide-git off for that invocation (useful when global/project config sets it true)
- `dark-factory daemon --hide-git` and `dark-factory daemon --no-hide-git` work the same way as the `run` equivalents
- `dark-factory run --model claude-haiku-4-5` overrides the model for that invocation, beating both global and project config
- `dark-factory daemon --model NAME` works the same way
- Passing both `--hide-git` and `--no-hide-git` in one invocation exits non-zero with a usage error
- Passing `--model` with no argument exits non-zero with a usage error
- Passing `--model 'foo;rm -rf /'` (value with shell metacharacter) exits non-zero with a validation error
- The `effective config` log line shows `hideGitSource=arg` and `modelSource=arg` when the respective CLI flags are used
- All existing flags and command behavior are unchanged
</summary>

<objective>
Add three new per-invocation CLI flags — `--hide-git`, `--no-hide-git`, and `--model NAME` — to the `run` and `daemon` commands. These flags apply as the top-priority override in the layering chain (arg beats project beats global beats default). Apply the overrides to `cfg` after the global+project merge (established in prompt 1) and update `sources` so the `effective config` log shows `Source=arg` for overridden fields.

**Precondition:** Prompt 1 (`1-spec-060-global-config-schema-and-merge.md`) has been executed successfully. The `config.FieldSources` type, `applyGlobalOverrides`, `computeFieldSources`, and the updated `runCommand`/`runRunCommand`/`runDaemonCommand` signatures (with `sources config.FieldSources`) all exist.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`

Key files to read before editing:
- `main.go` — full file: `ParseArgs` function (near EOF), `run()` (~line 39), `runCommand` (~line 122), `runRunCommand` (~line 191), `runDaemonCommand` (~line 222), `printRunHelp` (~line 575), `printDaemonHelp` (~line 589), `printHelp` (~line 540), `extractMaxContainers` helper (~line 492)
- `parse_args_test.go` — `parseArgsResult` struct, `assertParseArgs`, existing test functions; must be updated for new return values
- `main_internal_test.go` — the `ParseArgs` Describe block (with local `result` struct and `parse` func), and the new `applyGlobalOverrides`/`computeFieldSources` tests added in prompt 1

The spec this implements: `specs/in-progress/060-config-layering-phase-1.md` (Behaviors 5 and 6).
</context>

<requirements>

## 1. Update `ParseArgs` in `main.go`

### 1a. Extend the return type

`ParseArgs` currently returns `(bool, string, string, []string, bool, bool)`:
`(debug, command, subcommand, args, autoApprove, skipPreflight)`

Change it to return `(bool, string, string, []string, bool, bool, *bool, string)`:
`(debug, command, subcommand, args, autoApprove, skipPreflight, hideGit, model)`

Where:
- `hideGit *bool` — `nil` = flag absent; `&true` = `--hide-git` passed; `&false` = `--no-hide-git` passed
- `model string` — empty string = flag absent; non-empty = `--model NAME` value extracted from args

### 1b. Update the doc comment for `ParseArgs`

```go
// ParseArgs parses command line arguments (without program name) and returns
// (debug, command, subcommand, args, autoApprove, skipPreflight, hideGit, model).
// The -debug flag can appear anywhere and is extracted before parsing.
// The --auto-approve flag is extracted for the "run" command.
// The --skip-preflight flag is extracted for the "run" and "daemon" commands.
// The --hide-git / --no-hide-git flags are extracted for "run" and "daemon".
// The --model NAME flag is extracted for "run" and "daemon".
// hideGit is nil when neither --hide-git nor --no-hide-git is passed.
// model is empty string when --model is not passed.
// No args → command="help" (prints usage, exits 0)
// Bare "help" word → command="help" (same as --help)
// Unknown command → command="unknown", args[0]=the unrecognized command
// Two-level: "prompt list" → command="prompt", subcommand="list"
// Top-level: "status", "list", "run", "daemon" → command=<cmd>, subcommand=""
```

### 1c. Implement the new flag extraction in `ParseArgs`

The existing extraction loop processes `-debug`, `--auto-approve`, `--skip-preflight` with a switch statement. Extend it to handle the new flags.

`--hide-git` and `--no-hide-git` are boolean flags (no following value — they consume nothing):

```go
case "--hide-git":
    t := true
    hideGit = &t
case "--no-hide-git":
    f := false
    hideGit = &f
```

`--model NAME` takes the NEXT argument as its value. Handle it specially in the loop using an index-based approach:

The simplest implementation: after the existing boolean flag extraction (which uses a range loop), do a second pass to extract `--model`:

```go
// Extract --model NAME from the filtered args (already stripped of boolean flags)
model := ""
modelFiltered := make([]string, 0, len(filtered))
for i := 0; i < len(filtered); i++ {
    if filtered[i] == "--model" {
        if i+1 >= len(filtered) {
            // Missing value — keep "--model" in filtered so runCommand can reject it
            modelFiltered = append(modelFiltered, filtered[i])
            continue
        }
        model = filtered[i+1]
        i++ // skip the value
        continue
    }
    modelFiltered = append(modelFiltered, filtered[i])
}
filtered = modelFiltered
```

Declare at the top of `ParseArgs` alongside other boolean vars:
```go
hideGit := (*bool)(nil)
model := ""
```

Update ALL return statements in `ParseArgs` to include `hideGit` and `model` as the 7th and 8th values. There are **7 return statements** — verify with `grep -n "^	return debug" main.go` and update each:

```go
return debug, "help", "", []string{}, autoApprove, skipPreflight, hideGit, model
// ... same for all 7 return statements
```

### 1d. Handle `--model` with no value

When `--model` appears as the last arg (no following value), keep `"--model"` in `filtered` so `validateNoArgs` rejects it as `unknown flag: "--model"`. This mirrors how `--max-containers` without a value is handled by `extractMaxContainers`. No sentinel needed.

## 2. Update the `ParseArgs` call site in `run()`

Change:
```go
debug, command, subcommand, args, autoApprove, skipPreflight := ParseArgs(os.Args[1:])
```
To:
```go
debug, command, subcommand, args, autoApprove, skipPreflight, hideGit, model := ParseArgs(os.Args[1:])
```

## 3. Thread `hideGit` and `model` through `runCommand`

### 3a. Pass to `runCommand`

The `runCommand` call in `run()` already passes `sources`. After the arg overrides are applied, update sources and pass:

```go
// Apply arg overrides to cfg and update sources
if hideGit != nil {
    cfg.HideGit = *hideGit
    sources.HideGit = "arg"
}
if model != "" {
    if err := validateModelArg(ctx, model); err != nil {
        return err
    }
    cfg.Model = model
    sources.Model = "arg"
}

return runCommand(ctx, cfg, command, subcommand, args, autoApprove, skipPreflight, sources, currentDateTimeGetter)
```

Place this block AFTER `sources := computeFieldSources(...)` and BEFORE the `runCommand` call.

### 3b. Add `validateModelArg` helper in `main.go`

Reuse the exported `globalconfig.ModelRegex` (defined in prompt 1) — single source of truth. Do NOT redeclare the regex.

```go
// validateModelArg validates a --model flag value against the shared model identifier regex.
// Returns an error if the value contains invalid characters.
func validateModelArg(ctx context.Context, model string) error {
    if !globalconfig.ModelRegex.MatchString(model) {
        return errors.Errorf(ctx, "--model value %q does not match required pattern %s", model, globalconfig.ModelPattern)
    }
    return nil
}
```

`pkg/globalconfig` is already imported in `main.go` (used for the global config loader). No new imports needed.

## 4. Reject `--hide-git`/`--no-hide-git` and `--model` on wrong commands

These flags only apply to `run` and `daemon`. In `run()`, BEFORE applying arg overrides, validate the command:

```go
// Validate that --hide-git/--no-hide-git and --model are only used with run/daemon
if hideGit != nil && command != "run" && command != "daemon" {
    return errors.Errorf(ctx, "unknown flag: --hide-git")
}
if model != "" && command != "run" && command != "daemon" {
    return errors.Errorf(ctx, "unknown flag: --model")
}
```

Place this block AFTER `sources := computeFieldSources(...)` and BEFORE the arg-overrides block from §3a.

## 5. Reject both `--hide-git` and `--no-hide-git` together

Detect the conflict in `run()` BEFORE calling `ParseArgs`, so it fails fast with a clear message:

```go
// Check for contradictory flags before full parsing
if slices.Contains(os.Args[1:], "--hide-git") && slices.Contains(os.Args[1:], "--no-hide-git") {
    fmt.Fprintf(os.Stderr, "error: --hide-git and --no-hide-git are mutually exclusive\n")
    return errors.Errorf(ctx, "--hide-git and --no-hide-git are mutually exclusive")
}

debug, command, subcommand, args, autoApprove, skipPreflight, hideGit, model := ParseArgs(os.Args[1:])
```

Add `"slices"` to main.go imports (Go 1.21+ stdlib).

## 6. Update help text for `run` and `daemon` commands

### 6a. `printRunHelp`

Add `[--hide-git|--no-hide-git]` and `[--model NAME]` to the usage line and flags:

```go
func printRunHelp() {
	fmt.Fprintf(
		os.Stdout,
		"Usage: dark-factory run [--max-containers N] [--auto-approve] [--skip-preflight] [--hide-git|--no-hide-git] [--model NAME]\n\n"+
			"Process all queued prompts and exit.\n\n"+
			"Flags:\n"+
			"  --max-containers N      Override the container limit for this run\n"+
			"  --auto-approve          Automatically approve new prompts found during run\n"+
			"  --skip-preflight        Skip preflight baseline check for this invocation.\n"+
			"                          Prompts may run on a broken baseline — use with caution.\n"+
			"  --hide-git              Force hide-git mode on for this invocation (overrides yaml)\n"+
			"  --no-hide-git           Force hide-git mode off for this invocation (overrides yaml)\n"+
			"  --model NAME            Override model for this invocation (overrides yaml)\n"+
			"  --help, -h              Show this help\n",
	)
}
```

### 6b. `printDaemonHelp`

```go
func printDaemonHelp() {
	fmt.Fprintf(
		os.Stdout,
		"Usage: dark-factory daemon [--max-containers N] [--skip-preflight] [--hide-git|--no-hide-git] [--model NAME]\n\n"+
			"Watch for queued prompts and execute them (long-running).\n\n"+
			"Flags:\n"+
			"  --max-containers N      Override the container limit for this run\n"+
			"  --skip-preflight        Skip preflight baseline check for this invocation.\n"+
			"                          Prompts may run on a broken baseline — use with caution.\n"+
			"  --hide-git              Force hide-git mode on for this invocation (overrides yaml)\n"+
			"  --no-hide-git           Force hide-git mode off for this invocation (overrides yaml)\n"+
			"  --model NAME            Override model for this invocation (overrides yaml)\n"+
			"  --help, -h              Show this help\n",
	)
}
```

### 6c. `printHelp`

Find the lines for `run` and `daemon` in the commands table and update:
```go
"  run [--max-containers N] [--skip-preflight] [--hide-git|--no-hide-git] [--model NAME]    Process all queued prompts and exit\n"+
"  daemon [--max-containers N] [--skip-preflight] [--hide-git|--no-hide-git] [--model NAME] Watch for queued prompts and execute them (long-running)\n"+
```

## 7. Update `parse_args_test.go`

### 7a. Extend `parseArgsResult` struct

```go
type parseArgsResult struct {
	debug         bool
	command       string
	subcommand    string
	args          []string
	autoApprove   bool
	skipPreflight bool
	hideGit       *bool
	model         string
}
```

### 7b. Update `assertParseArgs` to check new return values

```go
func assertParseArgs(t *testing.T, input []string, want parseArgsResult) {
	t.Helper()
	debug, command, subcommand, args, autoApprove, skipPreflight, hideGit, model := ParseArgs(input)
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
	// hideGit: both nil → ok; both non-nil with same value → ok; otherwise fail
	if (hideGit == nil) != (want.hideGit == nil) {
		t.Errorf("hideGit: got %v, want %v", hideGit, want.hideGit)
	} else if hideGit != nil && want.hideGit != nil && *hideGit != *want.hideGit {
		t.Errorf("hideGit: got %v, want %v", *hideGit, *want.hideGit)
	}
	if model != want.model {
		t.Errorf("model: got %q, want %q", model, want.model)
	}
}
```

### 7c. Add `TestParseArgsHideGit`

```go
func TestParseArgsHideGit(t *testing.T) {
	t.Parallel()
	trueVal := true
	falseVal := false

	// --hide-git after command
	assertParseArgs(t,
		[]string{"run", "--hide-git"},
		parseArgsResult{command: "run", args: []string{}, hideGit: &trueVal},
	)
	// --no-hide-git after command
	assertParseArgs(t,
		[]string{"run", "--no-hide-git"},
		parseArgsResult{command: "run", args: []string{}, hideGit: &falseVal},
	)
	// flag before command (position-agnostic)
	assertParseArgs(t,
		[]string{"--hide-git", "daemon"},
		parseArgsResult{command: "daemon", args: []string{}, hideGit: &trueVal},
	)
	// no flag → hideGit is nil
	assertParseArgs(t,
		[]string{"run"},
		parseArgsResult{command: "run", args: []string{}, hideGit: nil},
	)
	// combined with other flags
	assertParseArgs(t,
		[]string{"run", "--hide-git", "--skip-preflight"},
		parseArgsResult{command: "run", args: []string{}, hideGit: &trueVal, skipPreflight: true},
	)
}
```

### 7d. Add `TestParseArgsModel`

```go
func TestParseArgsModel(t *testing.T) {
	t.Parallel()

	// --model after command
	assertParseArgs(t,
		[]string{"run", "--model", "claude-opus-4-7"},
		parseArgsResult{command: "run", args: []string{}, model: "claude-opus-4-7"},
	)
	// --model before command
	assertParseArgs(t,
		[]string{"--model", "claude-haiku-4-5", "daemon"},
		parseArgsResult{command: "daemon", args: []string{}, model: "claude-haiku-4-5"},
	)
	// no flag → model is empty string
	assertParseArgs(t,
		[]string{"run"},
		parseArgsResult{command: "run", args: []string{}, model: ""},
	)
	// combined with other flags
	assertParseArgs(t,
		[]string{"run", "--model", "claude-sonnet-4-6", "--skip-preflight"},
		parseArgsResult{command: "run", args: []string{}, model: "claude-sonnet-4-6", skipPreflight: true},
	)
	// model with colon (other provider)
	assertParseArgs(t,
		[]string{"run", "--model", "qwen3.6:35b-a3b"},
		parseArgsResult{command: "run", args: []string{}, model: "qwen3.6:35b-a3b"},
	)
}
```

## 8. Update `main_internal_test.go` — ParseArgs Describe block

The `ParseArgs` Describe block defines a local `result` struct and `parse` func. Update both:

```go
type result struct {
    debug         bool
    command       string
    subcommand    string
    args          []string
    autoApprove   bool
    skipPreflight bool
    hideGit       *bool
    model         string
}
parse := func(rawArgs []string) result {
    debug, command, subcommand, args, autoApprove, skipPreflight, hideGit, model := ParseArgs(rawArgs)
    return result{debug, command, subcommand, args, autoApprove, skipPreflight, hideGit, model}
}
```

Existing `It` blocks do not assert `hideGit` or `model` so their bodies need no changes — only the struct and parse func.

## 9. Add `validateModelArg` Ginkgo tests in `main_internal_test.go`

The `--hide-git`/`--model` command-gate rejection happens in `run()` (not `runCommand`), so we test the helper directly rather than through `runCommand`:

```go
var _ = Describe("validateModelArg", func() {
	ctx := context.Background()

	It("accepts a valid Anthropic model ID", func() {
		Expect(validateModelArg(ctx, "claude-opus-4-7")).To(Succeed())
	})

	It("accepts a model with colon and slash", func() {
		Expect(validateModelArg(ctx, "qwen3.6:35b-a3b")).To(Succeed())
	})

	It("accepts a namespaced local model", func() {
		Expect(validateModelArg(ctx, "local/qwen3.6:35b-a3b")).To(Succeed())
	})

	It("accepts a Docker image ref", func() {
		Expect(validateModelArg(ctx, "docker.io/bborbe/claude-yolo:v0.6.1")).To(Succeed())
	})

	It("rejects model with semicolon (shell metachar)", func() {
		err := validateModelArg(ctx, "claude;rm -rf /")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("does not match required pattern"))
	})

	It("rejects model with dollar sign", func() {
		err := validateModelArg(ctx, "claude-$VERSION")
		Expect(err).To(HaveOccurred())
	})

	It("rejects model with spaces", func() {
		err := validateModelArg(ctx, "claude opus")
		Expect(err).To(HaveOccurred())
	})

	It("rejects empty model", func() {
		err := validateModelArg(ctx, "")
		Expect(err).To(HaveOccurred())
	})
})
```

Verify `context` and the test packages already imported; no new imports needed beyond what's already in `main_internal_test.go`.

## 10. Append to CHANGELOG entry

The `## Unreleased` section already has entries from prompt 1. Append:

```
- feat: add --hide-git and --no-hide-git CLI flags to run and daemon commands to override hideGit setting per invocation
- feat: add --model NAME CLI flag to run and daemon commands to override model per invocation
- fix: passing both --hide-git and --no-hide-git in one invocation exits non-zero with usage error
```

## 11. Run `make test`

```bash
cd /workspace && make test
```

All tests must pass before running `make precommit`.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do NOT touch `go.mod` / `go.sum` / `vendor/`
- Prompt 1 (`1-spec-060-global-config-schema-and-merge.md`) must have been executed first — this prompt depends on `config.FieldSources`, `applyGlobalOverrides`, `computeFieldSources`, and the updated `runCommand`/`runRunCommand`/`runDaemonCommand` signatures
- `--hide-git` and `--no-hide-git` are BOOLEAN flags — they do NOT consume the following argument
- `--model NAME` consumes exactly one following argument; if no argument follows, leave `"--model"` in filtered args so `validateNoArgs` rejects it with `unknown flag: "--model"`
- The model regex is REUSED from `globalconfig.ModelRegex` (exported in prompt 1) — single source of truth. Do NOT redefine the pattern in `main.go`. Import `pkg/globalconfig` if not already.
- The contradictory-flags check (`--hide-git` AND `--no-hide-git` together) must happen BEFORE `ParseArgs` is called (check `os.Args[1:]` directly using `slices.Contains`)
- The command-gate check (`--hide-git`/`--model` only valid for `run`/`daemon`) must happen in `run()` BEFORE arg overrides are applied to `cfg`
- Arg overrides are applied in `run()` (not in `runCommand`/`runRunCommand`/`runDaemonCommand`) — the `sources` struct is updated accordingly BEFORE `runCommand` is called
- Use `errors.Errorf(ctx, ...)` from `github.com/bborbe/errors` — never `fmt.Errorf`
- All test files use Ginkgo/Gomega for internal tests and standard `testing` for table-driven `parse_args_test.go` tests
- The `//nolint:funlen` annotation on factory functions must remain untouched (they are not changed in this prompt)
- Help text must NOT use Unicode arrows or special characters — ASCII only (consistent with existing help text style)
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Spot checks:
1. `grep -n "hide-git\|no-hide-git\|hideGit\|HideGit" main.go` — should find: extraction in ParseArgs, conflict check, command gate, arg apply, help text in printRunHelp/printDaemonHelp/printHelp
2. `grep -n "\-\-model\|modelArg\|modelArgPattern" main.go` — should find: extraction in ParseArgs, `validateModelArg`, `modelArgPattern` regex, arg apply, help text
3. `grep -n "hideGit\|model" parse_args_test.go` — should find: struct fields, assertParseArgs checks, TestParseArgsHideGit, TestParseArgsModel
4. `grep -n "hideGit.*result\|model.*result\|hideGit.*parse\|model.*parse" main_internal_test.go` — should find the updated result struct and parse func
5. `go build ./...` — no compile errors
6. `go test ./...` — all tests pass
7. Manually verify: `dark-factory run --hide-git --no-hide-git 2>&1 | grep -i "mutually exclusive"` — must contain the error
</verification>
