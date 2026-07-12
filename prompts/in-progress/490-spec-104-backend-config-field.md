---
status: approved
spec: ["104"]
created: "2026-07-12T19:00:00Z"
queued: "2026-07-12T19:08:22Z"
---

<summary>

- Adds a new project/global/CLI config field `backend` with two valid values: `docker` (the default) and `local`.
- Operators can select it three ways with the same precedence as `maxContainers`/`hideGit`: global config, project `.dark-factory.yaml`, or `--set backend=local` on `run`/`daemon`.
- An invalid `backend` value (in YAML or via `--set`) is rejected at startup with a clear error naming the valid values.
- The startup "effective config" log line now reports the resolved backend and where it came from (default / global / project / cli).
- Leaving `backend` unset behaves exactly as today — the field defaults to `docker` and nothing else changes.
- This prompt is config-only: it does NOT create the local executor or switch any wiring (later prompts do that). The resolved value is plumbed so prompt 3 can read it.

</summary>

<objective>
Introduce a layered string-enum config field `backend` (values `docker|local`, default `docker`) resolvable from global config, project `.dark-factory.yaml`, and `--set backend=<value>`, with the same precedence and `*Source` reporting as `maxContainers`, plus validation and an effective-config log line. No executor, factory, or healthcheck change in this prompt.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` first, then `/workspace/CLAUDE.md`.

Read the parent spec end-to-end:
- `/workspace/specs/in-progress/104-local-execution-backend.md` — Desired Behavior 1–2; Constraints; Failure Modes rows "backend unset", "Invalid backend value"; Acceptance Criteria 1, 2.

Read these coding-plugin docs (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-enum-type-pattern.md` — string enum: `Available*` slice, `Validate()`, plural type, `Contains()`, `String()`, `Ptr()`.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo/Gomega, table tests, coverage ≥80%.
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — changelog format.

Read these files END-TO-END before editing — the new field mirrors these existing patterns exactly:
- `/workspace/pkg/config/workflow.go` — the canonical string-enum pattern in THIS repo (`Workflow` type + `AvailableWorkflows` + `Validate` + `Contains` + `String` + `Ptr`). Model the new `Backend` type on this file verbatim.
- `/workspace/pkg/config/config.go` — the `Config` struct (line ~86; `MaxContainers int` at ~117, `Workflow Workflow` at ~89), `Defaults()` (line ~132), and `Validate(ctx)` (line ~178) which registers per-field validators via `validation.Name(...)`.
- `/workspace/pkg/config/sources.go` — the `FieldSources` struct (has `MaxContainers string`, `Workflow string`).
- `/workspace/pkg/config/loader.go` — `LayeredProjectOverrides` (line ~33; has `MaxContainers *int`, `Workflow *Workflow`), `partialConfig` (line ~86; has `Workflow *Workflow yaml:"workflow"`, `MaxContainers *int yaml:"maxContainers,omitempty"`), and where partial fields flow into `LayeredProjectOverrides` (~line 185) and into `cfg` (~line 425).
- `/workspace/pkg/config/layer.go` — `SupportedSetKeys` (line ~19), `ApplyGlobalOverrides` (line ~37), `ComputeFieldSources` (line ~107), `ApplyOneSetOverride` (line ~248; the `case "maxContainers":` and `case "model":` blocks are the closest models — `backend` is a string enum so mirror `case "workflow"` in `applyDeliverySetOverride`: construct the typed value, call `.Validate(ctx)`, assign, set source to `"arg"`).
- `/workspace/pkg/globalconfig/globalconfig.go` — `GlobalConfig` struct (line ~54; `Model *string yaml:"model,omitempty"` is the model for an optional pointer field), its `Validate` (line ~66), `defaults()` (~108), and the partial-config unmarshal (~line 222/235). Add a `Backend *string yaml:"backend,omitempty"` to `GlobalConfig` and its partial, mirroring `Model`.
- `/workspace/pkg/factory/factory.go` — `LogEffectiveConfig` (line ~99) is where the effective-config `slog.Info("effective config", ...)` line is emitted (line ~136). Add `backend` + `backendSource` key/value pairs here mirroring `"model", cfg.Model, "modelSource", sources.Model`.
- `/workspace/pkg/config/roundtrip_test.go` (line ~169) — the `Entry("maxContainers", ...)` / `Entry("hideGit", ...)` table-test rows are the exact evidence shape to add a `backend` row to.
- `/workspace/pkg/config/config_notifications_test.go` (line ~331) — the `MaxContainers validation` Describe block is the model for a `Backend validation` block.

