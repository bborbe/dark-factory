---
status: completed
spec: [038-global-container-limit]
summary: 'Added ContainerCount/ContainerMax to Status struct, wired ContainerCounter into NewChecker, rendered Containers: N/M (system-wide) in formatter between Project and Daemon lines, updated factory wiring with global config load, and added tests for all new behavior'
container: dark-factory-234-spec-038-status-display
dark-factory-version: v0.80.0-1-g2b37ac1
created: "2026-03-31T18:02:00Z"
queued: "2026-03-31T19:11:44Z"
started: "2026-03-31T20:21:18Z"
completed: "2026-03-31T20:32:27Z"
branch: dark-factory/global-container-limit
---

<summary>
- Status output shows how many containers are running system-wide vs the configured limit
- Container line appears between project and daemon lines
- When no limit is configured, the container line is hidden
- Global config load errors are non-fatal for status — defaults apply with a warning
- All existing status tests continue to pass
</summary>

<objective>
Surface the system-wide container count in `dark-factory status` output. Add `ContainerCount` and `ContainerMax` to the `Status` struct, populate them via a `ContainerCounter`, and render them in the status formatter as `Containers: N/M (system-wide)`.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `/home/node/.claude/docs/go-patterns.md` — interface→constructor→struct, counterfeiter annotations.
Read `/home/node/.claude/docs/go-testing.md` — Ginkgo/Gomega, external test packages, ≥80% coverage.
Read `pkg/status/status.go` — `Status` struct (lines 26–41), `Checker` interface (lines 57–63), `checker` struct (lines 65–75), `NewChecker` constructor (lines 77–95), `GetStatus` method (lines 97–141).
Read `pkg/status/formatter.go` — `Format` method (lines 30–79), output layout to understand where to insert the container line.
Read `pkg/status/status_test.go` and `pkg/status/formatter_test.go` — existing test patterns.
Read `pkg/executor/checker.go` — `ContainerCounter` interface added in the previous prompt.
Read `pkg/factory/factory.go` — find `CreateCombinedStatusCommand` and `CreateStatusCommand` factory functions (search for "Status" in factory.go) to understand how `NewChecker` is wired.
Read `pkg/globalconfig/globalconfig.go` — `GlobalConfig` struct and `NewLoader()` created in the first prompt.
</context>

<requirements>
1. Update `pkg/status/status.go` — add two fields to the `Status` struct:
   ```go
   ContainerCount int `json:"container_count,omitempty"`
   ContainerMax   int `json:"container_max,omitempty"`
   ```
   Place them after the `CompletedCount` field.

2. Update the `checker` struct in `pkg/status/status.go` — add two new fields:
   ```go
   containerCounter executor.ContainerCounter
   maxContainers    int
   ```

3. Update `NewChecker` in `pkg/status/status.go` — add two new parameters at the end:
   ```go
   containerCounter executor.ContainerCounter,
   maxContainers int,
   ```
   Wire them into the struct.

   Add import `"github.com/bborbe/dark-factory/pkg/executor"` to `pkg/status/status.go`.

4. Add container count population to `GetStatus` in `pkg/status/status.go`.

   After the `populateLogInfo` call, add:
   ```go
   // Populate system-wide container count
   if s.containerCounter != nil && s.maxContainers > 0 {
       count, err := s.containerCounter.CountRunning(ctx)
       if err != nil {
           slog.Debug("failed to count running containers for status", "error", err)
       } else {
           status.ContainerCount = count
           status.ContainerMax = s.maxContainers
       }
   }
   ```

5. Update `pkg/status/formatter.go` — update `Format` to render the container line.

   In the `Format` method, after the `Project:` line and before the `Daemon:` line, add:
   ```go
   // Container count (only when limit is configured)
   if st.ContainerMax > 0 {
       fmt.Fprintf(&b, "  Containers: %d/%d (system-wide)\n", st.ContainerCount, st.ContainerMax)
   }
   ```

   The block around `Project:` currently looks like:
   ```go
   if st.ProjectDir != "" {
       fmt.Fprintf(&b, "  Project:    %s\n", st.ProjectDir)
   }

   // Daemon status
   if st.DaemonPID > 0 {
   ```

   Insert the new container block between `Project:` and `Daemon:`.

6. Update factory wiring — find `CreateCombinedStatusCommand` and any other factory functions that call `status.NewChecker(...)` in `pkg/factory/factory.go`. Add the two new parameters:

   Read the factory file to locate all calls to `status.NewChecker(...)`. For each:
   a. Load global config before the call:
      ```go
      globalCfg, err := globalconfig.NewLoader().Load(context.Background())
      if err != nil {
          slog.Warn("globalconfig load failed for status, using default", "error", err)
          globalCfg = globalconfig.GlobalConfig{MaxContainers: globalconfig.DefaultMaxContainers}
      }
      ```
      (Status is a read-only command — a bad global config should not prevent `dark-factory status` from running; log and use defaults.)

   b. Pass `executor.NewDockerContainerCounter()` and `globalCfg.MaxContainers` as new last params to `status.NewChecker(...)`.

   c. Add imports as needed:
      - `"github.com/bborbe/dark-factory/pkg/executor"` (may already be imported)
      - `"github.com/bborbe/dark-factory/pkg/globalconfig"`

