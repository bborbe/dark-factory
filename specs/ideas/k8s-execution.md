---
tags:
  - dark-factory
  - spec
status: idea
---

## Summary

- Dark-factory runs as a K8S Deployment with a git-sync sidecar
- git-sync keeps a local directory in sync with a Git repository
- Dark-factory daemon watches the synced directory — same as local execution
- When approved prompts/specs are detected, YOLO containers run as K8S Jobs
- No custom K8S controller needed — reuse existing daemon + directory watching

## Problem

Dark-factory currently requires a developer's machine running Docker to process prompts. Execution stops when the machine sleeps, the terminal closes, or the developer goes offline. Teams sharing a repository cannot use a single always-on dark-factory instance without dedicating a physical machine.

## Goal

After this work, dark-factory runs inside a Kubernetes cluster as a Deployment. A git-sync sidecar keeps the project directory in sync with a remote Git repository. The dark-factory daemon watches this directory — identical to local mode — and spawns YOLO containers as K8S Jobs when approved items appear. Runs unattended, 24/7.

## Architecture

```
┌─────────────────────────────────────────────────┐
│ Deployment: dark-factory                         │
│                                                  │
│  ┌──────────────┐    ┌───────────────────────┐  │
│  │  git-sync    │    │  dark-factory daemon   │  │
│  │  sidecar     │◄──►│  watches /repo dir     │  │
│  │              │    │                        │  │
│  └──────┬───────┘    └───────────┬────────────┘  │
│         │ shared volume          │               │
│         │ /repo                  │ creates       │
│         │                        ▼               │
│  ┌──────┴───────┐    ┌───────────────────────┐  │
│  │  Git remote  │    │  K8S Job              │  │
│  │  (GitHub)    │    │  claude-yolo container │  │
│  └──────────────┘    └───────────────────────┘  │
└─────────────────────────────────────────────────┘
```

## Non-goals

- Helm chart (separate concern)
- Multi-tenant (one repo per Deployment)
- Custom CRDs or operator pattern
- Web UI (use kubectl + dark-factory status)
- Replacing local Docker executor (both coexist)

## Desired Behavior

1. **git-sync sidecar**: Standard `registry.k8s.io/git-sync` container runs alongside dark-factory. Clones repo to a shared volume (`/repo`), polls on configurable interval (default: 30s). After YOLO Job completes, git-sync pushes changes back. Authentication via K8S Secret (SSH key or netrc).

2. **Dark-factory daemon watches /repo**: The daemon runs with `--project-dir /repo` (or equivalent config). It watches the synced directory for approved prompts/specs — same poll loop as local mode. No new code needed for detection logic.

3. **K8S Job executor**: When a prompt needs execution, dark-factory creates a K8S Job instead of `docker run`. The Job runs the same claude-yolo container image. The executor is selected by config (`executor: kubernetes` vs `executor: docker`).

4. **Shared volume**: The repo directory is a shared volume (emptyDir or PVC) mounted in both the git-sync sidecar and the dark-factory daemon container. YOLO Jobs get the project directory mounted as well.

5. **Secret injection**: Anthropic API key, git credentials, and `.claude-yolo` config are provided via K8S Secrets. Mounted into Jobs the same way local mode mounts `~/.claude-yolo` and `~/.netrc`.

6. **Concurrency**: Existing `maxContainers` global limit applies. Before creating a Job, daemon counts active Jobs with `dark-factory.project` label. Same logic as local Docker container counting.

7. **Git push after completion**: When a YOLO Job completes, the daemon updates prompt/spec status in `/repo`. git-sync detects the changes and pushes to remote. Alternatively, daemon pushes directly using mounted git credentials.

## Constraints

- Reuse existing daemon/runner/processor code — no separate controller binary
- K8S executor implements existing Executor interface (Execute, Reattach, StopAndRemoveContainer)
- Same DARK-FACTORY-REPORT parsing from Job container logs
- Claude-yolo container image unchanged
- Local Docker executor continues working
- All existing tests pass, `make precommit` passes

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Daemon pod restarts | Scans for executing prompts, checks for matching Jobs (like local resume) | Automatic |
| Job fails (OOM, crash) | Daemon reads logs, marks prompt failed | User retries prompt |
| Job hangs (no REPORT after timeout) | Daemon deletes Job, marks prompt failed | User retries |
| git-sync push conflict | git-sync retries with rebase | Resolves automatically or user fixes |
| git-sync cannot reach remote | git-sync retries with backoff, daemon continues with stale state | Network recovers |
| Secret missing | Job creation fails, prompt marked failed with clear error | User fixes Secret |

## Security / Abuse Cases

- API key stored as K8S Secret, never in Git or ConfigMap, never logged
- Git credentials: minimal permissions (single repo read/write)
- Jobs: no privileged access, no host mounts, automountServiceAccountToken: false
- Resource limits on Jobs prevent cluster impact
- NetworkPolicy: Jobs need outbound HTTPS only (Anthropic API, git remote)

## Acceptance Criteria

- [ ] K8S executor implements existing Executor interface
- [ ] Config selects between Docker and K8S executor
- [ ] git-sync sidecar keeps `/repo` in sync with remote
- [ ] Daemon watches synced directory, detects approved prompts/specs
- [ ] YOLO containers run as K8S Jobs with same image as local
- [ ] Secrets injected via K8S Secrets (API key, git creds, claude-yolo config)
- [ ] Concurrency limit controls max simultaneous Jobs
- [ ] Daemon recovers after pod restart
- [ ] Local Docker executor unchanged
- [ ] `make precommit` passes

## Verification

```bash
make precommit
```

Manual verification:

1. Deploy dark-factory + git-sync to K8S cluster pointing at test repo
2. Push an approved prompt to the repo
3. Observe: git-sync pulls, daemon detects, Job spawns, prompt completes, changes pushed
4. Kill daemon pod during Job execution — Job continues, restarted daemon re-attaches
5. Set maxContainers: 1, approve 3 prompts — Jobs execute one at a time

## Do-Nothing Option

Keep dark-factory as local-only. Developers must keep a machine running with Docker. For teams or overnight processing, dedicate a machine as build server. Works for individuals, bottleneck for teams.
