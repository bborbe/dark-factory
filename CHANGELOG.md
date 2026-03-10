# Changelog

All notable changes to this project will be documented in this file.

Please choose versions by [Semantic Versioning](http://semver.org/).

* MAJOR version when you make incompatible API changes,
* MINOR version when you add functionality in a backwards-compatible manner, and
* PATCH version when you make backwards-compatible bug fixes.

## v0.36.0

- feat: add `env` config field to `.dark-factory.yaml` — passes a map of key-value pairs as `-e KEY=VALUE` environment variables to the YOLO container, enabling project-specific settings like `GOPRIVATE` and `GONOSUMCHECK` without modifying the container image

## v0.35.0

- feat: add `gitconfigFile` config field to `.dark-factory.yaml` — mounts the specified file as a writable `/home/node/.gitconfig` in the YOLO container, enabling per-project git URL rewrites for private Go module resolution over HTTPS
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
