// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runner_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/runner"
)

// writePromptFile writes a minimal prompt .md file with given status and container name.
func writePromptFile(dir, name, status, container string) string {
	content := fmt.Sprintf(
		"---\nstatus: %s\ncontainer: %s\n---\n\nPrompt body.\n",
		status,
		container,
	)
	path := filepath.Join(dir, name)
	Expect(os.WriteFile(path, []byte(content), 0600)).To(Succeed())
	return path
}

// writePromptFileWithStarted writes a prompt .md file with status, container, and started fields.
func writePromptFileWithStarted(dir, name, status, container, started string) string {
	content := fmt.Sprintf(
		"---\nstatus: %s\ncontainer: %s\nstarted: %s\n---\n\nPrompt body.\n",
		status,
		container,
		started,
	)
	path := filepath.Join(dir, name)
	Expect(os.WriteFile(path, []byte(content), 0600)).To(Succeed())
	return path
}

// writeSpecFile writes a minimal spec .md file with given status.
func writeSpecFile(dir, name, status string) string {
	content := fmt.Sprintf("---\nstatus: %s\n---\n\nSpec body.\n", status)
	path := filepath.Join(dir, name)
	Expect(os.WriteFile(path, []byte(content), 0600)).To(Succeed())
	return path
}

var _ = Describe("CheckExecutingPrompts", func() {
	var (
		tempDir         string
		inProgressDir   string
		checker         *mocks.ContainerChecker
		mgr             *mocks.RunnerPromptManager
		n               *mocks.Notifier
		ctx             context.Context
		cancel          context.CancelFunc
		currentDateTime libtime.CurrentDateTimeGetter
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "health-check-test-*")
		Expect(err).NotTo(HaveOccurred())

		inProgressDir = filepath.Join(tempDir, "in-progress")
		Expect(os.MkdirAll(inProgressDir, 0750)).To(Succeed())

		checker = &mocks.ContainerChecker{}
		mgr = &mocks.RunnerPromptManager{}
		n = &mocks.Notifier{}
		currentDateTime = libtime.NewCurrentDateTime()

		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() {
		cancel()
		_ = os.RemoveAll(tempDir)
	})

	It("returns nil when dir is empty (no prompts)", func() {
		err := runner.CheckExecutingPromptsForTest(
			ctx,
			inProgressDir,
			checker,
			mgr,
			n,
			"",
			0,
			nil,
			currentDateTime,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(checker.IsRunningCallCount()).To(Equal(0))
		Expect(n.NotifyCallCount()).To(Equal(0))
	})

	It("returns nil when dir does not exist", func() {
		err := runner.CheckExecutingPromptsForTest(
			ctx,
			"/nonexistent/path",
			checker,
			mgr,
			n,
			"",
			0,
			nil,
			currentDateTime,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(checker.IsRunningCallCount()).To(Equal(0))
	})

	It("skips directories inside inProgressDir", func() {
		subDir := filepath.Join(inProgressDir, "subdir")
		Expect(os.MkdirAll(subDir, 0750)).To(Succeed())

		err := runner.CheckExecutingPromptsForTest(
			ctx,
			inProgressDir,
			checker,
			mgr,
			n,
			"",
			0,
			nil,
			currentDateTime,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(checker.IsRunningCallCount()).To(Equal(0))
	})

	It("skips non-.md files", func() {
		Expect(
			os.WriteFile(filepath.Join(inProgressDir, "notes.txt"), []byte("text"), 0600),
		).To(Succeed())

		err := runner.CheckExecutingPromptsForTest(
			ctx,
			inProgressDir,
			checker,
			mgr,
			n,
			"",
			0,
			nil,
			currentDateTime,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(checker.IsRunningCallCount()).To(Equal(0))
	})

	It("skips prompts not in executing state", func() {
		path := writePromptFile(inProgressDir, "001-draft.md", "approved", "")
		pf := prompt.NewPromptFile(
			path,
			prompt.Frontmatter{Status: "approved"},
			nil,
			currentDateTime,
		)
		mgr.LoadReturns(pf, nil)

		err := runner.CheckExecutingPromptsForTest(
			ctx,
			inProgressDir,
			checker,
			mgr,
			n,
			"",
			0,
			nil,
			currentDateTime,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(checker.IsRunningCallCount()).To(Equal(0))
	})

	It("does not reset when container is running", func() {
		path := writePromptFile(inProgressDir, "001-running.md", "executing", "my-container")
		pf := prompt.NewPromptFile(
			path,
			prompt.Frontmatter{Status: "executing", Container: "my-container"},
			nil,
			currentDateTime,
		)
		mgr.LoadReturns(pf, nil)
		checker.IsRunningReturns(true, nil)

		err := runner.CheckExecutingPromptsForTest(
			ctx,
			inProgressDir,
			checker,
			mgr,
			n,
			"proj",
			0,
			nil,
			currentDateTime,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(checker.IsRunningCallCount()).To(Equal(1))
		Expect(n.NotifyCallCount()).To(Equal(0))
	})

	It("resets and notifies when container is not running", func() {
		path := writePromptFile(inProgressDir, "001-gone.md", "executing", "gone-container")
		pf := prompt.NewPromptFile(
			path,
			prompt.Frontmatter{Status: "executing", Container: "gone-container"},
			nil,
			currentDateTime,
		)
		mgr.LoadReturns(pf, nil)
		checker.IsRunningReturns(false, nil)
		n.NotifyReturns(nil)

		err := runner.CheckExecutingPromptsForTest(
			ctx,
			inProgressDir,
			checker,
			mgr,
			n,
			"proj",
			0,
			nil,
			currentDateTime,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(checker.IsRunningCallCount()).To(Equal(1))
		Expect(n.NotifyCallCount()).To(Equal(1))
		_, event := n.NotifyArgsForCall(0)
		Expect(event.EventType).To(Equal("stuck_container"))
		Expect(event.PromptName).To(Equal("001-gone.md"))

		// Verify file was reset to approved
		content, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(content)).To(ContainSubstring("status: approved"))
	})

	It("does not reset when IsRunning returns error (graceful degradation)", func() {
		path := writePromptFile(inProgressDir, "001-error.md", "executing", "err-container")
		pf := prompt.NewPromptFile(
			path,
			prompt.Frontmatter{Status: "executing", Container: "err-container"},
			nil,
			currentDateTime,
		)
		mgr.LoadReturns(pf, nil)
		checker.IsRunningReturns(false, fmt.Errorf("docker API error"))

		err := runner.CheckExecutingPromptsForTest(
			ctx,
			inProgressDir,
			checker,
			mgr,
			n,
			"proj",
			0,
			nil,
			currentDateTime,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(n.NotifyCallCount()).To(Equal(0))

		// File should NOT be reset
		content, _ := os.ReadFile(path)
		Expect(string(content)).To(ContainSubstring("status: executing"))
	})

	It("resets only the dead container when two executing prompts exist", func() {
		// Two prompt files: one alive, one dead
		pathAlive := writePromptFile(inProgressDir, "001-alive.md", "executing", "alive-container")
		pathDead := writePromptFile(inProgressDir, "002-dead.md", "executing", "dead-container")

		pfAlive := prompt.NewPromptFile(
			pathAlive,
			prompt.Frontmatter{Status: "executing", Container: "alive-container"},
			nil,
			currentDateTime,
		)
		pfDead := prompt.NewPromptFile(
			pathDead,
			prompt.Frontmatter{Status: "executing", Container: "dead-container"},
			nil,
			currentDateTime,
		)

		mgr.LoadStub = func(ctx context.Context, path string) (*prompt.PromptFile, error) {
			if filepath.Base(path) == "001-alive.md" {
				return pfAlive, nil
			}
			return pfDead, nil
		}
		checker.IsRunningStub = func(ctx context.Context, name string) (bool, error) {
			return name == "alive-container", nil
		}
		n.NotifyReturns(nil)

		err := runner.CheckExecutingPromptsForTest(
			ctx,
			inProgressDir,
			checker,
			mgr,
			n,
			"proj",
			0,
			nil,
			currentDateTime,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(n.NotifyCallCount()).To(Equal(1))

		// alive file should still be executing
		aliveContent, _ := os.ReadFile(pathAlive)
		Expect(string(aliveContent)).To(ContainSubstring("status: executing"))

		// dead file should be reset to approved
		deadContent, _ := os.ReadFile(pathDead)
		Expect(string(deadContent)).To(ContainSubstring("status: approved"))
	})

	Context("timeout enforcement", func() {
		var (
			stopper        *mocks.ContainerStopper
			fixedNow       time.Time
			fixedCurrentDT libtime.CurrentDateTime
		)

		BeforeEach(func() {
			stopper = &mocks.ContainerStopper{}
			stopper.StopContainerReturns(nil)
			n.NotifyReturns(nil)

			// Fixed "now" for deterministic tests
			fixedNow = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
			fixedCurrentDT = libtime.NewCurrentDateTime()
			fixedCurrentDT.SetNow(libtime.DateTime(fixedNow))
		})

		It("stops and marks failed when prompt exceeds maxPromptDuration", func() {
			// started 2 hours before fixedNow
			startedTime := fixedNow.Add(-2 * time.Hour)
			started := startedTime.UTC().Format(time.RFC3339)

			path := writePromptFileWithStarted(
				inProgressDir,
				"001-timeout.md",
				"executing",
				"timeout-container",
				started,
			)
			pf := prompt.NewPromptFile(
				path,
				prompt.Frontmatter{
					Status:    "executing",
					Container: "timeout-container",
					Started:   started,
				},
				nil,
				fixedCurrentDT,
			)
			mgr.LoadReturns(pf, nil)
			checker.IsRunningReturns(true, nil)

			err := runner.CheckExecutingPromptsForTest(
				ctx,
				inProgressDir,
				checker,
				mgr,
				n,
				"proj",
				1*time.Hour,
				stopper,
				fixedCurrentDT,
			)
			Expect(err).NotTo(HaveOccurred())

			// Stopper should have been called
			Expect(stopper.StopContainerCallCount()).To(Equal(1))
			_, stoppedContainer := stopper.StopContainerArgsForCall(0)
			Expect(stoppedContainer).To(Equal("timeout-container"))

			// Prompt should be failed
			content, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("status: failed"))
			Expect(string(content)).To(ContainSubstring("exceeded maxPromptDuration"))

			// Notification should have been sent
			Expect(n.NotifyCallCount()).To(Equal(1))
			_, event := n.NotifyArgsForCall(0)
			Expect(event.EventType).To(Equal("prompt_timeout"))
			Expect(event.PromptName).To(Equal("001-timeout.md"))
		})

		It("does not stop when maxPromptDuration is 0 (disabled)", func() {
			// started 2 hours before fixedNow
			startedTime := fixedNow.Add(-2 * time.Hour)
			started := startedTime.UTC().Format(time.RFC3339)

			path := writePromptFileWithStarted(
				inProgressDir,
				"001-noduration.md",
				"executing",
				"noduration-container",
				started,
			)
			pf := prompt.NewPromptFile(
				path,
				prompt.Frontmatter{
					Status:    "executing",
					Container: "noduration-container",
					Started:   started,
				},
				nil,
				fixedCurrentDT,
			)
			mgr.LoadReturns(pf, nil)
			checker.IsRunningReturns(true, nil)

			err := runner.CheckExecutingPromptsForTest(
				ctx,
				inProgressDir,
				checker,
				mgr,
				n,
				"proj",
				0,
				stopper,
				fixedCurrentDT,
			)
			Expect(err).NotTo(HaveOccurred())

			// Stopper should NOT have been called
			Expect(stopper.StopContainerCallCount()).To(Equal(0))

			// Prompt should still be executing
			content, _ := os.ReadFile(path)
			Expect(string(content)).To(ContainSubstring("status: executing"))
		})

		It("does not stop when prompt has no started timestamp", func() {
			path := writePromptFile(
				inProgressDir,
				"001-nostarted.md",
				"executing",
				"nostarted-container",
			)
			pf := prompt.NewPromptFile(
				path,
				prompt.Frontmatter{Status: "executing", Container: "nostarted-container"},
				nil,
				fixedCurrentDT,
			)
			mgr.LoadReturns(pf, nil)
			checker.IsRunningReturns(true, nil)

			err := runner.CheckExecutingPromptsForTest(
				ctx,
				inProgressDir,
				checker,
				mgr,
				n,
				"proj",
				1*time.Hour,
				stopper,
				fixedCurrentDT,
			)
			Expect(err).NotTo(HaveOccurred())

			// Stopper should NOT have been called
			Expect(stopper.StopContainerCallCount()).To(Equal(0))

			// Prompt should still be executing
			content, _ := os.ReadFile(path)
			Expect(string(content)).To(ContainSubstring("status: executing"))
		})

		It("does not stop when prompt is within maxPromptDuration", func() {
			// started 10 minutes before fixedNow
			startedTime := fixedNow.Add(-10 * time.Minute)
			started := startedTime.UTC().Format(time.RFC3339)

			path := writePromptFileWithStarted(
				inProgressDir,
				"001-recent.md",
				"executing",
				"recent-container",
				started,
			)
			pf := prompt.NewPromptFile(
				path,
				prompt.Frontmatter{
					Status:    "executing",
					Container: "recent-container",
					Started:   started,
				},
				nil,
				fixedCurrentDT,
			)
			mgr.LoadReturns(pf, nil)
			checker.IsRunningReturns(true, nil)

			err := runner.CheckExecutingPromptsForTest(
				ctx,
				inProgressDir,
				checker,
				mgr,
				n,
				"proj",
				1*time.Hour,
				stopper,
				fixedCurrentDT,
			)
			Expect(err).NotTo(HaveOccurred())

			// Stopper should NOT have been called
			Expect(stopper.StopContainerCallCount()).To(Equal(0))

			// Prompt should still be executing
			content, _ := os.ReadFile(path)
			Expect(string(content)).To(ContainSubstring("status: executing"))
		})
	})
})

var _ = Describe("CheckGeneratingSpecs", func() {
	var (
		tempDir            string
		specsInProgressDir string
		checker            *mocks.ContainerChecker
		ctx                context.Context
		cancel             context.CancelFunc
		currentDateTime    libtime.CurrentDateTimeGetter
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "health-check-spec-test-*")
		Expect(err).NotTo(HaveOccurred())

		specsInProgressDir = filepath.Join(tempDir, "specs-in-progress")
		Expect(os.MkdirAll(specsInProgressDir, 0750)).To(Succeed())

		checker = &mocks.ContainerChecker{}
		currentDateTime = libtime.NewCurrentDateTime()

		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() {
		cancel()
		_ = os.RemoveAll(tempDir)
	})

	It("returns nil when dir is empty", func() {
		err := runner.CheckGeneratingSpecsForTest(ctx, specsInProgressDir, checker, currentDateTime)
		Expect(err).NotTo(HaveOccurred())
		Expect(checker.IsRunningCallCount()).To(Equal(0))
	})

	It("returns nil when dir does not exist", func() {
		err := runner.CheckGeneratingSpecsForTest(
			ctx,
			"/nonexistent/specs",
			checker,
			currentDateTime,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(checker.IsRunningCallCount()).To(Equal(0))
	})

	It("skips non-.md files", func() {
		Expect(
			os.WriteFile(filepath.Join(specsInProgressDir, "notes.txt"), []byte("text"), 0600),
		).To(Succeed())

		err := runner.CheckGeneratingSpecsForTest(ctx, specsInProgressDir, checker, currentDateTime)
		Expect(err).NotTo(HaveOccurred())
		Expect(checker.IsRunningCallCount()).To(Equal(0))
	})

	It("skips spec not in generating state", func() {
		writeSpecFile(specsInProgressDir, "001-myspec.md", "approved")

		err := runner.CheckGeneratingSpecsForTest(ctx, specsInProgressDir, checker, currentDateTime)
		Expect(err).NotTo(HaveOccurred())
		Expect(checker.IsRunningCallCount()).To(Equal(0))
	})

	It("does not reset when spec container is running", func() {
		writeSpecFile(specsInProgressDir, "001-myspec.md", "generating")
		checker.IsRunningReturns(true, nil)

		err := runner.CheckGeneratingSpecsForTest(ctx, specsInProgressDir, checker, currentDateTime)
		Expect(err).NotTo(HaveOccurred())
		Expect(checker.IsRunningCallCount()).To(Equal(1))

		content, _ := os.ReadFile(filepath.Join(specsInProgressDir, "001-myspec.md"))
		Expect(string(content)).To(ContainSubstring("status: generating"))
	})

	It("resets spec to approved when container is gone", func() {
		path := writeSpecFile(specsInProgressDir, "001-myspec.md", "generating")
		checker.IsRunningReturns(false, nil)

		err := runner.CheckGeneratingSpecsForTest(ctx, specsInProgressDir, checker, currentDateTime)
		Expect(err).NotTo(HaveOccurred())
		Expect(checker.IsRunningCallCount()).To(Equal(1))

		// Verify container name derived correctly
		_, containerName := checker.IsRunningArgsForCall(0)
		Expect(containerName).To(Equal("dark-factory-gen-001-myspec"))

		content, _ := os.ReadFile(path)
		Expect(string(content)).To(ContainSubstring("status: approved"))
	})

	It("does not reset when IsRunning returns error (graceful degradation)", func() {
		path := writeSpecFile(specsInProgressDir, "001-myspec.md", "generating")
		checker.IsRunningReturns(false, fmt.Errorf("docker error"))

		err := runner.CheckGeneratingSpecsForTest(ctx, specsInProgressDir, checker, currentDateTime)
		Expect(err).NotTo(HaveOccurred())

		content, _ := os.ReadFile(path)
		Expect(string(content)).To(ContainSubstring("status: generating"))
	})
})

var _ = Describe("RunHealthCheckLoop", func() {
	var (
		tempDir         string
		inProgressDir   string
		specsDir        string
		checker         *mocks.ContainerChecker
		mgr             *mocks.RunnerPromptManager
		ctx             context.Context
		cancel          context.CancelFunc
		currentDateTime libtime.CurrentDateTimeGetter
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "health-loop-test-*")
		Expect(err).NotTo(HaveOccurred())

		inProgressDir = filepath.Join(tempDir, "in-progress")
		specsDir = filepath.Join(tempDir, "specs-in-progress")
		Expect(os.MkdirAll(inProgressDir, 0750)).To(Succeed())
		Expect(os.MkdirAll(specsDir, 0750)).To(Succeed())

		checker = &mocks.ContainerChecker{}
		mgr = &mocks.RunnerPromptManager{}
		currentDateTime = libtime.NewCurrentDateTime()

		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() {
		cancel()
		_ = os.RemoveAll(tempDir)
	})

	It("stops cleanly when context is cancelled", func() {
		errCh := make(chan error, 1)
		go func() {
			errCh <- runner.RunHealthCheckLoopForTest(ctx, 50*time.Millisecond, inProgressDir, specsDir, checker, mgr, nil, "", currentDateTime, 0, nil)
		}()

		// Cancel context quickly
		time.Sleep(20 * time.Millisecond)
		cancel()

		Eventually(errCh, 2*time.Second).Should(Receive(BeNil()))
	})

	It("calls checks on each tick", func() {
		// No files so checks are no-ops
		mgr.LoadReturns(nil, nil)

		errCh := make(chan error, 1)
		go func() {
			errCh <- runner.RunHealthCheckLoopForTest(ctx, 50*time.Millisecond, inProgressDir, specsDir, checker, mgr, nil, "", currentDateTime, 0, nil)
		}()

		// Wait for at least one tick
		time.Sleep(120 * time.Millisecond)
		cancel()

		Eventually(errCh, 2*time.Second).Should(Receive(BeNil()))
		// With empty dirs, IsRunning should never be called
		Expect(checker.IsRunningCallCount()).To(Equal(0))
	})
})
