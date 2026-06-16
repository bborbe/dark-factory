// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package executor

import (
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/bborbe/dark-factory/pkg/config"
)

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
}

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
	args = appendExtraLabels(args, opts.ExtraLabels)
	for _, c := range opts.CapAdd {
		args = append(args, "--cap-add="+c)
	}
	args = appendEnv(args, opts.Env)
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

func appendStandardMounts(args []string, opts ContainerLaunchOpts) []string {
	if opts.ProjectRoot != "" {
		args = append(args, "-v", opts.ProjectRoot+":/workspace")
	}
	if opts.ClaudeDir != "" {
		args = append(args, "-v", opts.ClaudeDir+":/home/node/.claude")
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
