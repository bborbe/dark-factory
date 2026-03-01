// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package executor

import (
	"context"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

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

	Describe("buildDockerCommand", func() {
		It("builds correct docker command", func() {
			cmd := buildDockerCommand(
				ctx,
				"test-container",
				"/tmp/prompt.md",
				"/workspace",
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
			cmd := buildDockerCommand(ctx, "test", "/tmp/test.md", "/workspace", "/home/user")

			Expect(cmd.Args).To(ContainElement("/tmp/test.md:/tmp/prompt.md:ro"))
			Expect(cmd.Args).To(ContainElement("/workspace:/workspace"))
			Expect(cmd.Args).To(ContainElement("/home/user/.claude-yolo:/home/node/.claude"))
			Expect(cmd.Args).To(ContainElement("/home/user/go/pkg:/home/node/go/pkg"))
		})

		It("includes network capabilities", func() {
			cmd := buildDockerCommand(ctx, "test", "/tmp/test", "/workspace", "/home/user")

			Expect(cmd.Args).To(ContainElement("--cap-add=NET_ADMIN"))
			Expect(cmd.Args).To(ContainElement("--cap-add=NET_RAW"))
		})

		It("includes environment variable", func() {
			cmd := buildDockerCommand(ctx, "test", "/tmp/test", "/workspace", "/home/user")

			Expect(cmd.Args).To(ContainElement("-e"))
			Expect(cmd.Args).To(ContainElement("YOLO_PROMPT_FILE=/tmp/prompt.md"))
		})

		It("sets correct context", func() {
			testCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			cmd := buildDockerCommand(testCtx, "test", "/tmp/test", "/workspace", "/home/user")

			Expect(cmd).NotTo(BeNil())
			Expect(cmd.Path).NotTo(BeEmpty())
		})
	})
})
