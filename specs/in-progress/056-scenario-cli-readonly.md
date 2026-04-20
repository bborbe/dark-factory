---
status: prompted
tags:
    - dark-factory
    - spec
approved: "2026-04-20T15:52:38Z"
generating: "2026-04-20T15:53:05Z"
prompted: "2026-04-20T15:59:40Z"
branch: dark-factory/scenario-cli-readonly
---

## Summary

- Add a `scenario` command group to the `dark-factory` CLI with three read-only subcommands: `list`, `show`, and `status`.
- Scenarios already exist as markdown files under `scenarios/` but have no CLI visibility today — users must read the directory by hand.
- Behavior parallels the existing `prompt` and `spec` command groups so muscle memory transfers.
- Write operations (`new`, lifecycle transitions, execution) are intentionally deferred.
- Must degrade gracefully when `scenarios/` is missing, filenames don't match the expected pattern, or frontmatter is malformed.

## Problem

Scenarios are first-class artifacts in the dark-factory workflow (Task → Scenario → Spec → Prompts) and the project currently tracks ten of them under `scenarios/`. Every other artifact type (`prompt`, `spec`) has CLI subcommands for inspection — `list`, `show`, `status` — but scenarios have none. Users wanting to see which scenarios exist, check their status, or read one in context have to navigate the filesystem and parse frontmatter mentally. This is friction every time someone audits regression coverage or onboards to the project.

## Goal

After this work, a user can run `dark-factory scenario list`, `dark-factory scenario show <id>`, and `dark-factory scenario status` and get the same quality of output they get from the equivalent `prompt` and `spec` commands. Scenarios become visible from the CLI without touching the filesystem.

## Non-goals

- No scaffolding (`scenario new`) — template selection and next-number assignment are a separate scope.
- No execution (`scenario run`) — running scenarios is the job of the `dark-factory:run-scenario` Claude skill, not the CLI.
- No lifecycle transitions (`activate`, `deprecate`, `draft`) — status changes are rare enough to do by editing frontmatter.
- No changes to existing scenario files, the scenario writing guide, or the scenario status lifecycle.
- No changes to `prompt` or `spec` command behavior.

## Desired Behavior

1. `dark-factory scenario list` prints a table of all scenarios in `scenarios/` with columns for number, status, and title, sorted by number ascending.
2. `dark-factory scenario show <id>` prints the full contents of one scenario — its frontmatter, title, description line, and body sections — when given either the number prefix or a unique name fragment.
3. `dark-factory scenario status` prints counts grouped by status value (`idea`, `draft`, `active`, `outdated`). If any scenario has an unrecognized or malformed status, an additional `unknown` row is included with the count of such files.
4. `dark-factory scenario` with no subcommand, `--help`, or `-h` prints a help summary in the same style as `dark-factory prompt` and `dark-factory spec`.
5. The top-level `dark-factory help` output lists the new `scenario` subcommands alongside the existing `prompt` and `spec` entries.
6. When `scenarios/` does not exist, `list` and `status` produce empty output and exit successfully; they do not error.
7. When `show <id>` matches no scenario, the command exits with a non-zero status and a clear error message. When it matches more than one, the command exits non-zero and lists all matches so the user can pick.
8. Files in `scenarios/` that do not match the `NNN-*.md` pattern (e.g. a `README.md`) are silently skipped — they neither appear in output nor cause errors.

## Constraints

- Status vocabulary is fixed at `idea`, `draft`, `active`, `outdated` as defined in `docs/scenario-writing.md`. Any other frontmatter status is treated as unknown rather than rejected, so a malformed file can still be listed.
- Filename convention is `NNN-kebab-name.md` where `NNN` is a zero-padded integer. This is the existing convention in `scenarios/` and must not change.
- Title is extracted from the first `# ` heading after the closing `---` of the frontmatter block. Description line (starting "Validates that...") is optional.
- The CLI must follow the existing top-level command-dispatch pattern used by `prompt` and `spec` — `scenario` joins the same router.
- Existing `prompt` and `spec` command behavior, output format, and exit codes must not change.
- Parse failures (missing frontmatter, unreadable file, non-integer number prefix) must not abort the whole command; they either skip the file or surface it with `status=unknown`.
- See `docs/scenario-writing.md` for the scenario format, status lifecycle, and filename convention this spec assumes.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| `scenarios/` directory missing | `list` and `status` print empty / zero counts and exit 0 | None needed — this is a valid empty state |
| File in `scenarios/` does not match `NNN-*.md` | Skip silently | None needed |
| Frontmatter missing or malformed | Include in `list` with status shown as `unknown`; do not crash | User edits frontmatter |
| Title heading missing | Show blank title in `list`; `show` still prints file body | User edits file |
| `show <id>` matches zero scenarios | Exit non-zero with "no scenario matching <id>" | User checks spelling / runs `list` |
| `show <id>` matches multiple scenarios | Exit non-zero, print all matches with numbers and names | User reruns with a more specific fragment or the numeric prefix |
| Duplicate number prefix across two files | Both appear in `list`; `show <number>` reports multiple matches | User renames one of the files |

## Security / Abuse Cases

- All input is the local `scenarios/` directory inside the project's git root — no network, no user-supplied paths outside the repo.
- The `<id>` argument to `show` is used only for matching against known scenario filenames, never as a path component, so directory traversal is not a concern.
- Commands are read-only; they never mutate scenario files.

## Acceptance Criteria

- [ ] `dark-factory scenario list` prints a table with columns number, status, title, sorted by number ascending.
- [ ] `dark-factory scenario show 001` and `dark-factory scenario show workflow-direct` both resolve to scenario 001 and print its contents.
- [ ] `dark-factory scenario status` prints one line per status value with a count, covering at minimum `idea`, `draft`, `active`, `outdated`.
- [ ] `dark-factory scenario` (no subcommand), `dark-factory scenario help`, `dark-factory scenario --help`, and `dark-factory scenario -h` all print the scenario help summary and exit 0.
- [ ] `dark-factory help` mentions the three new `scenario` subcommands.
- [ ] Running any `scenario` subcommand in a repo with no `scenarios/` directory exits 0 with empty output (for `list`/`status`) or a clear not-found error (for `show`).
- [ ] A file in `scenarios/` that does not match `NNN-*.md` is skipped by all three subcommands.
- [ ] A scenario with malformed frontmatter appears in `list` with `status=unknown` and does not crash any subcommand.
- [ ] `dark-factory scenario show <fragment>` matching multiple files exits non-zero and lists all matches.
- [ ] Existing `prompt` and `spec` command output and exit codes are unchanged.
- [ ] `dark-factory scenario status` includes an `unknown` row with a count when any scenario has a malformed or unrecognized status value.

## Verification

```
make precommit
dark-factory scenario list
dark-factory scenario show 001
dark-factory scenario show workflow-direct
dark-factory scenario status
dark-factory scenario
dark-factory help
```

Expected:
- `scenario list` shows all ten current scenarios in numeric order with their statuses.
- Both `show` forms print the same scenario.
- `scenario status` shows a count for each status value in use.
- `scenario` with no subcommand prints the help summary.
- `dark-factory help` includes `scenario list`, `scenario show`, `scenario status`.
- `make precommit` passes.

## Do-Nothing Option

Users continue to read `scenarios/` by hand. This is tolerable today with ten files but becomes painful as the count grows and as more people work with the project. The cost of not doing this is low-grade ongoing friction and an asymmetry with `prompt` and `spec` that makes the CLI feel inconsistent. Acceptable short-term; not acceptable long-term.
