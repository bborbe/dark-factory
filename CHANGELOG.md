# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
