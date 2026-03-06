<objective>
Write the `/generate-prompts-for-spec` slash command for claude-yolo. This command is called by dark-factory's SpecGenerator to automatically create prompt files from an approved spec. It must be non-interactive and fully automated — no AskUserQuestion, no user prompts.
</objective>

<context>
Read CLAUDE.md for project conventions.

The YOLO container mounts `~/.claude-yolo` as `/home/node/.claude`. Slash commands live at `~/.claude-yolo/commands/` on the host, which maps to `/home/node/.claude/commands/` inside the container. This command will be called as `/generate-prompts-for-spec <spec-file>` where `<spec-file>` is the path to the spec relative to the project root (e.g. `specs/020-auto-prompt-generation.md`).

The container runs with `/workspace` as the project root.

There is also a canonical source repo at `~/Documents/workspaces/claude-yolo/` — the same file must be committed there too.

Reference the existing `/create-prompt` command at `~/.claude-yolo/commands/create-prompt.md` for style, but this command is non-interactive unlike `create-prompt`.

Read the Dark Factory Guide at `/workspace/docs/` if present, or read `~/.claude-yolo/docs/` for conventions on what makes a good prompt.
</context>

<requirements>
1. Write the command file to `~/.claude-yolo/commands/generate-prompts-for-spec.md` with the following frontmatter:
   ```yaml
   ---
   description: Generate dark-factory prompt files from an approved spec (non-interactive)
   argument-hint: <spec-file>
   allowed-tools: [Read, Write, Glob, Bash]
   ---
   ```

2. The command body must:
   a. Read the spec file at `$ARGUMENTS` (e.g. `/workspace/specs/020-auto-prompt-generation.md`)
   b. Read the Dark Factory Guide for prompt writing conventions (check `/workspace/prompts/completed/` for recent examples, read `~/.claude-yolo/docs/go-patterns.md` for code patterns)
   c. Extract the spec number from the filename (e.g. `020` from `020-auto-prompt-generation.md`)
   d. Use Glob on `/workspace/prompts/*.md` and `/workspace/prompts/completed/*.md` to determine the highest existing prompt number — new prompts must NOT conflict
   e. Decompose the spec into 2–6 prompt files following the spec's Desired Behaviors, grouping coupled behaviors together
   f. Write each prompt file to `/workspace/prompts/<slug>.md` with this exact frontmatter:
      ```yaml
      ---
      spec: ["NNN"]
      status: created
      created: "<ISO8601 UTC timestamp>"
      ---
      ```
      where `NNN` is the zero-padded spec number (e.g. `"020"`)
   g. Each prompt body uses XML tags: `<objective>`, `<context>`, `<requirements>`, `<constraints>`, `<verification>`
   h. Each prompt `<constraints>` section repeats the relevant constraints from the spec
   i. Each prompt `<verification>` section ends with `Run `make precommit` — must pass.`
   j. Prompts are named for execution order using alphabetical prefixes (e.g. `spec-020-1-foo.md`, `spec-020-2-bar.md`) so dark-factory processes them in the correct sequence
   k. Do NOT add frontmatter beyond what is listed above — dark-factory adds `container`, `queued`, `started`, `completed`, `summary` during execution

3. After writing the command file to `~/.claude-yolo/commands/`, also write the identical file to `~/Documents/workspaces/claude-yolo/commands/generate-prompts-for-spec.md`.

4. Commit `~/.claude-yolo`:
   ```bash
   cd ~/.claude-yolo && git add commands/generate-prompts-for-spec.md && git commit -m "add generate-prompts-for-spec command"
   ```

5. Commit `~/Documents/workspaces/claude-yolo`:
   ```bash
   cd ~/Documents/workspaces/claude-yolo && git add commands/generate-prompts-for-spec.md && git commit -m "add generate-prompts-for-spec command"
   ```
</requirements>

<constraints>
- The command must be completely non-interactive — no AskUserQuestion, no questions to the user
- Do NOT commit the dark-factory project — dark-factory handles its own git
- Generated prompts land in `/workspace/prompts/` (inbox) only, never in `/workspace/prompts/queue/`
- Prompt filenames must NOT have numbers that conflict with existing completed prompts
- The `spec` frontmatter field is a YAML array: `spec: ["020"]` not `spec: "020"`
</constraints>

<verification>
Run `make precommit` — must pass.

Also verify manually:
```bash
ls ~/.claude-yolo/commands/generate-prompts-for-spec.md
ls ~/Documents/workspaces/claude-yolo/commands/generate-prompts-for-spec.md
cd ~/.claude-yolo && git log --oneline -1
cd ~/Documents/workspaces/claude-yolo && git log --oneline -1
```
</verification>
