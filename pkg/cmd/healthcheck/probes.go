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
	mountTimeout    = 15 * time.Second

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
// Exposed so the factory can pass the same value the probe expects.
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

// claudeProbe runs a one-shot `docker run --rm <image> claude -p <prompt>` and
// asserts the literal "OK" substring is present in stdout. The hard-coded
// prompt guarantees no operator input reaches the Claude shell.
type claudeProbe struct {
	containerImage string
	projectName    string
	runner         subproc.Runner
}

// NewClaudeProbe returns a Probe that boots a one-shot container, runs
// `claude -p` against it, and asserts the literal "OK" substring is present
// in stdout. A 10-second hard timeout caps the wall-clock impact even when
// Claude is slow to respond.
func NewClaudeProbe(containerImage string, projectName string, r subproc.Runner) Probe {
	return &claudeProbe{
		containerImage: containerImage,
		projectName:    projectName,
		runner:         r,
	}
}

func (c *claudeProbe) Name() string {
	return "claude"
}

func (c *claudeProbe) Run(ctx context.Context) error {
	name, err := uniqueContainerName(c.projectName, "claude")
	if err != nil {
		return errors.Wrap(ctx, err, "generate claude probe container name")
	}
	out, err := c.runner.RunWithWarnAndTimeout(
		ctx,
		"claude session probe",
		"docker",
		"run",
		"--rm",
		"--name", name,
		c.containerImage,
		"claude",
		"-p", claudeProbePrompt,
	)
	if err != nil {
		slog.Error(
			"healthcheck probe failed",
			"probe",
			c.Name(),
			"stdout",
			truncate(string(out)),
			"error",
			err,
		)
		return errors.Errorf(
			ctx,
			"claude session probe failed: stdout=%q",
			truncate(string(out)),
		)
	}
	if !strings.Contains(string(out), "OK") {
		slog.Error(
			"healthcheck probe failed",
			"probe",
			c.Name(),
			"stdout",
			truncate(string(out)),
		)
		return errors.Errorf(
			ctx,
			"claude session probe failed: stdout=%q",
			truncate(string(out)),
		)
	}
	slog.Info("healthcheck probe passed", "probe", c.Name())
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
	out, err := g.runner.RunWithWarnAndTimeout(
		ctx,
		"gh auth status",
		"gh",
		"auth",
		"status",
	)
	if err != nil {
		slog.Error(
			"healthcheck probe failed",
			"probe",
			g.Name(),
			"stdout",
			truncate(string(out)),
			"error",
			err,
		)
		return errors.Wrapf(
			ctx,
			err,
			"gh auth status failed: stdout=%q",
			truncate(string(out)),
		)
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

// NotificationsConfigured reports whether the operator has opted into at
// least one notification channel. The Telegram channel is "configured" when
// its env var name is set; "operational" requires the env var to be
// non-empty. The probe handles both cases by no-oping when the URL is
// empty.
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
// in) — tests inject a test double via a fake RoundTripper.
//
// When neither channel is configured the constructor still returns a Probe
// whose Run no-ops with a Debug log line. The orchestrator normally omits
// this probe from the iteration list via NotificationsConfigured; the
// in-probe no-op is a defensive safety net.
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
			"probe",
			n.Name(),
			"channel",
			n.channel.kind,
			"error",
			err,
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
			"probe",
			n.Name(),
			"channel",
			n.channel.kind,
			"host",
			host,
			"status",
			resp.StatusCode,
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
		"probe",
		n.Name(),
		"channel",
		n.channel.kind,
		"host",
		host,
		"status",
		resp.StatusCode,
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
