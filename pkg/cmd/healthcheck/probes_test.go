// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package healthcheck_test

import (
	"context"
	"io"
	"net/http"
	"os/exec"
	"strings"

	"github.com/bborbe/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/cmd/healthcheck"
	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/launchpolicy"
	"github.com/bborbe/dark-factory/pkg/subproc"
)

var _ = Describe("DockerProbe", func() {
	var (
		ctx      context.Context
		subprocR *mocks.SubprocRunner
	)

	BeforeEach(func() {
		ctx = context.Background()
		subprocR = &mocks.SubprocRunner{}
	})

	It("returns nil when docker version succeeds", func() {
		subprocR.RunWithWarnAndTimeoutReturns([]byte(""), nil)
		p := healthcheck.NewDockerProbe(subprocR)
		Expect(p.Name()).To(Equal("docker"))
		Expect(p.Run(ctx)).To(Succeed())
		Expect(subprocR.RunWithWarnAndTimeoutCallCount()).To(Equal(1))
	})

	It("wraps error with docker daemon unreachable on non-zero exit", func() {
		subprocR.RunWithWarnAndTimeoutReturns(
			[]byte("Cannot connect to the Docker daemon at unix:///var/run/docker.sock"),
			errors.Errorf(ctx, "exit 1"),
		)
		p := healthcheck.NewDockerProbe(subprocR)
		err := p.Run(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("docker daemon unreachable"))
	})
})

var _ = Describe("ImageProbe", func() {
	var (
		ctx      context.Context
		subprocR *mocks.SubprocRunner
	)

	BeforeEach(func() {
		ctx = context.Background()
		subprocR = &mocks.SubprocRunner{}
	})

	It("returns nil when image inspect succeeds", func() {
		subprocR.RunWithWarnAndTimeoutReturns([]byte("sha256:abc"), nil)
		p := healthcheck.NewImageProbe("alpine:latest", subprocR)
		Expect(p.Name()).To(Equal("image"))
		Expect(p.Run(ctx)).To(Succeed())
		_, _, name, args := subprocR.RunWithWarnAndTimeoutArgsForCall(0)
		Expect(name).To(Equal("docker"))
		Expect(args).To(ContainElement("alpine:latest"))
	})

	It("returns image-not-present error on non-zero exit", func() {
		subprocR.RunWithWarnAndTimeoutReturns(nil, errors.Errorf(ctx, "exit 1"))
		p := healthcheck.NewImageProbe("missing:tag", subprocR)
		err := p.Run(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(`"missing:tag" not present locally`))
	})
})

var _ = Describe("BootProbe", func() {
	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	testPolicy := func() launchpolicy.Policy {
		return launchpolicy.NewPolicy(
			"alpine:latest",
			"test-proj",
			"/tmp",
			"/tmp",
			"/tmp",
			nil,
			nil,
			"",
			"",
			false,
		)
	}

	It("returns nil when stdout contains the BOOT_OK marker", func() {
		subprocR := &mocks.SubprocRunner{}
		subprocR.RunWithWarnAndTimeoutReturns([]byte("BOOT_OK\n"), nil)
		p := healthcheck.NewBootProbe(testPolicy(), subprocR)
		Expect(p.Name()).To(Equal("boot"))
		Expect(p.Run(ctx)).To(Succeed())
	})

	It("wraps error from underlying docker run failure", func() {
		subprocR := &mocks.SubprocRunner{}
		subprocR.RunWithWarnAndTimeoutReturns(nil, errors.Errorf(ctx, "docker run failed"))
		p := healthcheck.NewBootProbe(testPolicy(), subprocR)
		err := p.Run(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("container boot probe failed"))
	})
})

