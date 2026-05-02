---
status: completed
spec: [061-cli-set-config-flag]
summary: Replaced --hide-git/--no-hide-git with generic --set key=value flag on run and daemon commands, removed slices import, updated ParseArgs signature, added parseSetFlags/applySetOverrides/applyOneSetOverride/parseStrictBool helpers, extended FieldSources with MaxContainers, updated LogEffectiveConfig, and removed all hideGit test references.
container: dark-factory-360-spec-061-implementation
dark-factory-version: v0.141.1-1-g4fd8246-dirty
created: "2026-05-02T11:00:00Z"
queued: "2026-05-02T11:07:13Z"
started: "2026-05-02T11:08:19Z"
completed: "2026-05-02T11:27:08Z"
branch: dark-factory/cli-set-config-flag
---

<summary>
- A new `--set key=value` CLI flag on `run` and `daemon` overrides any supported config field for one invocation; the flag may appear multiple times
- Supported keys are `hideGit`, `autoRelease`, `dirtyFileThreshold`, `model`, and `maxContainers`; unknown keys exit non-zero with an error listing supported keys
- Bool fields accept only `true` or `false` (strict); ints must parse and be in range; `model` is validated against the existing regex
- Last occurrence of a key wins when `--set` is repeated for the same key; a debug-level log records the override sequence
- The effective-config log shows `*Source=arg` for every field set via `--set`
- `--hide-git` and `--no-hide-git` are removed; the CLI rejects them as unknown flags
- All previous `--hide-git` / `--no-hide-git` tests are removed; all test call sites of `ParseArgs` and `applyArgOverrides` are updated to match the new signatures
- `--model NAME` and `--max-containers N` dedicated flags continue to work unchanged
- `make precommit` passes with zero lint violations (no new `//nolint` annotations added)
</summary>

<objective>
Replace the two dedicated `--hide-git` / `--no-hide-git` CLI flags with a generic `--set key=value` flag on the `run` and `daemon` commands. The flag accepts any of the 5 supported yaml-backed user-pref keys, applies strict type coercion, and sets `Source=arg` in the effective-config log. Removing the dedicated flags eliminates the growing per-field pattern; future fields need only a single new entry in the supported-keys table.

**Precondition:** Spec 060 prompts 1–3 have executed. `config.FieldSources`, `applyGlobalOverrides`, `computeFieldSources`, `applyArgOverrides`, `validateModelArg`, and the updated `runCommand` / `runRunCommand` / `runDaemonCommand` signatures (accepting `sources config.FieldSources`) all exist.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-parse-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/`

Key files to read in full before editing:
- `main.go` — full file: `run()` (~line 40), `ParseArgs` (~line 841), `applyArgOverrides` (~line 615), `extractMaxContainers` (~line 586), `printHelp` (~line 678), `printRunHelp` (~line 713), `printDaemonHelp` (~line 730), imports block (~line 7)
- `parse_args_test.go` — `parseArgsResult` struct (~line 9), `assertParseArgs` (~line 22), `TestParseArgsHideGit` (~line 272), `TestParseArgsModel` (~line 304)
- `main_internal_test.go` — ParseArgs Describe block (~line 70), `applyArgOverrides` Describe block (~line 248), `validateModelArg` Describe block (~line 306)

The spec this implements: `specs/in-progress/061-cli-set-config-flag.md` (Behaviors 1–9, Constraints, and Acceptance Criteria).
</context>

<requirements>

## 1. Add `parseSetFlags` helper to `main.go`

Add this function after `extractMaxContainers` (~line 611) and before `applyArgOverrides` (~line 613):

```go
// supportedSetKeys is the authoritative list of yaml-backed user-pref keys
// accepted by --set. Adding a new yaml field requires a new entry here.
var supportedSetKeys = []string{"hideGit", "autoRelease", "dirtyFileThreshold", "model", "maxContainers"}

