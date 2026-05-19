---
status: committing
summary: Extended validateClaudeAuth to accept a merged env map and short-circuit when both ANTHROPIC_BASE_URL and ANTHROPIC_AUTH_TOKEN are non-empty, enabling alt-provider routing without requiring OAuth on disk.
container: dark-factory-exec-395-validate-claude-auth-alt-provider
dark-factory-version: v0.162.0
created: "2026-05-19T22:30:00Z"
queued: "2026-05-19T20:01:39Z"
started: "2026-05-19T20:07:09Z"
---

<summary>
- `validateClaudeAuth` currently refuses to launch the container unless `~/.claude-yolo/.credentials.json` (or legacy `.claude.json`) contains a valid Anthropic OAuth token, OR the host `ANTHROPIC_API_KEY` env var is set
- This blocks Anthropic-compatible alt-provider routing (MiniMax etc.) where authentication flows via the merged container env (`ANTHROPIC_BASE_URL` + `ANTHROPIC_AUTH_TOKEN`) rather than OAuth on disk
- Extend `validateClaudeAuth` to accept the merged env map and short-circuit when the env declares alt-provider auth: both `ANTHROPIC_BASE_URL` and `ANTHROPIC_AUTH_TOKEN` set to non-empty values
- Existing OAuth and `ANTHROPIC_API_KEY` paths continue unchanged; this is purely an additional skip condition
</summary>

<objective>
Allow dark-factory to launch the YOLO container when the merged container env provides alt-provider auth (`ANTHROPIC_BASE_URL` + `ANTHROPIC_AUTH_TOKEN`), without requiring a valid OAuth token on disk.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `docs/dod.md` for the project's definition of done.

Files to read in full before editing:
- `pkg/executor/executor.go` — `validateClaudeAuth` function (lines ~577-622), the call site in `Execute()` (line ~135 in the same file), and the `dockerExecutor` struct (around line 30-90) which holds the `env map[string]string` field that already flows to the container
- `pkg/executor/export_test.go` — line 170 defines `ValidateClaudeAuthForTest(ctx context.Context, configDir string) error` which currently calls the 2-arg form. This is the public test seam used by `executor_test.go`.
- `pkg/executor/executor_test.go` — Ginkgo suite. The existing `Describe("validateClaudeAuth", ...)` block lives around lines 1801-1896 and uses `executor.ValidateClaudeAuthForTest(ctx, ...)` at seven call sites (lines ~1810, ~1817, ~1836, ~1851, ~1867, ~1878, ~1891). Match this exact style.
- `pkg/executor/executor_suite_test.go` — confirms suite bootstrap is `TestExecutor`.
</context>

<requirements>

## 1. Add env-based short-circuit to `validateClaudeAuth`

Change the function signature in `pkg/executor/executor.go` from:

```go
func validateClaudeAuth(ctx context.Context, configDir string) error {
```

to:

```go
func validateClaudeAuth(ctx context.Context, configDir string, env map[string]string) error {
```

Inside the function, immediately after the existing `ANTHROPIC_API_KEY` check (which reads from host `os.Getenv` and remains unchanged), add a second short-circuit that reads from the passed-in `env` map:

```go
// Alt-provider auth (e.g. MiniMax): if the merged container env declares a
// non-Anthropic base URL together with an auth token, OAuth on disk is not
// required — the container authenticates via env at request time.
if env["ANTHROPIC_BASE_URL"] != "" && env["ANTHROPIC_AUTH_TOKEN"] != "" {
    return nil
}
```

The rest of the function (the OAuth file checks for `.credentials.json` and `.claude.json`) is unchanged.

Update the GoDoc comment above the function to reflect the new behavior:

```go
// validateClaudeAuth checks that the Claude config directory contains a valid OAuth token.
// The check is skipped when any of:
//   - host env ANTHROPIC_API_KEY is set (API key auth path, no OAuth needed)
//   - merged container env declares alt-provider routing: ANTHROPIC_BASE_URL and
//     ANTHROPIC_AUTH_TOKEN are both non-empty (e.g. MiniMax via Anthropic-compatible API)
// Supports both legacy (.claude.json oauthAccount.accessToken) and current
// (.credentials.json claudeAiOauth.accessToken) token locations.
```

## 2. Update the single caller

In the same file, update the call site (currently around line 135) from:

```go
if err := validateClaudeAuth(ctx, claudeConfigDir); err != nil {
```

to:

```go
if err := validateClaudeAuth(ctx, claudeConfigDir, e.env); err != nil {
```

`e.env` is already populated by `NewDockerExecutor` and is what dark-factory passes to the container via `-e` flags. Reusing it ensures the pre-check observes the SAME env the container will see. `e.env` is read-only inside `validateClaudeAuth` — do not copy, sort, or mutate it.

## 3. Migrate the test seam and existing call sites

The signature change breaks compilation of the existing test plumbing. Update both:

### 3a. `pkg/executor/export_test.go` line ~170

Change:
```go
func ValidateClaudeAuthForTest(ctx context.Context, configDir string) error {
    return validateClaudeAuth(ctx, configDir)
}
```

