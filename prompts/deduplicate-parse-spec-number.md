---
status: created
created: "2026-03-08T21:12:08Z"
---

<objective>
Deduplicate `parseSpecNumber` which exists identically in both `pkg/prompt/prompt.go` and `pkg/spec/spec.go`. Move the canonical version to `pkg/spec/` and have `pkg/prompt/` import it.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/spec/spec.go` — `parseSpecNumber` function (lines ~27-41) and `specNumericPrefixRegexp`.
Read `pkg/prompt/prompt.go` — duplicate `parseSpecNumber` function (lines ~1063-1073) and its own `specNumericPrefixRegexp`.
Read callers of both functions to understand usage.
</context>

<requirements>
1. In `pkg/spec/spec.go`:
   - Export the function: rename `parseSpecNumber` → `ParseSpecNumber`
   - Export the regex if needed, or keep it private (the function is the public API)
   - Add GoDoc: `// ParseSpecNumber extracts the numeric prefix from a spec filename. Returns -1 if no prefix found.`

2. In `pkg/prompt/prompt.go`:
   - Remove the duplicate `parseSpecNumber` function
   - Remove the duplicate `specNumericPrefixRegexp` variable
   - Replace all calls to `parseSpecNumber(...)` with `spec.ParseSpecNumber(...)`
   - Add import for `"github.com/bborbe/dark-factory/pkg/spec"` if not present

3. In `pkg/spec/spec.go`, update the internal caller:
   - `SpecFile.SpecNumber()` calls `parseSpecNumber(s.Name)` → change to `ParseSpecNumber(s.Name)`

4. Verify no other files have a copy of this function.
</requirements>

<constraints>
- Do NOT change the function logic — only move and export
- Do NOT introduce circular imports (prompt → spec is fine, spec must NOT import prompt)
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- `make precommit` must pass
</constraints>

<verification>
Run `make precommit` — must pass.

Verify no duplicate:
```bash
grep -rn "parseSpecNumber\|ParseSpecNumber" pkg/
# Expected: only in pkg/spec/spec.go (definition) and pkg/prompt/prompt.go (caller)
```

Verify no duplicate regex:
```bash
grep -rn "specNumericPrefixRegexp" pkg/
# Expected: only in pkg/spec/spec.go
```
</verification>

<success_criteria>
- Single `ParseSpecNumber` in `pkg/spec/spec.go` (exported)
- `pkg/prompt/prompt.go` imports and uses `spec.ParseSpecNumber`
- No duplicate regex variable
- `make precommit` passes
</success_criteria>