Verified facts (do not re-derive):
- The enum pattern in this repo uses a plural collection type with a `Contains` method (`Workflows`/`AvailableWorkflows`). Mirror it: `Backend` string type, `Backends` collection, `AvailableBackends = Backends{BackendDocker, BackendLocal}`.
- `--set` string-enum handling lives in `applyDeliverySetOverride` (the `workflow`/`pr`/`autoMerge` switch) — add `backend` there OR add a dedicated `case "backend":` in `ApplyOneSetOverride`. Either is fine; pick the `case "backend":` in `ApplyOneSetOverride` for locality (construct `Backend(value)`, call `.Validate(ctx)`, assign `cfg.Backend`, set `sources.Backend = "arg"`).
- The default value is `BackendDocker`. Set it in `Defaults()` (`Backend: BackendDocker`).
- `sources.Backend` must be populated in `ComputeFieldSources` mirroring `Model`: `"default"` in the initial struct, `"global"` when `global.Backend != nil`, `"project"` when `proj.Backend != nil`, `"arg"` in the `--set` path.

</context>

<requirements>

## 1. Add the `Backend` string-enum type

Create `/workspace/pkg/config/backend.go` mirroring `/workspace/pkg/config/workflow.go` exactly (same imports: `context`, `strings`, `github.com/bborbe/collection`, `github.com/bborbe/errors`, `github.com/bborbe/validation`). Include the standard BSD license header (copy from `workflow.go`).

```go
package config

const (
	BackendDocker Backend = "docker"
	BackendLocal  Backend = "local"
)

// AvailableBackends contains the two valid backend values.
var AvailableBackends = Backends{BackendDocker, BackendLocal}

// Backend selects how LLM steps (prompt execution and generation) are launched.
type Backend string

func (b Backend) String() string { return string(b) }

// Validate checks that the Backend is a known value.
func (b Backend) Validate(ctx context.Context) error {
	if !AvailableBackends.Contains(b) {
		// build valid-values list and return errors.Wrapf(ctx, validation.Error, "unknown backend %q, valid values: %s", b, ...)
	}
	return nil
}

func (b Backend) Ptr() *Backend { return &b }

type Backends []Backend

func (b Backends) Contains(backend Backend) bool { return collection.Contains(b, backend) }
```

Fill in the `Validate` body identically to `Workflow.Validate` (build the `validValues` slice, join with `, `, wrap `validation.Error`). The error message MUST list `docker, local`.

## 2. Add the `Backend` field to `Config`

In `/workspace/pkg/config/config.go`:
- Add to the `Config` struct (near `Workflow`): `Backend Backend \`yaml:"backend,omitempty"\``.
- In `Defaults()`, set `Backend: BackendDocker`.
- In `Validate(ctx)`, register the field validator mirroring the `workflow` entry: `validation.Name("backend", c.Backend)` (the `Backend` type satisfies the validator interface via its `Validate` method, exactly like `c.Workflow`).

## 3. Add `Backend` to `FieldSources`

In `/workspace/pkg/config/sources.go`, add `Backend string` to the `FieldSources` struct (near `Workflow`).

## 4. Add `Backend` to `LayeredProjectOverrides`, `partialConfig`, and the loader plumbing

In `/workspace/pkg/config/loader.go`:
- Add `Backend *Backend` to `LayeredProjectOverrides` (near `Workflow *Workflow`).
- Add `Backend *Backend \`yaml:"backend"\`` to `partialConfig` (near `Workflow *Workflow`).
- Where partial fields flow into `LayeredProjectOverrides` (~line 185, next to `Workflow: partial.Workflow`), add `Backend: partial.Backend`.
- Where partial fields flow into `cfg` (~line 425, mirror the `if partial.MaxContainers != nil { cfg.MaxContainers = *partial.MaxContainers }` block): add `if partial.Backend != nil { cfg.Backend = *partial.Backend }`.

## 5. Add `Backend` to `GlobalConfig`

In `/workspace/pkg/globalconfig/globalconfig.go`:
- Add `Backend *string \`yaml:"backend,omitempty"\`` to `GlobalConfig` (mirror `Model *string`).
- Add the matching `Backend *string \`yaml:"backend"\`` to the partial-config struct (~line 222) and the `if partial.Backend != nil { cfg.Backend = partial.Backend }` copy (~line 235, mirror how `Model` or another `*string` field is copied — grep the file for how an existing `*string` pointer field like `Model` is threaded through the partial unmarshal and mirror it EXACTLY).
- In `Validate`, if `g.Backend != nil`, validate it is one of `docker|local`: `if *g.Backend != "docker" && *g.Backend != "local" { return errors.Errorf(ctx, "globalconfig: backend %q invalid, valid values: docker, local", *g.Backend) }`. (globalconfig cannot import pkg/config, so validate the string literals directly here.)

