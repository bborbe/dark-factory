# Definition of Done

After completing your implementation, review your own changes against each criterion below. These are quality checks you perform by inspecting your work — not commands to run (linting and tests already ran via `validationCommand`). Report any unmet criterion as a blocker.

## Code Quality

- Exported types, functions, and interfaces have doc comments
- Error handling follows project conventions (no silently ignored errors)
- No debug output (print statements, fmt.Printf) — use structured logging

## Testing

- New code has good test coverage (target >= 80%)
- Changes to existing code have tests covering at least the changed behavior

## Dependencies

- If `make precommit` fails due to a dependency vulnerability with a known fix version, update the dependency (`go get <pkg>@<fixed-version> && go mod tidy && go mod vendor`) as part of your change
- `go install github.com/bborbe/dark-factory@latest` works
- No `exclude` or `replace` directives in go.mod (break remote install)

## Documentation

- README.md is updated if the change affects usage, configuration, or setup
- Documentation is updated if the change affects behavior described in docs/
- When changing CLI args, config fields, env vars, or flags (add, rename, remove, change default), grep the entire repo and update all references in docs/, README.md, examples, and comments
- CHANGELOG.md has an entry under `## Unreleased`
- New or changed integration seam (per `docs/scenario-writing.md` "When to Write a Scenario") → scenario added or updated
