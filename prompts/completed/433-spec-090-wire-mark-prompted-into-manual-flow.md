---
status: completed
spec: ["090"]
summary: Wired spec mark-prompted CLI into manual generation flow command file and updated docs/running.md to reflect the same approved→prompted lifecycle transition for both auto and manual paths
container: dark-factory-exec-433-spec-090-wire-mark-prompted-into-manual-flow
dark-factory-version: v0.171.1-3-gd94f1fa
created: "2026-05-25T20:30:01Z"
queued: "2026-05-25T20:21:28Z"
started: "2026-05-25T20:32:48Z"
completed: "2026-05-25T20:35:29Z"
---

<summary>
- Wires the new `dark-factory spec mark-prompted` subcommand into the manual prompt-generation flow.
- The `/dark-factory:generate-prompts-for-spec` command runs `spec mark-prompted` as its final step, only when the prompt-creator produced at least one file AND the per-prompt audit pass completed.
- The skip condition (zero new prompt files, or creator reported failure) is documented explicitly — matching the auto path's `handleNoNewFiles` behavior.
- `docs/running.md` § "Two ways to generate prompts" no longer falsely implies the manual path leaves the spec stuck at `approved`; both rows now describe the same lifecycle outcome.
- The CLI Reference table at the bottom of `docs/running.md` gains a `spec mark-prompted` row.
</summary>

