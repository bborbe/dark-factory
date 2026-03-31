---
status: completed
spec: [012-duplicate-frontmatter-handling]
container: dark-factory-049-rewrite-prompt-file-load-save
dark-factory-version: dev
created: "2026-03-02T20:17:47Z"
started: "2026-03-02T20:17:47Z"
completed: "2026-03-02T20:36:09Z"
---
<objective>
Rewrite prompt file handling to use a Load/Save pattern instead of per-field setField calls.
Currently each frontmatter change (status, container, version, timestamps) does a separate
read-parse-modify-write cycle (6 per lifecycle). This causes body loss under concurrent access.
Replace with: Load once → modify struct → Save once. Body is immutable after Load.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/prompt/prompt.go` — current implementation with `setField`, `splitFrontmatter`,
  `addFrontmatterWithSetter`, `updateExistingFrontmatterWithSetter`, `SetStatus`, `SetContainer`,
  `SetVersion`, `ensureCreatedTimestamp`, `Content`, `Title`, `MoveToCompleted`.
Read `pkg/processor/processor.go` — caller: `setupPromptMetadata` calls SetContainer, SetVersion,
  SetStatus separately. `processPrompt` calls Content, Title, MoveToCompleted.
Read `pkg/watcher/watcher.go` — calls `NormalizeFilenames` which calls `ensureCreatedTimestamp` on ALL files.
Read `pkg/prompt/prompt_test.go` — existing tests.
</context>

<requirements>

## 1. Add PromptFile struct to `pkg/prompt/prompt.go`

```go
// PromptFile represents a loaded prompt file with immutable body and mutable frontmatter.
type PromptFile struct {
    Path        string
    Frontmatter Frontmatter
    Body        []byte // immutable after Load — never modified
}
```

## 2. Add Load function

```go
// Load reads a prompt file from disk, parsing frontmatter and body.
// Body is stored as-is and never modified by Save.
func Load(ctx context.Context, path string) (*PromptFile, error) {
    content, err := os.ReadFile(path)
    if err != nil {
        return nil, errors.Wrap(ctx, err, "read file")
    }

    var fm Frontmatter
    body, err := frontmatter.Parse(bytes.NewReader(content), &fm)
    if err != nil {
        // No frontmatter — entire file is body
        return &PromptFile{Path: path, Body: content}, nil
    }

    return &PromptFile{
        Path:        path,
        Frontmatter: fm,
        Body:        body,
    }, nil
}
```

## 3. Add Save function

```go
// Save writes the prompt file back to disk: frontmatter + body.
// Body is always preserved exactly as loaded.
func (pf *PromptFile) Save() error {
    fm, err := yaml.Marshal(&pf.Frontmatter)
    if err != nil {
        return fmt.Errorf("marshal frontmatter: %w", err)
    }

    var buf bytes.Buffer
    buf.WriteString("---\n")
    buf.Write(fm)
    buf.WriteString("---\n")
    buf.Write(pf.Body)
    return os.WriteFile(pf.Path, buf.Bytes(), 0600)
}
```

## 4. Add Content and Title methods on PromptFile

```go
// Content returns the body as a string, stripped of leading empty frontmatter blocks.
// Returns ErrEmptyPrompt if body is empty or whitespace-only.
func (pf *PromptFile) Content() (string, error) {
    result := strings.TrimSpace(string(pf.Body))
    if len(result) == 0 {
        return "", ErrEmptyPrompt
    }
    return string(pf.Body), nil
}

