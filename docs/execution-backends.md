# Execution Backends

The `Executor` interface and its collaborators (`ExecutionChecker`, `ExecutionStopper`) form a backend-neutral abstraction: callers speak `executionID`, not `containerName`, and depend only on the neutral interfaces. Today there is one backend — the docker-CLI implementation — but the abstraction is designed so a second backend can be added by touching three files or fewer, without any change to the neutral-layer packages. This split was established in spec 102.

## Neutral vs Container Vocabulary

Two categories of packages exist in dark-factory. Each package belongs to exactly one:

**Neutral packages** — use the abstraction; must NOT contain container vocabulary. Guarded by `make hotpath-execution-naming-check` (wired into `make precommit`), which fails the build if any of the tokens `containerName`, `ContainerChecker`, `ContainerStopper`, or `containerslot` appear in these packages:

- `pkg/factory` — wires the chosen backend into the dependency graph
- `pkg/runner` — drives the prompt lifecycle (start, wait, reattach, stop)
- `pkg/promptresumer` — resumes executions across restarts
- `pkg/cancellationwatcher` — watches for cancellation signals
- `pkg/queuescanner` — scans the prompt queue for work
- `pkg/healthcheckgate` — verifies the execution backend is reachable before processing
- `pkg/generator` — generates prompts from approved specs
- `pkg/processor` — core orchestration loop
- `pkg/executionslot` — allocates exclusive slots keyed by `executionID`

These packages speak `executionID` / `ExecutionChecker` / `ExecutionStopper` — no docker concepts.

**Docker-CLI (container-flavored) packages** — own the docker concepts; container vocabulary is correct and intentional here:

- `pkg/executor` — `dockerExecutor`, `dockerExecutionChecker`, `dockerExecutionStopper`, `dockerContainerCounter`, and `launch.go`; the docker-CLI implementation of the neutral interfaces
- `pkg/launchpolicy` — docker launch shape (image, mounts, network capabilities, environment)
- `pkg/containerlock` — instance lock backed by a docker container name

`ContainerCounter` and `containerlock` remain container-named intentionally: they describe docker-specific concepts with no neutral-layer equivalent.

Any container token (`containerName`, `ContainerChecker`, `ContainerStopper`, `containerslot`) appearing in a neutral package fails `make hotpath-execution-naming-check`.

## Adding a Backend

The `LocalSubprocessExecutor` (shipped in spec 104, documented below) is the worked example — an executor that runs `claude` as a local subprocess instead of a Docker container. Adding it touched exactly three files, the shape any new backend follows:

- `pkg/executor/local_subprocess.go` — new file: a `localSubprocessExecutor` struct implementing the existing `Executor` interface (`Execute`, `Reattach`, `StopAndRemoveContainer`). Its `ExecutionChecker` and `ExecutionStopper` implementations live here too, each with a `NewLocalSubprocess*` constructor. No new interface is introduced — the implementations satisfy the existing neutral interfaces directly.

- `pkg/factory/factory.go` — the construction sites route through backend-select helpers (`createExecutor` / `createExecutionChecker` / `createExecutionStopper` / `createContainerCounter`) that return the `NewLocalSubprocess*` variant when `cfg.Backend == config.BackendLocal` and the `NewDocker*` variant otherwise. The docker branch calls the same constructors with the same arguments as before, so the default path stays byte-identical. Every other factory line is unchanged.

- `pkg/config/backend.go` — the `backend: docker|local` config field (resolvable via global config, project `.dark-factory.yaml`, or `--set`; default `docker`) that the factory helpers switch on.

No caller package changes — `pkg/runner`, `pkg/promptresumer`, `pkg/processor`, and the rest already depend only on the neutral `Executor`, `ExecutionChecker`, and `ExecutionStopper` interfaces, so they compile unchanged. This is the proof that the abstraction holds. (The one exception is a small `pkg/promptresumer` recovery branch that re-queues a prompt when the local backend returns `ErrReattachUnsupported` — a sentinel on the existing `Reattach` method, not a new interface.)

## The Local Backend (backend: local)

`backend: local` runs `claude` (and each prompt's Definition-of-Done commands) as a **local subprocess in the current working directory** — no `docker run`, no bind mounts, no nested container. It is selected with the `backend` config field (default `docker`) via global config, project `.dark-factory.yaml`, or `--set backend=local`.

**Trust boundary.** The local backend runs `claude` with the **full credentials and filesystem of the dark-factory process** — there is no container sandbox around the agent. This is acceptable **only** because the intended caller — an already-isolated, single-tenant, ephemeral pod such as the `github-dark-factory-agent` Job — **is** the isolation boundary. The pod is the container; dark-factory does not add a second one.

**MUST NOT.** Never enable `backend: local` on a shared, multi-tenant, or developer host: it exposes that host's credentials and filesystem to whatever the prompt content instructs `claude` to do. The feature defaults to `docker`, never auto-enables, and does **not** auto-detect "am I inside a container" — the operator makes an explicit, documented choice.

**Fail-closed.** With `backend: local`, if `claude` is not on `PATH`, dark-factory fails immediately with `claude not found on PATH` and **never silently falls back to docker**.

**Reattach.** A local subprocess dies with the dark-factory process, so on restart the local backend cannot reattach to an in-flight execution. `Reattach` returns `ErrReattachUnsupported`; the resumer recovers by re-queueing the prompt for a fresh run (safe because execution commits per prompt).

**Healthcheck.** The daemon-startup healthcheck's docker probes are **skipped** under `backend: local` (the gate is constructed disabled), so no docker daemon is required at runtime. The toolchain (`claude` plus the prompt's DoD tools) is assumed already present in `PATH`, provisioned by the image — not by dark-factory.
