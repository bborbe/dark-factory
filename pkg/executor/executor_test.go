// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package executor_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/executor"
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
		e = executor.NewDockerExecutor()

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
		It("creates a new DockerExecutor", func() {
			executor := executor.NewDockerExecutor()
			Expect(executor).NotTo(BeNil())
		})
	})

	Describe("prepareLogFile", func() {
		Context("with valid log file path", func() {
			It("creates log directory and opens file", func() {
				logFile := filepath.Join(tempDir, "logs", "test.log")

				file, err := executor.PrepareLogFile(ctx, logFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(file).NotTo(BeNil())

				// Clean up
				file.Close()

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
				// Create existing file with content
				err := os.WriteFile(logFile, []byte("old content"), 0600)
				Expect(err).NotTo(HaveOccurred())
			})

			It("truncates existing file", func() {
				file, err := executor.PrepareLogFile(ctx, logFile)
				Expect(err).NotTo(HaveOccurred())
				defer file.Close()

				// Write new content
				_, err = file.WriteString("new content")
				Expect(err).NotTo(HaveOccurred())
				file.Close()

				// Verify file was truncated and has only new content
				content, err := os.ReadFile(logFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(content)).To(Equal("new content"))
			})
		})

		Context("with invalid path", func() {
			It("returns error", func() {
				logFile := "/invalid/path/that/does/not/exist/test.log"

				file, err := executor.PrepareLogFile(ctx, logFile)
				Expect(err).To(HaveOccurred())
				Expect(file).To(BeNil())
			})
		})
	})

	Describe("createPromptTempFile", func() {
		Context("with valid content", func() {
			It("creates temp file with content", func() {
				promptContent := "# Test Prompt\n\nThis is test content."

				filePath, cleanup, err := executor.CreatePromptTempFile(ctx, promptContent)
				Expect(err).NotTo(HaveOccurred())
				Expect(filePath).NotTo(BeEmpty())
				defer cleanup()

				// Verify file exists
				_, err = os.Stat(filePath)
				Expect(err).NotTo(HaveOccurred())

				// Verify content
				content, err := os.ReadFile(filePath)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(content)).To(Equal(promptContent))

				// Call cleanup
				cleanup()

				// Verify file is deleted
				_, err = os.Stat(filePath)
				Expect(os.IsNotExist(err)).To(BeTrue())
			})
		})

		Context("with special characters", func() {
			It("handles special characters correctly", func() {
				promptContent := "# Test with `backticks`\n\nAnd \"quotes\" and $variables"

				filePath, cleanup, err := executor.CreatePromptTempFile(ctx, promptContent)
				Expect(err).NotTo(HaveOccurred())
				defer cleanup()

				content, err := os.ReadFile(filePath)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(content)).To(Equal(promptContent))
			})
		})

		Context("with empty content", func() {
			It("creates empty file", func() {
				promptContent := ""

				filePath, cleanup, err := executor.CreatePromptTempFile(ctx, promptContent)
				Expect(err).NotTo(HaveOccurred())
				defer cleanup()

				content, err := os.ReadFile(filePath)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(content)).To(Equal(""))
			})
		})
	})

	Describe("buildDockerCommand", func() {
		It("builds correct docker command", func() {
			containerName := "test-container"
			promptFilePath := "/tmp/prompt.md"
			projectRoot := "/workspace"
			home := "/home/user"

			cmd := executor.BuildDockerCommand(
				ctx,
				containerName,
				promptFilePath,
				projectRoot,
				home,
			)

			Expect(cmd).NotTo(BeNil())
			Expect(cmd.Args).To(ContainElement("docker"))
			Expect(cmd.Args).To(ContainElement("run"))
			Expect(cmd.Args).To(ContainElement("--rm"))
			Expect(cmd.Args).To(ContainElement("--name"))
			Expect(cmd.Args).To(ContainElement(containerName))
			Expect(cmd.Args).To(ContainElement("-v"))
			Expect(cmd.Args).To(ContainElement(promptFilePath + ":/tmp/prompt.md:ro"))
			Expect(cmd.Args).To(ContainElement(projectRoot + ":/workspace"))
			Expect(cmd.Args).To(ContainElement(home + "/.claude-yolo:/home/node/.claude"))
			Expect(cmd.Args).To(ContainElement("docker.io/bborbe/claude-yolo:latest"))
		})

		It("includes network capabilities", func() {
			cmd := executor.BuildDockerCommand(ctx, "test", "/tmp/test", "/workspace", "/home/user")

			Expect(cmd.Args).To(ContainElement("--cap-add=NET_ADMIN"))
			Expect(cmd.Args).To(ContainElement("--cap-add=NET_RAW"))
		})

		It("includes YOLO_PROMPT_FILE environment variable", func() {
			cmd := executor.BuildDockerCommand(ctx, "test", "/tmp/test", "/workspace", "/home/user")

			Expect(cmd.Args).To(ContainElement("-e"))
			Expect(cmd.Args).To(ContainElement("YOLO_PROMPT_FILE=/tmp/prompt.md"))
		})

		It("sets correct context", func() {
			testCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			cmd := executor.BuildDockerCommand(
				testCtx,
				"test",
				"/tmp/test",
				"/workspace",
				"/home/user",
			)

			Expect(cmd).NotTo(BeNil())
			// Verify command has context set (can't directly check but can verify command was created)
			Expect(cmd.Path).NotTo(BeEmpty())
		})
	})

	Describe("Helper function integration", func() {
		It("prepareLogFile and createPromptTempFile work together", func() {
			logFile := filepath.Join(tempDir, "integration-test.log")
			promptContent := "# Integration Test\n\nThis tests helper functions together."

			// Prepare log file
			logHandle, err := executor.PrepareLogFile(ctx, logFile)
			Expect(err).NotTo(HaveOccurred())
			defer logHandle.Close()

			// Create prompt temp file
			promptPath, cleanup, err := executor.CreatePromptTempFile(ctx, promptContent)
			Expect(err).NotTo(HaveOccurred())
			defer cleanup()

			// Verify both exist
			_, err = os.Stat(logFile)
			Expect(err).NotTo(HaveOccurred())

			_, err = os.Stat(promptPath)
			Expect(err).NotTo(HaveOccurred())

			// Verify can write to log file
			_, err = logHandle.WriteString("Test log entry\n")
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("createPromptTempFile multiple calls", func() {
		It("creates unique files for each call", func() {
			content1 := "# Prompt 1"
			content2 := "# Prompt 2"

			path1, cleanup1, err := executor.CreatePromptTempFile(ctx, content1)
			Expect(err).NotTo(HaveOccurred())
			defer cleanup1()

			path2, cleanup2, err := executor.CreatePromptTempFile(ctx, content2)
			Expect(err).NotTo(HaveOccurred())
			defer cleanup2()

			// Paths should be different
			Expect(path1).NotTo(Equal(path2))

			// Both files should exist
			_, err = os.Stat(path1)
			Expect(err).NotTo(HaveOccurred())

			_, err = os.Stat(path2)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("prepareLogFile nested directories", func() {
		It("creates nested directories", func() {
			logFile := filepath.Join(tempDir, "level1", "level2", "level3", "test.log")

			file, err := executor.PrepareLogFile(ctx, logFile)
			Expect(err).NotTo(HaveOccurred())
			defer file.Close()

			// Verify all directories were created
			_, err = os.Stat(filepath.Join(tempDir, "level1"))
			Expect(err).NotTo(HaveOccurred())

			_, err = os.Stat(filepath.Join(tempDir, "level1", "level2"))
			Expect(err).NotTo(HaveOccurred())

			_, err = os.Stat(filepath.Join(tempDir, "level1", "level2", "level3"))
			Expect(err).NotTo(HaveOccurred())

			_, err = os.Stat(logFile)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
