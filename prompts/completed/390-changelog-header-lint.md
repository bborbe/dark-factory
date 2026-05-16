---
status: completed
summary: Added scripts/check-changelog.sh with 4 preamble lint rules, wired check-changelog into precommit, and added processUnreleasedSection fixture test with full real-world SemVer preamble; all rules verified including negative test (stranded preamble → rule 3 fires, exit=1).
container: dark-factory-390-changelog-header-lint
dark-factory-version: v0.156.1-1-g04f3863-dirty
created: "2026-05-16T14:30:00Z"
queued: "2026-05-16T13:03:39Z"
started: "2026-05-16T13:03:40Z"
completed: "2026-05-16T13:07:37Z"
---

<summary>
- Add a `make check-changelog` Makefile target that lints `CHANGELOG.md` for header-preservation violations and wires into `precommit`
- The lint catches: (a) `## Unreleased` or `## vX.Y.Z` appearing before the SemVer preamble ("Please choose versions by [Semantic Versioning]"), (b) the SemVer preamble appearing after the first `## ` heading (stranded), (c) missing `# Changelog` title
- Add a `processUnreleasedSection` unit test fixture that includes the full real-world preamble (SemVer link + MAJOR/MINOR/PATCH bullets) and asserts the rename preserves the header byte-for-byte
- This is a one-shot regression guard for the v0.160.1 bug: a past `## Unreleased` was inserted above the SemVer preamble; later `## vX.Y.Z` rename stranded the preamble between `## v0.51.8` and `## v0.51.7`
</summary>

<objective>
Add a structural lint for `CHANGELOG.md` so the failure mode that produced v0.160.1 (stranded SemVer preamble) cannot recur on this repository, and lock the `processUnreleasedSection` rename helper against header drift via a fixture that mirrors the real-world preamble.
</objective>

<context>
Read `CLAUDE.md` for project conventions (errors, Ginkgo/Gomega, no fmt.Errorf, no bare `return err`).
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `changelog-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` for the canonical header rule (everything before the first `##` is frozen).

Files to read in full before editing:
- `Makefile` — find the `precommit:` target and the `check-versions:` target (if present, mirror its shape and placement)
- `scripts/` — check `ls scripts/` to see if a scripts directory exists; if yes, the new lint script lives there; if no, embed the check inline in the Makefile target
- `CHANGELOG.md` — the first ~10 lines are the canonical preamble shape the lint must accept; later versions are the body shape it must NOT confuse with the preamble
- `pkg/git/git_internal_test.go` — lines ~103–210 for the existing `processUnreleasedSection` test suite; the new fixture is added inside the same `Describe` block, following the existing pattern
- `pkg/git/git.go` — lines ~430–446 for the `processUnreleasedSection` implementation under test (read-only; no edit here)
</context>

<requirements>

## 1. Add a `check-changelog` Makefile target + script

Create `scripts/check-changelog.sh` (the `scripts/` directory already exists and contains `check-versions.sh`). Match the shell style of `scripts/check-versions.sh` exactly: `#!/usr/bin/env bash` shebang and `set -euo pipefail`. Read `scripts/check-versions.sh` in full first; mirror its layout (variable assignments at top, sequential rule checks, clear error messages, `exit 1` on failure, silent success).

Add a `check-changelog:` target to `Makefile` that invokes the script:

```
check-changelog:
	scripts/check-changelog.sh
```

Place the target alphabetically near other `check-*` targets in the Makefile.

The four rules (the lint MUST exit non-zero with a clear message if any fails):

1. **File starts with `# Changelog`** — the very first non-blank line of `CHANGELOG.md` must be exactly `# Changelog`. Detection: `head -1 CHANGELOG.md` equals `# Changelog`.

2. **SemVer preamble present and before first version** — the file must contain the line `Please choose versions by [Semantic Versioning](http://semver.org/).` somewhere BEFORE the first line matching `^## ` (Unreleased or vX.Y.Z). Detection: line number of the SemVer-link line is less than line number of the first `^## ` line.

3. **No stranded preamble** — the line `Please choose versions by [Semantic Versioning]` must appear EXACTLY ONCE in the file. A second occurrence means it was stranded by a past rename. Detection: `grep -c "Please choose versions by \[Semantic Versioning\]" CHANGELOG.md` equals 1.

4. **No MAJOR/MINOR/PATCH bullet stranded** — the line starting with `* MAJOR version when you make incompatible API changes,` must appear EXACTLY ONCE. Same detection pattern. (This catches the failure-mode case where the SemVer link was edited but the bullet block was stranded.)

Error messages must name the specific rule that failed and (if possible) the line number of the offending content. Suggested format:

```
CHANGELOG.md lint failed: rule 3 — SemVer preamble appears N times, expected 1. Stranded copy at line LINENO.
```

## 2. Wire `check-changelog` into `precommit`

The actual current line (verified): `precommit: ensure format generate test check addlicense`.

