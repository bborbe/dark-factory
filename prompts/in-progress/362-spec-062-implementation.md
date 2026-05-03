---
status: committing
spec: [062-cli-set-workflow-pr-automerge]
summary: Extended --set CLI flag to accept workflow, pr, and autoMerge keys with source tracking, combination validation, and updated help text and docs
container: dark-factory-362-spec-062-implementation
dark-factory-version: v0.143.0-5-g73d1db8
created: "2026-05-03T09:00:00Z"
queued: "2026-05-03T09:13:08Z"
started: "2026-05-03T09:15:21Z"
branch: dark-factory/spec-062
---

<summary>
- Three new keys (`workflow`, `pr`, `autoMerge`) are accepted by `--set` on `run` and `daemon` commands
- `workflow` accepts the four valid enum values (`direct`, `branch`, `worktree`, `clone`); the legacy `pr` enum value is rejected at the arg layer with a message pointing to `--set workflow=clone --set pr=true`
- `pr` and `autoMerge` accept strict `true`/`false` only, same as existing bool keys
- The effective-config log shows `workflowSource`, `prSource`, `autoMergeSource` in the startup log line; each shows `arg` when the field was overridden via `--set`
- Existing combination validators (`workflow: direct` + `pr: true`; `autoMerge: true` without `pr: true`) run on the post-override config, so invalid `--set` combinations exit non-zero
- `FieldSources` gains three new string fields to track the new keys
- `LayeredProjectOverrides` gains three new pointer fields so project-level values are correctly attributed to "project" in the source log
- Help text for `run` and `daemon` lists all eight supported `--set` keys and adds examples for the new types
- `CHANGELOG.md` gains an `## Unreleased` entry
- All new code paths are covered by Ginkgo/Gomega unit tests in `main_internal_test.go`
</summary>

<objective>
Extend the `--set key=value` CLI flag to accept `workflow`, `pr`, and `autoMerge`. Each key uses the same per-invocation-only override semantics as the existing five keys: coerce at parse time, set `Source=arg` in the effective-config log, and run the standard `Config.Validate` on the fully-merged config so combination errors (e.g. `workflow: direct` + `pr: true`) are caught regardless of which layer supplied each value.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-enum-type-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/`

Read these files in full before editing:
- `main.go` — focus on: `run()` (~line 60–116), `supportedSetKeys` (~line 617–625), `applyOneSetOverride` (~line 686–743), `parseStrictBool` (~line 745–762), `computeFieldSources` (~line 547–586), `printRunHelp` (~line 856–874), `printDaemonHelp` (~line 877–895)
- `main_internal_test.go` — full file; see the `applySetOverrides` Describe block (~line 434) for the exact test pattern to follow
- `pkg/config/sources.go` — `FieldSources` struct (5 fields)
- `pkg/config/loader.go` — `LayeredProjectOverrides` struct (~line 33), `loadWithOverrides` (~line 137), `overrides` capture block (~line 166–173)
- `pkg/factory/factory.go` — `LogEffectiveConfig` (~line 90–133) — the slog.Info call with all field+source pairs
- `pkg/config/workflow.go` — `AvailableWorkflows`, `Workflow.Validate`, `WorkflowPR` constant
- `pkg/config/config.go` — `validateWorkflowPR` and autoMerge validator (~line 211–217)

The spec this implements: `specs/in-progress/062-cli-set-workflow-pr-automerge.md`
The prior spec's prompt (for context): `prompts/completed/360-spec-061-implementation.md`
</context>

<requirements>

## 1. Extend `FieldSources` in `pkg/config/sources.go`

Add three new string fields to the end of `FieldSources`:

```go
type FieldSources struct {
	HideGit            string
	AutoRelease        string
	DirtyFileThreshold string
	Model              string
	MaxContainers      string
	Workflow           string
	PR                 string
	AutoMerge          string
}
```

No other changes to this file.

## 2. Extend `LayeredProjectOverrides` in `pkg/config/loader.go`

### 2a. Add three pointer fields to `LayeredProjectOverrides`

```go
type LayeredProjectOverrides struct {
	HideGit            *bool
	AutoRelease        *bool
	DirtyFileThreshold *int
	Model              *string
	MaxContainers      *int // included for completeness; maxContainers uses its own precedence path
	Workflow           *Workflow // new: non-nil when .dark-factory.yaml explicitly sets workflow
	PR                 *bool     // new: non-nil when .dark-factory.yaml explicitly sets pr
	AutoMerge          *bool     // new: non-nil when .dark-factory.yaml explicitly sets autoMerge
}
```

### 2b. Capture them in `loadWithOverrides`

In the `overrides` struct literal at ~line 167 (immediately after the comment "Capture the 4 layered user-pref fields"), extend it:

```go
overrides := LayeredProjectOverrides{
	HideGit:            partial.HideGit,
	AutoRelease:        partial.AutoRelease,
	DirtyFileThreshold: partial.DirtyFileThreshold,
	Model:              partial.Model,
	MaxContainers:      partial.MaxContainers,
	Workflow:           partial.Workflow,   // new
	PR:                 partial.PR,         // new
	AutoMerge:          partial.AutoMerge, // new
}
```

Note: `partial.Workflow` may be `&WorkflowPR` when the yaml uses the legacy `workflow: pr` value. That is fine — the legacy mapping runs on `cfg` after the capture, and the override pointer just signals "project explicitly set this field". Do not add special handling here.

No other changes to this file.

## 3. Add three new cases to `applyOneSetOverride` in `main.go`

Before the `default:` case in the switch inside `applyOneSetOverride` (~line 734), insert three new cases:

```go
case "workflow":
	// Reject the legacy "pr" enum value at the arg layer. The yaml loader maps it to
	// workflow: clone + pr: true; the arg layer intentionally does not reproduce that mapping.
	if value == string(config.WorkflowPR) {
		return errors.Errorf(
			ctx,
			"legacy workflow value %q not accepted via --set; use --set workflow=clone --set pr=true",
			value,
		)
	}
	w := config.Workflow(value)
	if err := w.Validate(ctx); err != nil {
		return err
	}
	cfg.Workflow = w
	sources.Workflow = "arg"
