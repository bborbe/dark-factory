---
status: approved
created: "2026-04-19T20:27:24Z"
queued: "2026-04-19T20:27:27Z"
---

<summary>
- Add a reflection-based invariant test that enforces parity between `Config` and `partialConfig`
- Every leaf field in `Config` with a counterpart in `partialConfig` must round-trip through yaml (load-then-read)
- Catches the class of bugs where a new `Config` field is added but `partialConfig` / `mergePartial*` are not updated, silently dropping yaml values
- Motivated by a real incident where `Config.PreflightCommand` was silently dropped because `partialConfig` lacked the field
- No behavior change — pure test addition
</summary>

<objective>
Add an automated guard against `partialConfig` desync in the `pkg/config` test suite. The test must fail when a developer adds a field to `Config` without threading it through `partialConfig` and the merge helpers.
</objective>

<context>
Read `CLAUDE.md` and `docs/prompt-writing.md` for conventions.
Read `pkg/config/config.go` — the `Config` struct (top-level fields only — nested structs `GitHubConfig`, `BitbucketConfig`, `NotificationsConfig`, `PromptsConfig`, `SpecsConfig` are out of scope for this prompt).
Read `pkg/config/loader.go` — `partialConfig` struct at lines ~53-94, the merge helpers `mergePartialWorkflow`, `mergePartialContainer`, `mergePartialReview`, `mergePartialProviders`, `mergePartialLimits`.
Read `pkg/config/config_test.go` and `pkg/config/config_suite_test.go` to see the existing Ginkgo v2 test suite (external `package config_test`). New tests go into the same test package.
The bug that motivated this: `Config.PreflightCommand` and `Config.PreflightInterval` existed, but `partialConfig` was missing both fields and `mergePartialContainer` had no branch for them. Any `preflightCommand:` yaml value was silently dropped — operator disable workaround had no effect.
</context>

<requirements>
1. Create a single new test file `pkg/config/roundtrip_test.go` in `package config` (INTERNAL test package — needed for reflection access to the unexported `partialConfig`). Add a Ginkgo `Describe("Config/partialConfig parity", ...)` block. The existing `config_suite_test.go` bootstrap auto-discovers specs in the package.
2. Parity sub-test (reflection-based):
   - Walk `reflect.TypeOf(Config{})` fields.
   - For each `Config` field, assert a `partialConfig` field exists with the **same Go struct field name**.
   - Exclusions (skip these fields, document each with a short reason in a `var exclusions = map[string]string{...}`): `Prompts`, `Specs`, `GitHub`, `Bitbucket`, `Notifications`, `Env`, `ExtraMounts`, `AllowedReviewers` ("nested/collection, own merge helpers"); `Worktree` ("zeroed by Load step D, legacy"); `Workflow` ("special legacy pr-enum mapping in Load step A"); `PR`, `AutoMerge`, `AutoReview` ("validation-coupled — round-trip covered via paired-yaml test below, not in isolation").
3. Round-trip sub-test (per-field, isolated scalar):
   - Use `ginkgo/v2.DescribeTable` + `Entry` so each field is a separate entry and failure messages name the yaml key.
   - For each scalar yaml key listed in requirement 4, construct a minimal yaml document with that single key set to a sentinel value that (a) differs from `Defaults()` and the Go zero value, AND (b) passes `Config.Validate()`.
   - Use `t.TempDir()` (or GinkgoT().TempDir()), write yaml, instantiate `fileLoader{configPath: <path>}`, call `Load(ctx)`, assert the resulting `Config` field equals the sentinel.
4. Round-trip coverage — isolated-scalar list (only fields that pass `Validate()` when set alone): `projectName`, `defaultBranch`, `containerImage`, `netrcFile`, `gitconfigFile`, `model`, `validationCommand`, `validationPrompt`, `testCommand`, `debounceMs`, `serverPort`, `autoRelease`, `verificationGate`, `maxReviewRetries`, `useCollaborators`, `pollIntervalSec`, `claudeDir`, `generateCommand`, `additionalInstructions`, `maxContainers`, `dirtyFileThreshold`, `maxPromptDuration`, `autoRetryLimit`, `hideGit`, `preflightCommand`, `preflightInterval`, `provider`.
5. Paired-yaml round-trip sub-test (for validation-coupled fields):
   - `pr: true` must be set together with `workflow: clone` (or another workflow accepting PR).
   - `autoMerge: true` requires `pr: true` + `workflow: clone`.
   - `autoReview: true` requires `pr: true` + `autoMerge: true` + `workflow: clone` + a non-empty `allowedReviewers`.
   - Either cover each in its own `It` (not the table) with a fully valid yaml, OR skip these from round-trip and leave a TODO comment pointing at the future gap. Prefer covering them.
6. Sentinel-value rules:
   - Strings: `"sentinel-<field>"` (e.g., `"sentinel-containerImage"`) — except yaml keys whose value must satisfy a validator. Concretely: `maxPromptDuration` and `preflightInterval` must be valid `time.ParseDuration` strings — use `"7h"` and `"3h"` (not default `"2h"`).
   - Bools: flip from default.
   - Ints: clearly non-default positive values (e.g., `42`, `9999`) within any documented range (`serverPort` 1–65535, `debounceMs` positive).
   - `provider`: use a valid `Provider` enum value that differs from default — check `pkg/config/config.go` for available values.
7. Workflow / Worktree handling:
   - Do NOT include `workflow` in any round-trip test — legacy `pr`-enum mapping in `Load` step A rewrites it.
   - Do NOT include `worktree` — `Load` zeroes `cfg.Worktree = false` unconditionally at step D.
8. The test must currently PASS. If it fails, stop and report — do not modify production code.
9. No changes to `pkg/config/loader.go` or `pkg/config/config.go`. The test uses the internal `package config` declaration for reflection access — no new exports needed.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
- Use Ginkgo v2 / Gomega patterns as elsewhere in the repo.
- Repo-relative paths only.
- Do NOT broaden scope to nested structs (`GitHubConfig` etc.) — document in a code comment that those are excluded and handled by their dedicated merge helpers.
- Do NOT modify `partialConfig` or `mergePartial*`. If the test reveals a real gap, stop and report — do not silently fix.
- Do NOT temporarily break production code as part of verification. The new test is itself the guard.
</constraints>

<verification>
Run `make precommit` — must pass. The new Describe block must appear in `go test -v ./pkg/config/...` output with all entries passing.
</verification>
