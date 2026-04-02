---
status: completed
spec: [041-additional-instructions]
summary: Added `additionalInstructions` config field that prepends project-level context to every prompt and spec generation command, with full test coverage and documentation update.
container: dark-factory-238-spec-041-additional-instructions
dark-factory-version: v0.85.0
created: "2026-04-02T00:00:00Z"
queued: "2026-04-02T09:17:49Z"
started: "2026-04-02T09:17:51Z"
completed: "2026-04-02T09:47:54Z"
---

<summary>
- Projects can configure `additionalInstructions` in `.dark-factory.yaml` — a multiline text block
- The configured instructions are automatically prepended to every prompt before execution
- The configured instructions are also prepended to every spec-to-prompt generation command
- When `additionalInstructions` is empty or absent, prompt and generation content is unchanged
- Eliminates the need to repeat project-level context (e.g. doc paths) in every prompt's `<context>` section
- Fully backwards compatible — existing projects without the field continue to work
- `docs/configuration.md` updated with the new field and a usage example
</summary>

<objective>
Add an `additionalInstructions` config field to `.dark-factory.yaml` whose content is automatically prepended to every prompt before execution and to every spec generation command. This removes the need for users to repeat per-project context (e.g. "read /docs/go-testing-guide.md") in every prompt's `<context>` section.
</objective>

<context>
Read CLAUDE.md for project conventions.

Read these files before making changes:
- `pkg/config/config.go` — `Config` struct (~line 81), `Defaults()` (~line 116), `Validate()` (~line 153)
- `pkg/config/loader.go` — `partialConfig` struct (~line 53) and `mergePartial()` (~line 140). CRITICAL: any new Config field MUST also be added to `partialConfig` and `mergePartial`, otherwise the YAML value is silently dropped.
- `pkg/processor/processor.go` — `NewProcessor()` constructor (~line 45), `processor` struct (~line 106), `enrichPromptContent()` (~line 657). The `content` variable is assembled at lines ~587–603: `content, err := pf.Content()` then `content = p.enrichPromptContent(ctx, content)` then passed to `executor.Execute`.
- `pkg/generator/generator.go` — `NewSpecGenerator()` (~line 32), `dockerSpecGenerator` struct (~line 57), `Generate()` (~line 71). The generation command is assembled at line ~73: `promptContent := g.generateCommand + " " + specPath`.
- `pkg/factory/factory.go` — `CreateProcessor()` (~line 419) and `CreateSpecGenerator()` (~line 357). Both `CreateRunner()` (~line 206) and `CreateOneShotRunner()` (~line 276) call `CreateProcessor`.
- `pkg/config/config_test.go` — existing config validation and loader tests
- `pkg/processor/processor_test.go` — existing processor tests
- `pkg/generator/generator_test.go` — existing generator tests
</context>

<requirements>
1. **Config struct** — add `AdditionalInstructions string \`yaml:"additionalInstructions,omitempty"\`` field to the `Config` struct in `pkg/config/config.go`. No default value (empty string means no injection).

2. **partialConfig** — add `AdditionalInstructions *string \`yaml:"additionalInstructions,omitempty"\`` to `partialConfig` in `pkg/config/loader.go` (pointer so nil = "not set in YAML").

3. **mergePartial** — add merge logic in `mergePartial()` in `pkg/config/loader.go`:
   ```go
   if partial.AdditionalInstructions != nil {
       cfg.AdditionalInstructions = *partial.AdditionalInstructions
   }
   ```

4. **Processor struct** — add `additionalInstructions string` field to the `processor` struct in `pkg/processor/processor.go`. Add it as the last parameter of `NewProcessor()` (after `maxContainers int`) and store it on the struct.

5. **Processor content injection** — prepend `additionalInstructions` to the prompt content in `enrichPromptContent()` in `pkg/processor/processor.go`. When non-empty, prepend before all other content, separated by a blank line:
   ```go
   func (p *processor) enrichPromptContent(ctx context.Context, content string) string {
       if p.additionalInstructions != "" {
           content = p.additionalInstructions + "\n\n" + content
       }
       // ... existing suffix appending unchanged ...
   }
   ```
   When `additionalInstructions` is empty, no whitespace or newlines are injected.

