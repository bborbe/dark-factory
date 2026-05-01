---
status: draft
spec: [060-config-layering-phase-1]
created: "2026-05-01T08:00:00Z"
---

<summary>
- The user-level config file `~/.dark-factory/config.yaml` accepts four new fields beyond `maxContainers`: `hideGit`, `autoRelease`, `dirtyFileThreshold`, and `model`.
- Each new field is optional; absence means the user expressed no opinion at the global layer.
- The global config validates each field when set: `dirtyFileThreshold` must be `>= 0`, `model` must be non-empty and match a strict regex that blocks shell metacharacters.
- `maxContainers` validation and behavior are unchanged.
- Loading an empty or absent global config still returns defaults — no behavior change for operators with no global config.
- Counterfeiter mocks regenerate cleanly.
</summary>

<objective>
Extend `pkg/globalconfig.GlobalConfig` with four new optional fields (`hideGit`, `autoRelease`, `dirtyFileThreshold`, `model`) and validate each. This prompt only touches `pkg/globalconfig`. The project-level merge and CLI flags are subsequent prompts.
</objective>

<context>
Read `CLAUDE.md` for project conventions (errors, Ginkgo/Gomega, Counterfeiter, no commits).

Read these guides in `~/.claude/plugins/marketplaces/coding/docs/`:
- `go-validation-framework-guide.md`
- `go-testing-guide.md`
- `go-mocking-guide.md`

Read these files before editing:
- `pkg/globalconfig/globalconfig.go` — current struct, Validate, fileLoader.Load
- `pkg/globalconfig/globalconfig_test.go` — existing tests (Ginkgo/Gomega in external `_test` package)
- `pkg/config/config.go` — for reference: how Config.Validate handles `dirtyFileThreshold` and `model` validation today

Spec: `specs/in-progress/060-config-layering-phase-1.md` — desired behaviors 1, 6 and the constraint about pointer-sentinel semantics.
</context>

<requirements>

## 1. Add new fields to `GlobalConfig`

In `pkg/globalconfig/globalconfig.go`, extend the `GlobalConfig` struct:

```go
type GlobalConfig struct {
    MaxContainers      int     `yaml:"maxContainers"`
    HideGit            *bool   `yaml:"hideGit"`
    AutoRelease        *bool   `yaml:"autoRelease"`
    DirtyFileThreshold *int    `yaml:"dirtyFileThreshold"`
    Model              *string `yaml:"model"`
}
```

The new fields are pointer types so `nil` distinguishes "operator did not set it" from "operator explicitly set it to the zero value (false / 0 / empty string)". Existing `MaxContainers int` stays unchanged — its zero value already means "use default" via existing `EffectiveMaxContainers` logic in factory.

Document each new field with a short GoDoc comment explaining what `nil` means.

## 2. Add a model regex constant

Add a package-level constant:

```go
// ModelPattern is the regex that valid model identifiers must match.
// It allows Anthropic IDs (claude-opus-4-7), OSS IDs (qwen3.6:35b-a3b),
// namespaced paths (local/qwen3.6:35b-a3b), and Docker image refs
// (docker.io/bborbe/claude-yolo:v0.6.1). Shell metacharacters and
// whitespace are rejected because the value flows to YOLO container args.
const ModelPattern = `^[a-zA-Z0-9._:/-]{1,256}$`
```

Compile the pattern once in a package-level `var modelRegex = regexp.MustCompile(ModelPattern)`. Add the `regexp` import.

## 3. Extend `GlobalConfig.Validate`

Update `Validate` to validate the new fields when set:

```go
func (g GlobalConfig) Validate(ctx context.Context) error {
    if g.MaxContainers < 1 {
        return errors.Errorf(
            ctx,
            "globalconfig: maxContainers must be >= 1, got %d",
            g.MaxContainers,
        )
    }
    if g.DirtyFileThreshold != nil && *g.DirtyFileThreshold < 0 {
        return errors.Errorf(
            ctx,
            "globalconfig: dirtyFileThreshold must be >= 0, got %d",
            *g.DirtyFileThreshold,
        )
    }
    if g.Model != nil {
        if *g.Model == "" {
            return errors.Errorf(
                ctx,
                "globalconfig: model must not be empty when set",
            )
        }
        if !modelRegex.MatchString(*g.Model) {
            return errors.Errorf(
                ctx,
                "globalconfig: model %q does not match required pattern %s",
                *g.Model,
                ModelPattern,
            )
        }
    }
    return nil
}
```

`HideGit` and `AutoRelease` need no validation: any bool is acceptable.

## 4. Extend the partial-struct unmarshal in `Load`

In `(*fileLoader).Load`, expand the local `partial` struct to include the new fields and dereference each into `cfg`:

