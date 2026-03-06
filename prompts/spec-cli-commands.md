<objective>
Add `spec list`, `spec status`, and `spec approve` CLI commands so users can inspect and approve specs from the terminal. Wires into existing factory and main.go routing. Depends on `pkg/spec` (previous prompt).
</objective>

<context>
Read CLAUDE.md for project conventions.
Read pkg/cmd/list.go and pkg/cmd/status.go for existing command patterns to follow.
Read pkg/factory/factory.go for how commands are constructed and wired.
Read main.go for the current command routing switch.
Read pkg/spec/manager.go for the Manager interface (just added).
</context>

<requirements>
1. Create `pkg/cmd/spec_commands.go` with three command types following existing patterns:

   `SpecListCommand` interface + implementation:
   - `Run(ctx, args) error`
   - Lists all specs from Manager.List()
   - Output table: `%-10s %-30s %s\n` → STATUS, NAME, FILE
   - Supports `--json` flag (outputs JSON array with fields: status, name, file)

   `SpecStatusCommand` interface + implementation:
   - `Run(ctx, args) error`
   - Counts specs by status (draft/approved/prompted/completed)
   - Output: one line per status: `<count> <status>`

   `SpecApproveCommand` interface + implementation:
   - `Run(ctx, args) error`
   - First arg is the spec file path
   - Calls Manager.Approve(ctx, path)
   - Prints: `approved: <path>`
   - If no arg → returns error "usage: dark-factory spec approve <file>"

2. Add to `pkg/factory/factory.go`:
   - `CreateSpecListCommand(cfg config.Config) cmd.SpecListCommand`
   - `CreateSpecStatusCommand(cfg config.Config) cmd.SpecStatusCommand`
   - `CreateSpecApproveCommand(cfg config.Config) cmd.SpecApproveCommand`
   - Use `cfg.SpecsDir` (add this field to config if missing; default: "specs")

3. Add `SpecsDir string` field to `pkg/config/config.go` Config struct (yaml: "specsDir,omitempty") with default "specs" in Defaults().

4. Update `main.go` to handle `dark-factory spec <subcmd>`:
   - In `parseArgs`, recognize "spec" as a known top-level command
   - In the `run()` switch, add case "spec" that dispatches on args[0]:
     - "list" → CreateSpecListCommand
     - "status" → CreateSpecStatusCommand
     - "approve" → CreateSpecApproveCommand (pass args[1:])
     - unknown subcmd → error "unknown spec command: <subcmd>"
   - Update help text to include: `  spec     Manage specs (list, status, approve)`

5. Add tests to `pkg/cmd/spec_commands_test.go` and `pkg/cmd/spec_suite_test.go`:
   - SpecListCommand outputs table with correct columns
   - SpecStatusCommand outputs counts grouped by status
   - SpecApproveCommand calls Manager.Approve with correct path
   - SpecApproveCommand with no args returns usage error

6. Regenerate mocks with `go generate ./...`
</requirements>

<constraints>
- Follow existing pkg/cmd patterns exactly (interface + struct + New constructor)
- Do NOT modify existing commands
- Do NOT commit — dark-factory handles git
- Coverage ≥ 80% for pkg/cmd new files
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
