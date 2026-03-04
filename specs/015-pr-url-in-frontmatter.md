## PR URL in Frontmatter

### Problem

When dark-factory creates a PR via the `pr` workflow, the PR URL is logged but not persisted in the prompt's frontmatter. This makes it hard to find the associated PR for a completed prompt.

### Solution

Add a `pr-url` field to the prompt frontmatter. After `prCreator.Create()` returns the URL, store it in the prompt file before switching back to the original branch.

### Acceptance Criteria

1. `Frontmatter` struct has `PRURL string \`yaml:"pr-url,omitempty"\`` field
2. `PromptFile` has a `SetPRURL(url string)` method
3. `handlePRWorkflow` saves the PR URL to frontmatter after successful PR creation
4. The prompt file is saved to disk before switching branches (so the field persists on the feature branch)
5. Completed prompts show `pr-url: https://github.com/...` in frontmatter
6. Direct and worktree workflows are unaffected (no `pr-url` field added)
7. Existing prompts without `pr-url` parse without error (backward compatible)
8. `make precommit` passes
