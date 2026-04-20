---
status: approved
spec: [056-scenario-cli-readonly]
created: "2026-04-20T16:00:00Z"
queued: "2026-04-20T16:26:08Z"
branch: dark-factory/scenario-cli-readonly
---

<summary>
- Three CLI command files added to `pkg/cmd/`: `scenario_list.go`, `scenario_show.go`, `scenario_status.go`
- `scenario list` prints a table with columns NUMBER, STATUS, TITLE sorted by number ascending
- `scenario show <id>` prints the full raw contents of a matched scenario file; exits non-zero with a clear message on zero or multiple matches (lists all matches when ambiguous)
- `scenario status` prints one line per status value (idea, draft, active, outdated); appends an `unknown` row when any scenario has malformed or unrecognized status
- Factory wiring added in `pkg/factory/factory.go` for all three commands
- `main.go` updated: `ParseArgs` recognizes `scenario` as a two-level command; `runCommand` dispatches to new `runScenarioCommand`; `printHelp` and `printCommandHelp` include the new subcommands; `printScenarioHelp` added
- Counterfeiter mocks generated for all three command interfaces
- Tests for each command with Ginkgo/Gomega
- Existing `prompt` and `spec` command behavior is unchanged
</summary>

<objective>
Wire the `pkg/scenario/` package (from prompt 1) into the CLI as three read-only subcommands: `list`, `show`, and `status`. After this prompt, users can run `dark-factory scenario list`, `dark-factory scenario show <id>`, and `dark-factory scenario status` from any directory with a git root containing a `scenarios/` directory.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-factory-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/`

Key files to read before editing:
- `pkg/scenario/lister.go` — `Lister` interface with `List`, `Summary`, and `Find` methods; `NewLister(dir string)`
- `pkg/scenario/scenario.go` — `ScenarioFile` struct (Path, Name, Number, Frontmatter.Status, Title, RawContent)
- `pkg/cmd/spec_list.go` — reference pattern for list command (counterfeiter directive, interface, private struct, constructor, table/JSON output)
- `pkg/cmd/spec_show.go` — reference pattern for show command
- `pkg/cmd/spec_status.go` — reference pattern for status command
- `pkg/factory/factory.go` — anchor by function name: `CreateSpecListCommand`, `CreateSpecStatusCommand`, `CreateSpecShowCommand` — follow the same pattern
- `main.go` — `ParseArgs` (line ~551), `runCommand` (line ~107), `runSpecCommand` (line ~247), `printSpecHelp` (line ~529), `printHelp` (line ~420), `printCommandHelp` (line ~86) — all need analogous additions for `scenario`
- `mocks/scenario-lister.go` — generated counterfeiter mock (available after prompt 1)
</context>

<requirements>

## 1. Create `pkg/cmd/scenario_list.go`

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
    "context"
    "encoding/json"
    "fmt"
    "os"

    "github.com/bborbe/errors"

    "github.com/bborbe/dark-factory/pkg/scenario"
)

//counterfeiter:generate -o ../../mocks/scenario-list-command.go --fake-name ScenarioListCommand . ScenarioListCommand

// ScenarioListCommand executes the scenario list subcommand.
type ScenarioListCommand interface {
    Run(ctx context.Context, args []string) error
}

// ScenarioEntry represents a single scenario in the list output.
type ScenarioEntry struct {
    Number int    `json:"number"`
    Status string `json:"status"`
    Title  string `json:"title"`
    File   string `json:"file"`
}

// scenarioListCommand implements ScenarioListCommand.
type scenarioListCommand struct {
    lister scenario.Lister
}

// NewScenarioListCommand creates a new ScenarioListCommand.
func NewScenarioListCommand(lister scenario.Lister) ScenarioListCommand {
    return &scenarioListCommand{lister: lister}
}

// Run executes the scenario list command.
func (s *scenarioListCommand) Run(ctx context.Context, args []string) error {
    jsonOutput := false
    for _, arg := range args {
        if arg == "--json" {
            jsonOutput = true
        }
    }

    files, err := s.lister.List(ctx)
    if err != nil {
        return errors.Wrap(ctx, err, "list scenarios")
    }

    entries := make([]ScenarioEntry, 0, len(files))
    for _, sf := range files {
        status := sf.Frontmatter.Status
        if status == "" || !scenario.IsKnown(scenario.Status(status)) {
            status = "unknown"
        }
        entries = append(entries, ScenarioEntry{
            Number: sf.Number,
            Status: status,
            Title:  sf.Title,
            File:   sf.Name + ".md",
        })
    }

    if jsonOutput {
        encoder := json.NewEncoder(os.Stdout)
        encoder.SetIndent("", "  ")
        return encoder.Encode(entries)
    }
    return outputScenarioListTable(entries)
}

// outputScenarioListTable prints scenarios as a fixed-width table.
func outputScenarioListTable(entries []ScenarioEntry) error {
    fmt.Printf("%-6s %-9s %s\n", "NUMBER", "STATUS", "TITLE")
    for _, e := range entries {
        num := fmt.Sprintf("%03d", e.Number)
        fmt.Printf("%-6s %-9s %s\n", num, e.Status, e.Title)
    }
    return nil
}
```

