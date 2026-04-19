---
status: committing
spec: [055-preflight-baseline-check]
summary: Created pkg/preflight package with Docker-backed Checker, SHA-keyed cache, notification on failure, and 81.8% test coverage
container: dark-factory-320-spec-055-preflight-package
dark-factory-version: v0.125.1
created: "2026-04-19T12:00:00Z"
queued: "2026-04-19T16:15:50Z"
started: "2026-04-19T16:22:30Z"
branch: dark-factory/preflight-baseline-check
---

<summary>
- A new `pkg/preflight/` package provides the `Checker` interface and its Docker-backed implementation
- `Checker.Check()` returns true when the baseline is green, false when broken or preflight is disabled
- When `preflightCommand` is empty, `Check()` immediately returns true (disabled path — zero container overhead)
- Results are cached per git HEAD commit SHA: the same SHA within `preflightInterval` reuses the cached result without re-running the container
- When the cached SHA advances or the interval elapses, preflight re-runs
- The preflight command runs inside the same Docker container image used for YOLO prompt execution (`containerImage`), so toolchain and dependencies match
- Extra volume mounts configured in `extraMounts` are applied to the preflight container using the same resolution logic as the YOLO executor
- On failure, a structured error log records the command, captured output, and git SHA; a notification is sent with event type `"preflight_failed"`
- Any Docker execution error (image pull failure, non-zero exit, timeout) is treated as preflight failure — not propagated as a Go error
- `go generate ./pkg/preflight/...` produces `mocks/preflight-checker.go`
- `make precommit` passes
</summary>

<objective>
Implement `pkg/preflight/` — the core preflight baseline-check package. It provides a `Checker` interface backed by a Docker container runner, an in-memory SHA-keyed cache, and a notification hook for baseline failures. This prompt does not wire the checker into the processor or factory — that is prompt 3.

**Precondition:** Prompt 1 has been executed. `Config.PreflightCommand`, `Config.PreflightInterval`, and `Config.ParsedPreflightInterval()` are all in place.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`

Key files to read before editing:
- `pkg/executor/executor.go` — `buildDockerCommand` (line ~473) and `resolveExtraMountSrc` (line ~645): understand how extraMounts are resolved; copy `resolveExtraMountSrc` into the preflight package (do NOT export it from executor — that refactor is out of scope)
- `pkg/notifier/notifier.go` — `Event` struct and the `EventType` comment
- `pkg/config/config.go` — `ExtraMount` struct and `IsReadonly()` method
- `pkg/git/root.go` — pattern for running `git rev-parse` commands
- Existing package with counterfeiter: `pkg/processor/dirty.go` (simple interface + counterfeiter annotation pattern)
</context>

<requirements>

## 1. Create `pkg/preflight/doc.go`

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package preflight provides the baseline check that runs before each prompt execution.
// It verifies that the project's CI command passes on the clean main-branch tree,
// caching results per git commit SHA to avoid redundant container runs.
package preflight
```

## 2. Create `pkg/preflight/preflight.go`

The complete file (adapt imports to what the compiler requires):

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package preflight

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/notifier"
)

//counterfeiter:generate -o ../../mocks/preflight-checker.go --fake-name PreflightChecker . Checker

// Checker verifies the project baseline before each prompt execution.
type Checker interface {
	// Check returns true when the baseline is green and the prompt may proceed.
	// Returns false when the baseline is broken or preflight is disabled (empty command).
	// Docker execution errors and non-zero exit codes are both treated as "broken baseline":
	// they are logged and cause false to be returned, never propagated as Go errors.
	Check(ctx context.Context) (bool, error)
}

// cacheEntry stores the result of the last preflight run.
type cacheEntry struct {
	sha       string
	checkedAt time.Time
	ok        bool
	output    string
}

// checker implements Checker.
type checker struct {
	command        string
	interval       time.Duration
	projectRoot    string
	containerImage string
	extraMounts    []config.ExtraMount
	notifier       notifier.Notifier
	projectName    string
	cache          *cacheEntry // nil until the first run; in-memory only (lost on restart)
}

// NewChecker creates a new preflight Checker.
// command is the shell command to run (empty string disables preflight).
// interval is how long a cached green result is valid for the same git SHA (0 disables caching).
// projectRoot is the absolute path of the project directory mounted as /workspace.
// containerImage is the Docker image to use (same as the YOLO executor).
// extraMounts are additional volume mounts applied to the preflight container.
// n is used to notify humans when the baseline is broken.
func NewChecker(
	command string,
	interval time.Duration,
	projectRoot string,
	containerImage string,
	extraMounts []config.ExtraMount,
	n notifier.Notifier,
	projectName string,
) Checker {
	return &checker{
		command:        command,
		interval:       interval,
		projectRoot:    projectRoot,
		containerImage: containerImage,
		extraMounts:    extraMounts,
		notifier:       n,
		projectName:    projectName,
	}
}

