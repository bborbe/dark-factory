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

## Running dark-factory from a git worktree or submodule

When dark-factory is started from a git worktree (where `.git` is a regular file pointing to
`<parent>/.git/worktrees/<name>`) or from a git submodule (where `.git` contains
`gitdir: ../.git/modules/<name>`), the container mount cannot follow the pointer back to the
parent repo's git metadata. This causes git commands inside the container to fail with:

```
fatal: not a git repository: <parent>/.git/worktrees/<name>
```

The remediation is to set `hideGit: true` in `.dark-factory.yaml` or pass `--set hideGit=true`
on the command line. When `hideGit=true`, the `.git` pointer is masked so the container
workspace is treated as a non-git directory, which works correctly for both prompt execution
and spec generation.

See the 'PR via Pre-Created Worktree' runbook for the canonical workflow.

Auto-enabling `hideGit` when a worktree is detected was considered but rejected in favor of
explicit configuration.
