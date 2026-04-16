---
status: completed
spec: [052-split-prompt-manager]
summary: Defined narrow per-consumer PromptManager interfaces in seven packages (processor, runner, server, status, review, watcher, cmd) and generated counterfeiter fakes for each in the mocks directory without changing any existing code.
container: dark-factory-304-spec-052-consumer-interfaces
dark-factory-version: v0.111.2
created: "2026-04-16T19:53:47Z"
queued: "2026-04-16T21:01:36Z"
started: "2026-04-16T21:10:47Z"
completed: "2026-04-16T21:16:39Z"
---

<summary>
- Seven consumer packages each gain a `prompt_manager.go` file declaring a narrow `PromptManager` interface containing only the methods that consumer actually calls
- A counterfeiter fake is generated per consumer (e.g., `mocks/processor-prompt-manager.go` with type `ProcessorPromptManager`)
- Consumer constructors and struct fields are NOT changed yet — all existing code continues to compile against the wide `prompt.Manager` interface
- The seven consumers and their method counts: processor (9), runner (3 covering both runner+oneshot), server (1), status (4), review (5), watcher (1), cmd (2)
- `make test` passes without any behavioral change
</summary>

<objective>
Define narrow per-consumer `PromptManager` interfaces in each of the seven consumer packages (`pkg/processor`, `pkg/runner`, `pkg/server`, `pkg/status`, `pkg/review`, `pkg/watcher`, `pkg/cmd`) and generate counterfeiter fakes for each. This is the foundation step before switching constructors to use the narrow interfaces. No existing code changes in this prompt — only new interface files and generated fakes.
</objective>

<context>
Read `CLAUDE.md` for project conventions (errors, Ginkgo/Gomega, Counterfeiter).
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-composition.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`

Key files to read before implementing:
- `pkg/prompt/prompt.go` — the full `Manager` interface (lines ~479–505) and types `Prompt`, `PromptFile`, `Frontmatter`, `Rename`
- `pkg/processor/processor.go` — to confirm all `p.promptManager.*` call sites
- `pkg/runner/runner.go`, `pkg/runner/oneshot.go`, `pkg/runner/lifecycle.go`, `pkg/runner/health_check.go` — to confirm runner+oneshot method usage (both live in the same `runner` package; the interface covers both)
- `pkg/server/queue_helpers.go`, `pkg/server/queue_action_handler.go` — server method usage
- `pkg/status/status.go` — status method usage
- `pkg/review/poller.go` — review method usage
- `pkg/watcher/watcher.go` — watcher method usage
- `pkg/cmd/approve.go`, `pkg/cmd/unapprove.go`, `pkg/cmd/prompt_complete.go` — cmd method usage
- `pkg/processor/dirty.go` — counterfeiter directive format used in this repo
- `mocks/mocks.go` — how the mocks package is set up
</context>

<requirements>

## 1. Verify method usage before writing interfaces

Before creating any interface file, run these greps to confirm the exact methods each consumer calls. Do NOT rely on memory — read the output:

```bash
grep -n "\.promptManager\.\|\.promptMgr\." pkg/processor/processor.go
grep -n "promptManager\." pkg/runner/runner.go pkg/runner/oneshot.go pkg/runner/lifecycle.go pkg/runner/health_check.go pkg/runner/export_test.go
grep -n "promptManager\." pkg/server/queue_helpers.go pkg/server/queue_action_handler.go
grep -n "promptMgr\." pkg/status/status.go
grep -n "\.promptManager\." pkg/review/poller.go
grep -n "\.promptManager\." pkg/watcher/watcher.go
grep -n "\.promptManager\." pkg/cmd/approve.go pkg/cmd/unapprove.go pkg/cmd/prompt_complete.go
```

Record the distinct method names per package from the grep output and use those as the definitive interface contents.

## 2. Create `pkg/processor/prompt_manager.go`

The processor calls: `ListQueued`, `Load`, `AllPreviousCompleted`, `FindMissingCompleted`, `FindPromptStatusInProgress`, `SetStatus`, `MoveToCompleted`, `HasQueuedPromptsOnBranch`, `SetPRURL`.

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor

import (
	"context"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

//counterfeiter:generate -o ../../mocks/processor-prompt-manager.go --fake-name ProcessorPromptManager . PromptManager

// PromptManager is the subset of prompt.Manager that the processor package uses.
type PromptManager interface {
	ListQueued(ctx context.Context) ([]prompt.Prompt, error)
	Load(ctx context.Context, path string) (*prompt.PromptFile, error)
	AllPreviousCompleted(ctx context.Context, n int) bool
	FindMissingCompleted(ctx context.Context, n int) []int
	FindPromptStatusInProgress(ctx context.Context, number int) string
	SetStatus(ctx context.Context, path string, status string) error
	MoveToCompleted(ctx context.Context, path string) error
	HasQueuedPromptsOnBranch(ctx context.Context, branch string, excludePath string) (bool, error)
	SetPRURL(ctx context.Context, path string, url string) error
}
```