## 2. Create `pkg/cmd/scenario_show.go`

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
    "context"
    "fmt"
    "os"
    "strings"

    "github.com/bborbe/errors"

    "github.com/bborbe/dark-factory/pkg/scenario"
)

//counterfeiter:generate -o ../../mocks/scenario-show-command.go --fake-name ScenarioShowCommand . ScenarioShowCommand

// ScenarioShowCommand executes the scenario show subcommand.
type ScenarioShowCommand interface {
    Run(ctx context.Context, args []string) error
}

// scenarioShowCommand implements ScenarioShowCommand.
type scenarioShowCommand struct {
    lister scenario.Lister
}

// NewScenarioShowCommand creates a new ScenarioShowCommand.
func NewScenarioShowCommand(lister scenario.Lister) ScenarioShowCommand {
    return &scenarioShowCommand{lister: lister}
}

// Run executes the scenario show command.
func (s *scenarioShowCommand) Run(ctx context.Context, args []string) error {
    id := ""
    for _, arg := range args {
        if !strings.HasPrefix(arg, "-") && id == "" {
            id = arg
        }
    }
    if id == "" {
        return errors.Errorf(ctx, "scenario identifier required")
    }

    matches, err := s.lister.Find(ctx, id)
    if err != nil {
        return errors.Wrap(ctx, err, "find scenario")
    }

    switch len(matches) {
    case 0:
        return errors.Errorf(ctx, "no scenario matching %q", id)
    case 1:
        _, err := os.Stdout.Write(matches[0].RawContent)
        return err
    default:
        fmt.Fprintf(os.Stderr, "scenario %q matches multiple files:\n", id)
        for _, sf := range matches {
            fmt.Fprintf(os.Stderr, "  %s\n", sf.Name+".md")
        }
        return errors.Errorf(ctx, "ambiguous scenario identifier %q: %d matches", id, len(matches))
    }
}
```

## 3. Create `pkg/cmd/scenario_status.go`

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
    "context"
    "encoding/json"
    "fmt"
    "os"

    "github.com/bborbe/errors"

    "github.com/bborbe/dark-factory/pkg/scenario"
)

//counterfeiter:generate -o ../../mocks/scenario-status-command.go --fake-name ScenarioStatusCommand . ScenarioStatusCommand

// ScenarioStatusCommand executes the scenario status subcommand.
type ScenarioStatusCommand interface {
    Run(ctx context.Context, args []string) error
}

// scenarioStatusCommand implements ScenarioStatusCommand.
type scenarioStatusCommand struct {
    lister scenario.Lister
}

// NewScenarioStatusCommand creates a new ScenarioStatusCommand.
func NewScenarioStatusCommand(lister scenario.Lister) ScenarioStatusCommand {
    return &scenarioStatusCommand{lister: lister}
}

// Run executes the scenario status command.
func (s *scenarioStatusCommand) Run(ctx context.Context, args []string) error {
    jsonOutput := false
    for _, arg := range args {
        if arg == "--json" {
            jsonOutput = true
        }
    }

    summary, err := s.lister.Summary(ctx)
    if err != nil {
        return errors.Wrap(ctx, err, "get scenario summary")
    }

    if jsonOutput {
        encoder := json.NewEncoder(os.Stdout)
        encoder.SetIndent("", "  ")
        return encoder.Encode(summary)
    }

    fmt.Printf("idea:     %d\n", summary.Idea)
    fmt.Printf("draft:    %d\n", summary.Draft)
    fmt.Printf("active:   %d\n", summary.Active)
    fmt.Printf("outdated: %d\n", summary.Outdated)
    if summary.Unknown > 0 {
        fmt.Printf("unknown:  %d\n", summary.Unknown)
    }
    return nil
}
```