// Title extracts the first # heading from the body.
func (pf *PromptFile) Title() string {
    scanner := bufio.NewScanner(bytes.NewReader(pf.Body))
    for scanner.Scan() {
        line := strings.TrimSpace(scanner.Text())
        if strings.HasPrefix(line, "# ") {
            return strings.TrimPrefix(line, "# ")
        }
    }
    return ""
}
```

## 5. Add PrepareForExecution method

Replace the 3 separate calls (SetContainer, SetVersion, SetStatus executing) with one method:

```go
// PrepareForExecution sets all fields needed before container launch.
// This replaces separate SetContainer + SetVersion + SetStatus calls.
func (pf *PromptFile) PrepareForExecution(container, version string) {
    now := time.Now().UTC().Format(time.RFC3339)
    pf.Frontmatter.Status = string(StatusExecuting)
    pf.Frontmatter.Container = container
    pf.Frontmatter.DarkFactoryVersion = version
    pf.Frontmatter.Started = now
    // Ensure created/queued timestamps exist
    if pf.Frontmatter.Created == "" {
        pf.Frontmatter.Created = now
    }
    if pf.Frontmatter.Queued == "" {
        pf.Frontmatter.Queued = now
    }
}
```

## 6. Add MarkCompleted and MarkFailed methods

```go
func (pf *PromptFile) MarkCompleted() {
    pf.Frontmatter.Status = string(StatusCompleted)
    pf.Frontmatter.Completed = time.Now().UTC().Format(time.RFC3339)
}

func (pf *PromptFile) MarkFailed() {
    pf.Frontmatter.Status = string(StatusFailed)
    pf.Frontmatter.Completed = time.Now().UTC().Format(time.RFC3339)
}