case "pr":
	b, err := parseStrictBool(ctx, key, value)
	if err != nil {
		return err
	}
	cfg.PR = b
	sources.PR = "arg"
case "autoMerge":
	b, err := parseStrictBool(ctx, key, value)
	if err != nil {
		return err
	}
	cfg.AutoMerge = b
	sources.AutoMerge = "arg"
```

These three cases must be added BEFORE the `default:` case (not after it). The existing `maxContainers` case is currently the last non-default case — insert the three new cases between `maxContainers` and `default`.

## 4. Add "workflow", "pr", "autoMerge" to `supportedSetKeys` in `main.go`

Update the slice:

```go
var supportedSetKeys = []string{
	"hideGit",
	"autoRelease",
	"dirtyFileThreshold",

	"model",
	"maxContainers",

	"workflow",
	"pr",
	"autoMerge",
}
```

Preserve the existing blank-line grouping and add a new group for the three new keys.

## 5. Update `computeFieldSources` in `main.go`

### 5a. Initialise default values for the three new fields

In the `s := config.FieldSources{...}` literal, add:

```go
s := config.FieldSources{
	HideGit:            "default",
	AutoRelease:        "default",
	DirtyFileThreshold: "default",
	Model:              "default",
	Workflow:           "default",
	PR:                 "default",
	AutoMerge:          "default",
}
```

(`MaxContainers` is intentionally left at empty string per the existing comment.)

### 5b. Detect project-level overrides

After the last `if proj.MaxContainers != nil` block (the last existing project-detection block, ~line 582–584), add:

```go
if proj.Workflow != nil {
	s.Workflow = "project"
}
if proj.PR != nil {
	s.PR = "project"
}
if proj.AutoMerge != nil {
	s.AutoMerge = "project"
}
```

`workflow`, `pr`, and `autoMerge` have no global config counterpart (the global config schema does not include them), so no "global" detection block is needed. The three new fields jump directly from "default" to "project" or "arg".

## 6. Add post-override validation in `run()` in `main.go`

After `applySetOverrides` returns successfully (~line 100–102) and before `currentDateTimeGetter := libtime.NewCurrentDateTime()` (~line 104), add:

```go
if command == "run" || command == "daemon" {
	if err := cfg.Validate(ctx); err != nil {
		return err
	}
}
```

This ensures combination errors introduced by `--set` (e.g. `--set workflow=direct --set pr=true`) are caught before any prompt executes. The first `Validate` call happens inside `config.LoadWithOverrides` on the yaml-only config; this second call catches post-arg violations.

**Placement:** The block must appear between `applySetOverrides` and `currentDateTimeGetter`. Do NOT move or delete any other code.

## 7. Update help text in `main.go`

### 7a. `printRunHelp`

The `--set` block in `printRunHelp` already has these lines (verbatim):

```go
"  --set key=value         Override a config field for this invocation; may repeat\n"+
"                          Supported keys: hideGit, autoRelease, dirtyFileThreshold, model, maxContainers\n"+
"                          Bool example:   --set hideGit=true\n"+
"                          Int example:    --set dirtyFileThreshold=5\n"+
"                          String example: --set model=claude-opus-4-7\n"+
"                          Note: --max-containers N takes precedence over --set maxContainers=N if both are passed.\n"+
```

Make these targeted edits (do NOT replace the entire block):

1. On the `Supported keys:` line, append `, workflow, pr, autoMerge` so it ends with `..., maxContainers, workflow, pr, autoMerge`.
2. On the `Bool example:` line, append `  --set pr=true  --set autoMerge=false`.
3. On the `String example:` line, append `  --set workflow=branch`.
4. Insert two new lines BETWEEN the existing `String example:` line and the existing `Note: --max-containers N` line:
   ```go
   "                          Workflow example: --set workflow=branch --set pr=true\n"+
   "                          Note: 'workflow: pr' is yaml-only legacy; use --set workflow=clone --set pr=true\n"+
   ```

Keep all other lines unchanged. The `Note: --max-containers N` line and the trailing `--help, -h` line stay in place.

### 7b. `printDaemonHelp`

Apply the identical change to `printDaemonHelp`. The two help functions have mirrored `--set` sections; both must be updated.

## 8. Update `LogEffectiveConfig` in `pkg/factory/factory.go`

The `slog.Info("effective config", ...)` call already logs `workflow`, `pr`, `autoMerge`, `autoRelease` etc. but only some have a paired `Source` field. The relevant existing lines (verbatim from `pkg/factory/factory.go:113-117`):

```go
"workflow", cfg.Workflow,
"pr", cfg.PR,
"autoRelease", cfg.AutoRelease,
"autoReleaseSource", sources.AutoRelease,
"autoMerge", cfg.AutoMerge,
```

Make three targeted insertions:

1. After `"workflow", cfg.Workflow,` insert: `"workflowSource", sources.Workflow,`
2. After `"pr", cfg.PR,` insert: `"prSource", sources.PR,`
3. After `"autoMerge", cfg.AutoMerge,` insert: `"autoMergeSource", sources.AutoMerge,`

Keep all other pairs in their existing order. Do NOT change the function signature — `sources config.FieldSources` is already a parameter.

## 9. Add tests to `main_internal_test.go`

### 9a. Tests for new `applySetOverrides` cases

Append the following It-blocks to the existing `Describe("applySetOverrides", ...)` block (after the last existing It block):

```go
It("sets workflow=branch and marks source=arg", func() {
    cfg := config.Defaults()
    sources := config.FieldSources{}
    Expect(
        applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"workflow": "branch"}),
    ).To(Succeed())
    Expect(cfg.Workflow).To(Equal(config.WorkflowBranch))
    Expect(sources.Workflow).To(Equal("arg"))
})