## 4. Generate counterfeiter mocks

Run `go generate ./pkg/cmd/...` to generate mocks for the three new interfaces. This creates:
- `mocks/scenario-list-command.go` (fake name `ScenarioListCommand`)
- `mocks/scenario-show-command.go` (fake name `ScenarioShowCommand`)
- `mocks/scenario-status-command.go` (fake name `ScenarioStatusCommand`)

## 5. Add factory functions to `pkg/factory/factory.go`

Add three new `Create*` functions after `CreateSpecShowCommand` (around line 1082). Follow the exact pattern of existing `CreateSpec*Command` functions — zero business logic, only wiring:

```go
const scenariosDir = "scenarios"

// CreateScenarioListCommand creates a ScenarioListCommand.
func CreateScenarioListCommand(_ config.Config) cmd.ScenarioListCommand {
    return cmd.NewScenarioListCommand(scenario.NewLister(scenariosDir))
}

// CreateScenarioShowCommand creates a ScenarioShowCommand.
func CreateScenarioShowCommand(_ config.Config) cmd.ScenarioShowCommand {
    return cmd.NewScenarioShowCommand(scenario.NewLister(scenariosDir))
}

// CreateScenarioStatusCommand creates a ScenarioStatusCommand.
func CreateScenarioStatusCommand(_ config.Config) cmd.ScenarioStatusCommand {
    return cmd.NewScenarioStatusCommand(scenario.NewLister(scenariosDir))
}
```

Add import for `"github.com/bborbe/dark-factory/pkg/scenario"` to the factory's import block.

## 6. Update `main.go`

### 6a. Update `ParseArgs` — recognize `scenario` as a two-level command

In the `switch command` block (around line 573), change:
```go
case "prompt", "spec":
```
to:
```go
case "prompt", "spec", "scenario":
```

### 6b. Update `runCommand` — dispatch to `runScenarioCommand`

In the `switch command` block in `runCommand` (around line 114), add after `case "spec":`:
```go
case "scenario":
    return runScenarioCommand(ctx, cfg, subcommand, args)
```

### 6c. Add `runScenarioCommand` function

Add after `runSpecCommand` (around line 290):

```go
func runScenarioCommand(
    ctx context.Context,
    cfg config.Config,
    subcommand string,
    args []string,
) error {
    switch subcommand {
    case "", "--help", "-h", "help":
        printScenarioHelp()
        return nil
    case "list":
        if err := validateNoArgs(ctx, args, printScenarioHelp); err != nil {
            return err
        }
        return factory.CreateScenarioListCommand(cfg).Run(ctx, args)
    case "show":
        if err := validateOneArg(ctx, args, printScenarioHelp); err != nil {
            return err
        }
        return factory.CreateScenarioShowCommand(cfg).Run(ctx, args)
    case "status":
        if err := validateNoArgs(ctx, args, printScenarioHelp); err != nil {
            return err
        }
        return factory.CreateScenarioStatusCommand(cfg).Run(ctx, args)
    default:
        return errors.Errorf(ctx, "unknown scenario subcommand: %s", subcommand)
    }
}
```

### 6d. Add `printScenarioHelp` function

Add after `printSpecHelp` (around line 540):

```go
func printScenarioHelp() {
    fmt.Fprintf(
        os.Stdout,
        "Usage: dark-factory scenario <subcommand>\n\nSubcommands:\n"+
            "  list          List scenarios with their status\n"+
            "  show <id>     Show full contents of a scenario\n"+
            "  status        Show scenario status counts\n",
    )
}
```

### 6e. Update `printHelp` — add scenario entries

In `printHelp` (around line 420), add after the spec lines (after `"  spec show <id>       Show details for a single spec\n\n"`):

```go
"  scenario list          List scenarios\n"+
"  scenario show <id>     Show full contents of a scenario\n"+
"  scenario status        Show scenario status counts\n\n"+
```

The final string should now list prompt, spec, and scenario command groups before the Options section.

### 6f. Update `printCommandHelp` — add scenario case

In `printCommandHelp` (around line 86), add after `case "spec":`:
```go
case "scenario":
    printScenarioHelp()
```

## 7. Write tests

### 7a. Create `pkg/cmd/scenario_list_test.go`

