---
status: completed
summary: Changed Go module cache mount from read-only (:ro) to read-write in executor.go and updated test expectation to match.
container: dark-factory-204-mount-go-cache-readwrite
dark-factory-version: v0.57.3
created: "2026-03-16T16:08:47Z"
queued: "2026-03-16T16:08:47Z"
started: "2026-03-16T16:08:56Z"
completed: "2026-03-16T16:14:55Z"
---

<summary>
- Go module cache mount changed from read-only to read-write
- Containers can download new Go modules during execution
- Fixes `go mod tidy` failures in projects needing uncached dependencies
- Test expectations updated to match new mount mode
</summary>

<objective>
Change the Go module cache Docker volume mount from read-only back to read-write so that containers can download Go modules not yet in the host cache.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes (read ALL of these first):
- `pkg/executor/executor.go` — line ~295 has `"-v", home+"/go/pkg:/home/node/go/pkg:ro"`, line ~137 has the debug log with `:ro`
- `pkg/executor/executor_internal_test.go` — line ~280 has test expecting `:ro` in mount string
</context>

<constraints>
- Only change the mount mode from `:ro` to read-write — nothing else about the Docker command
- Update both the actual mount AND the debug log message
- Update test expectations to match
</constraints>

<requirements>

## 1. Update mount in `pkg/executor/executor.go`

Change the volume mount from read-only to read-write:
- `"-v", home+"/go/pkg:/home/node/go/pkg:ro"` → `"-v", home+"/go/pkg:/home/node/go/pkg"`

## 2. Update debug log in `pkg/executor/executor.go`

Change the log message to match:
- `"goPkgMount", home+"/go/pkg:/home/node/go/pkg:ro"` → `"goPkgMount", home+"/go/pkg:/home/node/go/pkg"`

## 3. Update test in `pkg/executor/executor_internal_test.go`

Update the test expectation:
- `"/home/user/go/pkg:/home/node/go/pkg:ro"` → `"/home/user/go/pkg:/home/node/go/pkg"`

</requirements>

<verification>
make precommit
</verification>
