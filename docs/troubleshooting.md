# Troubleshooting

When a prompt mysteriously fails or stalls, run `dark-factory healthcheck` first. The command probes Docker, the container image, the boot sequence, the Claude session, the workspace mount, and (when configured) `gh` and notifications. It exits 0 on a full pass and non-zero with a categorized table naming the failing probe. If `healthcheck` passes, the pipeline is green and the failure is in prompt content or the project tree itself — proceed to `dark-factory doctor` next. Healthcheck is all-or-nothing — there is no flag to skip individual probes.

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

### hideGit guidance fragment

When `hideGit=true` is configured (either explicitly in `.dark-factory.yaml` or automatically
when running from a git worktree), every emitted prompt includes a guidance fragment
explaining that `/workspace/.git` appears as a character device by design. The fragment
instructs the agent to run `make precommit` regardless of how `.git` appears and notes that
static analysis tools work normally in this mode.

This behavior is intentional and the fragment cannot be disabled separately from `hideGit`
itself. Set `hideGit: false` in `.dark-factory.yaml` to suppress the fragment (and the
character-device masking).

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

## Preflight baseline failure on daemon start

When the daemon starts, it runs `preflightCommand` (typically `make precommit`) against the
current tree. If that command exits non-zero, the daemon logs:

```
level=ERROR msg="preflight: baseline check FAILED — prompts will not start until baseline is fixed"
level=ERROR msg="preflight baseline broken — dark-factory exiting. Fix the tree (e.g. run the failing command manually), then restart dark-factory."
```

Common cause: `vulncheck` or `lint` fails because a transitive dependency has a newly-disclosed
CVE that the project's pinned version no longer covers (e.g. `golang.org/x/net` < v0.55.0 after
GO-2026-5025…5030 dropped). The fix is to bump the affected modules.

**Resolution: run `updater all`** from the project root. The `updater` tool walks every Go module
under the current directory and bumps the affected modules to their latest tagged version via
`go get <module>@latest` — one targeted upgrade per direct dependency. After `updater all`
completes and you've merged its branch, run `make precommit` locally to confirm the tree is
green, then restart the daemon.

For projects without a published `updater` binary, run the equivalent manually — one targeted
`go get <module>@latest` per affected module, then `go mod tidy`:

```bash
go get golang.org/x/net@latest
go get golang.org/x/crypto@latest
go mod tidy
make precommit
```

**Never use `go get -u`.** It upgrades every direct AND indirect (transitive) dependency in the
module graph — far more than the vuln scanner flagged. That broad sweep can pull in unrelated
breaking changes from deep in the graph, leaving you to diagnose churn that has nothing to do
with the original CVE. Targeted `go get <module>@latest` upgrades only the named module (and
the minimal set of indirects required to satisfy it), keeping the diff small and reviewable.

If `make precommit` still fails after dependency bumps, the failure is not a vuln drift — read
the error output and fix it before restarting the daemon. The daemon is intentionally strict
about baseline health so prompts never execute against a broken tree.

## gosec / errcheck internal error under Go 1.26+

**Symptom:** `make precommit` fails inside the YOLO container (passes on host) with:

```
internal error: package "strconv" without types was imported from "go/build"
exit status 1
```

The same error shape (`package "X" without types was imported from "Y"`) appears for the
standalone `errcheck` target under Go 1.26.

**Root cause:** Both `errcheck` and `gosec` load packages with `Mode: NeedSyntax | NeedTypes
| NeedTypesInfo` — **without `NeedDeps`**. Under Go 1.26, the typechecker leaves transitive
dependencies' `Types.Complete()` set to `false`, and `golang.org/x/tools/go/packages` fatals
when it encounters one. The failure is intermittent because it depends on which packages get
touched (specifically those pulling in `go/build` transitively, like anything that uses
`strconv`).

It surfaces in the container and not on the host because module-cache and GOMODCACHE state
differ — the host happens to have warm types for the affected dep; the container hits the
cold path.

**Fix:** Drop the standalone `errcheck` and `gosec` targets from `Makefile`. golangci-lint
embeds both linters with the correct loader (uses `NeedDeps`) and works on Go 1.26+.

Per-project recipe:

1. Remove `errcheck` and `gosec` from `check:` chain in `Makefile`
2. Delete `.PHONY: errcheck` and `.PHONY: gosec` target blocks
3. Confirm `.golangci.yml` enables both:
   ```yaml
   linters:
     enable:
       - errcheck
       - gosec
   ```
4. Migrate any `-ignore` (errcheck) or `-exclude=GXXX` (gosec) flags into
   `linters-settings`:
   ```yaml
   linters-settings:
     errcheck:
       exclude-functions:
         - (io.Closer).Close
         - (io.Writer).Write
         - fmt.Fprint
         - fmt.Fprintf
         - fmt.Fprintln
     gosec:
       excludes:
         - G104  # unhandled errors (delegated to errcheck)
         - G115  # integer overflow conversion
   ```
5. Run `make precommit` — must pass.
6. Commit.

**Rejected alternatives:** pin Go to 1.25 (blocks unrelated upgrades), fork the tools and
add `NeedDeps` (maintenance burden), wait for upstream fix (no ETA as of 2026-05).

**Tracking:** the migration across all 35+ active Go projects is tracked in the personal
vault task **Drop Standalone errcheck and gosec Makefile Targets**.
