---
status: approved
spec: [082-global-env-layering]
created: "2026-05-19T00:00:00Z"
queued: "2026-05-19T16:50:54Z"
branch: dark-factory/global-env-layering
---

<summary>
- `docs/config-layering.md` removes `env` from the "project-only" list and documents it as a globally-eligible field with key-level merge semantics
- `docs/config-layering.md` documents the home-file secrets exception: literal values are permitted only in the global home file, not in the project yaml
- `docs/configuration.md` gains a "Global env" subsection under the Global Config section describing the home-file env node, override semantics, the key-name constraint, the permission warning, and how the effective-config log reports env keys by source
</summary>

<objective>
Update two documentation files to reflect the global env layering feature delivered in prompt 1. No code changes are involved in this prompt.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read in full before editing:
- `docs/config-layering.md` — understand the current field category lists, the Out-of-scope section, and the Secrets carve-out section
- `docs/configuration.md` — understand the existing Global Config section structure (starting around line 186) and where the Layered fields table lives
</context>

<requirements>

## 1. Update `docs/config-layering.md`

### 1a. Remove `env` from Category B (project-only)

In Section **B. Project-shape (project-only)**, locate the bullet:

```
- `extraMounts`, `env` — repo-specific
```

Replace it with:

```
- `extraMounts` — repo-specific (global override not supported; each project mounts its own paths)
```

`env` is no longer project-only and must not appear in this list.

### 1b. Add `env` to Category A (user-prefs eligible for global)

In Section **A. User-prefs (eligible for global)**, add `env` to the list of currently supported global fields. The existing list ends with `dirtyFileThreshold`. Append `env` as the next item:

```
- `env` — machine-wide environment variables injected into every YOLO container (key-level merge; project values override global values per key)
```

Place it immediately after the `dirtyFileThreshold` bullet.

### 1c. Add secrets exception note to Category A

Still in Section **A. User-prefs**, add a note after the `env` bullet (before the "Planned but not yet supported globally" paragraph) explaining the secrets exception:

```
  - **Secrets exception**: the global home file (`~/.dark-factory/config.yaml`) is not committed and may carry literal secret values in `env:`. The project file (`.dark-factory.yaml`) is repo-tracked and must never carry literal secrets — the existing validation rejects known secret patterns.
```

### 1d. Update the "Out of scope" section

In the **Out of scope** section, locate the line:

```
- `extraMounts`, `env`, `notifications.*` — too structured for global override; per-project only
```

Replace it with:

```
- `extraMounts`, `notifications.*` — too structured for global override; per-project only
- `env` is now supported globally with key-level merge semantics (see Category A above)
```

The spec acceptance criterion requires `grep -n 'env: now supported globally' docs/config-layering.md` to return at least one line. Ensure the replacement line contains the phrase `env: now supported globally` OR rewrite it so the phrase appears verbatim. The simplest approach:

```
- `env: now supported globally` with key-level merge (project values override global per key); see Category A above
- `extraMounts`, `notifications.*` — too structured for global override; per-project only
```

Also verify that after your edit, `grep -n 'env.*per-project only' docs/config-layering.md` returns ZERO lines. If any such phrase remains, remove it.

## 2. Update `docs/configuration.md`

### 2a. Locate the insertion point

Read the Global Config section (starting around line 186). The current structure is:

```
## Global Config
(example yaml)
### Precedence
### Layered fields (phase 1)   ← table of currently supported global fields
### Validation
### Source tracing
```

### 2b. Extend the "Layered fields" table

Locate the **Layered fields (phase 1)** table. It currently has rows for `maxContainers`, `model`, `hideGit`, `autoRelease`, `dirtyFileThreshold`. Add a new row for `env`:

```
| `env` | (none) | Environment variables injected into the YOLO container; key-level merge with project `env:` (project wins on collision) |
```

Append this row as the last row of the table.

### 2c. Add a "Global env" subsection

After the existing **Validation** subsection inside the Global Config section (and before the next `##`-level section), insert a new subsection. The content below is shown inside a 4-backtick fence so the inner triple-backtick yaml blocks render correctly — when you insert it into `docs/configuration.md`, copy ONLY the markdown content between the outer ` ```` ` fences, NOT the outer fences themselves:

````markdown
### Global env

Set machine-wide environment variables that every project on this machine will receive:

```yaml
# ~/.dark-factory/config.yaml
env:
  ANTHROPIC_BASE_URL: https://my-provider.example.com/v1
  ANTHROPIC_API_KEY: sk-ant-your-key-here   # only safe here — this file is not committed
```

**Key-level merge**: the project `.dark-factory.yaml` can add or override individual keys without replacing the whole map. Project values win on collision. Non-overlapping keys from both layers are passed to the container.

```yaml
# .dark-factory.yaml  (project file — committed to git)
env:
  GOPATH: /home/node/go   # adds a key; global keys still flow through
```

**Key name rules**: keys must match `^[A-Z_][A-Z0-9_]*$`. Any key that does not match causes config load to fail with an error naming the offending key.

**Secrets**: literal secret values (API keys, tokens) are permitted in the global home file because it is never committed. Never put literal secrets in `.dark-factory.yaml` — that file is tracked by git and may be read by anyone with repo access.

**Permission warning**: if `~/.dark-factory/config.yaml` has group or world read/write permissions, dark-factory logs a warning at startup and recommends `chmod 600 ~/.dark-factory/config.yaml`. Loading continues regardless.

**Effective config log**: the startup `effective config` log line reports `envFromGlobal`, `envProjectOverrides`, and `envProjectOnly` showing which keys came from which layer. Values are never logged.
````

Ensure the inserted text uses standard triple-backtick yaml fences (matching other code blocks in configuration.md), not 4-backtick fences.

Ensure the subsection heading is `### Global env` so that `grep -n 'Global env' docs/configuration.md` returns at least one line.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Do NOT change any code files — only `docs/config-layering.md` and `docs/configuration.md`.
- Preserve the existing Markdown formatting style of each file (headings, tables, fenced code blocks).
- The phrase `env.*per-project only` must no longer appear anywhere in `docs/config-layering.md` after your edit.
- The phrase `env: now supported globally` must appear at least once in `docs/config-layering.md` after your edit.
- The phrase `Global env` must appear at least once in `docs/configuration.md` after your edit.
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0 (docs-only changes; all linters pass).

Additional checks:
1. `grep -n 'Global env' docs/configuration.md` — returns at least one line.
2. `grep -n 'env: now supported globally' docs/config-layering.md` — returns at least one line.
3. `grep -n 'env.*per-project only' docs/config-layering.md` — returns ZERO lines.
4. `grep -n 'env' docs/config-layering.md | grep -i 'project-only'` — returns ZERO lines.
</verification>
