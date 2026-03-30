---
status: approved
created: "2026-03-30T15:08:40Z"
queued: "2026-03-30T15:08:40Z"
---

<summary>
- Update default container image from v0.4.3 to v0.5.0
- Single constant change plus changelog entry
</summary>

<objective>
Bump the default claude-yolo container image to v0.5.0.
</objective>

<context>
Read CLAUDE.md for project conventions.

Key file:
- `pkg/const.go` — `DefaultContainerImage` constant (line 9)
</context>

<requirements>
### 1. Update container image version

In `pkg/const.go`, change:
```go
const DefaultContainerImage = "docker.io/bborbe/claude-yolo:v0.4.3"
```
to:
```go
const DefaultContainerImage = "docker.io/bborbe/claude-yolo:v0.5.0"
```

### 2. Update CHANGELOG.md

Add to the `## Unreleased` section (create if missing, above the first version entry):
```
- chore: Bump default claude-yolo image to v0.5.0
```
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Only change `pkg/const.go` and `CHANGELOG.md`
</constraints>

<verification>
```bash
make precommit
```
Must exit 0.
</verification>
