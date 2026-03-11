---
status: created
created: "2026-03-11T16:45:24Z"
---

<summary>
- Path traversal protection is strengthened with an explicit containment check after path joining
- The queue action handler verifies the resolved path stays within the expected directory
- Symlink-based and non-Unix path traversal vectors are mitigated
</summary>

<objective>
Add an explicit path containment check in the queue action handler after `filepath.Join(inboxDir, filename)` to verify the resulting path is still within the expected directory. The existing `filepath.Base()` sanitization is good but insufficient on systems with symlinks.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/server/queue_action_handler.go` — find the `queueSingleFile` function and the path handling around ~line 105-114 where `filepath.Base` is applied.
</context>

<requirements>
1. In `pkg/server/queue_action_handler.go`, after the existing `filepath.Base` sanitization and before calling `queueSingleFile`, add a containment check:
   ```go
   safePath := filepath.Join(inboxDir, filename)
   cleanInbox := filepath.Clean(inboxDir) + string(os.PathSeparator)
   if !strings.HasPrefix(filepath.Clean(safePath)+string(os.PathSeparator), cleanInbox) {
       return libhttp.WrapWithStatusCode(
           errors.New(ctx, "path escapes inbox directory"),
           http.StatusBadRequest,
       )
   }
   ```

2. Add imports for `"strings"` and `"os"` if not already present.

3. Add a test case in `pkg/server/queue_action_handler_test.go` that verifies a crafted filename that might escape the directory is rejected with HTTP 400.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
- Use `github.com/bborbe/errors` for error construction.
- Use `github.com/bborbe/http` (`libhttp`) for status code wrapping — already imported.
</constraints>

<verification>
Run `make precommit` -- must pass.
</verification>
