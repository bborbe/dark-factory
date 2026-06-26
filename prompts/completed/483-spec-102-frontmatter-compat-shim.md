---
status: completed
spec: [102-executor-backend-neutral-naming]
summary: 'Changed Frontmatter.Container YAML tag to execution_id, added backward-compatible container: read alias via applyLegacyContainerKey helper, and added three named tests covering legacy read, new write, and semantic round-trip.'
container: dark-factory-exec-483-spec-102-frontmatter-compat-shim
dark-factory-version: v0.183.0
created: "2026-06-26T09:00:02Z"
queued: "2026-06-26T10:11:44Z"
started: "2026-06-26T10:33:25Z"
completed: "2026-06-26T10:41:03Z"
branch: dark-factory/executor-backend-neutral-naming
---

<summary>

- Makes prompt files write the execution identifier under the neutral key `execution_id:` instead of `container:`.
- Keeps reading old prompt files that still use `container:` — they load with the execution ID populated exactly as before, so nothing on disk breaks.
- When a file somehow has both keys, the canonical `execution_id:` wins and a warning is logged naming the conflict.
- Logs an informational line when a file is loaded via the legacy `container:` alias, so operators can see which files predate the rename.
- Adds unit tests proving legacy read works and new write emits `execution_id:`, plus an integration test proving a legacy file round-trips through load+save with zero on-disk diff.
- No runtime behavior change beyond the YAML key name; the in-memory Go field is unchanged so no other package needs editing.

</summary>

<objective>
Make prompt frontmatter write the execution identifier as `execution_id:` while still reading the legacy `container:` key. When both keys are present, `execution_id:` wins. Add unit + integration tests. This prompt is independent of prompts 1 and 2 (it only changes YAML serialization, not Go type names).
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` first, then `/workspace/CLAUDE.md`.

Read the parent spec:
- `/workspace/specs/in-progress/102-executor-backend-neutral-naming.md` — Desired Behavior 5; Acceptance Criteria 4, 5, 6; the Failure Modes table rows 1, 2, and 6 (legacy-only key, both-keys conflict, old-binary-reads-new-file).

NOTE on package name: the spec text says tests live in `pkg/promptfile`, but the actual frontmatter package in this repo is `pkg/prompt`. Implement everything in `pkg/prompt`. The neutral Go field is `Frontmatter.Container` (a `string`). Do NOT rename the Go field — only its YAML tag and the read-compat change. Keeping the Go field name avoids rippling into promptresumer/runner/status which read `pf.Frontmatter.Container`.

Read these source files fully:
- `/workspace/pkg/prompt/prompt.go`:
  - `Frontmatter` struct (lines 245-265). The relevant field is `Container string \`yaml:"container,omitempty"\`` (line 251).
  - `load(ctx, path, currentDateTimeGetter)` (lines 312-359) — note the existing read-compat block for the legacy `rejected_reason` key (lines 342-349). Mirror this exact pattern for the `container:` alias.
  - `Save(ctx)` (lines 361-389) — marshals `pf.Frontmatter` via `yaml.Marshal`.
  - Logging in this file uses `slog` (e.g. `slog.Debug`, `slog.InfoContext`). Use `slog.InfoContext(ctx, ...)` for the legacy-alias info line and `slog.WarnContext(ctx, ...)` for the conflict line, matching the file's existing style at line 852.
- `/workspace/pkg/prompt/prompt_test.go` (lines 818-836) — the `StampRejected` test shows the load pattern: `prompt.NewManager("", "", "", "", nil, libtime.NewCurrentDateTime()).Load(ctx, path)`. Use this to load files in your tests. Tests are `package prompt_test`, Ginkgo/Gomega.
- `/workspace/pkg/prompt/prompt_suite_test.go` — suite registration; `package prompt_test`.
</context>

<requirements>

## 1. Emit `execution_id:` on write

1.1. In `/workspace/pkg/prompt/prompt.go`, change the YAML tag of the `Container` field (line 251) from:
```go
	Container          string   `yaml:"container,omitempty"`
```
to:
```go
	Container          string   `yaml:"execution_id,omitempty"`
```
Keep the Go field name `Container` so existing readers (`pf.Frontmatter.Container` in promptresumer/runner/status) compile unchanged. Add a doc comment above the field: `// Container holds the execution identifier. Written as execution_id; container is accepted on read as a legacy alias (spec 102).`

## 2. Accept `container:` on read (legacy alias)

2.1. In the `load` function, AFTER the main `frontmatter.Parse` populates `fm` (and after the existing `rejected_reason` read-compat block at lines 342-349), add a read-compat block that mirrors the `rejected_reason` pattern. With the tag now `execution_id`, a file containing only `container:` will leave `fm.Container` empty after the main parse; so:

```go
	// Read-compat for legacy `container:` key (spec 102). Files written before the
	// execution-neutral migration carry `container:`; accept that on read and populate
	// the same field. Writes always emit `execution_id:`. When both keys are present,
	// `execution_id:` wins (it was already populated by the main parse above).
	if fm.Container == "" {
		var legacy struct {
			ExecutionID string `yaml:"execution_id,omitempty"`
			Container   string `yaml:"container,omitempty"`
		}
		if _, parseErr := frontmatter.Parse(bytes.NewReader(content), &legacy, yamlV3Format); parseErr == nil {
			if legacy.Container != "" {
				fm.Container = legacy.Container
				slog.InfoContext(ctx, "prompt_load legacy_container_alias=true", "file", path)
			}
		}
	} else {
		// fm.Container is non-empty: execution_id was present. If the file ALSO carries a
		// stale `container:` key, warn so the operator can remove it.
		var legacy struct {
			Container string `yaml:"container,omitempty"`
		}
		if _, parseErr := frontmatter.Parse(bytes.NewReader(content), &legacy, yamlV3Format); parseErr == nil && legacy.Container != "" {
			slog.WarnContext(ctx, "prompt_load conflicting_keys", "execution_id", fm.Container, "container", legacy.Container, "file", path)
		}
	}
```

