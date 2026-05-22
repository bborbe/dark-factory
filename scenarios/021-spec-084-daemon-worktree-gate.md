---
status: active
---

# Scenario 021: daemon refuses to start from worktree CWD without hideGit

Validates that `dark-factory daemon` invoked from a worktree-shaped CWD (`.git` is a regular file) with `hideGit=false` exits non-zero before any container spawns, emitting an actionable error naming the condition + remediation + runbook reference. Spec 084 AC1, AC5, AC7.

## Setup

```bash
# Fresh binary from current HEAD
go build -C ~/Documents/workspaces/dark-factory -o /tmp/new-dark-factory .

WORK_DIR=$(mktemp -d)
trap 'rm -rf "$WORK_DIR"' EXIT

# Worktree-shaped CWD: .git is a regular file (mimics `git worktree add` pointer)
mkdir -p "$WORK_DIR/wt"
printf 'gitdir: /nonexistent/worktrees/wt\n' > "$WORK_DIR/wt/.git"

# Minimal .dark-factory.yaml — hideGit defaults to false
cat > "$WORK_DIR/wt/.dark-factory.yaml" << 'YAML'
workflow: direct
autoRelease: false
YAML

# Isolate global config
export HOME="$WORK_DIR/home"
mkdir -p "$HOME/.dark-factory"

cd "$WORK_DIR/wt"
```

## Action

- [ ] Invoke daemon: `/tmp/new-dark-factory daemon > /tmp/df-021.out 2> /tmp/df-021.err; echo "exit=$?" > /tmp/df-021.code`

## Expected

- [ ] Exit code is non-zero: `grep -q 'exit=[1-9]' /tmp/df-021.code`
- [ ] Stderr names the condition: `grep -qE 'worktree.*detected|worktree CWD' /tmp/df-021.err`
- [ ] Stderr names the remediation: `grep -q 'hideGit=true' /tmp/df-021.err`
- [ ] Stderr references the runbook: `grep -q 'PR via Pre-Created Worktree' /tmp/df-021.err`
- [ ] Stderr references the in-repo troubleshooting doc: `grep -q 'docs/troubleshooting.md' /tmp/df-021.err`
- [ ] No container was launched (AC7): `docker ps --filter 'name=dark-factory' --format '{{.Names}}' | wc -l` returns 0
- [ ] Same gating applies to submodule CWD: repeat the Action from `"$WORK_DIR/wt"` with `.git` containing `gitdir: ../.git/modules/foo` instead of a worktree-style pointer; assert the same stderr substrings and non-zero exit (AC5)

## Cleanup

`trap` handler removes `$WORK_DIR` automatically.
