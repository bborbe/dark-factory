# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## v0.1.0

### Added
- Initial project structure from go-skeleton
- MVP main loop: scan prompts/ for queued prompts, execute via Docker, commit + tag + push
- pkg/prompt: YAML frontmatter parsing, status management, prompt listing
- pkg/executor: Docker claude-yolo container execution in attached mode
- pkg/git: commit, CHANGELOG update, semver tagging, push
- pkg/factory: main loop orchestrating prompt → execute → release cycle