Confirm `slog` is already imported in `prompt.go` (it is — used at line 852). Confirm `bytes`, `frontmatter`, and the `yamlV3Format` variable are in scope inside `load` (they are — used at lines 326-327).

## 3. Unit test: legacy read

3.1. Add a Ginkgo test (in `pkg/prompt/prompt_test.go` or a new `pkg/prompt/frontmatter_compat_test.go`, `package prompt_test`) with an `It` block whose description is exactly `TestLoadAcceptsLegacyContainerKey` semantics — to satisfy the AC's named-test requirement, ALSO add a standard Go test function `func TestLoadAcceptsLegacyContainerKey(t *testing.T)` in the same `package prompt_test` file (a plain `testing.T` test, not Ginkgo, so `go test -run TestLoadAcceptsLegacyContainerKey` matches it).

The `TestLoadAcceptsLegacyContainerKey` test must:
   - Write a temp prompt file whose frontmatter contains `container: legacy-name-123` and NO `execution_id:` key, plus a `# Title` body.
   - Load it via `prompt.NewManager("", "", "", "", nil, libtime.NewCurrentDateTime()).Load(ctx, path)`.
   - Assert `pf.Frontmatter.Container == "legacy-name-123"`. Use `t.Fatalf` on mismatch.

## 4. Unit test: write emits `execution_id:`

4.1. Add `func TestSaveEmitsExecutionIDKey(t *testing.T)` (plain `testing.T`, `package prompt_test`):
   - Construct a `PromptFile` via `prompt.NewPromptFile(path, prompt.Frontmatter{Status: "executing", Container: "exec-abc"}, []byte("# Title\n\nBody\n"), libtime.NewCurrentDateTime())`.
   - Call `pf.Save(ctx)`.
   - Read the file back as raw bytes; assert the content contains `execution_id: exec-abc` and does NOT contain a line matching `^container:` (use `strings.Contains` for the positive check and a check that `"\ncontainer:"` is absent / a regexp `(?m)^container:` returns no match). Use `t.Fatalf` on failure.

## 5. Integration test: legacy round-trip

5.1. Add `func TestLegacyContainerKeyRoundTripsUnchanged(t *testing.T)` (plain `testing.T`, `package prompt_test`):
   - Create a fixture file in a temp dir with EXACT bytes:
     ```
     ---
     status: executing
     container: foo
     ---

     # Title

     Body content.
     ```
     (This represents a real pre-spec prompt with the legacy key.)
   - This AC requires: load it, then Save WITHOUT mutating any field, and `diff` against the original returns zero lines. With the `execution_id` tag, Save will re-emit the key as `execution_id: foo`, NOT `container: foo` — which would make a naive byte-diff FAIL. Resolve this correctly: the spec's AC #6 says "saves it back **without mutation** ... diff returns zero lines". A load→save with the new writer canonicalizes the key, so a byte-identical round-trip is impossible for a legacy file. Therefore this test asserts the SEMANTIC round-trip: after load→save→reload, `pf2.Frontmatter.Container == "foo"` AND the saved file contains `execution_id: foo`. Document this in a test comment: "Legacy files canonicalize to execution_id on save by design (spec 102 writers emit execution_id only); we assert semantic equality, not byte-identity." Use `t.Fatalf` on failure.

   NOTE for the spec verifier: the literal byte-diff phrasing in AC #6 is incompatible with the canonical-write requirement in Desired Behavior 5 ("Writers emit execution_id: only"). The semantic round-trip above is the correct interpretation; flag this AC-wording conflict in `## Improvements` (category: PROMPT).

## 6. Changelog

Append to `## Unreleased`:
```
- feat: Prompt frontmatter writes execution_id and reads legacy container key (backward compatible) (spec 102)
```

</requirements>

<constraints>
- No new third-party dependencies.
- BSD license headers preserved on every touched file.
- Backward compatible: a prompt written before this spec (with `container:`) must load unchanged after this lands. The `container:` alias is supported on read.
- No behavior change at runtime beyond the YAML key name.
- Do NOT rename the Go field `Frontmatter.Container` (avoids rippling into promptresumer/runner/status).
- Coverage: the new `load` read-compat branches and the YAML-tag write path must be covered by the tests above (legacy-only, both-keys-conflict, new-write).
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass — verify no existing test asserts on the literal `container:` YAML key (grep `container:` in `pkg/prompt/*_test.go` and `pkg/status/*_test.go` before finishing; update any that assert the old key).
</constraints>

<verification>
Run from `/workspace`:

```
go test ./pkg/prompt/... -run 'TestLoadAcceptsLegacyContainerKey|TestSaveEmitsExecutionIDKey|TestLegacyContainerKeyRoundTripsUnchanged' -v
grep -rn 'yaml:"container' pkg/prompt/prompt.go
grep -rn '"\\ncontainer:"\|container:' pkg/prompt/*_test.go pkg/status/*_test.go
go test -coverprofile=/tmp/cover.out ./pkg/prompt/... && go tool cover -func=/tmp/cover.out | grep -E 'load|Save'
make precommit
```

Expected: the three named tests pass; the YAML tag grep shows `execution_id`; no existing test depends on the literal `container:` key (or they were updated); `make precommit` exits 0.
</verification>
