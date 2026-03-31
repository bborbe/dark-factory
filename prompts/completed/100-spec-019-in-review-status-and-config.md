---
status: completed
spec: [019-review-fix-loop]
summary: Added StatusInReview status constant, autoReview/maxReviewRetries/allowedReviewers/useCollaborators/pollIntervalSec config fields with validation and tests
container: dark-factory-100-spec-019-in-review-status-and-config
dark-factory-version: v0.17.29
created: "2026-03-06T14:21:44Z"
queued: "2026-03-06T14:21:44Z"
started: "2026-03-06T14:21:44Z"
completed: "2026-03-06T14:31:56Z"
---
<objective>
Add `in_review` as a new prompt status and add config fields for the review-fix loop (spec 018). This is the foundation — no polling logic yet, just the data model and config.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read pkg/prompt/prompt.go for existing status constants and Frontmatter struct.
Read pkg/config/config.go for existing config fields and Defaults()/Validate() patterns.
</context>

<requirements>
1. In `pkg/prompt/prompt.go`, add `StatusInReview Status = "in_review"` alongside existing status constants.

2. In `pkg/config/config.go`, add these fields to the `Config` struct:
   ```go
   AutoReview        bool     `yaml:"autoReview"`
   MaxReviewRetries  int      `yaml:"maxReviewRetries"`
   AllowedReviewers  []string `yaml:"allowedReviewers,omitempty"`
   UseCollaborators  bool     `yaml:"useCollaborators"`
   PollIntervalSec   int      `yaml:"pollIntervalSec"`
   ```
   In `Defaults()`, set: `AutoReview: false`, `MaxReviewRetries: 3`, `PollIntervalSec: 60`, `UseCollaborators: false`.

3. In `Validate()`, add:
   - `autoReview: true` requires `workflow: pr` or `workflow: worktree` → error "autoReview requires workflow 'pr' or 'worktree'"
   - `autoReview: true` requires `autoMerge: true` → error "autoReview requires autoMerge"
   - `autoReview: true` with both `allowedReviewers` empty and `useCollaborators: false` → error "autoReview requires allowedReviewers or useCollaborators: true"

4. Update `example/.dark-factory.yaml` with commented-out examples for all new fields:
   ```yaml
   # autoReview: false        # watch PRs for reviews and generate fix prompts
   # maxReviewRetries: 3      # max fix iterations before marking failed
   # allowedReviewers: []     # trusted GitHub usernames (or use useCollaborators)
   # useCollaborators: false  # trust repo collaborators as reviewers
   # pollIntervalSec: 60      # how often to poll in_review prompts (seconds)
   ```

5. Add tests to `pkg/config/config_test.go`:
   - Defaults set MaxReviewRetries=3, PollIntervalSec=60
   - autoReview=true + workflow=direct → validation error
   - autoReview=true + autoMerge=false → validation error
   - autoReview=true + no reviewer source → validation error
   - autoReview=true + autoMerge=true + workflow=pr + allowedReviewers=["bborbe"] → no error

6. Add test to `pkg/prompt/prompt_test.go`:
   - StatusInReview is a valid status value (parse from frontmatter)
</requirements>

<constraints>
- Follow existing config validation patterns exactly (validation.Name, validation.HasValidationFunc)
- Do NOT modify existing status constants or config fields
- Do NOT commit — dark-factory handles git
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
