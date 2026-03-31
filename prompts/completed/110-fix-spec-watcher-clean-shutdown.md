---
status: completed
spec: [020-auto-prompt-generation]
summary: 'In handleFileEvent, differentiate context-cancelled errors from real failures: log ''spec generation cancelled'' at Info (no stack trace) when ctx is done or err is context.Canceled, otherwise log ''spec generation failed'' as before; added captureHandler and test to verify the correct log message on cancellation.'
container: dark-factory-110-fix-spec-watcher-clean-shutdown
dark-factory-version: v0.18.2
created: "2026-03-06T17:30:00Z"
queued: "2026-03-06T16:28:09Z"
started: "2026-03-06T16:28:09Z"
completed: "2026-03-06T16:41:28Z"
---

<objective>
Suppress the noisy stack trace when SpecWatcher generation is cancelled by context (Ctrl+C shutdown).
</objective>

<context>
When dark-factory shuts down (SIGINT), the running spec generator receives a cancelled context and returns an error. Currently this is logged as "spec generation failed" with a full stack trace, which looks like a real failure. It is not — it is a clean shutdown.

Read pkg/specwatcher/watcher.go — the handleFileEvent method logs the error unconditionally.
</context>

<requirements>
1. In handleFileEvent (and any other place that logs spec generation errors), check if the error is due to context cancellation:
   - If `ctx.Err() != nil` or `errors.Is(err, context.Canceled)` → log at Info level: "spec generation cancelled" with the spec path, no stack trace
   - Otherwise → log at Error/Warn level with full error as before

2. Same pattern for scanExistingApproved if it logs errors directly.

3. Add test: cancelled context during generation → logs "cancelled" not "failed".
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- make precommit must pass
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
