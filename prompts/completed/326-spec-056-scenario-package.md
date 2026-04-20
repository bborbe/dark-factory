---
status: completed
spec: [056-scenario-cli-readonly]
summary: Created pkg/scenario package with ScenarioFile model, Lister interface, Find method, counterfeiter mock, comprehensive tests at 87.5% coverage, and CHANGELOG entry
container: dark-factory-326-spec-056-scenario-package
dark-factory-version: v0.129.0
created: "2026-04-20T16:00:00Z"
queued: "2026-04-20T16:26:08Z"
started: "2026-04-20T16:28:36Z"
completed: "2026-04-20T16:36:39Z"
branch: dark-factory/scenario-cli-readonly
---

<summary>
- A new `pkg/scenario/` package is created with types, parsing, listing, and finding for scenario files
- Scenario status constants `idea`, `draft`, `active`, `outdated` are defined; any other value (including empty) is treated as unknown in summary output
- `Load(ctx, path)` parses a scenario file: reads YAML frontmatter, extracts the title from the first `# ` heading, handles malformed frontmatter gracefully (stores empty status rather than erroring)
- `Lister` interface with `List`, `Summary`, and `Find` methods — counterfeiter mock generated
- `List` returns all scenario files in the directory sorted by number ascending, silently skipping non-`NNN-*.md` files
- When the `scenarios/` directory does not exist, `List` returns empty slice with no error
- Files with missing or malformed frontmatter appear in the list with status shown as empty (callers treat it as unknown)
- `Summary` returns counts grouped by status (idea, draft, active, outdated, unknown, total)
- `Find(ctx, id)` matches by numeric prefix or name fragment; returns all matches — callers handle 0 vs 1 vs many
- Comprehensive tests with Ginkgo/Gomega covering happy paths and all failure modes from the spec
</summary>

<objective>
Create the `pkg/scenario/` package — the data model and file-system access layer for scenario files. This package provides the `Lister` interface and `Load` function used by the CLI commands in prompt 2. No CLI wiring or main.go changes are made here.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-enum-type-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`

Key files to read before editing:
- `pkg/spec/spec.go` — reference pattern for Load (frontmatter parsing via `github.com/adrg/frontmatter`, yamlV3Format, graceful error handling), Frontmatter struct, status constants
- `pkg/spec/lister.go` — reference pattern for Lister interface and implementation (ReadDir, skip non-.md, sort, Summary counts)
- `pkg/specnum/specnum.go` — `Parse(s string) int` extracts leading integer from filename; use this for scenario number parsing
- `scenarios/001-workflow-direct.md` — example scenario file structure (frontmatter, title heading, body)
- `scenarios/006-spec-lifecycle.md` — example with `status: idea`
- `pkg/spec/spec_suite_test.go` — reference for suite file pattern
- `pkg/spec/lister.go` — reference for how `Summary` counts across statuses
</context>

<requirements>

## 1. Create `pkg/scenario/doc.go`

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package scenario provides types and file-system access for scenario markdown files.
package scenario
```

## 2. Create `pkg/scenario/scenario.go` — status constants, Frontmatter, ScenarioFile, and Load

### 2a. Status type and constants

```go
// Status represents the lifecycle state of a scenario.
type Status string

const (
    // StatusIdea indicates a rough concept not yet ready for execution.
    StatusIdea Status = "idea"
    // StatusDraft indicates a scenario that is written but not yet exercised.
    StatusDraft Status = "draft"
    // StatusActive indicates a scenario that is currently in use for regression coverage.
    StatusActive Status = "active"
    // StatusOutdated indicates a scenario that no longer reflects the current behavior.
    StatusOutdated Status = "outdated"
)

// KnownStatuses is the set of valid scenario status values.
var KnownStatuses = []Status{StatusIdea, StatusDraft, StatusActive, StatusOutdated}

// IsKnown returns true if s is one of the four defined scenario statuses.
func IsKnown(s Status) bool {
    for _, k := range KnownStatuses {
        if s == k {
            return true
        }
    }
    return false
}
```

### 2b. Frontmatter struct

```go
// Frontmatter represents the YAML frontmatter in a scenario file.
type Frontmatter struct {
    Status string `yaml:"status"`
}
```