var _ = Describe("MountProbe", func() {
	var (
		ctx      context.Context
		subprocR *mocks.SubprocRunner
	)

	BeforeEach(func() {
		ctx = context.Background()
		subprocR = &mocks.SubprocRunner{}
	})

	testPolicy := func() launchpolicy.Policy {
		return launchpolicy.NewPolicy(
			"alpine:latest",
			"test-proj",
			"/tmp",
			"/tmp",
			"/tmp",
			nil,
			nil,
			"",
			"",
			false,
		)
	}

	It("returns nil when mount write succeeds", func() {
		subprocR.RunWithWarnAndTimeoutReturns([]byte("MOUNT_OK\n"), nil)
		p := healthcheck.NewMountProbe(testPolicy(), subprocR)
		Expect(p.Name()).To(Equal("mount"))
		Expect(p.Run(ctx)).To(Succeed())
		_, _, name, args := subprocR.RunWithWarnAndTimeoutArgsForCall(0)
		Expect(name).To(Equal("docker"))
		Expect(args).To(ContainElement("alpine:latest"))
	})

	It("returns mount-not-writable error when exit non-zero", func() {
		subprocR.RunWithWarnAndTimeoutReturns(nil, errors.Errorf(ctx, "exit 1"))
		p := healthcheck.NewMountProbe(testPolicy(), subprocR)
		err := p.Run(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("workspace mount not writable"))
	})

	It("returns mount-not-writable error when stdout missing MOUNT_OK", func() {
		subprocR.RunWithWarnAndTimeoutReturns([]byte("partial"), nil)
		p := healthcheck.NewMountProbe(testPolicy(), subprocR)
		err := p.Run(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("workspace mount not writable"))
	})
})

var _ = Describe("ClaudeProbe", func() {
	var (
		ctx      context.Context
		subprocR *mocks.SubprocRunner
	)

	BeforeEach(func() {
		ctx = context.Background()
		subprocR = &mocks.SubprocRunner{}
	})

	testPolicy := func() launchpolicy.Policy {
		return launchpolicy.NewPolicy(
			"alpine:latest",
			"myproject",
			"/tmp",
			"/tmp",
			"/tmp",
			nil,
			nil,
			"",
			"",
			false,
		)
	}

	It("returns nil when stdout contains the OK marker", func() {
		subprocR.RunWithWarnAndTimeoutReturns([]byte("OK\n"), nil)
		p := healthcheck.NewClaudeProbe(testPolicy(), subprocR)
		Expect(p.Name()).To(Equal("claude"))
		Expect(p.Run(ctx)).To(Succeed())
		_, _, name, args := subprocR.RunWithWarnAndTimeoutArgsForCall(0)
		Expect(name).To(Equal("docker"))
		Expect(args).To(ContainElement("--entrypoint"))
		Expect(args).To(ContainElement("claude"))
		Expect(args).To(ContainElement("-p"))
		Expect(args).To(ContainElement("reply with exactly: OK"))
	})

	It("returns error when stdout does not contain OK", func() {
		subprocR.RunWithWarnAndTimeoutReturns([]byte("unexpected\n"), nil)
		p := healthcheck.NewClaudeProbe(testPolicy(), subprocR)
		err := p.Run(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("claude session probe failed"))
	})

	It("returns error on non-zero exit", func() {
		subprocR.RunWithWarnAndTimeoutReturns(
			[]byte(""),
			errors.Errorf(ctx, "exit 1"),
		)
		p := healthcheck.NewClaudeProbe(testPolicy(), subprocR)
		err := p.Run(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("claude session probe failed"))
	})

	It("surfaces wrapped runner error in error message on non-zero exit", func() {
		// Regression: previously the runContainerProbe failure path dropped the
		// runner err entirely and returned only stdout — operator saw
		// `stdout=""` with no clue why. Now the underlying err must be in the
		// returned string so the cause is visible without checking logs.
		subprocR.RunWithWarnAndTimeoutReturns(
			[]byte(""),
			errors.Errorf(ctx, "claude session probe failed: exit 1"),
		)
		p := healthcheck.NewClaudeProbe(testPolicy(), subprocR)
		err := p.Run(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("err="))
		Expect(err.Error()).To(ContainSubstring("exit 1"))
	})

	It("extracts stderr from a real *exec.ExitError chain wrapped via bborbe/errors", func() {
		// End-to-end check that errors.As walks through subproc's
		// errors.Wrapf wrapping to find *exec.ExitError.Stderr.
		// Mirrors the actual production path: subproc.Runner.cmd.Output()
		// returns *exec.ExitError when the subprocess exits non-zero,
		// wraps it via errors.Wrapf, and the probe must still surface
		// the captured stderr.
		realCmd := exec.Command("sh", "-c", "echo MARKER-STDERR >&2; exit 7")
		_, runErr := realCmd.Output() // captures stderr into *exec.ExitError.Stderr
		Expect(runErr).To(HaveOccurred())
		wrapped := errors.Wrap(ctx, runErr, "subprocess simulation")

		subprocR.RunWithWarnAndTimeoutReturns([]byte(""), wrapped)
		p := healthcheck.NewClaudeProbe(testPolicy(), subprocR)
		err := p.Run(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("MARKER-STDERR"))
	})
})

