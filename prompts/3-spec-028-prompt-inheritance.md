---
spec: ["028"]
status: created
created: "2026-03-10T19:39:55Z"
---
<summary>
- Prompt frontmatter gains an optional `issue` field (freeform issue tracker reference such as a Jira ID or GitHub URL)
- Existing prompts without `issue` load and save without error — the field is fully optional
- After the generator creates new prompt files from a spec, it post-processes each new file to copy the spec's `branch` and `issue` values into the prompt's frontmatter
- If a generated prompt already has `branch` or `issue` set in its frontmatter, the inherited value is NOT written — explicit values take priority
- The `issue` value is passed as a literal string wherever it is used — never shell-expanded or interpolated
</summary>

<objective>
Add an `issue` field to prompt frontmatter and make the spec generator post-process newly created prompt files to inherit `branch` and `issue` from the parent spec — enabling automatic linking across all prompts from a multi-prompt spec.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/prompt/prompt.go` — `Frontmatter` struct (~line 160): it already has `Branch string` (~line 171). We need to add `Issue string`. Also read `SetBranch()` (~line 404) and the `Manager` interface (~line 437) to understand how frontmatter mutations work.
Read `pkg/generator/generator.go` — `Generate()` method (~line 60): after execution, it counts new `.md` files in inboxDir. After this spec work, it must also post-process new files to inherit branch/issue from the spec.
Read `pkg/spec/spec.go` — `Frontmatter` struct (~line 42, modified by prompt 2 of this spec to add `Branch` and `Issue`): the generator reads the spec's frontmatter and copies those values to new prompts.
Read `pkg/generator/generator_test.go` — existing tests.
Read `pkg/prompt/prompt_test.go` — existing prompt tests.
</context>

<requirements>
1. **Add `Issue string` to `prompt.Frontmatter`** in `pkg/prompt/prompt.go`:
   ```go
   type Frontmatter struct {
       Status             string   `yaml:"status"`
       Specs              SpecList `yaml:"spec,omitempty,flow"`
       Summary            string   `yaml:"summary,omitempty"`
       Container          string   `yaml:"container,omitempty"`
       DarkFactoryVersion string   `yaml:"dark-factory-version,omitempty"`
       Created            string   `yaml:"created,omitempty"`
       Queued             string   `yaml:"queued,omitempty"`
       Started            string   `yaml:"started,omitempty"`
       Completed          string   `yaml:"completed,omitempty"`
       PRURL              string   `yaml:"pr-url,omitempty"`
       Branch             string   `yaml:"branch,omitempty"`
       Issue              string   `yaml:"issue,omitempty"`
       RetryCount         int      `yaml:"retryCount,omitempty"`
   }
   ```

2. **Add `SetIssue(issue string)` method to `PromptFile`** in `pkg/prompt/prompt.go`:
   ```go
   func (pf *PromptFile) SetIssue(issue string) {
       pf.Frontmatter.Issue = issue
   }
   ```

3. **Add `SetIssueIfEmpty(issue string)` method to `PromptFile`** — sets `Issue` only if currently empty (preserves explicit values):
   ```go
   func (pf *PromptFile) SetIssueIfEmpty(issue string) {
       if pf.Frontmatter.Issue == "" {
           pf.Frontmatter.Issue = issue
       }
   }
   ```
   Also add a corresponding `SetBranchIfEmpty(branch string)` method to `PromptFile` (parallel to `SetIssueIfEmpty`):
   ```go
   func (pf *PromptFile) SetBranchIfEmpty(branch string) {
       if pf.Frontmatter.Branch == "" {
           pf.Frontmatter.Branch = branch
       }
   }
   ```

4. **Add `inheritFromSpec` helper in `pkg/generator/generator.go`**: a function that, given a list of newly created prompt file paths and the loaded spec, copies `branch` and `issue` from the spec into each prompt's frontmatter (without overwriting existing values):
   ```go
   func inheritFromSpec(
       ctx context.Context,
       paths []string,
       specBranch string,
       specIssue string,
       currentDateTimeGetter libtime.CurrentDateTimeGetter,
   ) error {
       for _, p := range paths {
           pf, err := prompt.Load(ctx, p, currentDateTimeGetter)
           if err != nil {
               return errors.Wrap(ctx, err, "load prompt for inheritance")
           }
           pf.SetBranchIfEmpty(specBranch)
           pf.SetIssueIfEmpty(specIssue)
           if err := pf.Save(ctx); err != nil {
               return errors.Wrap(ctx, err, "save prompt after inheritance")
           }
       }
       return nil
   }
   ```
   This helper is only called when at least one of `specBranch` or `specIssue` is non-empty (no-op for specs without these fields).

5. **Snapshot inbox before generation** in `Generate()` in `pkg/generator/generator.go`: before calling `g.executor.Execute(...)`, snapshot the current `.md` files in `inboxDir` by name. After execution, find the new files by diffing the before and after sets:
   ```go
   // before snapshot
   beforeFiles, err := listMDFiles(g.inboxDir)
   // ... execute ...
   // after snapshot
   afterFiles, err := listMDFiles(g.inboxDir)
   newFiles := diffFiles(beforeFiles, afterFiles)
   ```
   Implement `listMDFiles(dir string) (map[string]struct{}, error)` (returns a set of filenames) and `diffFiles(before, after map[string]struct{}) []string` (returns full paths of files in `after` but not `before`).

6. **Call `inheritFromSpec` in `Generate()`** after the new files are identified and before loading/saving the spec status:
   ```go
   if len(newFiles) > 0 && (sf_branch != "" || sf_issue != "") {
       if err := inheritFromSpec(ctx, newFiles, sf_branch, sf_issue, g.currentDateTimeGetter); err != nil {
           return errors.Wrap(ctx, err, "inherit spec metadata to prompts")
       }
   }
   ```
   The spec must be loaded once to get its branch and issue before calling inheritFromSpec, then used again for `sf.SetStatus(prompted)` / `sf.Save()`. Load the spec file once and reuse it.

7. **Refactor `Generate()`** to replace the existing `countMDFiles` approach with the new `listMDFiles`/`diffFiles` approach. The `after <= before` check becomes `len(newFiles) == 0`. Remove the `countMDFiles` function if it is no longer used (check `countCompletedPromptsForSpec` — it uses `prompt.Load` separately, not `countMDFiles`).

8. **Add tests for `Issue` field in prompt** in `pkg/prompt/prompt_test.go`:
   - Load a prompt with `issue: BRO-19476` in frontmatter — value is preserved after load
   - Save a prompt with `issue` set — field appears in output YAML
   - `SetIssueIfEmpty` does not overwrite an existing value
   - `SetBranchIfEmpty` does not overwrite an existing value

9. **Add tests for generator inheritance** in `pkg/generator/generator_test.go`:
   - When spec has `branch: dark-factory/spec-028` and `issue: BRO-123`, new prompts inherit both
   - When a generated prompt already has `branch: my-override`, the inherited branch is NOT applied
   - When spec has no branch/issue, prompts are left unmodified (no extra Load/Save calls)
   - Use the existing test infrastructure (fake executor, temp directories)
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- The `Issue` field is optional — existing prompts without it continue to work unchanged
- `Issue` is a literal string — never shell-interpolated or command-substituted anywhere in the codebase
- Do NOT overwrite existing `branch` or `issue` values on generated prompts
- Inheritance is a no-op when both `specBranch` and `specIssue` are empty — no extra file I/O
- `listMDFiles` must return an empty map (not error) when the directory does not exist
- All existing tests must still pass
- `make precommit` must pass
</constraints>

<verification>
```bash
# Issue field in Frontmatter
grep -n "Issue" pkg/prompt/prompt.go
# Expected: field definition in Frontmatter struct and SetIssue/SetIssueIfEmpty methods

# inheritFromSpec called in Generate
grep -n "inheritFromSpec\|listMDFiles\|diffFiles" pkg/generator/generator.go
# Expected: all three referenced

make precommit
```
Must pass with no errors.
</verification>