In `package cmd_test`. Follow the patterns in `pkg/cmd/spec_list_test.go` or `pkg/cmd/spec_show_test.go`.

Required test cases:
- **Empty directory**: Create a `ScenarioLister` fake that returns `([]*scenario.ScenarioFile{}, nil)`. Expect `Run(ctx, nil)` to succeed and print only the header line.
- **Normal output**: Fake returns two ScenarioFiles (e.g., Number=1 status="active" Title="First scenario", Number=2 status="draft" Title="Second scenario"). Expect table rows with 001/002 numbers in correct columns.
- **Unknown status**: File with empty `Frontmatter.Status`. Expect status shown as "unknown".
- **JSON output**: Pass `["--json"]`. Expect valid JSON array output.

Use `mocks.ScenarioLister` (counterfeiter fake) to inject test data. Capture output via `os.Stdout` redirection or a writer — follow the pattern used in other command tests in `pkg/cmd/`.

### 7b. Create `pkg/cmd/scenario_show_test.go`

Required test cases:
- **Not found (0 matches)**: Fake returns empty slice. Expect non-zero error with "no scenario matching".
- **Found (1 match)**: Fake returns one ScenarioFile with `RawContent: []byte("# title\nbody")`. Expect that content written to stdout, no error.
- **Multiple matches**: Fake returns two files. Expect non-zero error with "ambiguous".
- **Missing ID arg**: Call `Run(ctx, nil)`. Expect error "scenario identifier required".

### 7c. Create `pkg/cmd/scenario_status_test.go`

Required test cases:
- **Normal output**: Fake Summary returns `{Idea: 2, Draft: 0, Active: 3, Outdated: 1, Unknown: 0, Total: 6}`. Expect lines "idea: 2", "draft: 0", "active: 3", "outdated: 1". Expect NO "unknown" line when Unknown==0.
- **With unknown**: Summary has `Unknown: 2`. Expect "unknown: 2" line present.
- **JSON output**: Pass `["--json"]`. Expect valid JSON.
- **Summary error**: Fake `Summary` returns an error. Expect `Run` to return an error.

## 8. Write CHANGELOG entry

If `## Unreleased` already exists in `CHANGELOG.md` (added by prompt 1), append to it. Otherwise add it. Append:

```
- feat: add `scenario list`, `scenario show`, and `scenario status` CLI subcommands for read-only scenario inspection
```

## 9. Run `make test`

```bash
cd /workspace && make test
```

Must pass before `make precommit`.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do not touch `go.mod` / `go.sum` / `vendor/`
- Existing `prompt` and `spec` command behavior must not change — do not modify any existing command files or their tests
- The `scenario show` command prints the **full raw file content** (not a structured summary) — use `matches[0].RawContent`, not a formatted struct
- `scenario show` exits non-zero when 0 OR multiple matches — it never silently picks one from multiple
- `scenario list` and `scenario status` exit 0 with empty output when `scenarios/` does not exist (the Lister handles this)
- `scenario status` always prints the four known statuses (even with count 0); the `unknown` row is added ONLY when `summary.Unknown > 0`
- Factory functions are zero-logic: only wiring, no conditionals, no business logic
- `scenariosDir = "scenarios"` is a package-level constant in factory.go (relative to git root, which is the cwd at startup)
- Follow the counterfeiter directive format exactly: `//counterfeiter:generate -o ../../mocks/<name>.go --fake-name <FakeName> . <InterfaceName>`
- External test package (`package cmd_test`)
- Coverage ≥80% for new files in `pkg/cmd/`
- Use `errors.Wrap` / `errors.Errorf` from `github.com/bborbe/errors` — never `fmt.Errorf`
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Spot checks:
1. `dark-factory scenario list` — prints table header + 10 rows (one per scenario in `scenarios/`)
2. `dark-factory scenario show 001` — prints full contents of `scenarios/001-workflow-direct.md`
3. `dark-factory scenario show workflow-direct` — prints same file as above
4. `dark-factory scenario status` — prints counts for idea/draft/active/outdated (and unknown if any)
5. `dark-factory scenario` — prints the scenario help summary and exits 0
6. `dark-factory help` — output includes "scenario list", "scenario show", "scenario status"
7. `dark-factory prompt list` — still works correctly (no regression)
8. `go test ./pkg/cmd/... ./pkg/scenario/...` — passes with coverage ≥80%
9. `ls mocks/scenario-*.go` — three mock files exist
</verification>
