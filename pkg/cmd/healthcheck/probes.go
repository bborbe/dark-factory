// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package healthcheck provides the local-probe implementations used by the
// `dark-factory healthcheck` subcommand. Each Probe covers one category of
// the pipeline-execution stack (Docker daemon, container image presence,
// container boot, workspace mount writability, Claude session, gh auth,
// notification channels). The seven probes run in fixed order
// (docker → image → boot → claude → mount → gh → notifications) and fail
// fast on the first error.
//
// Boot / mount / claude all share executor.BuildDockerRunArgs so the probes
// exercise the same launch path that production prompt containers and spec
// generation containers use. A regression in that path is caught by the
// healthcheck immediately, by construction.
package healthcheck

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/executor"
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

const (
	claudeWarnAfter = 2 * time.Second
	claudeTimeout   = 10 * time.Second
	ghWarnAfter     = 3 * time.Second
	ghTimeout       = 5 * time.Second
)

// claudeProbePrompt is the hard-coded prompt sent to `claude -p` to verify
// the in-container Claude session. It is a constant (never interpolated) so
// no operator input can reach the Claude shell.
const claudeProbePrompt = "reply with exactly: OK"

// ClaudeWarnAfterForFactory returns the warn-after threshold the factory
// should use when constructing a subproc.Runner for the Claude probe.
func ClaudeWarnAfterForFactory() time.Duration { return claudeWarnAfter }

// ClaudeTimeoutForFactory returns the hard timeout the factory should use
// when constructing a subproc.Runner for the Claude probe.
func ClaudeTimeoutForFactory() time.Duration { return claudeTimeout }

// GhWarnAfterForFactory returns the warn-after threshold the factory should
// use when constructing a subproc.Runner for the gh probe.
func GhWarnAfterForFactory() time.Duration { return ghWarnAfter }

// GhTimeoutForFactory returns the hard timeout the factory should use when
// constructing a subproc.Runner for the gh probe.
func GhTimeoutForFactory() time.Duration { return ghTimeout }

// ProbeLaunchConfig is the per-invocation container-launch config that the
// boot / mount / claude probes pass to executor.BuildDockerRunArgs. The
// factory builds one instance and shares it across all three probes so they
// hit the exact same launch path production uses.
type ProbeLaunchConfig struct {
	ContainerImage string
	ProjectName    string
	ProjectRoot    string
	ClaudeDir      string
	Home           string
	Env            map[string]string
	ExtraMounts    []config.ExtraMount
	NetrcFile      string
	GitconfigFile  string
	HideGit        bool
}

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
	out, err := d.runner.RunWithWarnAndTimeout(ctx, "docker version", "docker", "version")
	if err != nil {
		slog.Error("healthcheck probe failed", "probe", d.Name(), "error", err)
		if isDockerDaemonUnreachable(out) {
			return errors.Wrapf(ctx, err, "docker daemon unreachable")
		}
		return errors.Wrapf(ctx, err, "docker version failed")
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
		"docker image inspect",
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
		return errors.Errorf(ctx, "container image %q not present locally", i.containerImage)
	}
	slog.Info("healthcheck probe passed", "probe", i.Name(), "image", i.containerImage)
	return nil
}

// bootProbe runs a one-shot container via the same launch path production
// prompt containers use and verifies /workspace is writable inside it. The
// probe assembles a ContainerLaunchOpts from ProbeLaunchConfig + a
// boot-specific entrypoint (`/bin/sh`) + boot-specific command and calls
// executor.BuildDockerRunArgs to produce the argv.
type bootProbe struct {
	launch ProbeLaunchConfig
	runner subproc.Runner
}

// bootProbeCommand is the in-container shell command that verifies /workspace
// is writable and prints the BOOT_OK marker on success.
const bootProbeCommand = "mkdir -p /workspace/.dark-factory-healthcheck && " +
	"touch /workspace/.dark-factory-healthcheck/probe && " +
	"rm -rf /workspace/.dark-factory-healthcheck && " +
	"echo BOOT_OK"

// NewBootProbe returns a Probe that boots a throwaway container via the
// shared executor.BuildDockerRunArgs launch path and verifies /workspace
// is writable. The probe uses the same mounts, env, hideGit and extraMounts
// wiring as a production prompt container — any regression in the launch
// path surfaces here.
func NewBootProbe(launch ProbeLaunchConfig, r subproc.Runner) Probe {
	return &bootProbe{launch: launch, runner: r}
}

