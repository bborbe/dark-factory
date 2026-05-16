# Troubleshooting

## Reading prompt-failure errors

When a prompt fails, dark-factory records the error in the prompt file's `lastFailReason`
field and in `.dark-factory.log`.

### Before this fix

The daemon log showed only the exit code and a Go stack trace:

```
time=2026-05-16T13:06:19Z level=ERROR msg="prompt failed" file=120-fix.md error="exit status 2
merge origin/master
github.com/bborbe/errors.Wrap
   ...pkg/git/brancher.go:343
..."
```

The actual reason (`Your local changes would be overwritten by merge`) was absent. The operator
had to SSH into the worktree and re-run `git merge origin/master` manually.

### After this fix

The daemon log contains git's stderr verbatim:

```
time=2026-05-16T13:06:19Z level=ERROR msg="prompt failed" file=120-fix.md error="merge origin/master: exit status 2: error: Your local changes to the following files would be overwritten by merge:\n\tprompts/spec-031.md\nPlease commit your changes or stash them before you merge.\nAborting"
```

The `dark-factory prompt show <id>` output also shows the full error under the `Error:` field.

**Resolution for dirty-tree failures:** commit or stash the listed files in the project
worktree, then run `dark-factory prompt retry` to re-queue the failed prompt.