It("sets workflow=clone and marks source=arg", func() {
    cfg := config.Defaults()
    sources := config.FieldSources{}
    Expect(
        applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"workflow": "clone"}),
    ).To(Succeed())
    Expect(cfg.Workflow).To(Equal(config.WorkflowClone))
    Expect(sources.Workflow).To(Equal("arg"))
})

It("rejects workflow=invalid with error listing valid values", func() {
    cfg := config.Defaults()
    sources := config.FieldSources{}
    err := applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"workflow": "invalid"})
    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("unknown workflow"))
    Expect(err.Error()).To(ContainSubstring("direct"))
    Expect(err.Error()).To(ContainSubstring("branch"))
    Expect(err.Error()).To(ContainSubstring("worktree"))
    Expect(err.Error()).To(ContainSubstring("clone"))
})

It("rejects workflow=pr (legacy enum) with message pointing to clone+pr", func() {
    cfg := config.Defaults()
    sources := config.FieldSources{}
    err := applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"workflow": "pr"})
    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("legacy workflow value"))
    Expect(err.Error()).To(ContainSubstring("workflow=clone"))
    Expect(err.Error()).To(ContainSubstring("pr=true"))
})

It("sets pr=true and marks source=arg", func() {
    cfg := config.Defaults()
    cfg.Workflow = config.WorkflowBranch // pr=true requires non-direct workflow
    sources := config.FieldSources{}
    Expect(
        applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"pr": "true"}),
    ).To(Succeed())
    Expect(cfg.PR).To(BeTrue())
    Expect(sources.PR).To(Equal("arg"))
})

