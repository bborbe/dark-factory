---
status: approved
created: "2026-04-16T10:41:23Z"
queued: "2026-04-16T10:47:13Z"
---
<summary>
- `pkg/config/loader.go` currently uses a `partialConfig` struct that is missing fields present on `Config`, so those YAML keys are silently dropped by `yaml.Unmarshal` and never merged onto the defaults
- Concrete proven breakage: `maxPromptDuration`, `dirtyFileThreshold`, and `autoRetryLimit` — a sibling project had `maxPromptDuration: 60m` set and still ran prompts for 2+ hours because the loader never applied the value
- Root cause is shared with several other fields: `partialConfig` lists only 19 of the ~37 `yaml:` tags on `Config` (verified by `grep -c "yaml:" pkg/config/{config,loader}.go` → 56 vs 27)
- This prompt adds the missing pointer fields to `partialConfig`, extends `mergePartial` to copy them, and adds an exhaustive loader test suite that writes one YAML file with a non-default value for every `Config` field and asserts every field round-trips correctly
- The exhaustive test is the real deliverable: it fails today for every missing field and will prevent this class of bug from regressing
- No change to `Defaults()`, no change to `Validate`, no change to CLI — strictly a loader correctness fix
</summary>

<objective>
Close the root cause of the "silent YAML field drop" bug in the dark-factory config loader. A user sets a field in `.dark-factory.yaml`, the loader parses the file without error, and the setting is silently discarded because the internal `partialConfig` struct does not declare that field. This has already caused a production incident: prompts that should have been killed at `60m` ran past 2 hours each (billomat 009–015), because `maxPromptDuration` was not wired into `partialConfig`.

Fix the three known-broken fields AND add a loader test that covers every single field on `Config`, so any future field addition on `Config` that forgets to update `partialConfig` fails the tests immediately.
</objective>

<context>
Read `/workspace/CLAUDE.md` for project conventions: errors wrapping with `github.com/bborbe/errors` (no `fmt.Errorf`, no bare `return err`), Ginkgo/Gomega tests, Counterfeiter mocks.

Read these source files in full before editing:

- `pkg/config/config.go` — the `Config` struct (lines ~82–120). Note the `yaml:` tag on every field; that is the source of truth for what the loader must accept. Fields of interest for this prompt:
  - `MaxPromptDuration string` yaml:`maxPromptDuration` (line ~118)
  - `DirtyFileThreshold int` yaml:`dirtyFileThreshold,omitempty` (line ~117)
  - `AutoRetryLimit int` yaml:`autoRetryLimit` (line ~119)
  - And ~15 other fields that are similarly absent from `partialConfig` — enumerate them yourself by diffing the `yaml:` tags of `Config` vs `partialConfig`.
  - `Defaults()` at line ~123 sets some defaults but deliberately leaves `MaxPromptDuration = ""` (empty = disabled) and `AutoRetryLimit = 0`, `DirtyFileThreshold = 0`. Keep those defaults unchanged.
  - `ParsedMaxPromptDuration()` at line ~239 returns `0` on empty or invalid input — keep this untouched.

- `pkg/config/loader.go` — the `partialConfig` struct at line ~53, the `fileLoader.Load` method at line ~76, and `mergePartial` at line ~142. Every yaml key on `Config` that matters at runtime must have a matching pointer field on `partialConfig` AND a corresponding merge block in `mergePartial`. The pointer-field pattern is intentional: it distinguishes "user set this to the zero value" from "user did not set this at all" so defaults win in the latter case. Keep that pattern — do not switch to unmarshalling directly into `Config`.

- `pkg/config/config_test.go` — there is already a `Describe("Loader", ...)` block around line 1030 with a `tmpDir` + `chdir` pattern. Reuse that exact setup for the new exhaustive test.