// parseSetFlags scans rawArgs for --set key=value occurrences, collects them into a
// map (last occurrence wins for duplicates), and returns filtered args with --set
// entries removed. Call before ParseArgs to avoid contaminating the arg list.
func parseSetFlags(ctx context.Context, rawArgs []string) (map[string]string, []string, error) {
	overrides := make(map[string]string)
	filtered := make([]string, 0, len(rawArgs))
	for i := 0; i < len(rawArgs); i++ {
		if rawArgs[i] != "--set" {
			filtered = append(filtered, rawArgs[i])
			continue
		}
		if i+1 >= len(rawArgs) {
			return nil, nil, errors.Errorf(ctx, "--set requires a value")
		}
		val := rawArgs[i+1]
		i++ // consume the value
		parts := strings.SplitN(val, "=", 2)
		if len(parts) != 2 {
			return nil, nil, errors.Errorf(ctx, "--set value must be key=value, got %q", val)
		}
		key := parts[0]
		value := parts[1]
		if key == "" {
			return nil, nil, errors.Errorf(ctx, "--set key must not be empty")
		}
		if prev, exists := overrides[key]; exists {
			slog.Debug("--set: duplicate key, last value wins", "key", key, "prev", prev, "new", value)
		}
		overrides[key] = value
	}
	return overrides, filtered, nil
}
```

Add `"strings"` to imports if not already present (it is already in `main.go`). No new imports needed.

## 2. Add `applySetOverrides` and helpers to `main.go`

Add these functions after `parseSetFlags` (before `applyArgOverrides`). Keep each function under 80 lines to satisfy the `funlen` linter:

```go
// applySetOverrides validates command-gate rules and applies --set key=value overrides
// to cfg and sources. Valid only for "run" and "daemon" commands.
func applySetOverrides(
	ctx context.Context,
	cfg *config.Config,
	sources *config.FieldSources,
	command string,
	setOverrides map[string]string,
) error {
	if len(setOverrides) == 0 {
		return nil
	}
	switch command {
	case "run", "daemon":
		// valid
	default:
		return errors.Errorf(ctx, "unknown flag: --set")
	}
	for key, value := range setOverrides {
		if err := applyOneSetOverride(ctx, cfg, sources, key, value); err != nil {
			return err
		}
	}
	return nil
}

// applyOneSetOverride applies a single --set key=value entry with type coercion and validation.
func applyOneSetOverride(
	ctx context.Context,
	cfg *config.Config,
	sources *config.FieldSources,
	key, value string,
) error {
	switch key {
	case "hideGit":
		b, err := parseStrictBool(ctx, key, value)
		if err != nil {
			return err
		}
		cfg.HideGit = b
		sources.HideGit = "arg"
	case "autoRelease":
		b, err := parseStrictBool(ctx, key, value)
		if err != nil {
			return err
		}
		cfg.AutoRelease = b
		sources.AutoRelease = "arg"
	case "dirtyFileThreshold":
		n, err := strconv.Atoi(value)
		if err != nil {
			return errors.Errorf(ctx, "--set %s: invalid integer %q", key, value)
		}
		if n < 0 {
			return errors.Errorf(ctx, "--set %s: dirtyFileThreshold must be >= 0, got %d", key, n)
		}
		cfg.DirtyFileThreshold = n
		sources.DirtyFileThreshold = "arg"
	case "model":
		if err := validateModelArg(ctx, value); err != nil {
			return err
		}
		cfg.Model = value
		sources.Model = "arg"
	case "maxContainers":
		n, err := strconv.Atoi(value)
		if err != nil {
			return errors.Errorf(ctx, "--set %s: invalid integer %q", key, value)
		}
		if n < 1 {
			return errors.Errorf(ctx, "--set %s: maxContainers must be >= 1, got %d", key, n)
		}
		cfg.MaxContainers = n
		sources.MaxContainers = "arg"
	default:
		return errors.Errorf(ctx, "unknown config key: %s (supported: %s)", key, strings.Join(supportedSetKeys, ", "))
	}
	return nil
}

