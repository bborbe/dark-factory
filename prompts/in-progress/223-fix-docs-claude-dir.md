---
status: approved
created: "2026-03-30T16:04:31Z"
queued: "2026-03-30T16:04:31Z"
---

<summary>
- Update yolo-container-setup.md to reference `claudeDir` config field instead of removed env var
- Remove stale `DARK_FACTORY_CLAUDE_CONFIG_DIR` references from docs
- Document new default behavior (defaults to `~/.claude-yolo`, no env var needed)
- Simplify verification section (no env var to check)
- Keep doc accurate with current codebase after prompt 220 changes
</summary>

<objective>
Update `docs/yolo-container-setup.md` to replace all references to the removed `DARK_FACTORY_CLAUDE_CONFIG_DIR` environment variable with the new `claudeDir` config field in `.dark-factory.yaml`.
</objective>

<context>
Read CLAUDE.md for project conventions.

Key file:
- `docs/yolo-container-setup.md` — lines 117-124 and 152 reference the removed `DARK_FACTORY_CLAUDE_CONFIG_DIR` env var
- `pkg/config/config.go` — `ClaudeDir` field with default `~/.claude-yolo` (added by prompt 220)
</context>

<requirements>
### 1. Replace env var section with config field

In `docs/yolo-container-setup.md`, replace lines 117-124:

```markdown
The mount path is configurable via the `DARK_FACTORY_CLAUDE_CONFIG_DIR` environment variable:

\`\`\`bash
# Default: ~/.claude (override to use separate YOLO config)
export DARK_FACTORY_CLAUDE_CONFIG_DIR=~/.claude-yolo
\`\`\`

**Important:** If you don't set this variable, dark-factory uses `~/.claude` (your main Claude Code config). Setting it to `~/.claude-yolo` keeps YOLO config isolated from your interactive sessions.
```

with:

```markdown
The mount path defaults to `~/.claude-yolo` and is configurable per project via `.dark-factory.yaml`:

\`\`\`yaml
claudeDir: ~/my-custom-claude-config
\`\`\`
```

### 2. Update verification section

In `docs/yolo-container-setup.md`, remove line 152:
```bash
echo $DARK_FACTORY_CLAUDE_CONFIG_DIR
```

And remove the comment above it:
```bash
# Env var set (add to ~/.zshrc or ~/.bashrc)
```
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Only modify `docs/yolo-container-setup.md`
- Do NOT add a CHANGELOG entry (doc-only change)
</constraints>

<verification>
```bash
make precommit
```
Must exit 0.
</verification>
