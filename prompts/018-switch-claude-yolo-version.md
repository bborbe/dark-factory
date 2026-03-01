# Pin claude-yolo image to a specific version tag

## Goal

Replace `docker.io/bborbe/claude-yolo:latest` with a pinned version tag so that
dark-factory uses a reproducible, auditable image rather than a floating tag.

## Current Behavior

`pkg/executor/executor.go` runs:
```
docker.io/bborbe/claude-yolo:latest
```

`latest` can change silently — a new push to `latest` could break the running system
without any indication in git history.

## Expected Behavior

```
docker.io/bborbe/claude-yolo:v0.0.7
```

- Image tag is pinned in source code
- To upgrade: change the version constant and commit

## Implementation

1. In `pkg/executor/executor.go`, replace the image string with a named constant:

```go
const claudeYoloImage = "docker.io/bborbe/claude-yolo:v0.0.7"
```

2. Use the constant in the `docker run` command instead of the inline string.

3. Update `CHANGELOG.md` to note the pinned version.

## Constraints

- Latest available tag as of now: `v0.0.7`
- Run `make precommit` for validation only — do NOT commit, tag, or push (dark-factory handles all git operations)
