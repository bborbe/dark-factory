---
status: completed
container: dark-factory-046-add-timestamp-frontmatter
dark-factory-version: v0.10.2
---





# Add timestamp fields to prompt frontmatter

Track the full lifecycle of each prompt with ISO 8601 timestamps in frontmatter.

## New Fields

| Field | Set when | Format |
|-------|----------|--------|
| `created` | first seen (normalize or queue) | `2026-03-01T22:30:00Z` |
| `queued` | status set to `queued` | `2026-03-01T22:35:00Z` |
| `started` | status set to `executing` | `2026-03-01T22:36:00Z` |
| `completed` | status set to `completed` or `failed` | `2026-03-01T22:45:00Z` |

## Rules

- `created`: set to `time.Now().UTC()` when the file is first seen during normalize. Only if not already present. Never overwrite.
- `queued`: set whenever `SetStatus(ctx, path, StatusQueued)` is called.
- `started`: set whenever `SetStatus(ctx, path, StatusExecuting)` is called.
- `completed`: set whenever `SetStatus(ctx, path, StatusCompleted)` or `SetStatus(ctx, path, StatusFailed)` is called.
- All timestamps use `time.Now().UTC().Format(time.RFC3339)`.
- Existing fields are never overwritten — if `queued` is already set (e.g. retry after failure), keep the original value.

## Implementation

### 1. Add fields to `Frontmatter` struct

In `pkg/prompt/prompt.go`, add to the `Frontmatter` struct:

```go
type Frontmatter struct {
    Status             string `yaml:"status"`
    Container          string `yaml:"container,omitempty"`
    DarkFactoryVersion string `yaml:"dark-factory-version,omitempty"`
    Created            string `yaml:"created,omitempty"`
    Queued             string `yaml:"queued,omitempty"`
    Started            string `yaml:"started,omitempty"`
    Completed          string `yaml:"completed,omitempty"`
}
```

### 2. Set timestamps in `SetStatus`

Modify `SetStatus` (or add a new `SetStatusWithTimestamp`) to automatically set the corresponding timestamp field when status changes:

- `StatusQueued` → set `queued` (if empty)
- `StatusExecuting` → set `started` (always overwrite — retries get fresh start time)
- `StatusCompleted` → set `completed` (always overwrite)
- `StatusFailed` → set `completed` (always overwrite — marks when failure happened)

### 3. Set `created` during normalize

In `NormalizeFilenames`, when a file is first seen (during rename/normalization), set `created` to `time.Now().UTC().Format(time.RFC3339)` if the field is empty.

### 4. Update status display

In `pkg/status/status.go`, include timestamps in the status output so `dark-factory status` shows timing info for executing/completed prompts.

## Example

A completed prompt would look like:

```yaml
---
status: completed
container: dark-factory-041-fix-security
dark-factory-version: v0.10.2
created: 2026-03-01T22:30:00Z
queued: 2026-03-01T22:35:12Z
started: 2026-03-01T22:35:15Z
completed: 2026-03-01T22:45:33Z
---
```

## Verification

Run `make precommit` — must pass.
