# Changelog

All notable changes to this project will be documented in this file.

Please choose versions by [Semantic Versioning](http://semver.org/).

* MAJOR version when you make incompatible API changes,
* MINOR version when you add functionality in a backwards-compatible manner, and
* PATCH version when you make backwards-compatible bug fixes.

> **Known-broken versions:** `v0.179.0` and `v0.179.1` shipped a `dark-factory healthcheck` subcommand that did not actually work — boot/mount/claude probes failed against any real `.dark-factory.yaml` project (container-name leading `-`, foreground `docker run` design never executed wait/exec, mount probe missing `/workspace` bind, claude probe missing `<claudeDir>` mount). All other commands (`run`, `daemon`, `spec`, `prompt`, `doctor`) function normally in those versions. Fixed in `v0.180.0+`. `go install github.com/bborbe/dark-factory@latest` picks up the fix; only pinned `@v0.179.x` consumers see broken healthcheck.

## Unreleased

- feat: add `pkg/launchpolicy` package with `Policy` value type and `BuildOpts` method that centralises the shared container launch shape (image, project, mounts, base env, netrc/gitconfig, hide-git, canonical `NET_ADMIN`+`NET_RAW` caps) for both the executor prompt-run path and the healthcheck probes (spec 098 prompt 1)
- refactor: migrate executor's prompt-run path to consume `launchpolicy.Policy` via `BuildOpts`; remove inline `CapAdd: []string{"NET_ADMIN", "NET_RAW"}` from `pkg/executor/executor.go`; move `ContainerLaunchOpts` struct to `pkg/launchpolicy` (type alias preserved in `pkg/executor` for callers); wire `Policy` through both `executor.NewDockerExecutor` call sites in factory (spec 098 prompt 2)
- refactor: migrate healthcheck boot/mount/claude probes to consume `launchpolicy.Policy` instead of removed `ProbeLaunchConfig`; probes now share the same launch path as production prompt containers (including `CanonicalCaps`) by construction; factory wires a single `launchpolicy.NewPolicy` call to all three probes; adds `DescribeTable` test asserting each probe's argv carries `--cap-add=NET_ADMIN` and `--cap-add=NET_RAW` (spec 098 prompt 3)
- test: add `pkg/launchpolicy/regression_lock_test.go` regression-lock test that injects a synthetic `SYS_PTRACE` cap via `Policy.WithCapAddForTest` and asserts it propagates to both `executor.BuildDockerRunArgs` (executor call path) and each of the boot/mount/claude probes' argv; also adds `executor.BuildDockerCommandFromPolicyForTest` helper to `pkg/executor/export_test.go` (spec 098 prompt 4)
- fix: healthcheck probes now carry NET_ADMIN/NET_RAW caps; daemon starts on OrbStack without --skip-healthcheck (spec 098)
- refactor: pkg/launchpolicy is the single source of truth for container launch shape; canonical capability set lives there only (spec 098)

## v0.182.0

- refactor: replace `FileLock`/`NewFileLock` sidecar-file locking with `DirLock`/`NewDirLock` directory flock in `pkg/lock`; no `.lock` files created on disk, crash-release handled automatically by kernel
- feat: add directory-scoped locking to `spec approve`, `spec reject`, `spec complete`, `spec unapprove`, and `spec mark-prompted` commands so concurrent CLI invocations on the same directory serialize; lock acquired before first file read and held across the move
- feat: add `dark-factory doctor` `legacy-lock-file` check that removes pre-existing `*.md.lock` litter from the old per-file locking scheme; fixer is idempotent and audit-logged (spec 097)

## v0.181.0

- feat: add `healthcheckEnabled` (*bool, default nil=true) and `healthcheckInterval` (string, default "8h") to Config, with validation, parsed getters, source detection in FieldSources, and four new keys in the effective-config startup log line
- feat: add `pkg/healthcheckgate` package providing a `Gate` that runs the healthcheck probe sequence at daemon startup with success-only on-disk caching keyed by SHA256 of container image, project name, and interval; corrupt or future-dated cache treated as a miss; failure fires `healthcheck_failed` notification and returns a category-naming error
- feat: wire `healthcheckgate.Gate` into `dark-factory daemon` startup; gate runs after preflight, before watcher loop; `healthcheckEnabled: false` in config disables gate entirely; add `--skip-healthcheck` CLI flag (position-agnostic) to bypass gate at invocation time
- feat: document daemon healthcheck startup gate — `healthcheckEnabled`/`healthcheckInterval` config reference, `--skip-healthcheck` flag, cross-reference in running.md healthcheck section, terminal-failure policy in architecture-flow.md

## v0.180.2

- docs(release): codify "release-gate scenarios test EXISTING features, not new ones" in `docs/releasing-dark-factory.md`. Any release introducing or substantively changing a user-facing subcommand now requires a new-feature live-smoke against ≥2 real targets before the gate is considered green. For spec-driven releases, `/dark-factory:verify-spec <id>` is the live-smoke and must not be skipped — `spec complete` without `verify-spec` is the failure mode that let `v0.179.0`/`v0.179.1` ship a non-functional `healthcheck`.
- docs(changelog): header note in `CHANGELOG.md` flags `v0.179.0` and `v0.179.1` as broken for the healthcheck command, with explicit redirect to `@latest`/`v0.180.0+`. Standard `go.mod retract` is unavailable because this project's `go-modtool` formatter strips retract blocks.

## v0.180.1

- fix(healthcheck): raise the claude probe timeout from 10s to 30s. Cold-start `claude` inside the YOLO container involves auth load + an Anthropic-or-MiniMax round-trip, which routinely takes 5-25s on a fresh container on a laptop. The 10s deadline caused flaky verify-spec failures even on green stacks; runs that took 4-5s when warm timed out when cold. 30s gives realistic headroom while still catching genuinely stuck sessions. Warn-after raised from 2s to 5s to match.
- docs(healthcheck): drop stale `--no-claude` references from `docs/running.md` and `docs/troubleshooting.md` — the flag was removed in v0.180.0 but the docs lagged the code. Spec 095 acceptance criteria amended to match the all-or-nothing contract: any unknown flag (including `--no-claude`) exits non-zero with "unknown flag" error; `--help` is the only accepted flag.

## v0.180.0

- fix(healthcheck)!: rewrite `dark-factory healthcheck` to use the same docker-launch path as production prompt containers, and remove the `--no-claude` flag entirely. **Breaking:** `--no-claude` is no longer accepted — skipping the highest-failure-rate probe defeats the purpose of a healthcheck. The claude probe is required.
  - **New shared launch helper `executor.BuildDockerRunArgs(opts ContainerLaunchOpts)`** in `pkg/executor/launch.go` — single source of truth for `docker run --rm ...` argv assembly. Used by `dockerExecutor.buildDockerCommand` (production prompt containers) AND by the healthcheck boot/mount/claude probes. By construction, a regression in the production launch path now surfaces in the healthcheck immediately.
  - **`pkg/runner/probe.go` deleted.** The previous `runner.BootContainerProbe` was a duplicate launch implementation that had nothing to do with `pkg/runner/`'s actual purpose (the daemon's event loop) and which silently diverged from production wiring. Boot logic now lives in `pkg/cmd/healthcheck/probes.go` as a thin probe over the shared helper.
  - **`ProbeLaunchConfig` struct** in `pkg/cmd/healthcheck/probes.go` carries all the per-invocation launch knobs (image, mounts, env, hideGit, extraMounts). The factory builds one and shares it across boot/mount/claude — they all hit the same launch path with only their probe-specific `Entrypoint` + `Command` differing.
  - **Boot probe** now boots via `--entrypoint /bin/sh` and asserts `BOOT_OK` in stdout from a single `docker run` call. No detached-container + WaitUntilRunning + docker-exec dance (that design never worked: foreground `docker run` blocks until the container exits, and the claude-yolo default entrypoint exits non-zero with no args).
  - **Mount probe** now mounts `<projectRoot>:/workspace` (it didn't before — so the v0.179.0 mount probe was writing to an in-container overlay, not testing the actual host mount). Uses `--entrypoint /bin/sh` like the boot probe.
  - **Claude probe** now mounts `<claudeDir>:/home/node/.claude` (it didn't before — so v0.179.0's claude probe failed on every real install because the container had no auth credentials) and inherits the same env/mounts/hideGit wiring as production. Uses `--entrypoint claude` and asserts `OK` substring in stdout.
  - **Project-name git-root fallback** (carried from v0.179.1): factory now calls `project.Resolve(cfg.ResolvedProjectOverride()).String()` so container names never start with `-` when the operator omits `project:` / `projectName:` from `.dark-factory.yaml`.
  - **`--no-claude` flag removed** — flag parser, help text, AC reference. Healthcheck is all-or-nothing.
  - **Live-verified** before commit against two repos (`dark-factory`, `coding`) — all six configured probes (docker, image, boot, claude, mount, plus the config-gated gh/notifications when applicable) report green; total wall time ~5.5s.

## v0.179.0

- feat(runner): introduce `BootContainerProbe` in `pkg/runner/probe.go` — boots a throwaway `docker run --rm` container from the configured image with a unique `<project>-healthcheck-boot-<16 hex>` name, waits for it to be running, and runs an in-container `sh -c` to create+delete `.dark-factory-healthcheck/probe` under `/workspace` and assert `BOOT_OK`. The helper detects the "UID remap to root" / mount-permission regression that scenario 003 catches. Uses `errors.Wrap`/`errors.Wrapf` from `github.com/bborbe/errors` for every error path, `log/slog` for success/failure logging, and the same `if m.IsReadonly() { ":ro" }` rule as `dockerExecutor.buildDockerCommand`. Re-export helpers `TruncateForTest` and `UniqueContainerNameForTest` added to `pkg/runner/export_test.go`; new Ginkgo coverage in `pkg/runner/probe_test.go` exercises the green path, the `WaitUntilRunning` failure, the missing-`BOOT_OK` stdout, the `docker run` failure, the `docker exec` failure, the extraMounts append + `:ro` path, the extraMounts skip-on-missing-src path, the unique-name-per-invocation property, the `truncate` helper, and the `uniqueContainerName` shape.
- feat(cmd): add `dark-factory healthcheck` subcommand plus the four local probes (docker / image / boot / mount) — `pkg/cmd/healthcheck.go` defines the `HealthcheckCommand` interface, private `healthcheckCommand` struct, and `NewHealthcheckCommand(probes Probes)` factory mirroring `pkg/cmd/doctor.go`'s shape; `pkg/cmd/healthcheck/probes.go` defines the `Probe` interface and four exported constructors (`NewDockerProbe`, `NewImageProbe`, `NewBootProbe`, `NewMountProbe`) that emit one `slog.Info` line on success and one `slog.Error` line on failure with `probe=<name>`. Probes run in fixed order (docker → image → boot → mount) and fail-fast; on failure the command prints the category on its own line followed by a 2-space-indented detail line, matching `dark-factory doctor`'s table shape. `--no-claude` is consumed for forward-compat with prompt 2b; `--help`/`-h` is intercepted by `main.go` before reaching `Run`. The factory `CreateHealthcheckCommand` in `pkg/factory/factory.go` wires the probes in the documented order using the production `subproc.NewRunner()`, `executor.NewDockerContainerChecker`, and `&runner.BootContainerProbe{...}` from `cfg`. Counterfeiter mocks generated at `mocks/healthcheck-command.go` and `mocks/healthcheck-probe.go`. New tests in `pkg/cmd/healthcheck_test.go` (unknown-flag rejection, all-green path, first-probe-failure short-circuit, stdout category on failure, `--no-claude` consumption) and `pkg/cmd/healthcheck/probes_test.go` (one `It` per probe — success path with fake `subproc.Runner`, failure path with non-zero exit, `BootProbe` wrapper success+failure). Coverage on the new packages ≥87% statements; CLI surface wired into `main.go`'s `ParseArgs`, `runCommand`, `printCommandHelp`, and `printHelp` so `dark-factory healthcheck --help` prints the usage and exits 0, and `dark-factory help` shows the new command row.
- feat(cmd): extend `dark-factory healthcheck` to the full seven-probe sequence (docker → image → boot → claude → mount → gh → notifications) — three new probe constructors in `pkg/cmd/healthcheck/probes.go` (`NewClaudeProbe`, `NewGhProbe`, `NewNotificationsProbe`) plus a `NotificationsConfigured(cfg config.Config) bool` sentinel. `NewClaudeProbe` runs `docker run --rm <image> claude -p "reply with exactly: OK"` against a one-shot container with a hard 10s timeout (`subproc.NewRunnerWithThresholds(2s, 10s)`) and asserts the literal `OK` substring in stdout; the prompt is a hard-coded constant (`claudeProbePrompt`) so no operator input can reach the Claude shell. `NewGhProbe` runs `gh auth status` with 3s/5s thresholds and surfaces the captured stdout in the wrapped error. `NewNotificationsProbe(cfg, *http.Client)` POSTs a fixed minimal JSON payload (`{"chat_id":...,"text":"dark-factory healthcheck"}` for Telegram; `{"content":"dark-factory healthcheck"}` for Discord) via the injected `*http.Client` (factory passes a 5s-timeout client) and asserts a 2xx response. The full URL is stored in memory but is NEVER logged — only `url.Host` and the response status are surfaced, so the bot token and webhook URL cannot leak via slog. The factory `CreateHealthcheckCommand` appends the gh probe only when `cfg.PR` is true and the notifications probe only when `NotificationsConfigured(cfg)` is true (env var name set AND env value non-empty); the `--no-claude` flag trims the Claude probe at run time. The `healthcheckCommand.Run` orchestrator now emits one `slog.Info "healthcheck probe starting" probe=<name>` line per probe (the captured-output hook the new tests need) and a `slog.Error "healthcheck probe failed"` line on each failure, before printing the categorized table. New Ginkgo coverage in `pkg/cmd/healthcheck/probes_test.go` exercises the green path, the `OK`-missing stdout, the non-zero exit (Claude), the non-zero exit (gh), the no-config no-op, the 2xx success via injected `RoundTripper`, the 401-with-body-snippet failure, and the transport error; a `NotificationsConfigured` block covers the four env combinations. `pkg/cmd/healthcheck_test.go` gets a 7-probe orchestration block with a `slog.SetDefault` capture handler that asserts the seven `probe=<name>` markers appear in the captured output in fixed order, plus negative-assertion tests for `--no-claude`, `pr=false` (gh skipped), and `NotificationsConfigured=false` (notifications skipped). Per-probe timeouts sum to ≤100s worst case; the spec AC (≤15s full / ≤5s `--no-claude` on a warm sandbox) is a property of the green path.
- feat(cli): add `dark-factory healthcheck` subcommand that probes the full pipeline-execution stack (Docker, image, boot, claude, mount, gh, notifications) in fixed order with fail-fast semantics; exit 0 on all-pass, non-zero with a categorized table naming the failing probe; `--no-claude` skips the only token-spending probe. New shared boot helper under `pkg/runner/` is also reused by the scenario-003 smoke test.

## v0.178.3

- fix(lock): per-prompt file lock around the concurrent reject+advance path — `pkg/cmd/reject.go` and `pkg/queuescanner/scanner.go` both take the same `lock.FileLock` on the prompt path before mutating (reject: load→stamp→save→rename; advance: load→re-read post-lock→ProcessPrompt). Wires the previously-dead `prompt.ReasonProjectLockTimeout` constant as the new "blocked" reason emitted by the scanner when the advance loses the lock race; the daemon re-polls on the next cycle instead of silently stalling. The reject-side acquire is unconditional; the scanner-side acquire is conditional on a candidate being selected. The single-slot `s.lastBlockedMsg` dedupe field in the scanner is replaced with a per-key `map[string]struct{}` keyed by `file|spec|reason|missing`, so two distinct blocked specs each log once and re-log only when their blocker changes (the old single-slot key alternated and both re-logged every poll cycle). On resolved block, only the resolved key is cleared — wiping the whole map would re-trigger a log for every other blocked state on every processed prompt. The `FileLockFactory func(path string) lock.FileLock` + `lockTimeout time.Duration` dependency shape mirrors `pkg/doctor/fixer.go`; nil factory defaults to `lock.NewFileLock` and zero timeout defaults to 5s, so existing simple callers can pass `nil, 0`. All call sites updated (`pkg/factory/factory.go` wires the production default; tests pass `nil, 0`). Replaces the vacuous concurrency test in `pkg/queuescanner/scanner_test.go` with a real contention test that races a real `cmd.NewRejectCommand` against the real `queuescanner.Scanner` over a real prompt fixture and asserts the regression-lock property: exactly one final on-disk state (`in-progress/` XOR `rejected/`, never both, never neither). The test uses a pre-acquired starter `FileLock` to force both contenders onto the lock simultaneously, so the assertion is deterministic and would fail if either side's lock acquire were removed. Removed the misleading "does not invoke the reject command directly" comment on the FileLock-primitive test now that the contention test below it exercises the real reject command. The "Read-compat for legacy rejected_reason key" test in `pkg/cmd/reject_test.go` is extended to call `pf.Save(ctx)` after the legacy load and assert the file on disk now contains `rejectedReason:` and no longer contains `rejected_reason:` — full round-trip proof. Operator checklist in `scenarios/011-reject-spec-cascade.md` line 102 (PROMPT frontmatter) corrected to `rejectedReason:`; line 91 (SPEC frontmatter) intentionally unchanged because `pkg/spec/spec.go` deliberately keeps the snake_case `rejected_reason` key.
- fix(prompt): rename `Frontmatter.RejectedReason` YAML tag from `rejected_reason` to `rejectedReason` (camelCase) so `yq '.rejectedReason' prompts/rejected/<NNN>-*.md` returns the operator-supplied reason instead of `null` (spec 092 contract). Add read-compat on the legacy `rejected_reason:` key in the `load` function so the five existing `prompts/completed/*.md` files written before the migration continue to parse into the same typed field — writes always emit the new camelCase key, no on-disk migration. `pkg/cmd/reject.go` now always stamps `originalStatus` (the pre-stamp value) for both pre-execution and failed-path rejects, so a rejected draft carries `originalStatus: draft` (remediates spec 094 defects 1 & 2). Inverted the draft-reject and idea-reject test assertions in `pkg/cmd/reject_test.go` to assert the spec contract; added an explicit read-compat test that loads a legacy `rejected_reason:` fixture and verifies the typed field round-trips. Updated string-match assertions on prompt-file content to `rejectedReason:`; spec-file content (which uses its own `pkg/spec/spec.go` struct, unchanged per spec 094 constraint) still emits `rejected_reason:`.
- fix(scanner): emit the hyphenated reason enum in the daemon's `prompt blocked` log line so log and `dark-factory status` share one token, sourced from five new exported `Reason*` constants in `pkg/prompt` (`ReasonPreviousPromptNotCompleted`, `ReasonPreviousPromptMissing`, `ReasonPromptFrontmatterParseError`, `ReasonPromptFileReadError`, `ReasonProjectLockTimeout`). The scanner's two `logBlockedOnce` call sites now pass `prompt.ReasonPreviousPromptNotCompleted` (replacing the spaced human string `previous prompt not completed`) and `prompt.ReasonPromptFrontmatterParseError` (replacing the human string `malformed frontmatter`); `GetBlockedPrompt`'s four reason returns reference the same constants (remediates spec 094 defect 3). Rewrote the parity test in `pkg/status/status_test.go` to derive its expected reason from the shared `prompt.ReasonPreviousPromptNotCompleted` symbol (no more hand-written literal). Added a new scanner test in `pkg/queuescanner/scanner_test.go` that captures the `slog` output and asserts both the positive (`reason=previous-prompt-not-completed` is present) and the negative (the spaced human string is absent); updated the existing multi-spec malformed-prompt test to assert the canonical `reason=prompt-frontmatter-parse-error` token. The v0.178.2 `Blocked:` line format in `pkg/status/formatter.go` is byte-stable — no change to the two `Blocked:` format strings.
- fix(scanner): cross-spec queue independence — when a per-spec candidate's predecessor is not completed, the scanner now `continue`s to the next candidate instead of returning `DONE`. An unrelated spec's advanceable candidate is now processed in the same scan cycle. This is the implementation behind spec 092 AC "Per-spec ordering allows unrelated spec to advance" and spec 094 test gap 4. Added a hermetic regression test in `pkg/queuescanner/scanner_test.go` that sets up spec A blocked (predecessor failed/missing) AND spec B advanceable, and asserts BOTH that B is processed AND that A is not — the negative assertion is the regression lock. Added a second hermetic test in the same package that runs a real `cmd.NewRejectCommand` against a real prompt fixture concurrently with the scanner's `ScanAndProcess`, and asserts exactly one final on-disk state (remediates spec 094 test gap 5). All test fixtures use `os.MkdirTemp` paths; no real `prompts/` state is touched.

## v0.178.2

- fix(status): `Blocked:` line now matches the spec-092 contract exactly — `Blocked: NNN (reason=..., missing=MMM)` with a single space after the colon and bare (non-zero-padded) prompt numbers. Previously emitted `Blocked:  %03d` (double space, zero-padded), which broke the spec's anchored regex and any operator tooling parsing the documented format. Test assertions in `pkg/status/status_test.go` updated to the spec format.

## v0.178.1

- fix(committingrecoverer): make Ginkgo suite hermetic against the real repo — every spec in `pkg/committingrecoverer/recoverer_test.go` now runs with cwd inside a sandbox temp git repo (via the shared `BeforeEach` chdir), and a suite-level guard (`assertNotInRealRepo`) fails loudly with a message naming the real-repo cwd if a spec ever reaches the package-level dirty/commit helpers without the sandbox chdir. Stops the recurring data-corruption bug where mid-`precommit` the suite committed the developer's uncommitted working tree onto a live PR branch with the message `Test prompt` (last reproduced on `2e57ae5` and `a779152`). Production behavior, `NewRecoverer` signature, and all callers in `pkg/factory` and the three processor test files are unchanged. Cross-package audit confirms no other `_test.go` reaches the same escape (`grep -rln 'git.HasDirtyFiles|git.CommitAll|git.CommitWithRetry' pkg/ --include='*_test.go'` returns only `pkg/git/git_test.go`).

## v0.178.0

- feat(skills/watch): liveness-based stuck detection — at >=10min elapsed (gen or exec mode), inspect the active container via `docker logs --since=3m | wc -l`. <5 lines → Basso (likely stuck); 0 lines + >=15min elapsed → 3x Sosumi + break (silent stuck, treat as failure). Eliminates the false positive on legitimately slow gen runs (12-18min on heavy specs) and closes the gen-mode coverage gap that the elapsed-time-only `executing since` check missed. Falls back gracefully when docker is unavailable.

## v0.177.1

- chore(deps): bump default claude-yolo container image from `v0.9.1` to `v0.10.1`. Updates `pkg.DefaultContainerImage` (the single source of truth) plus the v0.9.0/v0.9.1 references in `README.md`, `docs/configuration.md`, `docs/init-project.md`, and `docs/yolo-container-setup.md`. Historical version strings inside `specs/completed/` and `prompts/completed/` and the regex-fixture versions in `pkg/config/config_loader_test.go`, `pkg/globalconfig/globalconfig.go`, and `main_internal_test.go` are intentionally untouched — they document past behavior or pin specific parser inputs.

## v0.177.0

- feat(spec-writing): make spec authoring verification-first at write time, not just audit time. The `spec-creator` agent's `<spec_template>` block now orders sections Summary → Problem → Goal → Non-goals → **Acceptance Criteria** → **Verification** → **Desired Behavior** → Constraints → Failure Modes → Security → Suggested Decomposition → Do-Nothing Option (AC + Verification promoted from end-of-template to immediately after Non-goals). The `<workflow>` step 2 (Gather requirements) inserts an explicit verification ask between "Goal" and "what behaviors": *"For that goal, what observable proof would convince you it's reached? Each proof needs an evidence shape — exit code / log line / file diff / HTTP status / kafka message / metric / cluster state / file artifact."* `docs/rules/spec-writing.md` opens its `## Spec Structure` section with the verification-first ordering principle (*"Write the proof before the behavior. If you can't describe how you'd verify the goal, you don't yet know what the spec is asking for."*) and reorders the `### Sections` list to match. The recursive-demonstration spec at `specs/verification-first-spec-authoring.md` is itself authored in the new order, providing first concrete instance of the discipline. `spec-auditor` enforcement and `spec-verifier` execution paths are unchanged — the auditor's section checks are presence-based and order-agnostic, so legacy specs in `specs/in-progress/` and `specs/completed/` continue to pass without modification.

## v0.176.4

- docs: address PR #21 review nits on `docs/choosing-a-flow.md`. Remove the maintainer-project-specific "three-rung ladder" jargon from the verification bullet — replace with a generic reference to `spec-verification.md`. Soften the boundary-case row for "test-only regression PR adding bug coverage" — spec is preferred but a prompt is acceptable when the bug already shipped a fix (the cited `bug-workflow.md` doesn't strictly mandate a spec for the standalone regression case). Add `spec-verification.md` to the Related section so the verify-spec reference in §Concrete Examples has a target.

## v0.176.3

- docs: consolidate the direct vs prompt vs spec authoring decision into a single canonical guide `docs/choosing-a-flow.md`. Demote the decision tables previously restated across `CLAUDE.md`, `docs/architecture-flow.md`, `docs/rules/spec-writing.md`, `docs/rules/prompt-writing.md`, `docs/claude-md-guide.md`, and `commands/read-guides.md` to short pointers at the canonical doc — eliminates the drift that previously had spec-writing.md saying "default: spec" while CLAUDE.md said "code = prompt by default." Adds concrete examples per row, a boundary-cases table (SKILL.md / bash scripts / READMEs with code / agent files), anti-patterns, and a clarifying note that "direct" (this guide) ≠ dark-factory's `workflow: direct` config (an isolation/landing concept covered in `workflows.md`). README Documentation table links the canonical doc first.

## v0.176.2

- docs: add `docs/manual-mode.md` documenting the operator-paced 6-step slash-command chain (create-spec → audit-spec → generate-prompts-for-spec → audit-prompt → run-prompt → verify-spec). Covers when to pick manual mode vs the daemon, trade-offs table, worked example, and failure-recovery notes per step. Linked from README Documentation table. No code change.

## v0.176.1

- fix(commands/run-prompt): drop explicit `name: run-prompt` from frontmatter so the command registers under the plugin namespace as `/dark-factory:run-prompt`, matching every other dark-factory command. Previously it appeared bare as `/run-prompt`, breaking the `/dark-factory:` picker prefix discovery.

## v0.176.0

- fix(prompt): `dark-factory prompt complete <id>` now honours `cfg.AutoRelease` and the new `--release` flag. By default, completion on a non-master branch is commit-only (no CHANGELOG rewrite, no tag, no push) regardless of `autoRelease`; pass `--release` to force release on any branch. The daemon executor path is unchanged.

## v0.175.0

- feat(status): surface the queue-advance guard's refusal in `dark-factory status` — new `Blocked` field on `Status` and a `Blocked:  NNN (reason=<reason>[, missing=MMM])` line under `Queue:` when the daemon refuses to advance; absent on empty or advanceable queues so existing parsers see byte-identical output. `status.PromptManager` gains `GetBlockedPrompt`, implemented on `*prompt.Manager` by delegating to the per-spec `AllPreviousInSpecCompleted` / `FindMissingInSpecCompleted` helpers (spec 092).
- feat(queuescanner): per-spec predecessor lookup — `prompt.PromptScanner.AllPreviousInSpecCompleted` and `FindMissingInSpecCompleted` walk in-progress/ AND completed/ for the same spec, the queue-advance loop in `pkg/queuescanner/scanner.go` iterates candidates and picks the first whose per-spec guard passes (alphabetical tiebreak from `ListQueued`), and the `prompt blocked` log line now carries `spec=<id>`. A failed prompt on one spec no longer blocks unrelated specs (spec 092).
- feat(prompt): widen `dark-factory prompt reject` to accept prompts in `failed` state — adds `OriginalStatus` field to frontmatter, new `StampRejectedWithOriginal` helper, and idempotent re-run after partial move; eliminates manual `git mv failed→completed` workaround (rejects prompts move to `prompts/rejected/` with `originalStatus: failed` preserved alongside `status: rejected` and `rejectedReason: <text>`).
- feat(prompt): `Manager.AllPreviousInSpecCompleted` and `Manager.FindMissingInSpecCompleted` delegate to the new scanner methods.
- fix(skills/watch): drop naive `grep -q "failed"` against `dark-factory status` output (false-positive on prompt filenames containing "failed"); use `dark-factory prompt list` as source of truth for failure detection.
- test(prompt,queuescanner): unit tests for the per-spec helpers (happy path, missing predecessor, gap detection, cross-spec non-interference, empty-spec-id fallback, `specnum.Parse` normalization) and ginkgo tests for the queue-advance loop (cross-spec advance, deterministic tiebreak, missing-predecessor block, spec id in log line, multi-spec malformed). Concurrent reject+advance lock test exercises `lock.NewFileLock` acquire/release.
- chore: add Go stdlib CVEs GO-2026-5037/5038/5039 to vulncheck + osv-scanner ignore lists (separate Go 1.26.4 bump task).

## v0.174.7

- bump claude-yolo container image to v0.9.1 (Go 1.26.4)

## v0.174.4

- fix: watch script false-positive alert on prompt filenames containing "failed"
- update go 1.26.3→1.26.4
- update anthropic-sdk-go v1.38.0→v1.46.0, openai-go v3.32.0→v3.37.0
- update grpc v1.80.0→v1.81.1, otel v1.41.0→v1.44.0
- update gosec, googleapis, golang.org/x and other indirect deps

## v0.174.3

- fix(skills/watch): swallow grep no-match exit in stuck-prompt detector so the watcher survives polls while the daemon is in spec-generation mode (no "executing since" line in status); previously `set -euo pipefail` killed the watcher on the first poll.

## v0.174.2

- fix(doctor): `detectDuplicateSpecNumbers` now sets `TargetPaths` to the loser spec (lex-last), not the surviving specs. The fixer's `filterRelevantRenames` matches reindex's emitted `OldPath` against `filepath.Join(specDir, targetPath)`; since reindex moves the loser, the previous detection emitted survivors as TargetPaths and the filter returned empty. Symptom: `dark-factory doctor --fix --yes` on a duplicate-spec-numbers finding ran reindex (file renamed on disk) but silently skipped `applyDuplicateSpecNumbersRename` — no `previous_id` written, no audit-log entry. Unit test passed because it constructed Finding directly, bypassing detection.

## v0.174.1

- fix(doctor): drop double-move in `fix_renumber` — reindex.Reindex already moves the file; fixer now operates on `NewPath` after the move (the old "MoveFile after Save" path worked only against mocks).
- fix(spec): `Load()` surfaces YAML parse errors via `errors.Wrapf` instead of silently returning an empty-frontmatter `SpecFile`; corrupted spec files are now visible to doctor detectors, the generator, and auto-completers.
- fix(lock): `filelock.Release` reorders Close before flock(LOCK_UN) (closing the fd implicitly releases all flock holds); add `sync.Mutex` to guard `fd` against concurrent Acquire/Release; drop the os.Remove TOCTOU window.
- fix(doctor): audit-before-mutation in all five fix_* functions — audit entry written before any durable mutation so a crash leaves the audit recording intent with no orphan on-disk state.
- fix(doctor): `fix_renumber` pre-acquires per-file locks on every TargetPath BEFORE reindex AND on every NewPath after reindex — closes the race window where a concurrent process could mutate a moved file.
- fix(doctor): `fix_sweep` now acquires per-file lock for consistency with the other four fix_* functions.
- fix(project): `FindRoot` canonicalizes paths via `filepath.EvalSymlinks` so a malicious parent-symlink cannot redirect downstream writes (audit log, .dark-factory directory).
- fix(audit): `WriteAuditEntry` calls `f.Sync()` before close — guarantees audit-trail durability across crashes.
- fix(doctor): replace non-existent `dark-factory spec verify <id>` reference with `/dark-factory:verify-spec <id>` slash skill in `verifying-stale` fix-line.
- chore(cmd): drop dead `verifyingStaleHours` field from `doctorCommand`; reorder struct after constructor.
- chore(doctor): dedupe `doctor.Deps` in factory; drop duplicate `CurrentDateTimeGetter` from `FixerDeps` (embedded in `Deps`); remove dead `GitRunner` dependency.
- chore(doctor): drop hardcoded relative audit-log path in `cmd/doctor.go`; resolve via `project.FindRoot` so the path is CWD-invariant.
- fix(counterfeiter): drop stray leading space in 4 `//counterfeiter:generate` directives — `go generate` did not recognize the space-prefixed form, silently dropping three mocks.
- test(doctor): coverage additions for `fix_renumber` branches, `verifying_stale` unparseable timestamp, `status-dir-mismatch` happy paths, and `main_internal_test` for the CLI flag helpers.

## v0.174.0

- refactor: Move all os.ReadDir calls to shared helpers in pkg/doctor/parse_errors.go (constraint: only parse_errors.go may call os.ReadDir)
- feat: Add `dark-factory doctor [--fix] [--yes] [--verifying-stale-hours=N]` — detects 6 state-anomaly categories (`duplicate-spec-numbers`, `prompted-but-not-swept`, `verifying-stale`, `orphan-prompt-link`, `orphan-in-progress-prompt`, `status-dir-mismatch`) in `specs/` and `prompts/`, prints a copy-paste fix line per finding.
- feat: `dark-factory doctor --fix` applies safe mutations under a per-file lock with audit log at `.dark-factory/doctor.log`. Renumber fix records `previous_id: NNN` (unquoted YAML) in the renamed spec's frontmatter; linked prompt `spec:` fields are rewritten to the new id, prompt filenames untouched.
- fix: Remove silent startup reconciliation — the daemon no longer renumbers spec or prompt files on startup. Operators run `dark-factory doctor` to find and fix such conditions on demand.

## v0.173.2

- spec-writing rules: add Scope Check size budget (`DB × AC > 50` OR > 3 code layers → consider split) and mandatory `## Suggested Decomposition` section template for multi-layer specs. spec-auditor flags both as Should-Fix; spec-creator emits the decomposition section template and adds a size-check workflow step. Reduces prompt-creator research time on large multi-layer specs (real example: spec 043 zombie detection — 10 DBs × 10 ACs × 5 layers, first generation attempt spent 30 min in research without writing any prompts).
- spec-auditor + spec-creator: clarify that Size Budget and Suggested Decomposition rules are intentional heuristics requiring auditor judgment (no canonical layer enumeration) and document the relationship between the two thresholds (`> 3 layers` → consider split; `> 1 layer` → require decomposition section).
- mocks/mocks.go header: the `make generate` target wipes and rewrites `mocks/` from scratch on each invocation; `mocks/mocks.go` is regenerator output with only `package mocks` by design (no copyright header). Documented here to make the absence intentional rather than accidental — subsequent regenerations will continue to produce a header-less file.

## v0.173.1

- chore: Bump default container image to claude-yolo:v0.9.0 (adds `@ast-grep/cli` so projects using the default image have ast-grep available for the doc-driven code-review pipeline's mechanical-rules step)

## v0.173.0

- release plugin v0.173.0: bundle commands/, agents/, docs/, skills/ updates accumulated through binary v0.172.0 (specs 088 + 089 + 090: `autoGeneratePrompts` rename + default flip, `dark-factory spec mark-prompted` CLI command, manual generate-prompts-for-spec wiring, spec-writing/prompt-writing/scenario-writing guide refinements)
- fix scenarios 010 + 012: switch `preflightCommand` from inline `sh -c '...'` to script-file path (validator rejects shell metacharacters)
- fix scenario 019: add `autoGeneratePrompts: true` to setup YAML (spec 089 flipped default to OFF)

## v0.172.0

- feat: Add `dark-factory spec mark-prompted <id>` CLI command for manual prompt-generation lifecycle completion

## v0.171.18

- BREAKING: Renamed `disableAutoGeneratePrompts` to `autoGeneratePrompts` everywhere in config, CLI, factory, watcher, and docs. Polarity inverted: `autoGeneratePrompts: true` enables auto-generation, `false` (or unset) disables it. Default flipped from auto-gen ON to auto-gen OFF. Operators with `disableAutoGeneratePrompts` in `~/.dark-factory/config.yaml` or `.dark-factory.yaml` must rewrite to `autoGeneratePrompts: true` to preserve pre-rename behavior.

## v0.171.17

- refactor: Reorder files to follow Interface → Struct → Constructor pattern in pkg/generator, pkg/queuescanner, pkg/subproc

## v0.171.16

- refactor: Extract switch dispatch from CreateWorkflowExecutor into WorkflowExecutorProvider interface
- refactor: Extract notifier creation into pure sub-helpers CreateTelegramNotifier and CreateDiscordNotifier
- refactor: createProviderDeps no longer contains if/else, pure helpers CreateGitHubProviderDeps and CreateBitbucketServerProviderDeps now available

## v0.171.15

- fix: Replace fmt.Errorf with errors.Errorf in pkg/prompt/prompt.go and pkg/spec/spec.go
- fix: Replace bare return err with errors.Wrap in multiple pkg/ files
- fix: Add ctx parameter to CanTransitionTo methods in pkg/prompt/prompt.go and pkg/spec/spec.go
- fix: Change errors.Wrapf to errors.Wrap in pkg/runner/worktree.go

## v0.171.14

- docs: Add GoDoc comments to 38 exported items in pkg/factory/factory.go
- docs: Add GoDoc comments to exported items in pkg/config/workflow.go, pkg/processor/processor.go, pkg/processor/workflow_helpers.go, pkg/formatter/message.go, pkg/server/queue_action_handler.go, and pkg/server/inbox_handler.go

## v0.171.13

- fix: Use caller's context in slog.Default().Enabled() call in runner.go instead of context.Background()

## v0.171.12

- fix: Add exit checks to cd commands in scenarios/helper/lib.sh to prevent silent failure on directory change errors

## v0.171.11

- fix: Inject time dependencies in pkg/preflight/ and pkg/formatter/ using libtime pattern

## v0.171.10

- docs: Document HTTP server timeout defaults as sufficient for dark-factory threat model

## v0.171.9

- refactor: Extract sequential concerns in dockerSpecGenerator.Generate() into named helpers

## v0.171.8

- fix: Add repo name validation before constructing gh api URL in collaborator_fetcher.go to prevent path traversal

## v0.171.7

- fix: Add shell metacharacter validation for preflightCommand in config validation to prevent command injection
- fix: Redact token configuration in warning messages to avoid leaking whether a token is configured

## v0.171.6

- refactor: Split oversized Manager in pkg/prompt/prompt.go into focused types: PromptStatusManager (status mutations), PromptScanner (directory queries), PromptMover (file operations), PromptFileLoader (file I/O); extract PrepareRollback/RollbackMove from RollbackMoveToCompleted

## v0.171.5

- test: Add standard Ginkgo suite setup and go:generate directive to specsweeper test suite

## v0.171.4

- refactor: replace manual test fakes with Counterfeiter-generated mocks in pkg/preflightconditions/conditions_test.go and pkg/containerslot/manager_test.go

## v0.171.3

- fix: use sync/atomic.Bool for cancelledByUser in processor.go to fix data race between goroutines

## v0.171.2

- docs: reorganize `docs/spec-writing.md`, `docs/prompt-writing.md`, `docs/scenario-writing.md` under `docs/rules/` to separate agent-loadable writing rules from operator/architecture/reference docs; update all live cross-references (README.md, agents/, commands/, docs/) — historical CHANGELOG/specs-completed/prompts-completed entries intentionally left at original paths
- docs: add "Two ways to generate prompts from an approved spec" section to `docs/running.md` with auto-vs-manual comparison table, when-to-pick guidance, cost/benefit, and switching examples; cross-link from README.md, docs/configuration.md "Disable Auto Prompt Generation", and `commands/generate-prompts-for-spec.md`
- fix(commands/daemon.md): clarify the dead-PID branch — agent must NOT `rm .dark-factory.lock` when the previous daemon's PID is dead. The lock is flock-based; the new daemon acquires it automatically on start.

## v0.171.1

- docs: document `disableAutoGeneratePrompts` in README.md "User-level defaults" paragraph, `--set` table in docs/configuration.md, and CLI help text in main.go

## v0.171.0

- feat: gate spec watcher auto-generation behind `disableAutoGeneratePrompts` config flag — when enabled, watcher logs INFO skip message instead of calling generator

## v0.170.0

- feat: add `disableAutoGeneratePrompts` config field threaded through all config layers (default, global, project, CLI --set) for gating spec watcher auto-generation

## v0.169.0

- test: add ginkgo coverage for the clone/worktree sync helper (`syncPromptFileToOriginalRepo`) — idempotent destination-exists path, move-on-source-present path, and the `clone-sync-mismatch` error when both source and destination are absent (spec 087)
- test: integration coverage for clone + worktree end-to-end producing a single combined commit on the remote with the prompt at `prompts/completed/<id>.md` and no local commits ahead of `origin/master`
- agents: YAGNI pass added to spec-creator + prompt-auditor — guidance to keep specs/prompts scoped to actual requirements, no speculative extras
- agents: prompt-creator + spec-auditor updated alongside the YAGNI guidance
- docs(workflows): post-push mirror section added for clone + worktree (spec 087); protected-master callout warns against `autoRelease: true` and points to the separate GitHub Release Agent pipeline; protected-master spec/prompt admin gap documented inline
- docs(troubleshooting): added preflight-baseline-failure section with `updater all` resolution path and the `go get -u` anti-pattern explanation
- docs(releasing-dark-factory): tightened the release-gate rule — every release walks every active scenario, no in-session shortcuts; the only valid skip is byte-equivalent binary verified by `git diff $INSTALLED..HEAD`
- specs/ideas: `protected-master-admin-bundle.md` describes the gap where spec/prompt admin work doesn't land in PRs on protected-master projects (no solution chosen)
- specs/ideas: `spec-lifecycle-transition-from-all-workflows.md` describes the gap where `prompted → verifying` only fires from direct + branch executors (not clone, worktree, or `prompt complete`)
- plugin: version bumped from 0.168.0 → 0.169.0 to align with binary CHANGELOG after agents/ + docs/ changes

## v0.168.4

- fix: clone and worktree workflows now mirror the in-progress → completed rename into the original repo after push, so the daemon's local view matches `origin/master` and `savePRURLToFrontmatter` no longer errors (spec 087, follow-up to spec 086)

## v0.168.3

- update golang.org/x/crypto v0.51.0 → v0.52.0
- add integration tests for all four workflow executor modes verifying move-before-commit ordering (spec 086)
- add make fix target for bulk dependency updates

## v0.168.2

- fix: prompt move from `in-progress/` to `completed/` is now part of the same commit as the code change, so master no longer diverges from the local daemon view after a PR merge (spec 086, addresses BRO-20203 lib-crypto repro)

## v0.168.1

- bump github.com/bborbe/run v1.9.26 → v1.9.27
- bump golang.org/x/net v0.54.0 → v0.55.0
- bump golang.org/x/sys v0.44.0 → v0.45.0

## v0.168.0

- Add sibling-coverage rule to prompt-auditor agent — catches the AC6-class bug where a prompt edits one entry point's setup logic (e.g. `runner.Run`) but misses parallel implementations (e.g. `oneShotRunner.Run`); flags via four detection heuristics (same-package method parity, entry-point name pairs, spec multi-subcommand signals, helper-extraction asymmetry)
- Refactor vulncheck Makefile target with VULNCHECK_IGNORE variable and jq-based filtering for clearer ignore list management
- Add license header to mocks/mocks.go
- Complete spec 084 (fail-fast on worktree without hideGit) — moved to specs/completed/
- Complete spec 085 (auto-inject hideGit guidance) — moved to specs/completed/
- Add scenario 021: daemon refuses to start from worktree CWD without hideGit (covers AC1, AC5, AC7)
- Add scenario 022: dark-factory run refuses to start from worktree CWD without hideGit (covers AC6 regression guard)
- Bump plugin version 0.164.0 → 0.168.0 (sync with binary stream — prompt-auditor agent change)

## v0.167.1

- Update github.com/bborbe/run v1.9.24 → v1.9.26
- Add tools.env with canonical tool version pins
- Update Makefile to use versioned tool invocations via tools.env

## v0.167.0

- refactor: Extract `checkGitSafety` from `(*runner)` method to package-level `CheckGitSafety(ctx, hideGit)` function shared by daemon and one-shot runners
- feat: Add `hideGit` field to `oneShotRunner` and call `CheckGitSafety` gate in `oneShotRunner.Run` after lock acquisition — `dark-factory run` now refuses to start from a worktree/submodule CWD without `hideGit=true`, matching daemon behavior (spec 084 AC6)
- test: Add worktree gating integration tests for one-shot run path: worktree+hideGit=false refuses, worktree+hideGit=true passes, submodule+hideGit=false refuses, regular repo always passes

## v0.166.1

- test: Add stat-error detection test case to `DetectWorktreeOrSubmodule` exercising the EACCES path when `.git` is inaccessible
- test: Replace source-text count assertion in factory tests with behavioral `DescribeTable` testing `resolveSpecGeneratorHideGit` across four input shapes

## v0.166.0

- feat: Wire `workflow == config.WorkflowWorktree || hideGit` to `promptenricher.NewEnricher` in `factory.CreateProcessor`, matching the expression passed to `executor.NewDockerExecutor` — the enricher guidance fragment now appears in emitted prompts when `hideGit=true`
- test: Add factory integration tests verifying the enricher and executor receive identical `hideGit` expressions, and that the guidance fragment is emitted when `hideGit=true` and suppressed when `hideGit=false`
- docs: Document `hideGit` guidance fragment behavior in `docs/troubleshooting.md`

## v0.165.0

- feat: Add `hideGit` parameter to `promptenricher.NewEnricher` — when `true`, `Enrich` prepends a guidance fragment after `additionalInstructions` explaining that `/workspace/.git` appears as a character device by design, `GOFLAGS=-buildvcs=false` is typically set, and to run `make precommit` regardless of `.git`'s appearance

## v0.164.3

- fix: Fail-fast worktree/submodule detection — dark-factory now refuses to start from a worktree or submodule CWD (where `.git` is a regular file) without `hideGit=true`, before any container is launched. Error message names the condition, remediation (`hideGit=true`), and references `docs/troubleshooting.md` and the `PR via Pre-Created Worktree` runbook.

## v0.164.2

- fix: SpecGenerator now uses `cfg.Workflow == config.WorkflowWorktree || cfg.HideGit` for `hideGit` parameter, matching prompt executor behavior

## v0.164.1

- docs: Document worktree/submodule + `hideGit` failure mode in `troubleshooting.md`

## v0.164.0

- plugin: sync — spec-writing / prompt-writing / configuration / config-layering / releasing-dark-factory / init-project / yolo-container-setup doc refinements; prompt-auditor agent tweak

## v0.163.5

- fix: `getNextVersion` now bumps from `max(highest_tag, highest_changelog)` to prevent semver regression when a CHANGELOG `## vX.Y.Z` heading is written above the highest git tag; emits `slog.Warn` when orphan version detected

## v0.163.4

- chore: Bump default container image to claude-yolo:v0.8.1 (ANTHROPIC_MODEL-aware model resolution for alt-provider routing + one-shot prompt-file permission fix)

## v0.163.3

- fix: `validateClaudeAuth` no longer blocks dark-factory launch when the merged container env provides alt-provider auth (`ANTHROPIC_BASE_URL` + `ANTHROPIC_AUTH_TOKEN`). Required for routing to MiniMax and other Anthropic-compatible providers without an OAuth token on disk.

## v0.163.2

- chore: Bump default container image to claude-yolo:v0.7.0 (adds api.minimax.io to tinyproxy egress allowlist for MiniMax Anthropic-compatible API)

## v0.163.1

- docs: Document global `env` layering in `config-layering.md` — move `env` from project-only to Category A, add key-level merge semantics, secrets exception, and update Out-of-scope section; add `### Global env` subsection to `configuration.md` with example yaml, key-name rules, secrets guidance, permission warning, and effective-config log description

## v0.163.0

- feat: Support global env vars in `~/.dark-factory/config.yaml`; project env overrides per-key (key-level merge, project wins). Env keys must match `^[A-Z_][A-Z0-9_]*$`. Effective-config log line reports env keys by source layer (`envFromGlobal`, `envProjectOverrides`, `envProjectOnly`). Home config file emits warning when group/world readable.

## v0.162.0

- feat: `spec-verifier` agent gains Phase 0.5 — target-deploy freshness gate that runs before the AC walk when the spec declares Post-Deploy ACs. Refuses verification upfront if any environment is pre-fix, naming the captured-vs-required tokens and the exact deploy command. Catches stale-deploy verification cheaply rather than reactively during Phase 4 anti-evidence.
- feat: `spec-writing.md` documents the `**Post-Deploy (Rung-N):**` AC marker and the `deploy_check:` / `deploy_target:` evidence shapes the verifier consumes.
- feat: `spec-auditor` agent gains a Post-Deploy marker check — ACs whose body queries a deployed system (`kubectlquant`, `kubectl -n`, `make buca` evidence, `--version` against a deployed binary) without the marker + evidence lines are flagged as Critical. Specs already in `specs/in-progress/` and `specs/completed/` are grandfathered.
- docs: `docs/releasing-dark-factory.md` — scenario-status walk rule (active only, skip drafts/ideas), runner-helper map, gate cost/time expectations, Docker/gh preflight, daemon coexistence note, troubleshooting table, plugin `make check-versions` as gate; clarify `make install` vs `go install @latest` (local source vs proxy); per-surface gate cadence rule replacing the impractical "before every prompt approval"; explicit plugin non-surface list (`scenarios/`, `prompts/`, `specs/`, `pkg/`, `main.go`).

## v0.161.1

- fix: git wrappers in pkg/git/ now capture stderr and include it verbatim in errors, so dirty-tree, auth, and network failures are diagnosable from the daemon log without manual worktree reproduction

## v0.161.0

- feat: Add `check-changelog` Makefile target and `scripts/check-changelog.sh` that lints CHANGELOG.md for stranded SemVer preamble (rules: correct title, preamble before first `##`, preamble appears exactly once, MAJOR bullet appears exactly once); wired into `precommit`
- test: Add `processUnreleasedSection` fixture with full real-world SemVer preamble asserting header is preserved byte-for-byte on Unreleased rename

## v0.160.1

- fix: Restore CHANGELOG.md preamble (SemVer link + MAJOR/MINOR/PATCH bullets) at top after a past `## Unreleased` insertion above the preamble stranded it between `## v0.51.8` and `## v0.51.7`

## v0.160.0

- feat: Container names now follow `<project>-gen-<spec>` and `<project>-exec-<prompt>` schema (was `dark-factory-gen-<spec>` and `<project>-<prompt>`). The project name defaults to the git root directory basename. External tooling that greps for `dark-factory-gen-` must be updated to grep for `-gen-` or `<project>-gen-`. The optional `project:` field in `.dark-factory.yaml` overrides the project name.

## v0.159.0

- feat: Add optional `project:` field to `.dark-factory.yaml` config that overrides the container name prefix for spec-generation containers, with validation that rejects empty/whitespace-only values
- feat: Rename spec-generation containers from `dark-factory-gen-<spec>` to `<project>-gen-<spec>` using `ContainerName.Sanitize()`; startup recovery probes both new and legacy container names for zero-downtime upgrade

## v0.158.0

**Prompt writing detail-level spectrum + softer auditor.** Reframes "specificity over brevity" — over-specifying inlined Go ships the prompt author's bugs and prevents the agent from applying project conventions. New 5-level grain spectrum + writer-side discipline + mechanical auditor checks.

### `docs/prompt-writing.md` (canonical guide)

- New section: **Detail Levels** — 5-level spectrum from Very Detailed (full inlined function bodies) to Very Rough (one-paragraph intent), with pros, cons, and concrete when-to-use guidance per level
- New section: **Choosing a level** — discovery-step-first decision tree with concrete `rg` commands; hard-split question 1 prevents silent fall-through to Level 3 on greenfield projects without exemplars
- New section: **What the spectrum does NOT solve** — honest scope: pattern-anchoring fixes convention-drift bugs, not logic bugs in genuinely novel code (`io.LimitReader` before `json.Unmarshal`, off-by-one, swapped args); those need spec-level rigor or adversarial review
- New worked example: same surface (bot-identity self-check) shown at Level 1, Level 3, Level 5 — concrete comparison of how Level 3 prevents the `fmt.Errorf` vs `errors.Wrapf` convention-drift bug
- Reframed **Specificity over brevity** to distinguish contracts/anchors (good specificity) from pre-deciding every line (bad specificity that ships author bugs)

### `agents/prompt-auditor.md`

- New **Recommendation** checks under Documentation Placement:
  - **Pattern collision** (priority signal, not line count) — 5 mechanical checks the auditor executes via Bash: error wrapping (`fmt.Errorf` inlined while project uses `errors.Wrapf`), HTTP client style, test framework, mock pattern, context propagation. Explicit instruction: "You (the auditor) execute these searches via Bash."
  - **Volume × collision** — backup signal for >200 lines of inlined Go when a matching project pattern exists
  - **Other inlining smells** — pre-decided `if/else` chains when spec failure-modes already enumerate; per-`It` test scenarios when `DescribeTable` suffices
  - **Author-logic bug risk** — mechanical: classification differs from spec failure-modes table, retry policy diverges from spec, no matching project import, state-machine transitions not in any existing state-machine file

### `commands/create-prompt.md` (writer-side symmetry)

- Mandatory pre-write step: `rg`-based pattern discovery for every surface the prompt will touch
- 2x2 decision matrix (patterns exist × structure novel) maps each cell to a detail level; closes the "Level 3 because default" trap
- Explicit guidance that the prompt's `<context>` MUST list the exemplar files; auditor's "Pattern collision" check fails otherwise

## v0.157.0

**Symmetric update to the spec writer + auditor + spec-writing guide.** Closes the audit↔verify gap by adding three new dimensions; specs now declare evidence up front so verification is mechanical, and the writer self-checks against the audit rules before reporting.

### `docs/spec-writing.md` (canonical guide)

- New section: **Evidence Shape per Acceptance Criterion** — table of acceptable shapes (exit code, log line, file diff, HTTP status, kafka message, metric, cluster state, file artifact); good/bad AC examples
- New section: **Adversarial Laziness Test** — read the spec assuming the laziest implementation; if it's a no-op or hardcoded fake, the ACs are under-specified; fix pattern is to replace artifact-existence with behavior
- New section: **Hedge Words to Avoid** — flagged words list and resolution rules (resolve to concrete rule OR mark "agent decides at impl time")
- New section: **Failure Modes — Optional Columns for Non-Trivial Specs** — Detection, Reversibility, Concurrency; categories to cover for specs with real-world side effects (external unavailability, schema drift, partial-progress crash, rate limiting, resource exhaustion, clock skew)

### `agents/spec-creator.md`

- Constraints expanded: every AC must declare evidence shape; hedge words avoided
- Workflow step 5 expanded: three self-checks before reporting (adversarial laziness, hedge-word grep, evidence-shape check)
- Template AC section: prose-level evidence-shape guidance with good/bad examples
- Template Failure Modes section: optional columns + category coverage prompts
- Report output: now includes self-check pass/fail per dimension

### `agents/spec-auditor.md`

- Three new audit dimensions matching the writer-side self-checks:
  - **Evidence shape per AC** — flag each AC without a declared evidence shape. Evidence-shape table includes positive (exit code / log line / HTTP / kafka / metric), state transition (delta with before/after framing), and negative evidence (grep returns 0 / git diff empty / no kafka publish during window).
  - **Adversarial laziness pass** — "what's the laziest impl that passes every AC?" Report MUST include a concrete code-shaped one-liner naming the laziest implementation gesture (not vibes).
  - **Hedge-word audit** — grep for hedges, distinguish deferral from descriptive English (don't flag "daemon should be running" / "the relevant config file"); each flagged hedge resolves to a concrete rule or is marked "agent decides at impl time" with concrete acceptable/non-acceptable exemplars.
- Failure Modes guidance: optional columns (Detection, Reversibility, Concurrency); failure-mode categories to check; **Recovery rows follow the same evidence-shape vocabulary as ACs** (so the verifier can confirm the recovery path was exercised, not just that the failure was reached).
- Filename-Content Alignment: extended to cover Acceptance Criteria, not just Summary + Goal
- Report format: three new top-level sections before Spec-vs-Prompt Fitness
- Scoring rubric: three new adjustments (laziness FAIL: -2, hedges >3: -1, no evidence shapes: -1) + explicit floor of 1 (so a spec failing every adjustment still scores 1, not negative)
- Output format polish: Score line shows `(minimum 1)`; "Flagged words" list renamed "Words to scrutinise for deferrals (flag only when they defer a decision)" to pre-empt grep-first mechanical flagging
- Template: spec-creator AC block pre-fills one example (`make precommit` exits 0 — exit-code evidence) so writers see the evidence-shape pattern inline; BigQuery example added to docs/spec-writing.md State-transition row for parity with the auditor

## v0.156.3

- bump bborbe/{http,time,kv,math,parse,sentry} deps
- bump go-git/go-git v5.18→v5.19, go-billy v5.8→v5.9
- bump golang.org/x/{crypto,net,sys,term,text,exp}
- bump osv-scanner v2.3.7→v2.3.8, errcheck v1.10→v1.20
- docs: add import-before-tidy and sequential spec verification rules

## v0.156.2

- chore: extract `check-versions` to `scripts/check-versions.sh` (4-field locked check: CHANGELOG top + `plugin.json` + `marketplace.json` `metadata.version` + `plugins[0].version`); add `make release-check` (`precommit + check-versions`); unwire from `precommit` so binary↔plugin drift is allowed during development and only enforced at install time. Aligns with `vault-cli` / `semantic-search` release-gate shape.
- chore: catch-up bump plugin manifests `0.154.0` → `0.156.2` to re-align with binary CHANGELOG (no plugin surface changes since `0.154.0`).
- test: scenarios 019 (active) and 020 (idea) for spec 078 auto-approve-generated-prompts; clarified `dark-factory-sandbox` is the dedicated test repo for dark-factory's own scenarios
- test: fix flaky `pkg/committingrecoverer` suite — autoRelease matrix test now restores cwd to the original directory (not to the temp dir it later removes), preventing `os.Getwd: no such file or directory` cascades in subsequent specs under random ordering

## v0.156.1

- fix: ParseFromLog selects last complete marker pair in tail window, preventing orphaned end-marker boundary artifact from silently swallowing agent-reported failures

## v0.156.0

- feat: When `autoApprovePrompts: true`, daemon audits and auto-approves each generated prompt via the existing YOLO executor; audit failure stops further auto-approvals for the spec without changing spec status

## v0.155.0

- feat: Add `autoApprovePrompts` boolean setting resolvable from global config, project config, and `--auto-approve-prompts` CLI flag; effective value and source logged at daemon startup

## v0.154.0

- chore: bump claude-yolo v0.6.2 → v0.6.3
- chore: bump Go 1.26.2 → 1.26.3

## v0.153.0

- docs: Split release docs — new `docs/releasing-dark-factory.md` for releasing dark-factory itself (binary + plugin); `docs/release-process.md` clarified to cover autoRelease behavior in consuming projects only. CLAUDE.md `make install` rule now points at the new doc.

## v0.152.0

- feat: `/dark-factory:configure` slash command for create/reconfigure/auto-migrate of `.dark-factory.yaml` (greenfield delegates to `init-project`; valid existing config offers reconfigure menu; invalid config detects spec-073-style legacy fields and proposes migration; backup + diff + validate + revert on every write)
- docs: `docs/configuration.md` documents `idleLogInterval` field added in v0.150.5

## v0.151.2

- fix: Cancelled executing prompts now classified as `cancelled` (not `failed`) — fixed race between cancellationwatcher's `close(ch)` and `StopAndRemoveContainer`, plus added deterministic fallback in processor to re-read file after `Execute` returns

## v0.151.1

- fix: `prompt cancel` now moves approved prompts to `prompts/cancelled/` immediately, preventing daemon re-spawn
- fix: processor moves executing prompts to `prompts/cancelled/` after container stops on cancel
- fix: `prompt cancel` is idempotent — cancelling an already-cancelled prompt returns exit 0
- fix: `NewCancelCommand` accepts `cancelledDir` parameter; factory wires `cfg.Prompts.CancelledDir`

## v0.151.0

- feat: Add `cancelledDir` config field (`prompts/cancelled/` default) and `MoveToCancelled` method on `prompt.Manager` to move cancelled prompt files with a UTC timestamp; add `cancelled` frontmatter field; `listQueued` skips `cancelled` status files; `cmd.PromptManager` and `processor.PromptManager` interfaces extended accordingly

## v0.150.5

- fix: Suppress noisy `"nothing to do, waiting for changes"` idle log — emits once per idle entry, then at most once per `idleLogInterval` (default 1m) heartbeat; configurable via `idleLogInterval:` in `.dark-factory.yaml`

## v0.150.4

- BREAKING: removed `autoReview`, `allowedReviewers`, `useCollaborators`, `maxReviewRetries`, `pollIntervalSec` config fields. Use GitHub branch protection to gate merges. Configs containing any of the three user-visible removed fields (`autoReview`, `allowedReviewers`, `useCollaborators`) now fail at load time with a friendly error.

## v0.150.3

- refactor: Remove entire `autoReview` code path — deleted `pkg/review/`, `ReviewPoller`, `ReviewFetcher`, `FixPromptGenerator`, `review_fetcher.go`, related mocks, and all wiring; `autoMerge: true` with branch protection is now the only merge gate

## v0.150.2

- fix: autoReview routing in `handleAfterIsolatedCommit` — check `AutoReview` before `AutoMerge` so `autoReview: true` prompts transition to `in_review` instead of being routed to `WaitAndMerge` directly
- fix: `WaitAndMerge` field mismatch — switch on correct `mergeStateStatus` values (`CLEAN` → merge, `DIRTY` → fail) instead of wrong `mergeable` enum values (`MERGEABLE`/`CONFLICTING`)

## v0.150.1

- fix: `autoReview` approval now runs `postMergeActions` after merge — master is pulled locally, `## Unreleased` is promoted to `## vX.Y.Z`, tag is created and pushed (matches `autoMerge`-only path)

## v0.150.0

- feat: Run preflight at daemon startup before the watcher loop; daemon exits non-zero immediately when baseline is broken at start

## v0.149.5

- improvement: `commands/generate-prompts-for-spec.md` now references `~/.claude/plugins/marketplaces/coding/docs/test-pyramid-triggers.md` for the per-prompt test-type decision rules instead of inlining them. Each generated prompt also pulls the triggers doc into its `<context>` so the implementing agent applies the same pyramid (default unit, integration per real boundary, E2E only when scenario rule mandates).

## v0.149.4

- policy: scenarios are now treated as rare E2E tests at the top of the test pyramid — default is NO new scenario per spec. Updated `docs/scenario-writing.md` (test-pyramid framework + four-condition trigger), `docs/spec-writing.md` (preflight + test-layer table soften), `agents/spec-creator.md` (template default), `commands/refine-spec.md` (scenario_trigger_check tightened), and `commands/generate-prompts-for-spec.md` (do not generate speculative scenario prompts; when one IS required, inline as a step in the implementation prompt — never split into a separate prompt).

## v0.149.0

- feat: Enrich PR body with prompt summary, spec reference, and issue reference

## v0.149.3

- improvement: `spec-verifier` Phase 7 now appends a `## Verification Result` block to the spec before moving it to `specs/completed/`. Captures timestamp, HEAD sha, binary path, scenario, and concrete evidence — so future readers can answer "what proved this spec passed?" from the spec file alone, without grepping conversation history. Specs remain append-only (existing content immutable).

## v0.149.2

- improvement: `spec-verifier` agent now adds Phase 0 binary-freshness check — when verifying a dark-factory spec, builds `/tmp/dark-factory-<sha>` from HEAD if the installed binary lags, and uses that path through Phase 7 `spec complete`. Prevents stale-binary verifications that would run old code and produce evidence saying nothing about the fix.

## v0.148.5

- fix: `clone` workflow (`workflow: clone`, legacy `workflow: pr`) now completes end-to-end — feature branch pushed from inside the clone before removal, parent repo fetches the branch ref before `CommitsAhead`, eliminating `exit 128` crash at post-commit step

## v0.149.1

- refactor: `/dark-factory:refine-spec` is now a single slash command (no agent) — preserves conversation context and removes the redundant restart-question UX that fired even when the human/Claude session already had the single-sentence anchor

## v0.149.0

- feat: add `/dark-factory:refine-spec` command + `spec-refiner` agent — interactive spec narrowing between `create-spec` (capture) and `audit-spec` (structure check); forces single-sentence scope, splits adjacent concerns into `specs/ideas/` stubs, transitions status idea→draft

## v0.148.4

- fix: `branch` workflow retry no longer crashes at `git checkout` when the feature branch has divergent content for prompt-file paths (discards dark-factory's own bookkeeping dirt before the branch switch)

## v0.148.3

- fix: all workflows (direct, branch, worktree, clone) handle "agent reports success but produces no diff" gracefully — no more `git commit: exit status 1` crash; prompt moves to completed/ as expected

## v0.148.2

- fix: branch workflow no longer rejects its own prompt-file frontmatter writes as working-tree dirt; IsCleanIgnoring filters dark-factory state directories before branch checkout

## v0.148.1

- fix: pass explicit `branch string` parameter to `PRCreator.Create` and supply `--head <branch>` to `gh pr create`, preventing "head branch is the same as base branch" error when cwd has been reset to master worktree in isolated workflows

## v0.148.0

- docs: rework "Choosing a Flow" framing across `docs/architecture-flow.md`, `docs/claude-md-guide.md`, `commands/read-guides.md`, and `CLAUDE.md`. Headline reason for using prompts/specs is now **safe unattended execution** (YOLO container, permission checks disabled) — documentation, decomposition, and Sonnet/Opus token savings are framed as side benefits. Decision matrix changed from 2-row size-based ("simple fix" vs "multi-prompt feature") to 3-row artifact-based: **Direct** (doc/config/yaml — no code), **Prompt** (any code change), **Spec → prompts** (feature delivering business value). The prompt/spec split is now explicitly framed as **business-why vs technical-how**, not big vs small. `commands/read-guides.md` Step 4 emits the decision table verbatim in every read-guides invocation so users see it before they pick a flow.

## v0.147.2

- fix: resolve symlinks in `pkg/project/root_test.go` before comparing `FindRoot` results. On macOS, `os.MkdirTemp` returns paths under `/var/folders/...` but `os.Getwd()` after `os.Chdir` resolves the symlink to `/private/var/folders/...`. Tests now wrap `projectDir` with `filepath.EvalSymlinks` for stable equality checks. Caught during spec 064 verification — `make test` was failing on macOS even though production code (which uses `os.Getwd()` directly) was correct.

## v0.147.1

- docs: add accepted `<id>` format note to `spec` and `prompt` help text (padded/unpadded number, basename, .md extension)

## v0.147.0

- feat: walk up directory tree from cwd to find .dark-factory.yaml project root, replacing git root detection
- fix: spec list / spec status / prompt list now error with "not a dark-factory project" when no .dark-factory.yaml found, instead of silently returning empty results

## v0.146.0

- feat: CLI spec and prompt <id> arguments now resolve by integer value, accepting unpadded numbers ("63" matches "063-foo.md") and full basenames with or without .md extension; ambiguous numeric matches across directories return a descriptive error

## v0.145.5

- docs: align `prompt-writing.md` and `agents/prompt-auditor.md` with the actual canonical `spec:` frontmatter form. Daemon-generated prompts use full-slug entries (`spec: ["030-foo-bar-baz"]`) and `pkg/slugmigrator` rewrites bare numbers to the full form after each generation cycle. Both docs and the auditor rule now show the long form as canonical and explicitly accept bare numbers as input. Eliminates a recurring audit false-positive that flagged daemon-generated prompts as malformed.

## v0.145.4

- docs: extend `docs/workflows.md` Invalid section to document `pr: true + autoMerge: false + autoRelease: true` rejection with three actionable resolutions
- docs: add scenarios 014, 015, 016 for spec 063 — config-validation rejection, branch+PR+autoMerge+autoRelease happy path, and direct+autoRelease regression

## v0.145.3

- fix: branch workflow always creates feature branch from baseName even when prompt frontmatter has no branch field

## v0.145.2

- fix: reject pr: true + autoMerge: false + autoRelease: true at config load with actionable error naming all three valid resolutions

## v0.145.1

- docs: cross-reference `bug-workflow.md` from `spec-writing.md` (When to Write a Spec table + Next Steps) and `documentation.md` (Spec section).
- specs: add `specs/ideas/bug-autorelease-overrides-pr-workflow.md` — bug report (filename-prefix convention, `kind: bug`) documenting that `autoRelease: true` bypasses both branch creation (`workflow: branch`) and PR creation (`pr: true`) when `autoMerge: false`, committing direct to master with a release tag.

## v0.145.0

- feat: --set now accepts workflow, pr, autoMerge keys for per-invocation delivery override
- docs: add `docs/bug-workflow.md` — guide for filing, triaging, fixing, and verifying bugs as specs with `kind: bug` frontmatter. Covers reproduction-section requirements, lifecycle mapping (idea → completed), bug-specific verification (must replay reproduction, not just run tests), and anti-patterns (no `specs/bugs/` folder, no `status: bug`, no fix prompts without a spec). [Note: this file was bundled into v0.145.0 by the autoRelease bug being documented in `specs/ideas/bug-autorelease-overrides-pr-workflow.md` — it should have been a separate release.]

## v0.144.1

- docs: cross-reference `/dark-factory:verify-spec` from README.md, CLAUDE.md, docs/spec-writing.md, docs/spec-verification.md, docs/claude-md-guide.md so the new verification command is discoverable from every entry point

## v0.144.0

- feat: add `/dark-factory:verify-spec` command + `spec-verifier` agent — interactive end-to-end spec verification gate that refuses completion on inspection-only "evidence" (logs, unit tests, old operational evidence, wire-level probes) and only calls `dark-factory spec complete` after the scenario passes against fresh evidence from the deployed binary

## v0.143.0

- feat: add --set key=value CLI flag to run and daemon for per-invocation config override (supported keys: hideGit, autoRelease, dirtyFileThreshold, model, maxContainers)
- BREAKING: remove --hide-git and --no-hide-git flags; use --set hideGit=true / --set hideGit=false instead. Replace --hide-git with --set hideGit=true; replace --no-hide-git with --set hideGit=false.

## v0.142.0

- docs: README mentions global config + new CLI flags; link to docs/configuration.md
- docs: add "Common Patterns" section to docs/configuration.md — covers running on an existing manual worktree (global `hideGit: true` + `autoRelease: false`, project workflow stays direct) and per-machine model preference

## v0.141.1

- docs: document global config layering in `docs/configuration.md` — new "Global Config" section with precedence chain, layered fields table, validation notes, and source tracing; extend "CLI Flags" section with `--hide-git`/`--no-hide-git` and `--model NAME` documentation
- docs: add `scenarios/013-config-layering.md` — manual verification checklist for global→project→CLI precedence, invalid config rejection, contradictory flags, and missing global file fallback

## v0.141.0

- feat: add --hide-git and --no-hide-git CLI flags to run and daemon commands to override hideGit setting per invocation
- feat: add --model NAME CLI flag to run and daemon commands to override model per invocation
- fix: passing both --hide-git and --no-hide-git in one invocation exits non-zero with usage error

## v0.140.0

- feat: extend global config (~/.dark-factory/config.yaml) with `hideGit`, `autoRelease`, `dirtyFileThreshold`, `model` fields; implement default←global←project merge precedence for these 4 fields
- feat: effective config log line now shows per-field source annotations (`modelSource=global`, `hideGitSource=project`, etc.) for the 4 new layered fields
- fix: tighten model validation — explicit `model: ""` rejected at every config layer; model values with shell metacharacters rejected via regex `^[a-zA-Z0-9._:/-]{1,256}$` (BREAKING: rare — no known valid model names contain these chars)

## v0.139.0

- docs: clarify `autoRelease` vs `CHANGELOG.md` semantics across `configuration.md`, `workflows.md`, `running.md`, `architecture-flow.md`, and `README.md` — push is gated on `autoRelease`, tag is gated on `CHANGELOG.md` presence (orthogonal concerns)
- docs: add `release-process.md` documenting the binary auto-release path (with mandatory pre-release scenario gate) and the manual plugin release procedure

## v0.138.1

- fix: autoRelease now pushes the branch on every prompt completion, not only on the release path. Previously, the post-release "move prompt to completed" commit and the no-CHANGELOG work commit stayed local.

## v0.138.0

- feat: add `--skip-preflight` CLI flag to `run` and `daemon` commands to bypass preflight baseline check for a single invocation
- fix: pin `osv-scanner` to `@v2.3.1` in `Makefile` — newer versions (v2.3.2+) are broken upstream due to a `github.com/bazelbuild/buildtools/build` package resolution bug in osv-scalibr's transitive deps. Switching from `go run -mod=mod ...` to `go run pkg@v2.3.1` bypasses the project's go.mod and uses a fresh temp module with the last working version.

## v0.137.0

- feat: preflight baseline failure is now terminal — dark-factory exits non-zero instead of waiting for the next tick; rename `ErrPreflightSkip` → `ErrPreflightFailed`; propagate error through scanner, processor tick methods, and runner; log clear exit message at `slog.Error` level

## v0.136.0

- audit: prompt-auditor now checks `go-time-injection.md` and flags test-only package-level mutable state anti-pattern (`var X = default` + `SetX()` setter pair)
- fix: spec-derived prompts now generated with `status: draft` instead of invalid `status: created` (matches prompt-writing guide inbox-status rule)

## v0.135.19

- refactor: drop noise typed primitives MaxContainers/VerificationGate/DirtyFileThreshold/AutoRetryLimit (no methods, immediately unwrapped); split filename-normalization logic out of prompt.go into pkg/prompt/normalize.go; inject currentDateTimeGetter once at CreateRunner/CreateOneShotRunner entry instead of 21 separate libtime.NewCurrentDateTime() instantiations

## v0.135.18

- refactor: unexport ~20 free functions in `pkg/prompt` (Load, SetStatus, HasExecuting, etc.) and make `prompt.Manager` the sole public mutation API; extend `PromptManager` interfaces in consumer packages (cmd, generator, spec, watcher, reindex, runner) to include `Load`; update all callers and tests

## v0.135.17

- chore: enable revive `file-length-limit` rule (max 2000 total lines, including blank lines and comments) in `.golangci.yml` to prevent file-size regressions; matches the CLAUDE.md rule and `wc -l`-based survey
- refactor: relocate BaseName/ContainerName from pkg/processor to pkg/prompt; consolidate ProjectName as pkg/project.Name; inject queuescanner.Scanner via NewProcessor constructor (eliminates SetScanner two-phase init); delete workflowExecutorResumerAdapter from factory (no longer needed once BaseName lives in pkg/prompt)

## v0.135.16

- refactor: split oversized Go test files (`processor_test.go` 7450→<1915/file, `prompt_test.go` 3067→<1500/file, `config_test.go` 2648→<1000/file) into per-concern files; no behavior changes, all tests pass; eliminates Read-tool token-limit friction and reduces merge-conflict surface

## v0.135.15

- refactor: extract `validationprompt.Resolver` interface into `pkg/validationprompt/` and inject into `promptenricher.NewEnricher`, removing disk I/O from promptenricher

## v0.135.14

- refactor: extracted QueueScanner from processor — final pass; processor.go reduced from 655 to 472 lines

## v0.135.13

- refactor: extracted CommittingRecoverer from processor — pure refactor, no behaviour change

## v0.135.12

- refactor: extracted PromptResumer from processor — pure refactor, no behaviour change

## v0.135.11

- refactor: extracted FailureHandler from processor — pure refactor, no behaviour change

## v0.135.10

- refactor: extracted PreflightConditions and ContainerSlotManager from processor — pure refactor, no behaviour change

## v0.135.9

- refactor: extracted SpecSweeper from processor — pure refactor, no behaviour change

## v0.135.8

- refactor: extracted CancellationWatcher from processor — replaces *bool out-parameter with a closed-on-cancel channel; pure refactor

## v0.135.7

- refactor: extracted CompletionReportValidator and PromptEnricher from processor — pure refactor, no behaviour change

## v0.135.6

- refactor: NewProcessor argument order — services/interfaces first, typed config second; renamed ready→wakeup; exported ErrPreflightSkip for external test packages

## v0.135.5

- refactor: NewProcessor primitive parameters replaced with named types (ProjectName, ContainerName, Dirs, Commands, MaxContainers, DirtyFileThreshold, AutoRetryLimit, AdditionalInstructions, VerificationGate) — purely internal, no behaviour change

## v0.135.4

- refactor: unify daemon and one-shot processor loops via `NothingToDoCallback` constructor parameter; `processor.ProcessQueue` removed from interface and implementation; one-shot mode's manual loop in `pkg/runner/oneshot.go` replaced by `processor.Process` with a cancel-on-idle callback; daemon mode logs "nothing to do, waiting for changes" on idle ticks

## v0.135.3

- fix: `list --all`, `prompt list --all`, and `spec list --all` no longer fail with "unexpected argument" — new `validateListArgs` helper passes `--all` through to the list commands' own flag parsers

## v0.135.2

- fix: spec auto-complete now fires AFTER the last prompt is moved to prompts/completed/, not before — specs transition to `verifying` immediately on prompt completion without requiring a daemon restart (regression: workflow_executor_direct.go phase ordering); the daemon also runs a separate 60-second auto-complete sweep, self-healing any future stuck specs within ~1 minute

## v0.135.1

- fix: preflight cache is now time-based instead of SHA-based — sequential prompts within preflightInterval reuse the cached green result, saving ~1 minute per prompt; failed preflights are not cached so operator fixes are picked up immediately

## v0.135.0

- feat: dark-factory prompt list and spec list hide rejected items by default; --all shows them; rejected/ dirs are scanned for display

## v0.134.0

- feat: add dark-factory prompt reject and dark-factory spec reject commands with --reason flag, cascade, and preflight

## v0.133.0

- feat: add rejected status, IsRejectable() predicate, and Rejected/RejectedReason frontmatter fields to spec and prompt lifecycle model

## v0.132.3

- refactor: replace ad-hoc status string comparisons in pkg/cmd/ with CanTransitionTo() and typed constant checks

## v0.132.2

- refactor: add SpecStatuses/CanTransitionTo/predicates to spec.Status and CanTransitionTo/predicates to prompt.PromptStatus (Load() stays permissive — strict checks at transition boundary)

## v0.132.1

- docs: add "Test the boundaries the new code crosses" section to `docs/prompt-writing.md` — mandates unit contract tests (level 1) and/or integration tests (level 2) for every boundary a new value crosses (library validators, parsers, registries, serialization, subprocess, external services); adds DoD checkbox
- docs: add "Test-layer responsibilities" table and integration-seam preflight item to `docs/spec-writing.md` — specs must require scenario coverage for new integration seams
- docs: add triggers for "new operation through library validator" and "new registry entry" to `docs/scenario-writing.md` "When to Write a Scenario"; cite spec 015 cqrs regex incident as canonical example
- agent: add "Boundary-crossing contract tests" quality criterion + DoD checklist item to `agents/prompt-auditor.md` — flag as Critical when a prompt introduces a library-typed value without a validator/integration test
- agent: add "Test the boundaries the new code crosses" quality rule to `agents/prompt-creator.md`
- agent: add "Integration-seam scenario coverage" section + output-format checkbox to `agents/spec-auditor.md` — flag as Critical when a spec introduces a new seam without scenario coverage
- agent: add mandatory scenario-coverage acceptance criterion to `agents/spec-creator.md` template
- agent: add "Real-seam traversal" criterion to `agents/scenario-auditor.md` — scenarios must cross the actual boundary, not run in-process equivalents

## v0.132.0

- docs: add `docs/spec-verification.md` — human verification checklist for specs in `verifying` state covering three layers (technical, business, scenarios), six-step procedure, live-evidence requirement, and supersession hygiene
- docs: cross-link spec-verification from `docs/running.md` (complete-spec section) and `docs/spec-writing.md` (next-steps)

## v0.131.0

- feat: add `scenario list`, `scenario show`, and `scenario status` CLI subcommands for read-only scenario inspection

## v0.130.0

- feat: add `pkg/scenario` package with ScenarioFile model, Lister interface, and Find for scenario CLI support

## v0.129.0

- docs: replace "When to Write a Scenario" table in `docs/scenario-writing.md` with integration-seam criterion — triggers are orchestration/lifecycle, container boundary, git boundary, config→runtime, regression from missed bug; "could a unit-test-with-mocks pass while this silently breaks?"
- docs: add rule 2a to `docs/scenario-writing.md` — meta-scenarios in the dark-factory repo must build a fresh binary (`go build -C ~/Documents/workspaces/dark-factory -o /tmp/new-dark-factory .`) and invoke it, because `cd`-into-sandbox breaks `go run`
- docs: add scenario-update line to `docs/dod.md` — new/changed integration seam → scenario added or updated
- agent: add step 9 (scenario check) to `agents/prompt-creator.md` — during spec decomposition, apply the seam criterion and either add an update-scenario requirement or emit a dedicated write-scenario prompt
- CLAUDE.md: add rule — run all `scenarios/` against a freshly built binary before `make install`; unit tests + `make precommit` alone are not sufficient
- scenarios: all 4 active scenarios (`001-workflow-direct`, `002-workflow-pr`, `003-smoke-test-container`, `010-preflight-baseline-gate`) now build `/tmp/new-dark-factory` in Setup and invoke it — tests code under change, not stale installed binary
- scenarios/001: add `autoRelease: true` to yaml so tag+push expected outcomes match configured workflow

## v0.128.4

- test: add `Config`/`partialConfig` parity and round-trip invariant tests to `pkg/config` — catches silent field drops when new `Config` fields are added without updating `partialConfig` and merge helpers

## v0.128.3

- refactor: remove dead Docker-execution helpers from `pkg/preflight` (`buildPreflightDockerArgs`, `resolveExtraMountSrc`, `resolveHostCacheDir`, `darwinCacheDir`, `linuxCacheDir`) and drop `containerImage`/`extraMounts` params from `NewChecker` — preflight runs on host via `sh -c`

## v0.128.2

- fix: stop preflight-failure busy-loop by returning `errPreflightSkip` sentinel from `checkPreflightConditions`, causing `processExistingQueued` to exit the scan loop and wait for the next 5s ticker instead of re-scanning immediately

## v0.128.1

- test: fix `buildPreflightDockerArgs` extra-mount tests to use `GinkgoT().TempDir()` so `os.Stat` succeeds on the host

## v0.128.0

- feat: wire preflight baseline checker into processor — prompts skip (not fail) when baseline is broken
- docs: document preflight step in architecture-flow.md execution table and what-runs-where table

## v0.127.0

- feat: add `pkg/preflight` package — Docker-backed baseline checker with SHA-keyed in-memory cache and notification on failure

## v0.126.0

- feat: add `preflightCommand` and `preflightInterval` config fields for baseline check before prompt execution

## v0.125.1

- fix: `dark-factory` with no args and `dark-factory help` now print usage and exit 0 instead of erroring
- chore: update github.com/go-git/go-git/v5 to v5.18.0 to fix GHSA-3xc5-wrhm-f963

## v0.125.0

- feat: `dark-factory status` displays `committing` prompts with count and filenames
- docs: add `committing` status to prompt-writing.md lifecycle table and architecture-flow.md diagram

## v0.124.0

- feat: retry git commit with exponential backoff (3 retries, 2s/4s/8s) on index.lock or failure
- feat: direct workflow sets `committing` status before git ops; daemon continues on commit failure
- feat: startup and daemon-cycle recovery for `committing` prompts

## v0.123.0

- feat: add `committing` prompt status for git-persistence phase between container exit and completed

## v0.122.1

- refactor: make daemon log writer injectable via io.Writer to prevent .dark-factory.log creation during tests

## v0.122.0

- feat: daemon writes structured log output to `.dark-factory.log` (truncated on each start); `dark-factory status` shows `Daemon log:` path

## v0.121.2

- fix: hideGit now works with workflow:direct by switching from anonymous volume to tmpfs overlay for .git directories
- fix: HideGit field now parsed from .dark-factory.yaml via partialConfig
- fix: skip host-side dirty-file and git-lock checks when hideGit is enabled (nil guard + nil checker injection)
- fix: runner skips .git/index.lock startup check when hideGit is true

## v0.121.1

- bump default container image to claude-yolo v0.6.1 (includes updater v0.22.0 with --no-git support)

## v0.121.0

- feat: skip push and PR creation in clone/worktree workflows when agent produces no code changes (CommitsAhead=0 guard in handleAfterIsolatedCommit)

## v0.120.0

- feat: render rate_limit_event as a human-readable warning line in the stream formatter (previously emitted [unknown type: rate_limit_event] noise)

## v0.119.7

- refactor: extract shared startupSequence from runner and oneshot into pkg/runner/lifecycle.go

## v0.119.6

- refactor: replace 20-method prompt.Manager god interface with per-consumer narrow interfaces at point of use

## v0.119.5

- refactor: Wire narrow PromptManager interfaces into consumer packages (processor, runner, server, status, review, watcher, cmd)

## v0.119.4

- refactor: inline single-caller factory helpers createDockerExecutor and createRunnerInstance

## v0.119.3

- refactor: extract git workflow logic from processor into four WorkflowExecutor implementations (direct, branch, clone, worktree) with shared WorkflowDeps struct and factory wiring via CreateWorkflowExecutor

## v0.119.2

- refactor: define narrow per-consumer PromptManager interfaces in processor, runner, server, status, review, watcher, and cmd packages with counterfeiter fakes

## v0.119.1

- refactor: deduplicate status-checker and container-counter construction in factory

## v0.119.0

- feat: add WorkflowExecutor interface and WorkflowDeps struct to pkg/processor for git-lifecycle abstraction

## v0.118.1

- fix: clear lastFailReason from frontmatter when prompt transitions to completed status

## v0.118.0

- feat: mask /workspace/.git inside containers for worktree workflow and via hideGit config opt-in

## v0.117.0

- feat: Add `hideGit` config field and plumb `hideGit bool` through `NewDockerExecutor` and `buildDockerCommand` to conditionally mask `/workspace/.git` in the container via anonymous volume (directory) or `/dev/null` bind (worktree pointer file)

## v0.116.1

- fix: Remove broken `--tmpfs /workspace/.git` overlay from worktree-mode executor; the overlay caused a Docker mount error because a git worktree's `.git` is a file, not a directory

## v0.116.0

- feat: Wire `pkg/formatter.Formatter` into executor so every prompt run produces two log files: raw JSONL (container stdout verbatim) and human-readable formatted log; add `YOLO_OUTPUT=json` env var to docker command; bump `DefaultContainerImage` to `v0.6.0` (minimum version supporting `YOLO_OUTPUT=json` passthrough)

## v0.115.0

- feat: Add `pkg/formatter/` package — Go port of claude-yolo Python v2 stream-json formatter; reads JSONL from io.Reader, writes raw lines verbatim and human-readable formatted lines simultaneously; handles all Claude Code message types and tool types with glyphs matching the Python v2 oracle; generates Counterfeiter mock in `mocks/formatter.go`

## v0.114.0

- feat: Wire four `workflow` enum values (`direct`, `branch`, `worktree`, `clone`) into the processor, replacing `worktree bool`; add `handleWorktreeWorkflow`, `handleAfterIsolatedCommit`, `handleBranchPRCompletion`; update `CreateProcessor` factory signature; add workflow routing unit tests

## v0.113.0

- feat: Add `Worktreer` interface to `pkg/git/` with `Add`/`Remove` methods for `git worktree` operations; add `worktreeMode bool` parameter to `NewDockerExecutor` that appends `--tmpfs /workspace/.git` to hide the `.git` directory from the YOLO container

## v0.112.0

- feat: Expand `workflow` enum from two values (`direct`/`pr`) to four (`direct`, `branch`, `worktree`, `clone`); invert deprecation so `workflow` is primary and `worktree: bool` is the legacy field; add `workflow: direct + pr: true` validation; map `workflow: pr` → `workflow: clone, pr: true` and legacy `worktree: bool` combos via Compatibility Matrix

## v0.111.2

- fix: Release container flock between slot-wait polls so daemons with higher maxContainers limits are not blocked behind daemons waiting for a slot

## v0.111.1

- fix: Wire 18 missing Config fields into partialConfig and mergePartial so YAML keys like `maxPromptDuration`, `dirtyFileThreshold`, and `autoRetryLimit` are no longer silently dropped by the loader
- test: Add exhaustive loader test that round-trips every Config field through YAML to prevent regression of silent field-drop bugs

## v0.111.0

- feat: Log effective configuration at daemon and one-shot run startup — emits a single `msg="effective config"` slog line with `maxContainers`, `maxContainersSource` (project/global/default), container image, model, workflow flags, commands, debounce, and prompt lifecycle directories

## v0.110.2

- fix: Detect claude CLI critical failures (auth error, API error) in container log when no completion report is present, marking prompt as failed instead of completed
- refactor: Replace `validateCompletionReport` `(string, error)` return with `(*CompletionReport, error)` to eliminate ambiguous `("", nil)` case

## v0.110.1

- chore: Bump default claude-yolo container image to v0.5.4 (proxy allowlist + debug logging)

## v0.110.0

- feat: Bound `dark-factory status` subprocess calls (git status, docker ps) with a 3s warning and 10s hard timeout. Skipped calls are marked `(skipped)` in human output and as `*_skipped: true` flags in JSON output.

## v0.109.0

- fix: read-guides command uses `~/.claude/plugins/marketplaces/coding/docs/` (was `~/.claude-yolo/...`)

## v0.108.2

- prompt-creator and generate-prompts-for-spec discover and reference coding plugin guides before writing prompts

## v0.108.1

- chore: verify HOST_CACHE_DIR auto-default end-to-end — confirmed go-build and golangci-lint caches bind-mounted in YOLO container

## v0.108.0

- feat: auto-resolve `$HOST_CACHE_DIR` in extraMounts src with platform-appropriate default (macOS: `$HOME/Library/Caches`, Linux: `$XDG_CACHE_HOME` or `$HOME/.cache`) when unset, without mutating global environment

## v0.107.9

- fix: prompt-auditor honors container mounts from `dark-factory config` — container-absolute paths backed by a mount (e.g. `/docs/...` when `../docs → /docs`) are no longer flagged as path-portability violations

## v0.107.8

- Ignore CVE-2026-33817 in bbolt (no fix available)

## v0.107.7

- Bump go.mod toolchain to Go 1.26.2 to fix stdlib vulnerabilities

## v0.107.6

- Bump default claude-yolo container image to v0.5.3 (Go 1.26.2)

## v0.107.5

- fix: Defer `release()` in `startContainerLockRelease` goroutine to guarantee container lock is freed on context cancellation

## v0.107.4

- fix: Sanitize PR review body before embedding into generated fix prompt files to prevent prompt injection via XML/HTML-like tags

## v0.107.3

- refactor: Move constructor before struct definition in seven files to match dominant Interface → Constructor → Struct → Methods order

## v0.107.2

- refactor: Move Bitbucket current-user HTTP call out of factory into `BitbucketCurrentUserFetcher` in `pkg/git`; defer collaborator-reviewer resolution to PR creation time so the factory performs zero I/O at construction

## v0.107.1

- fix: Propagate caller context through `NewContainerLock` constructor by adding `ctx context.Context` parameter and replacing `context.Background()` error wraps in `pkg/containerlock`

## v0.107.0

- feat: Enforce `maxPromptDuration` in the health check loop by adding `ContainerStopper` interface and `dockerContainerStopper` implementation; health check now stops and marks failed any executing prompt whose container has been running longer than the configured limit

## v0.106.12

- refactor: Replace `time.Time` with `libtime.DateTime` for timestamp fields in `CompletedPrompt`, `promptWithTime`, `executingPrompt`, `skippedPrompts` map, and `parseCreated` return type across `pkg/status`, `pkg/processor`, and `pkg/reindex`

## v0.106.11

- refactor: Inject `libtime.CurrentDateTimeGetter` into `bitbucketPRMerger`, `dockerContainerChecker`, and status `checker`, replacing all direct `time.Now()` calls with the injected getter

## v0.106.10

- fix: Add `time.Local = time.UTC` and `format.TruncatedDiff = false` to suite runner functions in `pkg/globalconfig` and `pkg/reindex` test suites

## v0.106.9

- fix: Use `gopkg.in/yaml.v3` via `frontmatter.NewFormat` in `pkg/reindex/reindex.go` for consistent YAML unmarshaling across all frontmatter parsing sites

## v0.106.8

- fix: Add `ValidatePRURL` validation in `pkg/git/validate.go` and guard `gh` subprocess calls in `pr_merger.go` and `review_fetcher.go` against malformed PR URLs

## v0.106.7

- refactor: Replace bit-mask fsnotify event checks (`event.Op & fsnotify.Write == 0`) with `event.Has()` method in `pkg/watcher/watcher.go`, `pkg/specwatcher/watcher.go`, and `pkg/processor/processor.go`

## v0.106.6

- fix: Wrap bare `return err` statements in `pkg/processor/processor.go` with `errors.Wrap(ctx, err, ...)` to preserve stack traces across all processor error paths

## v0.106.5

- fix: Replace `fmt.Errorf` with `errors.Errorf`/`errors.Wrapf` in `pkg/git/validate.go`, `pkg/git/bitbucket_pr_merger.go`, and `pkg/executor/executor.go`; add `ctx context.Context` to `ValidateBranchName`, `ValidatePRTitle`, and `parseBitbucketPRID`

## v0.106.4

- fix: Wrap bare `return err` with `errors.Wrap(ctx, err, ...)` in `generator`, `slugmigrator`, `server`, `runner`, and `cmd/kill` to preserve stack traces

## v0.106.3

- test: Add 60-second suite timeout via `GinkgoConfiguration()` to all 26 `*_suite_test.go` files to prevent indefinite CI hangs

## v0.106.2

- fix: Add non-blocking `ctx.Done()` guard at top of one-shot runner loop to prevent unnecessary `ProcessQueue` cycle after context cancellation

## v0.106.1

- refactor: Replace `errors.Wrapf` with `errors.Wrap` for plain messages without format verbs in `containerlock`, `reindex`, and `executor/checker`

## v0.106.0

- feat: Wire `testCommand` config field into processor; inject `TestCommandSuffix` between changelog and validation suffixes in `enrichPromptContent`

## v0.105.0

- feat: Add `testCommand` config field (default `make test`) for fast iteration feedback during YOLO prompt execution
- feat: Add `TestCommandSuffix` function in report package to inject fast-feedback instructions into prompts
- refactor: Update `ValidationSuffix` wording to clarify it runs exactly once at the end as the authoritative final gate

## v0.104.3

- refactor: Replace manual fakeCommandRunner and multiFailRunner stubs in executor tests with counterfeiter-generated mocks.CommandRunner; move tests to external package executor_test using export_test.go bridge

## v0.104.2

- fix: reattach timeout uses remaining wall-clock duration instead of full maxPromptDuration

## v0.104.1

- chore: Generate 18 fix prompts from full code review of dark-factory root covering error handling, time injection, security, concurrency, factory pattern, and test quality findings

## v0.104.0

- fix: Add 30s timeout to git fetch in syncWithRemote — prevents daemon from hanging indefinitely when SSH credentials are unavailable
- fix: Move container lock acquisition after prompt prep work (load, enrich, setupWorkflow) — lock is now held only during the check-and-start window, not during slow operations
- feat: Prompt creator and auto-generator now extract Failure Modes and Security from specs
- feat: Prompt auditor checks failure mode coverage
- feat: Rewrite generate-code-review-prompts to delegate to `/coding:code-review full`
- docs: Add code-review-prompts guide

## v0.103.3

- fix: Replace time.After in timeoutKiller and watchForCompletionReport with wall-clock deadline polling to prevent macOS timer coalescing from delaying timeouts by 20-30 minutes when running as a background process

## v0.103.2

- fix: defaultCommandRunner now respects context cancellation — sends SIGINT/SIGKILL to child process when ctx is cancelled

## v0.103.1

- refactor: Thread ctx from main() through factory functions (CreateRunner, CreateOneShotRunner, CreateServer, CreateStatusCommand, CreateCombinedStatusCommand, CreateReviewPoller, CreatePromptCompleteCommand) and status.isContainerRunning — ensures signal cancellation propagates to all long-running operations

## v0.103.0

- feat: Add git health warnings to `dark-factory status` output — shows `.git/index.lock` and dirty file count with threshold so operators can diagnose why the daemon is skipping prompts

## v0.102.2

- refactor: Remove `permanently_failed` prompt status — exhausted retries now mark prompts `failed` with `lastFailReason` and `retryCount` preserved; users can requeue to reset the retry budget

## v0.102.1

- fix: Stop daemon when post-execution git failure occurs after prompt moved to completed/ — prevents uncommitted code changes from being silently overwritten by subsequent prompts

## v0.102.0

- feat: Add git index lock preflight check — daemon startup aborts with a clear error if `.git/index.lock` exists, and prompt/spec-generation cycles skip and retry instead of failing with `git add` exit 128

## v0.101.0

- feat: Flip `ExtraMount` default from read-only to read-write; rename struct field from `Readonly` to `ReadOnly` and YAML tag from `readonly` to `readOnly`

## v0.100.0

- feat: Wire auto-retry into processor — failed prompts are re-queued up to `autoRetryLimit` times, exhausted prompts are marked `permanently_failed` with a notification, and the daemon continues processing; `requeue --failed` also handles `permanently_failed` prompts and resets the retry counter

## v0.99.0

- feat: Wire `maxPromptDuration` into executor — containers exceeding their wall-clock budget are stopped cleanly via `docker stop` (with `docker kill` fallback) and return a descriptive timeout error; zero duration disables the timeout

## v0.98.0

- feat: Add `maxPromptDuration` and `autoRetryLimit` config fields with validation and `ParsedMaxPromptDuration()` helper
- feat: Add `permanently_failed` prompt status for prompts that exhaust auto-retries
- feat: Add `lastFailReason` frontmatter field and `MarkPermanentlyFailed`/`SetLastFailReason` methods on `PromptFile`

## v0.97.0

- feat: Improve blocked prompt diagnostics — log missing prompt numbers and their status (e.g., `failed`, `executing`, `not found`) and deduplicate repeated blocked messages

## v0.96.0

- feat: Add `dirtyFileThreshold` config to skip prompts when git working tree has too many dirty files

## v0.95.0

- feat: Add global container lock (`pkg/containerlock`) that serializes the count-and-start window across multiple dark-factory daemon instances, preventing concurrent daemons from exceeding `maxContainers`

## v0.94.2

- chore: Verify GOPATH mount provides Go module cache inside YOLO container at `/home/node/go/pkg`

## v0.94.1

- **breaking:** Remove hardcoded `$HOME/go/pkg` mount — add `extraMounts` with `${GOPATH}/pkg` for Go projects

## v0.94.0

- feat: Expand environment variables (`$VAR`, `${VAR}`) in `extraMounts` `src` paths before tilde and relative path resolution

## v0.93.0

- feat: Add `--max-containers N` flag to `run` and `daemon` commands to override the container limit for a single invocation without editing config files; priority chain is CLI arg > project config > global config > default (3)

## v0.92.0

- feat: Strict CLI arg validation — every command rejects unknown args and flags with an error and relevant help text; `--help`/`-h` intercepted before config load or lock acquisition on all commands and subcommands

## v0.91.0

- feat: Periodic container health check loop detects disappeared executing prompt containers and generating spec containers at runtime, resetting them to approved within 30-60 seconds without requiring a daemon restart

## v0.90.0

- feat: Add `generating` lifecycle state to spec so the health-check loop can identify specs with an active generation container; generator sets spec to `generating` before launching Execute() and resets to `approved` on non-cancellation failure; daemon startup resets orphaned `generating` specs whose container is gone

## v0.89.2

- Update docs and agents to reference coding plugin path instead of deprecated ~/.claude-yolo/docs/
- Fix scenario auditor: detect broken curl|jq piping and reserved shell variable names

## v0.89.1

- docs: Add directory locations to spec/prompt lifecycle tables and creation instructions
- docs: Update Full Example with maxContainers, additionalInstructions, container image v0.5.1
- docs: Update example .dark-factory.yaml with new config fields
- chore: Bump plugin version to 0.89.0

## v0.89.0

- feat: Add per-project `maxContainers` field to `.dark-factory.yaml` that overrides the global container limit for a specific project's daemon

## v0.88.0

- feat: Add `additionalInstructions` config field that prepends project-level context to every prompt and spec generation command

## v0.87.0

- feat: Add `extraMounts` config field to inject additional volume mounts into YOLO containers

## v0.86.1

- chore: Bump default claude-yolo container image to v0.5.1

## v0.86.0

- feat: Add scenario-auditor agent with full DoD checklist, scoring, and quality criteria
- feat: Add filename-content alignment check to prompt-auditor, spec-auditor, and scenario-auditor
- refactor: Refactor audit-scenario command to delegate to scenario-auditor agent (matching prompt/spec pattern)
- chore: Bump plugin version to 0.86.0

## v0.85.0

- feat: Add configurable `generateCommand` config field (default `/dark-factory:generate-prompts-for-spec`)
- feat: Add `generate-prompts-for-spec` and `run-prompt` plugin commands for YOLO container use
- chore: Add `autoRelease: true` to project config

## v0.84.1

- fix: Respect `autoRelease` flag in direct workflow — commit-only when false, tag and push when true
- fix: Remove incorrect validation that required `autoRelease` to have `autoMerge` enabled

## v0.84.0

- feat: Add `dark-factory kill` command that stops the running daemon via SIGTERM with SIGKILL fallback after 5 seconds

## v0.83.0

- feat: Show system-wide container count in `dark-factory status` output as `Containers: N/M (system-wide)`

## v0.82.0

- feat: Enforce system-wide container limit via ~/.dark-factory/config.yaml maxContainers (default: 3)
- feat: Add ContainerCounter interface counting running dark-factory containers via docker ps label filter

## v0.81.0

- feat: Add `pkg/globalconfig` package to load ~/.dark-factory/config.yaml with maxContainers field
- feat: Show global config section in `dark-factory config` output

## v0.80.0

- feat: Add `project_dir` to `dark-factory status` output so users can confirm which project root was resolved when running from subdirectories

## v0.79.0

- feat: Add per-group help to `prompt` and `spec` commands — `--help`, `-h`, `help`, and no-args print available subcommands with descriptions instead of returning an error

## v0.78.0

- feat: Add `dark-factory spec unapprove` command to reverse spec approval, moving file back to inbox with draft status, clearing approved/branch metadata, and renumbering remaining in-progress specs to close the gap

## v0.77.0

- feat: Add `dark-factory prompt unapprove` command to reverse prompt approval, moving file back to inbox with draft status and renumbering remaining queue entries

## v0.76.0

- feat: Add `UpdateSpecRefs` to `pkg/reindex` to propagate spec file renames to prompt frontmatter and filenames, and wire full reindex sequence into daemon and one-shot runner startup

## v0.75.0

- feat: Add `pkg/reindex` package to detect and resolve duplicate numeric prefixes across lifecycle directories using `created` frontmatter date as tie-breaker

## v0.74.0

- feat: Wire `slugmigrator.Migrator` into generator post-processing, daemon runner startup, and one-shot runner startup so all spec number references are migrated to full slugs on every run

## v0.73.0

- feat: Add `pkg/slugmigrator` with `Migrator` interface and implementation to replace bare spec number references in prompt files with full spec slugs

## v0.72.1

- docs: Update yolo-container-setup.md to replace `DARK_FACTORY_CLAUDE_CONFIG_DIR` env var with `claudeDir` config field

## v0.72.0

- feat: Add `config` command to show effective configuration

## v0.71.1

- chore: Bump default claude-yolo image to v0.5.0

## v0.71.0

- feat: Add `claudeDir` config field to set claude-yolo config directory per project (replaces `DARK_FACTORY_CLAUDE_CONFIG_DIR` env var)

## v0.70.0

- Add documentation placement guide (docs/documentation.md) covering 4 knowledge locations
- Extend prompt-auditor and spec-auditor with documentation placement checks
- Extend prompt-creator and spec-creator to scan existing docs before writing
- Expand read-guides command to scan project docs and yolo docs index

## v0.69.1

- Update bborbe/http to v1.26.8
- Update bborbe/run to v1.9.12
- Update indirect dependencies (kv, log, math, sentry, getsentry)

## v0.69.0

- feat: rename `prompt verify` to `prompt complete`, accept `failed`, `in_review`, and `executing` prompts in addition to `pending_verification`

## v0.68.1

- fix: `MergeOriginDefault` now skips gracefully with a warning instead of returning an error when the default branch cannot be determined (local bare repos, non-GitHub remotes without `defaultBranch` config)

## v0.68.0

- feat: Add git-native fallback to `DefaultBranch()` using `git symbolic-ref refs/remotes/origin/HEAD` so dark-factory works with non-GitHub remotes (Bitbucket, GitLab, local bare repos) without requiring `defaultBranch` in config

## v0.67.9

- Wrap bare `return err` in spec-show command with `errors.Wrap` for consistent error context
- Update default container image to claude-yolo v0.4.2 (fixes prompt quoting in headless mode)

## v0.67.8

- Update default container image to claude-yolo v0.4.1 (fixes root UID remapping on Docker Desktop for Mac)

## v0.67.7

- Bump core deps: errors, time, validation, golangci-lint, osv-scanner, go-modtool
- Bump docker/moby deps: docker v28.5.2, buildkit v0.28.1, containerd/hcsshim
- Fix non-deterministic test assertion in executor Reattach cancellation case
- Add daemon vs run decision table to running.md
- Add DoD setup step to init-project guide

## v0.67.6

- update default container image to claude-yolo v0.4.0

## v0.67.5

- fix prompt temp file permissions so container user can read mounted prompt

## v0.67.4

- update plugin version to match release

## v0.67.3

- fix: Daemon shutdown leaves prompt in executing state so resume-on-restart can reattach
- fix: Check ctx.Err() on both error and success paths after Execute to prevent post-execution on shutdown
- refactor: Extract enrichPromptContent to fix funlen lint

## v0.67.2

- test: Verify resume-on-restart by sleeping 300s and creating test-resume-marker.txt
- fix: Skip `handlePromptFailure` on daemon shutdown so prompt stays in `executing` state for resume-on-restart

## v0.67.1

- test: Verify dark-factory resume-on-restart by sleeping 120s and creating test-resume-marker.txt

## v0.67.0

- feat: Extend re-attach mechanism to spec generation containers — SpecGenerator checks if a generation container is already running on restart and reattaches to it instead of launching a new one

## v0.66.0

- feat: Add `Reattach` to Executor and `ResumeExecuting` to Processor so executing prompts with still-running containers are reconnected to on daemon restart instead of being reset

## v0.65.0

- feat: selective container liveness check on startup — executing prompts with a still-running container are left in executing state for re-attach; prompts without a container are reset to approved and fire `stuck_container` notification

## v0.64.1

- refactor: remove `prompts/ideas/` directory concept — ideas with `status: idea` live in the normal `prompts/` inbox; removed `IdeasCount` from status output

## v0.64.0

- feat: show spec generation progress in `dark-factory status` — detects running `gen-*` containers and displays `generating spec <name>` instead of `idle`

## v0.63.2

- Update bborbe/* dependencies (collection, errors, http, run, time, validation)
- Update golang.org/x/* packages (crypto, mod, net, sys, term, text, tools)
- Update osv-scanner, gosec, go-modtool, and other tooling deps
- Remove replace/exclude blocks and clean up go.mod

## v0.63.1

- docs: add `prompt cancel` and `prompt requeue` to README command table; remove renamed `prompt retry` row

## v0.63.0

- feat: detect prompt cancellation during execution via fsnotify and gracefully stop/remove the Docker container

## v0.62.0

- feat: add `cancelled` prompt status and `dark-factory prompt cancel <id>` CLI command to cancel approved or executing prompts

## v0.61.1

- fix: replace `created` with `draft` as prompt inbox status in docs, agents, and templates

## v0.61.0

- feat: add `idea` status to both prompt and spec lifecycles, representing rough concepts needing refinement before draft

## v0.60.1

- upgrade golangci-lint from v1 to v2
- update .golangci.yml to v2 format
- standardize Makefile: go mod tidy -e, parallel golines, gosec flags
- fix lint issues: use fmt.Fprintf, tagged switch, nolint for ST1005

## v0.60.0

- feat: Add `--auto-approve` flag to `dark-factory run`; without it, generated prompts stay in inbox for manual review instead of being auto-approved and executed

## v0.59.6

- chore: upgrade github.com/modelcontextprotocol/go-sdk to v1.4.1 and google.golang.org/grpc to v1.79.3 to resolve CVEs GHSA-89xv-2j6f-qhc8, GHSA-q382-vc8q-7jhj, and GHSA-p77j-4mvh-x3m3

## v0.59.5

- Update claude-yolo container image from v0.3.0 to v0.3.1

## v0.59.4

- Add init smoke-test prompt template (docs/init-prompt-fix-tests-and-dod.md)
- Update init-project guide with current config format (pr/worktree instead of legacy workflow)
- Add Step 7 smoke-test prompt to init-project guide

## v0.59.3

- Document watch skill in README and docs/running.md
- Add all plugin commands to README table

## v0.59.2

- Watch skill auto-detects project directory via lock file when not in project root

## v0.59.1

- Restructure watch skill: command invokes skill via Skill tool, script in scripts/ subdirectory

## v0.59.0

- Add watch skill with standalone bash script for monitoring daemon execution with sound alerts

## v0.58.1

- Clarify run command documentation: dark-factory run generates prompts from approved specs then executes all queued prompts
- Fix generate-code-review-prompts Glob instruction clarity

## v0.58.0

- Add `generate-code-review-prompts` command — reviews a service against project coding guidelines and generates fix prompts for Critical and Important findings

## v0.57.5

- fix: Change Go module cache mount from read-only to read-write so YOLO containers can download uncached dependencies

## v0.57.4

- Add prompt to mount Go module cache read-write in YOLO container

## v0.57.3

- Add architecture and execution flow guide (docs/architecture-flow.md)
- Update README.md with new docs, current config format, and validationPrompt example
- Replace deprecated workflow config in Quick Start and examples

## v0.57.2

- Add comprehensive configuration guide (docs/configuration.md) covering all config fields
- Improve validationPrompt documentation to clarify it's AI self-evaluation, not a command
- Add context header to DoD file explaining its purpose to the executing agent

## v0.57.1

- Add Definition of Done (docs/dod.md) and enable validationPrompt in config
- Complete specs 028-032 (shared-branch, branch-execution, bitbucket-server, notifications, validation-prompt)

## v0.57.0

- feat: wire `validationPrompt` into prompt execution pipeline — resolves config value as file path or inline text, appends AI-judged criteria suffix after `validationCommand`, logs warning and skips on missing `.md` file

## v0.56.0

- feat: add `validationPrompt` config field with path-safety validation (rejects absolute paths and `..` traversal)

## v0.55.2

- docs: add validation-prompt spec (AI-judged quality criteria, inline text or file path, partial status on failure)
- docs: add container-health-check and resume-executing-on-restart draft specs

## v0.55.1

- fix: spec-creator agent wrote specs to `specs/in-progress/` with numbered filenames — must go to `specs/` inbox without numbers (dark-factory assigns numbers on approve)

## v0.55.0

- feat: add `/dark-factory:run`, `/dark-factory:daemon`, `/dark-factory:init-project`, `/dark-factory:read-guides` slash commands
- fix: sync plugin version in `.claude-plugin/plugin.json` and `marketplace.json` with CHANGELOG
- docs: add plugin version sync rule to CLAUDE.md

## v0.54.1

- fix: data race in executor test fakeCommandRunner (protect err field with mutex)
- add draft specs for container health check and resume-on-restart

## v0.54.0

- feat: add `ValidateBranchName` and `ValidatePRTitle` in `pkg/git/validate.go` to reject argument-injection payloads from YAML frontmatter before they reach exec.CommandContext

## v0.53.0

- feat: mount Go module cache read-only in Docker containers to prevent prompt code from tampering with the shared module cache

## v0.52.4

- docs: add CI, Go Reference, and Go Report Card badges to README
- docs: update Go prerequisite from 1.24 to 1.26 to match go.mod

## v0.52.3

- refactor: extract `DetermineBumpFromChangelog` into `pkg/git/changelog.go`, replacing duplicated logic in `pkg/processor` and `pkg/cmd`

## v0.52.2

- refactor: extract `normalizeFilenames`, `migrateQueueDir`, and `createDirectories` into shared package-level functions in `pkg/runner/lifecycle.go`, eliminating verbatim duplication between `runner` and `oneShotRunner`

## v0.52.1

- test: add `time.Local = time.UTC` and `format.TruncatedDiff = false` setup to `pkg/specnum` suite, and add `//go:generate` counterfeiter directive to `specnum`, `report`, `project`, and root test suites

## v0.52.0

- feat: reject literal GitHub token values in `github.token` config field; only `${VAR_NAME}` env var references are accepted to prevent accidental credential leakage

## v0.51.16

- refactor: move counterfeiter directive in `pkg/git/cloner.go` above the GoDoc comment to match canonical placement pattern

## v0.51.15

- test: add `Workflow.Validate` coverage for empty string, worktree migration error, and unknown workflow in `pkg/config`

## v0.51.14

- refactor: remove `mock` prefix from Counterfeiter fake variable names in test files to match project convention

## v0.51.13

- fix: use yaml.v3 format in `spec.Load` frontmatter parsing to match `prompt.Load` behavior

## v0.51.12

- refactor: inject `libtime.CurrentDateTimeGetter` into `spec.Lister`, `prompt.Counter`, `prompt.ListQueued`, `prompt.HasExecuting`, `server.NewQueueActionHandler`, and `generator.countCompletedPromptsForSpec` instead of constructing inline

## v0.51.11

- fix: create prompt temp files inside a restricted subdirectory (`os.MkdirTemp`) to prevent other local processes from reading prompt content

## v0.51.10

- fix: add explicit path containment check in queue action handler to reject filenames that escape the inbox directory

## v0.51.9

- fix: reject env variable values containing control characters (`\x00`, `\n`, `\r`) in config validation to prevent Docker environment injection

## v0.51.8

- refactor: share single `QueueActionHandler` instance between `/api/v1/queue/action` and `/api/v1/queue/action/all` routes in `CreateServer`

## v0.51.7

- refactor: inline `createOptionalServer` and `createOptionalReviewPoller` into `CreateRunner` to eliminate conditional branching in factory helpers

## v0.51.6

- refactor: defer collaborator fetch from factory initialization to first ReviewPoller polling iteration using lazy resolution via CollaboratorFetcher interface

## v0.51.5

- fix: wrap bare `return err` calls in processor, cloner, and collaborator_fetcher with `errors.Wrap` for error context

## v0.51.4

- fix: add ctx.Done() cancellation guards to loops in generateFromApprovedSpecs, approveInboxPrompts, and scanExistingInProgress

## v0.51.3

- refactor: reorder declarations in generator, prompt manager, spec auto-completer, cloner, and collaborator fetcher to follow Interface → Constructor → Struct → Methods convention

## v0.51.2

- refactor: reorder declarations in processor, runner, oneshot, watcher, specwatcher, and executor to follow Interface → Constructor → Struct → Methods convention

## v0.51.1

- fix: remove shadowed named return `err` in `preparePromptForExecution` to resolve golangci-lint `result err is always nil` warning

## v0.51.0

- feat: add shared bitbucketClient HTTP helper with redactToken for Bitbucket Server implementations in pkg/git
- refactor: rename extractBitbucketPRID to parseBitbucketPRID with improved URL parsing that handles /overview suffix and non-numeric IDs
- test: add unit tests for parseBitbucketPRID and redactToken

## v0.50.0

- feat: add Provider enum (github, bitbucket-server) with validation to pkg/config
- feat: add BitbucketConfig struct, provider/bitbucket fields to Config, validateBitbucketConfig, and ResolvedBitbucketToken helper
- feat: add ParseBitbucketRemoteURL and ParseBitbucketRemoteFromGit to pkg/git for extracting project key and repo slug from SSH and HTTPS Bitbucket Server remote URLs

## v0.49.0

- feat: add Bitbucket Server PR creator, merger, review fetcher, and collaborator fetcher implementations in pkg/git
- refactor: replace ghToken/defaultBranch params in CreateProcessor with brancher/prCreator/prMerger interfaces; add createProviderDeps factory helper that selects GitHub or Bitbucket Server implementations based on cfg.Provider

## v0.48.0

- feat: wire notifier into processor (prompt_failed, prompt_partial), spec auto-completer (spec_verifying), review poller (review_limit), and runner (stuck_container) trigger points

## v0.47.0

- feat: add pkg/notifier package with Telegram, Discord, and multi-notifier implementations
- feat: add NotificationsConfig to Config with ResolvedTelegramBotToken, ResolvedTelegramChatID, ResolvedDiscordWebhook helpers and HTTPS validation for Discord webhook URLs
- fix: strip number prefix from inbox files on approve so NormalizeFilenames assigns correct numbers

## v0.46.2

- fix: NormalizeFilenames no longer scans inbox dir for used numbers, preventing inbox drafts from polluting number assignment
- fix: notification prompts remove invalid branch field, fix test placement, resolve contradictions

## v0.46.1

- docs: add notifications spec (Telegram + Discord) with tokenEnv pattern and security constraints

## v0.46.0

- feat: detect stuck containers by polling log for `DARK-FACTORY-REPORT` marker and calling `docker stop` after 2-minute grace period via `run.CancelOnFirstFinish`

## v0.45.0

- feat: add provider enum and Bitbucket Server config with URL parser for git remote detection
- feat: add stuck container detection prompt with `run.CancelOnFirstFinish` pattern
- chore: bump default container image to claude-yolo v0.3.0
- chore: queue prompts 173-198 for execution

## v0.44.3

- fix: stop infinite retry cycle — one-shot mode (`ProcessQueue`) exits non-zero on prompt failure; daemon mode (`Process`) logs a warning and stays alive; failed prompts require explicit `dark-factory prompt retry` to re-enter the queue

## v0.44.2

- fix: ignore `branch` frontmatter field in direct mode (`pr: false`, `worktree: false`) to prevent false "working tree is not clean" failures

## v0.44.1

- fix: mock gh CLI in PRCreator tests to avoid CI failure when GH_TOKEN is not set

## v0.44.0

- feat: add Claude Code marketplace plugin manifest (`.claude-plugin/marketplace.json`)
- refactor: replace `workflow` config field with explicit `pr` and `worktree` booleans
- fix: correct FindOpenPR test to expect success when no open PR exists
- chore: reformat CHANGELOG preamble

## v0.43.0

- feat: make PR creation idempotent per branch — `FindOpenPR` on `PRCreator` checks for an existing open PR before calling `Create`; auto-merge is deferred until the last prompt on a branch completes via `HasQueuedPromptsOnBranch`; PR body includes issue tracker reference when `Frontmatter.Issue` is set; add `Issue()` getter to `PromptFile`

## v0.42.0

- feat: guard releases on feature branches — `handleDirectWorkflow` commits without releasing when `featureBranch` is set; after the last prompt on a branch completes, `handleBranchCompletion` merges to default and triggers a full release; add `HasQueuedPromptsOnBranch` to `prompt.Manager` and `MergeToDefault` to `git.Brancher`

## v0.41.0

- feat: split `worktree bool` from `pr bool` in processor — clone-based execution and PR creation are now independent flags; add in-place branch switching for non-worktree mode (switches/creates branch before execution, restores default branch after); add existing-remote-branch tracking in clone mode via `git checkout --track`; add `IsClean` to `Brancher` interface

## v0.40.0

- feat: add `issue` field to prompt frontmatter; spec generator post-processes new prompt files to inherit `branch` and `issue` from the parent spec without overwriting explicit values

## v0.39.0

- feat: add optional `branch` and `issue` fields to spec frontmatter; `spec approve` auto-assigns `dark-factory/spec-NNN` branch from spec number, preserving any pre-existing branch value; validates branch name at approve time

## v0.38.0

- feat: replace `workflow: direct|pr` string enum with `pr` and `worktree` boolean flags in `.dark-factory.yaml`; old `workflow` field is deprecated and mapped automatically with a warning; setting both is a validation error

## v0.37.0

- feat: `dark-factory run` now generates prompts from approved specs before draining the queue, looping until no approved specs and no queued prompts remain

## v0.36.3

- refactor: inject `libtime.CurrentDateTimeGetter` into all production code that previously called `time.Now()` directly — eliminates `nowFunc func() time.Time` patterns in `PromptFile`, `SpecFile`, `PRMerger`, `Watcher`, `SpecWatcher`, and all cmd/server/generator constructors; promotes `github.com/bborbe/time` to a direct dependency; adds `prompt.NewPromptFile` constructor for test use

## v0.36.2

- fix: silence idle daemon log noise — `processExistingQueued` no longer logs at INFO when the queue is empty; daemon prints "waiting for changes" once after startup; one-shot mode logs "no queued prompts" once when nothing is queued

## v0.36.1

- fix: mount `gitconfigFile` as read-only at `/home/node/.gitconfig-extra` instead of writable at `/home/node/.gitconfig` — Docker bind-mounted files cannot be atomically replaced by `git config --global`, causing "Device or resource busy" errors; the claude-yolo entrypoint (v0.2.9+) copies the staged file before appending proxy settings
- chore: bump default container image from `v0.2.8` to `v0.2.9`
- refactor: extract `DefaultContainerImage` const to `pkg/const.go` — single source of truth for container image version

## v0.36.0

- feat: add `env` config field to `.dark-factory.yaml` — passes a map of key-value pairs as `-e KEY=VALUE` environment variables to the YOLO container, enabling project-specific settings like `GOPRIVATE` and `GONOSUMCHECK` without modifying the container image

## v0.35.0

- feat: add `gitconfigFile` config field to `.dark-factory.yaml` — mounts the specified file at `/home/node/.gitconfig-extra:ro` in the YOLO container, enabling per-project git URL rewrites for private Go module resolution over HTTPS
- fix: `netrcFile` and `gitconfigFile` config paths now expand leading `~/` to the home directory before validation and docker mount, matching actual Go/Docker path resolution behaviour

## v0.34.0

- feat: add `netrcFile` config field to `.dark-factory.yaml` — mounts the specified file read-only at `/home/node/.netrc` in the YOLO container, enabling `go mod tidy`/`go mod verify` for projects with private modules on Bitbucket Server, GitLab, or any HTTPS-authenticated git host
- chore: bump default container image from `v0.2.7` to `v0.2.8`

## v0.33.1

- fix: `defaultBranch` config field was silently ignored — missing from `partialConfig` in loader

## v0.33.0

- feat: add `defaultBranch` config field to `.dark-factory.yaml` so dark-factory works with non-GitHub repos (Bitbucket, GitLab, etc.) — when set, `DefaultBranch()` returns the configured value directly without calling `gh repo view`

## v0.32.1

- fix: one-shot mode logs `no queued prompts, exiting` at INFO level when the queue is empty, so users get clear feedback instead of a silent exit
- fix: preflight auth check supports Claude Code v2.x token location (`.credentials.json`) in addition to legacy `.claude.json`
- fix: root_test.go symlink resolution on macOS (`/var` vs `/private/var`)

## v0.32.0

- feat: Resolve git repository root at startup and chdir to it, so config, lock file, and prompt directories resolve correctly when dark-factory is invoked from any subdirectory

## v0.31.1

- fix: Add preflight OAuth token check before starting Docker — fails fast with actionable error when Claude config is missing or token is absent; skipped when `ANTHROPIC_API_KEY` is set

## v0.31.0

- feat: split `dark-factory run` into one-shot mode (drain queue and exit) and `dark-factory daemon` (long-running watcher, previous behavior)
- feat: `run` exits cleanly after processing all queued prompts — suitable for CI and scripted scenarios
- feat: `daemon` preserves all existing long-running watcher behavior
- fix: bare `dark-factory` with no subcommand now returns an error instead of defaulting to `run`

## v0.30.17

- fix: `getNextVersion` falls back to parsing CHANGELOG.md for the latest version when no git tags exist, so repos with changelog history but no tags get correct version increments

## v0.30.16

- fix: PR workflow prompt lifecycle (move to completed, status updates, PR URL) now happens in the original repo, not in the clone
- fix: Stale clone directories from crashed runs are automatically removed before cloning
- fix: Git clone and checkout errors now include stderr output for easier debugging
- fix: Clone operation logs at Info level for visibility

## v0.30.15

- refactor: Extract RepoNameFetcher and CollaboratorLister interfaces from CollaboratorFetcher to enable proper mocking
- fix: Resolve data race in processor_test.go by using ExecuteArgsForCall instead of captured variable
- refactor: Simplify PR workflow config by removing unused WorkflowWorktree variant
- test: Add PR workflow processor tests for clone, branch, commit, push, and PR creation

## v0.30.14

- test: Add ChangelogSuffix test coverage in pkg/report

## v0.30.13

- test: Standardise test suite setup with time.Local and format.TruncatedDiff across report, review, project, and spec packages
- test: Extract TestReport entry point into dedicated report_suite_test.go
- test: Rename mock-prefixed test variables to follow project convention in poller_test.go and status_test.go

## v0.30.12

- refactor: Extract fetchCollaborators from factory into CollaboratorFetcher type in pkg/git

## v0.30.11

- fix: Reject PR titles starting with a dash to prevent CLI argument injection in gh pr create
- fix: Make Docker NET_ADMIN/NET_RAW capabilities opt-in via netAdmin config field (default false)
- fix: Log and continue on fsnotify watcher errors instead of terminating Watch loop

## v0.30.10

- refactor: Extract shared parseSpecNumber logic into new pkg/specnum package, used by pkg/spec and pkg/prompt

## v0.30.9

- refactor: Move standalone format helper functions (formatDuration, formatTime, formatHMS, formatMS, formatS) from status.go to formatter.go

## v0.30.8

- fix: Wrap bare error returns in server handlers, review poller, fix prompt generator, runner, and workflow validation with errors.Wrap/Wrapf and %q formatting

## v0.30.7

- docs: Add package-level doc.go files to all 19 packages and GoDoc comments to Workflow methods, report markers, and spec status constants

## v0.30.6

- refactor: Move counterfeiter directives above GoDoc comments with blank line separator across all interfaces

## v0.30.5

- chore: Update actions/checkout from v4 to v6 in all GitHub Actions workflows

## v0.30.4

- refactor: Replace git worktree with local git clone in PR workflow; clone path moved to `/tmp/dark-factory/` for self-contained `.git` directory compatible with Docker mounts

## v0.30.3

- refactor: Unify `workflow: pr` to always use an isolated git worktree; remove old in-place PR branching code path, `AmendCommit`, and `ForcePush`

## v0.30.2

- refactor: Remove `workflow: worktree` as a valid config value; startup now returns a migration error directing users to use `workflow: pr` instead

## v0.30.1

- refactor: Replace go-git library with `git mv` and `git tag --list` CLI calls in `pkg/git`, eliminating the direct go-git dependency

## v0.30.0

- feat: Add `dark-factory prompt verify <file>` command to complete the human verification gate: moves prompt to completed/, commits, and runs the appropriate post-completion git flow (commit+release for direct workflow; commit+push+PR for PR/worktree workflow)

## v0.29.0

- feat: Wire verification gate into processor post-execution flow: when `verificationGate` is enabled, successfully-executed prompts enter `pending_verification` instead of completing; queue blocks until human verifies

## v0.28.0

- feat: Add `PendingVerificationPromptStatus` constant, `MarkPendingVerification()` and `VerificationSection()` methods on `PromptFile`, skip `pending_verification` prompts in `ListQueued`, and add `verificationGate` config field

## v0.27.0

- feat: Inject project-level validation command into every prompt; configure via `validationCommand` in `.dark-factory.yaml` (default: `make precommit`), overriding per-prompt `<verification>` sections

## v0.26.0

- feat: Assign sequential numeric prefix to spec files on `spec approve`, consistent with prompt numbering

## v0.25.1

- fix: Scan for specs stuck in `prompted` status on daemon startup and transition them to `verifying` if all linked prompts are completed

## v0.25.0

- feat: Add `spec show <id>` and `prompt show <id>` subcommands to display single-item details (status, timestamps, linked prompts, log path) with `--json` support

## v0.24.2

- refactor: Rename `Status` to `PromptStatus`, add `draft` and `approved` statuses (replaces `queued`), follow go-enum-type-pattern with `AvailablePromptStatuses`, `PromptStatuses`, `Contains`, `String`, `Validate`; rename `MarkQueued()` to `MarkApproved()`

## v0.24.1

- refactor: Remove duplicate `prompt queue` command; use `prompt approve` instead

## v0.24.0

- feat: Make Docker executor Claude config dir configurable via `DARK_FACTORY_CLAUDE_CONFIG_DIR` env var, defaulting to `~/.claude`

## v0.23.2

- refactor: Rename `spec verify` command to `spec complete` for clarity

## v0.23.1

- fix: Compare spec IDs by parsed integer prefix so "019" matches "019-review-fix-loop.md" in spec progress counting and auto-completion

## v0.23.0

- fix: Stop dark-factory from writing prompt filenames as changelog entries; remove `insertVersionSection` dead code
- feat: Append changelog instructions to YOLO prompt when project has CHANGELOG.md, delegating entry authorship to the agent
- fix: `updateChangelog` now returns an error when no `## Unreleased` section is found, causing the prompt to fail and retry

## v0.22.2

- docs: Add spec for prompt verification gate (host-side verification after container execution)
- docs: Add spec for prompt validation command (in-container validation via config)
- docs: Add prompt to fix mechanical changelog entries (delegate to YOLO agent)

## v0.22.1

- fix: SpecWatcher skips non-approved specs during prompt generation

## v0.22.0

- fix: SpecWatcher skips prompt generation for specs not in `approved` status (prevents re-generation of `prompted`/`verifying` specs)
- feat: `prompt approve`, `prompt requeue`, and `prompt queue` accept filename without `.md` extension or numeric prefix (e.g. `122` matches `122-fix-bug.md`)

## v0.21.1

- chore: Update Go dependencies

## v0.21.0

- feat: Hide completed prompts and specs from list commands by default; add --all flag to prompt list, spec list, and combined list to include completed items

## v0.20.6

- test: Confirm migration logic exists in runner

## v0.20.5

- test: Confirm spec verify moves file to completed

## v0.20.4

- test: Confirm SpecWatcher only triggers on Create events

## v0.20.3

- refactor: replace flat Config fields (InboxDir, QueueDir, CompletedDir, LogDir, SpecDir) with nested PromptsConfig and SpecsConfig structs; rename queueDir to inProgressDir throughout runner, watcher, and factory

## v0.20.2

- refactor: Remove keyword list from config

## v0.20.1

- feat: stamp `created` timestamp on inbox prompt files that lack one when watcher detects a file event

## v0.20.0

- feat: Add lifecycle timestamp fields (approved, prompted, verifying, completed) to spec Frontmatter, written once on each status transition via SetStatus, MarkVerifying, and MarkCompleted

## v0.19.0

- fix: SpecGenerator no longer errors on zero new prompt files when completed prompts already exist for the spec

## v0.18.6

- feat: Add dark-factory spec verify command to transition specs from verifying to completed

## v0.18.5

- feat: Add StatusVerifying to spec lifecycle so all-prompts-merged transitions spec to verifying instead of completed

## v0.18.4

- fix: Backfill spec: ["019"] field into completed spec-019 prompts so auto-complete can detect all linked prompts are done

## v0.18.3

- fix: Suppress noisy stack trace when SpecWatcher generation is cancelled by context on shutdown

## v0.18.2

- fix: scan specsDir for already-approved specs on SpecWatcher startup so approved specs are not missed before fsnotify fires

## v0.18.1

- feat: wire SpecWatcher and SpecGenerator into Runner and Factory so approved specs automatically trigger prompt generation

## v0.18.0

- feat: Add SpecWatcher to watch specs/ directory and trigger SpecGenerator when a spec transitions to approved status

## v0.17.39

- feat: Add SpecGenerator interface and dockerSpecGenerator to run /generate-prompts-for-spec via YOLO container

## v0.17.38

- Add ReviewPoller and fix-prompt pipeline wired into factory runner (completes spec 019 review-fix loop)

## v0.17.37

- Add ReviewPoller goroutine to watch in-review prompts and trigger fix-prompt generation on PR review comments

## v0.17.36

- Add FixPromptGenerator to write fix prompts to inbox from PR review comment content

## v0.17.35

- Add ReviewFetcher to poll GitHub PR review comments for prompts in `in_review` status

## v0.17.34

- Add retry_count field to prompt frontmatter to track fix-loop attempts and enforce retry limit

## v0.17.33

- Add autoReview config option to enable automated PR review-fix loop (default: false)

## v0.17.32

- Improve dark-factory list output with compact aligned columns

## v0.17.31

- Add backfill command to populate spec fields on existing completed prompts

## v0.17.30

- Add spec[] array field to prompt frontmatter for linking prompts to their source spec

## v0.17.29

- Fix unknown subcommand to display help text instead of silently failing

## v0.17.28

- Fix NormalizeFilenames number conflict when renaming prompts with non-standard filename format

## v0.17.27

- Remove approve-all command (superseded by per-spec approve)

## v0.17.26

- Auto-transition spec to completed when all linked prompts are merged

## v0.17.25

- Add combined spec+prompt view to dark-factory list output

## v0.17.24

- Add spec field to prompt frontmatter for linking prompts to their source spec

## v0.17.23

- Add dark-factory spec subcommands: list, approve, show

## v0.17.22

- Restructure CLI to two-level commands (dark-factory spec <subcommand>, dark-factory prompt <subcommand>)

## v0.17.21

- Fix dark-factory status to correctly detect running daemon process

## v0.17.20

- Add Spec model with YAML frontmatter parsing and status lifecycle (draft → approved → prompted → completed)

## v0.17.19

- Add branch reuse to processor: resume existing feature branch when prompt already has one assigned

## v0.17.18

- Add fetch-and-verify step to Brancher to confirm remote branch state before push

## v0.17.17

- Add branch field to prompt frontmatter to track the associated feature branch

## v0.17.16

- Improve processor test coverage to ≥80%

## v0.17.15

- Add dark-factory list, requeue, and retry CLI subcommands

## v0.17.14

- Fix executor to remove stale Docker container before starting a new run

## v0.17.13

- Improve workflow package test coverage to ≥80%

## v0.17.9
- Bump default container image to claude-yolo:v0.1.2

## v0.17.8
- Add --help/-h and --version/-v flags (exit before factory starts)

## v0.17.2

- Fix CommitCompletedFile to stage only the completed file instead of `git add -A`

## v0.17.1

- Log skipped prompts at Warn/Info level instead of Debug
- Auto-set `status: queued` when prompt is picked up from queue directory

## v0.17.0

- Add specs section to README listing all 16 specs with problem summaries
- Bump test coverage: executor 46.9% → 80.0%, factory 41.7% → 97.3%
- Code review cleanup: accept ctx in ParseFromLog, inject time dependency, fix hardcoded defaults, add ctx to Save, fix worktree double-cleanup, hoist regexps, replace bubble sort with sort.Slice
- Fix infinite recursion in PromptFile.now()
- Extract worktree cleanup to reduce cognitive complexity

## v0.16.0

- Add `pr-url` field to prompt frontmatter in PR and worktree workflows
- Add `ForcePush` with `--force-with-lease` for safe force-pushing
- Add `AmendCommit` for amending commits with frontmatter updates
- Add spec 016: prompt status frontmatter (draft)

## v0.15.0

- Add configurable GitHub identity via `github.token` config with `${ENV_VAR}` resolution
- Add `GH_TOKEN` injection for `gh` CLI calls in PR creator
- Add config file permission warning for world-readable files
- Add env var resolution tests and backward compatibility tests

## v0.14.5

- Refactor changelog handling to only rename ## Unreleased section

## v0.14.4

- Refactor changelog entry to use prompt summary instead of filename

## v0.14.3

- Add origin/master fetch and merge before each prompt execution

## v0.14.2

- Refactor git commit from go-git to subprocess

## v0.14.1

- Add worktree workflow to processor and factory

## v0.14.0

- Add Worktree interface and WorkflowWorktree config

## v0.13.3

- Update queueDir default to prompts/queue

## v0.13.2

- Add dark-factory version logging on startup

## v0.13.1

- Add completion summary to prompt frontmatter

## v0.13.0

- Add debug logging to watcher and processor

## v0.12.2

- Update claude-yolo image from v0.0.7 to v0.0.8

## v0.12.1

- Add project name as Docker container name prefix

## v0.12.0

- Add debug logging to executor

## v0.11.8

- Fix commit to only accept success completion report

## v0.11.7

- Add completion report parsing from YOLO log output

## v0.11.6

- Refactor prompt file Load/Save handling

## v0.11.5

- Add prompt for prompt file Load/Save refactor

## v0.11.4

- Add completion report suffix to prompt content

## v0.11.3

- Refactor frontmatter parser to use adrg/frontmatter library

## v0.11.2

- Fix completed directory files counted regardless of frontmatter

## v0.11.1

- Add retroactive specs for all existing features (12 spec files)

## v0.11.0

- Add timestamp fields to prompt frontmatter

## v0.10.7

- Remove duplicate/empty frontmatter from prompt content

## v0.10.6

- Fix failed prompts by resetting to queued on startup

## v0.10.5

- Fix Runner incorrectly normalizing inbox directory

## v0.10.4

- Fix code quality issues from code review

## v0.10.3

- Fix security issues from code review

## v0.10.2

- Fix compilation errors from code review

## v0.10.1

- Update HTTP server to disable when serverPort is 0 or missing

## v0.10.0

- Add CLI and API endpoint to queue prompts

## v0.9.1

- Fix log directory path

## v0.9.0

- Add PR-based workflow (feature branch + pull request)

## v0.8.2

- Update README with prerequisites, installation, quick start, prompt writing guide
- Add example project with config, inbox prompt, queued prompt, completed prompt
- Add `make install` target to Makefile
- Remove `build` target from Makefile
- Refactor `.PHONY` declarations next to each target

## v0.8.1

- Fix watcher to watch queueDir instead of inboxDir

## v0.8.0

- Add `dark-factory status` CLI subcommand

## v0.7.0

- Add REST API for real-time status and monitoring

## v0.6.1

- Refactor prompt directories with configurable paths

## v0.6.0

- Add dark-factory version to prompt frontmatter

## v0.5.0

- Add project configuration file support

## v0.4.0

- Add Validate() to Prompt and use in processor

## v0.3.1

- Refactor watcher and processor into independent goroutines

## v0.3.0

- Add instance lock to prevent concurrent dark-factory runs

## v0.2.36

- Refactor prompt file renames and moves to use git mv

## v0.2.35

- Fix go-git push authentication

## v0.2.34

- Fix CommitCompletedFile "entry not found" error

## v0.2.33

- Fix semver sorting with SemanticVersionNumber type

## v0.2.32

- Fix code quality issues across codebase

## v0.2.31

- Refactor git subprocess calls to use go-git library

## v0.2.30

- Add minor version bump support (never major)

## v0.2.29

- Update CHANGELOG.md and git release to be optional

## v0.2.28

- Update claude-yolo image from latest to v0.0.7

## v0.2.27

- Fix completed/ prompt file missing from release commit

## v0.2.26

- Refactor runner god object to extract service interfaces

## v0.2.25

- Refactor factory to follow Go factory pattern

## v0.2.24

- Add prompt execution serialization to prevent concurrent git conflicts

## v0.2.23

- Fix NormalizeFilenames to include completed/ numbers when assigning new numbers

## v0.2.22

- Update claude-yolo image from latest to v0.0.7

## v0.2.21

- Update .claude-yolo mount to read-write so Claude Code can write session data
- Remove unimplemented prompts 012/013 (reverted to ideas)

## v0.2.20

- Update claude-yolo image from latest to v0.0.7

## v0.2.19

- Fix NormalizeFilenames to scan completed/ and avoid duplicate number assignment
- Fix processPrompt to use context.WithoutCancel for git ops to prevent shutdown interruption
- Fix processExistingQueued to log SetStatus failure gracefully when file already moved

## v0.2.18

- Fix NormalizeFilenames to include completed/ numbers when assigning new numbers

## v0.2.17

- Fix NormalizeFilenames to include completed/ numbers when assigning new numbers

## v0.2.16

- Fix completed/ prompt file missing from release commit

## v0.2.15

- Add prompt filename validation and auto-renumbering

## v0.2.14

- Fix completed/ files not staged before git commit

## v0.2.13

- Fix code quality issues from code review

## v0.2.12

- Improve test coverage to ≥80% for all packages

## v0.2.11

- Add Docker container name tracking to prompt frontmatter

## v0.2.10

- Remove frontmatter from prompt content before passing to executor (fixes `---` parsed as CLI option)
- Fix prompt 007 status by resetting to queued after failure

## v0.2.9

- Refactor prompt content to pass via file instead of env var

## v0.2.8

- Fix frontmatter parser to not match inline `---` in content (only line-boundary delimiters)
- Fix Watcher to handle CHMOD events (macOS touch)

## v0.2.7

- Refactor prompt delivery to use mounted temp file instead of `-e` env var (fixes shell escaping with backticks, `---`, quotes)

## v0.2.6

- Fix stuck "executing" prompts by resetting to "queued" on startup
- Fix status in completed prompts 003 and 004
- Add prompt 006 for container name tracking
- Add YOLO docs for counterfeiter and suite file patterns

## v0.2.5

- Add container output logging to file

## v0.2.4

- Add container output logging to `prompts/log/{name}.log` while streaming to terminal
- Add counterfeiter-generated mock for Executor interface
- Add Executor and factory suite test files with `//go:generate` for counterfeiter
- Remove hand-written MockExecutor in factory tests (replaced by counterfeiter)

## v0.2.3

- Add test coverage for prompt and executor packages

## v0.2.2

- Fix Title to fallback to filename when no heading found (instead of failing)
- Fix empty/whitespace-only prompts to skip gracefully (not marked as failed)

## v0.2.1

- Update frontmatter to be optional for prompt pickup (no frontmatter = pick it up)
- Fix MoveToCompleted to set status to completed before moving
- Fix completed prompts having wrong status (queued/failed instead of completed)

## v0.2.0

- Add fsnotify-based directory watching for persistent daemon mode
- Add Executor interface for testability with mock executor
- Add integration tests for factory watch-and-process flow
- Add signal handling (SIGINT/SIGTERM) for graceful shutdown
- Add 500ms debounce for editor multi-event noise
- Add logging with dark-factory: prefix

## v0.1.0

- Add initial project structure from go-skeleton
- Add MVP main loop to scan prompts/ for queued prompts, execute via Docker, commit + tag + push
- Add pkg/prompt with YAML frontmatter parsing, status management, prompt listing
- Add pkg/executor with Docker claude-yolo container execution in attached mode
- Add pkg/git with commit, CHANGELOG update, semver tagging, push
- Add pkg/factory with main loop orchestrating prompt → execute → release cycle
