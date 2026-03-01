# Changelog

## v0.8.0

### Added
- Add `dark-factory status` CLI subcommand

## v0.7.0

### Added
- Add REST API for real-time status and monitoring

## v0.6.1

### Added
- Restructure prompt directories with configurable paths

## v0.6.0

### Added
- Add dark-factory version to prompt frontmatter

## v0.5.0

### Added
- Add project configuration file support

## v0.4.0

### Added
- Add Validate() to Prompt and use in processor

## v0.3.1

### Added
- Split watcher and processor into independent goroutines

## v0.3.0

### Added
- Add instance lock to prevent concurrent dark-factory runs

## v0.2.36

### Added
- Use git mv for prompt file renames and moves

## v0.2.35

### Added
- Fix go-git push authentication

## v0.2.34

### Added
- Fix CommitCompletedFile "entry not found" error

## v0.2.33

### Added
- Fix semver sorting with SemanticVersionNumber type

## v0.2.32

### Added
- Codebase cleanup: fix all existing code quality issues

## v0.2.31

### Added
- Replace git subprocess calls with go-git library

## v0.2.30

### Added
- Add minor version bump support (never major)

## v0.2.29

### Added
- Make CHANGELOG.md and git release optional

## v0.2.28

### Added
- Pin claude-yolo image to a specific version tag

## v0.2.27

### Added
- Fix: completed/ prompt file missing from release commit

## v0.2.26

### Added
- Extract service interfaces from runner god object

## v0.2.25

### Added
- Refactor factory to follow Go factory pattern

## v0.2.24

### Added
- Serialize prompt execution to prevent concurrent git conflicts

## v0.2.23

### Added
- Fix: NormalizeFilenames must include completed/ numbers when assigning new numbers

## v0.2.22

### Added
- Pin claude-yolo image to a specific version tag

## v0.2.21

### Fixed
- Mount .claude-yolo as read-write so Claude Code can write session data
- Revert unimplemented prompts 012/013 back to ideas

## v0.2.20

### Added
- Pin claude-yolo image to a specific version tag

## v0.2.19

### Fixed
- NormalizeFilenames now scans completed/ to avoid duplicate number assignment
- processPrompt uses context.WithoutCancel for git ops to prevent shutdown interruption
- processExistingQueued logs SetStatus failure gracefully when file already moved

## v0.2.18

### Added
- Fix: NormalizeFilenames must include completed/ numbers when assigning new numbers

## v0.2.17

### Added
- Fix: NormalizeFilenames must include completed/ numbers when assigning new numbers

## v0.2.16

### Added
- Fix: completed/ prompt file missing from release commit

## v0.2.15

### Added
- Add prompt filename validation and auto-renumbering

## v0.2.14

### Added
- Fix: completed/ files not staged before git commit

## v0.2.13

### Added
- Fix code review findings

## v0.2.12

### Added
- Increase test coverage to ≥80% for all packages

All notable changes to this project will be documented in this file.

Please choose versions by [Semantic Versioning](http://semver.org/).

* MAJOR version when you make incompatible API changes,
* MINOR version when you add functionality in a backwards-compatible manner, and
* PATCH version when you make backwards-compatible bug fixes.

## v0.2.11

### Added
- Track Docker container name in prompt frontmatter

## v0.2.10

### Fixed
- Strip frontmatter from prompt content before passing to executor (fixes `---` parsed as CLI option)
- Reset prompt 007 to queued after failure

## v0.2.9

### Added
- Pass prompt content via file instead of env var

## v0.2.8

### Fixed
- Frontmatter parser no longer matches inline `---` in content (only line-boundary delimiters)
- Watcher now handles CHMOD events (macOS touch)

## v0.2.7

### Fixed
- Pass prompt via mounted temp file instead of `-e` env var (fixes shell escaping with backticks, `---`, quotes)

## v0.2.6

### Fixed
- Reset stuck "executing" prompts to "queued" on startup
- Fixed status in completed prompts 003 and 004

### Added
- Prompt 006 for container name tracking
- YOLO docs for counterfeiter and suite file patterns

## v0.2.5

### Added
- Save container output to log file

## v0.2.4

### Added
- Save container output to `prompts/log/{name}.log` while streaming to terminal
- Counterfeiter-generated mock for Executor interface
- Executor and factory suite test files with `//go:generate` for counterfeiter

### Removed
- Hand-written MockExecutor in factory tests (replaced by counterfeiter)

## v0.2.3

### Added
- 005-test

## v0.2.2

### Fixed
- Title fallback to filename when no heading found (instead of failing)
- Empty/whitespace-only prompts skipped gracefully (not marked as failed)

## v0.2.1

### Changed
- Make frontmatter optional for prompt pickup (no frontmatter = pick it up)
- MoveToCompleted now sets status to completed before moving

### Fixed
- Completed prompts had wrong status (queued/failed instead of completed)

## v0.2.0

### Added
- fsnotify-based directory watching for persistent daemon mode
- Executor interface for testability with mock executor
- Integration tests for factory watch-and-process flow
- Signal handling (SIGINT/SIGTERM) for graceful shutdown
- 500ms debounce for editor multi-event noise
- Logging with dark-factory: prefix

## v0.1.0

### Added
- Initial project structure from go-skeleton
- MVP main loop: scan prompts/ for queued prompts, execute via Docker, commit + tag + push
- pkg/prompt: YAML frontmatter parsing, status management, prompt listing
- pkg/executor: Docker claude-yolo container execution in attached mode
- pkg/git: commit, CHANGELOG update, semver tagging, push
- pkg/factory: main loop orchestrating prompt → execute → release cycle
