# Logging Conventions

Structured logging in dark-factory uses stdlib `log/slog`. Every log emission in a
hot-path package **must** use the context-bound logger retrieved via `log.From(ctx)`,
not the bare package-level `slog` functions.

## Canonical Keys

The following attribute keys are the **only** permitted keys in hot-path packages.
Introducing a new key requires editing **both** this document **and** the
`hotpath-logcheck` allow-list in `scripts/hotpath-logcheck.sh` in the same PR.

| Key             | Meaning                                                      |
|-----------------|--------------------------------------------------------------|
| `prompt_id`     | Identifier of the prompt being processed (filename stem)     |
| `spec_id`       | Identifier of the parent spec, when applicable               |
| `container`     | Docker container name assigned to the prompt run             |
| `workflow_type` | Workflow mode: `direct` or `pr`                              |
| `error`         | Error value (use `slog.Any("error", err)`)                   |
| `file`          | Basename of a file being read/written (non-prompt-identity)  |
| `dir`           | Directory path relevant to the operation                     |
| `branch`        | Git branch name                                              |
| `workflow_step` | Named step within the processor lifecycle                    |
| `container_old` | Previous container name, emitted only on the "container assigned" transition line |

Correlation keys are fixed; informational keys are permitted but MUST be snake_case.

## Threading

- Use **snake_case** for all attribute keys (e.g. `prompt_id`, not `promptID` or `PromptId`).
- In hot-path packages (`pkg/processor`, `pkg/executor`, `pkg/promptresumer`,
  `pkg/committingrecoverer`, `pkg/cancellationwatcher`, `pkg/queuescanner`),
  **all log emissions must go through `log.From(ctx)`**, the context-bound logger
  from `github.com/bborbe/dark-factory/pkg/log`. This ensures every line carries the
  full set of correlation attributes (`prompt_id`, `container`, etc.) attached when
  the context was built.
- Bind a logger to a context with `log.NewContext(ctx, logger)`. Re-bind (to add more
  attrs) with `log.NewContext(ctx, log.From(ctx).With("key", val))`.
- Bare package-level calls (`slog.Info(...)`, `slog.Warn(...)`, `slog.Error(...)`) are
  **rejected** by `make hotpath-logcheck` (strict mode, wired in via prompt 5) in the
  six hot-path packages listed above.

### Documented exclusion

`pkg/executor/launch.go`'s pure argv-builder functions (`BuildDockerRunArgs` and
helpers) are **excluded** from the hotpath-logcheck because they are shared boot/probe
launch-shape builders with no per-prompt `ctx`. They are not per-prompt hot-path code
and do not perform log emissions — see spec 099 Non-goal "Does NOT migrate boot-time logs".

## Removed Synonyms

The following keys were used in the codebase before the spec-099 migration and have
been **removed** (not deprecated). Do not introduce them again.

| Removed key | Canonical replacement | Notes                                         |
|-------------|-----------------------|-----------------------------------------------|
| `file`      | `prompt_id`           | When used for prompt-identity; `file` remains valid for non-identity file basenames |
| `path`      | `prompt_id`           | Was used for the prompt file path             |
| `prompt`    | `prompt_id`           | Was used for the prompt filename or stem      |

These were renamed during the prompt 2–4 migration phase. The canonical target for
all prompt-identity logging is `prompt_id`.

---

<!-- OPEN QUESTION for human reviewer:
     Spec AC 5/6 evidence greps target `slog.String(...)`/`slog.Int(...)` call forms, but
     this codebase emits attrs exclusively via the inline key-value form
     `slog.Info(msg, "key", val)`. Those ACs are therefore vacuously satisfied by the grep
     (zero matches → grep "passes"), but the real key-hygiene work is renaming inline kv
     keys (done in prompts 2-4). This doc is the durable record of the canonical set
     regardless of call form. Prompts 2-4 should be reviewed to confirm they rename
     `"file"`, `"path"`, and `"prompt"` inline kv keys to `"prompt_id"` where they refer
     to the prompt identity.
-->