### 2c. ScenarioFile struct

```go
// ScenarioFile represents a loaded scenario file.
type ScenarioFile struct {
    Path        string
    Name        string     // filename without .md extension, e.g. "001-workflow-direct"
    Number      int        // numeric prefix, -1 if none
    Frontmatter Frontmatter
    Title       string     // text from the first "# " heading after frontmatter, empty if absent
    RawContent  []byte     // full file bytes (used by show command to print entire file)
}
```

### 2d. Load function

```go
// filenameRe matches files that follow the NNN-*.md convention (one or more leading digits).
var filenameRe = regexp.MustCompile(`^\d+-.*\.md$`)

// Load reads one scenario file from disk. On frontmatter parse failure the file is still
// returned with an empty Frontmatter — callers treat an empty/unrecognized status as unknown.
// Returns an error only if the file cannot be read at all.
func Load(ctx context.Context, path string) (*ScenarioFile, error) {
    // #nosec G304 -- path is from caller who controls the scenarios directory
    content, err := os.ReadFile(path)
    if err != nil {
        return nil, errors.Wrap(ctx, err, "read scenario file")
    }

    name := strings.TrimSuffix(filepath.Base(path), ".md")
    number := specnum.Parse(name)

    var fm Frontmatter
    yamlV3Format := frontmatter.NewFormat("---", "---", yaml.Unmarshal)
    body, fmErr := frontmatter.Parse(bytes.NewReader(content), &fm, yamlV3Format)
    if fmErr != nil {
        // Frontmatter missing or malformed — leave fm empty, body = full content
        body = content
        fm = Frontmatter{}
    }

    return &ScenarioFile{
        Path:        path,
        Name:        name,
        Number:      number,
        Frontmatter: fm,
        Title:       extractTitle(body),
        RawContent:  content,
    }, nil
}

// extractTitle returns the text of the first "# " heading found in body, or "" if none.
func extractTitle(body []byte) string {
    for _, line := range strings.Split(string(body), "\n") {
        line = strings.TrimRight(line, "\r")
        if strings.HasPrefix(line, "# ") {
            return strings.TrimPrefix(line, "# ")
        }
    }
    return ""
}
```

Required imports for `pkg/scenario/scenario.go`:
```go
import (
    "bytes"
    "context"
    "os"
    "path/filepath"
    "regexp"
    "strings"

    "github.com/adrg/frontmatter"
    "github.com/bborbe/errors"
    "gopkg.in/yaml.v3"

    "github.com/bborbe/dark-factory/pkg/specnum"
)
```

Use `strings.Split` for title extraction — no `bufio` needed.

## 3. Create `pkg/scenario/lister.go` — Lister interface and implementation

```go
// Summary holds counts of scenarios grouped by status.
type Summary struct {
    Idea     int
    Draft    int
    Active   int
    Outdated int
    Unknown  int
    Total    int
}

//counterfeiter:generate -o ../../mocks/scenario-lister.go --fake-name ScenarioLister . Lister

// Lister lists scenario files from a directory.
type Lister interface {
    List(ctx context.Context) ([]*ScenarioFile, error)
    Summary(ctx context.Context) (*Summary, error)
    Find(ctx context.Context, id string) ([]*ScenarioFile, error)
}

// NewLister creates a Lister that scans the given directory.
func NewLister(dir string) Lister {
    return &lister{dir: dir}
}

type lister struct {
    dir string
}
```

### 3a. List implementation

```go
// List returns all scenario files in l.dir whose names match NNN-*.md, sorted by Number ascending.
// Returns an empty slice (no error) when the directory does not exist.
func (l *lister) List(ctx context.Context) ([]*ScenarioFile, error) {
    entries, err := os.ReadDir(l.dir)
    if err != nil {
        if os.IsNotExist(err) {
            return nil, nil
        }
        return nil, errors.Wrap(ctx, err, "read scenarios directory")
    }

    var files []*ScenarioFile
    for _, entry := range entries {
        if entry.IsDir() || !filenameRe.MatchString(entry.Name()) {
            continue
        }
        path := filepath.Join(l.dir, entry.Name())
        sf, err := Load(ctx, path)
        if err != nil {
            slog.Warn("skipping scenario file", "file", entry.Name(), "error", err)
            continue
        }
        files = append(files, sf)
    }

    sort.Slice(files, func(i, j int) bool {
        return files[i].Number < files[j].Number
    })
    return files, nil
}
```

