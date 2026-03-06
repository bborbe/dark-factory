<objective>
Fix a bug in NormalizeFilenames: files with wrong-format numeric prefixes (e.g. `01-foo.md`) are renamed to `001-foo.md` even when that number is already used by a completed file, causing duplicates like `001-foo.md` alongside `001-core-pipeline.md` in completed.

The root cause is two problems:
1. `scanPromptFiles` adds improperly-formatted files to `usedNumbers` (claiming number=1 for `01-foo.md`), preventing the conflict check from working correctly.
2. Case 3 in `determineRename` (wrong format) never checks `usedNumbers` — it just reformats and keeps the same number, even if that number conflicts with a completed file.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read pkg/prompt/prompt.go — focus on:
- `scanPromptFiles` (around line 738): builds usedNumbers from directory entries
- `determineRename` (around line 847): Case 3 is the bug location
- `findNextAvailableNumber` (around line 842): already correct
Read pkg/prompt/prompt_test.go for existing NormalizeFilenames test patterns.
</context>

<requirements>
1. Fix `scanPromptFiles` in `pkg/prompt/prompt.go`:
   - Only add a file's number to `usedNumbers` if the file is **already properly formatted** (matches `validPatternRegexp`: exactly `NNN-slug.md` with 3-digit prefix).
   - Files with wrong-format prefixes (e.g. `01-foo.md`, `9-bar.md`) should NOT claim their number in `usedNumbers`. They have a number parsed (for slug extraction) but haven't earned it yet.

2. Fix `determineRename` Case 3 in `pkg/prompt/prompt.go`:
   - After establishing wrong format (`f.name != expectedName`), check `usedNumbers[f.number]`.
   - If the number is already used → find a new number with `findNextAvailableNumber`, add to `usedNumbers`, return new number.
   - If not used → keep the same number (just reformat), same as today.

3. Add tests to `pkg/prompt/prompt_test.go` (find the NormalizeFilenames test block):
   - `01-foo.md` in queue + `001-core-pipeline.md` in completed → renamed to next available (e.g. `002-foo.md` if only 001 is taken)
   - `01-foo.md` in queue + completed files `001` through `094` → renamed to `095-foo.md`
   - `01-foo.md` in queue with no completed files → renamed to `001-foo.md` (no conflict)
   - `095-foo.md` (already correct 3-digit format) in queue → not renamed
   - Existing tests must still pass
</requirements>

<constraints>
- Fix only these two functions — do not refactor unrelated code
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