func (b *bootProbe) Name() string {
	return "boot"
}

func (b *bootProbe) Run(ctx context.Context) error {
	return runContainerProbe(ctx, runContainerProbeArgs{
		launch:        b.launch,
		runner:        b.runner,
		category:      b.Name(),
		op:            "docker run boot probe",
		entrypoint:    "/bin/sh",
		command:       []string{"-c", bootProbeCommand},
		successMarker: "BOOT_OK",
		failurePrefix: "container boot probe failed",
	})
}

// claudeProbe runs `claude -p <prompt>` inside a throwaway container via the
// shared executor.BuildDockerRunArgs launch path and asserts the literal "OK"
// substring is present in stdout. Same launch path as production — so if
// production stops being able to start Claude, the probe notices.
type claudeProbe struct {
	launch ProbeLaunchConfig
	runner subproc.Runner
}

// NewClaudeProbe returns a Probe that boots a one-shot container via the
// shared launch path with `claude -p <prompt>` as the entrypoint+args.
// Asserts the literal "OK" substring in stdout.
func NewClaudeProbe(launch ProbeLaunchConfig, r subproc.Runner) Probe {
	return &claudeProbe{launch: launch, runner: r}
}

func (c *claudeProbe) Name() string {
	return "claude"
}

func (c *claudeProbe) Run(ctx context.Context) error {
	return runContainerProbe(ctx, runContainerProbeArgs{
		launch:        c.launch,
		runner:        c.runner,
		category:      c.Name(),
		op:            "claude session probe",
		entrypoint:    "claude",
		command:       []string{"-p", claudeProbePrompt},
		successMarker: "OK",
		failurePrefix: "claude session probe failed",
	})
}

// mountProbeCommand is the in-container shell command that verifies /workspace
// is writable. The mkdir/touch/rm sequence is the actual write test; the
// MOUNT_OK echo is the success marker.
const mountProbeCommand = "mkdir -p /workspace/.healthcheck-mount && " +
	"touch /workspace/.healthcheck-mount/p && " +
	"rm -rf /workspace/.healthcheck-mount && " +
	"echo MOUNT_OK"

// mountProbe boots a throwaway container via the shared launch path and
// verifies /workspace is writable from inside.
type mountProbe struct {
	launch ProbeLaunchConfig
	runner subproc.Runner
}

// NewMountProbe returns a Probe that boots a throwaway container via the
// shared launch path and verifies /workspace is writable inside it.
func NewMountProbe(launch ProbeLaunchConfig, r subproc.Runner) Probe {
	return &mountProbe{launch: launch, runner: r}
}

func (m *mountProbe) Name() string {
	return "mount"
}

func (m *mountProbe) Run(ctx context.Context) error {
	return runContainerProbe(ctx, runContainerProbeArgs{
		launch:        m.launch,
		runner:        m.runner,
		category:      m.Name(),
		op:            "docker run mount probe",
		entrypoint:    "/bin/sh",
		command:       []string{"-c", mountProbeCommand},
		successMarker: "MOUNT_OK",
		failurePrefix: "workspace mount not writable",
	})
}

// runContainerProbeArgs is the input to runContainerProbe — all the fields the three
// container probes (boot, mount, claude) need to launch one one-shot container, check
// stdout for a success marker, and report success/failure with consistent logging.
type runContainerProbeArgs struct {
	launch        ProbeLaunchConfig
	runner        subproc.Runner
	category      string   // probe name ("boot", "mount", "claude")
	op            string   // subproc op label
	entrypoint    string   // container --entrypoint
	command       []string // args appended after the image
	successMarker string   // substring required in stdout for success
	failurePrefix string   // error message prefix on failure
}