var _ = Describe("GhProbe", func() {
	var (
		ctx      context.Context
		subprocR *mocks.SubprocRunner
	)

	BeforeEach(func() {
		ctx = context.Background()
		subprocR = &mocks.SubprocRunner{}
	})

	It("returns nil when gh auth status succeeds", func() {
		subprocR.RunWithWarnAndTimeoutReturns([]byte(""), nil)
		p := healthcheck.NewGhProbe(subprocR)
		Expect(p.Name()).To(Equal("gh"))
		Expect(p.Run(ctx)).To(Succeed())
		_, _, name, args := subprocR.RunWithWarnAndTimeoutArgsForCall(0)
		Expect(name).To(Equal("gh"))
		Expect(args).To(ConsistOf("auth", "status"))
	})

	It("returns wrapped error on non-zero exit", func() {
		subprocR.RunWithWarnAndTimeoutReturns(
			[]byte("You are not logged into any GitHub hosts"),
			errors.Errorf(ctx, "exit 1"),
		)
		p := healthcheck.NewGhProbe(subprocR)
		err := p.Run(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("gh auth status failed"))
	})
})

// roundTripFunc is a test double that lets each test inject its own RoundTrip behavior.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

var _ = Describe("NotificationsProbe", func() {
	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("returns error without HTTP call when no channel configured", func() {
		var invoked bool
		client := &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				invoked = true
				return nil, nil
			}),
		}
		cfg := config.Config{} // zero Notifications block
		p := healthcheck.NewNotificationsProbe(cfg, client)
		Expect(p.Name()).To(Equal("notifications"))
		err := p.Run(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no channel configured"))
		Expect(invoked).To(BeFalse())
	})

	It("returns nil on 2xx response via injected RoundTripper", func() {
		client := &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				Expect(r.Method).To(Equal(http.MethodPost))
				Expect(r.Header.Get("Content-Type")).To(Equal("application/json"))
				body, _ := io.ReadAll(r.Body)
				Expect(string(body)).To(ContainSubstring("dark-factory healthcheck"))
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("")),
				}, nil
			}),
		}
		cfg := config.Config{
			Notifications: config.NotificationsConfig{
				Discord: config.DiscordConfig{
					WebhookEnv: "DUMMY_WEBHOOK_ENV",
				},
			},
		}
		// Pre-set the resolved env value (the constructor reads it).
		GinkgoT().Setenv("DUMMY_WEBHOOK_ENV", "https://discord.example.com/webhook")
		p := healthcheck.NewNotificationsProbe(cfg, client)
		Expect(p.Run(ctx)).To(Succeed())
	})

	It("returns error with status and body snippet on non-2xx", func() {
		client := &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusUnauthorized,
					Body:       io.NopCloser(strings.NewReader("invalid token")),
				}, nil
			}),
		}
		cfg := config.Config{
			Notifications: config.NotificationsConfig{
				Discord: config.DiscordConfig{
					WebhookEnv: "DUMMY_WEBHOOK_ENV_401",
				},
			},
		}
		GinkgoT().Setenv("DUMMY_WEBHOOK_ENV_401", "https://discord.example.com/webhook")
		p := healthcheck.NewNotificationsProbe(cfg, client)
		err := p.Run(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("status=401"))
		Expect(err.Error()).To(ContainSubstring("invalid token"))
	})

	It("returns transport error when client.Do fails", func() {
		client := &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return nil, errors.Errorf(ctx, "connection refused")
			}),
		}
		cfg := config.Config{
			Notifications: config.NotificationsConfig{
				Discord: config.DiscordConfig{
					WebhookEnv: "DUMMY_WEBHOOK_ENV_TE",
				},
			},
		}
		GinkgoT().Setenv("DUMMY_WEBHOOK_ENV_TE", "https://discord.example.com/webhook")
		p := healthcheck.NewNotificationsProbe(cfg, client)
		err := p.Run(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("notifications POST transport error"))
	})

	It("POSTs the telegram payload and returns nil on 2xx", func() {
		client := &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				Expect(r.Method).To(Equal(http.MethodPost))
				Expect(r.Header.Get("Content-Type")).To(Equal("application/json"))
				Expect(r.URL.Host).To(Equal("api.telegram.org"))
				body, _ := io.ReadAll(r.Body)
				Expect(string(body)).To(ContainSubstring(`"chat_id":"42"`))
				Expect(string(body)).To(ContainSubstring("dark-factory healthcheck"))
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("")),
				}, nil
			}),
		}
		cfg := config.Config{
			Notifications: config.NotificationsConfig{
				Telegram: config.TelegramConfig{
					BotTokenEnv: "DUMMY_TELEGRAM_BOT",
					ChatIDEnv:   "DUMMY_TELEGRAM_CHAT",
				},
			},
		}
		GinkgoT().Setenv("DUMMY_TELEGRAM_BOT", "test-token")
		GinkgoT().Setenv("DUMMY_TELEGRAM_CHAT", "42")
		p := healthcheck.NewNotificationsProbe(cfg, client)
		Expect(p.Run(ctx)).To(Succeed())
	})
})