// Check verifies the project baseline before prompt execution.
func (c *checker) Check(ctx context.Context) (bool, error) {
	if c.command == "" {
		return true, nil
	}

	sha, err := c.getHeadSHA(ctx)
	if err != nil {
		slog.Warn("preflight: could not get HEAD SHA, skipping cache", "error", err)
		sha = "" // treat as cache miss but still run
	}

	// Cache hit: same SHA and within interval
	if c.cache != nil && sha != "" && c.cache.sha == sha &&
		c.interval > 0 && time.Since(c.cache.checkedAt) < c.interval {
		slog.Debug("preflight: cache hit", "sha", sha[:minLen(sha, 12)], "ok", c.cache.ok)
		return c.cache.ok, nil
	}

	slog.Info("preflight: running baseline check", "command", c.command, "sha", truncateSHA(sha))

	output, runErr := c.runInContainer(ctx)
	ok := runErr == nil

	c.cache = &cacheEntry{
		sha:       sha,
		checkedAt: time.Now(),
		ok:        ok,
		output:    output,
	}

	if ok {
		slog.Info("preflight: baseline check passed", "sha", truncateSHA(sha))
		return true, nil
	}

	slog.Error("preflight: baseline check FAILED — prompts will not start until baseline is fixed",
		"command", c.command,
		"sha", truncateSHA(sha),
		"output", output,
		"error", runErr,
	)
	_ = c.notifier.Notify(ctx, notifier.Event{
		ProjectName: c.projectName,
		EventType:   "preflight_failed",
	})
	return false, nil
}

// getHeadSHA returns the current HEAD commit SHA using git rev-parse.
func (c *checker) getHeadSHA(ctx context.Context) (string, error) {
	// #nosec G204 -- fixed args; projectRoot is from trusted config
	cmd := exec.CommandContext(ctx, "git", "-C", c.projectRoot, "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", errors.Wrap(ctx, err, "git rev-parse HEAD")
	}
	return strings.TrimSpace(string(output)), nil
}

// runInContainer executes the preflight command inside a Docker container.
// Returns combined stdout+stderr output and nil on success, or output + error on failure.
// Non-zero exit code and Docker-level errors are both returned as errors.
func (c *checker) runInContainer(ctx context.Context) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errors.Wrap(ctx, err, "get home dir")
	}

	args := []string{
		"run", "--rm",
		"-v", c.projectRoot + ":/workspace",
		"-w", "/workspace",
	}

	// Apply extra mounts with the same resolution logic as the YOLO executor
	for _, m := range c.extraMounts {
		src := resolveExtraMountSrc(m.Src, os.Getenv, runtime.GOOS)
		if strings.HasPrefix(src, "~/") {
			src = home + src[1:]
		} else if !filepath.IsAbs(src) {
			src = filepath.Join(c.projectRoot, src)
		}
		if _, err := os.Stat(src); err != nil {
			slog.Warn("preflight: extraMounts src does not exist, skipping", "src", src, "dst", m.Dst)
			continue
		}
		mount := src + ":" + m.Dst
		if m.IsReadonly() {
			mount += ":ro"
		}
		args = append(args, "-v", mount)
	}

	args = append(args, c.containerImage)
	args = append(args, "sh", "-c", c.command)

	// #nosec G204 -- containerImage and command are from trusted project config
	cmd := exec.CommandContext(ctx, "docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), errors.Wrap(ctx, err, "preflight container exited non-zero")
	}
	return string(output), nil
}

// resolveExtraMountSrc expands environment variables and ${HOST_CACHE_DIR} in src.
// Copied from pkg/executor/executor.go — kept private to avoid coupling.
func resolveExtraMountSrc(src string, lookupEnv func(string) string, goos string) string {
	// Default HOST_CACHE_DIR if unset
	if strings.Contains(src, "HOST_CACHE_DIR") && lookupEnv("HOST_CACHE_DIR") == "" {
		var cacheDir string
		if goos == "darwin" {
			if home := lookupEnv("HOME"); home != "" {
				cacheDir = home + "/Library/Caches"
			}
		} else {
			if xdg := lookupEnv("XDG_CACHE_HOME"); xdg != "" {
				cacheDir = xdg
			} else if home := lookupEnv("HOME"); home != "" {
				cacheDir = home + "/.cache"
			}
		}
		if cacheDir != "" {
			src = strings.ReplaceAll(src, "$HOST_CACHE_DIR", cacheDir)
			src = strings.ReplaceAll(src, "${HOST_CACHE_DIR}", cacheDir)
		}
	}
	// Expand remaining ${VAR} and $VAR references
	return os.Expand(src, lookupEnv)
}

