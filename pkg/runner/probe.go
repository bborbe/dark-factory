// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/subproc"
)

// probeCommand is the in-container command that verifies /workspace is writable.
const probeCommand = "mkdir -p /workspace/.dark-factory-healthcheck && " +
	"touch /workspace/.dark-factory-healthcheck/probe && " +
	"rm -rf /workspace/.dark-factory-healthcheck && " +
	"echo BOOT_OK"

// BootContainerProbe boots a throwaway container from cfg.ContainerImage and verifies that
// /workspace is writable inside it. Used by the `dark-factory healthcheck` command and
// scenario 003 to detect the "UID remap to root" / mount-permission regression that breaks
// prompt execution.
type BootContainerProbe struct {
	ContainerImage string
	ProjectName    string
	Subproc        subproc.Runner
	ExtraMounts    []config.ExtraMount
	ClaudeDir      string
	// WorkspaceDir is read from os.Getwd() inside Run — no need to inject.
}

// Run starts a one-shot `docker run --rm` probe container and verifies /workspace is writable.
// The container name embeds 8 random bytes (16 hex chars) so two concurrent invocations of
// `dark-factory healthcheck` cannot collide, and the -healthcheck-boot- prefix guarantees no
// collision with prompt containers (which use -exec-) or spec generation containers (-gen-).
//
// On success returns nil. The container is removed automatically by `docker run --rm`.
// On failure returns a wrapped error describing which step failed and the captured output.
func (b *BootContainerProbe) Run(ctx context.Context) error {
	name, err := b.uniqueContainerName()
	if err != nil {
		return errors.Wrap(ctx, err, "generate unique container name")
	}
	args, err := b.buildRunArgs(ctx, name)
	if err != nil {
		return errors.Wrap(ctx, err, "build docker run args")
	}
	return b.runProbe(ctx, name, args)
}

// buildRunArgs assembles the `docker run --rm` arg list for the probe container.
func (b *BootContainerProbe) buildRunArgs(ctx context.Context, name string) ([]string, error) {
	workspaceDir, err := os.Getwd()
	if err != nil {
		return nil, errors.Wrap(ctx, err, "get working directory")
	}
	args := []string{
		"run", "--rm",
		"--name", name,
		"--label", "dark-factory.project=" + b.ProjectName,
		"--entrypoint", "/bin/sh",
		"-v", workspaceDir + ":/workspace",
		"-v", b.ClaudeDir + ":/home/node/.claude",
	}
	for _, m := range b.ExtraMounts {
		src := resolveExtraMountPath(m.Src, workspaceDir)
		if _, serr := os.Stat(src); serr != nil {
			slog.Debug("extraMounts: src path does not exist, skipping", "src", src, "dst", m.Dst)
			continue
		}
		mount := src + ":" + m.Dst
		if m.IsReadonly() {
			mount += ":ro"
		}
		args = append(args, "-v", mount)
	}
	args = append(args, b.ContainerImage, "-c", probeCommand)
	return args, nil
}

// resolveExtraMountPath expands a ~/ prefix against $HOME and resolves a non-absolute
// path against workspaceDir. Inline copy of executor.resolveExtraMountSrc + the
// absolute/relative branch from dockerExecutor.buildDockerCommand so probe.go does not
// need to import the unexported symbol.
func resolveExtraMountPath(src, workspaceDir string) string {
	if strings.HasPrefix(src, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return home + src[1:]
		}
	}
	if !filepath.IsAbs(src) {
		return filepath.Join(workspaceDir, src)
	}
	return src
}

// runProbe invokes `docker run --rm --entrypoint /bin/sh <image> -c <probeCommand>` in a
// single subprocess call. The probe command runs inline inside the container — no separate
// `docker exec` step is required and there is no need to wait for the container to be in
// "running" state because the run is synchronous: the container starts, executes
// probeCommand, prints BOOT_OK, and exits. Exit-non-zero or missing-BOOT_OK both fail the
// probe.
func (b *BootContainerProbe) runProbe(ctx context.Context, name string, args []string) error {
	// #nosec G204 -- args are derived from the configured container image, not user input
	out, err := b.Subproc.RunWithWarnAndTimeout(ctx, "docker run probe", "docker", args...)
	if err != nil {
		slog.Error(
			"container-boot probe docker run failed",
			"image",
			b.ContainerImage,
			"container",
			name,
			"error",
			err,
		)
		return errors.Errorf(
			ctx,
			"probe container boot check failed: stdout=%q stderr=%q",
			truncate(string(out), 200),
			truncate(err.Error(), 200),
		)
	}
	if !strings.Contains(string(out), "BOOT_OK") {
		slog.Error(
			"container-boot probe missing BOOT_OK",
			"image",
			b.ContainerImage,
			"container",
			name,
		)
		return errors.Errorf(
			ctx,
			"probe container boot check failed: missing BOOT_OK marker in stdout=%q",
			truncate(string(out), 200),
		)
	}
	slog.Info("container-boot probe passed", "image", b.ContainerImage, "container", name)
	return nil
}

// uniqueContainerName returns "<projectName>-healthcheck-boot-<16 hex chars>".
// The 8-byte random suffix is from crypto/rand; collision probability is negligible
// even with concurrent invocations, and the prefix guarantees no collision with
// prompt containers (which use "-exec-") or spec generation containers ("-gen-").
func (b *BootContainerProbe) uniqueContainerName() (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return b.ProjectName + "-healthcheck-boot-" + hex.EncodeToString(buf[:]), nil
}

// truncate returns s shortened to at most n bytes, appending "..." if truncation happened.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n < 3 {
		return s[:n]
	}
	return s[:n-3] + "..."
}