## 6. Layer the global override into `Config`

In `/workspace/pkg/config/layer.go`:
- In `ApplyGlobalOverrides`, add (mirror `Model`): `if global.Backend != nil && proj.Backend == nil { cfg.Backend = Backend(*global.Backend) }`.
- In `ComputeFieldSources`, initialize `Backend: "default"` in the struct literal, then `if global.Backend != nil { s.Backend = "global" }` and `if proj.Backend != nil { s.Backend = "project" }` (mirror `Model`).
- Add `"backend"` to `SupportedSetKeys`.
- In `ApplyOneSetOverride`, add a `case "backend":` block:
  ```go
  case "backend":
      b := Backend(value)
      if err := b.Validate(ctx); err != nil {
          return err
      }
      cfg.Backend = b
      sources.Backend = "arg"
  ```

## 7. Emit `backend` + `backendSource` in the effective-config log

In `/workspace/pkg/factory/factory.go`, in `LogEffectiveConfig`'s `slog.Info("effective config", ...)` call (line ~136), add two key/value pairs mirroring the model pair:
```go
"backend", cfg.Backend,
"backendSource", sources.Backend,
```

## 8. Tests

- In `/workspace/pkg/config/roundtrip_test.go`, add a table `Entry` for `backend` mirroring the `maxContainers`/`hideGit` entries:
  `Entry("backend", "backend", "local", func(cfg Config) { Expect(cfg.Backend).To(Equal(BackendLocal)) })`.
- Add a `backend.go` unit test (`/workspace/pkg/config/backend_test.go`, `package config_test`) covering: `BackendDocker.Validate` and `BackendLocal.Validate` return nil; `Backend("bogus").Validate` returns an error whose message contains `docker, local`; `AvailableBackends.Contains(BackendDocker)` true and `.Contains("bogus")` false; `BackendDocker.String() == "docker"`; `(*BackendLocal.Ptr()) == BackendLocal`.
- Add a `Backend validation` Ginkgo `Describe` block (model on the `MaxContainers validation` block in `config_notifications_test.go`) asserting: `Defaults()` yields `Backend == BackendDocker` and `Validate` succeeds; a config with `Backend = "bogus"` fails `Validate` with a message containing `backend`.
- Add a `globalconfig.Validate` test asserting an invalid backend value is rejected: `GlobalConfig{Backend: ptr("bogus")}.Validate(ctx)` returns an error whose message contains `docker, local`.
- Add a layering table test (in the appropriate existing `layer_test.go` or `config_test.go` — grep for where `sources.Model`/`sources.MaxContainers` resolution is asserted and add parallel rows) asserting the resolved `cfg.Backend` + `sources.Backend` for each of: default (unset → `docker`/`default`), global-only (`global`), project-only (`project`), and `--set backend=local` (`arg`). This is the AC-1 evidence.

## 9. CHANGELOG

Append ONE bullet to `## Unreleased` in `/workspace/CHANGELOG.md`:
```
- feat: add layered config field backend (docker|local, default docker) resolvable via global/project/--set with backendSource reporting (spec 104 prompt 1)
```

</requirements>

<constraints>

- Default (`backend` unset) resolves to `docker` — byte-for-byte identical behavior to today. No executor/factory/healthcheck wiring change in this prompt (later prompts do that).
- Introduce NO new interface. `Backend` is a plain string-enum type mirroring `Workflow`.
- `globalconfig` MUST NOT import `pkg/config` (would create an import cycle) — validate the backend string literally inside `globalconfig.Validate`.
- Container vocabulary must not leak into neutral packages — this prompt only touches `pkg/config`, `pkg/globalconfig`, and `pkg/factory` (LogEffectiveConfig). Do not add container tokens.
- `make precommit` passes; `go generate ./...` leaves mocks clean (no interface added, so no new mocks expected).
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.

</constraints>

<verification>

```bash
cd /workspace

# unit + layering tests
go test -mod=mod ./pkg/config/... ./pkg/globalconfig/... ./pkg/factory/...
# expected: PASS

# coverage on the new/changed config code
go test -coverprofile=/tmp/cover.out -mod=mod ./pkg/config/... && go tool cover -func=/tmp/cover.out | tail -1
# expected: PASS, changed code covered

# effective-config log includes backend keys (source grep)
grep -n '"backend"' pkg/factory/factory.go && grep -n '"backendSource"' pkg/factory/factory.go
# expected: >= 1 line each

# changelog entry present
grep -n 'spec 104 prompt 1' CHANGELOG.md
# expected: >= 1 line

make precommit
# expected: exit 0
```

</verification>
