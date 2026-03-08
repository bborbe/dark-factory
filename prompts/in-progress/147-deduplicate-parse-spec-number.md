---
status: approved
created: "2026-03-08T21:12:08Z"
queued: "2026-03-08T23:18:05Z"
---

<summary>
- Extract shared `ParseSpecNumber` function to new `pkg/specnum/` package
- Both `pkg/spec` and `pkg/prompt` import the shared package instead of having duplicates
- Eliminates code duplication without creating circular imports
</summary>

<objective>
Eliminate the duplicated `parseSpecNumber` function (identical in `pkg/spec/spec.go` and `pkg/prompt/prompt.go`) by extracting it to a shared package, avoiding the circular import between `spec` and `prompt`.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/spec/spec.go` — `parseSpecNumber` function (lines ~31-41) and `specNumericPrefixRegexp` (line ~25). Note: `spec.go` imports `pkg/prompt` (line 22), so `pkg/prompt` cannot import `pkg/spec`.
Read `pkg/prompt/prompt.go` — duplicate `parseSpecNumber` function (lines ~1063-1073) and duplicate `specNumericPrefixRegexp` (line ~37).
Read callers: `spec.go` L89 (`SpecFile.SpecNumber()`), `prompt.go` L177 and L180.
</context>

<requirements>
1. Create `pkg/specnum/specnum.go`:
   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   // Package specnum provides spec number parsing utilities.
   package specnum

   import (
       "regexp"
       "strconv"
   )

   var numericPrefixRegexp = regexp.MustCompile(`^(\d+)`)

   // Parse extracts the leading numeric value from a spec ID string.
   // Handles bare numbers ("019" → 19), padded numbers ("0019" → 19),
   // and full spec names ("019-review-fix-loop" → 19).
   // Returns -1 if s has no numeric prefix.
   func Parse(s string) int {
       matches := numericPrefixRegexp.FindStringSubmatch(s)
       if matches == nil {
           return -1
       }
       num, err := strconv.Atoi(matches[1])
       if err != nil {
           return -1
       }
       return num
   }
   ```

2. In `pkg/spec/spec.go`:
   - Remove `parseSpecNumber` function (lines ~31-41)
   - Remove `specNumericPrefixRegexp` variable (line ~25)
   - Add import `"github.com/bborbe/dark-factory/pkg/specnum"`
   - Replace `parseSpecNumber(...)` with `specnum.Parse(...)` (line ~89)

3. In `pkg/prompt/prompt.go`:
   - Remove `parseSpecNumber` function (lines ~1063-1073)
   - Remove `specNumericPrefixRegexp` variable (line ~37)
   - Add import `"github.com/bborbe/dark-factory/pkg/specnum"`
   - Replace `parseSpecNumber(...)` with `specnum.Parse(...)` (lines ~177, ~180)

4. Verify no other files have a copy of this function.
</requirements>

<constraints>
- Do NOT change the function logic — only move and rename
- No circular imports — `pkg/specnum` must not import `pkg/spec` or `pkg/prompt`
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- `make precommit` must pass
</constraints>

<verification>
Run `make precommit` — must pass.

Verify no duplicate:
```bash
grep -rn "parseSpecNumber\|specNumericPrefixRegexp" pkg/
# Expected: no output (all removed, replaced by specnum.Parse)
```

Verify new package exists:
```bash
ls pkg/specnum/specnum.go
# Expected: file exists
```

Verify no circular imports:
```bash
grep -n "dark-factory/pkg/spec\|dark-factory/pkg/prompt" pkg/specnum/specnum.go
# Expected: no output
```
</verification>

<success_criteria>
- Single `Parse` function in `pkg/specnum/specnum.go`
- Both `pkg/spec` and `pkg/prompt` use `specnum.Parse`
- No duplicate regex variable
- No circular imports
- `make precommit` passes
</success_criteria>
