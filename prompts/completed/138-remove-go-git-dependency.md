---
status: completed
summary: Replaced go-git library with git CLI commands (git mv and git tag --list) in pkg/git/git.go, removing the direct go-git dependency
container: dark-factory-138-remove-go-git-dependency
dark-factory-version: v0.26.0
created: "2026-03-08T20:44:59Z"
queued: "2026-03-08T20:44:59Z"
started: "2026-03-08T20:45:06Z"
completed: "2026-03-08T20:54:01Z"
---

<objective>
Remove the `github.com/go-git/go-git/v5` dependency entirely. Replace its two usages with `git` CLI commands, making all git operations consistent (everything uses `exec.CommandContext`). This eliminates a heavy transitive dependency (~40 indirect packages) for two call sites.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/git/git.go` — the only file importing go-git. Two usages:
1. `MoveFile` (line 186-237): uses `gogit.PlainOpenWithOptions` to get worktree root, then `wt.Remove`/`wt.Add` to stage. Replace with `git mv`.
2. `getNextVersion` (line 262-310): uses `gogit.PlainOpen` + `repo.Tags()` to iterate semver tags. Replace with `git tag --list`.

Read `pkg/git/git_internal_test.go` — internal tests for `MoveFile` (line 126+).
Read `pkg/git/git_test.go` — external tests for `releaser.MoveFile` (line 1192+).
Read `pkg/prompt/prompt.go` — `FileMover` interface (line 401) and `MoveFile` callers (lines 800, 1022).
Read `/home/node/.claude/docs/go-patterns.md` and `/home/node/.claude/docs/go-testing.md`.
</context>

<requirements>
1. Replace `MoveFile` function (line 186-237) in `pkg/git/git.go`:

   Replace the entire go-git implementation with `git mv`:
   ```go
   func MoveFile(ctx context.Context, oldPath string, newPath string) error {
       cmd := exec.CommandContext(ctx, "git", "mv", oldPath, newPath) // #nosec G204
       if err := cmd.Run(); err != nil {
           // git mv failed (not a repo, file not tracked, etc.) — fallback to os.Rename
           return fallbackRename(ctx, oldPath, newPath)
       }
       return nil
   }
   ```

   `git mv` handles: rename, stage removal of old path, stage addition of new path — all in one command. Fallback to `os.Rename` preserves behavior for non-git contexts (tests using temp dirs).

2. Replace `getNextVersion` function (line 262-310) in `pkg/git/git.go`:

   Replace go-git tag iteration with `git tag --list`:
   ```go
   func getNextVersion(ctx context.Context, bump VersionBump) (string, error) {
       cmd := exec.CommandContext(ctx, "git", "tag", "--list", "v*") // #nosec G204
       out, err := cmd.Output()
       if err != nil {
           return "", errors.Wrap(ctx, err, "list git tags")
       }

       var versions []SemanticVersionNumber
       for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
           line = strings.TrimSpace(line)
           if line == "" {
               continue
           }
           version, parseErr := ParseSemanticVersionNumber(ctx, line)
           if parseErr != nil {
               continue // Skip invalid semver tags
           }
           versions = append(versions, version)
       }

       if len(versions) == 0 {
           return "v0.1.0", nil
       }

       maxVersion := versions[0]
       for _, v := range versions[1:] {
           if maxVersion.Less(v) {
               maxVersion = v
           }
       }

       return maxVersion.Bump(bump).String(), nil
   }
   ```

3. Remove go-git imports from `pkg/git/git.go`:
   - Delete `gogit "github.com/go-git/go-git/v5"`
   - Delete `"github.com/go-git/go-git/v5/plumbing"`

4. Run `go mod tidy` to remove the go-git dependency and all its transitive deps from `go.mod` and `go.sum`.

5. Update existing tests in `pkg/git/git_internal_test.go`:
   - The `MoveFile` tests (line 126+) use real temp directories. They should still pass since `git mv` will fail (not a git repo in temp dir) and fallback to `os.Rename`.
   - If tests for `MoveFile` inside a git repo exist, they remain valid — `git mv` produces the same result.

6. Update existing tests in `pkg/git/git_test.go`:
   - The `releaser.MoveFile` tests (line 1192+) use temp dirs initialized with `git init`. Verify they still pass with the CLI-based implementation.
   - The `GetNextVersion` tests should still pass — they create repos with `git tag` commands, and `git tag --list` reads the same data.

7. Remove `fallbackRename` if it becomes unused after the refactor. Check: if `MoveFile` is the only caller, keep it since the new implementation still calls it.
</requirements>

<constraints>
- Do NOT change the `MoveFile` or `GetNextVersion` function signatures — callers must not change
- Do NOT change the `Releaser` interface — `MoveFile` stays on it
- Do NOT change the `FileMover` interface in `pkg/prompt/prompt.go`
- Do NOT modify any files outside `pkg/git/` (except `go.mod`/`go.sum` via `go mod tidy`)
- Preserve the fallback-to-os.Rename behavior when git operations fail (non-git directory, untracked file)
- `#nosec G204` annotations on `exec.CommandContext` calls — arguments are controlled, not user input
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- `make precommit` must pass
</constraints>

<verification>
Run `make precommit` — must pass.

Verify go-git is removed:
```bash
grep -c "go-git" go.mod
# Expected: 0

grep -r "go-git" pkg/
# Expected: no output
```

Verify MoveFile still works (existing tests):
```bash
go test -v ./pkg/git/... -run "MoveFile"
```

Verify GetNextVersion still works (existing tests):
```bash
go test -v ./pkg/git/... -run "GetNextVersion|NextVersion"
```

Check dependency count reduction:
```bash
wc -l go.sum
# Should be significantly smaller (go-git pulls ~40 indirect deps)
```
</verification>

<success_criteria>
- `github.com/go-git/go-git/v5` is not in `go.mod`
- No imports of `go-git` anywhere in the codebase
- All existing tests pass unchanged (or with minimal adaptation)
- `make precommit` passes
- `MoveFile` fallback behavior preserved for non-git directories
- `go.sum` is significantly smaller
</success_criteria>
