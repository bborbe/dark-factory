---
spec: ["023"]
status: created
created: "2026-03-06T20:01:00Z"
---
<summary>
- Updates YOLO CLAUDE.md to require conventional prefixes on every `## Unreleased` entry
- Updates changelog-guide.md with the prefix table and anti-patterns
- Documentation-only change — no Go code touched
- NOTE: these docs were already updated manually today — this prompt is likely redundant
- NOTE: changes to `/home/node/.claude/` won't be committed by dark-factory (outside workspace)
</summary>

<objective>
Ensure the YOLO container documentation explicitly requires conventional prefixes on every `## Unreleased` changelog entry, and that the changelog guide documents the prefix→version-bump mapping with examples. The Go code change (spec-023-1) is a precondition — this prompt updates only documentation files.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `/home/node/.claude/CLAUDE.md` — the global YOLO container instructions. Contains a `## Changelog` section that must explicitly state: every `## Unreleased` entry requires a conventional prefix, and which prefixes map to minor vs patch bump.
Read `/home/node/.claude/docs/changelog-guide.md` — the detailed changelog writing guide. Must contain: the prefix table (`feat:` → minor, all others → patch), format rule, and at least one anti-pattern showing a missing-prefix entry.
</context>

<requirements>
1. Read `/home/node/.claude/CLAUDE.md`. In the `## Changelog` section, verify the following are present:
   - A rule that every `## Unreleased` entry must start with a conventional prefix
   - The list of valid prefixes: `feat:`, `fix:`, `refactor:`, `test:`, `docs:`, `chore:`, `perf:`
   - A note that `feat:` triggers a minor bump and all others trigger a patch bump

   If any of these are missing or unclear, add/update them. Do not reformat or rewrite sections that are already correct — only add what is genuinely missing.

2. Read `/home/node/.claude/docs/changelog-guide.md`. Verify it contains:
   - A prefix table with all required columns: Prefix, Meaning, Version bump — listing at minimum `feat:` (minor) and `fix:`, `refactor:`, `test:`, `docs:`, `chore:`, `perf:` (patch)
   - The format rule: `- <prefix>: <what> [context]`
   - At least one anti-pattern showing an entry WITHOUT a prefix and the corrected version WITH a prefix

   If any of these are missing or unclear, add/update them. Do not reformat or rewrite sections that are already correct.

3. Do NOT modify any Go source files — this prompt is documentation only.
</requirements>

<constraints>
- Only modify documentation files at `/home/node/.claude/CLAUDE.md` and `/home/node/.claude/docs/changelog-guide.md`
- Do not rewrite or reformat sections that already satisfy the requirements — make targeted additions only
- Do NOT commit — dark-factory handles git
- `make precommit` must pass (no Go source changes, so this should be trivially satisfied)
</constraints>

<verification>
Run `make precommit` — must pass.

Manual checks:
```bash
# Confirm feat: prefix is documented in global CLAUDE.md
grep -n "feat:" /home/node/.claude/CLAUDE.md

# Confirm prefix table exists in changelog guide
grep -n "feat:" /home/node/.claude/docs/changelog-guide.md

# Confirm minor/patch bump distinction is documented
grep -n -i "minor\|patch" /home/node/.claude/docs/changelog-guide.md
```
</verification>
