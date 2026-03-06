<objective>
Create a `pkg/spec` package that loads and parses spec files from a directory. This is the data layer for spec 019 (native spec integration) — all other spec commands depend on it.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read pkg/prompt/prompt.go for the existing PromptFile/Frontmatter patterns to follow exactly.
Specs live in a directory (default: `specs/`) as markdown files with YAML frontmatter.
Spec frontmatter has a `status` field: draft, approved, prompted, or completed.
</context>

<requirements>
1. Create `pkg/spec/spec.go` with:
   - `Frontmatter` struct: `Status string` (yaml: "status,omitempty")
   - `SpecFile` struct: `Path string`, `Name string` (filename without .md), `Frontmatter Frontmatter`
   - `Load(ctx, path string) (*SpecFile, error)` — reads file, parses YAML frontmatter (same approach as prompt.Load)
   - `Status() string` getter on SpecFile returning Frontmatter.Status (defaults to "draft" if empty)

2. Create `pkg/spec/manager.go` with:
   - `Manager` interface with counterfeiter directive:
     ```
     List(ctx context.Context) ([]*SpecFile, error)
     Approve(ctx context.Context, path string) error
     ```
   - `NewManager(specsDir string) Manager` constructor
   - `List` implementation: reads all `.md` files from specsDir (returns empty slice if dir not found, no error)
   - `Approve` implementation: reads file, sets `status: approved` in frontmatter, writes file back

3. Create `pkg/spec/spec_test.go` and `pkg/spec/spec_suite_test.go`:
   - `Load` parses status correctly from frontmatter
   - `Load` returns "draft" status when no frontmatter present
   - `Manager.List` returns empty slice when specsDir does not exist
   - `Manager.Approve` sets status to approved in file

4. Generate mocks: add counterfeiter directive to Manager interface, run `go generate ./...`
</requirements>

<constraints>
- Follow pkg/prompt patterns exactly: YAML frontmatter parsing, error wrapping, slog.Debug logging
- Do NOT modify any existing files
- Do NOT commit — dark-factory handles git
- Coverage ≥ 80% for pkg/spec
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
