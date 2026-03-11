---
status: draft
created: "2026-03-11T16:45:24Z"
queued: "2026-03-11T18:25:03Z"
---

<summary>
- Environment variable values in the config are validated for dangerous control characters
- Config loading rejects values containing null bytes, newlines, or carriage returns
- Docker environment injection via malicious YAML config values is prevented
- The validation is added to the `validateEnv` method (a method on `Config`, not a standalone function)
- Tests verify rejection of newlines, null bytes, carriage returns, and acceptance of normal values
</summary>

<objective>
Add validation to reject environment variable values containing control characters (`\x00`, `\n`, `\r`) in the config validation. These characters could be used to manipulate Docker container environment state if the config file is attacker-controlled.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/config/config.go` — find the `Validate` method and the existing `validateEnv` method (it is a method on `Config`, not a standalone function). The current validation checks for empty keys and reserved key names but does not validate values.
Read `pkg/executor/executor.go` — find `buildDockerCommand` where env values are passed as `-e KEY=VALUE` args to `docker run`.
</context>

<requirements>
1. In `pkg/config/config.go`, find the `validateEnv` method (or the env validation logic inside `Validate`).

2. Add value validation that rejects control characters. After the existing key checks, add:
   ```go
   if strings.ContainsAny(v, "\x00\n\r") {
       return errors.Errorf(ctx, "env value for %q contains invalid characters", k)
   }
   ```

3. Import `"strings"` if not already imported.

4. Add a test in `pkg/config/config_test.go` that verifies:
   - Env values with newlines are rejected
   - Env values with null bytes are rejected
   - Env values with carriage returns are rejected
   - Normal env values still pass validation
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
- Use `github.com/bborbe/errors` for error construction.
- Follow existing test patterns (Ginkgo/Gomega, `DescribeTable` where appropriate).
</constraints>

<verification>
Run `make precommit` -- must pass.
</verification>
