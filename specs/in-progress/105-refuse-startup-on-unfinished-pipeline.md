---
status: prompted
approved: "2026-09-15T07:37:57Z"
generating: "2026-09-15T07:41:33Z"
prompted: "2026-09-15T07:59:56Z"
branch: feature/pipeline-gate
---

## Summary

- `dark-factory doctor` ships eight detectors (`pkg/doctor/`), none of which detects a **gap in `prompts/completed/`**
- That gap is the one leftover class that silently blocks execution: `findMissingCompleted` (`pkg/prompt/prompt.go:1670`) returns every number below the candidate absent from `completed/`, and the spec-less guard at `:1086` refuses to run the prompt
- Observed on `bborbe/vault-cli`: 186 and 187 absent from `completed/` since 2026-08-21 (commit `79a6949`); every spec-less prompt has been blocked since, reporting `Blocked: (reason=previous-prompt-not-completed, missing=186)`
- `dark-factory doctor` on that repo today exits 1 with 11 findings and names **neither 186 nor 187** — it is red for parked specs and dir mismatches, not for the gap
- `checker.Check` additionally hard-errors `not a dark-factory project` (`pkg/doctor/doctor.go:108`) on a repo carrying `.dark-factory.yaml` with no pipeline tree, so callers cannot use it as a clean/dirty oracle
- Nothing runs any of this at daemon startup, so a dirty pipeline is discovered only when new work mysteriously never executes

## Problem

The daemon's startup gate verifies that the **build** is green (`preflightCommand`, `make precommit`) and never that the **pipeline** is clean. The two failure modes are both silent:

1. **A blocked queue looks like an idle queue.** A spec-less prompt below a `completed/` gap is skipped every scan with a log line nobody is reading. On `vault-cli` this has been true for over three weeks.
2. **Stale backlog burns container runs.** Specs parked at `verifying` are re-entered ahead of newly approved work.

Both put a human back in the loop, which is the property the factory exists to remove. The diagnosis is also expensive to re-derive: it took tracing three call sites to establish why a prompt "wasn't running".

A second consumer needs the same detection. The merge-gate sibling (`check-unfinished-pipeline.yml`) downloads a pinned `dark-factory` release and shells out to `doctor` to block PRs that would leave unfinished state. It is written and locally proven, and is blocked on this spec shipping. If detection were implemented inside the startup gate instead of in `doctor`, the two mechanisms would be two implementations of one definition, free to drift.

## Goal

`dark-factory doctor` gains a ninth detector that reports missing `prompts/completed/` numbers by number, tolerates an uninitialized project, and is reused — not reimplemented — by a new daemon startup gate that refuses to start on a dirty pipeline and names what blocks it.

The shared definition of "unfinished", agreed with the merge-gate sibling:

1. **Missing completed number** — a prompt number below the highest number present anywhere in `prompts/` that is absent from `prompts/completed/`. Independent of whether that number's file exists anywhere.
2. **Parked prompt** — a file in `prompts/in-progress/` whose `status` is not `executing` or `committing`.
3. **Parked spec** — `specs/in-progress/` with `status: verifying` whose **`verifying` frontmatter timestamp** is older than the threshold (default 24h), or `status: generating` with no corresponding prompt in `prompts/in-progress/`.

Conditions 2 and 3 are already covered by existing detectors. Only condition 1 is new.

## Non-goals

- **Do not implement a second detector.** The gate calls `pkg/doctor`; it does not re-derive state. Two callers of one detector set is the point.
- Do not compute staleness from file mtime. `detectVerifyingStale` (`verifying_stale.go:45-79`) parses the `verifying` frontmatter timestamp as RFC3339, and mtime is wrong: a `git checkout` rewrites mtimes and would silently reset every staleness clock in the tree.
- Do not cache the gate result. `healthcheckgate` caches because a probe sequence is expensive; a pipeline scan is a filesystem read, and a cached pass would hide a leftover that landed mid-session.
- Do not auto-remediate. The gate reports and refuses; `doctor --fix` already owns repair.
- Do not redesign `AllPreviousCompleted` or the spec-conditional carve-out at `prompt.go:1086`. The detector reads the same state the guard reads; it does not change how the guard decides.
- Do not change `doctor`'s exit code contract for initialized repos (findings → exit 1).
- Do not arm the gate by default (see AC7) and do not wire it into CI here — arming is an operational decision owned outside this spec.

## Acceptance Criteria

