---
status: generating
approved: "2026-05-01T07:59:39Z"
generating: "2026-05-01T08:21:52Z"
branch: dark-factory/config-layering-phase-1
---

## Summary

- Extend `~/.dark-factory/config.yaml` to carry 4 additional user-pref fields beyond `maxContainers`: `hideGit`, `autoRelease`, `dirtyFileThreshold`, `model`.
- Establish proper layered precedence (default ← global ← project) for those 4 fields so a value set globally is overridden by a project-level value, but is the effective value when project config is silent.
- Add 2 per-invocation CLI flags: `--hide-git` (toggle) and `--model NAME`. They override both global and project values.
- Validate the layering mechanism end-to-end with these 4 fields before generalizing to remaining user-prefs (later phase).
- No behavior change for operators who do not adopt global config: defaults and project config continue to work exactly as today.

## Problem

Today only one field (`maxContainers`) traverses default → global → project layering. Every other user preference (model choice, hide-git display, auto-release habit, dirty-file tolerance) must be repeated in every project's `.dark-factory.yaml` or accepted as the hardcoded default. There is no machine-wide opinion an operator can express once. The result: operators duplicate config across repos, and changing a personal preference means editing N yaml files. CLI overrides are also limited — only `--max-containers`, `--skip-preflight`, `--auto-approve`, and `-debug` exist. There is no way to flip `hideGit` or pick a model per invocation without editing `.dark-factory.yaml`.

## Goal

When the operator sets `model: claude-opus-4-7` in `~/.dark-factory/config.yaml`, every project that does not specify `model` in its own `.dark-factory.yaml` runs with opus. When a specific repo wants sonnet, its project config wins. When the operator runs `dark-factory run --model claude-haiku-4-5` for a single invocation, that arg wins over both. Same precedence pattern for `hideGit`, `autoRelease`, `dirtyFileThreshold`. The merge mechanism is a single function reused for all 4 fields, ready to extend to additional fields and a future env layer.

## Non-goals

- Env layer (`DF_<FIELD>` env vars). Deferred to phase 2 — same merge code, different source.
- Migration of remaining user-prefs (`containerImage`, `claudeDir`, `verificationGate`, etc.). Deferred to phase 3 — exercise the pattern with 4 fields first.
- Secrets registry / auto-redaction. Deferred to phase 4 — orthogonal concern.
- Project-shape fields (`workflow`, `validationCommand`, dirs, etc.). Stay project-only forever.
- Removing `maxContainers` from project config — global precedence layering applies, project still wins when set.
- Backwards-incompatible changes to `~/.dark-factory/config.yaml` schema. New fields are additive and optional.

## Desired Behavior

1. **Schema extended.** `~/.dark-factory/config.yaml` accepts `hideGit`, `autoRelease`, `dirtyFileThreshold`, `model` in addition to `maxContainers`. Each is optional. Validation rejects malformed values (negative `dirtyFileThreshold`, `maxContainers < 1`, `model: ""` when explicitly set). The empty-string rule applies uniformly across global, project, and arg layers — see Constraints.

2. **Precedence: project wins when set.** If `.dark-factory.yaml` sets a field, that value is used regardless of global config. Detection of "set" must distinguish from zero value (`hideGit: false` set explicitly is different from `hideGit` absent).

3. **Precedence: global applies when project silent.** If project config does not set a field but global does, global value is used.

4. **Precedence: defaults apply otherwise.** When neither global nor project sets the field, the existing hardcoded default applies — identical to today's behavior.

5. **CLI flags `--hide-git` and `--no-hide-git`** added to `run` and `daemon` commands. `--hide-git` forces `hideGit: true` for this invocation; `--no-hide-git` forces `hideGit: false`. Either overrides yaml. Passing both is a usage error. Absence of both means yaml precedence applies.

6. **CLI flag `--model NAME`** added to `run` and `daemon` commands. Takes a single argument. Overrides any yaml model setting for this invocation. Validation: must match regex `^[a-zA-Z0-9._:/-]{1,256}$`. This permits Anthropic IDs (`claude-opus-4-7`), other-provider IDs (`qwen3.6:35b-a3b`, `llama3.2:1b`, `gemini-2.0-flash`), namespaced/local model paths (`local/qwen3.6:35b-a3b`), and Docker image refs (`docker.io/bborbe/claude-yolo:v0.6.1`) — all formats currently or plausibly used as model identifiers. The regex blocks shell metacharacters (spaces, `;`, `|`, `$`, backticks, quotes, redirects, glob chars, parens) since `model` flows to YOLO container args. Same rule applies uniformly in global, project, and arg layers. No exhaustive whitelist of specific IDs.

7. **Effective config log line shows source.** The `effective config` info line emitted at startup names the source of each layered field — e.g. `model=claude-opus-4-7 modelSource=global` or `hideGit=true hideGitSource=arg`. Operators reviewing logs can tell which layer won.

8. **No behavior change without opt-in.** Operators with no global config file and no new flags see identical behavior to today.

## Constraints

