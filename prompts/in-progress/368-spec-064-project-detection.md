---
status: committing
spec: [064-cli-flexible-id-matching]
summary: Created pkg/project/root.go with FindRoot walk-up function, updated main.go to use project.FindRoot instead of git.ResolveGitRoot, added root_test.go with three Ginkgo tests, and added CHANGELOG Unreleased entries
container: dark-factory-368-spec-064-project-detection
dark-factory-version: v0.145.1-3-g93401a1
created: "2026-05-03T13:00:00Z"
queued: "2026-05-03T12:51:47Z"
started: "2026-05-03T13:05:12Z"
branch: dark-factory/cli-flexible-id-matching
---

<summary>
- Running `dark-factory spec list` from a subdirectory of a project (e.g. `pkg/config/`) now finds the project root via walk-up and shows specs correctly
- Running `dark-factory spec list` from `/tmp` (no `.dark-factory.yaml` anywhere up to `$HOME`) now exits non-zero with a clear "not a dark-factory project" error message
- The walk-up stops at `$HOME` — it never escapes to `/` or any ancestor of `$HOME`
- `dark-factory version` and `dark-factory help` continue to work from any directory (they exit before project detection)
- The silent "Specs: 0 total" output from non-project directories is replaced by an actionable error message
- A new `project.FindRoot` function encapsulates the walk-up logic in the existing `pkg/project/` package
- New unit tests in `pkg/project/root_test.go` cover the walk-up success, walk-up failure, and `$HOME` boundary cases
</summary>

<objective>
Replace `git.ResolveGitRoot(ctx)` in `main.go`'s `run()` function with a new `project.FindRoot(ctx)` function that walks up from the current working directory looking for `.dark-factory.yaml`, stopping at `$HOME`. When no project root is found, it returns a clear "not a dark-factory project" error. This eliminates the "silent zero" behavior where `spec list` and `spec status` return empty results when run outside a project directory.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`

Read these files in full before editing:
- `main.go` — full file; the git root resolution is at lines 76–83; note the `git` import must be removed if `git.ResolveGitRoot` was the only usage (grep for all `git.` usages in main.go first)
- `pkg/project/doc.go` — package declaration for `pkg/project`
- `pkg/project/name.go` — existing code in the package; the new `root.go` file must follow the same style
- `pkg/project/name_test.go` — read to understand the test file style and suite setup
- `pkg/project/project_suite_test.go` — suite file for the project package tests
- `pkg/git/root.go` — the function being replaced; read it to understand what it does; do NOT delete or modify it (other callers may exist)

The spec this implements: `specs/in-progress/064-cli-flexible-id-matching.md`
Precondition: prompt `1-spec-064-id-resolver.md` has been executed (ID resolver improvements are in place).
</context>

<requirements>

## 1. Create `pkg/project/root.go`

Create a new file `pkg/project/root.go` in the existing `pkg/project` package:

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package project

import (
	"context"
	"os"
	"path/filepath"

	"github.com/bborbe/errors"
)

const darkFactoryYAML = ".dark-factory.yaml"

