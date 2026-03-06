<objective>
Simplify `dark-factory list` prompt output by removing the LOCATION column. The status value already encodes location (created=inbox, queued/executing=queue, completed/failed=completed), so showing both is redundant.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read pkg/cmd/list.go for the current list command implementation and PromptEntry struct.
Read pkg/cmd/list_test.go for existing tests.
</context>

<requirements>
1. Remove the `Location` field from `PromptEntry` struct in `pkg/cmd/list.go`.

2. Update `outputTable` to print only two columns:
   ```
   STATUS     FILE
   created    spec-019-xxx.md
   completed  001-mvp.md
   ```
   Format: `%-12s %s\n` for STATUS and FILE.

3. Update `outputJSON` — the JSON output no longer includes `"location"` field.

4. Remove the `location` parameter from `scanDir` (no longer needed).

5. Remove the `--queue` flag logic that used location filtering — replace with status-based filtering:
   - `--queue` now shows only prompts with status `queued` or `executing`

6. Update tests in `pkg/cmd/list_test.go` to match new output format.
</requirements>

<constraints>
- Do NOT change the SPECS section output format
- Do NOT remove `--failed` or `--json` flags
- Do NOT commit — dark-factory handles git
- Existing tests must be updated, not deleted
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
