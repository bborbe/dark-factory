// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package executor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/bborbe/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/report"
)

// fakeCommandRunner is a test double for commandRunner.
type fakeCommandRunner struct {
	mu        sync.Mutex
	err       error
	runCalled bool
	commands  []*exec.Cmd
}

func (f *fakeCommandRunner) Run(ctx context.Context, cmd *exec.Cmd) error {
	f.mu.Lock()
	f.runCalled = true
	f.commands = append(f.commands, cmd)
	err := f.err
	f.mu.Unlock()
	return err
}

func (f *fakeCommandRunner) RunCalled() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.runCalled
}

func (f *fakeCommandRunner) Commands() []*exec.Cmd {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.commands
}

var _ = Describe("Internal helper functions", func() {
	var (
		ctx     context.Context
		tempDir string
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		tempDir, err = os.MkdirTemp("", "executor-internal-test-*")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	Describe("prepareLogFile", func() {
		Context("with valid log file path", func() {
			It("creates log directory and opens file", func() {
				logFile := filepath.Join(tempDir, "logs", "test.log")

				file, err := prepareLogFile(ctx, logFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(file).NotTo(BeNil())
				defer file.Close()

				// Verify file exists
				_, err = os.Stat(logFile)
				Expect(err).NotTo(HaveOccurred())

				// Verify directory exists
				_, err = os.Stat(filepath.Join(tempDir, "logs"))
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("with existing log file", func() {
			var logFile string

			BeforeEach(func() {
				logFile = filepath.Join(tempDir, "existing.log")
				err := os.WriteFile(logFile, []byte("old content"), 0600)
				Expect(err).NotTo(HaveOccurred())
			})

			It("truncates existing file", func() {
				file, err := prepareLogFile(ctx, logFile)
				Expect(err).NotTo(HaveOccurred())
				defer file.Close()

				_, err = file.WriteString("new content")
				Expect(err).NotTo(HaveOccurred())
				file.Close()

				content, err := os.ReadFile(logFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(content)).To(Equal("new content"))
			})
		})

		Context("with invalid path", func() {
			It("returns error", func() {
				logFile := "/invalid/path/that/does/not/exist/test.log"

				file, err := prepareLogFile(ctx, logFile)
				Expect(err).To(HaveOccurred())
				Expect(file).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("create log directory"))
			})
		})

		Context("with nested directories", func() {
			It("creates nested directories", func() {
				logFile := filepath.Join(tempDir, "level1", "level2", "level3", "test.log")

				file, err := prepareLogFile(ctx, logFile)
				Expect(err).NotTo(HaveOccurred())
				defer file.Close()

				_, err = os.Stat(logFile)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Describe("createPromptTempFile", func() {
		Context("with valid content", func() {
			It("creates temp file with content", func() {
				promptContent := "# Test Prompt\n\nThis is test content."

				filePath, cleanup, err := createPromptTempFile(ctx, promptContent)
				Expect(err).NotTo(HaveOccurred())
				Expect(filePath).NotTo(BeEmpty())
				defer cleanup()

				_, err = os.Stat(filePath)
				Expect(err).NotTo(HaveOccurred())

				content, err := os.ReadFile(filePath)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(content)).To(Equal(promptContent))

				cleanup()

				_, err = os.Stat(filePath)
				Expect(os.IsNotExist(err)).To(BeTrue())
			})
		})

		Context("with special characters", func() {
			It("handles special characters correctly", func() {
				promptContent := "# Test with `backticks`\n\nAnd \"quotes\" and $variables"

				filePath, cleanup, err := createPromptTempFile(ctx, promptContent)
				Expect(err).NotTo(HaveOccurred())
				defer cleanup()

				content, err := os.ReadFile(filePath)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(content)).To(Equal(promptContent))
			})
		})

		Context("with empty content", func() {
			It("creates empty file", func() {
				filePath, cleanup, err := createPromptTempFile(ctx, "")
				Expect(err).NotTo(HaveOccurred())
				defer cleanup()

				content, err := os.ReadFile(filePath)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(content)).To(Equal(""))
			})
		})

		Context("multiple calls", func() {
			It("creates unique files", func() {
				path1, cleanup1, err := createPromptTempFile(ctx, "# Prompt 1")
				Expect(err).NotTo(HaveOccurred())
				defer cleanup1()

				path2, cleanup2, err := createPromptTempFile(ctx, "# Prompt 2")
				Expect(err).NotTo(HaveOccurred())
				defer cleanup2()

				Expect(path1).NotTo(Equal(path2))
			})
		})
	})

	Describe("extractPromptBaseName", func() {
		It("extracts basename when prefix matches", func() {
			result := extractPromptBaseName("myproject-042-fix-bug", "myproject")
			Expect(result).To(Equal("042-fix-bug"))
		})

		It("returns full name when prefix does not match", func() {
			result := extractPromptBaseName("other-042-fix-bug", "myproject")
			Expect(result).To(Equal("other-042-fix-bug"))
		})

		It("returns containerName when it equals prefix with dash", func() {
			result := extractPromptBaseName("myproject-", "myproject")
			Expect(result).To(Equal("myproject-"))
		})

		It("returns full name when containerName is shorter than prefix", func() {
			result := extractPromptBaseName("my", "myproject")
			Expect(result).To(Equal("my"))
		})

		It("handles empty project name", func() {
			result := extractPromptBaseName("some-container", "")
			Expect(result).To(Equal("some-container"))
		})

		It("handles empty container name", func() {
			result := extractPromptBaseName("", "myproject")
			Expect(result).To(Equal(""))
		})
	})

	Describe("buildDockerCommand", func() {
		var exec *dockerExecutor

		BeforeEach(func() {
			exec = &dockerExecutor{
				containerImage: config.Defaults().ContainerImage,
				projectName:    "test-project",
			}
		})

		It("builds correct docker command", func() {
			cmd := exec.buildDockerCommand(
				ctx,
				"test-container",
				"/tmp/prompt.md",
				"/workspace",
				"/home/user/.claude",
				"test-prompt",
				"/home/user",
			)

			Expect(cmd).NotTo(BeNil())
			Expect(cmd.Args).To(ContainElement("docker"))
			Expect(cmd.Args).To(ContainElement("run"))
			Expect(cmd.Args).To(ContainElement("--rm"))
			Expect(cmd.Args).To(ContainElement("--name"))
			Expect(cmd.Args).To(ContainElement("test-container"))
		})

		It("includes volume mounts", func() {
			cmd := exec.buildDockerCommand(
				ctx,
				"test",
				"/tmp/test.md",
				"/workspace",
				"/home/user/.claude",
				"test",
				"/home/user",
			)

			Expect(cmd.Args).To(ContainElement("/tmp/test.md:/tmp/prompt.md:ro"))
			Expect(cmd.Args).To(ContainElement("/workspace:/workspace"))
			Expect(cmd.Args).To(ContainElement("/home/user/.claude:/home/node/.claude"))
			Expect(cmd.Args).To(ContainElement("/home/user/go/pkg:/home/node/go/pkg"))
		})

		It("always includes network capabilities", func() {
			cmd := exec.buildDockerCommand(
				ctx,
				"test",
				"/tmp/test",
				"/workspace",
				"/home/user/.claude",
				"test",
				"/home/user",
			)

			Expect(cmd.Args).To(ContainElement("--cap-add=NET_ADMIN"))
			Expect(cmd.Args).To(ContainElement("--cap-add=NET_RAW"))
		})

		It("includes environment variable", func() {
			cmd := exec.buildDockerCommand(
				ctx,
				"test",
				"/tmp/test",
				"/workspace",
				"/home/user/.claude",
				"test",
				"/home/user",
			)

			Expect(cmd.Args).To(ContainElement("-e"))
			Expect(cmd.Args).To(ContainElement("YOLO_PROMPT_FILE=/tmp/prompt.md"))
		})

		It("sets correct context", func() {
			testCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			cmd := exec.buildDockerCommand(
				testCtx,
				"test",
				"/tmp/test",
				"/workspace",
				"/home/user/.claude",
				"test",
				"/home/user",
			)

			Expect(cmd).NotTo(BeNil())
			Expect(cmd.Path).NotTo(BeEmpty())
		})

		It("uses injected container image", func() {
			exec.containerImage = "custom-image:v1.2.3"
			cmd := exec.buildDockerCommand(
				ctx,
				"test",
				"/tmp/test",
				"/workspace",
				"/home/user/.claude",
				"test",
				"/home/user",
			)

			Expect(cmd.Args).To(ContainElement("custom-image:v1.2.3"))
		})

		It("includes netrc mount when netrcFile is set", func() {
			exec.netrcFile = "/home/user/.netrc"
			cmd := exec.buildDockerCommand(
				ctx,
				"test",
				"/tmp/test",
				"/workspace",
				"/home/user/.claude",
				"test",
				"/home/user",
			)

			Expect(cmd.Args).To(ContainElement("/home/user/.netrc:/home/node/.netrc:ro"))
		})

		It("does not include netrc mount when netrcFile is empty", func() {
			exec.netrcFile = ""
			cmd := exec.buildDockerCommand(
				ctx,
				"test",
				"/tmp/test",
				"/workspace",
				"/home/user/.claude",
				"test",
				"/home/user",
			)

			for _, arg := range cmd.Args {
				Expect(arg).NotTo(ContainSubstring(".netrc"))
			}
		})

		It("includes gitconfig mount when gitconfigFile is set", func() {
			exec.gitconfigFile = "/home/user/.gitconfig"
			cmd := exec.buildDockerCommand(
				ctx,
				"test",
				"/tmp/test",
				"/workspace",
				"/home/user/.claude",
				"test",
				"/home/user",
			)

			Expect(
				cmd.Args,
			).To(ContainElement("/home/user/.gitconfig:/home/node/.gitconfig-extra:ro"))
		})

		It("mounts gitconfig as read-only at staging path", func() {
			exec.gitconfigFile = "/home/user/.gitconfig"
			cmd := exec.buildDockerCommand(
				ctx,
				"test",
				"/tmp/test",
				"/workspace",
				"/home/user/.claude",
				"test",
				"/home/user",
			)

			Expect(
				cmd.Args,
			).To(ContainElement("/home/user/.gitconfig:/home/node/.gitconfig-extra:ro"))
		})

		It("expands tilde in gitconfigFile mount", func() {
			exec.gitconfigFile = "~/.gitconfig"
			cmd := exec.buildDockerCommand(
				ctx,
				"test",
				"/tmp/test",
				"/workspace",
				"/home/user/.claude",
				"test",
				"/home/user",
			)

			Expect(
				cmd.Args,
			).To(ContainElement("/home/user/.gitconfig:/home/node/.gitconfig-extra:ro"))
		})

		It("does not include gitconfig mount when gitconfigFile is empty", func() {
			exec.gitconfigFile = ""
			cmd := exec.buildDockerCommand(
				ctx,
				"test",
				"/tmp/test",
				"/workspace",
				"/home/user/.claude",
				"test",
				"/home/user",
			)

			for _, arg := range cmd.Args {
				Expect(arg).NotTo(ContainSubstring(".gitconfig"))
			}
		})

		It("includes sorted -e KEY=VALUE flags when env is set", func() {
			exec.env = map[string]string{
				"GOPRIVATE":    "bitbucket.example.com/*",
				"GONOSUMCHECK": "bitbucket.example.com/*",
			}
			cmd := exec.buildDockerCommand(
				ctx,
				"test",
				"/tmp/test",
				"/workspace",
				"/home/user/.claude",
				"test",
				"/home/user",
			)

			args := cmd.Args
			Expect(args).To(ContainElement("GOPRIVATE=bitbucket.example.com/*"))
			Expect(args).To(ContainElement("GONOSUMCHECK=bitbucket.example.com/*"))

			// Verify sorted order: GONOSUMCHECK before GOPRIVATE
			var gonosumIdx, goprivateIdx int
			for i, a := range args {
				if a == "GONOSUMCHECK=bitbucket.example.com/*" {
					gonosumIdx = i
				}
				if a == "GOPRIVATE=bitbucket.example.com/*" {
					goprivateIdx = i
				}
			}
			Expect(gonosumIdx).To(BeNumerically("<", goprivateIdx))
		})

		It("does not include extra -e flags when env is nil", func() {
			exec.env = nil
			cmd := exec.buildDockerCommand(
				ctx,
				"test",
				"/tmp/test",
				"/workspace",
				"/home/user/.claude",
				"test",
				"/home/user",
			)

			// Only YOLO_PROMPT_FILE and ANTHROPIC_MODEL should be present as -e values
			var envValues []string
			args := cmd.Args
			for i, a := range args {
				if a == "-e" && i+1 < len(args) {
					envValues = append(envValues, args[i+1])
				}
			}
			Expect(envValues).To(ConsistOf("YOLO_PROMPT_FILE=/tmp/prompt.md", "ANTHROPIC_MODEL="))
		})
	})

	Describe("watchForCompletionReport", func() {
		var (
			fakeRunner    *fakeCommandRunner
			logFile       string
			containerName string
		)

		BeforeEach(func() {
			fakeRunner = &fakeCommandRunner{}
			containerName = "test-container"
			logFile = filepath.Join(tempDir, "test.log")
		})

		Context("when log file contains completion report marker", func() {
			BeforeEach(func() {
				err := os.WriteFile(logFile, []byte("some output\n"+report.MarkerEnd+"\n"), 0600)
				Expect(err).NotTo(HaveOccurred())
			})

			It("calls docker stop after grace period", func() {
				err := watchForCompletionReport(
					ctx,
					logFile,
					containerName,
					100*time.Millisecond,
					10*time.Millisecond,
					fakeRunner,
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(fakeRunner.Commands()).To(HaveLen(1))
				Expect(
					fakeRunner.Commands()[0].Args,
				).To(ContainElements("docker", "stop", containerName))
			})
		})

		Context("when context is cancelled before grace period expires", func() {
			BeforeEach(func() {
				err := os.WriteFile(logFile, []byte("some output\n"+report.MarkerEnd+"\n"), 0600)
				Expect(err).NotTo(HaveOccurred())
			})

			It("does not call docker stop", func() {
				cancelCtx, cancel := context.WithCancel(ctx)
				// cancel after the marker is found but before grace period
				go func() {
					time.Sleep(20 * time.Millisecond)
					cancel()
				}()

				err := watchForCompletionReport(
					cancelCtx,
					logFile,
					containerName,
					10*time.Second,
					10*time.Millisecond,
					fakeRunner,
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(fakeRunner.Commands()).To(BeEmpty())
			})
		})

		Context("when log file never contains marker", func() {
			BeforeEach(func() {
				err := os.WriteFile(logFile, []byte("some output without marker\n"), 0600)
				Expect(err).NotTo(HaveOccurred())
			})

			It("does not call docker stop", func() {
				cancelCtx, cancel := context.WithCancel(ctx)
				cancel()

				err := watchForCompletionReport(
					cancelCtx,
					logFile,
					containerName,
					100*time.Millisecond,
					10*time.Millisecond,
					fakeRunner,
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(fakeRunner.Commands()).To(BeEmpty())
			})
		})
	})

	Describe("Execute", func() {
		var (
			exec       *dockerExecutor
			fakeRunner *fakeCommandRunner
			logFile    string
			logDir     string
		)

		BeforeEach(func() {
			fakeRunner = &fakeCommandRunner{}
			exec = &dockerExecutor{
				containerImage: config.Defaults().ContainerImage,
				projectName:    "test-project",
				commandRunner:  fakeRunner,
			}

			logDir = filepath.Join(tempDir, "logs")
			logFile = filepath.Join(logDir, "test.log")

			// Skip OAuth check in Execute tests — auth is tested separately
			GinkgoT().Setenv("ANTHROPIC_API_KEY", "test-key")
		})

		Context("with successful command execution", func() {
			It("creates log file and temp prompt file", func() {
				promptContent := "# Test prompt\n\nThis is a test."

				err := exec.Execute(ctx, promptContent, logFile, "test-container")
				Expect(err).NotTo(HaveOccurred())
				Expect(fakeRunner.RunCalled()).To(BeTrue())

				// Verify log file was created
				_, err = os.Stat(logFile)
				Expect(err).NotTo(HaveOccurred())
			})

			It("handles empty prompt content", func() {
				err := exec.Execute(ctx, "", logFile, "test-container")
				Expect(err).NotTo(HaveOccurred())
				Expect(fakeRunner.RunCalled()).To(BeTrue())
			})

			It("handles special characters in prompt", func() {
				promptContent := "# Test\n\n```bash\necho `whoami` && echo \"test\"\n```"

				err := exec.Execute(ctx, promptContent, logFile, "test-container")
				Expect(err).NotTo(HaveOccurred())
				Expect(fakeRunner.RunCalled()).To(BeTrue())
			})
		})

		Context("when log dir creation fails", func() {
			It("returns error", func() {
				invalidLogFile := "/invalid/path/that/does/not/exist/test.log"

				err := exec.Execute(ctx, "test", invalidLogFile, "test-container")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("create log directory"))
				Expect(fakeRunner.RunCalled()).To(BeFalse())
			})
		})

		Context("when command runner fails", func() {
			BeforeEach(func() {
				fakeRunner.err = errors.New(ctx, "command execution failed")
			})

			It("returns error", func() {
				err := exec.Execute(ctx, "test prompt", logFile, "test-container")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("docker run failed"))
				Expect(fakeRunner.RunCalled()).To(BeTrue())
			})
		})

		Context("with context cancellation", func() {
			It("passes context to command runner", func() {
				cancelCtx, cancel := context.WithCancel(ctx)
				cancel() // Cancel immediately

				fakeRunner.err = context.Canceled
				_ = exec.Execute(cancelCtx, "test", logFile, "test-container")
				// fakeRunner is called for both docker rm -f and docker run
				Expect(fakeRunner.RunCalled()).To(BeTrue())
			})
		})

		Context("container cleanup before run", func() {
			It("calls docker rm -f before docker run", func() {
				err := exec.Execute(ctx, "test prompt", logFile, "test-container")
				Expect(err).NotTo(HaveOccurred())
				Expect(fakeRunner.Commands()).To(HaveLen(2))
				Expect(
					fakeRunner.Commands()[0].Args,
				).To(ContainElements("docker", "rm", "-f", "test-container"))
				Expect(fakeRunner.Commands()[1].Args).To(ContainElements("docker", "run"))
			})
		})

		Context("with YAML frontmatter", func() {
			It("handles frontmatter correctly", func() {
				promptContent := `---
status: queued
priority: high
---
# Test prompt

This has frontmatter.`

				err := exec.Execute(ctx, promptContent, logFile, "test-container")
				Expect(err).NotTo(HaveOccurred())
				Expect(fakeRunner.RunCalled()).To(BeTrue())
			})
		})

		Context("with DARK_FACTORY_CLAUDE_CONFIG_DIR set", func() {
			It("uses env var as claude config dir in volume mount", func() {
				GinkgoT().Setenv("DARK_FACTORY_CLAUDE_CONFIG_DIR", "/custom/claude-config")

				err := exec.Execute(ctx, "test prompt", logFile, "test-container")
				Expect(err).NotTo(HaveOccurred())
				Expect(fakeRunner.Commands()).To(HaveLen(2))
				Expect(
					fakeRunner.Commands()[1].Args,
				).To(ContainElement("/custom/claude-config:/home/node/.claude"))
			})
		})

		Context("with DARK_FACTORY_CLAUDE_CONFIG_DIR unset", func() {
			It("uses default ~/.claude as claude config dir in volume mount", func() {
				GinkgoT().Setenv("DARK_FACTORY_CLAUDE_CONFIG_DIR", "")

				home, err := os.UserHomeDir()
				Expect(err).NotTo(HaveOccurred())

				err = exec.Execute(ctx, "test prompt", logFile, "test-container")
				Expect(err).NotTo(HaveOccurred())
				Expect(fakeRunner.Commands()).To(HaveLen(2))
				Expect(
					fakeRunner.Commands()[1].Args,
				).To(ContainElement(home + "/.claude:/home/node/.claude"))
			})
		})
	})

	Describe("Reattach", func() {
		var (
			execImpl   *dockerExecutor
			fakeRunner *fakeCommandRunner
			logFile    string
			logDir     string
		)

		BeforeEach(func() {
			fakeRunner = &fakeCommandRunner{}
			execImpl = &dockerExecutor{
				containerImage: config.Defaults().ContainerImage,
				projectName:    "test-project",
				commandRunner:  fakeRunner,
			}

			logDir = filepath.Join(tempDir, "logs")
			logFile = filepath.Join(logDir, "reattach-test.log")
		})

		Context("when container exits normally", func() {
			It("returns nil and creates log file", func() {
				err := execImpl.Reattach(ctx, logFile, "test-container")
				Expect(err).NotTo(HaveOccurred())
				Expect(fakeRunner.RunCalled()).To(BeTrue())

				_, err = os.Stat(logFile)
				Expect(err).NotTo(HaveOccurred())
			})

			It("uses docker logs --follow command", func() {
				err := execImpl.Reattach(ctx, logFile, "my-container")
				Expect(err).NotTo(HaveOccurred())
				Expect(fakeRunner.Commands()).To(HaveLen(1))
				Expect(
					fakeRunner.Commands()[0].Args,
				).To(ContainElements("docker", "logs", "--follow", "my-container"))
			})
		})

		Context("when context is cancelled", func() {
			It("returns without blocking", func() {
				cancelCtx, cancel := context.WithCancel(ctx)
				cancel()

				fakeRunner.err = context.Canceled
				// With a pre-cancelled context, watchForCompletionReport returns nil immediately.
				// CancelOnFirstFinish returns the first result from either goroutine, so the
				// outcome is non-deterministic — just verify Reattach completes promptly.
				_ = execImpl.Reattach(cancelCtx, logFile, "test-container")
			})
		})

		Context("when docker logs command fails", func() {
			BeforeEach(func() {
				fakeRunner.err = errors.New(ctx, "container not found")
			})

			It("returns an error wrapping reattach failed", func() {
				err := execImpl.Reattach(ctx, logFile, "nonexistent-container")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("reattach failed"))
			})
		})

		Context("when log dir creation fails", func() {
			It("returns error without calling docker", func() {
				invalidLogFile := "/invalid/path/that/does/not/exist/test.log"

				err := execImpl.Reattach(ctx, invalidLogFile, "test-container")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("prepare log file for reattach"))
				Expect(fakeRunner.RunCalled()).To(BeFalse())
			})
		})
	})

	Describe("validateClaudeAuth", func() {
		BeforeEach(func() {
			// Ensure ANTHROPIC_API_KEY is unset so auth check runs
			GinkgoT().Setenv("ANTHROPIC_API_KEY", "")
		})

		Context("when ANTHROPIC_API_KEY is set", func() {
			It("skips the check and returns no error", func() {
				GinkgoT().Setenv("ANTHROPIC_API_KEY", "sk-ant-test")
				err := validateClaudeAuth(ctx, "/nonexistent/path")
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when no config files exist", func() {
			It("returns error with fix hint", func() {
				err := validateClaudeAuth(ctx, "/nonexistent/path")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Claude OAuth token missing or expired"))
				Expect(
					err.Error(),
				).To(ContainSubstring("CLAUDE_CONFIG_DIR=/nonexistent/path claude"))
			})
		})

		Context("when .credentials.json has valid token (v2.x)", func() {
			It("returns no error", func() {
				credFile := filepath.Join(tempDir, ".credentials.json")
				err := os.WriteFile(
					credFile,
					[]byte(`{"claudeAiOauth":{"accessToken":"valid-token"}}`),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())

				err = validateClaudeAuth(ctx, tempDir)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when .credentials.json has empty token", func() {
			It("falls back and returns error when no legacy token either", func() {
				credFile := filepath.Join(tempDir, ".credentials.json")
				err := os.WriteFile(
					credFile,
					[]byte(`{"claudeAiOauth":{"accessToken":""}}`),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())

				err = validateClaudeAuth(ctx, tempDir)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Claude OAuth token missing or expired"))
			})
		})

		Context("when only .claude.json has valid token (v1.x legacy)", func() {
			It("returns no error", func() {
				configFile := filepath.Join(tempDir, ".claude.json")
				err := os.WriteFile(
					configFile,
					[]byte(`{"oauthAccount":{"accessToken":"valid-token-abc"}}`),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())

				err = validateClaudeAuth(ctx, tempDir)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when .claude.json has no oauthAccount and no .credentials.json", func() {
			It("returns error with fix hint", func() {
				configFile := filepath.Join(tempDir, ".claude.json")
				err := os.WriteFile(configFile, []byte(`{}`), 0600)
				Expect(err).NotTo(HaveOccurred())

				err = validateClaudeAuth(ctx, tempDir)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Claude OAuth token missing or expired"))
				Expect(err.Error()).To(ContainSubstring("CLAUDE_CONFIG_DIR=" + tempDir + " claude"))
			})
		})

		Context("when .claude.json has oauthAccount with empty accessToken", func() {
			It("returns error", func() {
				configFile := filepath.Join(tempDir, ".claude.json")
				err := os.WriteFile(configFile, []byte(`{"oauthAccount":{"accessToken":""}}`), 0600)
				Expect(err).NotTo(HaveOccurred())

				err = validateClaudeAuth(ctx, tempDir)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Claude OAuth token missing or expired"))
			})
		})
	})

	Describe("resolveClaudeConfigDir", func() {
		It("returns home/.claude when env var is empty", func() {
			GinkgoT().Setenv("DARK_FACTORY_CLAUDE_CONFIG_DIR", "")
			result := resolveClaudeConfigDir("/home/testuser")
			Expect(result).To(Equal("/home/testuser/.claude"))
		})

		It("returns env var value when set to absolute path", func() {
			GinkgoT().Setenv("DARK_FACTORY_CLAUDE_CONFIG_DIR", "/custom/config")
			result := resolveClaudeConfigDir("/home/testuser")
			Expect(result).To(Equal("/custom/config"))
		})

		It("expands ~ prefix to home directory", func() {
			GinkgoT().Setenv("DARK_FACTORY_CLAUDE_CONFIG_DIR", "~/.my-claude")
			result := resolveClaudeConfigDir("/home/testuser")
			Expect(result).To(Equal("/home/testuser/.my-claude"))
		})

		It("expands standalone ~ to home directory", func() {
			GinkgoT().Setenv("DARK_FACTORY_CLAUDE_CONFIG_DIR", "~")
			result := resolveClaudeConfigDir("/home/testuser")
			Expect(result).To(Equal("/home/testuser"))
		})

		It("expands $HOME variable", func() {
			GinkgoT().Setenv("DARK_FACTORY_CLAUDE_CONFIG_DIR", "$HOME/.my-claude")
			result := resolveClaudeConfigDir("/home/testuser")
			home, _ := os.UserHomeDir()
			Expect(result).To(Equal(home + "/.my-claude"))
		})
	})
})
