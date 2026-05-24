---
status: draft
created: "2026-05-24T00:00:00Z"
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
