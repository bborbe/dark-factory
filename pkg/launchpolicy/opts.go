// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package launchpolicy

import "github.com/bborbe/dark-factory/pkg/config"

// ContainerLaunchOpts carries the inputs needed to assemble a `docker run --rm ...` argv
// for any dark-factory container — prompt execution, spec generation, or healthcheck
// probes. Centralising the argv build here keeps the production launch path and the
// healthcheck probes on the same mount/env/hideGit/extraMounts wiring; if production
// stops launching containers correctly, the healthcheck probes notice immediately.
type ContainerLaunchOpts struct {
	// ContainerName is the value passed to `--name`.
	ContainerName string
	// ContainerImage is the image reference (positional, last before Command).
	ContainerImage string
	// ProjectName is the value of the `dark-factory.project=` label.
	ProjectName string
	// ProjectRoot is the host path mounted at /workspace and the base used by
	// HideGit + ExtraMounts path resolution.
	ProjectRoot string
	// ClaudeDir is the host path mounted at /home/node/.claude (auth credentials).
	ClaudeDir string
	// Home is the host's HOME, used for ~/ expansion in NetrcFile/GitconfigFile/ExtraMounts.
	Home string
	// Env is appended as -e KEY=VALUE flags, sorted by key for stable argv shape.
	Env map[string]string
	// ExtraMounts is appended as -v <src>:<dst>[:ro] flags; missing src is skipped + logged.
	ExtraMounts []config.ExtraMount
	// NetrcFile, when non-empty, is mounted at /home/node/.netrc:ro.
	NetrcFile string
	// GitconfigFile, when non-empty, is mounted at /home/node/.gitconfig-extra:ro.
	GitconfigFile string
	// HideGit, when true, masks ProjectRoot/.git inside the container.
	HideGit bool
	// ExtraLabels is appended as additional --label KEY=VALUE flags after the project label.
	ExtraLabels map[string]string
	// CapAdd is appended as --cap-add=<value> flags.
	CapAdd []string
	// Entrypoint, when non-empty, is passed as --entrypoint <value>.
	Entrypoint string
	// Command is appended after the image (positional args to the container).
	Command []string

	// Security hardening fields (ADR-0001 Phase 1 — defaults preserve current behavior).

	// RunAsUser, when non-empty, is passed as --user <value>. Empty means
	// container runs as the image's default user (typically root).
	RunAsUser string
	// MemoryLimit, when non-empty, is passed as --memory <value> (e.g. "8g").
	MemoryLimit string
	// CPULimit, when non-empty, is passed as --cpus <value> (e.g. "4").
	CPULimit string
	// PIDsLimit, when > 0, is passed as --pids-limit <N>. Values <= 0 are
	// treated as "unset" — zero is the field's zero value used as the
	// "unset" sentinel; negative values are silently dropped (docker would
	// reject them anyway and the sentinel semantics are simpler than
	// returning an error from BuildDockerRunArgs).
	PIDsLimit int
	// ClaudeDirReadOnly, when true, mounts ClaudeDir with :ro suffix. Default
	// false preserves rw mount needed for OAuth token refresh.
	ClaudeDirReadOnly bool
}