> ⚠️ **Assert on `Finding.Category`, never on raw output.** Every detector prints file paths, and every path contains the prompt number, so `grep 186` matches the filename of any unrelated finding on that file. Two independent fixtures built for this spec "passed" on `orphan-prompt-link` findings against the *unchanged* binary. Every fixture below must additionally be clean under today's released binary (`rc=0`, "no findings"), so a red result can only come from the new detector.

- [ ] **AC1**: A ninth `Category` constant `missing-completed-prompt` exists alongside the existing eight in `pkg/doctor/doctor.go`, and its detector is invoked from `checker.Check`. Evidence: unit test asserting `Check` on a fixture with a gap returns a `Finding` whose `Category` equals that constant.
- [ ] **AC2**: The detector reports a number absent from `prompts/completed/` while **present** in `prompts/in-progress/` — the 186/187 shape. Evidence: unit test with spec-less prompts `185`/`188` in `completed/` and `186` (`status: failed`) / `187` (`status: approved`) in `in-progress/`; two `missing-completed-prompt` findings whose numbers are `186` and `187`. Prompts are spec-less deliberately — a `spec:` field pointing at an absent spec raises `orphan-prompt-link` and makes the fixture pass for the wrong reason, and spec-less is the class a gap actually blocks.
- [ ] **AC3**: The detector reports a number absent from **both** trees — the never-created gap. Evidence: unit test where `186` exists in neither `completed/` nor `in-progress/` while `185` and `188` exist in `completed/`; one `missing-completed-prompt` finding naming `186`. This is the shape a directory listing cannot detect, and it is what forces the range scan rather than an enumeration.
- [ ] **AC4**: A contiguous tree produces no finding from the new detector. Evidence: unit test with `completed/` running `1…N` with no gap; the new category contributes zero findings.
- [ ] **AC5**: `checker.Check` on a directory carrying `.dark-factory.yaml` but no pipeline tree returns **no findings and no error**, replacing today's `not a dark-factory project: missing <dir>` hard-error (`doctor.go:108`). Evidence: unit test asserting `(nil findings, nil error)` for a temp dir with no `specs/in-progress`; plus `dark-factory doctor` in such a dir exits 0.
- [ ] **AC6**: A new daemon startup gate refuses to start when `doctor` reports findings, and its error output names the offending numbers. Evidence: unit test asserting the gate returns a non-nil error whose message contains the offending numbers when its injected checker yields findings, and `nil` when it yields none. The gate obtains findings by calling the `pkg/doctor` checker — no duplicate scan logic in the gate package.
- [ ] **AC7**: The gate has independent `enabled` and `skip` controls, modelled on `pkg/healthcheckgate`. Evidence: unit tests — disabled gate returns `nil` regardless of findings; enabled + skip returns `nil`; enabled without skip returns the error. The default value of `enabled` is a single named constant so it can be flipped in one line.
- [ ] **AC8**: The gate performs **no caching** — two consecutive `Check` calls with a checker that reports findings on the second call only must fail on the second. Evidence: unit test with a fake checker returning no findings then findings; the second `Check` returns an error.
- [ ] **AC9**: Existing behaviour is unchanged. Evidence: `make precommit` green; the existing eight detectors' tests pass untouched; a clean-tree daemon still runs `preflightCommand` and still exits when the baseline is broken.

## Verification

```bash
cd ~/Documents/workspaces/dark-factory-pipelinegate
make precommit
go test ./pkg/doctor/... -v
```

End-to-end against pre-built fixtures. **All four read `rc=0` / "no findings" under today's released binary except the last**, so any red below can only come from the new detector:

```bash
# Expect: red, category missing-completed-prompt, numbers 186 + 187
cd /tmp/df-fixture-186187   && /tmp/new-dark-factory doctor; echo "EXIT=$?"

# Expect: red, category missing-completed-prompt, number 186 (absent from BOTH trees)
cd /tmp/df-fixture-gap-both && /tmp/new-dark-factory doctor; echo "EXIT=$?"

# Expect: EXIT=0 — contiguous tree must STAY green
cd /tmp/df-fixture-clean    && /tmp/new-dark-factory doctor; echo "EXIT=$?"

# Expect: EXIT=0 — today this is EXIT=1 "not a dark-factory project" (doctor.go:108)
cd /tmp/df-fixture-noprompts && /tmp/new-dark-factory doctor; echo "EXIT=$?"
```

Do **not** re-derive these fixtures from live `bborbe/vault-cli` — a sibling task is sweeping that repo and will delete the motivating state.

Per `docs/releasing-dark-factory.md`, verify against a freshly built binary, never the installed one.
