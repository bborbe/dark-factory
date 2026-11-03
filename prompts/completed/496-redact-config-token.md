---
status: completed
execution_id: dark-factory-redact-config-exec-496-redact-config-token
dark-factory-version: v0.198.7
created: "2026-11-03T00:00:00Z"
queued: "2026-11-03T20:24:28Z"
started: "2026-11-03T20:28:48Z"
completed: "2026-11-03T20:38:22Z"
---

# Redact secret values in dark-factory config output

<summary>

- `dark-factory config` stops printing secret values in cleartext
- Secret values are replaced with a fixed redaction marker; the key names stay visible
- Every place the value appears in the output is covered, not just the first one
- Non-secret environment entries such as cache and path settings stay readable, so the output remains useful for debugging
- A GitHub token held in the project config is redacted by the same change
- Existing behaviour is otherwise unchanged: same fields, same order, same YAML shape
- A unit test proves the value never reaches the encoded output

</summary>

<objective>

Make `dark-factory config` safe to paste into a terminal transcript or a bug report. Today it YAML-encodes the effective configuration verbatim, so the router auth token and any GitHub token appear in cleartext; redact secret values at the print site so the output stays diagnostically useful without leaking credentials.

</objective>

<context>

Read `CLAUDE.md` for project conventions.

The print site is `printConfig` in `main.go`. It loads the global config and YAML-encodes a struct holding both the global and the project configuration to `os.Stdout`.

The token reaches the output **twice**, not once: `ApplyGlobalOverrides` merges the global `env` map underneath the project env (project wins on key collision), so a key present only in the global file is still emitted under both the `global:` and the `project:` section. Read `pkg/config/layer.go` — the `ApplyGlobalOverrides` function — and `MergeEnv` in `pkg/config/env.go` to see the merge. A fix that redacts only one section leaves the other leaking.

The secret carriers:

- `pkg/globalconfig/globalconfig.go` — `GlobalConfig.Env` (`map[string]string`), keyed by environment variable name
- `pkg/config/config.go` — `Config.Env` (`map[string]string`) and `Config.GitHub` (`GitHubConfig.Token`, a `string`)

Read `docs/configuration.md` (the `env` and `github.token` fields) and `docs/config-layering.md` (the global↔project env merge and the secrets exception) for the documented behaviour this output must stay consistent with.

Pattern references to read before writing:

- `pkg/gitprovider/bitbucket/http.go` — `redactToken`, the existing redaction marker convention in this codebase. Reuse the same marker rather than inventing a second one.
- `pkg/globalconfig/globalconfig.go` — `envKeyRegexp`, the existing compiled-regex package-level var pattern to follow if you need a key-matching regex.
- `main_internal_test.go` — `Describe("printHelp")` shows the `bytes.Buffer` capture style for testing a print function.

</context>

<requirements>

1. In `main.go`, extract the encoding half of `printConfig` into a function that takes an `io.Writer` (for example `encodeConfig(w io.Writer, globalCfg globalconfig.GlobalConfig, cfg config.Config) error`; add `ctx context.Context` as the first parameter if you wrap the encoder error with `github.com/bborbe/errors`), so the encoded bytes can be asserted in a test without capturing `os.Stdout`. `printConfig` keeps its signature, still loads the global config, and calls the new function with `os.Stdout`.

2. In that function, redact before encoding:
   - every value in `globalCfg.Env` whose key looks like a credential
   - every value in `cfg.Env` whose key looks like a credential
   - `cfg.GitHub.Token` when it is non-empty

   A key "looks like a credential" when it contains `TOKEN`, `SECRET`, `PASSWORD`, `PASSWD`, `CREDENTIAL`, or `API_KEY`/`APIKEY`, or ends in `_KEY` — case-insensitive. Prefer a single compiled package-level regexp over a hand-written conditional chain.

3. Redact the **value only**. Key names stay in the output, because the key name is what tells a reader which setting is set. Non-matching entries (`GOPATH`, `GOCACHE`, `GOLANGCI_LINT_CACHE`, `GO_TEST_PARALLEL`, `GOFLAGS`, `ANTHROPIC_BASE_URL`, the `ANTHROPIC_DEFAULT_*_MODEL` entries) must keep their real values.

4. Do not mutate the caller's maps or struct. Build the redacted map as a copy; assign it to the local copy of the config structs only. The real values must still be intact for anything that reads the config after printing.

5. Use the existing marker from `redactToken` in `pkg/gitprovider/bitbucket/http.go` (`[REDACTED]`) as the replacement value, and define it once as a package-level constant in `main.go` rather than repeating the literal. `redactToken` hard-codes the same literal today; note in the constant's doc comment that the two must stay in sync — do not add a shared package just for this marker.

6. Add a Ginkgo test in `main_internal_test.go` (the file already imports `bytes`, `config` and `globalconfig`). It must:
   - build a global config whose `Env` holds the real credential key `ANTHROPIC_AUTH_TOKEN` with a recognisable fake value, plus the real non-secret keys `ANTHROPIC_BASE_URL` and `ANTHROPIC_DEFAULT_HAIKU_MODEL` with recognisable values
   - build a project config whose `Env` holds `ANTHROPIC_AUTH_TOKEN` with the same fake value, plus the real non-secret keys `GOPATH` and `GO_TEST_PARALLEL` with recognisable values, and `cfg.GitHub.Token` set to a recognisable fake value
   - encode to a `bytes.Buffer` and assert, on the encoded string: the fake secret values appear **zero** times anywhere, the credential key names still appear, the non-secret values still appear verbatim, and the redaction marker appears exactly three times (global env, project env, GitHub token)

   This is the boundary test: the change crosses YAML serialization, so the assertion must run on the marshalled bytes, not on the struct.

7. Leave every other field, section and ordering in the output unchanged.

</requirements>

<constraints>

- Do NOT commit — dark-factory handles git
- Do NOT edit `CHANGELOG.md` — its `## Unreleased` entry already describes this change
- Existing tests must still pass
- Do NOT redact values whose key does not match the credential pattern; `dark-factory config` exists to show effective settings, and hiding non-secrets makes it useless
- Do NOT change `Config`, `GlobalConfig` or any YAML struct tag — the fix is at the print site, not in the types
- Do NOT touch the global config file's permissions or the existing world-readable warning in `pkg/config/loader.go`
- Do NOT add a `MarshalYAML` method to the config types: the same types are written elsewhere, and a type-level redaction would silently corrupt a real config write
- Use `github.com/bborbe/errors` for any error handling, never `fmt.Errorf`
- No comments that restate the code

</constraints>

<verification>

```bash
ROOTDIR=/workspace make precommit
```

Must pass. Then, before finishing, re-run `<verification>` and confirm it passes, and walk each requirement above against the change — in particular confirm by reading the test that it asserts the fake secret values are absent from the encoded bytes and that the non-secret values are still present.

</verification>
