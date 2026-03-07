---
spec: ["019"]
---
<summary>
- Spec progress always shows 0/0 prompts even when prompts are correctly linked
- Specs never auto-transition to "verifying" after all linked prompts complete
- Root cause: spec IDs are compared as raw strings instead of parsed numbers
- After fix: spec "019-review-fix-loop" matches prompts with spec: ["019"] because both parse to 19
- Zero-padding differences don't matter: "019" = "19" = "0019"
- Non-numeric spec IDs still work via exact string fallback
</summary>

<objective>
Make spec-prompt linking work by comparing parsed integer prefixes instead of raw strings. A spec file `019-review-fix-loop.md` has spec ID `19`. A prompt with `spec: ["019"]` also resolves to `19`. They match.
</objective>

<context>
Read CLAUDE.md for project conventions.

The bug has two sides:
1. `pkg/cmd/spec_list.go:68` — `CountBySpec(ctx, sf.Name)` passes `"019-review-fix-loop"` but `HasSpec` in prompts checks against `"019"` → no match → `0/0`.
2. `pkg/spec/spec.go:204` — auto-completer receives `"019"` from prompt frontmatter, looks for file `019.md` → not found → spec never transitions to verifying.

Key existing code:
- `pkg/prompt/prompt.go:984` — `extractNumberFromFilename` already parses numeric prefix from filename to int.
- `pkg/prompt/prompt.go:146` — `HasSpec(id string)` does exact string match.
- `pkg/prompt/counter.go:71` — `HasSpec(specID)` called during counting.
- `pkg/spec/spec.go:181-232` — `CheckAndComplete` auto-completer logic.
- `pkg/spec/spec.go:204` — file lookup: `filepath.Join(dir, specID+".md")`.
- `pkg/spec/spec.go:287` — `HasSpec(specID)` in `scanDirForSpec`.
- `pkg/cmd/spec_list.go:68` — passes `sf.Name` to `CountBySpec`.
- `pkg/cmd/spec_status.go` — similar to spec_list, also calls `CountBySpec`.
</context>

<requirements>
1. Add a `SpecNumber() int` method to `SpecFile` in `pkg/spec/spec.go` (or a helper function) that extracts the numeric prefix from `sf.Name` and returns it as int. Return `-1` if no numeric prefix. Reuse the regex/parsing pattern from `extractNumberFromFilename` in `pkg/prompt/prompt.go` (move to a shared location or duplicate — your call, but keep it simple).

2. Change `HasSpec(id string)` in `pkg/prompt/prompt.go` to compare by parsed int. Parse both the stored spec value and the passed `id` to int. If either fails to parse, fall back to exact string match (so non-numeric specs still work).

3. In `pkg/cmd/spec_list.go:68` and `pkg/cmd/spec_status.go`, keep passing `sf.Name` to `CountBySpec` — the int-based `HasSpec` from requirement 2 handles the matching. No signature change needed.

4. Fix auto-completer file lookup in `pkg/spec/spec.go:200-212`:
   - Instead of `filepath.Join(dir, specID+".md")`, scan the directory for a `.md` file whose numeric prefix matches the parsed int of `specID`.
   - If no match found, fall back to exact `specID+".md"` lookup (backward compat).

5. Update all tests:
   - `pkg/prompt/prompt_test.go` — `HasSpec` tests should verify int-based matching (`"019"` matches `"19"`, `"0019"` matches `"019"`).
   - `pkg/prompt/counter_test.go` — test that counter finds prompts with `spec: ["019"]` when called with `"019-review-fix-loop"` or `"019"`.
   - `pkg/spec/spec_test.go` — update auto-completer tests to use realistic naming (spec file `019-foo.md`, prompts with `spec: ["019"]`). Keep existing tests passing.
</requirements>

<constraints>
- Do NOT change prompt frontmatter format — `spec: ["019"]` stays as-is
- Do NOT rename any spec files
- Do NOT change the `SpecList` YAML unmarshaling
- Non-numeric spec IDs (e.g. `spec: ["notifications"]`) must still work via exact string fallback
- Do NOT commit — dark-factory handles git
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