### 3b. Summary implementation

```go
// Summary returns counts of scenario files grouped by status.
func (l *lister) Summary(ctx context.Context) (*Summary, error) {
    files, err := l.List(ctx)
    if err != nil {
        return nil, errors.Wrap(ctx, err, "list scenarios")
    }
    s := &Summary{Total: len(files)}
    for _, sf := range files {
        switch Status(sf.Frontmatter.Status) {
        case StatusIdea:
            s.Idea++
        case StatusDraft:
            s.Draft++
        case StatusActive:
            s.Active++
        case StatusOutdated:
            s.Outdated++
        default:
            s.Unknown++
        }
    }
    return s, nil
}
```

### 3c. Find implementation

```go
// Find returns all scenarios in l.dir whose number or name matches id.
//
// Matching rules:
//   - If id parses as a number (specnum.Parse(id) >= 0), return files with that Number.
//   - Otherwise, return files whose Name contains id as a substring (case-sensitive).
//
// Returns an empty slice (not an error) when nothing matches.
func (l *lister) Find(ctx context.Context, id string) ([]*ScenarioFile, error) {
    files, err := l.List(ctx)
    if err != nil {
        return nil, errors.Wrap(ctx, err, "list scenarios for find")
    }

    num := specnum.Parse(id)
    var matches []*ScenarioFile
    for _, sf := range files {
        if num >= 0 {
            if sf.Number == num {
                matches = append(matches, sf)
            }
        } else {
            if strings.Contains(sf.Name, id) {
                matches = append(matches, sf)
            }
        }
    }
    return matches, nil
}
```

Required imports for `pkg/scenario/lister.go`:
```go
import (
    "context"
    "log/slog"
    "os"
    "path/filepath"
    "sort"
    "strings"

    "github.com/bborbe/errors"

    "github.com/bborbe/dark-factory/pkg/specnum"
)
```

## 4. Generate counterfeiter mock

Run `go generate ./pkg/scenario/...` after creating the files. The mock is placed at `mocks/scenario-lister.go` with fake name `ScenarioLister`.

## 5. Create `pkg/scenario/scenario_suite_test.go`

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package scenario_test

import (
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

func TestScenario(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "Scenario Suite")
}
```

## 6. Create `pkg/scenario/scenario_test.go`

Write a comprehensive test file in `package scenario_test`. Follow the Ginkgo/Gomega patterns from `pkg/spec/spec_test.go`.

### Test 6a: Load — normal file

Create a temp file named `plain-no-number.md` with:
```
---
status: active
---

# My scenario title

Validates that something works.
```
Call `Load(ctx, path)`. Expect:
- `sf.Frontmatter.Status == "active"`
- `sf.Title == "My scenario title"`
- `sf.Number == -1` (no numeric prefix in filename)
- `len(sf.RawContent) > 0`

### Test 6b: Load — numeric prefix in filename

Create file named `042-my-scenario.md`. Call `Load`. Expect `sf.Number == 42`.

### Test 6c: Load — malformed frontmatter

Create a file with no `---` delimiters. Call `Load`. Expect:
- No error returned
- `sf.Frontmatter.Status == ""`
- `sf.Title` extracted from body if present

### Test 6d: Load — missing title heading

Create a file with frontmatter but no `# ` line. Expect `sf.Title == ""`.

### Test 6e: IsKnown

```go
Expect(scenario.IsKnown(scenario.StatusActive)).To(BeTrue())
Expect(scenario.IsKnown(scenario.StatusIdea)).To(BeTrue())
Expect(scenario.IsKnown(scenario.Status("unknown"))).To(BeFalse())
Expect(scenario.IsKnown(scenario.Status(""))).To(BeFalse())
```

## 7. Create `pkg/scenario/lister_test.go`

Write a comprehensive test file in `package scenario_test`.

### Test 7a: List — missing directory returns empty, no error

