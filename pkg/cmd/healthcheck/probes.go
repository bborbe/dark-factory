// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package healthcheck provides the local-probe implementations used by the
// `dark-factory healthcheck` subcommand. Each Probe covers one category of
// the pipeline-execution stack (Docker daemon, container image presence,
// container boot, workspace mount writability). The Claude / gh / notifications
// probes are appended by prompt 2b.
package healthcheck

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/runner"
	"github.com/bborbe/dark-factory/pkg/subproc"
)

//counterfeiter:generate -o ../../../mocks/healthcheck-probe.go --fake-name HealthcheckProbe . Probe

// Probe executes a single healthcheck category. Implementations must be
// safe to run sequentially and must respect ctx cancellation.
type Probe interface {
	// Name returns the probe category (e.g. "docker", "image"). Stable across runs.
	Name() string
	// Run executes the probe; returns nil on success, an error otherwise.
	Run(ctx context.Context) error
}

// Thresholds for the local probes. They are deliberately longer than the
// default subproc thresholds (3s/10s) because docker image inspect against a
// large image and a container boot+exec cycle can take a few seconds on a
// cold daemon.
const (
	dockerWarnAfter = 3 * time.Second
	dockerTimeout   = 10 * time.Second
	mountTimeout    = 15 * time.Second
)

// dockerProbe checks that the docker daemon is reachable.
type dockerProbe struct {
	runner subproc.Runner
}

// NewDockerProbe returns a Probe that runs `docker version` and reports a
// failure if the daemon is not reachable.
func NewDockerProbe(r subproc.Runner) Probe {
	return &dockerProbe{runner: r}
}

func (d *dockerProbe) Name() string {
	return "docker"
}

func (d *dockerProbe) Run(ctx context.Context) error {
	out, err := d.runner.RunWithWarnAndTimeout(
		ctx,
		"docker version",
		"docker",
		"version",
	)
	if err != nil {
		if isDockerDaemonUnreachable(out) {
			slog.Error("healthcheck probe failed", "probe", d.Name(), "error", err)
			return errors.Wrapf(ctx, err, "docker daemon unreachable")
		}
		slog.Error("healthcheck probe failed", "probe", d.Name(), "error", err)
		return errors.Wrapf(ctx, err, "docker daemon unreachable")
	}
	slog.Info("healthcheck probe passed", "probe", d.Name())
	return nil
}

// isDockerDaemonUnreachable matches the canonical "Cannot connect to the Docker
// daemon" message in captured stdout/stderr from a failed `docker version`.
func isDockerDaemonUnreachable(out []byte) bool {
	return strings.Contains(string(out), "Cannot connect to the Docker daemon")
}

// imageProbe checks that the configured container image is present locally.
type imageProbe struct {
	containerImage string
	runner         subproc.Runner
}

// NewImageProbe returns a Probe that runs `docker image inspect --format={{.Id}} <image>`.
// On failure it returns a wrapped error naming the missing image.
func NewImageProbe(containerImage string, r subproc.Runner) Probe {
	return &imageProbe{containerImage: containerImage, runner: r}
}

func (i *imageProbe) Name() string {
	return "image"
}

func (i *imageProbe) Run(ctx context.Context) error {
	_, err := i.runner.RunWithWarnAndTimeout(
		ctx,
		"docker image inspect "+i.containerImage,
		"docker",
		"image",
		"inspect",
		"--format={{.Id}}",
		i.containerImage,
	)
	if err != nil {
		slog.Error(
			"healthcheck probe failed",
			"probe",
			i.Name(),
			"image",
			i.containerImage,
			"error",
			err,
		)
		return errors.Errorf(
			ctx,
			"container image %q not present locally",
			i.containerImage,
		)
	}
	slog.Info("healthcheck probe passed", "probe", i.Name(), "image", i.containerImage)
	return nil
}

// bootProbe wraps a runner.BootContainerProbe to fit the Probe interface.
// The BootContainerProbe uses a pointer receiver, so callers must pass a
// non-nil pointer.
type bootProbe struct {
	probe *runner.BootContainerProbe
}

// NewBootProbe returns a Probe that boots a throwaway container from the
// configured image and verifies /workspace is writable inside it. The pointer
// is required because runner.BootContainerProbe.Run has a pointer receiver.
func NewBootProbe(p *runner.BootContainerProbe) Probe {
	return &bootProbe{probe: p}
}

func (b *bootProbe) Name() string {
	return "boot"
}

func (b *bootProbe) Run(ctx context.Context) error {
	if err := b.probe.Run(ctx); err != nil {
		slog.Error("healthcheck probe failed", "probe", b.Name(), "error", err)
		return errors.Wrapf(ctx, err, "container boot probe failed")
	}
	slog.Info("healthcheck probe passed", "probe", b.Name())
	return nil
}

// mountProbe checks that /workspace is writable inside a throwaway container.
// It runs `docker run --rm <image> sh -c '... echo MOUNT_OK'` and asserts the
// expected marker in stdout.
type mountProbe struct {
	containerImage string
	runner         subproc.Runner
	timeout        time.Duration
	warnAfter      time.Duration
}

// mountProbeCommand is the in-container shell command that verifies /workspace
// is writable. The mkdir/touch/rm sequence is the actual write test; the
// MOUNT_OK echo is the success marker.
const mountProbeCommand = "mkdir -p /workspace/.healthcheck-mount && " +
	"touch /workspace/.healthcheck-mount/p && " +
	"rm -rf /workspace/.healthcheck-mount && " +
	"echo MOUNT_OK"

// NewMountProbe returns a Probe that boots a throwaway container and verifies
// /workspace is writable inside it. The container image is the same one
// passed to the boot probe.
func NewMountProbe(containerImage string, r subproc.Runner) Probe {
	return &mountProbe{
		containerImage: containerImage,
		runner:         r,
		timeout:        mountTimeout,
		warnAfter:      dockerWarnAfter,
	}
}

func (m *mountProbe) Name() string {
	return "mount"
}

func (m *mountProbe) Run(ctx context.Context) error {
	out, err := m.runner.RunWithWarnAndTimeout(
		ctx,
		"docker run mount probe",
		"docker",
		"run",
		"--rm",
		m.containerImage,
		"sh",
		"-c",
		mountProbeCommand,
	)
	if err != nil {
		slog.Error(
			"healthcheck probe failed",
			"probe",
			m.Name(),
			"stdout",
			truncate(string(out)),
			"error",
			err,
		)
		return errors.Wrapf(
			ctx,
			err,
			"workspace mount not writable: stdout=%q",
			truncate(string(out)),
		)
	}
	if !strings.Contains(string(out), "MOUNT_OK") {
		slog.Error(
			"healthcheck probe failed",
			"probe",
			m.Name(),
			"stdout",
			truncate(string(out)),
		)
		return errors.Errorf(
			ctx,
			"workspace mount not writable: stdout=%q",
			truncate(string(out)),
		)
	}
	slog.Info("healthcheck probe passed", "probe", m.Name())
	return nil
}

// truncate shortens s to at most 200 bytes, appending "..." if truncation happened.
func truncate(s string) string {
	const n = 200
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
