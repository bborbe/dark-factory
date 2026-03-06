---
spec: ["012"]
status: completed
container: dark-factory-048-replace-splitfrontmatter-with-adrg-frontmatter
dark-factory-version: v0.11.2
created: "2026-03-02T18:09:06Z"
started: "2026-03-02T18:09:06Z"
completed: "2026-03-02T18:15:38Z"
---
<objective>
Replace hand-rolled `splitFrontmatter` with `github.com/adrg/frontmatter` library.
Eliminates a class of parsing bugs (off-by-one fixed in previous commit) by using a battle-tested library.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/prompt/prompt.go` — all frontmatter handling lives here.
Read `pkg/prompt/prompt_test.go` — existing tests including `splitFrontmatter edge cases` and `setField does not accumulate blank lines`.
The project already uses `gopkg.in/yaml.v3` for marshal/unmarshal.
</context>

<requirements>
1. Add `github.com/adrg/frontmatter` dependency:
   ```
   go get github.com/adrg/frontmatter
   ```

2. Replace `splitFrontmatter(content []byte) ([]byte, []byte, bool)` (line ~564) with a new function that uses `frontmatter.Parse`:
   ```go
   func splitFrontmatter(content []byte) ([]byte, []byte, bool) {
       var fm map[string]interface{}
       body, err := frontmatter.Parse(bytes.NewReader(content), &fm)
       if err != nil || len(fm) == 0 {
           return nil, content, false
       }
       yamlBytes, err := yaml.Marshal(fm)
       if err != nil {
           return nil, content, false
       }
       return yamlBytes, body, true
   }
   ```
   Note: This preserves the existing function signature so all callers remain unchanged.

3. IMPORTANT: The callers of `splitFrontmatter` expect `yamlBytes` to be valid YAML that can be unmarshaled into `Frontmatter` struct. Verify that round-tripping through `map[string]interface{}` + `yaml.Marshal` preserves field names correctly (especially `dark-factory-version` with the hyphen).

4. If the round-trip approach in step 3 causes issues with field mapping, use an alternative: parse into `Frontmatter` struct directly and re-marshal:
   ```go
   func splitFrontmatter(content []byte) ([]byte, []byte, bool) {
       var fm Frontmatter
       body, err := frontmatter.Parse(bytes.NewReader(content), &fm)
       if err != nil {
           return nil, content, false
       }
       if fm.Status == "" && fm.Container == "" && fm.Created == "" {
           return nil, content, false
       }
       yamlBytes, err := yaml.Marshal(&fm)
       if err != nil {
           return nil, content, false
       }
       return yamlBytes, body, true
   }
   ```

5. Delete `stripLeadingEmptyFrontmatter` function (line ~824) — `adrg/frontmatter` handles edge cases cleanly. Update `Content()` (line ~483) to remove the `stripLeadingEmptyFrontmatter` call. Only remove if all existing tests still pass without it.

6. Run `go mod tidy && go mod vendor` after adding the dependency.
</requirements>

<constraints>
- Keep `splitFrontmatter` function signature identical — all callers must work unchanged
- Do NOT modify `addFrontmatterWithSetter` or `updateExistingFrontmatterWithSetter` — write path is fine
- Do NOT modify `Frontmatter` struct
- Do NOT break existing tests — all 93 tests in `pkg/prompt` must pass
- Do NOT remove the `setField does not accumulate blank lines` test — it's a regression guard
- Package name stays `prompt`, external test package `prompt_test`
</constraints>

<verification>
Run: `make test`
Run: `go test -v ./pkg/prompt/...`
Confirm: all 93+ tests pass
Confirm: no blank line accumulation (the "body size stays constant across 20 setField cycles" test passes)
Run: `make precommit`
</verification>

<success_criteria>
- `splitFrontmatter` uses `adrg/frontmatter` internally
- `stripLeadingEmptyFrontmatter` removed (if safe) or simplified
- All existing tests pass unchanged
- `go mod tidy` clean
- `make precommit` passes
</success_criteria>
