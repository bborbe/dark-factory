---
status: completed
summary: Updated pkg/reindex/reindex.go to use gopkg.in/yaml.v3 via frontmatter.NewFormat for consistent YAML unmarshaling, matching the pattern in pkg/prompt/prompt.go and pkg/spec/spec.go, and added a test verifying YAML v3 multi-line and boolean frontmatter parses correctly.
container: dark-factory-275-review-dark-factory-fix-reindex-frontmatter
dark-factory-version: v0.104.2-dirty
created: "2026-04-06T00:00:00Z"
queued: "2026-04-06T17:05:26Z"
started: "2026-04-06T18:36:22Z"
completed: "2026-04-06T18:45:57Z"
---

<summary>
- The reindex package parses frontmatter using the default yaml parser instead of gopkg.in/yaml.v3
- Every other frontmatter parsing site in the project uses a custom yamlV3Format with gopkg.in/yaml.v3
- Using different yaml parsers for the same file format can cause subtle unmarshaling differences
- The fix passes the same yamlV3Format to frontmatter.Parse in reindex.go for consistency
</summary>

<objective>
Update `pkg/reindex/reindex.go` to pass the same `yamlV3Format` (using `gopkg.in/yaml.v3`) to `frontmatter.Parse` that is used in `pkg/prompt/prompt.go` and `pkg/spec/spec.go`, ensuring consistent YAML unmarshaling across all frontmatter parsing sites.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes:
- `pkg/reindex/reindex.go` — `frontmatter.Parse` call at ~line 231; check what struct is passed and how the format is currently omitted
- `pkg/prompt/prompt.go` — how `yamlV3Format` is defined and passed to `frontmatter.Parse` (~line 256)
- `pkg/spec/spec.go` — same pattern (~line 105)
</context>

<requirements>
1. In `pkg/prompt/prompt.go` (or wherever `yamlV3Format` is defined), check whether it is a package-level variable or constructed inline. If it is already exported or accessible, use it directly.

2. In `pkg/reindex/reindex.go`:
   a. Define a local `yamlV3Format` variable (or import and reuse from prompt package if exported) using the same pattern as `pkg/prompt/prompt.go`:
      ```go
      var yamlV3Format = frontmatter.NewFormat("---", "---", yaml.Unmarshal)
      ```
      where `yaml` is imported as `"gopkg.in/yaml.v3"`.
   b. Change the `frontmatter.Parse(reader, &fm)` call at ~line 231 to:
      ```go
      frontmatter.Parse(reader, &fm, yamlV3Format)
      ```

3. Add `"gopkg.in/yaml.v3"` to the import block if not already present.

4. Add or update a test in `pkg/reindex/` that verifies frontmatter with YAML v3-specific features (e.g., multi-line strings, boolean values) parses correctly after the change.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Use `gopkg.in/yaml.v3` for the custom format — not `gopkg.in/yaml.v2` or the default frontmatter parser
- All paths are repo-relative
</constraints>

<verification>
make precommit
</verification>
