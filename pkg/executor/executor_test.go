// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package executor_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/bborbe/errors"
	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/executor"
	"github.com/bborbe/dark-factory/pkg/formatter"
	"github.com/bborbe/dark-factory/pkg/report"
)

var _ = Describe("DockerExecutor", func() {
	var (
		ctx     context.Context
		e       executor.Executor
		logFile string
		tempDir string
	)

	BeforeEach(func() {
		ctx = context.Background()
		e = executor.NewDockerExecutor(
			config.Defaults().ContainerImage,
			"test-project",
			config.Defaults().Model,
			"",
			"",
			nil,
			nil,
			config.Defaults().ResolvedClaudeDir(),
			0,
			libtime.NewCurrentDateTime(),
			formatter.NewFormatter(),
		)

		var err error
		tempDir, err = os.MkdirTemp("", "executor-test-*")
		Expect(err).NotTo(HaveOccurred())

		logFile = filepath.Join(tempDir, "test.log")
	})

	AfterEach(func() {
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	Describe("Execute", func() {
		Context("with simple prompt", func() {
			It("creates log file", func() {
				Skip("requires Docker and claude-yolo image")

				promptContent := "# Simple test prompt\n\nThis is a test."
				err := e.Execute(ctx, promptContent, logFile, "test-container")
				Expect(err).NotTo(HaveOccurred())

				// Verify log file was created
				_, err = os.Stat(logFile)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("with special characters", func() {
			It("handles backticks in prompt", func() {
				Skip("requires Docker and claude-yolo image")

				promptContent := "# Test with backticks\n\n```bash\necho `whoami`\n```"
				err := e.Execute(ctx, promptContent, logFile, "test-container")
				Expect(err).NotTo(HaveOccurred())
			})

			It("handles quotes in prompt", func() {
				Skip("requires Docker and claude-yolo image")

				promptContent := `# Test with "quotes" and 'single quotes'`
				err := e.Execute(ctx, promptContent, logFile, "test-container")
				Expect(err).NotTo(HaveOccurred())
			})

			It("handles YAML frontmatter with ---", func() {
				Skip("requires Docker and claude-yolo image")

				promptContent := `---
status: test
priority: high
---
# Test prompt

This has YAML frontmatter.`
				err := e.Execute(ctx, promptContent, logFile, "test-container")
				Expect(err).NotTo(HaveOccurred())
			})

			It("handles code blocks with special characters", func() {
				Skip("requires Docker and claude-yolo image")

				promptContent := `# Complex prompt

` + "```go" + `
cmd := exec.CommandContext(ctx, "docker", "run", "--rm",
    "-e", "YOLO_PROMPT="+promptContent,
    "-v", "$HOME/.claude:/home/node/.claude",
)
` + "```" + `

This should work!`
				err := e.Execute(ctx, promptContent, logFile, "test-container")
				Expect(err).NotTo(HaveOccurred())
			})

			It("handles newlines and multiline content", func() {
				Skip("requires Docker and claude-yolo image")

				promptContent := `Line 1
Line 2

Line 4 (with blank line)

More lines...`
				err := e.Execute(ctx, promptContent, logFile, "test-container")
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("with invalid log file path", func() {
			It("returns error when log directory cannot be created", func() {
				invalidLogFile := "/invalid/path/that/does/not/exist/test.log"
				err := e.Execute(ctx, "test prompt", invalidLogFile, "test-container")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("create log directory"))
			})
		})

		Context("when context is cancelled", func() {
			It("returns error", func() {
				Skip("requires Docker and claude-yolo image")

				cancelCtx, cancel := context.WithCancel(ctx)
				cancel() // Cancel immediately

				err := e.Execute(cancelCtx, "test prompt", logFile, "test-container")
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("NewDockerExecutor", func() {
		It("creates a new DockerExecutor with specified container image", func() {
			executor := executor.NewDockerExecutor(
				"custom-image:latest",
				"test-project",
				"claude-sonnet-4-6",
				"",
				"",
				nil,
				nil,
				config.Defaults().ResolvedClaudeDir(),
				0,
				libtime.NewCurrentDateTime(),
				formatter.NewFormatter(),
			)
			Expect(executor).NotTo(BeNil())
		})
	})
})

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

				file, err := executor.PrepareLogFileForTest(ctx, logFile)
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
				file, err := executor.PrepareLogFileForTest(ctx, logFile)
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

				file, err := executor.PrepareLogFileForTest(ctx, logFile)
				Expect(err).To(HaveOccurred())
				Expect(file).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("create log directory"))
			})
		})

		Context("with nested directories", func() {
			It("creates nested directories", func() {
				logFile := filepath.Join(tempDir, "level1", "level2", "level3", "test.log")

				file, err := executor.PrepareLogFileForTest(ctx, logFile)
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

				filePath, cleanup, err := executor.CreatePromptTempFileForTest(ctx, promptContent)
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

				filePath, cleanup, err := executor.CreatePromptTempFileForTest(ctx, promptContent)
				Expect(err).NotTo(HaveOccurred())
				defer cleanup()

				content, err := os.ReadFile(filePath)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(content)).To(Equal(promptContent))
			})
		})

		Context("with empty content", func() {
			It("creates empty file", func() {
				filePath, cleanup, err := executor.CreatePromptTempFileForTest(ctx, "")
				Expect(err).NotTo(HaveOccurred())
				defer cleanup()

				content, err := os.ReadFile(filePath)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(content)).To(Equal(""))
			})
		})

		Context("multiple calls", func() {
			It("creates unique files", func() {
				path1, cleanup1, err := executor.CreatePromptTempFileForTest(ctx, "# Prompt 1")
				Expect(err).NotTo(HaveOccurred())
				defer cleanup1()

				path2, cleanup2, err := executor.CreatePromptTempFileForTest(ctx, "# Prompt 2")
				Expect(err).NotTo(HaveOccurred())
				defer cleanup2()

				Expect(path1).NotTo(Equal(path2))
			})
		})
	})

	Describe("extractPromptBaseName", func() {
		It("extracts basename when prefix matches", func() {
			result := executor.ExtractPromptBaseNameForTest("myproject-042-fix-bug", "myproject")
			Expect(result).To(Equal("042-fix-bug"))
		})

		It("returns full name when prefix does not match", func() {
			result := executor.ExtractPromptBaseNameForTest("other-042-fix-bug", "myproject")
			Expect(result).To(Equal("other-042-fix-bug"))
		})

		It("returns containerName when it equals prefix with dash", func() {
			result := executor.ExtractPromptBaseNameForTest("myproject-", "myproject")
			Expect(result).To(Equal("myproject-"))
		})

		It("returns full name when containerName is shorter than prefix", func() {
			result := executor.ExtractPromptBaseNameForTest("my", "myproject")
			Expect(result).To(Equal("my"))
		})

		It("handles empty project name", func() {
			result := executor.ExtractPromptBaseNameForTest("some-container", "")
			Expect(result).To(Equal("some-container"))
		})

		It("handles empty container name", func() {
			result := executor.ExtractPromptBaseNameForTest("", "myproject")
			Expect(result).To(Equal(""))
		})
	})

	Describe("buildDockerCommand", func() {
		buildCmd := func(containerImage, projectName, model, netrcFile, gitconfigFile string, env map[string]string, extraMounts []config.ExtraMount) func(ctx context.Context, containerName, promptFilePath, projectRoot, claudeConfigDir, promptBaseName, home string) *exec.Cmd {
			return func(ctx context.Context, containerName, promptFilePath, projectRoot, claudeConfigDir, promptBaseName, home string) *exec.Cmd {
				return executor.BuildDockerCommandForTest(
					ctx,
					containerImage,
					projectName,
					model,
					netrcFile,
					gitconfigFile,
					env,
					extraMounts,
					containerName,
					promptFilePath,
					projectRoot,
					claudeConfigDir,
					promptBaseName,
					home,
				)
			}
		}

		defaultBuild := buildCmd(
			config.Defaults().ContainerImage,
			"test-project",
			"",
			"",
			"",
			nil,
			nil,
		)

		It("builds correct docker command", func() {
			cmd := defaultBuild(
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
			cmd := defaultBuild(
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
		})

		It("always includes network capabilities", func() {
			cmd := defaultBuild(
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
			cmd := defaultBuild(
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

			cmd := defaultBuild(
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
			build := buildCmd("custom-image:v1.2.3", "test-project", "", "", "", nil, nil)
			cmd := build(
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
			build := buildCmd(
				config.Defaults().ContainerImage,
				"test-project",
				"",
				"/home/user/.netrc",
				"",
				nil,
				nil,
			)
			cmd := build(
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
			cmd := defaultBuild(
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
			build := buildCmd(
				config.Defaults().ContainerImage,
				"test-project",
				"",
				"",
				"/home/user/.gitconfig",
				nil,
				nil,
			)
			cmd := build(
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
			build := buildCmd(
				config.Defaults().ContainerImage,
				"test-project",
				"",
				"",
				"/home/user/.gitconfig",
				nil,
				nil,
			)
			cmd := build(
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
			build := buildCmd(
				config.Defaults().ContainerImage,
				"test-project",
				"",
				"",
				"~/.gitconfig",
				nil,
				nil,
			)
			cmd := build(
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
			cmd := defaultBuild(
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
			envMap := map[string]string{
				"GOPRIVATE":    "bitbucket.example.com/*",
				"GONOSUMCHECK": "bitbucket.example.com/*",
			}
			build := buildCmd(
				config.Defaults().ContainerImage,
				"test-project",
				"",
				"",
				"",
				envMap,
				nil,
			)
			cmd := build(
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
			cmd := defaultBuild(
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
			Expect(
				envValues,
			).To(ConsistOf("YOLO_PROMPT_FILE=/tmp/prompt.md", "ANTHROPIC_MODEL=", "YOLO_OUTPUT=json"))
		})

		It("does not add extra -v flags when extraMounts is nil", func() {
			cmd := defaultBuild(
				ctx,
				"test",
				"/tmp/test.md",
				"/workspace",
				"/home/user/.claude",
				"test",
				"/home/user",
			)

			// Collect all -v values
			var mounts []string
			args := cmd.Args
			for i, a := range args {
				if a == "-v" && i+1 < len(args) {
					mounts = append(mounts, args[i+1])
				}
			}
			// Only the standard three mounts
			Expect(mounts).To(HaveLen(3))
		})

		It("adds extra mount without :ro suffix when ReadOnly is nil (default read-write)", func() {
			srcDir, err := os.MkdirTemp("", "extramount-src-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(srcDir) }()

			extraMounts := []config.ExtraMount{
				{Src: srcDir, Dst: "/docs"},
			}
			build := buildCmd(
				config.Defaults().ContainerImage,
				"test-project",
				"",
				"",
				"",
				nil,
				extraMounts,
			)
			cmd := build(
				ctx,
				"test",
				"/tmp/test.md",
				"/workspace",
				"/home/user/.claude",
				"test",
				"/home/user",
			)

			Expect(cmd.Args).To(ContainElement(srcDir + ":/docs"))
			for _, arg := range cmd.Args {
				if arg == srcDir+":/docs:ro" {
					Fail("expected no :ro suffix but found it")
				}
			}
		})

		It("adds extra mount with :ro suffix when ReadOnly is explicitly true", func() {
			srcDir, err := os.MkdirTemp("", "extramount-src-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(srcDir) }()

			t := true
			extraMounts := []config.ExtraMount{
				{Src: srcDir, Dst: "/docs", ReadOnly: &t},
			}
			build := buildCmd(
				config.Defaults().ContainerImage,
				"test-project",
				"",
				"",
				"",
				nil,
				extraMounts,
			)
			cmd := build(
				ctx,
				"test",
				"/tmp/test.md",
				"/workspace",
				"/home/user/.claude",
				"test",
				"/home/user",
			)

			Expect(cmd.Args).To(ContainElement(srcDir + ":/docs:ro"))
		})

		It("skips extra mount when src does not exist", func() {
			extraMounts := []config.ExtraMount{
				{Src: "/nonexistent/path/that/does/not/exist", Dst: "/docs"},
			}
			build := buildCmd(
				config.Defaults().ContainerImage,
				"test-project",
				"",
				"",
				"",
				nil,
				extraMounts,
			)
			cmd := build(
				ctx,
				"test",
				"/tmp/test.md",
				"/workspace",
				"/home/user/.claude",
				"test",
				"/home/user",
			)

			for _, arg := range cmd.Args {
				Expect(arg).NotTo(ContainSubstring("/nonexistent/path"))
			}
		})

		It("resolves relative src path against projectRoot", func() {
			projectRoot, err := os.MkdirTemp("", "extramount-project-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(projectRoot) }()

			// Create a subdirectory inside projectRoot
			subDir := filepath.Join(projectRoot, "docs")
			Expect(os.MkdirAll(subDir, 0755)).To(Succeed())

			extraMounts := []config.ExtraMount{
				{Src: "docs", Dst: "/container/docs"},
			}
			build := buildCmd(
				config.Defaults().ContainerImage,
				"test-project",
				"",
				"",
				"",
				nil,
				extraMounts,
			)
			cmd := build(
				ctx,
				"test",
				"/tmp/test.md",
				projectRoot,
				"/home/user/.claude",
				"test",
				"/home/user",
			)

			Expect(cmd.Args).To(ContainElement(subDir + ":/container/docs"))
		})

		It("expands tilde in extra mount src", func() {
			// Use tempDir as home and create a subdir inside it to simulate ~/docs
			homeDir, err := os.MkdirTemp("", "extramount-home-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(homeDir) }()

			docsDir := filepath.Join(homeDir, "docs")
			Expect(os.MkdirAll(docsDir, 0755)).To(Succeed())

			extraMounts := []config.ExtraMount{
				{Src: "~/docs", Dst: "/container/docs"},
			}
			build := buildCmd(
				config.Defaults().ContainerImage,
				"test-project",
				"",
				"",
				"",
				nil,
				extraMounts,
			)
			cmd := build(
				ctx,
				"test",
				"/tmp/test.md",
				"/workspace",
				"/home/user/.claude",
				"test",
				homeDir,
			)

			Expect(cmd.Args).To(ContainElement(docsDir + ":/container/docs"))
		})

		It("expands $VAR env var in extra mount src", func() {
			homeDir, err := os.MkdirTemp("", "extramount-envvar-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(homeDir) }()

			docsDir := filepath.Join(homeDir, "docs")
			Expect(os.MkdirAll(docsDir, 0755)).To(Succeed())

			Expect(os.Setenv("TEST_HOME_VAR", homeDir)).To(Succeed())
			DeferCleanup(func() { _ = os.Unsetenv("TEST_HOME_VAR") })

			extraMounts := []config.ExtraMount{
				{Src: "$TEST_HOME_VAR/docs", Dst: "/container/docs"},
			}
			build := buildCmd(
				config.Defaults().ContainerImage,
				"test-project",
				"",
				"",
				"",
				nil,
				extraMounts,
			)
			cmd := build(
				ctx,
				"test",
				"/tmp/test.md",
				"/workspace",
				"/home/user/.claude",
				"test",
				"/home/user",
			)

			Expect(cmd.Args).To(ContainElement(docsDir + ":/container/docs"))
		})

		It("expands ${VAR} env var in extra mount src", func() {
			gopathDir, err := os.MkdirTemp("", "extramount-gopath-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(gopathDir) }()

			pkgDir := filepath.Join(gopathDir, "pkg")
			Expect(os.MkdirAll(pkgDir, 0755)).To(Succeed())

			Expect(os.Setenv("TEST_GOPATH_VAR", gopathDir)).To(Succeed())
			DeferCleanup(func() { _ = os.Unsetenv("TEST_GOPATH_VAR") })

			extraMounts := []config.ExtraMount{
				{Src: "${TEST_GOPATH_VAR}/pkg", Dst: "/home/node/go/pkg"},
			}
			build := buildCmd(
				config.Defaults().ContainerImage,
				"test-project",
				"",
				"",
				"",
				nil,
				extraMounts,
			)
			cmd := build(
				ctx,
				"test",
				"/tmp/test.md",
				"/workspace",
				"/home/user/.claude",
				"test",
				"/home/user",
			)

			Expect(cmd.Args).To(ContainElement(pkgDir + ":/home/node/go/pkg"))
		})

		It("skips extra mount when env var is undefined (expands to empty string)", func() {
			Expect(os.Unsetenv("TEST_UNDEFINED_VAR_XYZ")).To(Succeed())

			extraMounts := []config.ExtraMount{
				{Src: "$TEST_UNDEFINED_VAR_XYZ/docs", Dst: "/container/docs"},
			}
			build := buildCmd(
				config.Defaults().ContainerImage,
				"test-project",
				"",
				"",
				"",
				nil,
				extraMounts,
			)
			cmd := build(
				ctx,
				"test",
				"/tmp/test.md",
				"/workspace",
				"/home/user/.claude",
				"test",
				"/home/user",
			)

			for _, arg := range cmd.Args {
				Expect(arg).NotTo(ContainSubstring("/container/docs"))
			}
		})

		It("expands tilde after env var expansion in extra mount src", func() {
			homeDir, err := os.MkdirTemp("", "extramount-tilde-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(homeDir) }()

			docsDir := filepath.Join(homeDir, "docs")
			Expect(os.MkdirAll(docsDir, 0755)).To(Succeed())

			extraMounts := []config.ExtraMount{
				{Src: "~/docs", Dst: "/container/docs"},
			}
			build := buildCmd(
				config.Defaults().ContainerImage,
				"test-project",
				"",
				"",
				"",
				nil,
				extraMounts,
			)
			cmd := build(
				ctx,
				"test",
				"/tmp/test.md",
				"/workspace",
				"/home/user/.claude",
				"test",
				homeDir,
			)

			Expect(cmd.Args).To(ContainElement(docsDir + ":/container/docs"))
		})
	})

	Describe("watchForCompletionReport", func() {
		var (
			fakeRunner    *mocks.CommandRunner
			logFile       string
			containerName string
		)

		BeforeEach(func() {
			fakeRunner = &mocks.CommandRunner{}
			containerName = "test-container"
			logFile = filepath.Join(tempDir, "test.log")
		})

		Context("when log file contains completion report marker", func() {
			BeforeEach(func() {
				err := os.WriteFile(logFile, []byte("some output\n"+report.MarkerEnd+"\n"), 0600)
				Expect(err).NotTo(HaveOccurred())
			})

			It("calls docker stop after grace period", func() {
				err := executor.WatchForCompletionReportForTest(
					ctx,
					logFile,
					containerName,
					100*time.Millisecond,
					10*time.Millisecond,
					fakeRunner,
					libtime.NewCurrentDateTime(),
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(fakeRunner.RunCallCount()).To(Equal(1))
				_, firstCmd := fakeRunner.RunArgsForCall(0)
				Expect(firstCmd.Args).To(ContainElements("docker", "stop", containerName))
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

				err := executor.WatchForCompletionReportForTest(
					cancelCtx,
					logFile,
					containerName,
					10*time.Second,
					10*time.Millisecond,
					fakeRunner,
					libtime.NewCurrentDateTime(),
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(fakeRunner.RunCallCount()).To(Equal(0))
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

				err := executor.WatchForCompletionReportForTest(
					cancelCtx,
					logFile,
					containerName,
					100*time.Millisecond,
					10*time.Millisecond,
					fakeRunner,
					libtime.NewCurrentDateTime(),
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(fakeRunner.RunCallCount()).To(Equal(0))
			})
		})
	})

	Describe("Execute", func() {
		var (
			fakeRunner *mocks.CommandRunner
			logFile    string
			logDir     string
		)

		BeforeEach(func() {
			fakeRunner = &mocks.CommandRunner{}
			logDir = filepath.Join(tempDir, "logs")
			logFile = filepath.Join(logDir, "test.log")

			// Skip OAuth check in Execute tests — auth is tested separately
			GinkgoT().Setenv("ANTHROPIC_API_KEY", "test-key")
		})

		newExec := func(runner *mocks.CommandRunner, claudeDir string) executor.Executor {
			fakeFormatter := &mocks.StreamFormatter{}
			fakeFormatter.ProcessStreamStub = func(_ context.Context, r io.Reader, _ io.Writer, _ io.Writer) error {
				_, _ = io.Copy(io.Discard, r)
				return nil
			}
			return executor.NewDockerExecutorWithRunnerForTest(
				config.Defaults().ContainerImage,
				"test-project",
				"",
				"",
				"",
				nil,
				nil,
				claudeDir,
				0,
				libtime.NewCurrentDateTime(),
				runner,
				fakeFormatter,
			)
		}

		Context("with successful command execution", func() {
			It("creates log file and temp prompt file", func() {
				e := newExec(fakeRunner, "/tmp/test-claude-yolo")
				promptContent := "# Test prompt\n\nThis is a test."

				err := e.Execute(ctx, promptContent, logFile, "test-container")
				Expect(err).NotTo(HaveOccurred())
				Expect(fakeRunner.RunCallCount()).To(BeNumerically(">", 0))

				// Verify log file was created
				_, err = os.Stat(logFile)
				Expect(err).NotTo(HaveOccurred())
			})

			It("handles empty prompt content", func() {
				e := newExec(fakeRunner, "/tmp/test-claude-yolo")
				err := e.Execute(ctx, "", logFile, "test-container")
				Expect(err).NotTo(HaveOccurred())
				Expect(fakeRunner.RunCallCount()).To(BeNumerically(">", 0))
			})

			It("handles special characters in prompt", func() {
				e := newExec(fakeRunner, "/tmp/test-claude-yolo")
				promptContent := "# Test\n\n```bash\necho `whoami` && echo \"test\"\n```"

				err := e.Execute(ctx, promptContent, logFile, "test-container")
				Expect(err).NotTo(HaveOccurred())
				Expect(fakeRunner.RunCallCount()).To(BeNumerically(">", 0))
			})
		})

		Context("when log dir creation fails", func() {
			It("returns error", func() {
				e := newExec(fakeRunner, "/tmp/test-claude-yolo")
				invalidLogFile := "/invalid/path/that/does/not/exist/test.log"

				err := e.Execute(ctx, "test", invalidLogFile, "test-container")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("create log directory"))
				Expect(fakeRunner.RunCallCount()).To(Equal(0))
			})
		})

		Context("when command runner fails", func() {
			BeforeEach(func() {
				fakeRunner.RunStub = func(ctx context.Context, cmd *exec.Cmd) error {
					return errors.New(ctx, "command execution failed")
				}
			})

			It("returns error", func() {
				e := newExec(fakeRunner, "/tmp/test-claude-yolo")
				err := e.Execute(ctx, "test prompt", logFile, "test-container")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("docker run failed"))
				Expect(fakeRunner.RunCallCount()).To(BeNumerically(">", 0))
			})
		})

		Context("with context cancellation", func() {
			It("passes context to command runner", func() {
				fakeRunner.RunStub = func(ctx context.Context, cmd *exec.Cmd) error {
					return context.Canceled
				}
				e := newExec(fakeRunner, "/tmp/test-claude-yolo")
				cancelCtx, cancel := context.WithCancel(ctx)
				cancel() // Cancel immediately

				_ = e.Execute(cancelCtx, "test", logFile, "test-container")
				// fakeRunner is called for both docker rm -f and docker run
				Expect(fakeRunner.RunCallCount()).To(BeNumerically(">", 0))
			})
		})

		Context("container cleanup before run", func() {
			It("calls docker rm -f before docker run", func() {
				e := newExec(fakeRunner, "/tmp/test-claude-yolo")
				err := e.Execute(ctx, "test prompt", logFile, "test-container")
				Expect(err).NotTo(HaveOccurred())
				Expect(fakeRunner.RunCallCount()).To(Equal(2))
				_, firstCmd := fakeRunner.RunArgsForCall(0)
				Expect(firstCmd.Args).To(ContainElements("docker", "rm", "-f", "test-container"))
				_, secondCmd := fakeRunner.RunArgsForCall(1)
				Expect(secondCmd.Args).To(ContainElements("docker", "run"))
			})
		})

		Context("with YAML frontmatter", func() {
			It("handles frontmatter correctly", func() {
				e := newExec(fakeRunner, "/tmp/test-claude-yolo")
				promptContent := `---
status: queued
priority: high
---
# Test prompt

This has frontmatter.`

				err := e.Execute(ctx, promptContent, logFile, "test-container")
				Expect(err).NotTo(HaveOccurred())
				Expect(fakeRunner.RunCallCount()).To(BeNumerically(">", 0))
			})
		})

		Context("with custom claudeDir", func() {
			It("uses claudeDir field as claude config dir in volume mount", func() {
				e := newExec(fakeRunner, "/custom/claude-config")
				err := e.Execute(ctx, "test prompt", logFile, "test-container")
				Expect(err).NotTo(HaveOccurred())
				Expect(fakeRunner.RunCallCount()).To(Equal(2))
				_, secondCmd := fakeRunner.RunArgsForCall(1)
				Expect(
					secondCmd.Args,
				).To(ContainElement("/custom/claude-config:/home/node/.claude"))
			})
		})

		Context("with default claudeDir", func() {
			It("uses claudeDir field as claude config dir in volume mount", func() {
				e := newExec(fakeRunner, "/tmp/test-claude-yolo")
				err := e.Execute(ctx, "test prompt", logFile, "test-container")
				Expect(err).NotTo(HaveOccurred())
				Expect(fakeRunner.RunCallCount()).To(Equal(2))
				_, secondCmd := fakeRunner.RunArgsForCall(1)
				Expect(
					secondCmd.Args,
				).To(ContainElement("/tmp/test-claude-yolo:/home/node/.claude"))
			})
		})
	})

	Describe("Reattach", func() {
		var (
			fakeRunner *mocks.CommandRunner
			logFile    string
			logDir     string
		)

		BeforeEach(func() {
			fakeRunner = &mocks.CommandRunner{}
			logDir = filepath.Join(tempDir, "logs")
			logFile = filepath.Join(logDir, "reattach-test.log")
		})

		newExec := func(runner *mocks.CommandRunner) executor.Executor {
			fakeFormatter := &mocks.StreamFormatter{}
			fakeFormatter.ProcessStreamStub = func(_ context.Context, r io.Reader, _ io.Writer, _ io.Writer) error {
				_, _ = io.Copy(io.Discard, r)
				return nil
			}
			return executor.NewDockerExecutorWithRunnerForTest(
				config.Defaults().ContainerImage,
				"test-project",
				"",
				"",
				"",
				nil,
				nil,
				"",
				0,
				libtime.NewCurrentDateTime(),
				runner,
				fakeFormatter,
			)
		}

		Context("when container exits normally", func() {
			It("returns nil and creates log file", func() {
				e := newExec(fakeRunner)
				err := e.Reattach(ctx, logFile, "test-container", 0)
				Expect(err).NotTo(HaveOccurred())
				Expect(fakeRunner.RunCallCount()).To(BeNumerically(">", 0))

				_, err = os.Stat(logFile)
				Expect(err).NotTo(HaveOccurred())
			})

			It("uses docker logs --follow command", func() {
				e := newExec(fakeRunner)
				err := e.Reattach(ctx, logFile, "my-container", 0)
				Expect(err).NotTo(HaveOccurred())
				Expect(fakeRunner.RunCallCount()).To(Equal(1))
				_, firstCmd := fakeRunner.RunArgsForCall(0)
				Expect(
					firstCmd.Args,
				).To(ContainElements("docker", "logs", "--follow", "my-container"))
			})
		})

		Context("when context is cancelled", func() {
			It("returns without blocking", func() {
				fakeRunner.RunStub = func(ctx context.Context, cmd *exec.Cmd) error {
					return context.Canceled
				}
				e := newExec(fakeRunner)
				cancelCtx, cancel := context.WithCancel(ctx)
				cancel()

				// With a pre-cancelled context, watchForCompletionReport returns nil immediately.
				// CancelOnFirstFinish returns the first result from either goroutine, so the
				// outcome is non-deterministic — just verify Reattach completes promptly.
				_ = e.Reattach(cancelCtx, logFile, "test-container", 0)
			})
		})

		Context("when docker logs command fails", func() {
			BeforeEach(func() {
				fakeRunner.RunStub = func(ctx context.Context, cmd *exec.Cmd) error {
					return errors.New(ctx, "container not found")
				}
			})

			It("returns an error wrapping reattach failed", func() {
				e := newExec(fakeRunner)
				err := e.Reattach(ctx, logFile, "nonexistent-container", 0)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("reattach failed"))
			})
		})

		Context("when log dir creation fails", func() {
			It("returns error without calling docker", func() {
				e := newExec(fakeRunner)
				invalidLogFile := "/invalid/path/that/does/not/exist/test.log"

				err := e.Reattach(ctx, invalidLogFile, "test-container", 0)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("prepare log file for reattach"))
				Expect(fakeRunner.RunCallCount()).To(Equal(0))
			})
		})
	})

	Describe("Execute streaming pipeline", func() {
		var (
			fakeRunner *mocks.CommandRunner
			logFile    string
			logDir     string
		)

		BeforeEach(func() {
			fakeRunner = &mocks.CommandRunner{}
			logDir = filepath.Join(tempDir, "stream-logs")
			logFile = filepath.Join(logDir, "042.log")
			GinkgoT().Setenv("ANTHROPIC_API_KEY", "test-key")
		})

		newStreamExec := func(runner *mocks.CommandRunner) executor.Executor {
			return executor.NewDockerExecutorWithRunnerForTest(
				config.Defaults().ContainerImage,
				"test-project",
				"",
				"",
				"",
				nil,
				nil,
				"/tmp/test-claude-yolo",
				0,
				libtime.NewCurrentDateTime(),
				runner,
				formatter.NewFormatter(),
			)
		}

		Context("9a: JSONL file is created alongside formatted log", func() {
			It("creates both .log and .jsonl files with correct content", func() {
				jsonLines := strings.Join([]string{
					`{"type":"system","subtype":"init","session_id":"test-123","model":"claude-opus","cwd":"/workspace","tools":[]}`,
					`{"type":"assistant","message":{"content":[{"type":"text","text":"hello"}]}}`,
					`{"type":"result","result":"success","duration_ms":100}`,
				}, "\n") + "\n"

				fakeRunner.RunStub = func(_ context.Context, cmd *exec.Cmd) error {
					if cmd.Stdout != nil && strings.Contains(strings.Join(cmd.Args, " "), "run") {
						_, _ = io.WriteString(cmd.Stdout, jsonLines)
					}
					return nil
				}

				e := newStreamExec(fakeRunner)
				err := e.Execute(ctx, "test prompt", logFile, "test-project-042-stream")
				Expect(err).NotTo(HaveOccurred())

				// Formatted .log file exists and is non-empty
				logContent, err := os.ReadFile(logFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(logContent).NotTo(BeEmpty())

				// JSONL file exists with exactly 3 lines
				rawFile := executor.RawLogPathForTest(logFile)
				Expect(rawFile).To(Equal(filepath.Join(logDir, "042.jsonl")))
				rawContent, err := os.ReadFile(rawFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(rawContent).NotTo(BeEmpty())

				rawLines := strings.Split(strings.TrimRight(string(rawContent), "\n"), "\n")
				Expect(rawLines).To(HaveLen(3))

				// Each line is valid JSON
				for _, line := range rawLines {
					var v interface{}
					Expect(
						json.Unmarshal([]byte(line), &v),
					).To(Succeed(), "line should be valid JSON: %s", line)
				}

				// Formatted log contains "hello" from the assistant message
				Expect(string(logContent)).To(ContainSubstring("hello"))
				// Formatted log contains "[init]" from system init event
				Expect(string(logContent)).To(ContainSubstring("[init]"))
			})
		})

		Context("9b: raw JSONL preserves non-JSON lines verbatim", func() {
			It("passes non-JSON through to both files and returns nil", func() {
				lines := "not-json-at-all\n" + `{"type":"result","result":"success","duration_ms":1}` + "\n"

				fakeRunner.RunStub = func(_ context.Context, cmd *exec.Cmd) error {
					if cmd.Stdout != nil && strings.Contains(strings.Join(cmd.Args, " "), "run") {
						_, _ = io.WriteString(cmd.Stdout, lines)
					}
					return nil
				}

				e := newStreamExec(fakeRunner)
				err := e.Execute(ctx, "test prompt", logFile, "test-project-042-nonjson")
				Expect(err).NotTo(HaveOccurred())

				rawFile := executor.RawLogPathForTest(logFile)
				rawContent, err := os.ReadFile(rawFile)
				Expect(err).NotTo(HaveOccurred())

				rawStr := string(rawContent)
				Expect(rawStr).To(ContainSubstring("not-json-at-all"))
				Expect(rawStr).To(ContainSubstring(`"type":"result"`))

				logContent, err := os.ReadFile(logFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(logContent)).To(ContainSubstring("not-json-at-all"))
			})
		})

		Context("9c: raw log file open failure returns error before container starts", func() {
			It("returns error mentioning raw log path and does not start container", func() {
				// Create a directory with the JSONL path name to block file creation.
				Expect(os.MkdirAll(logDir, 0750)).To(Succeed())
				rawFile := executor.RawLogPathForTest(logFile)
				// Create a directory where the JSONL file would go — OpenFile will fail.
				Expect(os.MkdirAll(rawFile, 0750)).To(Succeed())

				e := newStreamExec(fakeRunner)
				err := e.Execute(ctx, "test prompt", logFile, "test-project-042-rawfail")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(rawFile))

				// The docker run command was never started (only docker rm -f may have been called)
				for i := 0; i < fakeRunner.RunCallCount(); i++ {
					_, cmd := fakeRunner.RunArgsForCall(i)
					Expect(cmd.Args).NotTo(ContainElement("run"))
				}
			})
		})

		Context("9d: Reattach also produces JSONL file", func() {
			It("creates both .log and .jsonl files when reattaching", func() {
				jsonLines := strings.Join([]string{
					`{"type":"system","subtype":"init","session_id":"reattach-456","model":"claude-opus","cwd":"/workspace","tools":[]}`,
					`{"type":"assistant","message":{"content":[{"type":"text","text":"reattached"}]}}`,
					`{"type":"result","result":"success","duration_ms":50}`,
				}, "\n") + "\n"

				fakeRunner.RunStub = func(_ context.Context, cmd *exec.Cmd) error {
					if cmd.Stdout != nil {
						_, _ = io.WriteString(cmd.Stdout, jsonLines)
					}
					return nil
				}

				e := newStreamExec(fakeRunner)
				err := e.Reattach(ctx, logFile, "test-container-reattach", 0)
				Expect(err).NotTo(HaveOccurred())

				logContent, err := os.ReadFile(logFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(logContent).NotTo(BeEmpty())

				rawFile := executor.RawLogPathForTest(logFile)
				rawContent, err := os.ReadFile(rawFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(rawContent).NotTo(BeEmpty())
			})
		})

		Context("9e: YOLO_OUTPUT=json is present in docker run args", func() {
			It("includes -e YOLO_OUTPUT=json as adjacent args pair", func() {
				cmd := executor.BuildDockerCommandForTest(
					ctx,
					config.Defaults().ContainerImage,
					"test-project",
					"",
					"",
					"",
					nil,
					nil,
					"test-container",
					"/tmp/prompt.md",
					"/workspace",
					"/home/user/.claude",
					"test-prompt",
					"/home/user",
				)

				args := cmd.Args
				found := false
				for i, arg := range args {
					if arg == "-e" && i+1 < len(args) && args[i+1] == "YOLO_OUTPUT=json" {
						found = true
						break
					}
				}
				Expect(
					found,
				).To(BeTrue(), "expected -e followed by YOLO_OUTPUT=json in docker args")
			})
		})

		It("never emits --tmpfs /workspace/.git", func() {
			cmd := executor.BuildDockerCommandForTest(
				ctx,
				config.Defaults().ContainerImage,
				"test-project",
				"",
				"",
				"",
				nil,
				nil,
				"test-container",
				"/tmp/prompt.md",
				"/workspace",
				"/home/user/.claude",
				"test-prompt",
				"/home/user",
			)
			for _, arg := range cmd.Args {
				Expect(arg).NotTo(Equal("--tmpfs"), "docker args must not contain --tmpfs")
			}
		})
	})

	Describe("rawLogPath", func() {
		It("replaces .log extension with .jsonl", func() {
			Expect(
				executor.RawLogPathForTest("prompts/log/042.log"),
			).To(Equal("prompts/log/042.jsonl"))
		})

		It("handles path with no extension", func() {
			Expect(executor.RawLogPathForTest("prompts/log/042")).To(Equal("prompts/log/042.jsonl"))
		})

		It("handles .txt extension", func() {
			Expect(executor.RawLogPathForTest("/tmp/test.txt")).To(Equal("/tmp/test.jsonl"))
		})
	})

	Describe("timeoutKiller", func() {
		var (
			fakeRunner    *mocks.CommandRunner
			containerName string
		)

		BeforeEach(func() {
			fakeRunner = &mocks.CommandRunner{}
			containerName = "test-timeout-container"
		})

		Context("when context is cancelled before deadline", func() {
			It("returns nil without calling any docker command", func() {
				cancelCtx, cancel := context.WithCancel(ctx)
				cancel() // cancel immediately

				err := executor.TimeoutKillerForTest(
					cancelCtx,
					10*time.Second,
					containerName,
					fakeRunner,
					libtime.NewCurrentDateTime(),
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(fakeRunner.RunCallCount()).To(Equal(0))
			})
		})

		Context("when deadline fires before context is cancelled", func() {
			It("calls docker stop and returns timeout error", func() {
				err := executor.TimeoutKillerForTest(
					ctx,
					10*time.Millisecond,
					containerName,
					fakeRunner,
					libtime.NewCurrentDateTime(),
				)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("timed out after"))
				Expect(fakeRunner.RunCallCount()).To(Equal(1))
				_, firstCmd := fakeRunner.RunArgsForCall(0)
				Expect(firstCmd.Args).To(ContainElements("docker", "stop", containerName))
			})
		})

		Context("when docker stop fails", func() {
			It("falls back to docker kill", func() {
				callCount := 0
				multiRunner := &mocks.CommandRunner{}
				multiRunner.RunStub = func(ctx context.Context, cmd *exec.Cmd) error {
					callCount++
					if callCount == 1 {
						return errors.New(context.Background(), "docker stop failed")
					}
					return nil
				}

				err := executor.TimeoutKillerForTest(
					ctx,
					10*time.Millisecond,
					containerName,
					multiRunner,
					libtime.NewCurrentDateTime(),
				)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("timed out after"))
				Expect(multiRunner.RunCallCount()).To(Equal(2))
				_, firstCmd := multiRunner.RunArgsForCall(0)
				Expect(firstCmd.Args).To(ContainElements("docker", "stop", containerName))
				_, secondCmd := multiRunner.RunArgsForCall(1)
				Expect(secondCmd.Args).To(ContainElements("docker", "kill", containerName))
			})
		})
	})

	Describe("defaultCommandRunner", func() {
		It("returns nil when command exits normally", func() {
			runner := executor.NewDefaultCommandRunnerForTest()
			cmd := exec.Command("true")
			err := runner.Run(ctx, cmd)
			Expect(err).To(BeNil())
		})

		It("returns error when command exits with non-zero status", func() {
			runner := executor.NewDefaultCommandRunnerForTest()
			cmd := exec.Command("false")
			err := runner.Run(ctx, cmd)
			Expect(err).To(HaveOccurred())
		})

		It("terminates long-running process when context is cancelled", func() {
			runner := executor.NewDefaultCommandRunnerForTest()
			cancelCtx, cancel := context.WithCancel(ctx)

			cmd := exec.Command("sleep", "60")
			errCh := make(chan error, 1)
			go func() {
				errCh <- runner.Run(cancelCtx, cmd)
			}()

			time.Sleep(100 * time.Millisecond)
			cancel()

			Eventually(errCh).WithTimeout(2 * time.Second).Should(Receive(HaveOccurred()))
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
				err := executor.ValidateClaudeAuthForTest(ctx, "/nonexistent/path")
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when no config files exist", func() {
			It("returns error with fix hint", func() {
				err := executor.ValidateClaudeAuthForTest(ctx, "/nonexistent/path")
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

				err = executor.ValidateClaudeAuthForTest(ctx, tempDir)
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

				err = executor.ValidateClaudeAuthForTest(ctx, tempDir)
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

				err = executor.ValidateClaudeAuthForTest(ctx, tempDir)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when .claude.json has no oauthAccount and no .credentials.json", func() {
			It("returns error with fix hint", func() {
				configFile := filepath.Join(tempDir, ".claude.json")
				err := os.WriteFile(configFile, []byte(`{}`), 0600)
				Expect(err).NotTo(HaveOccurred())

				err = executor.ValidateClaudeAuthForTest(ctx, tempDir)
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

				err = executor.ValidateClaudeAuthForTest(ctx, tempDir)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Claude OAuth token missing or expired"))
			})
		})
	})

	Describe("waitUntilDeadline", func() {
		It("returns true when deadline is reached", func() {
			getter := libtime.NewCurrentDateTime()
			deadline := time.Time(getter.Now()).Add(50 * time.Millisecond)
			result := executor.WaitUntilDeadlineForTest(ctx, getter, deadline, 5*time.Millisecond)
			Expect(result).To(BeTrue())
		})

		It("returns false when context is cancelled", func() {
			cancelCtx, cancel := context.WithCancel(ctx)
			getter := libtime.NewCurrentDateTime()
			deadline := time.Time(getter.Now()).Add(10 * time.Second)
			go func() {
				time.Sleep(50 * time.Millisecond)
				cancel()
			}()
			result := executor.WaitUntilDeadlineForTest(
				cancelCtx,
				getter,
				deadline,
				5*time.Millisecond,
			)
			Expect(result).To(BeFalse())
		})

		It("returns true immediately when deadline is already past", func() {
			getter := libtime.NewCurrentDateTime()
			deadline := time.Time(getter.Now()).Add(-1 * time.Second)
			result := executor.WaitUntilDeadlineForTest(ctx, getter, deadline, 5*time.Millisecond)
			Expect(result).To(BeTrue())
		})

		It("uses injected time getter for deadline comparison", func() {
			calls := 0
			getter := libtime.CurrentDateTimeGetterFunc(func() libtime.DateTime {
				calls++
				if calls <= 2 {
					return libtime.DateTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
				}
				return libtime.DateTime(
					time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC),
				) // past deadline
			})
			deadline := time.Date(2026, 1, 1, 0, 30, 0, 0, time.UTC)
			result := executor.WaitUntilDeadlineForTest(ctx, getter, deadline, 5*time.Millisecond)
			Expect(result).To(BeTrue())
			Expect(calls).To(BeNumerically(">=", 3))
		})
	})
})
