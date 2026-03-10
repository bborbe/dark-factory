---
status: draft
---

## Summary

- Config replaces `workflow: direct|pr` enum with two booleans: `pr` and `worktree`
- Specs and prompts gain `branch` and `issue` metadata fields
- Spec approval auto-assigns a branch name inherited by all generated prompts
- Old `workflow:` config continues to work with a deprecation warning

## Problem

The dark-factory config uses a `workflow: direct|pr` enum that is effectively a boolean — there are only two values and the old `worktree` value was already removed. Meanwhile, specs that produce multiple prompts have no way to assign a shared branch or link to an external issue tracker. Users must manually set the `branch` frontmatter on each prompt, which is tedious and error-prone.

## Goal

After this work, the config uses `pr` and `worktree` booleans instead of a workflow enum. Specs carry `branch` and `issue` metadata that is automatically inherited by generated prompts. Existing configs continue to work during migration.

## Non-goals

- Changing execution behavior based on these new fields (handled by a follow-up spec)
- Parallel execution of spec prompts
- Removing `workflow:` support (deferred to a future release)

## Desired Behavior

1. **Config uses `pr` and `worktree` booleans**: The config accepts `pr` (bool, default false) and `worktree` (bool, default false) instead of `workflow`. When `pr` is true, prompts go through pull request review. When `worktree` is true, execution happens in an isolated clone rather than in-place.

2. **Old config is accepted with deprecation warning**: Configs using `workflow: direct` map to `pr: false, worktree: false`. Configs using `workflow: pr` map to `pr: true, worktree: true` (preserving the current PR workflow's clone-based isolation). A deprecation warning is logged at startup. Both old and new fields must not be set simultaneously (error if both present).

3. **Specs carry branch and issue metadata**: A spec can have an optional `branch` (git branch name) and `issue` (freeform string for any issue tracker — Jira ID, GitHub URL, etc.) in its frontmatter.

4. **Spec approval assigns a branch**: When a spec is approved, a branch name is auto-generated from the spec number (e.g., spec `028-my-feature.md` gets branch `dark-factory/spec-028`). The user can override via a flag. The branch is stored in the spec's frontmatter.

5. **Prompts carry issue metadata**: A prompt can have an optional `issue` field in its frontmatter. When set, dark-factory includes the issue reference in PR descriptions and execution logs, enabling automatic linking in Jira and GitHub.

6. **Generated prompts inherit from their spec**: When the generator creates prompts from a spec, it copies the spec's `branch` and `issue` values into each new prompt's frontmatter. Existing values on the prompt are not overwritten.

## Constraints

- Configs with no `pr`, `worktree`, or `workflow` field default to `pr: false, worktree: false` (current direct behavior)
- The `branch` and `issue` fields are optional — omitting them changes nothing
- Existing prompts without `issue` continue to work unchanged
- Existing specs without `branch` or `issue` continue to work unchanged
- All existing tests must pass
- `make precommit` must pass

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Both `workflow:` and `pr:` set in config | Validation error at startup with clear message | User removes one |
| Unknown `workflow:` value (e.g., `worktree`) | Existing error preserved — migration maps known values only | User updates config |
| Spec approved without a number prefix | Branch auto-generation falls back to spec filename | Works, just longer branch name |
| Generator finds prompt already has `branch` set | Does not overwrite — explicit values take priority | Works as expected |
| `spec approve` on spec that already has `branch` | Existing branch is preserved, not overwritten | User can re-run safely |

## Acceptance Criteria

- [ ] Config accepts `pr` and `worktree` booleans
- [ ] Old `workflow: direct` and `workflow: pr` map correctly with deprecation warning
- [ ] Error when both `workflow:` and `pr:` are present
- [ ] Spec frontmatter supports `branch` and `issue`
- [ ] Prompt with `issue: BRO-19476` loads without error and value is preserved through save
- [ ] `spec approve` auto-generates and stores branch name
- [ ] Generated prompts inherit `branch` and `issue` from spec
- [ ] Inheritance does not overwrite existing prompt values
- [ ] Omitting all new fields preserves current behavior exactly
- [ ] All existing tests pass
- [ ] `make precommit` passes

## Security

- The `branch` field is user-supplied text passed to `git checkout` and `git push`. Branch names must be validated against git's allowed ref format — reject names containing `..`, leading `-`, spaces, or shell metacharacters. Use `git check-ref-format` or equivalent validation.
- The `issue` field is freeform text included in PR descriptions and logs. It must be treated as untrusted — no shell interpolation, no command substitution. Pass as a literal string argument, never through shell expansion.
- Auto-generated branch names from spec numbers are safe (numeric prefix + sanitized filename).

## Verification

```bash
make precommit
```

Manual verification steps:

1. **Old config migration**: Create `.dark-factory.yaml` with `workflow: direct`. Run `dark-factory status`. Expected: startup logs deprecation warning, behaves as `pr: false, worktree: false`. Repeat with `workflow: pr` — maps to `pr: true, worktree: true`.
2. **Conflict detection**: Set both `workflow: direct` and `pr: true`. Run `dark-factory status`. Expected: validation error at startup.
3. **Spec approve**: Run `dark-factory spec approve 028-my-feature`. Expected: spec frontmatter contains `branch: dark-factory/spec-028`.
4. **Re-approve safety**: Run `dark-factory spec approve 028-my-feature` again. Expected: existing branch preserved, not overwritten.
5. **Generator inheritance**: Generate prompts from a spec with `branch` and `issue` set. Expected: each new prompt has both fields in frontmatter.
6. **No overwrite**: Generate prompts where one already has `branch` set. Expected: existing value preserved.
7. **Branch validation**: Set `branch: "../../etc"` on a spec. Expected: validation error.

## Do-Nothing Option

Keep the workflow enum — it works but adds unnecessary complexity for a two-value choice. Keep manual `branch` setting on prompts — works but tedious for multi-prompt specs and easy to forget. No issue linking — users track this mentally or in external docs.