var _ = Describe("probe argv carries the canonical caps", func() {
	var ctx context.Context
	BeforeEach(func() {
		ctx = context.Background()
	})

	testPolicy := func() launchpolicy.Policy {
		return launchpolicy.NewPolicy(
			"alpine:latest", "test-proj", "/tmp", "/tmp", "/tmp",
			nil, nil, "", "", false,
		)
	}

	DescribeTable(
		"the assembled argv contains both NET_ADMIN and NET_RAW",
		func(newProbe func(launchpolicy.Policy, subproc.Runner) healthcheck.Probe, successMarker string) {
			subprocR := &mocks.SubprocRunner{}
			subprocR.RunWithWarnAndTimeoutReturns([]byte(successMarker+"\n"), nil)
			p := newProbe(testPolicy(), subprocR)
			Expect(p.Run(ctx)).To(Succeed())
			_, _, name, args := subprocR.RunWithWarnAndTimeoutArgsForCall(0)
			Expect(name).To(Equal("docker"))
			Expect(args).To(ContainElement("--cap-add=NET_ADMIN"))
			Expect(args).To(ContainElement("--cap-add=NET_RAW"))
		},
		Entry("boot probe", healthcheck.NewBootProbe, "BOOT_OK"),
		Entry("mount probe", healthcheck.NewMountProbe, "MOUNT_OK"),
		Entry("claude probe", healthcheck.NewClaudeProbe, "OK"),
	)
})

var _ = Describe("NotificationsConfigured", func() {
	It("returns false when nothing is configured", func() {
		Expect(healthcheck.NotificationsConfigured(config.Config{})).To(BeFalse())
	})

	It("returns true when telegram env var is set and resolved", func() {
		GinkgoT().Setenv("DUMMY_BOT_TOKEN_NOTIF", "resolved-token")
		cfg := config.Config{
			Notifications: config.NotificationsConfig{
				Telegram: config.TelegramConfig{
					BotTokenEnv: "DUMMY_BOT_TOKEN_NOTIF",
				},
			},
		}
		Expect(healthcheck.NotificationsConfigured(cfg)).To(BeTrue())
	})

	It("returns true when discord env var is set and resolved", func() {
		GinkgoT().Setenv("DUMMY_DISCORD_NOTIF", "https://discord.example.com/wh")
		cfg := config.Config{
			Notifications: config.NotificationsConfig{
				Discord: config.DiscordConfig{
					WebhookEnv: "DUMMY_DISCORD_NOTIF",
				},
			},
		}
		Expect(healthcheck.NotificationsConfigured(cfg)).To(BeTrue())
	})

	It("returns false when telegram env var name is set but value is empty", func() {
		GinkgoT().Setenv("DUMMY_BOT_TOKEN_EMPTY", "")
		cfg := config.Config{
			Notifications: config.NotificationsConfig{
				Telegram: config.TelegramConfig{
					BotTokenEnv: "DUMMY_BOT_TOKEN_EMPTY",
				},
			},
		}
		Expect(healthcheck.NotificationsConfigured(cfg)).To(BeFalse())
	})
})
