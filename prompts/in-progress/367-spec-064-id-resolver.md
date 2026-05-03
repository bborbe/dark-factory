---
status: executing
spec: [064-cli-flexible-id-matching]
container: dark-factory-367-spec-064-id-resolver
dark-factory-version: v0.145.1-3-g93401a1
created: "2026-05-03T13:00:00Z"
queued: "2026-05-03T12:51:47Z"
started: "2026-05-03T12:52:39Z"
branch: dark-factory/cli-flexible-id-matching
---

<summary>
- `dark-factory spec show 63` now resolves to `063-*.md` — unpadded numbers are accepted
- `dark-factory spec show 063-foo-bar.md` now resolves correctly — `.md` extension is stripped before lookup
- `dark-factory prompt approve 63` resolves to `063-*.md` — same improvement for all prompt subcommands
- `dark-factory prompt approve 063-foo-bar.md` resolves correctly — `.md` extension accepted for prompts too
- Numeric matching compares integer values: input `1` matches `001-foo.md` but never `010-bar.md`
- When two spec files share the same numeric prefix (defensive case), error lists all matching paths
- When two prompt files share the same numeric prefix (defensive case), error lists all matching paths
- All four formats work uniformly across all spec and prompt subcommands that accept an `<id>` argument
- Unit tests cover all four resolution formats, ambiguity case, and zero-match case for both spec and prompt finders
</summary>

<objective>
Fix `pkg/cmd/spec_finder.go` and `pkg/cmd/prompt_finder.go` so that the CLI resolves `<id>` arguments by integer value when the input is a bare or unpadded number, and detects and reports ambiguous matches (multiple files with the same numeric prefix). Today only the full basename without `.md` extension works for padded numeric inputs; unpadded numbers (`63`) fail because the current prefix match compares strings not integers.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-parse-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/`

Read these files in full before editing:
- `pkg/cmd/spec_finder.go` — full file; current implementation uses string prefix match (`strings.HasPrefix(name, id+"-")`) which fails for unpadded numbers like `63` matching `063-foo.md`
- `pkg/cmd/spec_finder_test.go` — full file; existing tests to understand coverage and add the missing cases
- `pkg/cmd/prompt_finder.go` — full file; same issue as spec_finder; `FindPromptFile` is also called directly by `approve.go`, `cancel.go`, `requeue.go`, `unapprove.go`, `prompt_complete.go`
- `pkg/cmd/prompt_finder_test.go` — full file; existing tests to understand coverage
- `pkg/specnum/specnum.go` — full file; `specnum.Parse("63") == 63`, `specnum.Parse("063-foo.md") == 63` — integer equality handles both padded and unpadded forms
- `pkg/cmd/approve.go` — read lines 50–75; note that it calls `FindPromptFile` twice (inbox then queue) and must remain unchanged (the function signature doesn't change)

The spec this implements: `specs/in-progress/064-cli-flexible-id-matching.md`
</context>

<requirements>

## 1. Redesign `pkg/cmd/spec_finder.go`

Replace the entire file content with the following implementation. The key changes are:
- Import `specnum` and `strings` packages (strings is already imported; add specnum)
- Remove the private `findInDir` helper (fold its logic into `FindSpecFileInDirs`)
- Step 1: direct path check (unchanged)
- Step 2: exact match — normalize by stripping `.md` suffix, then try `cleanID+".md"` in each dir in order (handles formats: `063-bug-foo`, `063-bug-foo.md`, and `063.md` if that exact file exists)
- Step 3: numeric match — parse `cleanID` with `specnum.Parse`; if ≥0, scan ALL dirs and collect every file whose `specnum.Parse(entry.Name())` equals `idNum`; return unique match, or ambiguity error, or not-found error

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/specnum"
)

// FindSpecFile finds a spec by absolute/relative path, exact filename, or numeric prefix match within specsDir.
func FindSpecFile(ctx context.Context, specsDir, id string) (string, error) {
	return FindSpecFileInDirs(ctx, id, specsDir)
}

// FindSpecFileInDirs resolves an <id> argument against one or more spec directories.
// Accepts four formats:
//   - padded number:              "063"
//   - unpadded number:            "63"
//   - full basename (no ext):     "063-bug-foo"
//   - full basename (with .md):   "063-bug-foo.md"
//
// Resolution order:
//  1. Direct path (absolute or containing a directory separator) — checked as-is.
//  2. Exact basename match — strip .md suffix, append .md, stat in each dir in order.
//  3. Numeric match — parse cleanID as integer via specnum.Parse; scan ALL dirs and
//     collect files whose numeric prefix equals idNum; return unique or ambiguity error.
func FindSpecFileInDirs(ctx context.Context, id string, dirs ...string) (string, error) {
	// 1. Direct path (absolute or contains a directory separator)
	if filepath.IsAbs(id) || strings.ContainsRune(id, '/') {
		if _, err := os.Stat(id); err == nil {
			return id, nil
		}
	}

	// 2. Exact basename match (handles "063-bug-foo" and "063-bug-foo.md")
	cleanID := strings.TrimSuffix(id, ".md")
	for _, dir := range dirs {
		path := filepath.Join(dir, cleanID+".md")
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	// 3. Numeric match across all dirs (handles "063" and "63")
	idNum := specnum.Parse(cleanID)
	if idNum < 0 {
		return "", errors.Errorf(ctx, "spec not found: %s", id)
	}

	var matches []string
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", errors.Wrap(ctx, err, "read specs directory")
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			if specnum.Parse(entry.Name()) == idNum {
				matches = append(matches, filepath.Join(dir, entry.Name()))
			}
		}
	}

	switch len(matches) {
	case 0:
		return "", errors.Errorf(ctx, "spec not found: %s", id)
	case 1:
		return matches[0], nil
	default:
		return "", errors.Errorf(ctx, "ambiguous spec id %s: matches %s", id, strings.Join(matches, ", "))
	}
}
```