// runContainerProbe is the shared body of bootProbe.Run, mountProbe.Run, claudeProbe.Run.
// It launches one container via the same executor.BuildDockerRunArgs path production uses,
// asserts the success marker in stdout, and emits the standard slog success/failure lines.
func runContainerProbe(ctx context.Context, a runContainerProbeArgs) error {
	name, err := uniqueContainerName(a.launch.ProjectName, a.category)
	if err != nil {
		return errors.Wrap(ctx, err, "generate "+a.category+" probe container name")
	}
	opts := launchOpts(a.launch, name)
	opts.Entrypoint = a.entrypoint
	opts.Command = a.command
	// #nosec G204 -- args are derived from the configured container image, not user input
	out, err := a.runner.RunWithWarnAndTimeout(
		ctx,
		a.op,
		"docker",
		executor.BuildDockerRunArgs(opts)...)
	if err != nil {
		slog.Error(
			"healthcheck probe failed",
			"probe", a.category,
			"container", name,
			"stdout", truncate(string(out)),
			"error", err,
		)
		return errors.Errorf(ctx, "%s: stdout=%q", a.failurePrefix, truncate(string(out)))
	}
	if !strings.Contains(string(out), a.successMarker) {
		slog.Error(
			"healthcheck probe failed",
			"probe", a.category,
			"container", name,
			"stdout", truncate(string(out)),
		)
		return errors.Errorf(
			ctx,
			"%s: missing %s marker in stdout=%q",
			a.failurePrefix,
			a.successMarker,
			truncate(string(out)),
		)
	}
	slog.Info("healthcheck probe passed", "probe", a.category, "container", name)
	return nil
}

// launchOpts builds a base ContainerLaunchOpts from a ProbeLaunchConfig.
// Callers add probe-specific Entrypoint + Command before passing to
// executor.BuildDockerRunArgs. The probe-specific label
// (`dark-factory.probe=<category>`) is derived from the container name suffix
// pattern produced by uniqueContainerName.
func launchOpts(p ProbeLaunchConfig, containerName string) executor.ContainerLaunchOpts {
	return executor.ContainerLaunchOpts{
		ContainerName:  containerName,
		ContainerImage: p.ContainerImage,
		ProjectName:    p.ProjectName,
		ProjectRoot:    p.ProjectRoot,
		ClaudeDir:      p.ClaudeDir,
		Home:           p.Home,
		Env:            p.Env,
		ExtraMounts:    p.ExtraMounts,
		NetrcFile:      p.NetrcFile,
		GitconfigFile:  p.GitconfigFile,
		HideGit:        p.HideGit,
	}
}

// ghProbe runs `gh auth status` and reports success only when exit is 0.
// The gh CLI writes its diagnostic output to stdout, so the probe surfaces
// the captured stdout in the failure message.
type ghProbe struct {
	runner subproc.Runner
}

// NewGhProbe returns a Probe that runs `gh auth status`. The probe is
// unconditional from its own perspective; the config-gating (pr: true) is
// enforced by the orchestrator.
func NewGhProbe(r subproc.Runner) Probe {
	return &ghProbe{runner: r}
}

func (g *ghProbe) Name() string {
	return "gh"
}

func (g *ghProbe) Run(ctx context.Context) error {
	out, err := g.runner.RunWithWarnAndTimeout(ctx, "gh auth status", "gh", "auth", "status")
	if err != nil {
		slog.Error(
			"healthcheck probe failed",
			"probe", g.Name(),
			"stdout", truncate(string(out)),
			"error", err,
		)
		return errors.Wrapf(ctx, err, "gh auth status failed: stdout=%q", truncate(string(out)))
	}
	slog.Info("healthcheck probe passed", "probe", g.Name())
	return nil
}

// notificationsProbe POSTs a fixed minimal JSON payload to the configured
// notification channel (Telegram or Discord) and asserts a 2xx response.
// The full URL is stored in memory at construction time but is NEVER logged
// — only the URL host is surfaced.
type notificationsProbe struct {
	channel notificationsChannel
	client  *http.Client
}

// notificationsChannel is the active channel resolved at construction time.
// Exactly one of telegramURL or discordURL is non-empty.
type notificationsChannel struct {
	kind        string // "telegram" or "discord"
	telegramURL string
	discordURL  string
	chatID      string // telegram only
}

// NotificationsConfigured reports whether at least one notification channel
// is fully configured (env var name present AND env var non-empty). Returns
// true for Telegram when BotTokenEnv is set and resolves to a non-empty
// token, and for Discord when WebhookEnv is set and resolves to a non-empty
// webhook URL.
func NotificationsConfigured(cfg config.Config) bool {
	if cfg.Notifications.Telegram.BotTokenEnv != "" && cfg.ResolvedTelegramBotToken() != "" {
		return true
	}
	if cfg.Notifications.Discord.WebhookEnv != "" && cfg.ResolvedDiscordWebhook() != "" {
		return true
	}
	return false
}

