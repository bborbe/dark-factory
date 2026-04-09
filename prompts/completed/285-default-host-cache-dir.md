---
status: completed
summary: 'Added resolveExtraMountSrc helper that auto-defaults HOST_CACHE_DIR to platform-appropriate cache directory (macOS: $HOME/Library/Caches, Linux: $XDG_CACHE_HOME or $HOME/.cache) without global env mutation, replaced os.ExpandEnv call site, added 7 unit tests, and documented HOST_CACHE_DIR in docs/configuration.md.'
container: dark-factory-285-default-host-cache-dir
dark-factory-version: v0.107.6
created: "2026-04-09T00:00:00Z"
queued: "2026-04-09T15:35:17Z"
started: "2026-04-09T15:42:48Z"
completed: "2026-04-09T15:56:28Z"
---
<summary>
- extraMounts src referencing `$HOST_CACHE_DIR` now works cross-platform without requiring the user to set the env var
- On macOS, defaults to the standard user cache location; on Linux, honors XDG_CACHE_HOME or falls back to `~/.cache`
- Existing `HOST_CACHE_DIR` values set by the user are respected (no override)
- macOS cache mounts work out of the box with no shell setup required
- Linux cache mounts work out of the box with no shell setup required
- No global environment mutation — expansion is done via a pure resolver function
- Documentation explains the variable and recommends it for portable cache mounts
</summary>

<objective>
Resolve `$HOST_CACHE_DIR` inside extraMounts src with a platform-appropriate default when unset, so prompts using `$HOST_CACHE_DIR/...` in extraMounts work on both macOS and Linux without manual env setup — without mutating process-global environment variables.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/executor/executor.go` — the `buildDockerCommand` method, specifically the `for _, m := range e.extraMounts` loop that calls `os.ExpandEnv(src)`.
Read `pkg/executor/executor_test.go` (or the nearest existing executor test file) for test style and helpers.
Read `docs/configuration.md` and locate the `## extraMounts` heading — the new documentation goes directly under that section.
</context>

<requirements>

## 1. Add pure resolver helper `resolveExtraMountSrc`

In `pkg/executor/executor.go`, add an unexported helper with this exact signature:

```go
// resolveExtraMountSrc expands env vars in src using lookupEnv, with a
// platform-appropriate default for HOST_CACHE_DIR when lookupEnv returns
// empty for it. Pure function: no globals, no os.Getenv, no os.Setenv.
//
// goos is runtime.GOOS at call site.
// Defaults for HOST_CACHE_DIR:
//   darwin           → $HOME/Library/Caches
//   other + XDG set  → $XDG_CACHE_HOME
//   other + no XDG   → $HOME/.cache
// If HOME is empty and no fallback is possible, returns empty for that var.
func resolveExtraMountSrc(src string, lookupEnv func(string) string, goos string) string {
    mapper := func(name string) string {
        if name == "HOST_CACHE_DIR" {
            if v := lookupEnv("HOST_CACHE_DIR"); v != "" {
                return v
            }
            home := lookupEnv("HOME")
            if goos == "darwin" {
                if home == "" {
                    return ""
                }
                return home + "/Library/Caches"
            }
            if xdg := lookupEnv("XDG_CACHE_HOME"); xdg != "" {
                return xdg
            }
            if home == "" {
                return ""
            }
            return home + "/.cache"
        }
        return lookupEnv(name)
    }
    return os.Expand(src, mapper)
}
```

Notes:
- Use `os.Expand` (NOT `os.ExpandEnv`) with the custom mapper.
- The function MUST be pure: do not call `os.Getenv`, `os.Setenv`, `os.LookupEnv`, or read `runtime.GOOS` inside. All inputs via parameters.
- Add `"runtime"` to imports if not already present (for the call site, not the helper).

## 2. Replace `os.ExpandEnv` call site

In the `for _, m := range e.extraMounts` loop in `buildDockerCommand`, replace:

```go
src = os.ExpandEnv(src)
```

with:

```go
src = resolveExtraMountSrc(src, os.Getenv, runtime.GOOS)
```

Do NOT add any `os.Setenv("HOST_CACHE_DIR", ...)` call. There must be zero global env mutation introduced by this change. Do not change any other behavior in the loop (`~/` handling, relative paths, stat check).

## 3. Unit tests

