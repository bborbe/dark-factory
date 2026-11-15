// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package executor_test

import (
	"context"
	"regexp"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/executor"
	"github.com/bborbe/dark-factory/pkg/launchpolicy"
)

// argvContains returns true if args contains the (flag, value) pair in adjacent
// positions OR contains the single "flag=value" form. Used so tests don't need
// to know whether docker prefers space-separated or =-joined flag syntax.
func argvContains(args []string, flag, value string) bool {
	joined := flag + "=" + value
	for i, a := range args {
		if a == joined {
			return true
		}
		if a == flag && i+1 < len(args) && args[i+1] == value {
			return true
		}
	}
	return false
}

// argvHasFlag returns true if args contains the named flag at all.
func argvHasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag || strings.HasPrefix(a, flag+"=") {
			return true
		}
	}
	return false
}

var _ = Describe("BuildDockerRunArgs --add-host", func() {
	baseOpts := func() launchpolicy.ContainerLaunchOpts {
		return launchpolicy.ContainerLaunchOpts{
			ContainerName:  "df-test",
			ContainerImage: "busybox:latest",
			ProjectName:    "test-project",
		}
	}

	It("includes --add-host=host.docker.internal:host-gateway in the docker run argv", func() {
		args := executor.BuildDockerRunArgs(baseOpts())
		Expect(args).To(ContainElement("--add-host=host.docker.internal:host-gateway"))
	})
})

// The marker specs below cover the LEGACY `-e` path only (no EnvFile). Production
// never takes it: every launch goes through launchpolicy.PrepareEnvFile, which
// bakes the marker into the env-file — that path is covered by envfile_test.go
// ("emits the managed marker even when opts.Env is empty").
var _ = Describe("BuildDockerRunArgs DARK_FACTORY_MANAGED marker (legacy -e path)", func() {
	baseOpts := func() launchpolicy.ContainerLaunchOpts {
		return launchpolicy.ContainerLaunchOpts{
			ContainerName:  "df-test",
			ContainerImage: "claude-yolo:latest",
			ProjectName:    "test-project",
		}
	}

	It("emits -e DARK_FACTORY_MANAGED=true when no EnvFile is set", func() {
		args := executor.BuildDockerRunArgs(baseOpts())
		Expect(argvContains(args, "-e", "DARK_FACTORY_MANAGED=true")).To(BeTrue())
	})

	It("emits the marker even when opts.Env is nil (no opt-in field required)", func() {
		opts := baseOpts()
		opts.Env = nil
		args := executor.BuildDockerRunArgs(opts)
		Expect(argvContains(args, "-e", "DARK_FACTORY_MANAGED=true")).To(BeTrue())
	})

	It("marker takes precedence over an inbound opts.Env value for the same key", func() {
		opts := baseOpts()
		opts.Env = map[string]string{"DARK_FACTORY_MANAGED": "false"}
		args := executor.BuildDockerRunArgs(opts)
		Expect(argvContains(args, "-e", "DARK_FACTORY_MANAGED=true")).To(BeTrue())
		Expect(argvContains(args, "-e", "DARK_FACTORY_MANAGED=false")).To(BeFalse())
	})
})

var _ = Describe("BuildDockerRunArgs security hardening (ADR-0001 Phase 1)", func() {
	baseOpts := func() launchpolicy.ContainerLaunchOpts {
		return launchpolicy.ContainerLaunchOpts{
			ContainerName:  "test-container",
			ContainerImage: "claude-yolo:latest",
			ProjectName:    "test-project",
		}
	}

	Describe("--user", func() {
		It("is omitted when RunAsUser is empty (default — no behavior change)", func() {
			args := executor.BuildDockerRunArgs(baseOpts())
			Expect(argvHasFlag(args, "--user")).To(BeFalse())
		})

		It("emits --user <value> when RunAsUser is set", func() {
			opts := baseOpts()
			opts.RunAsUser = "1000:1000"
			args := executor.BuildDockerRunArgs(opts)
			Expect(argvContains(args, "--user", "1000:1000")).To(BeTrue())
		})

		It("emits bare-UID RunAsUser (docker accepts UID without GID)", func() {
			opts := baseOpts()
			opts.RunAsUser = "1000"
			args := executor.BuildDockerRunArgs(opts)
			Expect(argvContains(args, "--user", "1000")).To(BeTrue())
		})
	})

	Describe("resource limits", func() {
		It("omits all resource flags by default", func() {
			args := executor.BuildDockerRunArgs(baseOpts())
			Expect(argvHasFlag(args, "--memory")).To(BeFalse())
			Expect(argvHasFlag(args, "--cpus")).To(BeFalse())
			Expect(argvHasFlag(args, "--pids-limit")).To(BeFalse())
		})

		It("emits --memory, --cpus, --pids-limit when set", func() {
			opts := baseOpts()
			opts.MemoryLimit = "8g"
			opts.CPULimit = "4"
			opts.PIDsLimit = 1024
			args := executor.BuildDockerRunArgs(opts)
			Expect(argvContains(args, "--memory", "8g")).To(BeTrue())
			Expect(argvContains(args, "--cpus", "4")).To(BeTrue())
			Expect(argvContains(args, "--pids-limit", "1024")).To(BeTrue())
		})

		It("treats PIDsLimit == 0 as unset (no --pids-limit emitted)", func() {
			opts := baseOpts()
			opts.PIDsLimit = 0
			args := executor.BuildDockerRunArgs(opts)
			Expect(argvHasFlag(args, "--pids-limit")).To(BeFalse())
		})

		It("treats PIDsLimit < 0 as unset (negative values silently dropped per ADR doc)", func() {
			opts := baseOpts()
			opts.PIDsLimit = -1
			args := executor.BuildDockerRunArgs(opts)
			Expect(argvHasFlag(args, "--pids-limit")).To(BeFalse())
		})
	})

	Describe("claudeDir mount mode", func() {
		It("mounts claudeDir read-write by default (no :ro suffix)", func() {
			opts := baseOpts()
			opts.ClaudeDir = "/host/.claude"
			args := executor.BuildDockerRunArgs(opts)
			// Find the mount arg
			var found string
			for i, a := range args {
				if a == "-v" && i+1 < len(args) &&
					strings.HasPrefix(args[i+1], "/host/.claude:") {
					found = args[i+1]
					break
				}
			}
			Expect(found).To(Equal("/host/.claude:/home/node/.claude"))
		})

		It("mounts claudeDir read-only when ClaudeDirReadOnly is true", func() {
			opts := baseOpts()
			opts.ClaudeDir = "/host/.claude"
			opts.ClaudeDirReadOnly = true
			args := executor.BuildDockerRunArgs(opts)
			var found string
			for i, a := range args {
				if a == "-v" && i+1 < len(args) &&
					strings.HasPrefix(args[i+1], "/host/.claude:") {
					found = args[i+1]
					break
				}
			}
			Expect(found).To(Equal("/host/.claude:/home/node/.claude:ro"))
		})

		It(
			"emits no claudeDir mount when ClaudeDir is empty, even with ClaudeDirReadOnly=true",
			func() {
				opts := baseOpts()
				opts.ClaudeDir = ""
				opts.ClaudeDirReadOnly = true
				args := executor.BuildDockerRunArgs(opts)
				for i, a := range args {
					if a == "-v" && i+1 < len(args) &&
						strings.Contains(args[i+1], ":/home/node/.claude") {
						Fail("did not expect claudeDir mount when ClaudeDir is empty: " + args[i+1])
					}
				}
			},
		)
	})
})