// FindRoot walks up the directory tree from the current working directory,
// looking for a .dark-factory.yaml file. It stops at $HOME and never
// ascends above it. Returns the directory that contains .dark-factory.yaml.
//
// If no .dark-factory.yaml is found in any ancestor up to $HOME, it returns:
//
//	"not a dark-factory project: no .dark-factory.yaml in <cwd> or any parent directory"
func FindRoot(ctx context.Context) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errors.Wrap(ctx, err, "get home directory")
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", errors.Wrap(ctx, err, "get working directory")
	}

	dir := cwd
	for {
		yamlPath := filepath.Join(dir, darkFactoryYAML)
		if _, err := os.Stat(yamlPath); err == nil {
			return dir, nil
		}

		// Stop at $HOME — do not ascend above it.
		if dir == home {
			break
		}

		parent := filepath.Dir(dir)
		// Stop at filesystem root (guard against infinite loop on non-standard FS layouts).
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", errors.Errorf(
		ctx,
		"not a dark-factory project: no %s in %s or any parent directory",
		darkFactoryYAML,
		cwd,
	)
}
```

## 2. Update `main.go`: replace `git.ResolveGitRoot` with `project.FindRoot`

**Step 2a — Verify `git` import usage before removing it.**
Run:
```bash
grep -n "git\." main.go
```
If the ONLY match is `git.ResolveGitRoot(ctx)` on the one line, remove the `git` import. If there are other `git.` usages, keep it.

**Step 2b — Replace the resolution block (lines 76–83).**

Find this block in `main.go`:
```go
gitRoot, err := git.ResolveGitRoot(ctx)
if err != nil {
    return err
}
slog.Debug("resolved git root", "root", gitRoot)
if err := os.Chdir(gitRoot); err != nil {
    return errors.Wrap(ctx, err, "chdir to git root")
}
```

Replace it with:
```go
projectRoot, err := project.FindRoot(ctx)
if err != nil {
    return err
}
slog.Debug("resolved project root", "root", projectRoot)
if err := os.Chdir(projectRoot); err != nil {
    return errors.Wrap(ctx, err, "chdir to project root")
}
```

**Step 2c — Update imports in `main.go`.**

Remove `"github.com/bborbe/dark-factory/pkg/git"` from the import block (only if it has no other usages after step 2a).

Add `"github.com/bborbe/dark-factory/pkg/project"` to the import block (in the grouped dark-factory imports block).

## 3. Create `pkg/project/root_test.go`

Create a new test file `pkg/project/root_test.go`. Do NOT modify the existing suite file — it already registers the test suite.

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package project_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/project"
)

var _ = Describe("FindRoot", func() {
	var ctx context.Context
	var origDir string

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		origDir, err = os.Getwd()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		// Always restore original working directory.
		Expect(os.Chdir(origDir)).To(Succeed())
	})

	It("returns the directory containing .dark-factory.yaml when called from that directory", func() {
		projectDir, err := os.MkdirTemp("", "df-root-test-*")
		Expect(err).NotTo(HaveOccurred())
		defer os.RemoveAll(projectDir)

		Expect(os.WriteFile(filepath.Join(projectDir, ".dark-factory.yaml"), []byte("workflow: direct\n"), 0600)).To(Succeed())
		Expect(os.Chdir(projectDir)).To(Succeed())

		result, err := project.FindRoot(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(projectDir))
	})

	It("walks up to find .dark-factory.yaml in a parent directory", func() {
		projectDir, err := os.MkdirTemp("", "df-root-test-*")
		Expect(err).NotTo(HaveOccurred())
		defer os.RemoveAll(projectDir)

		subDir := filepath.Join(projectDir, "pkg", "config")
		Expect(os.MkdirAll(subDir, 0750)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(projectDir, ".dark-factory.yaml"), []byte("workflow: direct\n"), 0600)).To(Succeed())
		Expect(os.Chdir(subDir)).To(Succeed())

		result, err := project.FindRoot(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(projectDir))
	})

	It("returns error when no .dark-factory.yaml is found anywhere up to $HOME", func() {
		// Use a temp dir under /tmp — no .dark-factory.yaml should exist there.
		tmpDir, err := os.MkdirTemp("", "df-no-project-*")
		Expect(err).NotTo(HaveOccurred())
		defer os.RemoveAll(tmpDir)

		// Ensure there is no .dark-factory.yaml in tmpDir or its parents up to $HOME.
		// (os.MkdirTemp creates under os.TempDir() which is typically /tmp — no .dark-factory.yaml)
		Expect(os.Chdir(tmpDir)).To(Succeed())

		// Verify assumption: $HOME does not contain .dark-factory.yaml
		home, homeErr := os.UserHomeDir()
		Expect(homeErr).NotTo(HaveOccurred())
		if _, statErr := os.Stat(filepath.Join(home, ".dark-factory.yaml")); statErr == nil {
			Skip("$HOME contains .dark-factory.yaml — test cannot run in this environment")
		}

		_, err = project.FindRoot(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not a dark-factory project"))
		Expect(err.Error()).To(ContainSubstring(".dark-factory.yaml"))
	})
})
```

**Note about the test environment:** The test for "no project found" skips if `$HOME/.dark-factory.yaml` exists, because CI or dev environments may have it. The skip is safe — it does not cause false failure.

## 4. Add CHANGELOG entry

At the top of `CHANGELOG.md`, add a new `## Unreleased` section before the first `## vX.Y.Z` section:

```markdown
## Unreleased

- feat: walk up directory tree from cwd to find .dark-factory.yaml project root, replacing git root detection
- fix: spec list / spec status / prompt list now error with "not a dark-factory project" when no .dark-factory.yaml found, instead of silently returning empty results
```

If `## Unreleased` already exists, append the two bullets to it (do not create a duplicate section).

## 5. Run `make test`

```bash
cd /workspace && make test
```

All tests must pass before proceeding.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do NOT delete or modify `pkg/git/root.go` — `git.ResolveGitRoot` may be used elsewhere; only remove it from main.go's import if confirmed unused there
- The `project.FindRoot` function must use `os.UserHomeDir()` to get `$HOME` — never hardcode a path
- Walk-up must stop at `$HOME` (inclusive: `$HOME` itself is checked for `.dark-factory.yaml`, then the loop stops)
- Walk-up also stops at filesystem root (`filepath.Dir(dir) == dir`) as a safety guard
- Error message must contain the strings `"not a dark-factory project"` and `".dark-factory.yaml"` and the original cwd path — the acceptance criteria test verifies the format
- `dark-factory version` and `dark-factory help` must continue to work from any directory; confirm they return before the `project.FindRoot` call in `run()` (they are handled by the `switch command` block before `FindRoot` is called)
- Errors use `errors.Errorf(ctx, ...)` and `errors.Wrap(ctx, err, ...)` from `github.com/bborbe/errors` — never `fmt.Errorf`
- Tests use Ginkgo/Gomega in `package project_test` — the existing suite file (`project_suite_test.go`) already registers the suite; do NOT create a new suite file
- Do NOT touch `go.mod` / `go.sum` / `vendor/`
- Do NOT touch files outside of `pkg/project/root.go`, `pkg/project/root_test.go`, and `main.go`
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional spot checks:
1. `grep -n "project.FindRoot\|git.ResolveGitRoot" main.go` — `project.FindRoot` present; `git.ResolveGitRoot` absent
2. `grep -n "dark-factory/pkg/project" main.go` — project package imported
3. `grep -rn "FindRoot" pkg/project/root.go pkg/project/root_test.go` — function defined and tested
4. `go test ./pkg/project/... -v -run "FindRoot"` — new tests pass
5. `grep -A3 "## Unreleased" CHANGELOG.md` — shows feat: walk-up entry and fix: silent zero entry
6. `grep -n "not a dark-factory project" pkg/project/root.go` — error message format matches spec requirement
</verification>