- `pkg/processor/processor.go` around line 369 (`computeReattachDuration`) and the `executor.buildRunFuncsWithTimeout` path in `pkg/executor/executor.go` line ~187 — these are the consumers of `MaxPromptDuration`. No change needed here; they already read the resolved `time.Duration` correctly. The only reason the timeout did not fire in production is that the loader never populated `cfg.MaxPromptDuration` from YAML, so `ParsedMaxPromptDuration()` always returned `0` which disables the timeout kill path.

Concrete reproduction (verified against `/private/tmp/claude-501/.../bi0lamwjw.output` billomat daemon stdout on 2026-04-16):
- Project `.dark-factory.yaml` contained `maxPromptDuration: 60m`, `dirtyFileThreshold: 500`, `autoRetryLimit: 3`.
- Billomat prompts 009–014 each ran between 1h 56m and 2h 19m; prompt 015 was still running at 2h 30m when investigated.
- Zero `"container exceeded maxPromptDuration"` warnings in the log.
- Only `"stopping stuck container: completion report found"` messages (from `watchForCompletionReport`, unrelated to the timeout).
- `dark-factory config` printed the same cfg values as `Defaults()` rather than the values in the YAML file.

The fix must target the loader specifically. Do not touch the executor, processor, or factory wiring.
</context>

<requirements>

## 1. Identify every missing field

Diff the `yaml:` tags on `Config` in `pkg/config/config.go` against the `yaml:` tags on `partialConfig` in `pkg/config/loader.go`. For every `Config` field that has a `yaml:` tag and is NOT represented on `partialConfig`, that is a bug.

Known-missing, verified during investigation (at minimum, these must be fixed):

- `MaxPromptDuration` → `yaml:"maxPromptDuration"` → add `MaxPromptDuration *string`
- `DirtyFileThreshold` → `yaml:"dirtyFileThreshold,omitempty"` → add `DirtyFileThreshold *int`
- `AutoRetryLimit` → `yaml:"autoRetryLimit"` → add `AutoRetryLimit *int`

You are responsible for discovering the others by diffing the tags. Based on inspection, the following additional fields are also missing from `partialConfig` and must be added as pointer fields with the same yaml key (verify each one yourself):

- `ProjectName *string`
- `Model *string`
- `ValidationCommand *string`
- `ValidationPrompt *string`
- `TestCommand *string`
- `AutoReview *bool`
- `MaxReviewRetries *int`
- `AllowedReviewers []string` (slice — nil means "not set", same pattern as `Env` / `ExtraMounts`)
- `UseCollaborators *bool`
- `PollIntervalSec *int`
- `Provider *Provider`
- `Bitbucket *BitbucketConfig`
- `Notifications *NotificationsConfig`
- `ClaudeDir *string`
- `GenerateCommand *string`

If your diff finds additional fields not in this list, fix them too. If any in this list turns out to already be covered (e.g. by a parent struct that is itself a pointer), skip that one and document the decision in the test comment. Do NOT add fields that do not exist on `Config`.

## 2. Extend `partialConfig` in `pkg/config/loader.go`

Add each missing field as a pointer field with the identical `yaml:` tag (including `omitempty` if present on `Config`). Group the additions logically — keep the existing declaration order and append at the end of the struct, or slot them adjacent to thematically related existing fields, your choice, but keep it readable.

For slice fields (`AllowedReviewers`) use `[]string` directly (not `*[]string`) — a nil slice already signals "not set" and matches the existing `Env` / `ExtraMounts` convention in `partialConfig`.

For embedded-struct fields (`Bitbucket`, `Notifications`, `Provider`) use pointers to the existing `Config`-side types — do NOT introduce new `partialBitbucketConfig` / `partialNotificationsConfig` types. The justification: Bitbucket and Notifications are optional blocks; if the user omits them, the pointer is nil and defaults win. If the user includes the block, the entire struct is taken as-is (we do not need per-sub-field merging for these).

## 3. Extend `mergePartial` in `pkg/config/loader.go`

For every newly added field on `partialConfig`, add a corresponding merge block that follows the existing pattern:

```go
if partial.MaxPromptDuration != nil {
    cfg.MaxPromptDuration = *partial.MaxPromptDuration
}
```

For the slice field:

```go
if partial.AllowedReviewers != nil {
    cfg.AllowedReviewers = partial.AllowedReviewers
}
```

For the nested-struct pointers:

```go
if partial.Bitbucket != nil {
    cfg.Bitbucket = *partial.Bitbucket
}
if partial.Notifications != nil {
    cfg.Notifications = *partial.Notifications
}
if partial.Provider != nil {
    cfg.Provider = *partial.Provider
}
```

Keep the existing block ordering in `mergePartial` and append new blocks at the end, mirroring the order you used in `partialConfig`.

## 4. Do NOT change

- `Defaults()` — no new defaults. `MaxPromptDuration` stays `""`, `DirtyFileThreshold` and `AutoRetryLimit` stay `0`. The empty-string default for `MaxPromptDuration` is the documented "timeout disabled" signal and must stay that way.
- `Config.Validate` — unchanged.
- `ParsedMaxPromptDuration()` — unchanged. Still returns `0` on empty or unparseable.
- `mergePartialPrompts` / `mergePartialSpecs` — unchanged.
- Any code outside `pkg/config/`. The bug is strictly in the loader; the consumers are already correct.
- `go.mod` / `go.sum` / `vendor/`.

## 5. Exhaustive loader test — the real deliverable

Add a new `Describe("loads every Config field from YAML", ...)` block inside the existing `Describe("Loader", ...)` → `Describe("Load", ...)` in `pkg/config/config_test.go` (around line 1056). The existing test file is declared `package config_test` (external test package, line 5) — keep that. Use the existing `tmpDir` / `originalDir` / `chdir` `BeforeEach` — do not duplicate that setup.

The test must:

1. Build ONE YAML document that sets EVERY `yaml:` field on `Config` to a **distinct, non-default, non-zero value** that is still valid (passes `Validate`). For booleans, use the non-default (`true` when the default is `false`, `false` when the default is `true`). For ints, use a small positive non-default value. For strings, use a short recognisable literal (e.g. `"test-<fieldname>"`). For durations, use `"45m"` (must parse). For maps/slices, use a single entry.

2. Write the YAML file to `.dark-factory.yaml` in `tmpDir`, call `loader.Load(ctx)`, and assert `err` is nil.

3. Assert `cfg.<Field>` equals the value written in the YAML — for EVERY field on `Config`. One `Expect` per field, in the same declaration order as `Config`.

4. Provide a helper `fullYAML()` (or inline heredoc) and a helper `assertFullConfig(cfg)` that performs all the per-field assertions, so the test body stays readable.

5. Include a second `It(...)` that asserts the inverse: when `.dark-factory.yaml` is empty (`""`), `loader.Load(ctx)` returns exactly `config.Defaults()` — `Expect(cfg).To(Equal(config.Defaults()))`. This locks in that "empty YAML" and "missing YAML" produce identical results and that no new field accidentally defaults to a non-Defaults value.

6. Include a third `It(...)` specifically for the three originally-reported fields (`maxPromptDuration`, `dirtyFileThreshold`, `autoRetryLimit`) with a minimal YAML that sets only those three, asserting they round-trip. This is the regression test for the reported incident — it must fail today and pass after the fix.

Do NOT try to programmatically reflect over the struct (no `reflect` package in tests). The assertions must be written out one per field; the point is that the test file will fail to compile or fail at runtime for the developer if a new field is added to `Config` and they forget to update the test, which is the desired behaviour. To help future maintainers, add a top-of-block comment:

```go
// If you add a new field to config.Config, you MUST:
//   1. Add the field to partialConfig in loader.go
//   2. Add the merge block to mergePartial
//   3. Add the YAML value to fullYAML() below
//   4. Add the Expect() assertion to assertFullConfig() below
// The loader otherwise silently ignores the YAML key.
```

### Validity constraints on the YAML values