// NewNotificationsProbe returns a Probe that POSTs a fixed minimal JSON
// payload to the configured notification channel via the injected
// *http.Client. The factory supplies the client (5-second timeout baked
// in). When no channel is configured the probe returns an error on Run
// (the factory normally omits this probe entirely via NotificationsConfigured,
// so reaching this state indicates a wiring mistake).
func NewNotificationsProbe(cfg config.Config, client *http.Client) Probe {
	p := &notificationsProbe{client: client}
	if cfg.Notifications.Telegram.BotTokenEnv != "" && cfg.ResolvedTelegramBotToken() != "" {
		p.channel = notificationsChannel{
			kind:        "telegram",
			telegramURL: "https://api.telegram.org/bot" + cfg.ResolvedTelegramBotToken() + "/sendMessage",
			chatID:      cfg.ResolvedTelegramChatID(),
		}
		return p
	}
	if cfg.Notifications.Discord.WebhookEnv != "" && cfg.ResolvedDiscordWebhook() != "" {
		p.channel = notificationsChannel{
			kind:       "discord",
			discordURL: cfg.ResolvedDiscordWebhook(),
		}
		return p
	}
	return p
}

func (n *notificationsProbe) Name() string {
	return "notifications"
}

func (n *notificationsProbe) Run(ctx context.Context) error {
	if n.channel.kind == "" {
		return errors.Errorf(ctx, "notifications probe: no channel configured")
	}

	body, urlStr, err := n.buildPayload(ctx)
	if err != nil {
		return errors.Wrap(ctx, err, "build notifications payload")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlStr, strings.NewReader(body))
	if err != nil {
		return errors.Wrap(ctx, err, "create notifications request")
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		slog.Error(
			"healthcheck probe failed",
			"probe", n.Name(),
			"channel", n.channel.kind,
			"error", err,
		)
		return errors.Wrap(ctx, err, "notifications POST transport error")
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodySnippet, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		host := urlHost(urlStr)
		slog.Error(
			"healthcheck probe failed",
			"probe", n.Name(),
			"channel", n.channel.kind,
			"host", host,
			"status", resp.StatusCode,
		)
		return errors.Errorf(
			ctx,
			"notifications POST failed: status=%d body=%q",
			resp.StatusCode,
			truncate(string(bodySnippet)),
		)
	}

	host := urlHost(urlStr)
	slog.Info(
		"healthcheck probe passed",
		"probe", n.Name(),
		"channel", n.channel.kind,
		"host", host,
		"status", resp.StatusCode,
	)
	return nil
}

// buildPayload returns the JSON body and target URL for the active channel.
// The URL path (which contains the Telegram bot token) is intentionally
// kept out of the returned string for any log line.
func (n *notificationsProbe) buildPayload(ctx context.Context) (string, string, error) {
	switch n.channel.kind {
	case "telegram":
		payload := map[string]string{
			"chat_id": n.channel.chatID,
			"text":    "dark-factory healthcheck",
		}
		body, err := json.Marshal(payload)
		if err != nil {
			return "", "", errors.Wrap(ctx, err, "marshal telegram payload")
		}
		return string(body), n.channel.telegramURL, nil
	case "discord":
		payload := map[string]string{
			"content": "dark-factory healthcheck",
		}
		body, err := json.Marshal(payload)
		if err != nil {
			return "", "", errors.Wrap(ctx, err, "marshal discord payload")
		}
		return string(body), n.channel.discordURL, nil
	default:
		return "", "", errors.Errorf(ctx, "notifications: no active channel")
	}
}

// urlHost returns the host portion of a URL string. The full URL (which
// may contain the Telegram bot token in the path) is never logged — only
// the host is returned.
func urlHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Host
}

// uniqueContainerName returns "<projectName>-healthcheck-<category>-<16 hex chars>".
// The 8-byte random suffix is from crypto/rand; collision probability is
// negligible even with concurrent invocations, and the prefix guarantees
// no collision with prompt containers (-exec-) or spec generation
// containers (-gen-).
func uniqueContainerName(projectName, category string) (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return projectName + "-healthcheck-" + category + "-" + hex.EncodeToString(buf[:]), nil
}

// truncate shortens s to at most 200 bytes, appending "..." if truncation happened.
func truncate(s string) string {
	const n = 200
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
