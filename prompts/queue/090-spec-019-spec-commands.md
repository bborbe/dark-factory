---
status: queued
created: "2026-03-06T10:57:15Z"
queued: "2026-03-06T10:57:15Z"
---
<objective>
Implement the spec CLI commands: `spec list`, `spec status`, and `spec approve`. Replace the placeholders from the previous prompt with real implementations using the spec package.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read pkg/spec/spec.go and pkg/spec/lister.go for the spec model and lister.
Read pkg/cmd/list.go and pkg/cmd/status.go for the prompt command patterns to follow.
Read main.go for the current two-level command routing.
Read pkg/factory/ for how prompt commands are wired via Create* functions.
</context>

<requirements>
1. Create `pkg/cmd/spec_list.go`:
   - `SpecListCommand` interface with `Run(ctx, args) error`
   - Implementation uses `spec.Lister` to list all specs
   - Output format: `STATUS     FILE` table (similar to prompt list)
   - Support `--json` flag for JSON output
   - Tests in `pkg/cmd/spec_list_test.go`

2. Create `pkg/cmd/spec_status.go`:
   - `SpecStatusCommand` interface with `Run(ctx, args) error`
   - Implementation uses `spec.Lister.Summary()` to show counts
   - Output format: `Specs: N total (N draft, N approved, N prompted, N completed)`
   - Support `--json` flag
   - Tests in `pkg/cmd/spec_status_test.go`

3. Create `pkg/cmd/spec_approve.go`:
   - `SpecApproveCommand` interface with `Run(ctx, args) error`
   - Takes a spec identifier as argument (filename or NNN prefix match)
   - Loads the spec, sets status to "approved", saves
   - Error if spec not found or already approved
   - Tests in `pkg/cmd/spec_approve_test.go`

4. Add factory functions in `pkg/factory/`:
   - `CreateSpecListCommand(cfg) cmd.SpecListCommand`
   - `CreateSpecStatusCommand(cfg) cmd.SpecStatusCommand`
   - `CreateSpecApproveCommand(cfg) cmd.SpecApproveCommand`
   - Use `cfg.SpecDir` (add `SpecDir string` field to config with default `"specs"`)

5. Wire into main.go: replace spec placeholders with real factory calls.

6. Add `specDir` to config:
   - Add `SpecDir string \`yaml:"specDir"\`` to `Config` struct in `pkg/config/config.go`
   - Default: `"specs"` in `Defaults()`
   - Add to config tests
</requirements>

<constraints>
- Follow existing patterns in pkg/cmd/ exactly (interface, struct, constructor, Run method)
- Use `github.com/bborbe/errors` for error wrapping
- `make precommit` must pass
- Do NOT commit, tag, or push
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