- `~/.dark-factory/config.yaml` schema is additive only — existing `maxContainers` reads unchanged.
- Project `.dark-factory.yaml` schema is unchanged — no new required fields, no deprecations.
- The 4 chosen fields keep their existing types and validation rules in project config, with one minor tightening: explicit `model: ""` is rejected uniformly at every layer (today's project config does not reject this — flag in CHANGELOG as a behavior tightening). `model` absent (yaml line not present) is allowed and resolves via precedence; only an explicitly empty string is invalid.
- Merge precedence runs at config load time; downstream consumers (factory, processor, executor) receive a final merged `Config` and never know layers existed. No new context plumbing.
- Distinguishing "set" from "zero value" requires sentinel semantics. Use pointer fields (`*bool`, `*int`, `*string`) at the layer-loading stage, dereference into the final `Config` after merge. Final `Config` keeps its current value-type fields — no API churn for downstream code.
- The merge function is single-purpose and reused for all 4 fields. Adding a 5th field later is a one-line struct addition plus a one-line merge call.
- Validation runs once on the final merged `Config`, not per-layer. Per-layer rejection (invalid yaml in global) still surfaces as a parse/validate error before merge.
- Existing `--max-containers N` flag continues to work and overrides both yaml layers (already true today).
- Secrets are not in scope. None of the 4 fields carry secrets.

## Failure Modes

| Trigger | Expected Behavior | Recovery |
|---|---|---|
| `~/.dark-factory/config.yaml` has invalid value (e.g. `dirtyFileThreshold: -5`) | Startup fails with explicit error naming the file and field | Fix or delete the global config file |
| Global config file unreadable (permission denied) | Startup fails with explicit error naming the path | Fix permissions or delete |
| Global config absent (no file at `~/.dark-factory/config.yaml`) | Startup proceeds with defaults + project — identical to today | None needed |
| Project config sets `model: ""` (empty string) | Validation rejects: empty model is not a "set" sentinel — the empty string IS a value, so explicit empty fails validation | Remove the line or set a real value |
| `--model` flag with no arg | CLI parsing rejects with usage error | Pass `--model NAME` |
| `--hide-git` followed by an unrelated arg | `--hide-git` is a flag, not a key=value — consumes nothing after | Standard flag parsing |
| Both `--hide-git` and `--no-hide-git` passed in one invocation | CLI parsing rejects with usage error: contradictory flags | Pick one |
| Both global and project set a field | Project wins (documented precedence) | None needed — by design |
| Operator sets a field globally but expects per-project value | Operator inspects effective-config log line, sees `<field>Source=global`, adds project override | Documented in `effective config` output |

## Do-Nothing Option

Operators continue duplicating preferences across every project's `.dark-factory.yaml`. Editing one personal opinion (e.g. switching default model from sonnet to opus) means walking N project repos. Per-invocation overrides remain limited to `--skip-preflight`, `--auto-approve`, `--max-containers`. No incident — just persistent friction and no path to grow the layering pattern. The 5th and 6th user-pref additions later face the same effort as the 1st.

## Acceptance Criteria

- [ ] `~/.dark-factory/config.yaml` parses `hideGit`, `autoRelease`, `dirtyFileThreshold`, `model` without error
- [ ] Operator with `model: claude-opus-4-7` in global config and no project override sees opus selected at runtime
- [ ] Operator with global `model: claude-opus-4-7` and project `model: claude-sonnet-4-6` sees sonnet selected (project wins)
- [ ] Operator with no global config and no project override sees the existing default model — identical to today
- [ ] `dark-factory run --model claude-haiku-4-5` overrides both yaml layers for that invocation
- [ ] `dark-factory run --hide-git` enables hide-git for that invocation regardless of yaml
- [ ] `dark-factory run --no-hide-git` disables hide-git for that invocation regardless of yaml (e.g. yaml says `true`, arg forces `false`)
- [ ] Passing both `--hide-git` and `--no-hide-git` exits non-zero with a usage error
- [ ] `dark-factory run --model` with no value exits non-zero with a usage error
- [ ] `dark-factory run --model 'foo;rm -rf /'` (or any value with shell metachar) exits non-zero with a regex validation error
- [ ] Explicit `model: ""` in any yaml layer fails validation
- [ ] Invalid global config (`dirtyFileThreshold: -5`) fails startup with a clear error naming the file
- [ ] `effective config` log line includes a per-field source indicator for the 4 layered fields
- [ ] Existing `maxContainers` precedence (global → project → arg) is unchanged
- [ ] No project's existing `.dark-factory.yaml` requires modification to keep working (assuming none currently set `model: ""` explicitly)
- [ ] Existing scenarios 001, 006, 010, 011 still pass
- [ ] `make precommit` exits 0

## Verification

```bash
cd ~/Documents/workspaces/dark-factory
make generate
make precommit
```

Both must exit 0.

Manual smoke test (one operator session):

```bash
# Global file with one field set
mkdir -p ~/.dark-factory && cat > ~/.dark-factory/config.yaml <<YAML
model: claude-opus-4-7
hideGit: true
YAML

# Run in a sandbox project with no project model override (use `run` — it emits the effective-config line)
dark-factory run
# expect: model=claude-opus-4-7 modelSource=global, hideGit=true hideGitSource=global

# Override per invocation
dark-factory run --model claude-sonnet-4-6 --no-hide-git
# expect: model=claude-sonnet-4-6 modelSource=arg, hideGit=false hideGitSource=arg
```

Cleanup: remove `~/.dark-factory/config.yaml` after the smoke test.

## Reference

- Design background and 5-layer model: [docs/config-layering.md](../docs/config-layering.md)
- Existing single-field precedent: `pkg/globalconfig/globalconfig.go`
- Project config struct: `pkg/config/config.go`
- CLI flag parsing: `main.go` `ParseArgs`
