---
status: draft
created: "2026-03-11T20:22:56Z"
queued: "2026-03-11T20:22:56Z"
---

<summary>
- Go module cache mounted read-only in Docker containers
- Prevents malicious prompts from modifying the host module cache
- Container can still download modules to its own writable cache
- Existing prompt execution unchanged
</summary>

<objective>
Mount the host Go module cache as read-only in Docker containers to prevent prompt code from tampering with the shared module cache.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/executor/executor.go` — find the Docker volume mount for `go/pkg`. Look for the `-v` flag that mounts `home+"/go/pkg:/home/node/go/pkg"`.
</context>

<requirements>
1. In `pkg/executor/executor.go`, in the `buildDockerCommand` function (or wherever the Docker args are assembled), change the Go module cache mount from read-write to read-only:
   - Old: `"-v", home+"/go/pkg:/home/node/go/pkg"`
   - New: `"-v", home+"/go/pkg:/home/node/go/pkg:ro"`
2. Update `pkg/executor/executor_internal_test.go` if there is a test that checks the Docker command arguments — update the expected mount string to include `:ro`
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Only change the mount mode, nothing else about the Docker command
</constraints>

<verification>
Run `make precommit` -- must pass.
</verification>
