// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package launchpolicy materialises, exactly once per daemon process, the set of
// launch-shape inputs that are intrinsic to "this is a dark-factory container":
// image, project identity, mounts, base environment, netrc/gitconfig paths,
// hide-git, and the canonical Linux capability set (NET_ADMIN, NET_RAW).
//
// Two call sites consume it: the executor's prompt-run path
// (pkg/executor.dockerExecutor.buildDockerCommand) and the healthcheck probes'
// container-launch path (pkg/cmd/healthcheck.runContainerProbe). Both derive
// their executor.ContainerLaunchOpts from the same Policy value via
// Policy.BuildOpts; per-invocation differences (container name, entrypoint,
// command, env overlay, label overlay) flow through the Extras argument.
//
// A reader who wants to add a new launch-shape concern (new cap, new mount,
// new base env var) adds it here once. Both the executor and the probes pick
// it up with no further changes. The canonical container-startup-site
// inventory is maintained in the comment block on Policy itself; the
// inventory's enumerated set of file:line pairs must equal the UNION of:
//
//	grep -rnE 'exec\.Command(Context)?\(.*"docker"' pkg/ | grep -v _test.go
//	grep -rnE 'RunWithWarnAndTimeout\([^,]+,[^,]+,[^,]+"docker"' pkg/ | grep -v _test.go
//
// (Dark-factory invokes `docker` two ways: directly via `exec.Command*` and
// indirectly via `subproc.Runner.RunWithWarnAndTimeout`. The inventory must
// cover both. Any drift between the inventory and the union of the two grep
// outputs is an unresolved architectural divergence and must be fixed in the
// same change that introduced it.)
package launchpolicy

import (
	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/executor"
)

// CanonicalCaps is the Linux capability set every dark-factory container
// requires. NET_ADMIN + NET_RAW are needed by the claude-yolo entrypoint's
// init-firewall.sh (iptables rules) to run on container backends that
// reject those syscalls without explicit grants (e.g. OrbStack).
//
// This is the SINGLE production reference to the capability literals in the
// repository. The architectural invariant
//
//	grep -rn "NET_ADMIN" pkg/ | grep -v _test.go | wc -l
//
// MUST return exactly 1 (this file). Adding the literals anywhere else
// re-introduces spec-098's divergence-by-construction.
var CanonicalCaps = []string{"NET_ADMIN", "NET_RAW"}

// Policy carries the launch-shape inputs intrinsic to every dark-factory
// container. Constructed once per daemon process (or per command invocation)
// from config + environment; consumed by both the executor's prompt-run path
// and the healthcheck probes.
//
// All fields are unexported. Construct via NewPolicy and consume via BuildOpts.
//
// CONTAINER-STARTUP-SITE INVENTORY (spec 098 AC "Container-startup-site
// inventory is complete"). Every production-code site that invokes
// `exec.Command(ctx, "docker", ...)` OR
// `subproc.Runner.RunWithWarnAndTimeout(ctx, op, "docker", ...)` in pkg/
// is classified below. The enumerated set MUST equal the UNION of
//
//	grep -rnE 'exec\.Command(Context)?\(.*"docker"' pkg/ | grep -v _test.go
//	grep -rnE 'RunWithWarnAndTimeout\([^,]+,[^,]+,[^,]+"docker"' pkg/ | grep -v _test.go
//
// at HEAD. CI may grep this comment to detect drift.
//
// Routed through Policy.BuildOpts (docker run / claude-yolo containers):
//
//	pkg/executor/executor.go:517   exec.CommandContext(ctx, "docker", args...)
//	                               -- prompt-run path (buildDockerCommand)
//	                               -- also serves spec generation via the
//	                                  shared executor.NewDockerExecutor
//	pkg/cmd/healthcheck/probes.go:319 a.runner.RunWithWarnAndTimeout(
//	                                    ctx, a.op, "docker",
//	                                    executor.BuildDockerRunArgs(opts)...)
//	                               -- boot / mount / claude probes
//
// Explicitly out of scope (do NOT invoke claude-yolo; carry no caps, no
// mounts, no /workspace bind):
//
//	pkg/cmd/healthcheck/probes.go:115  "docker version"
//	pkg/cmd/healthcheck/probes.go:150  "docker image inspect --format=..."
//	pkg/executor/checker.go:72         "docker inspect --format ..."
//	pkg/executor/checker.go:105        "docker ps ..." (NewDockerContainerChecker)
//	pkg/executor/executor.go:244       "docker logs --follow <name>"
//	pkg/executor/executor.go:298       "docker stop <name>"
//	pkg/executor/executor.go:303       "docker kill <name>"
//	pkg/executor/executor.go:352       "docker stop <name>"
//	pkg/executor/executor.go:600       "docker stop <name>"
//	pkg/executor/executor.go:611       "docker rm -f <name>"
//	pkg/executor/stopper.go:32         "docker stop <name>"
//	pkg/status/status.go:546           "docker ps --filter ..."
//	pkg/status/status.go:569           "docker ps --filter ..."
//
// Out-of-scope rationale: these sites do not start a claude-yolo container;
// they query/inspect/stop/kill/log existing containers. They have no mount,
// env, capability, or hide-git surface.
type Policy struct {
	containerImage string
	projectName    string
	projectRoot    string
	claudeDir      string
	home           string
	baseEnv        map[string]string
	extraMounts    []config.ExtraMount
	netrcFile      string
	gitconfigFile  string
	hideGit        bool
	capAdd         []string
}