Add tests to the executor test file with these EXACT function names and assertions. Use a local `lookup` map helper, e.g.:

```go
func lookupFrom(m map[string]string) func(string) string {
    return func(k string) string { return m[k] }
}
```

### `TestResolveExtraMountSrc_Darwin_DefaultCache`
- Input: `src="$HOST_CACHE_DIR/go-build"`, lookup: `{"HOME": "/Users/alice"}`, goos: `"darwin"`
- Expected: `"/Users/alice/Library/Caches/go-build"`

### `TestResolveExtraMountSrc_Darwin_IgnoresXDG`
- Input: `src="$HOST_CACHE_DIR"`, lookup: `{"HOME": "/Users/alice", "XDG_CACHE_HOME": "/custom/xdg"}`, goos: `"darwin"`
- Expected: `"/Users/alice/Library/Caches"`

### `TestResolveExtraMountSrc_Linux_XDG`
- Input: `src="$HOST_CACHE_DIR/go-build"`, lookup: `{"HOME": "/home/bob", "XDG_CACHE_HOME": "/custom/xdg"}`, goos: `"linux"`
- Expected: `"/custom/xdg/go-build"`

### `TestResolveExtraMountSrc_Linux_NoXDG`
- Input: `src="$HOST_CACHE_DIR/go-build"`, lookup: `{"HOME": "/home/bob"}`, goos: `"linux"`
- Expected: `"/home/bob/.cache/go-build"`

### `TestResolveExtraMountSrc_UserOverride`
- Input: `src="$HOST_CACHE_DIR/x"`, lookup: `{"HOST_CACHE_DIR": "/preset", "HOME": "/home/bob"}`, goos: `"linux"`
- Expected: `"/preset/x"` — user value must win on both goos values; add a sub-assertion or sibling test for `goos="darwin"` with the same inputs expecting the same result.

### `TestResolveExtraMountSrc_Linux_EmptyHomeNoXDG`
- Input: `src="$HOST_CACHE_DIR/x"`, lookup: `{}`, goos: `"linux"`
- Expected: `"/x"` (var expands to empty string)

### `TestResolveExtraMountSrc_PassThroughOtherVars`
- Input: `src="$HOME/foo"`, lookup: `{"HOME": "/home/bob"}`, goos: `"linux"`
- Expected: `"/home/bob/foo"` — confirms non-HOST_CACHE_DIR vars still resolve via lookupEnv.

Do not use `os.Setenv` or `t.Setenv` in any of these tests — the helper is pure and must be tested by injecting the lookup function.

## 4. Documentation

Update `docs/configuration.md`. Locate the `## extraMounts` heading and add directly beneath it (before any existing subsections) a new subsection:

```markdown
### `HOST_CACHE_DIR`

extraMounts `src` supports environment variable expansion. The `HOST_CACHE_DIR`
variable is auto-defaulted by dark-factory when unset:

- **macOS**: `$HOME/Library/Caches`
- **Linux/other**: `$XDG_CACHE_HOME` if set, otherwise `$HOME/.cache`

Set `HOST_CACHE_DIR` explicitly to override. Recommended for portable cache
mounts:

​```yaml
extraMounts:
  - src: $HOST_CACHE_DIR/go-build
    dst: /home/node/.cache/go-build
​```
```

(Remove the zero-width spaces before the inner code fences when writing the file.)

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass (`make test`)
- Helper MUST be pure (no `os.Getenv`/`os.Setenv`/`os.LookupEnv`/`runtime.GOOS` inside) for testability
- NO global env mutation anywhere in the change — no `os.Setenv` calls added
- Never override a user-set `HOST_CACHE_DIR`
- Use `os.Expand` with a custom mapper, not `os.ExpandEnv`
- Use `runtime.GOOS` only at the call site, not inside the helper
- Do not change behavior of other extraMounts expansion logic (`~/` handling, relative paths, stat check)
</constraints>

<verification>
Run `make precommit` — must pass.
Run `make test` — new helper tests must pass.
Verify no new `os.Setenv` was added: `grep -n "os.Setenv" pkg/executor/executor.go` should show no new occurrences beyond pre-existing ones.
Verify docs: `grep -n "HOST_CACHE_DIR" docs/configuration.md` shows the new subsection under `## extraMounts`.
</verification>
