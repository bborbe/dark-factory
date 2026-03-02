---
status: completed
summary: Added version logging on startup to display dark-factory version when the application starts
container: dark-factory-056-log-version-on-startup
dark-factory-version: v0.13.1
created: "2026-03-02T22:56:57Z"
queued: "2026-03-02T22:56:57Z"
started: "2026-03-02T22:56:57Z"
completed: "2026-03-02T23:01:27Z"
---
<objective>
Log the dark-factory version on startup so operators can see which build is running.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `main.go` — `run()` function, slog configuration, command routing.
Read `pkg/version/version.go` — `Version` variable, set via ldflags at build time.
</context>

<requirements>

## 1. Log version after slog is configured

In `main.go`, after the slog handler is set up and before loading config, add:

```go
slog.Info("dark-factory starting", "version", version.Version)
```

This uses the package-level `version.Version` variable directly — no need for the Getter interface here since we're in main.

## 2. That's it

No other changes needed. The version is already set via ldflags in the Makefile.

</requirements>

<constraints>
- Single line change in main.go
- Do NOT change any other files
- Do NOT modify function signatures
- Use `version.Version` directly (already imported)
</constraints>

<verification>
Run: `make test`
Run: `make precommit`
Run: `go build -o /tmp/df . && /tmp/df status` (verify version appears in startup log)
</verification>

<success_criteria>
- `dark-factory starting version=dev` appears in stderr on startup
- All tests pass
- `make precommit` passes
</success_criteria>
