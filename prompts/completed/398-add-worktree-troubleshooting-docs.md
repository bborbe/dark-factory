---
status: completed
spec: [084-fail-fast-on-worktree-without-hidegit]
summary: Added worktree/submodule + hideGit failure mode documentation to troubleshooting.md
container: dark-factory-exec-398-add-worktree-troubleshooting-docs
dark-factory-version: v0.164.0
created: "2026-05-21T21:45:02Z"
queued: "2026-05-21T21:33:34Z"
started: "2026-05-21T21:33:35Z"
completed: "2026-05-21T21:34:18Z"
branch: dark-factory/fail-fast-on-worktree-without-hidegit
---

<summary>
- `docs/troubleshooting.md` gains a new section explaining the worktree-from-host + container `.git`-mount mismatch
- The section explicitly names the failure signature (`fatal: not a git repository: <parent>/.git/worktrees/<name>`), explains why it happens, and names `hideGit=true` as the remediation
- Section includes a "PR via Pre-Created Worktree" runbook reference
</summary>

<objective>
Update `docs/troubleshooting.md` to document the worktree/submodule + `hideGit` failure mode, remediation, and runbook reference.
</objective>

<context>
Read `docs/troubleshooting.md` in full to understand existing structure and style.
</context>

<requirements>
1. In `docs/troubleshooting.md`, append a new top-level `## ` section titled `## Running dark-factory from a git worktree or submodule` at the end of the file (do NOT insert inside or between the existing section's subsections).

2. The new section must include:
   - An explanation that when dark-factory is started from a git worktree (where `.git` is a regular file pointing to `<parent>/.git/worktrees/<name>`) or from a git submodule (where `.git` contains `gitdir: ../.git/modules/<name>`), the container mount cannot follow the pointer back to the parent repo's git metadata
   - The exact failure signature: `fatal: not a git repository: <parent>/.git/worktrees/<name>` (paste this verbatim)
   - The remediation: set `hideGit: true` in `.dark-factory.yaml` or pass `--set hideGit=true` on the command line
   - A note that `hideGit=true` masks the `.git` pointer so the container workspace is treated as a non-git directory, which works correctly for both prompt execution and spec generation
   - A reference to the "PR via Pre-Created Worktree" runbook (you may link to it as a plain-text reference, e.g., `See the 'PR via Pre-Created Worktree' runbook for the canonical workflow`)
   - A brief mention that auto-enabling `hideGit` when a worktree is detected was considered but rejected in favor of explicit configuration

3. Do NOT remove, rename, or restructure any existing sections.

4. Write the section in the same style as the existing content (plain English, actionable, no markdown-heavy formatting).
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing content in `docs/troubleshooting.md` must be preserved
- The new section itself (not the file as a whole) must contain the string `hideGit` at least once and the string `worktree` at least 3 times
- The new section must contain the exact failure signature string `fatal: not a git repository`
</constraints>

<verification>
```bash
# Verify hideGit appears
grep -ni 'hideGit' docs/troubleshooting.md | wc -l
# Verify worktree appears at least 3 times
grep -ni 'worktree' docs/troubleshooting.md | wc -l
# Verify the failure signature appears
grep -n 'fatal: not a git repository' docs/troubleshooting.md
# Verify a new top-level worktree section heading exists
grep -n '^## .*[Ww]orktree' docs/troubleshooting.md
```
</verification>