// truncateSHA returns the first 12 characters of sha for logging, or the full sha if shorter.
func truncateSHA(sha string) string {
	return sha[:minLen(sha, 12)]
}

// minLen returns the minimum of len(s) and n.
func minLen(s string, n int) int {
	if len(s) < n {
		return len(s)
	}
	return n
}
```

**Important notes about `resolveExtraMountSrc`:**
- Read the actual implementation in `pkg/executor/executor.go` at line ~645 BEFORE writing this function
- Copy it faithfully — do NOT simplify or alter behavior
- The copy is intentional: it avoids an import cycle and keeps the preflight package self-contained

## 3. Update `pkg/notifier/notifier.go`

In the `Event` struct, update the `EventType` comment to include `"preflight_failed"`:

```go
EventType   string // "prompt_failed", "prompt_partial", "spec_verifying", "review_limit", "stuck_container", "preflight_failed"
```

## 4. Create `pkg/preflight/preflight_suite_test.go`

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package preflight_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPreflight(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Preflight Suite")
}
```

## 5. Create `pkg/preflight/preflight_test.go`

Test the `checker` logic using a fake Notifier (from `mocks`). Since the Docker runner and git SHA getter require real infrastructure, test the pure logic paths only (disabled, cache hit, cache miss, notification emitted). Use a test-friendly constructor that accepts injected dependencies.

**Strategy:** Rather than testing `runInContainer` directly (requires Docker), expose the cache and test the branching logic. Define a `NewCheckerForTest` helper in `pkg/preflight/export_test.go` that allows injecting a fake runner function.

### `pkg/preflight/export_test.go`

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package preflight

// NewCheckerWithRunner creates a Checker for testing, replacing runInContainer with fn.
func NewCheckerWithRunner(
	command string,
	interval time.Duration,
	n notifier.Notifier,
	projectName string,
	headSHA string,
	runner func(ctx context.Context) (string, error),
) Checker {
	return &checkerWithFakeRunner{
		command:     command,
		interval:    interval,
		n:           n,
		projectName: projectName,
		headSHA:     headSHA,
		runner:      runner,
	}
}

// checkerWithFakeRunner wraps checker logic with an injected runner and SHA supplier.
type checkerWithFakeRunner struct {
	command     string
	interval    time.Duration
	n           notifier.Notifier
	projectName string
	headSHA     string
	runner      func(ctx context.Context) (string, error)
	cache       *cacheEntry
}

