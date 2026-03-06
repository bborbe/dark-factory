---
status: completed
summary: 'Added spec package foundations: Status type with constants, Frontmatter/SpecFile structs, Load/Save/SetStatus, Lister interface with List/Summary, wired AutoCompleter into processor, updated all callers'
container: dark-factory-087-spec-019-spec-model
dark-factory-version: v0.17.15
created: "2026-03-06T10:57:15Z"
queued: "2026-03-06T10:57:15Z"
started: "2026-03-06T11:02:43Z"
completed: "2026-03-06T11:14:34Z"
---
<objective>
Add a spec package that can parse spec files from a directory, extract frontmatter (status field), and provide listing and summary functionality. This is the foundation for native spec integration (spec 019).
</objective>

<context>
Read CLAUDE.md for project conventions.
Read pkg/prompt/prompt.go for how prompt frontmatter is parsed (reuse the YAML parsing pattern).
Read specs/ directory for real spec files to understand the frontmatter format (status field: draft/approved/prompted/completed).
</context>

<requirements>
1. Create `pkg/spec/spec.go` with:
   - `Status` type (string) with constants: `StatusDraft`, `StatusApproved`, `StatusPrompted`, `StatusCompleted`
   - `Frontmatter` struct with `Status string` and `Tags []string` yaml fields
   - `SpecFile` struct with `Path string`, `Frontmatter`, `Name string` (filename without extension)
   - `Load(ctx, path) (*SpecFile, error)` — parse a spec file, extract YAML frontmatter
   - `SetStatus(status string)` method on SpecFile
   - `Save(ctx) error` method on SpecFile — write back frontmatter + body (same pattern as prompt.Save)

2. Create `pkg/spec/lister.go` with:
   - `Lister` interface with `List(ctx) ([]*SpecFile, error)` and `Summary(ctx) (*Summary, error)`
   - `Summary` struct with counts per status: `Draft int`, `Approved int`, `Prompted int`, `Completed int`, `Total int`
   - Implementation that scans a directory for `.md` files and parses each

3. Create `pkg/spec/spec_test.go` with tests:
   - Load a spec file with valid frontmatter → status parsed correctly
   - Load a spec file without frontmatter → status is empty string
   - Summary counts specs by status correctly
   - SetStatus + Save roundtrips correctly

4. Regenerate mocks with `go generate ./...` if counterfeiter annotations are added.
</requirements>

<constraints>
- Follow existing code patterns in pkg/prompt/ for YAML frontmatter parsing
- Use `github.com/bborbe/errors` for error wrapping
- Spec directory path is passed as a parameter (not hardcoded)
- `make precommit` must pass
- Do NOT commit, tag, or push
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