// NewPolicy returns a Policy capturing the launch-shape inputs from cfg + the
// resolved process environment (home, projectRoot). capAdd is initialised to
// CanonicalCaps; callers cannot override (see spec 098 Non-goal "Do NOT make
// capabilities configurable").
//
// projectName is the value of the dark-factory.project label.
// projectRoot is the host path mounted at /workspace.
// home is the host HOME, used for ~/ expansion in mount paths.
// baseEnv is the operator-configured env map (cfg.Env) plus daemon-injected
// values such as ANTHROPIC_MODEL. Prompt-specific keys (YOLO_PROMPT_FILE,
// YOLO_OUTPUT) are NOT part of the base — they are passed in via Extras
// so unrelated invocations stay clean.
func NewPolicy(
	containerImage string,
	projectName string,
	projectRoot string,
	claudeDir string,
	home string,
	baseEnv map[string]string,
	extraMounts []config.ExtraMount,
	netrcFile string,
	gitconfigFile string,
	hideGit bool,
) Policy {
	envCopy := make(map[string]string, len(baseEnv))
	for k, v := range baseEnv {
		envCopy[k] = v
	}
	capsCopy := make([]string, len(CanonicalCaps))
	copy(capsCopy, CanonicalCaps)
	return Policy{
		containerImage: containerImage,
		projectName:    projectName,
		projectRoot:    projectRoot,
		claudeDir:      claudeDir,
		home:           home,
		baseEnv:        envCopy,
		extraMounts:    extraMounts,
		netrcFile:      netrcFile,
		gitconfigFile:  gitconfigFile,
		hideGit:        hideGit,
		capAdd:         capsCopy,
	}
}

// Extras carries the per-invocation inputs that differ between the executor's
// prompt-run and the healthcheck probes' one-shot containers. Everything in
// Extras is layered ON TOP of the Policy's base launch shape.
type Extras struct {
	// ContainerName is the value passed to --name. Required.
	ContainerName string
	// Entrypoint is passed as --entrypoint <value> when non-empty.
	Entrypoint string
	// Command is appended after the image (positional container args).
	Command []string
	// EnvOverlay is merged into the Policy's base env. Keys in EnvOverlay
	// win on collision with baseEnv — the executor's prompt-specific values
	// (YOLO_PROMPT_FILE, YOLO_OUTPUT, ANTHROPIC_MODEL) are passed this way.
	EnvOverlay map[string]string
	// ExtraLabels is appended as --label KEY=VALUE flags after the project
	// label. Used e.g. by the executor's "dark-factory.prompt=<basename>"
	// label. Empty / nil leaves no extra labels.
	ExtraLabels map[string]string
}

// BuildOpts returns an executor.ContainerLaunchOpts ready for
// executor.BuildDockerRunArgs. The returned value carries the policy's
// base launch shape plus the per-invocation extras.
//
// THIS IS THE ONLY production-code site that composes
// executor.ContainerLaunchOpts{...} after spec 098 lands. The architectural
// invariant
//
//	grep -rn "ContainerLaunchOpts{" pkg/ | grep -v _test.go | wc -l
//
// MUST return exactly 1 (this method).
func (p Policy) BuildOpts(extras Extras) executor.ContainerLaunchOpts {
	mergedEnv := make(map[string]string, len(p.baseEnv)+len(extras.EnvOverlay))
	for k, v := range p.baseEnv {
		mergedEnv[k] = v
	}
	for k, v := range extras.EnvOverlay {
		mergedEnv[k] = v
	}
	return executor.ContainerLaunchOpts{
		ContainerName:  extras.ContainerName,
		ContainerImage: p.containerImage,
		ProjectName:    p.projectName,
		ProjectRoot:    p.projectRoot,
		ClaudeDir:      p.claudeDir,
		Home:           p.home,
		Env:            mergedEnv,
		ExtraMounts:    p.extraMounts,
		NetrcFile:      p.netrcFile,
		GitconfigFile:  p.gitconfigFile,
		HideGit:        p.hideGit,
		ExtraLabels:    extras.ExtraLabels,
		CapAdd:         p.capAdd,
		Entrypoint:     extras.Entrypoint,
		Command:        extras.Command,
	}
}

// WithCapAddForTest returns a copy of p with capAdd replaced by caps. Test-only
// override; production callers cannot vary the cap set (spec 098 Non-goal).
// The method name carries "ForTest" so reviewers and tooling can flag any
// non-test caller as a violation.
func (p Policy) WithCapAddForTest(caps []string) Policy {
	capsCopy := make([]string, len(caps))
	copy(capsCopy, caps)
	p.capAdd = capsCopy
	return p
}
