---
status: completed
summary: Modified createPromptTempFile to create files inside a restricted os.MkdirTemp subdirectory and updated cleanup to use os.RemoveAll on the directory
container: dark-factory-189-use-restricted-temp-dir
dark-factory-version: v0.48.0
created: "2026-03-11T16:45:24Z"
queued: "2026-03-11T18:25:03Z"
started: "2026-03-12T00:17:58Z"
completed: "2026-03-12T00:22:01Z"
---

<summary>
- Prompt temp files are created in a restricted subdirectory instead of the shared system temp directory
- The subdirectory is created with `os.MkdirTemp` which applies restrictive permissions by default
- Prompt content (which may contain sensitive AI instructions) is protected from other local processes
- The restricted temp directory and all its contents are cleaned up via `os.RemoveAll` after use
- The function signature and return type remain unchanged — callers are unaffected
</summary>

<objective>
Change the prompt temp file creation in the executor to use a private temporary directory (`os.MkdirTemp` with mode 0700) instead of creating files directly in `/tmp`. This prevents other local processes from reading prompt content during the window between file creation and Docker mount.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/executor/executor.go` — find `createPromptTempFile` (~line 165). It currently uses `os.CreateTemp("", "dark-factory-prompt-*.md")` which creates files in the system temp directory.
</context>

<requirements>
1. In `pkg/executor/executor.go`, modify `createPromptTempFile` to first create a restricted temp directory:
   ```go
   func createPromptTempFile(ctx context.Context, promptContent string) (string, func(), error) {
       tmpDir, err := os.MkdirTemp("", "dark-factory-*")
       if err != nil {
           return "", nil, errors.Wrap(ctx, err, "create temp directory")
       }

       promptFile, err := os.CreateTemp(tmpDir, "prompt-*.md")
       if err != nil {
           _ = os.RemoveAll(tmpDir)
           return "", nil, errors.Wrap(ctx, err, "create prompt temp file")
       }

       cleanup := func() {
           promptFile.Close()
           _ = os.RemoveAll(tmpDir)
       }

       if _, err := promptFile.WriteString(promptContent); err != nil {
           cleanup()
           return "", nil, errors.Wrap(ctx, err, "write prompt content")
       }
       if err := promptFile.Close(); err != nil {
           _ = os.RemoveAll(tmpDir)
           return "", nil, errors.Wrap(ctx, err, "close prompt file")
       }
       // After successful close, update cleanup to only remove the dir (no double-close):
       cleanup = func() {
           _ = os.RemoveAll(tmpDir)
       }
       return promptFile.Name(), cleanup, nil
   }
   ```
   Read the existing function body to understand the exact write/close flow, and adapt the snippet above to match. The key change is: create `tmpDir` first, create the file inside it, and update `cleanup` to remove the entire directory.

2. Update the cleanup function to remove the entire temp directory (`os.RemoveAll(tmpDir)`) instead of just the single file.

3. The returned file path must still be the prompt file path (not the directory), since it is mounted into the Docker container.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
- Use `github.com/bborbe/errors` for error wrapping — already imported.
- The function signature must not change — callers are unaffected.
</constraints>

<verification>
Run `make precommit` -- must pass.
</verification>