// Secret-in-argv regression guard.
//
// A `docker run` argument list is world-readable: any local user or agent
// session can read a credential out of it with `ps -eo command`. The launch
// path therefore delivers the container env through a 0600 `--env-file` and
// emits no `-e` flag at all. These specs fail if that is undone — whether by
// restoring the `-e KEY=VALUE` emission or by emitting both forms.
var _ = Describe("BuildDockerRunArgs env delivery", func() {
	secretShaped := regexp.MustCompile(`(AUTH_TOKEN|API_KEY|SECRET|TOKEN|PASSWORD)=`)

	It("delivers env via --env-file and emits no -e flag", func() {
		opts := launchpolicy.ContainerLaunchOpts{
			ContainerName:  "df-test",
			ContainerImage: "busybox:latest",
			ProjectName:    "test-project",
			Env:            map[string]string{"ANTHROPIC_AUTH_TOKEN": "sentinel-token-value"},
		}
		cleanup, err := launchpolicy.PrepareEnvFile(context.Background(), &opts)
		Expect(err).To(BeNil())
		defer cleanup()

		args := executor.BuildDockerRunArgs(opts)

		Expect(argvHasFlag(args, "--env-file")).To(BeTrue())
		Expect(
			argvHasFlag(args, "-e"),
		).To(BeFalse(), "no -e flag may be emitted when --env-file is used")
	})

	It("keeps the credential out of argv even when Env and EnvFile are both set", func() {
		const sentinel = "sentinel-token-value"
		opts := launchpolicy.ContainerLaunchOpts{
			ContainerName:  "df-test",
			ContainerImage: "busybox:latest",
			ProjectName:    "test-project",
			Env:            map[string]string{"ANTHROPIC_AUTH_TOKEN": sentinel},
			EnvFile:        "/tmp/dark-factory-env-test/env",
		}

		args := executor.BuildDockerRunArgs(opts)

		for _, a := range args {
			Expect(a).NotTo(ContainSubstring(sentinel),
				"a credential value must never reach the docker argv")
		}
	})

	It("carries no secret-shaped KEY=VALUE element in the argv", func() {
		opts := launchpolicy.ContainerLaunchOpts{
			ContainerName:  "df-test",
			ContainerImage: "busybox:latest",
			ProjectName:    "test-project",
			Env: map[string]string{
				"ANTHROPIC_AUTH_TOKEN": "sentinel-token-value",
				"ANTHROPIC_BASE_URL":   "http://127.0.0.1:8788",
			},
		}
		cleanup, err := launchpolicy.PrepareEnvFile(context.Background(), &opts)
		Expect(err).To(BeNil())
		defer cleanup()

		for _, a := range executor.BuildDockerRunArgs(opts) {
			Expect(secretShaped.MatchString(a)).To(BeFalse(),
				"secret-shaped argv element found: %s", a)
		}
	})

	It("fails closed on the legacy -e path: a credential is dropped, not published", func() {
		opts := launchpolicy.ContainerLaunchOpts{
			ContainerName:  "df-test",
			ContainerImage: "busybox:latest",
			ProjectName:    "test-project",
			// No EnvFile — a call site that forgot PrepareEnvFile.
			Env: map[string]string{
				"ANTHROPIC_AUTH_TOKEN": "sentinel-token-value",
				"GOPRIVATE":            "example.com/*",
			},
		}

		args := executor.BuildDockerRunArgs(opts)

		for _, a := range args {
			Expect(a).NotTo(ContainSubstring("sentinel-token-value"),
				"a forgotten PrepareEnvFile must not publish the credential to argv")
			Expect(secretShaped.MatchString(a)).To(BeFalse(),
				"secret-shaped argv element found: %s", a)
		}
		// Non-credential keys keep working on the legacy path.
		Expect(argvContains(args, "-e", "GOPRIVATE=example.com/*")).To(BeTrue())
	})
})