**Why this works:**
- `"063"` → `cleanID = "063"` → step 2 tries `dir/063.md` (not found) → step 3: `specnum.Parse("063") = 63`, finds `063-bug-foo.md` where `specnum.Parse("063-bug-foo.md") = 63` → match
- `"63"` → `cleanID = "63"` → step 2 tries `dir/63.md` (not found) → step 3: `specnum.Parse("63") = 63`, finds `063-bug-foo.md` → match
- `"063-bug-foo"` → `cleanID = "063-bug-foo"` → step 2 tries `dir/063-bug-foo.md` → FOUND immediately (fast path, O(1) stat)
- `"063-bug-foo.md"` → `cleanID = "063-bug-foo"` → step 2 tries `dir/063-bug-foo.md` → FOUND immediately (fast path)
- `"1"` with `001-foo.md` and `010-bar.md`: `specnum.Parse("1") = 1`; `specnum.Parse("001-foo.md") = 1` matches, `specnum.Parse("010-bar.md") = 10` does NOT → unique match `001-foo.md`

## 2. Redesign `pkg/cmd/prompt_finder.go`

Replace the entire file content with the following implementation. Same strategy as spec_finder:

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/specnum"
)

// FindPromptFileInDirs resolves an <id> argument against one or more prompt directories.
// Accepts four formats: padded number, unpadded number, full basename, full basename with .md.
// For numeric IDs, collects all matches across all dirs and returns an ambiguity error if
// more than one file matches the same numeric prefix.
func FindPromptFileInDirs(ctx context.Context, id string, dirs ...string) (string, error) {
	// 1. Direct path
	if filepath.IsAbs(id) || strings.ContainsRune(id, '/') {
		if _, err := os.Stat(id); err == nil {
			return id, nil
		}
	}

	// 2. Exact basename match in each dir in order
	cleanID := strings.TrimSuffix(id, ".md")
	for _, dir := range dirs {
		path := filepath.Join(dir, cleanID+".md")
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	// 3. Numeric match across all dirs
	idNum := specnum.Parse(cleanID)
	if idNum < 0 {
		return "", errors.Errorf(ctx, "prompt not found: %s", id)
	}

	var matches []string
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", errors.Wrap(ctx, err, "read directory")
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			if specnum.Parse(entry.Name()) == idNum {
				matches = append(matches, filepath.Join(dir, entry.Name()))
			}
		}
	}

	switch len(matches) {
	case 0:
		return "", errors.Errorf(ctx, "prompt not found: %s", id)
	case 1:
		return matches[0], nil
	default:
		return "", errors.Errorf(ctx, "ambiguous prompt id %s: matches %s", id, strings.Join(matches, ", "))
	}
}

