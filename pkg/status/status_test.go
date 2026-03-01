// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package status_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/status"
)

var _ = Describe("StatusChecker", func() {
	var (
		ctx           context.Context
		tempDir       string
		queueDir      string
		completedDir  string
		ideasDir      string
		mockPromptMgr *mocks.Manager
		statusChecker status.Checker
	)

	BeforeEach(func() {
		ctx = context.Background()

		var err error
		tempDir, err = os.MkdirTemp("", "status-test-*")
		Expect(err).NotTo(HaveOccurred())

		queueDir = filepath.Join(tempDir, "prompts")
		completedDir = filepath.Join(tempDir, "prompts", "completed")
		ideasDir = filepath.Join(tempDir, "prompts", "ideas")

		err = os.MkdirAll(queueDir, 0750)
		Expect(err).NotTo(HaveOccurred())
		err = os.MkdirAll(completedDir, 0750)
		Expect(err).NotTo(HaveOccurred())
		err = os.MkdirAll(ideasDir, 0750)
		Expect(err).NotTo(HaveOccurred())

		mockPromptMgr = &mocks.Manager{}
		statusChecker = status.NewChecker(queueDir, completedDir, ideasDir, mockPromptMgr)
	})

	AfterEach(func() {
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	Describe("GetStatus", func() {
		It("returns status with no executing prompt", func() {
			mockPromptMgr.HasExecutingReturns(false)
			mockPromptMgr.ListQueuedReturns([]prompt.Prompt{}, nil)

			st, err := statusChecker.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(st.Daemon).To(Equal("not running"))
			Expect(st.CurrentPrompt).To(BeEmpty())
			Expect(st.QueueCount).To(Equal(0))
			Expect(st.CompletedCount).To(Equal(0))
			Expect(st.IdeasCount).To(Equal(0))
		})

		It("returns status with queued prompts", func() {
			mockPromptMgr.HasExecutingReturns(false)
			mockPromptMgr.ListQueuedReturns([]prompt.Prompt{
				{Path: filepath.Join(queueDir, "001-test.md"), Status: prompt.StatusQueued},
				{Path: filepath.Join(queueDir, "002-another.md"), Status: prompt.StatusQueued},
			}, nil)

			st, err := statusChecker.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(st.QueueCount).To(Equal(2))
			Expect(st.QueuedPrompts).To(ConsistOf("001-test.md", "002-another.md"))
		})

		It("counts completed prompts", func() {
			mockPromptMgr.HasExecutingReturns(false)
			mockPromptMgr.ListQueuedReturns([]prompt.Prompt{}, nil)

			// Create completed prompt files
			err := os.WriteFile(filepath.Join(completedDir, "001-done.md"), []byte("done"), 0600)
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(
				filepath.Join(completedDir, "002-also-done.md"),
				[]byte("done"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			st, err := statusChecker.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(st.CompletedCount).To(Equal(2))
		})

		It("counts ideas", func() {
			mockPromptMgr.HasExecutingReturns(false)
			mockPromptMgr.ListQueuedReturns([]prompt.Prompt{}, nil)

			// Create idea files
			err := os.WriteFile(filepath.Join(ideasDir, "idea1.md"), []byte("idea"), 0600)
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(filepath.Join(ideasDir, "idea2.md"), []byte("idea"), 0600)
			Expect(err).NotTo(HaveOccurred())

			st, err := statusChecker.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(st.IdeasCount).To(Equal(2))
		})

		It("handles missing ideas directory", func() {
			// Remove ideas directory
			err := os.RemoveAll(ideasDir)
			Expect(err).NotTo(HaveOccurred())

			mockPromptMgr.HasExecutingReturns(false)
			mockPromptMgr.ListQueuedReturns([]prompt.Prompt{}, nil)

			st, err := statusChecker.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(st.IdeasCount).To(Equal(0))
		})

		It("includes executing prompt info", func() {
			// Create an executing prompt
			execPath := filepath.Join(queueDir, "003-executing.md")
			execContent := `---
status: executing
container: dark-factory-003-executing
---
# Test
`
			err := os.WriteFile(execPath, []byte(execContent), 0600)
			Expect(err).NotTo(HaveOccurred())

			mockPromptMgr.HasExecutingReturns(true)
			mockPromptMgr.ReadFrontmatterReturns(&prompt.Frontmatter{
				Status:    "executing",
				Container: "dark-factory-003-executing",
			}, nil)
			mockPromptMgr.ListQueuedReturns([]prompt.Prompt{}, nil)

			st, err := statusChecker.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(st.CurrentPrompt).To(Equal("003-executing.md"))
			Expect(st.Container).To(Equal("dark-factory-003-executing"))
			Expect(st.ExecutingSince).NotTo(BeEmpty())
		})

		It("includes executing prompt with empty container name", func() {
			// Create an executing prompt without container
			execPath := filepath.Join(queueDir, "004-executing.md")
			execContent := `---
status: executing
---
# Test
`
			err := os.WriteFile(execPath, []byte(execContent), 0600)
			Expect(err).NotTo(HaveOccurred())

			mockPromptMgr.HasExecutingReturns(true)
			mockPromptMgr.ReadFrontmatterReturns(&prompt.Frontmatter{
				Status:    "executing",
				Container: "",
			}, nil)
			mockPromptMgr.ListQueuedReturns([]prompt.Prompt{}, nil)

			st, err := statusChecker.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(st.CurrentPrompt).To(Equal("004-executing.md"))
			Expect(st.Container).To(BeEmpty())
			Expect(st.ContainerRunning).To(BeFalse())
		})
	})

	Describe("GetQueuedPrompts", func() {
		It("returns queued prompts with metadata", func() {
			queuedPath1 := filepath.Join(queueDir, "001-test.md")
			queuedPath2 := filepath.Join(queueDir, "002-another.md")

			mockPromptMgr.ListQueuedReturns([]prompt.Prompt{
				{Path: queuedPath1, Status: prompt.StatusQueued},
				{Path: queuedPath2, Status: prompt.StatusQueued},
			}, nil)

			mockPromptMgr.TitleReturnsOnCall(0, "Test Prompt", nil)
			mockPromptMgr.TitleReturnsOnCall(1, "Another Prompt", nil)

			// Create files to get size
			err := os.WriteFile(queuedPath1, []byte("test content"), 0600)
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(queuedPath2, []byte("another content longer"), 0600)
			Expect(err).NotTo(HaveOccurred())

			queued, err := statusChecker.GetQueuedPrompts(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(queued).To(HaveLen(2))
			Expect(queued[0].Name).To(Equal("001-test.md"))
			Expect(queued[0].Title).To(Equal("Test Prompt"))
			Expect(queued[0].Size).To(BeNumerically(">", 0))
		})
	})

	Describe("GetStatus with log files", func() {
		It("includes latest log file information", func() {
			mockPromptMgr.HasExecutingReturns(false)
			mockPromptMgr.ListQueuedReturns([]prompt.Prompt{}, nil)

			// Create log directory and files
			logDir := filepath.Join(tempDir, "prompts", "log")
			err := os.MkdirAll(logDir, 0750)
			Expect(err).NotTo(HaveOccurred())

			// Create older log file
			err = os.WriteFile(filepath.Join(logDir, "001-old.log"), []byte("old log"), 0600)
			Expect(err).NotTo(HaveOccurred())

			time.Sleep(10 * time.Millisecond)

			// Create newer log file
			err = os.WriteFile(filepath.Join(logDir, "002-new.log"), []byte("newer log"), 0600)
			Expect(err).NotTo(HaveOccurred())

			// Create checker with log directory
			checkerWithLogs := status.NewCheckerWithOptions(
				queueDir,
				completedDir,
				ideasDir,
				logDir,
				8080,
				mockPromptMgr,
			)

			st, err := checkerWithLogs.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(st.LastLogFile).To(Equal(filepath.Join(logDir, "002-new.log")))
			Expect(st.LastLogSize).To(BeNumerically(">", 0))
		})

		It("handles missing log directory", func() {
			mockPromptMgr.HasExecutingReturns(false)
			mockPromptMgr.ListQueuedReturns([]prompt.Prompt{}, nil)

			// Create checker with non-existent log directory
			checkerWithLogs := status.NewCheckerWithOptions(
				queueDir,
				completedDir,
				ideasDir,
				"/nonexistent/log",
				8080,
				mockPromptMgr,
			)

			st, err := checkerWithLogs.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(st.LastLogFile).To(BeEmpty())
		})
	})

	Describe("GetCompletedPrompts", func() {
		It("returns completed prompts with limit", func() {
			// Create completed files with different mod times
			file1 := filepath.Join(completedDir, "001-old.md")
			file2 := filepath.Join(completedDir, "002-recent.md")

			err := os.WriteFile(file1, []byte("old"), 0600)
			Expect(err).NotTo(HaveOccurred())

			time.Sleep(10 * time.Millisecond)

			err = os.WriteFile(file2, []byte("recent"), 0600)
			Expect(err).NotTo(HaveOccurred())

			completed, err := statusChecker.GetCompletedPrompts(ctx, 10)
			Expect(err).NotTo(HaveOccurred())
			Expect(completed).To(HaveLen(2))

			// Most recent should be first
			Expect(completed[0].Name).To(Equal("002-recent.md"))
			Expect(completed[1].Name).To(Equal("001-old.md"))
		})

		It("respects limit parameter", func() {
			// Create 3 files
			for i := 1; i <= 3; i++ {
				file := filepath.Join(completedDir, "00"+string(rune('0'+i))+"-test.md")
				err := os.WriteFile(file, []byte("test"), 0600)
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(5 * time.Millisecond)
			}

			completed, err := statusChecker.GetCompletedPrompts(ctx, 2)
			Expect(err).NotTo(HaveOccurred())
			Expect(completed).To(HaveLen(2))
		})

		It("handles empty completed directory", func() {
			completed, err := statusChecker.GetCompletedPrompts(ctx, 10)
			Expect(err).NotTo(HaveOccurred())
			Expect(completed).To(HaveLen(0))
		})

		It("handles non-existent completed directory", func() {
			err := os.RemoveAll(completedDir)
			Expect(err).NotTo(HaveOccurred())

			completed, err := statusChecker.GetCompletedPrompts(ctx, 10)
			Expect(err).NotTo(HaveOccurred())
			Expect(completed).To(HaveLen(0))
		})
	})
})