func (pf *PromptFile) MarkQueued() {
    now := time.Now().UTC().Format(time.RFC3339)
    pf.Frontmatter.Status = string(StatusQueued)
    if pf.Frontmatter.Created == "" {
        pf.Frontmatter.Created = now
    }
    if pf.Frontmatter.Queued == "" {
        pf.Frontmatter.Queued = now
    }
}
```

## 7. Update Manager interface

Add new methods to the Manager interface:

```go
type Manager interface {
    // Existing methods that stay
    ResetExecuting(ctx context.Context) error
    ResetFailed(ctx context.Context) error
    HasExecuting(ctx context.Context) bool
    ListQueued(ctx context.Context) ([]Prompt, error)
    MoveToCompleted(ctx context.Context, path string) error
    NormalizeFilenames(ctx context.Context, dir string) ([]Rename, error)
    AllPreviousCompleted(ctx context.Context, n int) bool

    // New Load-based methods
    Load(ctx context.Context, path string) (*PromptFile, error)
}
```

## 8. Update processor to use Load/Save

In `pkg/processor/processor.go`, replace `setupPromptMetadata` with:

```go
func (p *processor) setupPromptMetadata(ctx context.Context, path string) (string, string, *prompt.PromptFile, error) {
    baseName := strings.TrimSuffix(filepath.Base(path), ".md")
    baseName = sanitizeContainerName(baseName)
    containerName := "dark-factory-" + baseName

    pf, err := p.promptManager.Load(ctx, path)
    if err != nil {
        return "", "", nil, errors.Wrap(ctx, err, "load prompt")
    }

    pf.PrepareForExecution(containerName, p.versionGetter.Get())
    if err := pf.Save(); err != nil {
        return "", "", nil, errors.Wrap(ctx, err, "save prompt")
    }

    return baseName, containerName, pf, nil
}
```

Update `processPrompt` to use the loaded PromptFile for content and title (no separate Content/Title calls needed):

```go
func (p *processor) processPrompt(ctx context.Context, pr prompt.Prompt) error {
    // Load prompt file once
    pf, err := p.promptManager.Load(ctx, pr.Path)
    if err != nil {
        return errors.Wrap(ctx, err, "load prompt")
    }

    // Check if empty
    content, err := pf.Content()
    if err != nil {
        if stderrors.Is(err, prompt.ErrEmptyPrompt) {
            // handle empty...
        }
        return errors.Wrap(ctx, err, "get content")
    }

    // Prepare for execution (one Save)
    baseName := strings.TrimSuffix(filepath.Base(pr.Path), ".md")
    baseName = sanitizeContainerName(baseName)
    containerName := "dark-factory-" + baseName

    pf.PrepareForExecution(containerName, p.versionGetter.Get())
    if err := pf.Save(); err != nil {
        return errors.Wrap(ctx, err, "save prompt metadata")
    }

    title := pf.Title()
    // ... rest of execution ...

    // Append suffix and execute
    content = content + report.Suffix()
    // ... executor.Execute ...

    // Complete (one Save + move)
    pf.MarkCompleted()
    if err := pf.Save(); err != nil {
        return errors.Wrap(ctx, err, "save completed status")
    }
    // Then move file...
}
```

## 9. Update NormalizeFilenames

Remove the `ensureCreatedTimestamp` loop. `NormalizeFilenames` should ONLY rename files.
Delete the `ensureCreatedTimestamp` function.
Created timestamp is now set by `MarkQueued()` or `PrepareForExecution()`.

## 10. Update ResetExecuting and ResetFailed

These should use Load/Save pattern too:

```go
func ResetExecuting(ctx context.Context, dir string) error {
    // ... iterate files ...
    pf, err := Load(ctx, path)
    if err != nil { continue }
    if pf.Frontmatter.Status == string(StatusExecuting) {
        pf.MarkQueued()
        pf.Save()
    }
}
```

## 11. Delete old functions

Remove these functions (no longer needed):
- `setField`
- `splitFrontmatter`
- `addFrontmatterWithSetter`
- `updateExistingFrontmatterWithSetter`
- `ensureCreatedTimestamp`
- `stripLeadingEmptyFrontmatter`
- Standalone `SetStatus`, `SetContainer`, `SetVersion` functions
- Standalone `Content` and `Title` functions
- `ReadFrontmatter` standalone function (replace with Load)

Keep the Manager interface methods but implement them using Load/Save internally.

## 12. Update tests

Update existing tests to work with the new API. Key test cases:
- Load a file without frontmatter → Body contains full content
- Load a file with frontmatter → Frontmatter parsed, Body is the rest
- Save preserves Body exactly as loaded (byte-for-byte)
- PrepareForExecution sets all fields in one call
- MarkCompleted/MarkFailed set status and timestamp
- 20 Load/Save cycles → Body size stays constant (regression test)
- NormalizeFilenames does NOT modify file contents (only renames)
- Full lifecycle test: Load → MarkQueued → Save → Load → PrepareForExecution → Save → Load → MarkCompleted → Save → verify body preserved

## 13. Regenerate mocks

Run `go generate ./...` to regenerate counterfeiter mocks after interface changes.
</requirements>

<constraints>
- Do NOT modify `pkg/executor/` — executor is unchanged
- Do NOT modify `pkg/report/` — report package is unchanged
- Do NOT modify `pkg/watcher/watcher.go` structure — only the NormalizeFilenames behavior changes (no more ensureCreatedTimestamp)
- Body must be preserved byte-for-byte through any number of Load/Save cycles
- Backwards compatible: files without frontmatter still work (Load handles both cases)
- Keep the Manager interface — update methods but don't remove the interface pattern
- Use `github.com/adrg/frontmatter` for parsing (already a dependency)
- Use Ginkgo v2 + Gomega for tests, Counterfeiter for mocks
</constraints>

<verification>
Run: `make test`
Run: `go test -v ./pkg/prompt/...`
Run: `go test -v ./pkg/processor/...`
Run: `go test -v ./pkg/watcher/...`
Run: `make precommit`
</verification>

<success_criteria>
- `PromptFile` struct with Load/Save pattern
- Body preserved byte-for-byte through unlimited Load/Save cycles
- `setupPromptMetadata` does ONE Save instead of THREE setField calls
- `MoveToCompleted` does ONE Save
- `NormalizeFilenames` only renames, never modifies content
- Old setField/splitFrontmatter/addFrontmatter functions deleted
- All tests pass
- `make precommit` passes
</success_criteria>
