---
status: created
spec: ["032"]
created: "2026-03-16T12:00:00Z"
---
<summary>
- Projects can configure AI-judged quality criteria via a new `validationPrompt` config field
- The field accepts either a file path (relative to project root) or inline criteria text
- Absolute paths are rejected at config validation with an error message
- Paths containing `..` that would traverse outside the project root are rejected at config validation
- Empty value (default) means no AI evaluation — zero overhead, no behavioral change
- The field is documented alongside `validationCommand` in the config struct
- Config tests cover the default value, valid inline text, valid relative paths, and all rejection cases
</summary>

<objective>
Add the `validationPrompt` string field to the `Config` struct and implement path-safety validation.
This is the first of two prompts for spec 032: it establishes the config contract so the second
prompt can wire the field into the processor without touching config concerns.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/config/config.go` — full file. The new field and its validation follow the pattern of
`ValidationCommand` (line 77). The `Validate` method (line 131) is where the path-safety rule goes.
Read `pkg/config/config_test.go` — understand existing test patterns before adding new ones.
The `validation.Name` + `validation.HasValidationFunc` pattern is already used in `Validate()` — see the `debounceMs` and `autoMerge` entries as examples.
</context>

<requirements>
1. **`pkg/config/config.go`** — Add field to `Config` struct:
   - Add `ValidationPrompt string \`yaml:"validationPrompt"\`` after the `ValidationCommand` field (line 77).
   - Do NOT add a default value in `Defaults()` — empty string is the default (disables evaluation).

2. **`pkg/config/config.go`** — Add path-safety validation in `Validate()`:
   - Add a new entry to the `validation.All{}` block:
     ```go
     validation.Name("validationPrompt", validation.HasValidationFunc(c.validateValidationPrompt)),
     ```
   - Implement `validateValidationPrompt` as a private method:
     ```go
     // validateValidationPrompt rejects absolute paths and paths that traverse outside the project root.
     func (c Config) validateValidationPrompt(ctx context.Context) error {
         v := c.ValidationPrompt
         if v == "" {
             return nil
         }
         if filepath.IsAbs(v) {
             return errors.Errorf(ctx, "validationPrompt must be a relative path or inline text, got absolute path: %q", v)
         }
         // Reject paths with .. that would escape the project root
         cleaned := filepath.Clean(v)
         if strings.HasPrefix(cleaned, "..") {
             return errors.Errorf(ctx, "validationPrompt path must not traverse outside the project root: %q", v)
         }
         return nil
     }
     ```
   - Note: `strings` is already imported. You must add `"path/filepath"` to the import block.

3. **`pkg/config/config_test.go`** — Add tests for the new validation:
   - `ValidationPrompt` defaults to `""` — assert `Defaults().ValidationPrompt == ""`.
   - Inline text `"readme.md is updated"` — `Validate()` must return nil (no path separators, not absolute).
   - Relative path `"docs/dod.md"` — `Validate()` must return nil (relative, no traversal).
   - Absolute path `"/etc/passwd"` — `Validate()` must return a non-nil error containing `"absolute path"`.
   - Traversal path `"../outside.md"` — `Validate()` must return a non-nil error containing `"outside the project root"`.
   - Traversal path `"../../etc/passwd"` — same rejection.
   - Empty string `""` — `Validate()` must return nil.

   Use the existing test suite setup (`var _ = Describe("Config", ...)`) and `Gomega` matchers.
</requirements>

<constraints>
- Config field name is `validationPrompt` — exact casing, parallel to `validationCommand`
- Empty string is valid and is the default — no default value in `Defaults()`
- Reject ONLY absolute paths and relative paths that escape the project root via `..`
- A value like `"readme.md is updated"` (inline text with spaces) must pass validation even though it's not a real path — the resolver (in the next prompt) decides whether it's a file or inline text at execution time
- Do NOT attempt to stat the file at config validation time — the file may not exist yet or on the validating machine
- Do NOT modify any processor, factory, or report code — that is the second prompt's responsibility
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- `make precommit` must pass
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