```go
lister := scenario.NewLister("/tmp/does-not-exist-" + randomSuffix())
files, err := lister.List(ctx)
Expect(err).NotTo(HaveOccurred())
Expect(files).To(BeEmpty())
```

### Test 7b: List — skips non-NNN-*.md files

Create a temp dir with `README.md` and `001-first.md`. Expect only `001-first.md` to be returned.

### Test 7c: List — sorted by number ascending

Create `003-third.md`, `001-first.md`, `002-second.md`. Expect order: 001, 002, 003.

### Test 7d: List — malformed frontmatter file included with empty status

Create `001-bad.md` with no frontmatter delimiters. Expect the file to be returned with `Frontmatter.Status == ""` (not skipped, not errored).

### Test 7e: Summary — counts all known statuses and unknown

Create files:
- `001-a.md` with `status: idea`
- `002-b.md` with `status: active`
- `003-c.md` with `status: outdated`
- `004-d.md` with no frontmatter (malformed → unknown)
- `005-e.md` with `status: bogus` (unrecognized → unknown)

Call `Summary`. Expect:
- `s.Idea == 1`, `s.Active == 1`, `s.Outdated == 1`, `s.Unknown == 2`, `s.Total == 5`

### Test 7f: Find — by number prefix

Create `001-workflow-direct.md`. Call `Find(ctx, "001")`. Expect 1 match with `Number == 1`.

### Test 7g: Find — by name fragment

Create `001-workflow-direct.md`. Call `Find(ctx, "workflow-direct")`. Expect 1 match.

### Test 7h: Find — no matches returns empty slice, no error

Call `Find(ctx, "nonexistent")`. Expect empty slice and no error.

### Test 7i: Find — multiple matches for same number (duplicate files)

Create `001-first.md` and `001-second.md`. Call `Find(ctx, "001")`. Expect 2 matches returned (no error — caller handles this).

### Test 7j: Find — fragment matches multiple files

Create `001-workflow-direct.md` and `002-workflow-pr.md`. Call `Find(ctx, "workflow")`. Expect 2 matches.

## 8. Write CHANGELOG entry

Add `## Unreleased` at the top of `CHANGELOG.md` if it does not already exist, then append:

```
- feat: add `pkg/scenario` package with ScenarioFile model, Lister interface, and Find for scenario CLI support
```

## 9. Run `make test`

```bash
cd /workspace && make test
```

Must pass before proceeding to `make precommit`.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do not touch `go.mod` / `go.sum` / `vendor/`
- `pkg/scenario/` is read-only: no status mutations, no Write/Save methods, no timestamp stamps
- The `Lister.Find` method returns ALL matches and an error only on I/O failure — zero matches is an empty slice, not an error; the CLI show command (prompt 2) handles the 0/1/many distinction
- Files in `scenarios/` not matching `^\d+-.*\.md$` must be silently skipped — no warnings, no errors
- Malformed frontmatter must NOT abort `List` or `Find` — the file is included with empty `Frontmatter.Status`
- Use `errors.Wrap` / `errors.Errorf` from `github.com/bborbe/errors` — never `fmt.Errorf`
- Use `github.com/adrg/frontmatter` with `frontmatter.NewFormat("---", "---", yaml.Unmarshal)` exactly as in `pkg/spec/spec.go`
- External test package (`package scenario_test`)
- Coverage ≥80% for new package
- `slog.Warn` on individual file load errors inside `List` (don't abort the whole scan)
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Spot checks:
1. `ls pkg/scenario/` — should show doc.go, scenario.go, lister.go, scenario_suite_test.go, scenario_test.go, lister_test.go
2. `ls mocks/scenario-lister.go` — counterfeiter mock must exist
3. `go test -cover ./pkg/scenario/... | tee /tmp/cov.txt && grep -E 'coverage: (8[0-9]|9[0-9]|100)\.[0-9]+%' /tmp/cov.txt` — passes AND coverage ≥80%
4. `grep -n "StatusActive\|StatusIdea\|StatusDraft\|StatusOutdated" pkg/scenario/scenario.go` — at least 4 matches
5. `grep -n "filenameRe" pkg/scenario/scenario.go` — regex must be defined and used in lister
</verification>
