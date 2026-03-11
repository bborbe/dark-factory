---
status: created
created: "2026-03-11T16:45:24Z"
---

<summary>
- Prompt temp files are created in a restricted directory with 0700 permissions instead of the shared system temp directory
- Prompt content (which may contain sensitive AI instructions) is protected from other local processes
- The restricted temp directory is cleaned up after use
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
       // ... rest of the function unchanged
   ```

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
