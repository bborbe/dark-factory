// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package executor

import (
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/bborbe/dark-factory/pkg/launchpolicy"
)

// ContainerLaunchOpts is an alias for launchpolicy.ContainerLaunchOpts.
// The type definition lives in pkg/launchpolicy to avoid a circular import
// (executor imports launchpolicy; launchpolicy must not import executor).
// Callers continue using executor.ContainerLaunchOpts with no source change.
type ContainerLaunchOpts = launchpolicy.ContainerLaunchOpts

// EnvManagedMarker is the environment variable dark-factory sets on every
// container it launches. Its presence signals to processes inside the container
// that the dark-factory daemon manages this container's lifecycle — for example,
// the generate-prompts-for-spec command skips its host-only `spec mark-prompted`
// step because the host-side generator finalizes the spec after the container
// exits (and the dark-factory CLI is not present inside the container).
const EnvManagedMarker = "DARK_FACTORY_MANAGED"

// BuildDockerRunArgs returns the argv for `docker run --rm` from opts.
// First argv element is "run"; caller invokes via `exec.CommandContext(ctx, "docker", args...)`
// or `subproc.Runner.RunWithWarnAndTimeout(ctx, op, "docker", args...)`.
//
// The returned argv shape matches what dark-factory's executor produces for prompt
// containers, minus the prompt-file mount and the YOLO_PROMPT_FILE/YOLO_OUTPUT env vars
// (those are prompt-specific and not part of the shared launch surface).
//
// Stable argv shape: env keys are sorted, mount order is deterministic. Callers can grep
// the argv in tests without flakiness.
func BuildDockerRunArgs(opts ContainerLaunchOpts) []string {
	args := []string{
		"run", "--rm",
		"--name", opts.ContainerName,
		"--label", "dark-factory.project=" + opts.ProjectName,
	}
	// host.docker.internal lets containers reach the host machine.
	// Docker Desktop / OrbStack / Rancher Desktop on macOS auto-provide
	// this alias; raw Linux dockerd does not. --add-host is a no-op when
	// the alias already exists (last-writer-wins with the same value)
	// and a real fix on Linux, so emit it unconditionally.
	args = append(args, "--add-host=host.docker.internal:host-gateway")
	args = appendSecurityLimits(args, opts)
	args = appendExtraLabels(args, opts.ExtraLabels)
	for _, c := range opts.CapAdd {
		args = append(args, "--cap-add="+c)
	}
	args = appendEnv(args, withManagedMarker(opts.Env))
	args = appendStandardMounts(args, opts)
	args = appendExtraMounts(args, opts)
	args = append(args, buildHideGitArgsForRoot(opts.HideGit, opts.ProjectRoot)...)
	if opts.Entrypoint != "" {
		args = append(args, "--entrypoint", opts.Entrypoint)
	}
	args = append(args, opts.ContainerImage)
	args = append(args, opts.Command...)
	return args
}

// appendSecurityLimits emits --user, --memory, --cpus, --pids-limit when the
// corresponding opts field is set (ADR-0001 Phase 1). Empty / zero values are
// skipped so production defaults match pre-ADR behavior.
func appendSecurityLimits(args []string, opts ContainerLaunchOpts) []string {
	if opts.RunAsUser != "" {
		args = append(args, "--user", opts.RunAsUser)
	}
	if opts.MemoryLimit != "" {
		args = append(args, "--memory", opts.MemoryLimit)
	}
	if opts.CPULimit != "" {
		args = append(args, "--cpus", opts.CPULimit)
	}
	if opts.PIDsLimit > 0 {
		args = append(args, "--pids-limit", strconv.Itoa(opts.PIDsLimit))
	}
	return args
}

func appendExtraLabels(args []string, labels map[string]string) []string {
	if len(labels) == 0 {
		return args
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "--label", k+"="+labels[k])
	}
	return args
}

func appendEnv(args []string, env map[string]string) []string {
	if len(env) == 0 {
		return args
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "-e", k+"="+env[k])
	}
	return args
}