func (c *checkerWithFakeRunner) Check(ctx context.Context) (bool, error) {
	if c.command == "" {
		return true, nil
	}
	sha := c.headSHA

	if c.cache != nil && sha != "" && c.cache.sha == sha &&
		c.interval > 0 && time.Since(c.cache.checkedAt) < c.interval {
		return c.cache.ok, nil
	}

	output, runErr := c.runner(ctx)
	ok := runErr == nil

	c.cache = &cacheEntry{
		sha:       sha,
		checkedAt: time.Now(),
		ok:        ok,
		output:    output,
	}

	if ok {
		return true, nil
	}

	slog.Error("preflight: baseline check FAILED",
		"command", c.command,
		"sha", truncateSHA(sha),
		"output", output,
		"error", runErr,
	)
	_ = c.n.Notify(ctx, notifier.Event{
		ProjectName: c.projectName,
		EventType:   "preflight_failed",
	})
	return false, nil
}
```

Note: `export_test.go` is in `package preflight` (not `package preflight_test`) so it can access unexported types. Add necessary imports (`"context"`, `"log/slog"`, `"time"`, the notifier package).

### `pkg/preflight/preflight_test.go`

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package preflight_test

import (
	"context"

	"github.com/bborbe/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/notifier"
	"github.com/bborbe/dark-factory/pkg/preflight"
)

var _ = Describe("Checker", func() {
	var (
		ctx          context.Context
		fakeNotifier *mocks.Notifier
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeNotifier = &mocks.Notifier{}
		fakeNotifier.NotifyReturns(nil)
	})

	Describe("disabled (empty command)", func() {
		It("returns true without calling the runner", func() {
			runnerCalled := false
			ch := preflight.NewCheckerWithRunner("", 0, fakeNotifier, "proj", "abc123",
				func(ctx context.Context) (string, error) {
					runnerCalled = true
					return "", nil
				},
			)
			ok, err := ch.Check(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(runnerCalled).To(BeFalse())
			Expect(fakeNotifier.NotifyCallCount()).To(Equal(0))
		})
	})

	Describe("preflight passes", func() {
		It("returns true and does not notify", func() {
			ch := preflight.NewCheckerWithRunner("make precommit", 8*time.Hour, fakeNotifier, "proj", "sha1",
				func(ctx context.Context) (string, error) { return "ok output", nil },
			)
			ok, err := ch.Check(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(fakeNotifier.NotifyCallCount()).To(Equal(0))
		})
	})

	Describe("preflight fails", func() {
		It("returns false and sends preflight_failed notification", func() {
			ch := preflight.NewCheckerWithRunner("make precommit", 8*time.Hour, fakeNotifier, "myproject", "sha2",
				func(ctx context.Context) (string, error) {
					return "lint error on line 42", errors.New(ctx, "exit status 1")
				},
			)
			ok, err := ch.Check(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeFalse())
			Expect(fakeNotifier.NotifyCallCount()).To(Equal(1))
			event := fakeNotifier.NotifyArgsForCall(0)
			Expect(event.EventType).To(Equal("preflight_failed"))
			Expect(event.ProjectName).To(Equal("myproject"))
		})
	})

	Describe("caching", func() {
		It("reuses cached result within interval for same SHA", func() {
			callCount := 0
			ch := preflight.NewCheckerWithRunner("make precommit", 1*time.Hour, fakeNotifier, "proj", "sha3",
				func(ctx context.Context) (string, error) {
					callCount++
					return "ok", nil
				},
			)
			ok1, _ := ch.Check(ctx)
			ok2, _ := ch.Check(ctx)
			Expect(ok1).To(BeTrue())
			Expect(ok2).To(BeTrue())
			Expect(callCount).To(Equal(1), "runner should be called only once due to cache")
		})

		It("re-runs when SHA changes", func() {
			callCount := 0
			// First check with sha-A
			ch1 := preflight.NewCheckerWithRunner("make precommit", 1*time.Hour, fakeNotifier, "proj", "sha-A",
				func(ctx context.Context) (string, error) {
					callCount++
					return "ok", nil
				},
			)
			_, _ = ch1.Check(ctx)
			// Second check with a different SHA (simulated by a separate checker instance)
			ch2 := preflight.NewCheckerWithRunner("make precommit", 1*time.Hour, fakeNotifier, "proj", "sha-B",
				func(ctx context.Context) (string, error) {
					callCount++
					return "ok", nil
				},
			)
			_, _ = ch2.Check(ctx)
			Expect(callCount).To(Equal(2), "runner should re-run when SHA changes")
		})

		It("re-runs when interval is zero (no caching)", func() {
			callCount := 0
			ch := preflight.NewCheckerWithRunner("make precommit", 0, fakeNotifier, "proj", "sha4",
				func(ctx context.Context) (string, error) {
					callCount++
					return "ok", nil
				},
			)
			_, _ = ch.Check(ctx)
			_, _ = ch.Check(ctx)
			Expect(callCount).To(Equal(2), "runner should always re-run when interval is 0")
		})
	})
})
```

Note: add `"time"` to the import block in the test file (for `time.Hour`).

## 6. Generate mocks

After creating the package and the counterfeiter annotation, run:

```bash
cd /workspace && go generate ./pkg/preflight/...
```

This creates `mocks/preflight-checker.go`. Verify it compiles:

```bash
cd /workspace && go build ./mocks/...
```

## 7. Write CHANGELOG entry

Add or extend `## Unreleased` at the top of `CHANGELOG.md`:

```
- feat: add `pkg/preflight` package — Docker-backed baseline checker with SHA-keyed in-memory cache and notification on failure
```

## 8. Run `make test`

```bash
cd /workspace && make test
```

Must pass before `make precommit`.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do not touch `go.mod` / `go.sum` / `vendor/`
- `Check()` must NEVER return a non-nil error for Docker failures (non-zero exit, image pull failure, timeout) — treat all container errors as "baseline broken", log them, and return `(false, nil)`
- `Check()` may return a non-nil error only for truly unexpected problems (e.g. if a future caller adds logic that can fail) — for this implementation, always return `nil` error
- The cache is in-memory only (lost on daemon restart) — this is intentional per the spec: "Daemon restarts: cache is in-memory; next run re-checks baseline. Acceptable."
- `resolveExtraMountSrc` must be copied faithfully from `pkg/executor/executor.go` — read the actual implementation before writing
- External test package (`package preflight_test`) for test files; `export_test.go` uses `package preflight`
- The counterfeiter annotation uses the relative output path `../../mocks/preflight-checker.go`
- Use `errors.Wrap` from `github.com/bborbe/errors` for all error wrapping (no `fmt.Errorf`)
- Do NOT import `pkg/processor` from `pkg/preflight` — that would create an import cycle
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Spot checks:
1. `ls pkg/preflight/` — shows `doc.go`, `preflight.go`, `export_test.go`, `preflight_suite_test.go`, `preflight_test.go`
2. `ls mocks/preflight-checker.go` — file exists
3. `grep -n "preflight_failed" pkg/notifier/notifier.go` — 1 match
4. `go test ./pkg/preflight/...` — passes
5. `go build ./pkg/preflight/... ./mocks/...` — no errors
</verification>
