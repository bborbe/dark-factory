// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package executor_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/executor"
)

var _ = Describe("DockerExecutor", func() {
	var (
		ctx     context.Context
		exec    *executor.DockerExecutor
		logFile string
		tempDir string
	)

	BeforeEach(func() {
		ctx = context.Background()
		exec = executor.NewDockerExecutor()

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
				err := exec.Execute(ctx, promptContent, logFile, "test-container")
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
				err := exec.Execute(ctx, promptContent, logFile, "test-container")
				Expect(err).NotTo(HaveOccurred())
			})

			It("handles quotes in prompt", func() {
				Skip("requires Docker and claude-yolo image")

				promptContent := `# Test with "quotes" and 'single quotes'`
				err := exec.Execute(ctx, promptContent, logFile, "test-container")
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
				err := exec.Execute(ctx, promptContent, logFile, "test-container")
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
				err := exec.Execute(ctx, promptContent, logFile, "test-container")
				Expect(err).NotTo(HaveOccurred())
			})

			It("handles newlines and multiline content", func() {
				Skip("requires Docker and claude-yolo image")

				promptContent := `Line 1
Line 2

Line 4 (with blank line)

More lines...`
				err := exec.Execute(ctx, promptContent, logFile, "test-container")
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("with invalid log file path", func() {
			It("returns error when log directory cannot be created", func() {
				invalidLogFile := "/invalid/path/that/does/not/exist/test.log"
				err := exec.Execute(ctx, "test prompt", invalidLogFile, "test-container")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("create log directory"))
			})
		})

		Context("when context is cancelled", func() {
			It("returns error", func() {
				Skip("requires Docker and claude-yolo image")

				cancelCtx, cancel := context.WithCancel(ctx)
				cancel() // Cancel immediately

				err := exec.Execute(cancelCtx, "test prompt", logFile, "test-container")
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
})