to:
```go
func ValidateClaudeAuthForTest(ctx context.Context, configDir string, env map[string]string) error {
    return validateClaudeAuth(ctx, configDir, env)
}
```

### 3b. `pkg/executor/executor_test.go` — migrate all seven existing call sites

The current call sites (lines ~1810, ~1817, ~1836, ~1851, ~1867, ~1878, ~1891) all use the 2-arg form `executor.ValidateClaudeAuthForTest(ctx, "/path")`. Update every one of them to pass `nil` as the third argument so existing test cases preserve their original behavior (OAuth-file-only path):

```go
err := executor.ValidateClaudeAuthForTest(ctx, "/nonexistent/path", nil)
```

Pass `nil` (not `map[string]string{}`) — both produce identical behavior in the function body since `nil["KEY"]` returns the zero string, but `nil` is the idiomatic "no env" sentinel here.

## 4. Add tests for the new skip condition

Add new Ginkgo `Context`/`It` blocks inside the existing `Describe("validateClaudeAuth", ...)` in `pkg/executor/executor_test.go`. Match the style of the seven existing `It` blocks around lines 1801-1896 (uses Gomega `Expect(...).To(...)`, `BeNil()`, `HaveOccurred()`, and `GinkgoT().TempDir()` for filesystem fixtures).

Four sub-cases:

- **Skip on alt-provider env** — `configDir` is a temp dir with NO credentials file; `env=map[string]string{"ANTHROPIC_BASE_URL": "https://example.com", "ANTHROPIC_AUTH_TOKEN": "sk-x"}`. Assert `err == nil`.
- **Reject on partial env — only BASE_URL** — same temp dir; `env=map[string]string{"ANTHROPIC_BASE_URL": "https://example.com"}` (no AUTH_TOKEN). Assert `err != nil` (falls through to OAuth check, which fails because the credentials file is absent).
- **Reject on partial env — only AUTH_TOKEN** — `env=map[string]string{"ANTHROPIC_AUTH_TOKEN": "sk-x"}` (no BASE_URL). Assert `err != nil`.
- **Reject on empty-string values** — `env=map[string]string{"ANTHROPIC_BASE_URL": "", "ANTHROPIC_AUTH_TOKEN": ""}` (both keys present, both empty). Assert `err != nil` (the `!= ""` check guarantees this).

These augment, not replace, the existing seven OAuth-path test cases — those continue to call `ValidateClaudeAuthForTest(ctx, path, nil)` and verify the unchanged OAuth-on-disk behavior.

## 5. Add CHANGELOG entry

If `CHANGELOG.md` does not already have an `## Unreleased` section above the topmost released version header (currently `## v0.163.2`), create one. Then add the bullet under it:

```
- fix: `validateClaudeAuth` no longer blocks dark-factory launch when the merged container env provides alt-provider auth (`ANTHROPIC_BASE_URL` + `ANTHROPIC_AUTH_TOKEN`). Required for routing to MiniMax and other Anthropic-compatible providers without an OAuth token on disk.
```

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Do NOT widen the env-skip condition beyond the explicit two-key check; both `ANTHROPIC_BASE_URL` AND `ANTHROPIC_AUTH_TOKEN` must be non-empty. Either alone is insufficient.
- Do NOT remove or weaken the existing `ANTHROPIC_API_KEY` host-env check at line 582 — keep it as-is.
- Do NOT change the OAuth file-reading logic for `.credentials.json` or `.claude.json` — those code paths must be byte-identical after the change.
- Function remains a free function (not a method) — pass `env` as a parameter rather than converting to a method on `*dockerExecutor`.
- Wrap any new errors with `errors.Wrap`/`errors.Errorf` from `github.com/bborbe/errors`. Never `fmt.Errorf`, never bare `return err`.
- Do not touch `go.mod` / `go.sum` / `vendor/`.
- All currently passing tests must continue to pass.
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional checks:
1. `grep -A3 'func validateClaudeAuth' pkg/executor/executor.go | grep -q 'env map\[string\]string'` — exits 0 (handles multi-line signatures after gofmt).
2. `grep -nF 'env["ANTHROPIC_BASE_URL"]' pkg/executor/executor.go` — returns exactly one line (the new short-circuit).
3. `grep -nF 'env["ANTHROPIC_AUTH_TOKEN"]' pkg/executor/executor.go` — returns exactly one line.
4. `grep -nF 'validateClaudeAuth(ctx, claudeConfigDir, e.env)' pkg/executor/executor.go` — returns exactly one line (the updated call site).
5. `grep -nF 'env map[string]string' pkg/executor/export_test.go` — returns at least one line (the migrated test helper).
6. `grep -cF 'ValidateClaudeAuthForTest(ctx,' pkg/executor/executor_test.go` — returns a count of at least 11 (7 original + 4 new alt-provider cases) and every line passes a third argument.
7. `go test ./pkg/executor/... -run TestExecutor -v 2>&1 | grep -i 'validateClaudeAuth'` — output contains at least one line mentioning `validateClaudeAuth` and the `TestExecutor` suite exits 0.
8. `grep -nF 'validateClaudeAuth' CHANGELOG.md` — returns at least one line under `## Unreleased`.
</verification>
