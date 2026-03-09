---
status: completed
summary: Added git.ResolveGitRoot function in pkg/git/root.go and integrated os.Chdir to git root in main.go run(), with tests covering subdirectory and non-repo cases; CHANGELOG updated.
container: dark-factory-156-resolve-git-root-at-startup
dark-factory-version: v0.31.0
created: "2026-03-09T21:30:00Z"
queued: "2026-03-09T22:01:50Z"
started: "2026-03-09T22:21:06Z"
completed: "2026-03-09T22:27:42Z"
---

<summary>
- Running dark-factory from any subdirectory behaves identically to running from the project root
- Config and lock files are always anchored to the project root, not the invocation directory
- Help and version commands work outside a git repository
- Clear error message when invoked outside a git repository
</summary>

<objective>
dark-factory uses the current working directory for config loading, prompt directories, and the lock file. Running from a subdirectory places files in wrong locations (e.g., lock file in `prompts/` instead of project root).

After this change, dark-factory resolves the git repository root at startup and uses it as the working directory, so all relative paths resolve correctly regardless of where the command is invoked.
</objective>

<context>
Read CLAUDE.md for project conventions.

- `main.go` — entry point, `run()` function (line 27). Config loaded at line 54-58, commands dispatched after.
- `pkg/config/loader.go` — `NewLoader()` hardcodes `configPath: ".dark-factory.yaml"` (line 31), resolved relative to cwd.
- `pkg/factory/factory.go` — `CreateLocker(".")` called at lines 94 and 153. Uses cwd for lock file placement.
- `pkg/lock/locker.go` — `NewLocker(dir)` creates `.dark-factory.lock` in given dir (line 43).
- All prompt/spec directories in config defaults are relative: `"prompts"`, `"prompts/in-progress"`, etc. (config.go lines 64-75).
</context>

<requirements>
1. Add a function (e.g., in a new file `pkg/git/root.go` or in `main.go`) that runs `git rev-parse --show-toplevel` and returns the absolute path. Return a clear error if not inside a git repo.
2. In `main.go` `run()`, after parsing args but before loading config (before line 54), resolve git root and `os.Chdir()` to it. Log the resolved root at debug level.
3. Verify `help` and `version` branches in `run()` at `main.go` lines 32-37 return before config loading; no changes needed for those commands.
4. No changes to config loader, factory, or locker — they continue using relative paths, which now resolve correctly because cwd is the git root.
5. Add test for the git root resolution function: verify it returns the correct root when called from a subdirectory, and returns an error outside a git repo.
</requirements>

<constraints>
- Do NOT change `NewLoader`, `CreateLocker`, or any relative path handling — only change where cwd is set.
- Do NOT add a `rootDir` parameter threading through the codebase — `os.Chdir()` is the simplest approach.
- The `os.Chdir()` must happen before `config.NewLoader().Load()` (line 54-55 in main.go).
- Use `os/exec` package (`exec.CommandContext`) to run `git rev-parse --show-toplevel`, not a shell.
- Regenerate mocks if any interface changes (unlikely for this change): `go generate ./...`
- Do NOT commit — dark-factory handles git.
</constraints>

<verification>
- `make precommit` passes
- From the project root: `dark-factory status` works as before
- From a subdirectory (`cd prompts && dark-factory status`): resolves to project root and works
- `.dark-factory.yaml` is read from git root, not from cwd
- `.dark-factory.lock` is created at git root, not in cwd (verify: `cd prompts && dark-factory run` places lock at project root)
- `dark-factory --version` and `dark-factory help` work without git (no git root resolution needed)
- Outside a git repo (`cd /tmp && dark-factory status`): clear error message, not silent misbehavior
</verification>
