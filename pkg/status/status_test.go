// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package status_test

import (
	"context"
	stderrors "errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/status"
)

// newSubprocRunner returns a mock SubprocRunner that returns empty output and nil error by default.
func newSubprocRunner() *mocks.SubprocRunner {
	r := &mocks.SubprocRunner{}
	r.RunWithWarnAndTimeoutReturns([]byte{}, nil)
	r.RunWithWarnAndTimeoutDirReturns([]byte{}, nil)
	return r
}

var _ = Describe("StatusChecker", func() {
	var (
		ctx           context.Context
		tempDir       string
		queueDir      string
		completedDir  string
		lockFilePath  string
		promptMgr     *mocks.StatusPromptManager
		statusChecker status.Checker
	)

	BeforeEach(func() {
		ctx = context.Background()

		var err error
		tempDir, err = os.MkdirTemp("", "status-test-*")
		Expect(err).NotTo(HaveOccurred())

		queueDir = filepath.Join(tempDir, "prompts")
		completedDir = filepath.Join(tempDir, "prompts", "completed")
		lockFilePath = filepath.Join(tempDir, ".dark-factory.lock")

		err = os.MkdirAll(queueDir, 0750)
		Expect(err).NotTo(HaveOccurred())
		err = os.MkdirAll(completedDir, 0750)
		Expect(err).NotTo(HaveOccurred())

		promptMgr = &mocks.StatusPromptManager{}
		statusChecker = status.NewChecker(
			"",
			queueDir,
			completedDir,
			"prompts/log",
			lockFilePath,
			8080,
			promptMgr,
			nil,
			0,
			0,
			libtime.NewCurrentDateTime(),
			newSubprocRunner(),
		)
	})

	AfterEach(func() {
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	Describe("GetStatus", func() {
		It("returns status with no executing prompt", func() {
			promptMgr.HasExecutingReturns(false)
			promptMgr.ListQueuedReturns([]prompt.Prompt{}, nil)

			st, err := statusChecker.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(st.Daemon).To(Equal("not running"))
			Expect(st.CurrentPrompt).To(BeEmpty())
			Expect(st.QueueCount).To(Equal(0))
			Expect(st.CompletedCount).To(Equal(0))
		})

		It("returns status with queued prompts", func() {
			promptMgr.HasExecutingReturns(false)
			promptMgr.ListQueuedReturns([]prompt.Prompt{
				{Path: filepath.Join(queueDir, "001-test.md"), Status: prompt.ApprovedPromptStatus},
				{
					Path:   filepath.Join(queueDir, "002-another.md"),
					Status: prompt.ApprovedPromptStatus,
				},
			}, nil)

			st, err := statusChecker.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(st.QueueCount).To(Equal(2))
			Expect(st.QueuedPrompts).To(ConsistOf("001-test.md", "002-another.md"))
		})

		It("counts completed prompts", func() {
			promptMgr.HasExecutingReturns(false)
			promptMgr.ListQueuedReturns([]prompt.Prompt{}, nil)

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

			promptMgr.HasExecutingReturns(true)
			promptMgr.ReadFrontmatterReturns(&prompt.Frontmatter{
				Status:    "executing",
				Container: "dark-factory-003-executing",
				Started:   time.Now().Add(-5 * time.Minute).UTC().Format(time.RFC3339),
			}, nil)
			promptMgr.ListQueuedReturns([]prompt.Prompt{}, nil)

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

			promptMgr.HasExecutingReturns(true)
			promptMgr.ReadFrontmatterReturns(&prompt.Frontmatter{
				Status:    "executing",
				Container: "",
			}, nil)
			promptMgr.ListQueuedReturns([]prompt.Prompt{}, nil)

			st, err := statusChecker.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(st.CurrentPrompt).To(Equal("004-executing.md"))
			Expect(st.Container).To(BeEmpty())
			Expect(st.ContainerRunning).To(BeFalse())
		})
	})

	Describe("Daemon detection", func() {
		It("shows not running when lock file is missing", func() {
			promptMgr.HasExecutingReturns(false)
			promptMgr.ListQueuedReturns([]prompt.Prompt{}, nil)

			st, err := statusChecker.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(st.Daemon).To(Equal("not running"))
			Expect(st.DaemonPID).To(Equal(0))
		})

		It("shows not running when lock file contains dead PID", func() {
			// Write a PID that is guaranteed not to exist
			err := os.WriteFile(lockFilePath, []byte("99999999\n"), 0600)
			Expect(err).NotTo(HaveOccurred())

			promptMgr.HasExecutingReturns(false)
			promptMgr.ListQueuedReturns([]prompt.Prompt{}, nil)

			st, err := statusChecker.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(st.Daemon).To(Equal("not running"))
			Expect(st.DaemonPID).To(Equal(0))
		})

		It("shows running when lock file contains the current process PID", func() {
			pid := os.Getpid()
			err := os.WriteFile(lockFilePath, []byte(fmt.Sprintf("%d\n", pid)), 0600)
			Expect(err).NotTo(HaveOccurred())

			promptMgr.HasExecutingReturns(false)
			promptMgr.ListQueuedReturns([]prompt.Prompt{}, nil)

			st, err := statusChecker.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(st.Daemon).To(Equal("running"))
			Expect(st.DaemonPID).To(Equal(pid))
		})

		It("shows not running when lock file is empty", func() {
			err := os.WriteFile(lockFilePath, []byte(""), 0600)
			Expect(err).NotTo(HaveOccurred())

			promptMgr.HasExecutingReturns(false)
			promptMgr.ListQueuedReturns([]prompt.Prompt{}, nil)

			st, err := statusChecker.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(st.Daemon).To(Equal("not running"))
		})

		It("shows not running when lock file contains invalid content", func() {
			err := os.WriteFile(lockFilePath, []byte("not-a-pid\n"), 0600)
			Expect(err).NotTo(HaveOccurred())

			promptMgr.HasExecutingReturns(false)
			promptMgr.ListQueuedReturns([]prompt.Prompt{}, nil)

			st, err := statusChecker.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(st.Daemon).To(Equal("not running"))
		})
	})

	Describe("GetQueuedPrompts", func() {
		It("returns queued prompts with metadata", func() {
			queuedPath1 := filepath.Join(queueDir, "001-test.md")
			queuedPath2 := filepath.Join(queueDir, "002-another.md")

			promptMgr.ListQueuedReturns([]prompt.Prompt{
				{Path: queuedPath1, Status: prompt.ApprovedPromptStatus},
				{Path: queuedPath2, Status: prompt.ApprovedPromptStatus},
			}, nil)

			promptMgr.TitleReturnsOnCall(0, "Test Prompt", nil)
			promptMgr.TitleReturnsOnCall(1, "Another Prompt", nil)

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
			promptMgr.HasExecutingReturns(false)
			promptMgr.ListQueuedReturns([]prompt.Prompt{}, nil)

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
			checkerWithLogs := status.NewChecker(
				"",
				queueDir,
				completedDir,
				logDir,
				lockFilePath,
				8080,
				promptMgr,
				nil,
				0,
				0,
				libtime.NewCurrentDateTime(),
				newSubprocRunner(),
			)

			st, err := checkerWithLogs.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(st.LastLogFile).To(Equal(filepath.Join(logDir, "002-new.log")))
			Expect(st.LastLogSize).To(BeNumerically(">", 0))
		})

		It("handles missing log directory", func() {
			promptMgr.HasExecutingReturns(false)
			promptMgr.ListQueuedReturns([]prompt.Prompt{}, nil)

			// Create checker with non-existent log directory
			checkerWithLogs := status.NewChecker(
				"",
				queueDir,
				completedDir,
				"/nonexistent/log",
				lockFilePath,
				8080,
				promptMgr,
				nil,
				0,
				0,
				libtime.NewCurrentDateTime(),
				newSubprocRunner(),
			)

			st, err := checkerWithLogs.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(st.LastLogFile).To(BeEmpty())
		})
	})

	Describe("isContainerRunning via GetStatus", func() {
		It("returns false for container running check when container does not exist", func() {
			// Create an executing prompt with a nonexistent container name
			execPath := filepath.Join(queueDir, "005-executing.md")
			execContent := `---
status: executing
container: dark-factory-nonexistent-container-xyz
---
# Test
`
			err := os.WriteFile(execPath, []byte(execContent), 0600)
			Expect(err).NotTo(HaveOccurred())

			promptMgr.HasExecutingReturns(true)
			promptMgr.ReadFrontmatterReturns(&prompt.Frontmatter{
				Status:    "executing",
				Container: "dark-factory-nonexistent-container-xyz",
			}, nil)
			promptMgr.ListQueuedReturns([]prompt.Prompt{}, nil)

			st, err := statusChecker.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(st.CurrentPrompt).To(Equal("005-executing.md"))
			// Container not running — docker ps returns empty or docker not available
			Expect(st.ContainerRunning).To(BeFalse())
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

		It("handles files without frontmatter by using file mod time", func() {
			// Create file without frontmatter
			file1 := filepath.Join(completedDir, "001-no-frontmatter.md")
			err := os.WriteFile(
				file1,
				[]byte("# Old Prompt\n\nContent without frontmatter.\n"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			time.Sleep(10 * time.Millisecond)

			// Create file with frontmatter
			file2 := filepath.Join(completedDir, "002-with-frontmatter.md")
			content := "---\nstatus: completed\ncompleted: 2026-03-01T10:00:00Z\n---\n\n# New Prompt\n"
			err = os.WriteFile(file2, []byte(content), 0600)
			Expect(err).NotTo(HaveOccurred())

			completed, err := statusChecker.GetCompletedPrompts(ctx, 10)
			Expect(err).NotTo(HaveOccurred())
			Expect(completed).To(HaveLen(2))

			// Both files should be included
			names := []string{completed[0].Name, completed[1].Name}
			Expect(names).To(ContainElement("001-no-frontmatter.md"))
			Expect(names).To(ContainElement("002-with-frontmatter.md"))
		})

		It("handles files with empty frontmatter by using file mod time", func() {
			// Create file with empty frontmatter
			file := filepath.Join(completedDir, "001-empty-fm.md")
			err := os.WriteFile(file, []byte("---\n---\n\n# Prompt\n"), 0600)
			Expect(err).NotTo(HaveOccurred())

			completed, err := statusChecker.GetCompletedPrompts(ctx, 10)
			Expect(err).NotTo(HaveOccurred())
			Expect(completed).To(HaveLen(1))
			Expect(completed[0].Name).To(Equal("001-empty-fm.md"))
			Expect(completed[0].CompletedAt).NotTo(BeZero())
		})
	})

	Describe("GetStatus container count", func() {
		It("populates ContainerCount and ContainerMax when counter returns count", func() {
			counter := &mocks.ContainerCounter{}
			counter.CountRunningReturns(2, nil)

			checker := status.NewChecker(
				"",
				queueDir,
				completedDir,
				"prompts/log",
				lockFilePath,
				8080,
				promptMgr,
				counter,
				3,
				0,
				libtime.NewCurrentDateTime(),
				newSubprocRunner(),
			)

			promptMgr.HasExecutingReturns(false)
			promptMgr.ListQueuedReturns([]prompt.Prompt{}, nil)

			st, err := checker.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(st.ContainerCount).To(Equal(2))
			Expect(st.ContainerMax).To(Equal(3))
		})

		It("leaves ContainerCount zero when counter returns error", func() {
			counter := &mocks.ContainerCounter{}
			counter.CountRunningReturns(0, stderrors.New("docker not available"))

			checker := status.NewChecker(
				"",
				queueDir,
				completedDir,
				"prompts/log",
				lockFilePath,
				8080,
				promptMgr,
				counter,
				3,
				0,
				libtime.NewCurrentDateTime(),
				newSubprocRunner(),
			)

			promptMgr.HasExecutingReturns(false)
			promptMgr.ListQueuedReturns([]prompt.Prompt{}, nil)

			st, err := checker.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(st.ContainerCount).To(Equal(0))
		})

		It("leaves ContainerCount zero when maxContainers is 0", func() {
			counter := &mocks.ContainerCounter{}

			checker := status.NewChecker(
				"",
				queueDir,
				completedDir,
				"prompts/log",
				lockFilePath,
				8080,
				promptMgr,
				counter,
				0,
				0,
				libtime.NewCurrentDateTime(),
				newSubprocRunner(),
			)

			promptMgr.HasExecutingReturns(false)
			promptMgr.ListQueuedReturns([]prompt.Prompt{}, nil)

			st, err := checker.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(st.ContainerCount).To(Equal(0))
			Expect(counter.CountRunningCallCount()).To(Equal(0))
		})
	})

	Describe("GetStatus git warnings", func() {
		It("sets GitIndexLock true when .git/index.lock exists", func() {
			gitDir := filepath.Join(tempDir, ".git")
			Expect(os.MkdirAll(gitDir, 0750)).To(Succeed())
			Expect(
				os.WriteFile(filepath.Join(gitDir, "index.lock"), []byte(""), 0600),
			).To(Succeed())

			checkerWithProject := status.NewChecker(
				tempDir,
				queueDir,
				completedDir,
				"prompts/log",
				lockFilePath,
				8080,
				promptMgr,
				nil,
				0,
				0,
				libtime.NewCurrentDateTime(),
				newSubprocRunner(),
			)

			promptMgr.HasExecutingReturns(false)
			promptMgr.ListQueuedReturns([]prompt.Prompt{}, nil)

			st, err := checkerWithProject.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(st.GitIndexLock).To(BeTrue())
		})

		It("leaves GitIndexLock false when .git/index.lock does not exist", func() {
			checkerWithProject := status.NewChecker(
				tempDir,
				queueDir,
				completedDir,
				"prompts/log",
				lockFilePath,
				8080,
				promptMgr,
				nil,
				0,
				0,
				libtime.NewCurrentDateTime(),
				newSubprocRunner(),
			)

			promptMgr.HasExecutingReturns(false)
			promptMgr.ListQueuedReturns([]prompt.Prompt{}, nil)

			st, err := checkerWithProject.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(st.GitIndexLock).To(BeFalse())
		})

		It("reflects DirtyFileThreshold from checker config", func() {
			checkerWithThreshold := status.NewChecker(
				"",
				queueDir,
				completedDir,
				"prompts/log",
				lockFilePath,
				8080,
				promptMgr,
				nil,
				0,
				42,
				libtime.NewCurrentDateTime(),
				newSubprocRunner(),
			)

			promptMgr.HasExecutingReturns(false)
			promptMgr.ListQueuedReturns([]prompt.Prompt{}, nil)

			st, err := checkerWithThreshold.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(st.DirtyFileThreshold).To(Equal(42))
		})
	})

	Describe("GetStatus subprocess timeout (skipped) behavior", func() {
		It("sets DirtyFileCheckSkipped true when git status times out", func() {
			runner := &mocks.SubprocRunner{}
			runner.RunWithWarnAndTimeoutReturns([]byte{}, nil)
			runner.RunWithWarnAndTimeoutDirReturns(nil, context.DeadlineExceeded)

			checker := status.NewChecker(
				tempDir,
				queueDir,
				completedDir,
				"prompts/log",
				lockFilePath,
				8080,
				promptMgr,
				nil,
				0,
				0,
				libtime.NewCurrentDateTime(),
				runner,
			)

			promptMgr.HasExecutingReturns(false)
			promptMgr.ListQueuedReturns([]prompt.Prompt{}, nil)

			st, err := checker.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(st.DirtyFileCheckSkipped).To(BeTrue())
			Expect(st.DirtyFileCount).To(Equal(0))
		})

		It("sets GeneratingSpecSkipped true when docker ps for gen-spec times out", func() {
			runner := &mocks.SubprocRunner{}
			runner.RunWithWarnAndTimeoutReturns(nil, context.DeadlineExceeded)
			runner.RunWithWarnAndTimeoutDirReturns([]byte{}, nil)

			checker := status.NewChecker(
				"",
				queueDir,
				completedDir,
				"prompts/log",
				lockFilePath,
				8080,
				promptMgr,
				nil,
				0,
				0,
				libtime.NewCurrentDateTime(),
				runner,
			)

			promptMgr.HasExecutingReturns(false)
			promptMgr.ListQueuedReturns([]prompt.Prompt{}, nil)

			st, err := checker.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(st.GeneratingSpecSkipped).To(BeTrue())
		})

		It("sets ContainerRunningSkipped true when docker ps for container times out", func() {
			runner := &mocks.SubprocRunner{}
			runner.RunWithWarnAndTimeoutReturns(nil, context.DeadlineExceeded)
			runner.RunWithWarnAndTimeoutDirReturns([]byte{}, nil)

			execPath := filepath.Join(queueDir, "006-executing.md")
			execContent := `---
status: executing
container: dark-factory-006
---
# Test
`
			err := os.WriteFile(execPath, []byte(execContent), 0600)
			Expect(err).NotTo(HaveOccurred())

			promptMgr.HasExecutingReturns(true)
			promptMgr.ReadFrontmatterReturns(&prompt.Frontmatter{
				Status:    "executing",
				Container: "dark-factory-006",
			}, nil)
			promptMgr.ListQueuedReturns([]prompt.Prompt{}, nil)

			checker := status.NewChecker(
				"",
				queueDir,
				completedDir,
				"prompts/log",
				lockFilePath,
				8080,
				promptMgr,
				nil,
				0,
				0,
				libtime.NewCurrentDateTime(),
				runner,
			)

			st, err := checker.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(st.ContainerRunningSkipped).To(BeTrue())
			Expect(st.ContainerRunning).To(BeFalse())
		})

		It("returns status without error even when all subprocess calls time out", func() {
			runner := &mocks.SubprocRunner{}
			runner.RunWithWarnAndTimeoutReturns(nil, context.DeadlineExceeded)
			runner.RunWithWarnAndTimeoutDirReturns(nil, context.DeadlineExceeded)

			counter := &mocks.ContainerCounter{}
			counter.CountRunningReturns(0, context.DeadlineExceeded)

			checker := status.NewChecker(
				tempDir,
				queueDir,
				completedDir,
				"prompts/log",
				lockFilePath,
				8080,
				promptMgr,
				counter,
				3,
				0,
				libtime.NewCurrentDateTime(),
				runner,
			)

			promptMgr.HasExecutingReturns(false)
			promptMgr.ListQueuedReturns([]prompt.Prompt{}, nil)

			st, err := checker.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(st).NotTo(BeNil())
			Expect(st.DirtyFileCheckSkipped).To(BeTrue())
			Expect(st.GeneratingSpecSkipped).To(BeTrue())
			Expect(st.ContainerCountSkipped).To(BeTrue())
		})
	})
})