It("sets pr=false and marks source=arg", func() {
    cfg := config.Defaults()
    cfg.PR = true
    cfg.Workflow = config.WorkflowBranch
    sources := config.FieldSources{}
    Expect(
        applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"pr": "false"}),
    ).To(Succeed())
    Expect(cfg.PR).To(BeFalse())
    Expect(sources.PR).To(Equal("arg"))
})

It("rejects pr=yes (invalid bool)", func() {
    cfg := config.Defaults()
    sources := config.FieldSources{}
    err := applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"pr": "yes"})
    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("invalid bool"))
    Expect(err.Error()).To(ContainSubstring("true or false"))
})

It("sets autoMerge=true and marks source=arg", func() {
    cfg := config.Defaults()
    cfg.PR = true
    cfg.Workflow = config.WorkflowBranch
    sources := config.FieldSources{}
    Expect(
        applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"autoMerge": "true"}),
    ).To(Succeed())
    Expect(cfg.AutoMerge).To(BeTrue())
    Expect(sources.AutoMerge).To(Equal("arg"))
})

It("rejects autoMerge=1 (invalid bool)", func() {
    cfg := config.Defaults()
    sources := config.FieldSources{}
    err := applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"autoMerge": "1"})
    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("invalid bool"))
})

It("workflow=direct pr=true combination fails cfg.Validate after both applied", func() {
    cfg := config.Defaults() // Workflow=direct (default)
    sources := config.FieldSources{}
    Expect(
        applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"pr": "true"}),
    ).To(Succeed()) // applySetOverrides itself succeeds — single-key validation only
    // The cross-field validator fires on cfg.Validate (called by run() after applySetOverrides)
    err := cfg.Validate(ctx)
    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("incompatible"))
})

It("autoMerge=true without pr=true fails cfg.Validate after applied", func() {
    cfg := config.Defaults() // PR=false (default)
    sources := config.FieldSources{}
    Expect(
        applySetOverrides(ctx, &cfg, &sources, "run", map[string]string{"autoMerge": "true"}),
    ).To(Succeed())
    err := cfg.Validate(ctx)
    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("autoMerge requires pr: true"))
})

