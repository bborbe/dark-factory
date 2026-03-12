---
status: completed
summary: Updated spec.Load to use yaml.v3 format in frontmatter.Parse, matching the behavior of prompt.Load.
container: dark-factory-191-fix-spec-frontmatter-yaml-v3
dark-factory-version: v0.48.0
created: "2026-03-11T16:45:29Z"
queued: "2026-03-11T18:25:03Z"
started: "2026-03-12T01:21:29Z"
completed: "2026-03-12T01:26:12Z"
---

<summary>
- Spec file frontmatter parsing uses yaml.v3 consistently, matching the prompt file parser
- Both spec and prompt parsers use the same YAML semantics (correct null handling, tag support)
- No silent behavioral differences between spec and prompt frontmatter parsing
- The `frontmatter.Parse` call in `spec.go` is updated to use an explicit `yamlV3Format`
- The existing `gopkg.in/yaml.v3` import (already at ~line 21) is reused — no new imports needed
</summary>

<objective>
Fix the frontmatter parsing inconsistency in `pkg/spec/spec.go` where `frontmatter.Parse` is called without an explicit yaml.v3 format, defaulting to the library's internal yaml.v2. The prompt parser in `pkg/prompt/prompt.go` correctly pins yaml.v3. Both should use the same format.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/spec/spec.go` — find the `Load` function (~line 87). At ~line 98 it calls `frontmatter.Parse(bytes.NewReader(content), &fm)` without a format argument.
Read `pkg/prompt/prompt.go` — find the `Load` function (~line 220). At ~line 235-236 it correctly uses:
```go
yamlV3Format := frontmatter.NewFormat("---", "---", yaml.Unmarshal)
body, err := frontmatter.Parse(bytes.NewReader(content), &fm, yamlV3Format)
```
The `adrg/frontmatter` library: `frontmatter.NewFormat(start, end, unmarshalFunc)` creates a custom format. `frontmatter.Parse(reader, target, ...formats)` uses provided formats or defaults to yaml.v2.
</context>

<requirements>
1. In `pkg/spec/spec.go`, in the `Load` function, add the yaml.v3 format to the `frontmatter.Parse` call:
   ```go
   // Old:
   body, err := frontmatter.Parse(bytes.NewReader(content), &fm)

   // New:
   yamlV3Format := frontmatter.NewFormat("---", "---", yaml.Unmarshal)
   body, err := frontmatter.Parse(bytes.NewReader(content), &fm, yamlV3Format)
   ```

2. The import `"gopkg.in/yaml.v3"` is already present in `spec.go` (at ~line 21). Verify it exists and use the existing `yaml.Unmarshal` — no import changes needed.

3. Verify existing spec tests still pass — the behavioral change is subtle (yaml.v2 vs v3 semantics) but should be backward-compatible for the frontmatter fields used by specs.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
- Use `gopkg.in/yaml.v3` — the same version used in `pkg/prompt/prompt.go`.
- Use `github.com/adrg/frontmatter` — already imported in `spec.go`.
</constraints>

<verification>
Run `make precommit` -- must pass.
</verification>
