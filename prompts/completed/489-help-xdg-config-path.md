---
status: completed
summary: 'Add Configuration: section to dark-factory --help output documenting XDG global config path, legacy fallback, and per-project config path'
execution_id: dark-factory-help-xdg-exec-489-help-xdg-config-path
dark-factory-version: v0.191.0
created: "2026-07-11T13:03:41Z"
queued: "2026-07-11T13:03:41Z"
started: "2026-07-11T13:07:10Z"
completed: "2026-07-11T13:11:30Z"
---

<summary>
- `dark-factory --help` (and bare `dark-factory help`) now tells users where the config file lives
- A short "Configuration:" section is added to the top-level help output
- It names the XDG global config path and the legacy fallback path
- It also names the per-project config file
- No behavior change: config loading already resolves XDG-first with legacy fallback; this only surfaces the paths in help text
- Existing commands, flags, and help sections stay exactly as they are
</summary>

<objective>
Make the config file location discoverable from `dark-factory --help` so users no longer have to read docs or guess where to put their configuration. Config resolution is unchanged — this only documents the already-active XDG-first paths in the help output.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `main.go` — find the `printHelp` function. It builds the top-level usage string with `fmt.Fprintf(os.Stdout, ...)`. The command list ends with the `scenario status` line, followed by an `Options:` section and a `Flags:` section.
Read `pkg/globalconfig/globalconfig.go` — the `GlobalConfig` doc comment states the config is loaded from `~/.config/dark-factory/config.yaml` (XDG) with fallback to `~/.dark-factory/config.yaml` (legacy). Use those exact paths.
Read `pkg/config/loader.go` — the per-project config file is `.dark-factory.yaml` in the current directory.
</context>

<requirements>
1. In `main.go`, in the `printHelp` function, insert a new `Configuration:` section into the usage string, positioned AFTER the `scenario status` command line and BEFORE the `Options:` section.
2. Under a `Configuration:` header, the section must document these two paths (wording may match the project's existing help style, but the paths must be exact):
   - Global config: `~/.config/dark-factory/config.yaml` (XDG), falling back to `~/.dark-factory/config.yaml` (legacy)
   - Per-project config: `.dark-factory.yaml` in the current directory
3. Do not change any existing command descriptions, options, or flags — only add the new section.
4. To make the help output testable, change `printHelp` to accept an `io.Writer` parameter (`func printHelp(w io.Writer)`) and write to it instead of `os.Stdout`. Update the sole caller in `run` (the `case "help":` branch, currently `printHelp()`) to pass `os.Stdout`: `printHelp(os.Stdout)`.
5. Add a test (in `main_internal_test.go`, matching the existing Ginkgo/Gomega style used in that file) that calls `printHelp` with a `bytes.Buffer`, then asserts the captured output contains `Configuration:`, `~/.config/dark-factory/config.yaml`, `~/.dark-factory/config.yaml`, and `.dark-factory.yaml`. This locks the documented paths against future drift.
6. Add a `## Unreleased` section at the top of `CHANGELOG.md` (above the latest `## v0.191.2` entry) with a single bullet, e.g. `docs(help): document XDG global config path and per-project config path in top-level --help output`.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
- Paths must be exact: `~/.config/dark-factory/config.yaml` (XDG) and `~/.dark-factory/config.yaml` (legacy). Do not invent an `XDG_CONFIG_HOME` env-var override — that is explicitly out of scope.
- Keep the change to the top-level help only; per-command help blocks (`printRunHelp`, etc.) are out of scope.
- Add the imports the change needs: `io` in `main.go` (it currently imports `os` but not `io`) and `bytes` in the test file.
</constraints>

<verification>
Run `make precommit` -- must pass.
Run `go run . --help` and confirm the output contains a `Configuration:` section listing `~/.config/dark-factory/config.yaml` and `~/.dark-factory/config.yaml`.
</verification>
