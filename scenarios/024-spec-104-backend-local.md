---
status: active
---

# Scenario 024: backend: local runs in-process and fails closed, never touching docker

Validates that with `backend: local` (spec 104), dark-factory resolves the local
execution backend from config, reaches prompt execution, and — when `claude` is
absent from `PATH` — fails closed via `localSubprocessExecutor` with `claude not
found on PATH`, **without spawning any docker container or contacting a docker
daemon**. This is the load-bearing guarantee the `github-dark-factory-agent` pod
depends on (the pod is already a container; a nested `docker run` would be DinD).
Spec 104 AC 4/6/9.

The default (`docker`) mode is already covered by `001-workflow-direct` and
`003-smoke-test-container`; this scenario covers the new `local` mode. One journey
per file (guide rule 3): the fail-closed / no-docker path.

## Setup

```bash
# Fresh binary from current HEAD (test the code under change, not an installed binary).
go build -C ~/Documents/workspaces/dark-factory -o /tmp/new-dark-factory .

WORK_DIR=$(mktemp -d)
trap 'rm -rf "$WORK_DIR"' EXIT

# A real git repo with a local bare remote so workflow:direct's `git fetch origin`
# in Setup succeeds and execution proceeds to the backend. preflightCommand="" so the
# baseline gate (make precommit) does not run before execution and mask the backend.
git init --bare "$WORK_DIR/remote.git" >/dev/null 2>&1
mkdir -p "$WORK_DIR/proj"; cd "$WORK_DIR/proj"
git init -q -b master; git config user.email t@t; git config user.name t
cat > .dark-factory.yaml << 'YAML'
workflow: direct
autoRelease: false
preflightCommand: ""
backend: local
YAML
echo x > file.txt; git add -A; git commit -qm init
git remote add origin "$WORK_DIR/remote.git"; git push -q origin master

# One approved prompt for the queue. Its body is irrelevant — execution fails at the
# claude PATH lookup before any prompt content runs.
mkdir -p prompts/in-progress prompts/completed prompts/log
printf -- '---\nstatus: approved\n---\n\n# Noop\n\nfail closed.\n' > prompts/in-progress/001-noop.md

# Isolate global config so the host's ~/.config is not consulted.
export HOME="$WORK_DIR/home"; mkdir -p "$HOME/.dark-factory"

# Build a run PATH with every dir containing a `claude` executable removed (keeps git,
# sh, coreutils — only claude is made unfindable, exercising the fail-closed path).
RUN_PATH="$PATH"
while cp=$(PATH="$RUN_PATH" command -v claude 2>/dev/null); do
  d=$(dirname "$cp")
  RUN_PATH=$(printf '%s' "$RUN_PATH" | tr ':' '\n' | grep -vx "$d" | paste -sd: -)
done
```

- [ ] `claude` is unfindable under `$RUN_PATH`: `PATH="$RUN_PATH" command -v claude` prints nothing
- [ ] `git` is still present: `PATH="$RUN_PATH" command -v git` prints a path

## Action

- [ ] Run one-shot with `claude` absent from `PATH`:
  `env PATH="$RUN_PATH" /tmp/new-dark-factory run > /tmp/df-024.out 2> /tmp/df-024.err`

## Expected

- [ ] Local backend resolved from the project file: `grep -qE 'backend=local.*backendSource=project' /tmp/df-024.err`
- [ ] Execution reached the backend: `grep -q 'executing prompt' /tmp/df-024.err`
- [ ] Failed closed via the local executor: `grep -q 'claude not found on PATH' /tmp/df-024.err` (the error originates in `pkg/executor/local_subprocess.go` `Execute`, never falling back to docker)
- [ ] **NO docker container was spawned** (the whole point of `backend: local`): `docker ps -a --filter 'name=dark-factory-exec' --filter 'name=proj-exec' --format '{{.Names}}' | wc -l` returns 0
- [ ] **No docker daemon was contacted** — the run reached execution with the healthcheck gate disabled and never errored on docker: `! grep -qiE 'cannot connect to the docker daemon|is the docker daemon running' /tmp/df-024.err`
- [ ] The prompt did not complete: `test -f prompts/in-progress/001-noop.md && ! test -f prompts/completed/001-noop.md` (and its frontmatter shows `status: failed`)

## Cleanup

`trap` handler removes `$WORK_DIR` on shell exit. The `docker ps -a` assertion is
read-only; no container is created, so there is nothing to remove.

## Notes

- Fail-closed is also unit-tested (`TestLocalMissingClaudeFailsClosed` in
  `pkg/executor/local_subprocess_test.go`). This scenario adds the runtime proof
  that unit tests cannot reach: the factory→processor wiring selects the local
  backend and the full `dark-factory run` path spawns no container and needs no
  docker daemon.
- Known cosmetic wart (not asserted): the processor logs the executor error under
  `msg="docker container exited with error"` even for `backend: local`. The
  behavior is correct (no docker); only the log string uses container vocabulary.