6. **Generator struct** — add `additionalInstructions string` field to the `dockerSpecGenerator` struct in `pkg/generator/generator.go`. Add it as the last parameter of `NewSpecGenerator()` (after `generateCommand string`) and store it on the struct.

7. **Generator content injection** — prepend `additionalInstructions` in `Generate()` in `pkg/generator/generator.go`. When non-empty, prepend before `promptContent`:
   ```go
   promptContent := g.generateCommand + " " + specPath
   if g.additionalInstructions != "" {
       promptContent = g.additionalInstructions + "\n\n" + promptContent
   }
   ```
   When empty, content is unchanged.

8. **Factory — CreateProcessor** — add `additionalInstructions string` as the last parameter of `CreateProcessor()` in `pkg/factory/factory.go` (after `maxContainers int`). Pass it to `processor.NewProcessor()`.

9. **Factory — CreateRunner** — update the `CreateProcessor()` call in `CreateRunner()` to pass `cfg.AdditionalInstructions` as the last argument.

10. **Factory — CreateOneShotRunner** — update the `CreateProcessor()` call in `CreateOneShotRunner()` to pass `cfg.AdditionalInstructions` as the last argument.

11. **Factory — CreateSpecGenerator** — update `CreateSpecGenerator()` to pass `cfg.AdditionalInstructions` as the last argument to `generator.NewSpecGenerator()`.

12. **Tests** — add tests covering:
    - Config loader: YAML with `additionalInstructions: |` loads correctly; YAML without the field leaves it empty
    - Processor `enrichPromptContent`: with non-empty `additionalInstructions`, result starts with the instructions followed by two newlines then original content; with empty `additionalInstructions`, result equals the original enriched content unchanged
    - Generator `Generate`: with non-empty `additionalInstructions`, the content passed to `executor.Execute` is prefixed with the instructions followed by two newlines then the generate command; with empty `additionalInstructions`, the content equals the generate command as before

13. **Documentation** — update `docs/configuration.md` to document the new `additionalInstructions` field. Add it near the `extraMounts` section (they are related — users often use `extraMounts` with `additionalInstructions` to mount and reference shared docs). Example:
    ```yaml
    additionalInstructions: |
      Read shared documentation at /docs for coding guidelines.
      Follow conventions in /docs/go-testing-guide.md for all test code.
    ```
    Add a row to the existing container-config table:
    | `additionalInstructions` | (empty) | Text prepended to every prompt and spec generation command |
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- `additionalInstructions` is a plain string — no parsing, no templating, no Markdown processing
- When `additionalInstructions` is empty, no whitespace or newlines are injected into prompt content
- Prepend order: `additionalInstructions` appears before all other prompt content and before any appended suffixes
- `.dark-factory.yaml` backward compatible — missing field = empty string = no injection
- Do NOT modify CLAUDE.md handling or system prompt
- Vendor mode: all commands use `-mod=vendor`
- Run `go generate -mod=vendor ./pkg/processor/... ./pkg/generator/...` if counterfeiter mocks need regeneration after struct changes (check if generated mocks exist for these packages first)
</constraints>

<verification>
```
make precommit
```
Must pass with no errors.

Additional spot-checks:
```bash
# Confirm AdditionalInstructions field appears in Config struct
grep -n "AdditionalInstructions" pkg/config/config.go pkg/config/loader.go

# Confirm processor and generator carry the field
grep -n "additionalInstructions" pkg/processor/processor.go pkg/generator/generator.go

# Confirm factory threads it through
grep -n "AdditionalInstructions\|additionalInstructions" pkg/factory/factory.go

# Run targeted tests
go test -mod=vendor ./pkg/config/... ./pkg/processor/... ./pkg/generator/... -v -count=1 2>&1 | tail -30
```
</verification>