<objective>
After the CLI subcommand from prompt 1 lands, wire it into the manual generation flow (`commands/generate-prompts-for-spec.md`) and update `docs/running.md` so the manual path describes the same `approved → prompted` lifecycle transition the auto path performs.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/agent-command-development-guide.md` for slash-command file structure.

Files to read before making changes:
- `specs/in-progress/090-cli-spec-mark-prompted.md` — full spec, especially Desired Behavior #7-8 and Acceptance Criteria related to `commands/generate-prompts-for-spec.md` and `docs/running.md`
- `commands/generate-prompts-for-spec.md` — current 39-line slash-command file (the final mark-prompted step must be added after the audit-pass step, gated on creator success)
- `docs/running.md` § "Two ways to generate prompts from an approved spec" (~line 39-90) — the comparison table and prose
- `docs/running.md` § "CLI Reference" (last section, ~line 250-263) — table to extend
- `prompts/1-spec-090-cli-spec-mark-prompted.md` (sibling prompt) — confirms the CLI subcommand and its `already prompted` stdout marker exist
- `pkg/generator/generator.go` lines ~223-247 — the `handleNoNewFiles` function whose skip semantics the manual flow must match
</context>

<requirements>

1. **Update `commands/generate-prompts-for-spec.md`** — add a new final-step section that runs `dark-factory spec mark-prompted` on the input spec. Place it AFTER the audit-pass paragraph that ends `...This front-loads the audit so the human reviewer sees the findings alongside the prompts without running /audit-prompt manually.` (currently the last paragraph of the file).

   The block below uses **four backticks** as the outer fence so the inner triple-backtick `bash` block nests cleanly. Copy ONLY the content BETWEEN the four-backtick fences (i.e. the markdown starting with `**Finally, transition...` and ending with `...for re-runs.`) into `commands/generate-prompts-for-spec.md`. Do NOT copy the four-backtick fences themselves — they are only here to delimit the literal content for you.

   ````markdown
   **Finally, transition the spec to `prompted`:** after the per-prompt audit pass completes (regardless of individual audit findings — finding-collection is non-blocking), invoke

   ```bash
   dark-factory spec mark-prompted <spec-basename>
   ```

   where `<spec-basename>` is the input spec's filename without `.md`. This closes the lifecycle gap — without it, the spec sits at `status: approved` forever while its prompts move through the queue.

   **Skip conditions** (do NOT run `spec mark-prompted` when):
   - The prompt-creator agent reported failure.
   - The prompt-creator wrote zero new prompt files.

   In both skip cases, surface the creator's report unchanged and exit — leaving the spec at its current status so the operator can inspect and re-run. This matches the auto path's `handleNoNewFiles` behavior in `pkg/generator/generator.go` (no `prompted` transition when zero files were produced).

   The `spec mark-prompted` call is idempotent: if the spec is already in `status: prompted` (e.g. a previous run already transitioned it, or the daemon's auto path ran first when `autoGeneratePrompts: true`), the subcommand exits 0 with stdout `already prompted: <basename>` and the manual command continues normally. No special handling needed for re-runs.

   **Race-window note:** the mark-prompted step is invoked unconditionally — there is no config check for `autoGeneratePrompts`. The idempotent CLI handles the "auto path already marked it" case cleanly. However, if the daemon is actively running the auto path for the SAME spec at the moment the operator triggers the manual command, both paths may race to write `status: prompted`. Last writer wins; no data corruption (single-file atomic `Save`). Operators should avoid running the manual command on a spec the daemon is currently processing.
   ````

   After writing, verify the destination file renders the inserted block as one paragraph + one fenced `bash` block + one bullet list + two trailing paragraphs (NOT as a code block containing the whole thing). If the render looks wrong, you over-copied the four-backtick fences — remove them.

2. **Naming consistency**: the subcommand argument is the spec basename without `.md`. The CLI also accepts numeric prefix (`090`) and full filename (`090-foo.md`) — but the command file should instruct operators/agents to pass the unambiguous basename form for clarity. The spec AC requires only that `spec mark-prompted` appears in the file (`grep -n 'spec mark-prompted' commands/generate-prompts-for-spec.md` returns ≥1 line).

3. **Update `docs/running.md`** — § "Two ways to generate prompts from an approved spec":

   a. In the comparison table (lines ~43-49), update the `Daemon role` row's `Manual` cell. Current text: `Daemon only executes prompts after the operator queues them; spec stays at status: approved until the operator acts`. Change the second clause to reflect the new lifecycle: `Daemon only executes prompts after the operator queues them; the manual command transitions the spec to status: prompted on success — same lifecycle outcome as the auto path`.

   b. In the § "When to pick manual" section (lines ~59-66), update the cost paragraph that currently reads `Cost: more steps. The spec sits at status: approved indefinitely until you remember to run the command. Easy to leave a queued spec stranded.` Change to: `Cost: more steps — you must remember to run the manual command, and until you do, the spec sits at status: approved. Once the manual command runs, the spec transitions to status: prompted automatically via dark-factory spec mark-prompted — same final state as the auto path.`

   c. In § "Switching modes" → "Manual command (when auto is disabled)" example (lines ~84-88), add a one-line clarifying note after the code block:

   ```
   On success this command also transitions the spec from `approved` to `prompted` (via `dark-factory spec mark-prompted`), so the spec's lifecycle status matches the auto path.
   ```

4. **Update `docs/running.md`** — § "CLI Reference" table (~lines 250-263): add a new row after the `spec complete` row:

   ```
   | `dark-factory spec mark-prompted <name>` | Transition a spec to `prompted` (used by the manual generation flow) |
   ```

5. **Verify the spec ACs that grep the files**:

   ```bash
   grep -n 'spec mark-prompted' commands/generate-prompts-for-spec.md       # ≥1 line
   grep -n -i 'zero\|no prompts\|skip\|failure' commands/generate-prompts-for-spec.md   # ≥1 line in the new section
   grep -n -i 'mark-prompted\|transitions.*prompted' docs/running.md         # ≥1 line in the "Two ways" section
   ```

   All three must return at least one line. If a grep returns zero, re-check the relevant edit.

6. **Do NOT modify** `pkg/spec/spec.go`, `pkg/generator/generator.go`, `pkg/cmd/spec_mark_prompted.go`, or any other Go source — this prompt is docs-and-command-file only. The CLI implementation lives in the sibling prompt 1.

7. **Run `make precommit`** — must pass. The repo's precommit may include markdown linting; fix any reported issues.

</requirements>

<constraints>
- Do NOT modify `pkg/generator/generator.go` — auto-path behavior is unchanged.
- Do NOT modify `pkg/spec/spec.go` — state machine is unchanged.
- Do NOT add an opt-out flag for the new mark-prompted step in the manual command — the whole point is to close the gap; an opt-out reintroduces the bug (spec § Non-goals).
- The mark-prompted step is GATED on prompt-creator success AND ≥1 file produced — match the auto path's `handleNoNewFiles` skip semantics exactly.
- The CLI subcommand from prompt 1 must already be merged/present when this prompt runs. If `dark-factory spec mark-prompted --help` is not recognized, STOP and surface that prompt 1 has not landed yet.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
</constraints>

<verification>
Run `make precommit` — must pass.

Run the three spec-AC grep commands listed in requirement 5 — each returns ≥1 line.

Read-through verification (manual): open `commands/generate-prompts-for-spec.md` and confirm the new final-step section is present, the skip-condition bullets are explicit, and the idempotency note is included.

Open `docs/running.md` and confirm:
- The "Daemon role" row's `Manual` cell mentions the `prompted` transition.
- The "When to pick manual" cost paragraph mentions `mark-prompted` and the matched final state.
- The "Switching modes" manual-command example has the trailing clarifying line.
- The CLI Reference table includes the new `spec mark-prompted` row.

End-to-end (optional, inside YOLO container — only meaningful if a real spec fixture is on disk): invoking the `/dark-factory:generate-prompts-for-spec` slash command on a fixture spec should now result in the spec's frontmatter showing `status: prompted` after the command returns successfully.
</verification>