// FindPromptFile finds a prompt file by id in a single directory.
// Accepts four formats: padded number, unpadded number, full basename, full basename with .md.
// Detects and reports ambiguous matches within the single directory.
func FindPromptFile(ctx context.Context, dir, id string) (string, error) {
	// 1. Direct path
	if filepath.IsAbs(id) || strings.ContainsRune(id, '/') {
		if _, err := os.Stat(id); err == nil {
			return id, nil
		}
	}

	// 2. Exact basename match
	cleanID := strings.TrimSuffix(id, ".md")
	path := filepath.Join(dir, cleanID+".md")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}

	// 3. Numeric match within dir
	idNum := specnum.Parse(cleanID)
	if idNum < 0 {
		return "", errors.Errorf(ctx, "file not found: %s", id)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", errors.Errorf(ctx, "file not found: %s", id)
		}
		return "", errors.Wrap(ctx, err, "read directory")
	}

	var matches []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if specnum.Parse(entry.Name()) == idNum {
			matches = append(matches, filepath.Join(dir, entry.Name()))
		}
	}

	switch len(matches) {
	case 0:
		return "", errors.Errorf(ctx, "file not found: %s", id)
	case 1:
		return matches[0], nil
	default:
		return "", errors.Errorf(ctx, "ambiguous prompt id %s: matches %s", id, strings.Join(matches, ", "))
	}
}
```

**Note:** `approve.go` calls `FindPromptFile(ctx, a.inboxDir, id)` then `FindPromptFile(ctx, a.queueDir, id)` — this pattern is unchanged and works correctly. Each call checks a single directory. Do NOT modify `approve.go`.

## 3. Add tests to `pkg/cmd/spec_finder_test.go`

Append the following `It` blocks inside the existing `Describe("findSpecFile", func() {` block (after the existing "finds spec by numeric prefix" test):

```go
It("finds spec by unpadded number", func() {
    writeSpec("063-bug-foo.md")
    result, err := cmd.FindSpecFile(ctx, specsDir, "63")
    Expect(err).NotTo(HaveOccurred())
    Expect(result).To(Equal(filepath.Join(specsDir, "063-bug-foo.md")))
})

It("finds spec by padded number", func() {
    writeSpec("063-bug-foo.md")
    result, err := cmd.FindSpecFile(ctx, specsDir, "063")
    Expect(err).NotTo(HaveOccurred())
    Expect(result).To(Equal(filepath.Join(specsDir, "063-bug-foo.md")))
})

It("does not match 010-bar.md when input is 1 (integer match, not string prefix)", func() {
    writeSpec("001-foo.md")
    writeSpec("010-bar.md")
    result, err := cmd.FindSpecFile(ctx, specsDir, "1")
    Expect(err).NotTo(HaveOccurred())
    Expect(result).To(Equal(filepath.Join(specsDir, "001-foo.md")))
})

It("returns ambiguity error when two specs share the same numeric prefix", func() {
    writeSpec("001-foo.md")
    writeSpec("001-bar.md")
    _, err := cmd.FindSpecFile(ctx, specsDir, "001")
    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("ambiguous spec id 001"))
    Expect(err.Error()).To(ContainSubstring("001-foo.md"))
    Expect(err.Error()).To(ContainSubstring("001-bar.md"))
})
```

Also append the following inside `Describe("FindSpecFileInDirs", func() {`:

```go
It("finds spec by unpadded number across dirs", func() {
    path := filepath.Join(dir2, "063-bug-foo.md")
    Expect(os.WriteFile(path, []byte("---\nstatus: draft\n---\n"), 0600)).To(Succeed())

    result, err := cmd.FindSpecFileInDirs(ctx, "63", dir1, dir2)
    Expect(err).NotTo(HaveOccurred())
    Expect(result).To(Equal(path))
})

It("returns ambiguity error when same numeric prefix exists in different dirs", func() {
    path1 := filepath.Join(dir1, "001-foo.md")
    path2 := filepath.Join(dir2, "001-bar.md")
    Expect(os.WriteFile(path1, []byte("---\nstatus: draft\n---\n"), 0600)).To(Succeed())
    Expect(os.WriteFile(path2, []byte("---\nstatus: draft\n---\n"), 0600)).To(Succeed())

    _, err := cmd.FindSpecFileInDirs(ctx, "1", dir1, dir2)
    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("ambiguous spec id 1"))
    Expect(err.Error()).To(ContainSubstring("001-foo.md"))
    Expect(err.Error()).To(ContainSubstring("001-bar.md"))
})
```

## 4. Add tests to `pkg/cmd/prompt_finder_test.go`

Append the following `It` blocks inside the existing `Describe("FindPromptFile", func() {`:

```go
It("finds prompt by unpadded number", func() {
    writePrompt("063-fix-bug.md")
    result, err := cmd.FindPromptFile(ctx, dir, "63")
    Expect(err).NotTo(HaveOccurred())
    Expect(result).To(Equal(filepath.Join(dir, "063-fix-bug.md")))
})

It("finds prompt by padded number matching zero-padded file", func() {
    writePrompt("063-fix-bug.md")
    result, err := cmd.FindPromptFile(ctx, dir, "063")
    Expect(err).NotTo(HaveOccurred())
    Expect(result).To(Equal(filepath.Join(dir, "063-fix-bug.md")))
})

It("does not match 010-bar.md when input is 1", func() {
    writePrompt("001-foo.md")
    writePrompt("010-bar.md")
    result, err := cmd.FindPromptFile(ctx, dir, "1")
    Expect(err).NotTo(HaveOccurred())
    Expect(result).To(Equal(filepath.Join(dir, "001-foo.md")))
})

It("returns ambiguity error when two prompts share the same numeric prefix", func() {
    writePrompt("001-foo.md")
    writePrompt("001-bar.md")
    _, err := cmd.FindPromptFile(ctx, dir, "001")
    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("ambiguous prompt id 001"))
    Expect(err.Error()).To(ContainSubstring("001-foo.md"))
    Expect(err.Error()).To(ContainSubstring("001-bar.md"))
})
```

Also add a new `Describe("FindPromptFileInDirs", ...)` block in `prompt_finder_test.go`:

```go
var _ = Describe("FindPromptFileInDirs", func() {
    var (
        dir1 string
        dir2 string
        ctx  context.Context
    )

    BeforeEach(func() {
        var err error
        dir1, err = os.MkdirTemp("", "prompt-finder-dirs1-*")
        Expect(err).NotTo(HaveOccurred())
        dir2, err = os.MkdirTemp("", "prompt-finder-dirs2-*")
        Expect(err).NotTo(HaveOccurred())
        ctx = context.Background()
    })

    AfterEach(func() {
        _ = os.RemoveAll(dir1)
        _ = os.RemoveAll(dir2)
    })

    writePromptIn := func(directory, name string) string {
        path := filepath.Join(directory, name)
        Expect(os.WriteFile(path, []byte("---\nstatus: draft\n---\n"), 0600)).To(Succeed())
        return path
    }

    It("finds prompt in first dir by basename", func() {
        path := writePromptIn(dir1, "122-fix-bug.md")
        result, err := cmd.FindPromptFileInDirs(ctx, "122-fix-bug.md", dir1, dir2)
        Expect(err).NotTo(HaveOccurred())
        Expect(result).To(Equal(path))
    })

    It("finds prompt in second dir when not in first", func() {
        path := writePromptIn(dir2, "122-fix-bug.md")
        result, err := cmd.FindPromptFileInDirs(ctx, "122-fix-bug.md", dir1, dir2)
        Expect(err).NotTo(HaveOccurred())
        Expect(result).To(Equal(path))
    })

    It("finds prompt by unpadded number across dirs", func() {
        path := writePromptIn(dir2, "063-bug-foo.md")
        result, err := cmd.FindPromptFileInDirs(ctx, "63", dir1, dir2)
        Expect(err).NotTo(HaveOccurred())
        Expect(result).To(Equal(path))
    })

    It("returns error when not found in any dir", func() {
        _, err := cmd.FindPromptFileInDirs(ctx, "999-missing.md", dir1, dir2)
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("prompt not found"))
    })

    It("returns ambiguity error when same numeric prefix exists in different dirs", func() {
        writePromptIn(dir1, "001-foo.md")
        writePromptIn(dir2, "001-bar.md")
        _, err := cmd.FindPromptFileInDirs(ctx, "1", dir1, dir2)
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("ambiguous prompt id 1"))
    })

    It("skips missing dirs silently", func() {
        path := writePromptIn(dir2, "003-spec.md")
        result, err := cmd.FindPromptFileInDirs(ctx, "003-spec.md", "/nonexistent/dir", dir2)
        Expect(err).NotTo(HaveOccurred())
        Expect(result).To(Equal(path))
    })
})
```

## 5. Run `make test`

```bash
cd /workspace && make test
```

All tests must pass before proceeding.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do NOT change any callers of `FindPromptFile`, `FindSpecFile`, or `FindSpecFileInDirs` — function signatures are unchanged
- Errors use `errors.Errorf(ctx, ...)` and `errors.Wrap(ctx, err, ...)` from `github.com/bborbe/errors` — never `fmt.Errorf`
- Tests use Ginkgo/Gomega in the existing `package cmd_test` test files; do not create new test files for the existing Describe blocks
- The `FindPromptFileInDirs` Describe block is a NEW top-level `var _ = Describe(...)` block added to `prompt_finder_test.go`
- The ambiguity error for specs must contain the string `"ambiguous spec id"` followed by the input and then the matching file paths
- The ambiguity error for prompts must contain the string `"ambiguous prompt id"` followed by the input and then the matching file paths
- The existing test `"finds spec by numeric prefix"` (which tests `"020"`) must still pass
- The existing test `"finds prompt by numeric prefix only"` (which tests `"122"`) must still pass
- Do NOT touch `go.mod` / `go.sum` / `vendor/`
- Do NOT touch any files outside of `pkg/cmd/spec_finder.go`, `pkg/cmd/prompt_finder.go`, `pkg/cmd/spec_finder_test.go`, `pkg/cmd/prompt_finder_test.go`
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional spot checks:
1. `grep -n "specnum" pkg/cmd/spec_finder.go pkg/cmd/prompt_finder.go` — specnum imported in both files
2. `grep -n "ambiguous spec id" pkg/cmd/spec_finder.go` — ambiguity error present
3. `grep -n "ambiguous prompt id" pkg/cmd/prompt_finder.go` — ambiguity error present
4. `go test ./pkg/cmd/... -v -run "unpadded"` — new unpadded tests pass
5. `go test ./pkg/cmd/... -v -run "ambiguous"` — ambiguity tests pass
6. `grep -c "FindPromptFileInDirs" pkg/cmd/prompt_finder_test.go` — at least 1 (new test block present)
</verification>