// parseStrictBool parses a --set bool value. Only "true" and "false" are accepted.
// strconv.ParseBool is intentionally NOT used — it accepts 1/0/yes/no which would
// diverge from yaml semantics.
func parseStrictBool(ctx context.Context, key, value string) (bool, error) {
	switch value {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, errors.Errorf(ctx, "--set %s: invalid bool %q, expected true or false", key, value)
	}
}
```

`strconv` is already imported. No new imports needed.

## 2.5. Extend `FieldSources` with `MaxContainers`

The existing `pkg/config/sources.go` `FieldSources` struct lacks a `MaxContainers` field — `maxContainersSource` is currently computed inline in `pkg/factory/factory.go` `LogEffectiveConfig` based on `cfg.MaxContainers > 0`. With `--set maxContainers=N`, we need explicit `arg` source tracking.

### 2.5a. Add field to `FieldSources`

In `pkg/config/sources.go`:

```go
type FieldSources struct {
	HideGit            string
	AutoRelease        string
	DirtyFileThreshold string
	Model              string
	MaxContainers      string
}
```

### 2.5b. Set source in existing project/global merge

Find where `cfg.MaxContainers` is set during global/project merge (search `MaxContainers` in `pkg/config/loader.go` and `pkg/factory/factory.go`). Anywhere the value is set from a layer, ALSO set `sources.MaxContainers` to that layer name (`"global"`, `"project"`, or leave as default-empty meaning `"default"`).

If the existing logic computes the source string only inside `LogEffectiveConfig`, hoist it to the merge stage so the source is recorded as a first-class signal at merge time, consistent with the other 4 fields. Concretely:
- `applyGlobalOverrides` (or equivalent): if `globalCfg.MaxContainers > 0`, set `cfg.MaxContainers = globalCfg.MaxContainers` AND `sources.MaxContainers = "global"`.
- Project-level merge: if `cfg.MaxContainers > 0` after project yaml load (and not set by global), `sources.MaxContainers = "project"`.

### 2.5c. Update `LogEffectiveConfig` to use `sources.MaxContainers`

In `pkg/factory/factory.go`, replace the inline `maxContainersSource` computation with `sources.MaxContainers` (treat empty string as `"default"`). The signature already accepts `sources` — just drop the inline ternary at lines ~85-100 and replace with:

```go
source := sources.MaxContainers
if source == "" {
	source = "default"
}
```

This change makes maxContainers symmetric with the other 4 layered fields. `--set maxContainers=N` now produces `maxContainersSource=arg` consistently.

### 2.5d. Document precedence between `--max-containers N` and `--set maxContainers=N`

If the operator passes both `--max-containers 3` and `--set maxContainers=5` in one invocation, both write to `cfg.MaxContainers` via different code paths. The order in `runRunCommand` / `runDaemonCommand` matters. Pick: **`--max-containers N` (the existing dedicated flag) wins** — it is applied AFTER `applySetOverrides` and represents an explicit operator preference for the older form. Add a unit test asserting this order. Document in the help text under `--set`: "Note: `--max-containers N` takes precedence over `--set maxContainers=N` if both are passed."

## 3. Update `applyArgOverrides` in `main.go` — remove `hideGit *bool` parameter

The current signature is:
```go
func applyArgOverrides(
	ctx context.Context,
	cfg *config.Config,
	sources *config.FieldSources,
	command string,
	hideGit *bool,
	model string,
) error {
```

Change to (remove `hideGit *bool`):
```go
func applyArgOverrides(
	ctx context.Context,
	cfg *config.Config,
	sources *config.FieldSources,
	command string,
	model string,
) error {
```

Remove the `hideGit` branches from the body:
```go
func applyArgOverrides(
	ctx context.Context,
	cfg *config.Config,
	sources *config.FieldSources,
	command string,
	model string,
) error {
	if model != "" && command != "run" && command != "daemon" {
		return errors.Errorf(ctx, "unknown flag: --model")
	}
	if model != "" {
		if err := validateModelArg(ctx, model); err != nil {
			return err
		}
		cfg.Model = model
		sources.Model = "arg"
	}
	return nil
}
```

Also update the doc comment at line ~613:
```go
// applyArgOverrides validates command-gate rules and applies --model CLI flag override to cfg and sources.
// model is the extracted flag value from ParseArgs (empty = not set).
```

## 4. Update `run()` in `main.go`

### 4a. Replace the contradictory-flag check with `parseSetFlags`

Remove the following block (lines 41-45):
```go
// Check for contradictory flags before full parsing
if slices.Contains(os.Args[1:], "--hide-git") && slices.Contains(os.Args[1:], "--no-hide-git") {
    fmt.Fprintf(os.Stderr, "error: --hide-git and --no-hide-git are mutually exclusive\n")
    return errors.Errorf(ctx, "--hide-git and --no-hide-git are mutually exclusive")
}
```

Replace with:
```go
setOverrides, filteredArgs, err := parseSetFlags(ctx, os.Args[1:])
if err != nil {
    return err
}
```

### 4b. Update `ParseArgs` call (line 47)

Change:
```go
debug, command, subcommand, args, autoApprove, skipPreflight, hideGit, model := ParseArgs(
    os.Args[1:],
)
```

To:
```go
debug, command, subcommand, args, autoApprove, skipPreflight, model := ParseArgs(filteredArgs)
```

### 4c. Update `applyArgOverrides` call (line 101)

Change:
```go
if err := applyArgOverrides(ctx, &cfg, &sources, command, hideGit, model); err != nil {
    return err
}
```

To:
```go
if err := applyArgOverrides(ctx, &cfg, &sources, command, model); err != nil {
    return err
}
if err := applySetOverrides(ctx, &cfg, &sources, command, setOverrides); err != nil {
    return err
}
```

### 4d. Remove `"slices"` from imports

After removing the `slices.Contains` call, `slices` is no longer used in `main.go`. Remove `"slices"` from the import block. Verify with:
```bash
grep -n "slices\." main.go
```
Should return no matches.

## 5. Update `ParseArgs` in `main.go`

### 5a. Remove `hideGit *bool` from return type

Change the signature from:
```go
func ParseArgs(rawArgs []string) (bool, string, string, []string, bool, bool, *bool, string) {
```
To:
```go
func ParseArgs(rawArgs []string) (bool, string, string, []string, bool, bool, string) {
```

### 5b. Remove `hideGit` declarations and `--hide-git`/`--no-hide-git` cases

Remove these lines from inside `ParseArgs`:
```go
hideGit := (*bool)(nil)
```
```go
case "--hide-git":
    t := true
    hideGit = &t
case "--no-hide-git":
    f := false
    hideGit = &f
```

### 5c. Update all 7 return statements in `ParseArgs`

The current return statements all end with `, hideGit, model`. Remove `hideGit,` from each one:

| Old | New |
|-----|-----|
| `return debug, "help", "", []string{}, autoApprove, skipPreflight, hideGit, model` | `return debug, "help", "", []string{}, autoApprove, skipPreflight, model` |
| (second help return) | same |
| `return debug, "version", "", []string{}, autoApprove, skipPreflight, hideGit, model` | remove `hideGit,` |
| `return debug, command, "", rest, autoApprove, skipPreflight, hideGit, model` | remove `hideGit,` |
| `return debug, command, "", []string{}, autoApprove, skipPreflight, hideGit, model` | remove `hideGit,` |
| `return debug, command, rest[0], rest[1:], autoApprove, skipPreflight, hideGit, model` | remove `hideGit,` |
| `return debug, "unknown", "", filtered, autoApprove, skipPreflight, hideGit, model` | remove `hideGit,` |

Verify count: `grep -c "return debug" main.go` should be 7.

### 5d. Update the `ParseArgs` doc comment

Replace:
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
```

With:
```go
// ParseArgs parses command line arguments (without program name) and returns
// (debug, command, subcommand, args, autoApprove, skipPreflight, model).
// The -debug flag can appear anywhere and is extracted before parsing.
// The --auto-approve flag is extracted for the "run" command.
// The --skip-preflight flag is extracted for the "run" and "daemon" commands.
// The --model NAME flag is extracted for "run" and "daemon".
// model is empty string when --model is not passed.
// --set key=value is NOT extracted here — call parseSetFlags before ParseArgs.
```

## 6. Update help text in `main.go`

### 6a. `printRunHelp` (~line 713)

Replace the current function body with:
```go
func printRunHelp() {
	fmt.Fprintf(
		os.Stdout,
		"Usage: dark-factory run [--max-containers N] [--auto-approve] [--skip-preflight] [--model NAME] [--set key=value ...]\n\n"+
			"Process all queued prompts and exit.\n\n"+
			"Flags:\n"+
			"  --max-containers N      Override the container limit for this run\n"+
			"  --auto-approve          Automatically approve new prompts found during run\n"+
			"  --skip-preflight        Skip preflight baseline check for this invocation.\n"+
			"                          Prompts may run on a broken baseline — use with caution.\n"+
			"  --model NAME            Override model for this invocation (overrides yaml)\n"+
			"  --set key=value         Override a config field for this invocation; may repeat\n"+
			"                          Supported keys: hideGit, autoRelease, dirtyFileThreshold, model, maxContainers\n"+
			"                          Bool example:   --set hideGit=true\n"+
			"                          Int example:    --set dirtyFileThreshold=5\n"+
			"                          String example: --set model=claude-opus-4-7\n"+
			"  --help, -h              Show this help\n",
	)
}
```

### 6b. `printDaemonHelp` (~line 730)

Replace the current function body with:
```go
func printDaemonHelp() {
	fmt.Fprintf(
		os.Stdout,
		"Usage: dark-factory daemon [--max-containers N] [--skip-preflight] [--model NAME] [--set key=value ...]\n\n"+
			"Watch for queued prompts and execute them (long-running).\n\n"+
			"Flags:\n"+
			"  --max-containers N      Override the container limit for this run\n"+
			"  --skip-preflight        Skip preflight baseline check for this invocation.\n"+
			"                          Prompts may run on a broken baseline — use with caution.\n"+
			"  --model NAME            Override model for this invocation (overrides yaml)\n"+
			"  --set key=value         Override a config field for this invocation; may repeat\n"+
			"                          Supported keys: hideGit, autoRelease, dirtyFileThreshold, model, maxContainers\n"+
			"                          Bool example:   --set hideGit=true\n"+
			"                          Int example:    --set dirtyFileThreshold=5\n"+
			"                          String example: --set model=claude-opus-4-7\n"+
			"  --help, -h              Show this help\n",
	)
}
```

### 6c. `printHelp` (~line 678)

Find the two lines for `run` and `daemon` in the commands table and replace:

Old `run` line:
```
"  run [--max-containers N] [--skip-preflight] [--hide-git|--no-hide-git] [--model NAME]    Process all queued prompts and exit\n"+
```
New:
```
"  run [--max-containers N] [--skip-preflight] [--model NAME] [--set key=value ...]    Process all queued prompts and exit\n"+
```

Old `daemon` line:
```
"  daemon [--max-containers N] [--skip-preflight] [--hide-git|--no-hide-git] [--model NAME] Watch for queued prompts and execute them (long-running)\n"+
```
New:
```
"  daemon [--max-containers N] [--skip-preflight] [--model NAME] [--set key=value ...] Watch for queued prompts and execute them (long-running)\n"+
```

## 7. Update `parse_args_test.go`

### 7a. Remove `hideGit *bool` from `parseArgsResult` struct

Change:
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

To:
```go
type parseArgsResult struct {
    debug         bool
    command       string
    subcommand    string
    args          []string
    autoApprove   bool
    skipPreflight bool
    model         string
}
```

### 7b. Update `assertParseArgs` function

Change the `ParseArgs` call and remove hideGit checks:
```go
func assertParseArgs(t *testing.T, input []string, want parseArgsResult) {
    t.Helper()
    debug, command, subcommand, args, autoApprove, skipPreflight, model := ParseArgs(input)
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
    if model != want.model {
        t.Errorf("model: got %q, want %q", model, want.model)
    }
}
```

### 7c. Remove `TestParseArgsHideGit` entirely

Delete the entire `TestParseArgsHideGit` function (all `assertParseArgs` calls within it and the function itself).

### 7d. Verify `TestParseArgsModel` still compiles

`TestParseArgsModel` uses `parseArgsResult{command: ..., args: ..., model: ...}` — no `hideGit` field. It requires no changes. Verify it compiles with `go build ./...`.

## 8. Update `main_internal_test.go`

### 8a. Update the ParseArgs Describe block's `result` struct and `parse` func

Find the `result` struct and `parse` func inside the `Describe("ParseArgs", ...)` block (~line 70). Update:

```go
type result struct {
    debug         bool
    command       string
    subcommand    string
    args          []string
    autoApprove   bool
    skipPreflight bool
    model         string
}
parse := func(rawArgs []string) result {
    debug, command, subcommand, args, autoApprove, skipPreflight, model := ParseArgs(rawArgs)
    return result{debug, command, subcommand, args, autoApprove, skipPreflight, model}
}
```

Existing `It` blocks in the ParseArgs Describe block do not assert `hideGit`, so their bodies need no changes.

### 8b. Remove `hideGit`-related tests from `applyArgOverrides` Describe block (~line 248)

Remove these `It` blocks entirely:
- `"applies hideGit=true override"` — removes `applyArgOverrides(ctx, &cfg, &sources, "run", &t, "")` style call
- `"applies hideGit=false override"` — same pattern
- `"rejects --hide-git on non-run/daemon command"` — tests removed flag

### 8c. Update surviving `applyArgOverrides` test signatures

The remaining tests call `applyArgOverrides` with the old 6-argument signature. Update each to use the new 5-argument signature (remove the `nil` or `&t`/`&f` hideGit argument):

Old:
```go
applyArgOverrides(ctx, &cfg, &sources, "daemon", nil, "claude-opus-4-7")
```
New:
```go
applyArgOverrides(ctx, &cfg, &sources, "daemon", "claude-opus-4-7")
```

Old:
```go
applyArgOverrides(ctx, &cfg, &sources, "config", nil, "claude-opus-4-7")
```
New:
```go
applyArgOverrides(ctx, &cfg, &sources, "config", "claude-opus-4-7")
```

Old:
```go
applyArgOverrides(ctx, &cfg, &sources, "run", nil, "claude;bad")
```
New:
```go
applyArgOverrides(ctx, &cfg, &sources, "run", "claude;bad")
```

Search for ALL remaining `applyArgOverrides` call sites:
```bash
grep -n "applyArgOverrides" main_internal_test.go
```
Update every call.

### 8d. Add `parseSetFlags` Ginkgo tests

Add a new Describe block in `main_internal_test.go`:

```go
var _ = Describe("parseSetFlags", func() {
    ctx := context.Background()

    It("returns empty map and unchanged args when no --set flags", func() {
        overrides, filtered, err := parseSetFlags(ctx, []string{"run", "--model", "claude-opus-4-7"})
        Expect(err).NotTo(HaveOccurred())
        Expect(overrides).To(BeEmpty())
        Expect(filtered).To(Equal([]string{"run", "--model", "claude-opus-4-7"}))
    })

    It("collects a single --set key=value", func() {
        overrides, filtered, err := parseSetFlags(ctx, []string{"run", "--set", "hideGit=true"})
        Expect(err).NotTo(HaveOccurred())
        Expect(overrides).To(HaveKeyWithValue("hideGit", "true"))
        Expect(filtered).To(Equal([]string{"run"}))
    })

    It("collects multiple distinct --set keys", func() {
        overrides, filtered, err := parseSetFlags(ctx, []string{
            "--set", "hideGit=false",
            "--set", "autoRelease=true",
            "run",
        })
        Expect(err).NotTo(HaveOccurred())
        Expect(overrides).To(HaveKeyWithValue("hideGit", "false"))
        Expect(overrides).To(HaveKeyWithValue("autoRelease", "true"))
        Expect(filtered).To(Equal([]string{"run"}))
    })

    It("last value wins for duplicate keys", func() {
        overrides, _, err := parseSetFlags(ctx, []string{
            "--set", "hideGit=true",
            "--set", "hideGit=false",
        })
        Expect(err).NotTo(HaveOccurred())
        Expect(overrides).To(HaveKeyWithValue("hideGit", "false"))
    })

    It("handles value containing = (e.g. model with docker image ref)", func() {
        overrides, _, err := parseSetFlags(ctx, []string{"--set", "model=docker.io/foo:v1=extra"})
        Expect(err).NotTo(HaveOccurred())
        Expect(overrides).To(HaveKeyWithValue("model", "docker.io/foo:v1=extra"))
    })

    It("returns error when --set has no following value", func() {
        _, _, err := parseSetFlags(ctx, []string{"--set"})
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("--set requires a value"))
    })

    It("returns error when --set value has no = sign", func() {
        _, _, err := parseSetFlags(ctx, []string{"--set", "hideGit"})
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("key=value"))
    })

    It("returns error when --set key is empty", func() {
        _, _, err := parseSetFlags(ctx, []string{"--set", "=true"})
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("key must not be empty"))
    })
})
```

### 8e. Add `applySetOverrides` Ginkgo tests

Add a new Describe block in `main_internal_test.go`:

```go
var _ = Describe("applySetOverrides", func() {
    ctx := context.Background()

    It("does nothing when overrides map is empty", func() {
        cfg := config.Defaults()
        sources := config.FieldSources{}
        Expect(applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{})).To(Succeed())
        Expect(sources.HideGit).To(BeEmpty())
    })

    It("sets hideGit=true and marks source=arg", func() {
        cfg := config.Defaults()
        sources := config.FieldSources{}
        Expect(applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"hideGit": "true"})).To(Succeed())
        Expect(cfg.HideGit).To(BeTrue())
        Expect(sources.HideGit).To(Equal("arg"))
    })

    It("sets hideGit=false and marks source=arg", func() {
        cfg := config.Defaults()
        cfg.HideGit = true
        sources := config.FieldSources{}
        Expect(applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"hideGit": "false"})).To(Succeed())
        Expect(cfg.HideGit).To(BeFalse())
        Expect(sources.HideGit).To(Equal("arg"))
    })

    It("rejects hideGit=yes (strict bool)", func() {
        cfg := config.Defaults()
        sources := config.FieldSources{}
        err := applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"hideGit": "yes"})
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("invalid bool"))
        Expect(err.Error()).To(ContainSubstring("true or false"))
    })

    It("sets autoRelease=false and marks source=arg", func() {
        cfg := config.Defaults()
        cfg.AutoRelease = true
        sources := config.FieldSources{}
        Expect(applySetOverrides(ctx, &cfg, &sources, "daemon", map[string]string{"autoRelease": "false"})).To(Succeed())
        Expect(cfg.AutoRelease).To(BeFalse())
        Expect(sources.AutoRelease).To(Equal("arg"))
    })

    It("sets dirtyFileThreshold=5 and marks source=arg", func() {
        cfg := config.Defaults()
        sources := config.FieldSources{}
        Expect(applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"dirtyFileThreshold": "5"})).To(Succeed())
        Expect(cfg.DirtyFileThreshold).To(Equal(5))
        Expect(sources.DirtyFileThreshold).To(Equal("arg"))
    })

    It("rejects dirtyFileThreshold=-1 (range check)", func() {
        cfg := config.Defaults()
        sources := config.FieldSources{}
        err := applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"dirtyFileThreshold": "-1"})
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("dirtyFileThreshold must be >= 0"))
    })

    It("rejects dirtyFileThreshold=abc (parse error)", func() {
        cfg := config.Defaults()
        sources := config.FieldSources{}
        err := applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"dirtyFileThreshold": "abc"})
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("invalid integer"))
    })

    It("sets model=claude-opus-4-7 and marks source=arg", func() {
        cfg := config.Defaults()
        sources := config.FieldSources{}
        Expect(applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"model": "claude-opus-4-7"})).To(Succeed())
        Expect(cfg.Model).To(Equal("claude-opus-4-7"))
        Expect(sources.Model).To(Equal("arg"))
    })

    It("rejects model with shell metachar", func() {
        cfg := config.Defaults()
        sources := config.FieldSources{}
        err := applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"model": "claude;rm -rf /"})
        Expect(err).To(HaveOccurred())
    })

    It("sets maxContainers=2", func() {
        cfg := config.Defaults()
        sources := config.FieldSources{}
        Expect(applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"maxContainers": "2"})).To(Succeed())
        Expect(cfg.MaxContainers).To(Equal(2))
    })

    It("rejects maxContainers=0 (range check)", func() {
        cfg := config.Defaults()
        sources := config.FieldSources{}
        err := applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"maxContainers": "0"})
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("maxContainers must be >= 1"))
    })

    It("rejects unknown key and lists supported keys", func() {
        cfg := config.Defaults()
        sources := config.FieldSources{}
        err := applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"unknownKey": "foo"})
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("unknown config key"))
        Expect(err.Error()).To(ContainSubstring("hideGit"))
        Expect(err.Error()).To(ContainSubstring("autoRelease"))
    })

    It("rejects --set on non-run/daemon command", func() {
        cfg := config.Defaults()
        sources := config.FieldSources{}
        err := applySetOverrides(ctx, &cfg, &sources, "status", map[string]string{"hideGit": "true"})
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("unknown flag: --set"))
    })
})
```

## 9. Write CHANGELOG entry

Add to `## Unreleased` at the top of `CHANGELOG.md`. If `## Unreleased` does not exist yet, create it. Append these lines:

```
- feat: add --set key=value CLI flag to run and daemon for per-invocation config override (supported keys: hideGit, autoRelease, dirtyFileThreshold, model, maxContainers)
- BREAKING: remove --hide-git and --no-hide-git flags; use --set hideGit=true / --set hideGit=false instead
```

## 10. Run `make test`

```bash
cd /workspace && make test
```

All tests must pass. Fix any compilation errors or test failures before proceeding.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do NOT touch `go.mod` / `go.sum` / `vendor/`
- `--model NAME` and `--max-containers N` dedicated flags must continue to work exactly as today — do NOT remove them
- `parseSetFlags` must call `strings.SplitN(val, "=", 2)` so values containing `=` (like Docker image refs) work correctly
- Strict bool parsing: only `"true"` and `"false"` are accepted — do NOT use `strconv.ParseBool` (accepts `1`/`0`/`yes`/`no` which diverges from yaml semantics)
- The `--hide-git` / `--no-hide-git` removal must be complete: no production code references remain; `grep -rn '"--hide-git"\|"--no-hide-git"' main.go` must return zero results
- Remove `"slices"` from imports after the `slices.Contains` call is removed — unused imports prevent compilation
- The `supportedSetKeys` variable must be a package-level `var` (not inlined) so the error message and help text both reference the same list
- All new functions must fit within 80 lines to pass the `funlen` linter; split into `applySetOverrides` + `applyOneSetOverride` as shown
- Use `errors.Errorf(ctx, ...)` from `github.com/bborbe/errors` — never `fmt.Errorf`
- Tests use Ginkgo/Gomega in external or internal `_test` packages as appropriate to the file
- The `//nolint:funlen` annotation on `CreateRunner` and `CreateOneShotRunner` in `factory.go` must remain untouched (those files are not changed in this prompt)
- Do NOT update `docs/configuration.md` or `scenarios/013-config-layering.md` in this prompt — that is prompt 2
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Spot checks:
1. `grep -rn '"--hide-git"\|"--no-hide-git"' main.go` — zero results
2. `grep -n "slices\." main.go` — zero results (import removed)
3. `grep -n "parseSetFlags\|applySetOverrides\|supportedSetKeys\|parseStrictBool\|applyOneSetOverride" main.go` — all 5 symbols present
4. `grep -c "return debug" main.go` — should return 7 (all ParseArgs return stmts updated)
5. `grep -n "hideGit" parse_args_test.go` — zero results (struct field and TestParseArgsHideGit removed)
6. `grep -n "TestParseArgsHideGit" parse_args_test.go` — zero results
7. `grep -n "applyArgOverrides" main_internal_test.go | grep -v "hide-git"` — all calls use 5-arg form
8. `grep -n "parseSetFlags\|applySetOverrides" main_internal_test.go` — Describe blocks present
9. `go build ./...` — zero errors
10. `go test ./...` — all tests pass
</verification>