**Important:** Use standard Go import block formatting. The license header must match the repo convention (read any existing `.go` file in `pkg/processor/` for the exact header text and year).

## 3. Create `pkg/runner/prompt_manager.go`

The `runner` package contains both `runner` (daemon) and `oneShotRunner`. Their combined method usage:
- `runner` (daemon): `NormalizeFilenames` (via `normalizeFilenames` helper), `Load` (via `resumeOrResetExecuting` and `checkExecutingPrompts` health check helpers)
- `oneShotRunner`: `ListQueued` (direct), `NormalizeFilenames` (direct), `Load` (via `resumeOrResetExecuting` helper)

The package-level helper functions in `lifecycle.go` (`normalizeFilenames`, `resumeOrResetExecuting`) and `health_check.go` (`runHealthCheckLoop`, `checkExecutingPrompts`) currently accept `prompt.Manager`. After this refactoring those helpers will accept this local `PromptManager`. The interface must therefore cover all methods called from any helper in the package.

Combined interface: `NormalizeFilenames`, `Load`, `ListQueued`.

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runner

import (
	"context"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

//counterfeiter:generate -o ../../mocks/runner-prompt-manager.go --fake-name RunnerPromptManager . PromptManager

// PromptManager is the subset of prompt.Manager that the runner package uses.
type PromptManager interface {
	NormalizeFilenames(ctx context.Context, dir string) ([]prompt.Rename, error)
	Load(ctx context.Context, path string) (*prompt.PromptFile, error)
	ListQueued(ctx context.Context) ([]prompt.Prompt, error)
}
```

## 4. Create `pkg/server/prompt_manager.go`

The server package calls only `NormalizeFilenames` through the prompt manager.

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

//counterfeiter:generate -o ../../mocks/server-prompt-manager.go --fake-name ServerPromptManager . PromptManager

// PromptManager is the subset of prompt.Manager that the server package uses.
type PromptManager interface {
	NormalizeFilenames(ctx context.Context, dir string) ([]prompt.Rename, error)
}
```

## 5. Create `pkg/status/prompt_manager.go`

The status package calls: `ListQueued`, `Title`, `ReadFrontmatter`, `HasExecuting`.

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package status

import (
	"context"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

//counterfeiter:generate -o ../../mocks/status-prompt-manager.go --fake-name StatusPromptManager . PromptManager

// PromptManager is the subset of prompt.Manager that the status package uses.
type PromptManager interface {
	ListQueued(ctx context.Context) ([]prompt.Prompt, error)
	Title(ctx context.Context, path string) (string, error)
	ReadFrontmatter(ctx context.Context, path string) (*prompt.Frontmatter, error)
	HasExecuting(ctx context.Context) bool
}
```

## 6. Create `pkg/review/prompt_manager.go`

The review package calls: `ReadFrontmatter`, `Load`, `MoveToCompleted`, `SetStatus`, `IncrementRetryCount`.

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package review

import (
	"context"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

//counterfeiter:generate -o ../../mocks/review-prompt-manager.go --fake-name ReviewPromptManager . PromptManager

// PromptManager is the subset of prompt.Manager that the review package uses.
type PromptManager interface {
	ReadFrontmatter(ctx context.Context, path string) (*prompt.Frontmatter, error)
	Load(ctx context.Context, path string) (*prompt.PromptFile, error)
	MoveToCompleted(ctx context.Context, path string) error
	SetStatus(ctx context.Context, path string, status string) error
	IncrementRetryCount(ctx context.Context, path string) error
}
```

## 7. Create `pkg/watcher/prompt_manager.go`

The watcher package calls only `NormalizeFilenames`.

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package watcher

import (
	"context"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

//counterfeiter:generate -o ../../mocks/watcher-prompt-manager.go --fake-name WatcherPromptManager . PromptManager

// PromptManager is the subset of prompt.Manager that the watcher package uses.
type PromptManager interface {
	NormalizeFilenames(ctx context.Context, dir string) ([]prompt.Rename, error)
}
```

## 8. Create `pkg/cmd/prompt_manager.go`

The cmd package uses:
- `approve.go`: `NormalizeFilenames`
- `unapprove.go`: `NormalizeFilenames`
- `prompt_complete.go`: `MoveToCompleted`

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"context"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

//counterfeiter:generate -o ../../mocks/cmd-prompt-manager.go --fake-name CmdPromptManager . PromptManager

// PromptManager is the subset of prompt.Manager that the cmd package uses.
type PromptManager interface {
	NormalizeFilenames(ctx context.Context, dir string) ([]prompt.Rename, error)
	MoveToCompleted(ctx context.Context, path string) error
}
```

## 9. Run `go generate` to create all fakes

Run counterfeiter for all seven packages:

```bash
cd /workspace
go generate ./pkg/processor/... ./pkg/runner/... ./pkg/server/... ./pkg/status/... ./pkg/review/... ./pkg/watcher/... ./pkg/cmd/...
```

If `go generate` fails for a package, read the error and fix the interface definition before retrying. Common causes: wrong import path for prompt types, syntax error in the interface file.

After generation, verify the expected fake files exist:

```bash
ls -la mocks/processor-prompt-manager.go mocks/runner-prompt-manager.go mocks/server-prompt-manager.go mocks/status-prompt-manager.go mocks/review-prompt-manager.go mocks/watcher-prompt-manager.go mocks/cmd-prompt-manager.go
```

## 10. Run `make test`

```bash
cd /workspace && make test
```

All existing tests must pass. No behavioral change has been made — only new interface files and generated fakes have been added.

</requirements>

<constraints>
- Do NOT change any existing constructor signatures, struct field types, or test files in this prompt
- The existing `prompt.Manager` interface in `pkg/prompt/prompt.go` must remain untouched — it is removed in a later prompt
- The existing `mocks/prompt-manager.go` must remain — it is deleted in a later prompt
- Do NOT commit — dark-factory handles git
- All new interface files must use the exact license header from adjacent files in the same package
- Counterfeiter directives must point to `../../mocks/<name>.go` relative to the package directory — same pattern as `pkg/processor/dirty.go`
- The `--fake-name` must use the package-prefixed naming convention: `ProcessorPromptManager`, `RunnerPromptManager`, `ServerPromptManager`, `StatusPromptManager`, `ReviewPromptManager`, `WatcherPromptManager`, `CmdPromptManager`
- Do not touch `go.mod` / `go.sum` / `vendor/`
- Interface method signatures must exactly match the concrete `*manager` method signatures in `pkg/prompt/prompt.go` — use grep to verify before writing
</constraints>

<verification>
Run `make test` in `/workspace` — must pass.

Additional spot checks:
1. `ls mocks/*-prompt-manager.go` — seven new fake files exist
2. `grep -c "func (f \*ProcessorPromptManager)" mocks/processor-prompt-manager.go` — should show 9 methods
3. `grep -c "func (f \*RunnerPromptManager)" mocks/runner-prompt-manager.go` — should show 3 methods
4. `grep -c "func (f \*ServerPromptManager)" mocks/server-prompt-manager.go` — should show 1 method
5. `grep -c "func (f \*StatusPromptManager)" mocks/status-prompt-manager.go` — should show 4 methods
6. `grep -c "func (f \*ReviewPromptManager)" mocks/review-prompt-manager.go` — should show 5 methods
7. `grep -c "func (f \*WatcherPromptManager)" mocks/watcher-prompt-manager.go` — should show 1 method
8. `grep -c "func (f \*CmdPromptManager)" mocks/cmd-prompt-manager.go` — should show 2 methods
</verification>