7. Update tests — find all `status.NewChecker(...)` calls in test files and add the two new params. Use:
   - `nil` for `containerCounter` (or a counterfeiter fake)
   - `0` for `maxContainers` (disables container line in tests that don't need it)

8. Add tests for the new behavior:

   In `pkg/status/formatter_test.go` (or a new `pkg/status/format_test.go`), add:

   ```go
   Describe("Format container line", func() {
       It("includes container line when ContainerMax > 0", func() {
           f := status.NewFormatter()
           st := &status.Status{
               Daemon:         "not running",
               QueuedPrompts:  []string{},
               ContainerCount: 2,
               ContainerMax:   3,
           }
           output := f.Format(st)
           Expect(output).To(ContainSubstring("Containers: 2/3 (system-wide)"))
       })

       It("omits container line when ContainerMax is 0", func() {
           f := status.NewFormatter()
           st := &status.Status{
               Daemon:        "not running",
               QueuedPrompts: []string{},
               ContainerMax:  0,
           }
           output := f.Format(st)
           Expect(output).NotTo(ContainSubstring("Containers:"))
       })

       It("shows container line after Project and before Daemon", func() {
           f := status.NewFormatter()
           st := &status.Status{
               ProjectDir:     "/my/project",
               Daemon:         "running",
               DaemonPID:      1234,
               QueuedPrompts:  []string{},
               ContainerCount: 1,
               ContainerMax:   5,
           }
           output := f.Format(st)
           lines := strings.Split(output, "\n")
           var projectIdx, containerIdx, daemonIdx int
           for i, l := range lines {
               if strings.Contains(l, "Project:") {
                   projectIdx = i
               }
               if strings.Contains(l, "Containers:") {
                   containerIdx = i
               }
               if strings.Contains(l, "Daemon:") {
                   daemonIdx = i
               }
           }
           Expect(containerIdx).To(BeNumerically(">", projectIdx))
           Expect(containerIdx).To(BeNumerically("<", daemonIdx))
       })
   })
   ```

   In `pkg/status/status_test.go`, add a test for `GetStatus` container population:
   ```go
   Describe("GetStatus container count", func() {
       It("populates ContainerCount and ContainerMax when counter returns count", func() {
           // Use counterfeiter mock for ContainerCounter and Checker internals
           // Set up NewChecker with maxContainers=3 and a mock counter returning 2
           // Call GetStatus, verify status.ContainerCount == 2 and status.ContainerMax == 3
       })

       It("leaves ContainerCount zero when counter returns error", func() {
           // Mock counter returns an error
           // Verify status.ContainerCount == 0 (not populated on error)
       })

       It("leaves ContainerCount zero when maxContainers is 0", func() {
           // NewChecker with maxContainers=0
           // Verify ContainerCount is not populated
       })
   })
   ```

   Use the counterfeiter `mocks.ContainerCounter` fake from `mocks/container-counter.go` (generated in the previous prompt).

9. Update CHANGELOG.md — add to `## Unreleased` section:
   ```
   - feat: Show system-wide container count in `dark-factory status` output as `Containers: N/M (system-wide)`
   ```
</requirements>

<constraints>
- `Containers:` line is only shown when `ContainerMax > 0` — never show it when no limit is configured
- A `CountRunning` error during status check is not fatal — log at Debug level and omit the container line (leave ContainerCount at 0)
- For `dark-factory status` command, a global config load error is NOT fatal — log a warning and use `DefaultMaxContainers` so the command still runs
- `status.NewChecker` must not import `pkg/globalconfig` — the max value is passed as an int, keeping the package boundary clean
- `Containers:` line appears between `Project:` and `Daemon:` lines in the formatter output
- Error wrapping must use `errors.Wrap(ctx, err, ...)` from `github.com/bborbe/errors` — never `fmt.Errorf`
- All existing tests must continue to pass
- Do NOT commit — dark-factory handles git
</constraints>

<verification>
Run `make precommit` — must pass.

Additional checks:
```bash
# Confirm ContainerCount/ContainerMax fields in Status struct
grep -n "ContainerCount\|ContainerMax" pkg/status/status.go

# Confirm Containers line in formatter
grep -n "Containers:" pkg/status/formatter.go

# Run status package tests with coverage
go test -coverprofile=/tmp/cover.out -mod=vendor ./pkg/status/...
go tool cover -func=/tmp/cover.out | grep total
# Expected: ≥80%

# Confirm NewChecker signature updated
grep -n "func NewChecker" pkg/status/status.go

# Simulate status output with container info
# (manual: run dark-factory status while containers are active)

# Run all tests
make test
```
</verification>