```go
var partial struct {
    MaxContainers      *int    `yaml:"maxContainers"`
    HideGit            *bool   `yaml:"hideGit"`
    AutoRelease        *bool   `yaml:"autoRelease"`
    DirtyFileThreshold *int    `yaml:"dirtyFileThreshold"`
    Model              *string `yaml:"model"`
}
if err := yaml.Unmarshal(data, &partial); err != nil {
    return GlobalConfig{}, errors.Wrap(ctx, err, "globalconfig: parse config file")
}

if partial.MaxContainers != nil {
    cfg.MaxContainers = *partial.MaxContainers
}
if partial.HideGit != nil {
    cfg.HideGit = partial.HideGit
}
if partial.AutoRelease != nil {
    cfg.AutoRelease = partial.AutoRelease
}
if partial.DirtyFileThreshold != nil {
    cfg.DirtyFileThreshold = partial.DirtyFileThreshold
}
if partial.Model != nil {
    cfg.Model = partial.Model
}
```

(For pointer fields, assigning `partial.Field` directly preserves the pointer — no double-deref needed.)

## 5. Update `defaults()`

Existing `defaults()` returns `GlobalConfig{MaxContainers: DefaultMaxContainers}`. Leave this unchanged — the new pointer fields default to `nil` (the zero value of `*T`), which is the correct semantic for "not set globally".

## 6. Update tests in `pkg/globalconfig/globalconfig_test.go`

The test file already covers `MaxContainers`. Add new `Describe` blocks (or extend the existing one) for the new fields. Use Ginkgo/Gomega in the external `_test` package.

### 6a. Test parsing each new field

- `hideGit: true` → `*cfg.HideGit == true`
- `hideGit: false` → `*cfg.HideGit == false` (explicit false is set, not nil)
- field absent → `cfg.HideGit == nil`
- Same shape for `autoRelease`, `dirtyFileThreshold`, `model`

### 6b. Test validation rejects bad values

- `dirtyFileThreshold: -5` → Load returns error mentioning "dirtyFileThreshold"
- `model: ""` → Load returns error mentioning "must not be empty"
- `model: "foo;rm -rf /"` → Load returns error mentioning "pattern"
- `model: "foo bar"` (with space) → Load returns error mentioning "pattern"
- `model: "claude-opus-4-7"` → Load succeeds
- `model: "qwen3.6:35b-a3b"` → Load succeeds
- `model: "local/qwen3.6:35b-a3b"` → Load succeeds
- `model: "docker.io/bborbe/claude-yolo:v0.6.1"` → Load succeeds

### 6c. Test absent file still returns defaults

The existing "config file does not exist" test must still pass. New fields should be nil. Add a check that `cfg.HideGit == nil` etc.

### 6d. Test empty file still returns defaults

Same as above for an empty config file. New fields should be nil.

Use `os.MkdirTemp` + override `userHomeDir` (existing test pattern). Do not write production code paths into tests — use the existing test infrastructure.

## 7. Regenerate counterfeiter mocks

```bash
cd /workspace && make generate
```

The `Loader` interface signature is unchanged, so the existing mock at `mocks/global-config-loader.go` should regenerate identically. Verify with `git diff mocks/`.

## 8. Run validation

```bash
cd /workspace
make generate
make precommit
```

Both must exit 0.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Do NOT touch `go.mod` / `go.sum` / `vendor/`.
- Do NOT modify `pkg/config/config.go`, `pkg/config/loader.go`, `pkg/factory/factory.go`, or `main.go` in this prompt — those are addressed in subsequent prompts (2 and 3).
- Existing behavior for `MaxContainers` must be unchanged. Tests asserting current `MaxContainers` precedence (in `pkg/globalconfig/globalconfig_test.go` and elsewhere) must continue to pass.
- Use pointer types (`*bool`, `*int`, `*string`) for the new GlobalConfig fields to distinguish "operator set this" from "operator did not set it". Do NOT use sentinel zero values for the new fields.
- Use `errors.Errorf(ctx, ...)` from `github.com/bborbe/errors` for any new error creation — never `fmt.Errorf`.
- The model regex must be exactly `^[a-zA-Z0-9._:/-]{1,256}$`. Compile once via `regexp.MustCompile` at package scope. Validation happens uniformly in this package (and later in the project loader and CLI flag parsing — same regex).
- The `Loader` interface signature `Load(ctx context.Context) (GlobalConfig, error)` does NOT change in this prompt.
- `defaults()` does NOT change — new pointer fields default to nil.
- Tests use Ginkgo/Gomega in external `_test` package (`package globalconfig_test`).
- All file changes confined to `pkg/globalconfig/`.
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Spot checks:

```bash
# New struct fields exist
grep -n "HideGit\s*\*bool\|AutoRelease\s*\*bool\|DirtyFileThreshold\s*\*int\|Model\s*\*string" pkg/globalconfig/globalconfig.go

# Regex constant defined and used
grep -n "ModelPattern\|modelRegex" pkg/globalconfig/globalconfig.go

# Validate covers new fields
grep -n "dirtyFileThreshold\|model.*empty\|model.*pattern" pkg/globalconfig/globalconfig.go

# Tests cover new fields
grep -n "hideGit\|autoRelease\|dirtyFileThreshold\|model" pkg/globalconfig/globalconfig_test.go

# No changes outside pkg/globalconfig
git diff --name-only | grep -v "^pkg/globalconfig/\|^mocks/global-config-loader.go" || echo "scope OK"
```

```bash
make generate
make precommit
```
</verification>
