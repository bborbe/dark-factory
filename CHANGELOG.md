# Changelog

All notable changes to this project will be documented in this file.

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

Please choose versions by [Semantic Versioning](http://semver.org/).

* MAJOR version when you make incompatible API changes,
* MINOR version when you add functionality in a backwards-compatible manner, and
* PATCH version when you make backwards-compatible bug fixes.

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