Some `Config` fields are cross-validated. The test YAML must still pass `Validate` — otherwise `Load` returns an error and the test cannot assert per-field round-trip. Concretely:

- `autoMerge: true` requires `pr: true`.
- `autoReview: true` requires `pr: true`, `autoMerge: true`, AND either `allowedReviewers` non-empty OR `useCollaborators: true`.
- `provider: bitbucket-server` requires `bitbucket.baseURL` non-empty.
- `workflow` and `pr`/`worktree` cannot both be set → the full-config YAML MUST NOT include a top-level `workflow:` key; set only `pr: true` and `worktree: true` instead. The loader explicitly rejects configs that have both (`pkg/config/loader.go:106`). Add a separate minimal `It(...)` that asserts the `workflow: pr` deprecation path still maps to `pr=true, worktree=true` (it should already pass — see `Load` at line ~117; this is a guard test, not a fix).
- `provider: bitbucket-server` requires the full bitbucket block: `bitbucket.baseURL: https://bitbucket.example.com` AND `bitbucket.tokenEnv: BITBUCKET_TOKEN`. Set both in the full-config YAML.
- `netrcFile` / `gitconfigFile` must exist on disk — use `tmpDir` + a 0-byte file you create during `BeforeEach` (or inside the `It(...)`), and point the YAML at that path.
- `debounceMs` > 0.
- `serverPort` in 0..65535.
- `env` keys must not be `YOLO_PROMPT_FILE` or `ANTHROPIC_MODEL`.
- `extraMounts` entries must have both `src` and `dst` non-empty.

If any field cannot sensibly be set in the full-config YAML because of a cross-validation constraint, drop it from the full-config test and add a dedicated `It(...)` that loads just that field with its own minimal YAML and asserts the round-trip. Do not leave any `Config` field untested.

## 6. Guard test for "empty yaml file" vs "missing yaml file"

Both cases must produce the exact same `Config` (i.e. `config.Defaults()`). Add a `DescribeTable` (or two `It`s) covering:

- file absent → equals `Defaults()`
- file present with empty content → equals `Defaults()`
- file present with `workflow: pr\n` only → equals `Defaults()` with `PR=true, Worktree=true` and nothing else changed

## 7. Verification

Run `make precommit` in `/workspace` — must exit 0. The Ginkgo suite extended in requirement 5 is the authoritative gate; `make precommit` runs it.

Also run the smaller `make test` first to iterate faster if you want — the tests you add are pure in-memory file round-trip with `os.TempDir`, no Docker, no network.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Do NOT add any new exported names to `pkg/config/` beyond what is strictly required by the fix. `partialConfig` and `mergePartial` stay unexported.
- Wrap all non-nil errors with `errors.Wrap` / `errors.Errorf` from `github.com/bborbe/errors`. Never `fmt.Errorf`, never `errors.New`, never `pkg/errors`.
- Keep error messages lowercase, no file paths in the message.
- Do not introduce new `partial*Config` types for `Bitbucket` / `Notifications` / `Provider` — use pointer-to-Config-type instead (see requirement 2).
- Do not change `Defaults()` — empty `MaxPromptDuration` is the documented "disabled" signal.
- Do not touch the executor, processor, factory, runner, or CLI. The bug is strictly in `pkg/config/loader.go`.
- Do not touch `go.mod` / `go.sum` / `vendor/`.
- Existing tests must still pass.
- No reflection in the test — per-field `Expect` is intentional so the test fails loudly when a new `Config` field is added without updating the loader.
</constraints>

<verification>
1. `cd /workspace && make precommit` must exit 0.
2. The exhaustive round-trip test added in requirement 5 must fail on `master` (before your changes) for the three known-broken fields at minimum, and pass after your changes for every `Config` field.
3. Inspect the diff of `pkg/config/loader.go`: every yaml tag on `Config` must have a matching field on `partialConfig` and a merge block in `mergePartial`. No new field on `Config` should be unhandled.
</verification>