`check-versions` is NOT in `precommit` — it lives in `release-check: precommit check-versions` (drift during development is allowed; version alignment is enforced at release time). Follow the same shape for `check-changelog`: wire it into `precommit` (the header structure is invariant; drift here is a bug, not allowed-at-dev-time).

Update the `precommit:` line by appending `check-changelog`:

```
# OLD:
precommit: ensure format generate test check addlicense
# NEW:
precommit: ensure format generate test check addlicense check-changelog
```

Do NOT reorder or remove the existing dependencies.

## 3. Add a real-world-preamble fixture test for `processUnreleasedSection`

In `pkg/git/git_internal_test.go`, locate the `Describe("processUnreleasedSection", ...)` block (anchor by function name, not line number). Append a new `It("...")` case AFTER the existing "handles empty Unreleased section" `It` block (the last one in the suite), using the full real-world preamble shape:

```go
It("preserves the full SemVer preamble when renaming Unreleased", func() {
    lines := []string{
        "# Changelog",
        "",
        "All notable changes to this project will be documented in this file.",
        "",
        "Please choose versions by [Semantic Versioning](http://semver.org/).",
        "",
        "* MAJOR version when you make incompatible API changes,",
        "* MINOR version when you add functionality in a backwards-compatible manner, and",
        "* PATCH version when you make backwards-compatible bug fixes.",
        "",
        "## Unreleased",
        "- feat: New thing",
        "",
        "## v1.0.0",
        "- Old change",
    }

    result, found := processUnreleasedSection(lines, "v1.1.0")
    Expect(found).To(BeTrue())
    Expect(len(result)).To(Equal(len(lines)))
    // Header (lines 0..9) must be byte-identical to input
    for i := 0; i < 10; i++ {
        Expect(result[i]).To(Equal(lines[i]), "header line %d drifted", i)
    }
    // The rename happened at the right place
    Expect(result[10]).To(Equal("## v1.1.0"))
    // The body that followed is untouched
    Expect(result[13]).To(Equal("## v1.0.0"))
})
```

Follow the exact style of the existing `It("...")` blocks in this `Describe` — no new test infrastructure, no helpers.

## 4. Run verification

After the changes:

```bash
make check-changelog   # must exit 0 on current CHANGELOG.md
make precommit         # must exit 0
go test ./pkg/git/...  # the new test case must pass alongside existing 6 cases
```

To prove rule 3 actually rejects the failure mode, manually verify (do NOT commit this change):

```bash
# Temporarily inject a stranded preamble to test rule 3 fires
cp CHANGELOG.md /tmp/changelog-backup.md
awk '/^## v0.51.8/{print; print ""; print "Please choose versions by [Semantic Versioning](http://semver.org/)."; next}1' CHANGELOG.md > CHANGELOG.md.broken
mv CHANGELOG.md.broken CHANGELOG.md
make check-changelog ; echo "exit=$?"   # must print exit=1 (or similar non-zero) with rule 3 message
cp /tmp/changelog-backup.md CHANGELOG.md   # restore
rm /tmp/changelog-backup.md
```

Report the captured `make check-changelog` output from the broken-fixture run in the completion report so the reviewer can confirm the rule fires.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- The lint script MUST match the bash style of `scripts/check-versions.sh` (`#!/usr/bin/env bash`, `set -euo pipefail`). Do not introduce a different shell convention.
- The lint MUST exit `1` on any rule failure with a clear message naming the failed rule; clean exit (0) only when ALL four rules pass.
- Do NOT change `pkg/git/git.go` — the production code is correct; this prompt only adds tests + Makefile lint.
- Do NOT change the existing `processUnreleasedSection` test cases — append the new case only.
- The new `It("...")` test MUST be added inside the existing `Describe("processUnreleasedSection", ...)` block, not as a sibling.
- If `scripts/` does not exist, do NOT create it for just this one script — embed the rules in the Makefile target. Only use `scripts/` if it already exists.
- No new external dependencies (Go or shell).
- The lint rules match the literal preamble text of THIS repository's CHANGELOG.md. If a future project has a different preamble, the lint would need adjustment — that's acceptable; this is a dark-factory-local guard, not a portable tool. Do not generalize.
- Wrap all Go errors with `errors.Wrapf` / `errors.Errorf` from `github.com/bborbe/errors`. (Test code uses Gomega, not error returns — N/A here.)
- Do not touch `go.mod` / `go.sum` / `vendor/` / `CHANGELOG.md` itself.
</constraints>

<verification>
1. `make check-changelog` — must exit 0 on current `CHANGELOG.md`.
2. `make precommit` — must exit 0 and must include `check-changelog` in the run.
3. `go test ./pkg/git/... -run 'processUnreleasedSection'` — all 7 cases (6 existing + 1 new) pass.
4. `grep -n "check-changelog" Makefile` — at least 2 matches (target definition + precommit dependency).
5. Negative test (in completion report only, do NOT leave the broken file behind): inject a stranded preamble copy, run `make check-changelog`, confirm it exits non-zero and the error message names rule 3 (or rule 4 if the bullet was stranded). Restore the file.
</verification>