It("last workflow wins when key appears twice", func() {
    cfg := config.Defaults()
    sources := config.FieldSources{}
    // Simulate last-wins by applying two separate calls (map iteration order is undefined;
    // use the parseSetFlags result directly for deterministic ordering in unit tests)
    Expect(applyOneSetOverride(ctx, &cfg, &sources, "workflow", "branch")).To(Succeed())
    Expect(applyOneSetOverride(ctx, &cfg, &sources, "workflow", "clone")).To(Succeed())
    Expect(cfg.Workflow).To(Equal(config.WorkflowClone))
    Expect(sources.Workflow).To(Equal("arg"))
})
```

### 9b. Tests for new `computeFieldSources` cases

Append to the existing `Describe("computeFieldSources", ...)` block:

```go
It("returns default for workflow/pr/autoMerge when project did not set them", func() {
    global := globalconfig.GlobalConfig{MaxContainers: 3}
    proj := config.LayeredProjectOverrides{}
    s := computeFieldSources(global, proj)
    Expect(s.Workflow).To(Equal("default"))
    Expect(s.PR).To(Equal("default"))
    Expect(s.AutoMerge).To(Equal("default"))
})

It("returns project for workflow when project explicitly sets it", func() {
    global := globalconfig.GlobalConfig{MaxContainers: 3}
    w := config.WorkflowBranch
    proj := config.LayeredProjectOverrides{Workflow: &w}
    s := computeFieldSources(global, proj)
    Expect(s.Workflow).To(Equal("project"))
})

It("returns project for pr when project explicitly sets it", func() {
    global := globalconfig.GlobalConfig{MaxContainers: 3}
    t := true
    proj := config.LayeredProjectOverrides{PR: &t}
    s := computeFieldSources(global, proj)
    Expect(s.PR).To(Equal("project"))
})

It("returns project for autoMerge when project explicitly sets it", func() {
    global := globalconfig.GlobalConfig{MaxContainers: 3}
    t := true
    proj := config.LayeredProjectOverrides{AutoMerge: &t}
    s := computeFieldSources(global, proj)
    Expect(s.AutoMerge).To(Equal("project"))
})
```

## 10. Add CHANGELOG entry

At the top of `CHANGELOG.md`, add a new `## Unreleased` section before the first `## vX.Y.Z` section:

```markdown
## Unreleased

- feat: --set now accepts workflow, pr, autoMerge keys for per-invocation delivery override
```

If `## Unreleased` already exists, append the bullet to it (do not create a duplicate section).

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Three new keys (`workflow`, `pr`, `autoMerge`) are recognized by `--set`; unknown keys remain rejected with the same error as today
- `workflow` accepts the four valid enum values (`direct`, `branch`, `worktree`, `clone`); the legacy `pr` enum value is rejected at the arg layer (not silently mapped)
- `pr` and `autoMerge` accept strict `true`/`false` only — same bool semantics as existing `--set` keys
- The combination validators already in `Config.Validate` fire on the post-`--set` merged config; the validate call in `run()` is the mechanism
- Errors use `errors.Errorf(ctx, ...)` from `github.com/bborbe/errors` — never `fmt.Errorf`
- Tests use Ginkgo/Gomega in the existing `package main` internal test file; do not create a new test file
- Do not touch `go.mod` / `go.sum` / `vendor/`
- Existing tests must still pass — do not delete or modify any existing test case
- The `FieldSources` zero value (empty string) is treated as "default" by callers; setting it to "default" explicitly in `computeFieldSources` matches the existing pattern for the other four fields
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional spot checks:
1. `grep -n "Workflow\|PR\b\|AutoMerge" pkg/config/sources.go` — three new fields present
2. `grep -n "Workflow\|PR\b\|AutoMerge" pkg/config/loader.go` — three new fields in `LayeredProjectOverrides` and capture block
3. `grep -n '"workflow"\|"pr"\|"autoMerge"' main.go` — three new cases in `applyOneSetOverride` switch and three new entries in `supportedSetKeys`
4. `grep -n 'cfg.Validate' main.go` — one occurrence after `applySetOverrides`
5. `grep -n "workflowSource\|prSource\|autoMergeSource" pkg/factory/factory.go` — three new source fields in slog.Info call
6. `grep -c "workflow\|autoMerge\|legacy" main_internal_test.go` — at least 12 matches (new test cases)
7. `grep -A1 "## Unreleased" CHANGELOG.md` — shows the new --set entry
</verification>
