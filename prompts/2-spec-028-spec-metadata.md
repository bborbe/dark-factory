---
spec: ["028"]
status: created
created: "2026-03-10T19:39:55Z"
---
<summary>
- Spec frontmatter gains two optional fields: `branch` (git branch name) and `issue` (freeform issue tracker reference)
- When a spec is approved, a branch name is auto-generated from the spec number (e.g., spec `028-my-feature.md` → `dark-factory/spec-028`) and stored in the spec frontmatter
- If the spec's frontmatter already has a `branch` value, re-approving it preserves the existing value rather than overwriting it
- Branch names are validated against git's allowed ref format — names with `..`, leading `-`, spaces, or shell metacharacters are rejected at approve time
- The `issue` field is stored as a literal string — never shell-interpolated or expanded
- Existing specs without these fields continue to work unchanged
</summary>

<objective>
Add optional `branch` and `issue` metadata to spec frontmatter, and make `spec approve` auto-assign a branch name derived from the spec number — storing it in the frontmatter so downstream prompts can inherit it.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/spec/spec.go` — `Frontmatter` struct (~line 42), `SpecFile` (~line 52), `Load()` (~line 80), `SetStatus()` (~line 114), `Save()` (~line 129). Understand how frontmatter fields are marshaled to YAML.
Read `pkg/cmd/spec_approve.go` — `specApproveCommand`, `Run()` (~line 50): this is where approve happens, spec is loaded, status set, saved, then moved to inProgressDir. The branch auto-generation and storage must happen here, before `Save()`.
Read `pkg/specnum/` — how spec numbers are parsed from filenames. The `specnum.Parse()` function returns the integer prefix.
Read `pkg/cmd/spec_approve_test.go` — existing tests for spec approve.
Read `pkg/spec/spec_test.go` — existing spec tests.
</context>

<requirements>
1. **Add `Branch` and `Issue` to `spec.Frontmatter`** in `pkg/spec/spec.go`:
   ```go
   type Frontmatter struct {
       Status    string   `yaml:"status"`
       Tags      []string `yaml:"tags,omitempty"`
       Approved  string   `yaml:"approved,omitempty"`
       Prompted  string   `yaml:"prompted,omitempty"`
       Verifying string   `yaml:"verifying,omitempty"`
       Completed string   `yaml:"completed,omitempty"`
       Branch    string   `yaml:"branch,omitempty"`
       Issue     string   `yaml:"issue,omitempty"`
   }
   ```

2. **Add `SetBranch(branch string)` method to `SpecFile`** in `pkg/spec/spec.go` — sets `s.Frontmatter.Branch = branch` only if the current value is empty (never overwrites an existing value):
   ```go
   func (s *SpecFile) SetBranchIfEmpty(branch string) {
       if s.Frontmatter.Branch == "" {
           s.Frontmatter.Branch = branch
       }
   }
   ```

3. **Add `AutoBranchName(name string) string` function** in `pkg/spec/spec.go` (package-level helper) that generates the canonical branch name from a spec file's basename (without `.md` extension):
   - Parse the numeric prefix from the name using `specnum.Parse()`
   - If a numeric prefix exists (≥ 0), return `"dark-factory/spec-" + fmt.Sprintf("%03d", num)`
   - If no numeric prefix, return `"dark-factory/" + name` (sanitized — replace spaces and special chars with `-`)
   ```go
   func AutoBranchName(name string) string {
       num := specnum.Parse(name)
       if num >= 0 {
           return fmt.Sprintf("dark-factory/spec-%03d", num)
       }
       // Sanitize: replace non-alphanumeric/hyphen/slash/underscore with "-"
       safe := regexp.MustCompile(`[^a-zA-Z0-9\-_/]`).ReplaceAllString(name, "-")
       return "dark-factory/" + safe
   }
   ```

4. **Add `ValidateBranchName(ctx context.Context, branch string) error` function** in `pkg/spec/spec.go` (package-level):
   Validate that `branch` does not contain patterns unsafe for `git checkout`:
   - Contains `..` → error
   - Has a leading `-` (or any path component starting with `-`) → error
   - Contains space, tab, `\n` → error
   - Contains shell metacharacters: `` ` ``, `$`, `(`, `)`, `|`, `&`, `;`, `<`, `>`, `!` → error
   - Empty string → always valid (field is optional)
   Use `regexp.MustCompile` for the combined check. Do NOT shell-exec `git check-ref-format` — implement the check inline.
   ```go
   var unsafeBranchPattern = regexp.MustCompile(`\.\.|^-|[\s` + "`" + `$()|\&;<>!]`)

   func ValidateBranchName(ctx context.Context, branch string) error {
       if branch == "" {
           return nil
       }
       if unsafeBranchPattern.MatchString(branch) {
           return errors.Errorf(ctx, "branch name %q contains invalid characters", branch)
       }
       return nil
   }
   ```

5. **Update `specApproveCommand.Run()`** in `pkg/cmd/spec_approve.go`: after `sf.SetStatus(string(spec.StatusApproved))` and before `sf.Save(ctx)`, auto-generate and set the branch:
   ```go
   autoBranch := spec.AutoBranchName(sf.Name)
   sf.SetBranchIfEmpty(autoBranch)
   ```
   No flag override is needed (the spec says "user can override via a flag" but also notes this is optional — for simplicity, skip the CLI flag for now and just auto-generate).
   If the spec's `Frontmatter.Branch` was already set (re-approve case), `SetBranchIfEmpty` is a no-op.

6. **Add branch validation in `specApproveCommand.Run()`** in `pkg/cmd/spec_approve.go`: after the auto-generation, validate the resulting branch name:
   ```go
   if err := spec.ValidateBranchName(ctx, sf.Frontmatter.Branch); err != nil {
       return errors.Wrap(ctx, err, "invalid branch name")
   }
   ```

7. **Add tests for `AutoBranchName`** in `pkg/spec/spec_test.go`:
   - `"028-shared-branch-per-spec"` → `"dark-factory/spec-028"`
   - `"001-my-feature"` → `"dark-factory/spec-001"`
   - `"no-number"` → `"dark-factory/no-number"`

8. **Add tests for `ValidateBranchName`** in `pkg/spec/spec_test.go`:
   - Empty string → no error
   - `"dark-factory/spec-028"` → no error
   - `"feature/my-thing"` → no error
   - `"../../etc"` → error (contains `..`)
   - `"-bad-start"` → error (leading `-`)
   - `"has space"` → error (space)
   - `"shell$var"` → error (shell metachar)

9. **Add tests for `spec approve` auto-branch** in `pkg/cmd/spec_approve_test.go`:
   - Approving a spec with no `branch` frontmatter sets `branch: dark-factory/spec-NNN`
   - Approving a spec that already has `branch` set preserves the existing value (re-approve safety)
   - A spec with `branch: "../../evil"` in its frontmatter fails approve with a validation error
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- The `Branch` and `Issue` fields are optional — specs without them continue to work unchanged
- `Issue` field is stored as a literal string — never shell-interpolated
- Branch auto-generation must NOT overwrite an existing `branch` value in frontmatter
- Branch validation applies to the value in frontmatter after auto-generation, so even a manually pre-set bad branch is caught at approve time
- All existing tests must pass
- `make precommit` must pass
</constraints>

<verification>
```bash
# Spec frontmatter with branch/issue loads and saves correctly
grep -n "Branch\|Issue" pkg/spec/spec.go
# Expected: field definitions in Frontmatter struct

# Validate branch check exists
grep -n "ValidateBranchName\|unsafeBranch" pkg/spec/spec.go

make precommit
```
Must pass with no errors.
</verification>
