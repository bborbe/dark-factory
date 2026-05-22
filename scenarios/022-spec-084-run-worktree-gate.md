---
status: active
---

# Scenario 022: `dark-factory run` refuses to start from worktree CWD without hideGit

Validates that `dark-factory run` (the one-shot subcommand) honors the same worktree/submodule gate as `dark-factory daemon`. Spec 084 AC6 — the regression-prone case where `oneShotRunner.Run` previously bypassed `CheckGitSafety` while `runner.Run` (daemon) called it. Locks down the shared-helper invariant from both entry points.

## Setup

```bash
# Fresh binary from current HEAD
go build -C ~/Documents/workspaces/dark-factory -o /tmp/new-dark-factory .

WORK_DIR=$(mktemp -d)
trap 'rm -rf "$WORK_DIR"' EXIT

# Worktree-shaped CWD: .git is a regular file
mkdir -p "$WORK_DIR/wt"
printf 'gitdir: /nonexistent/worktrees/wt\n' > "$WORK_DIR/wt/.git"

cat > "$WORK_DIR/wt/.dark-factory.yaml" << 'YAML'
workflow: direct
autoRelease: false
YAML

export HOME="$WORK_DIR/home"
mkdir -p "$HOME/.dark-factory"

cd "$WORK_DIR/wt"
```

## Action

- [ ] Invoke the one-shot subcommand: `/tmp/new-dark-factory run > /tmp/df-022.out 2> /tmp/df-022.err; echo "exit=$?" > /tmp/df-022.code`

## Expected

- [ ] Exit code is non-zero: `grep -q 'exit=[1-9]' /tmp/df-022.code`
- [ ] Stderr names the worktree condition: `grep -qE 'worktree.*detected|worktree CWD' /tmp/df-022.err`
- [ ] Stderr names the remediation: `grep -q 'hideGit=true' /tmp/df-022.err`
- [ ] Stderr references the runbook: `grep -q 'PR via Pre-Created Worktree' /tmp/df-022.err`
- [ ] Stderr references the in-repo troubleshooting doc: `grep -q 'docs/troubleshooting.md' /tmp/df-022.err`
- [ ] The error appears BEFORE any processor/container work — stderr does NOT contain `processor started`: `! grep -q 'processor started' /tmp/df-022.err`
- [ ] Re-running with `--set hideGit=true` proceeds past the gate (no worktree-gate error): `/tmp/new-dark-factory run --set hideGit=true > /tmp/df-022-pass.out 2> /tmp/df-022-pass.err & sleep 3; kill $! 2>/dev/null; ! grep -qE 'worktree.*detected|worktree CWD' /tmp/df-022-pass.err`

## Cleanup

`trap` handler removes `$WORK_DIR` automatically.
