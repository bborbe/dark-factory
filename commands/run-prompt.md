---
name: run-prompt
description: Execute prompt directly in YOLO container (simplified for one-shot headless execution)
argument-hint: <prompt-number-or-path>
allowed-tools: [Read, Write, Edit, Bash, Grep, Glob]
---

<!--
Attribution: Inspired by and based on work from https://github.com/glittercowboy/taches-cc-resources
Adapted for YOLO container workflow with prompt-based execution
-->

<context>
Git status: !`git status --short`
</context>

<objective>
Execute a single prompt directly in YOLO container without sub-agent delegation.

**YOLO-specific simplification:** Since the container runs headless and exits after execution, there's no benefit to spawning a Task agent. Execute the prompt directly for faster, simpler execution.

Prompts are stored at the **git root** of their target project: `{git_root}/prompts/`
</objective>

<prompt_discovery>
**Finding prompts - resolution order:**

1. **Absolute/relative path given** (e.g., `~/Documents/workspaces/trading/prompts/005`):
   - Use directly
2. **Number given** (e.g., "005", "5"):
   - Check `./prompts/` relative to CWD (works when CWD is the git repo, e.g., in Docker)
   - If not found, find git root of CWD: `git rev-parse --show-toplevel` → check `{git_root}/prompts/`
   - If still not found, scan all workspaces: `find ~/Documents/workspaces -maxdepth 2 -type d -name prompts 2>/dev/null`
   - Search each found prompts dir for matching number
   - If multiple matches across repos → list and ask user to pick
3. **Partial name given** (e.g., "retry", "notification"):
   - Same search order as above, match against filename
4. **Empty/no arguments**:
   - Same search order, pick most recently modified prompt file
</prompt_discovery>

<input>
The user will specify which prompt(s) to run via $ARGUMENTS, which can be:

**Single prompt (YOLO supports one prompt at a time):**

- Empty (no arguments): Run the most recently created prompt
- A prompt number (e.g., "001", "5", "42")
- A partial filename (e.g., "user-auth", "dashboard")
- A full path (e.g., "~/Documents/workspaces/trading/prompts/005")

**Note:** Model selection flags (--haiku, --sonnet, --opus) are not supported in YOLO. The container launches with the model specified in the Dockerfile.
</input>

<process>
<step1_parse_arguments>
Parse $ARGUMENTS to extract:
- Prompt numbers/names/paths (all arguments that are not flags)
- Execution strategy flag (--parallel or --sequential)
- Model flag (--haiku, --sonnet, or --opus)

<examples>
- "005" → Single prompt: 005, model: inherit from parent
- "005 --haiku" → Single prompt: 005, model: haiku
- "~/Documents/workspaces/trading/prompts/005" → Single prompt at absolute path
- "005 006 007" → Multiple prompts: [005, 006, 007], strategy: sequential (default), model: inherit
- "005 006 007 --parallel" → Multiple prompts: [005, 006, 007], strategy: parallel, model: inherit
- "005 006 007 --parallel --haiku" → Multiple prompts: [005, 006, 007], strategy: parallel, model: haiku
</examples>
</step1_parse_arguments>

<step2_resolve_files>
For each prompt number/name/path, follow the prompt_discovery resolution order above.

<matching_rules>

- If exactly one match found: Use that file
- If multiple matches found: List them and ask user to choose
- If no matches found: Report error and list available prompts across all known prompt directories
</matching_rules>

Once resolved, determine the git root for the prompt's project:
`git -C $(dirname $PROMPT_FILE) rev-parse --show-toplevel`

Store as `$PROJECT_ROOT` — this is where the sub-task should work.
</step2_resolve_files>

<step3_execute>
<single_prompt>

**YOLO Mode: Direct Execution (No Sub-Agent)**

For YOLO container (headless, one-shot execution), execute prompt directly without Task agent:

1. Read the complete contents of the prompt file
2. Change working directory to `$PROJECT_ROOT`: `cd $PROJECT_ROOT`
3. Execute the prompt content directly (you are already the agent)
4. Follow all instructions in the prompt
5. Archive prompt to `$PROJECT_ROOT/prompts/completed/` with timestamp metadata
6. Report completion summary

**Note:** Model flags (--haiku, --sonnet, --opus) are ignored in YOLO mode since the container already launched with a specific model.

**Why no sub-agent in YOLO:**
- Container is ephemeral (context discarded after execution)
- No token savings benefit from Task isolation
- Simpler execution path
- Faster startup
</single_prompt>

</step3_execute>
</process>

<context_strategy>
By delegating to a sub-task, the actual implementation work happens in fresh context while the main conversation stays lean for orchestration and iteration.
</context_strategy>

<output>
✓ Executed: $PROJECT_ROOT/prompts/001-implement-feature.md
✓ Project: $PROJECT_ROOT
✓ Archived to: $PROJECT_ROOT/prompts/completed/001-implement-feature-TIMESTAMP.md

<results>
[Summary of what was implemented, tests run, verification completed]
</results>
</output>

<notes>
- Archive prompts after successful completion with timestamp
- Always work from $PROJECT_ROOT (git root), not subdirectories
- YOLO executes single prompts only (no parallel/sequential support)
</notes>
