---
status: completed
summary: Replaced token configuration leak in GitHub and Bitbucket warning messages with opaque fallback message
container: dark-factory-exec-420-review-dark-factory-16-fix-token-warning-log
dark-factory-version: v0.171.1-3-gd94f1fa
created: "2026-05-24T00:00:00Z"
queued: "2026-05-25T14:51:20Z"
started: "2026-05-25T18:48:22Z"
completed: "2026-05-25T18:50:43Z"
---

<summary>
- Redacted GitHub/Bitbucket token warning messages that revealed whether a token was configured
- Changed from "github.token configured but env var is empty" to opaque "token env var not set"
</summary>

<objective>
Fix token warning messages that leak whether a token is configured in the config file.
</objective>

<context>
Files to read before making changes:
- `pkg/config/config.go` — line ~662 (GitHub token warning), line ~677 (Bitbucket token warning)
</context>

<requirements>
1. In `pkg/config/config.go`, change the GitHub token warning from:
   ```
   slog.Warn("github.token configured but env var is empty, using default gh auth")
   ```
   to:
   ```
   slog.Warn("token env var not set; falling back to default auth")
   ```

2. Similarly fix the Bitbucket token warning at line ~677.
</requirements>

<constraints>
- Only change files in this repo
- Do NOT commit — dark-factory handles git
</constraints>

<verification>
make precommit
</verification>
