---
status: completed
container: dark-factory-031-add-version-to-frontmatter
---


# Add dark-factory version to prompt frontmatter

## Goal

Track which dark-factory version processed each prompt by adding `dark-factory-version` to the frontmatter. Useful for debugging and auditing.

## Expected Behavior

When dark-factory picks up a prompt:
```yaml
---
status: executing
container: dark-factory-027-add-instance-lock
dark-factory-version: v0.2.37
---
```

After completion in `prompts/completed/`:
```yaml
---
status: completed
container: dark-factory-027-add-instance-lock
dark-factory-version: v0.2.37
---
```

## Implementation

### 1. Embed version at build time

Use `go build -ldflags` to set version from git tag:

```go
// main.go or pkg/version/version.go
var Version = "dev"  // overridden by -ldflags "-X main.Version=v0.2.37"
```

Update `Makefile` run/build targets:
```makefile
VERSION := $(shell git describe --tags --abbrev=0 2>/dev/null || echo "dev")
LDFLAGS := -X main.Version=$(VERSION)

build:
	go build -ldflags "$(LDFLAGS)" -o dark-factory .

run:
	go run -ldflags "$(LDFLAGS)" main.go
```

### 2. Pass version through to prompt manager

- Add version parameter to `NewRunner()` or inject via factory
- When setting `status: executing`, also set `dark-factory-version: <version>`
- `SetStatus()` in prompt manager should accept optional metadata fields

### 3. Tests

- Version appears in frontmatter after status change
- Default "dev" version when not built with ldflags
- Version preserved through move to completed

## Constraints

- Run `make precommit` for validation only
- Do NOT commit, tag, or push (dark-factory handles git)
- Follow `~/.claude-yolo/docs/go-patterns.md`
- Follow `~/.claude-yolo/docs/go-composition.md` (inject version, don't use global)
- Coverage ≥80% for changed packages