// withManagedMarker returns a copy of env with the daemon-owned EnvManagedMarker
// set to "true". The marker is written after copying so it takes precedence over
// any inbound value for the same key. A new map is returned so the caller's map
// is never mutated, and a non-nil map is always returned so the marker is emitted
// even when opts.Env is nil.
func withManagedMarker(env map[string]string) map[string]string {
	merged := make(map[string]string, len(env)+1)
	maps.Copy(merged, env)
	merged[EnvManagedMarker] = "true"
	return merged
}

// buildClaudeDirMount returns the `-v` value for the claudeDir mount, or ""
// if no mount should be emitted. Returns "" when ClaudeDir is empty or
// contains ':' (which would produce ambiguous docker volume syntax). The
// ':' case logs at ERROR so the agent's missing-credential failure is
// traceable.
//
// TODO(ADR-0001 Phase 2): validate at config-load time so the operator sees
// the error before daemon start, instead of relying on this silent-skip +
// log line.
func buildClaudeDirMount(opts ContainerLaunchOpts) string {
	if opts.ClaudeDir == "" {
		return ""
	}
	if strings.Contains(opts.ClaudeDir, ":") {
		slog.Error(
			"launchpolicy: ClaudeDir contains ':', skipping mount to avoid ambiguous docker volume syntax",
			"claudeDir",
			opts.ClaudeDir,
		)
		return ""
	}
	mount := opts.ClaudeDir + ":/home/node/.claude"
	if opts.ClaudeDirReadOnly {
		mount += ":ro"
	}
	return mount
}

func appendStandardMounts(args []string, opts ContainerLaunchOpts) []string {
	if opts.ProjectRoot != "" {
		args = append(args, "-v", opts.ProjectRoot+":/workspace")
	}
	if mount := buildClaudeDirMount(opts); mount != "" {
		args = append(args, "-v", mount)
	}
	if opts.NetrcFile != "" {
		args = append(
			args,
			"-v",
			resolveHomePath(opts.NetrcFile, opts.Home)+":/home/node/.netrc:ro",
		)
	}
	if opts.GitconfigFile != "" {
		args = append(
			args,
			"-v",
			resolveHomePath(opts.GitconfigFile, opts.Home)+":/home/node/.gitconfig-extra:ro",
		)
	}
	return args
}

func appendExtraMounts(args []string, opts ContainerLaunchOpts) []string {
	for _, m := range opts.ExtraMounts {
		src := resolveExtraMountSrc(m.Src, os.Getenv, runtime.GOOS)
		switch {
		case strings.HasPrefix(src, "~/"):
			src = opts.Home + src[1:]
		case !filepath.IsAbs(src):
			src = filepath.Join(opts.ProjectRoot, src)
		}
		if _, err := os.Stat(src); err != nil {
			slog.Debug("extraMounts: src path does not exist, skipping", "src", src, "dst", m.Dst)
			continue
		}
		mount := src + ":" + m.Dst
		if m.IsReadonly() {
			mount += ":ro"
		}
		args = append(args, "-v", mount)
	}
	return args
}

func resolveHomePath(path, home string) string {
	if strings.HasPrefix(path, "~/") {
		return home + path[1:]
	}
	return path
}

// buildHideGitArgsForRoot returns the .git masking args for projectRoot when hideGit is true.
// Free-function variant of dockerExecutor.buildHideGitArgs so BuildDockerRunArgs and the
// method share one implementation.
func buildHideGitArgsForRoot(hideGit bool, projectRoot string) []string {
	if !hideGit {
		return nil
	}
	gitPath := filepath.Join(projectRoot, ".git")
	fi, statErr := os.Stat(gitPath)
	if statErr != nil {
		if !os.IsNotExist(statErr) {
			slog.Debug("hideGit: stat .git failed, skipping mount mask", "error", statErr)
		}
		return nil
	}
	if fi.IsDir() {
		return []string{"--tmpfs", "/workspace/.git:rw,size=1k"}
	}
	return []string{"-v", "/dev/null:/workspace/.git"}
}
