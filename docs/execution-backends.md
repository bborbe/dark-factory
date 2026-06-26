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

To add a hypothetical `LocalSubprocessExecutor` — an executor that runs the YOLO agent as a local subprocess instead of a Docker container — exactly three files need to change:

- `pkg/executor/local_subprocess.go` — new file: a `localSubprocessExecutor` struct implementing the existing `Executor` interface (`Execute`, `Reattach`, `StopAndRemoveContainer`). If the backend needs liveness and stop support, `ExecutionChecker` and `ExecutionStopper` implementations go here too, each with a `NewLocalSubprocess*` constructor. No new interface is introduced — the implementations satisfy the existing neutral interfaces directly.

- `pkg/factory/factory.go` — one wiring change: select `NewLocalSubprocessExecutor` instead of `NewDockerExecutor` (lines 661 and 988), and the matching checker/stopper instead of `NewDockerExecutionChecker` (lines 668 and 768) and `NewDockerExecutionStopper` (line 481), behind a config switch. Every other factory line is unchanged.

- `pkg/config/...` (optional) — a config field selecting the backend (e.g. `backend: docker|local`). This third file is only required if the switch lives in config rather than a build tag; it may be omitted for a build-tag-based selection, keeping the total at two files.

No caller package changes — `pkg/runner`, `pkg/promptresumer`, `pkg/processor`, and the rest already depend only on the neutral `Executor`, `ExecutionChecker`, and `ExecutionStopper` interfaces, so they compile unchanged. This is the proof that the abstraction holds.
