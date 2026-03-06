---
spec: ["021"]
status: queued
created: "2026-03-06T18:00:00Z"
---
<objective>
Add a `verifying` status to the spec lifecycle so that when all linked prompts complete, the spec transitions to `verifying` instead of `completed`. This gives the human an explicit gate to evaluate whether the spec's acceptance criteria were met before closing it.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read pkg/spec/spec.go — contains Status constants, SpecFile, AutoCompleter, CheckAndComplete, and allLinkedPromptsCompleted. This is the main file to change.
Read pkg/spec/lister.go — contains Summary struct and the Summary() method that counts specs by status.
Read pkg/spec/spec_test.go — tests for AutoCompleter; extend for verifying behavior.
Read pkg/cmd/spec_list.go — outputSpecListTable; mark verifying specs prominently.
Read pkg/cmd/spec_list_test.go — extend with verifying test case.
Read pkg/cmd/spec_status.go — the summary line that prints counts; add verifying.
Read pkg/cmd/spec_status_test.go — extend with verifying count.
</context>

<requirements>
1. In `pkg/spec/spec.go`:
   a. Add the constant:
      ```go
      StatusVerifying Status = "verifying"
      ```
      Place it after `StatusPrompted` and before `StatusCompleted`.

   b. Add method to `SpecFile`:
      ```go
      // MarkVerifying sets the spec status to verifying.
      func (s *SpecFile) MarkVerifying() {
          s.Frontmatter.Status = string(StatusVerifying)
      }
      ```

   c. Update `CheckAndComplete` so that when all linked prompts are completed AND at least one linked prompt was found (`found == true`), it transitions the spec to `verifying` instead of `completed`:
      - Replace the call to `sf.MarkCompleted()` with `sf.MarkVerifying()`
      - Replace the log message `"spec auto-completed"` with `"spec awaiting verification"`
      - The existing guard `if sf.Frontmatter.Status == string(StatusCompleted)` must also skip if status is already `verifying`:
        ```go
        if sf.Frontmatter.Status == string(StatusCompleted) ||
            sf.Frontmatter.Status == string(StatusVerifying) {
            return nil
        }
        ```

   d. The case where `found == false` (no linked prompts) is unchanged — spec is NOT touched (behavior 5: specs without linked prompts still auto-complete as before, meaning they simply stay in their current state since auto-complete never ran). Confirm no code path auto-completes unlinked specs — the `!found` early return already handles this correctly.

2. In `pkg/spec/lister.go`:
   a. Add `Verifying int` field to the `Summary` struct, after `Prompted` and before `Completed`:
      ```go
      Verifying int
      ```
   b. In `Summary()`, add a case for `StatusVerifying` in the switch:
      ```go
      case StatusVerifying:
          s.Verifying++
      ```

3. In `pkg/cmd/spec_list.go`, update `outputSpecListTable` to mark `verifying` specs so they stand out:
   - When `e.Status == "verifying"`, prefix the status column with `!` (e.g., display `!verifying` in the STATUS column), so the human immediately sees which specs need attention.
   - The column width is currently `%-10s`; keep it wide enough to fit `!verifying` (10 chars — fits exactly).

4. In `pkg/cmd/spec_status.go`, update the `fmt.Printf` line in `Run` to include the verifying count:
   ```go
   fmt.Printf(
       "Specs: %d total (%d draft, %d approved, %d prompted, %d verifying, %d completed) | Linked prompts: %d/%d\n",
       summary.Total,
       summary.Draft,
       summary.Approved,
       summary.Prompted,
       summary.Verifying,
       summary.Completed,
       summary.LinkedPromptsCompleted,
       summary.LinkedPromptsTotal,
   )
   ```

5. Update tests:
   a. `pkg/spec/spec_test.go` — add test cases for `CheckAndComplete`:
      - All linked prompts completed AND found → spec transitions to `verifying` (not `completed`)
      - Spec already in `verifying` → no-op (no double-write)
      - No linked prompts found → spec status unchanged (not `verifying`, not `completed`)
   b. `pkg/cmd/spec_list_test.go` — add test: verifying spec renders with `!verifying` in STATUS column.
   c. `pkg/cmd/spec_status_test.go` — add test: summary output includes `verifying` count.

6. Run `make generate` if counterfeiter mocks need regenerating (check if any interface changed — in this case no interface changes, so `make generate` is NOT required).
</requirements>

<constraints>
- Specs with no linked prompts must NOT be affected — the `!found` early return in `CheckAndComplete` must remain intact
- Existing `spec approve` behavior is unchanged
- The `StatusCompleted` constant and `MarkCompleted()` method must remain (they are used by `spec verify` in the next prompt)
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- `make precommit` must pass
</constraints>

<verification>
Run `make precommit` — must pass.

Additional checks:
```bash
# Confirm StatusVerifying is exported and the constant value is "verifying"
grep -n 'StatusVerifying' pkg/spec/spec.go

# Confirm Summary has Verifying field
grep -n 'Verifying' pkg/spec/lister.go

# Confirm spec_list marks verifying with !
grep -n 'verifying' pkg/cmd/spec_list.go
```
</verification>
