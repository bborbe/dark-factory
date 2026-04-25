// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor

import (
	"context"
	stderrors "errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/prompt"
)

// fakePreflightChecker is a test stub for preflight.Checker.
type fakePreflightChecker struct {
	ok        bool
	err       error
	callCount int
}

func (f *fakePreflightChecker) Check(_ context.Context) (bool, error) {
	f.callCount++
	return f.ok, f.err
}

// fakeDirtyFileChecker is a test stub for DirtyFileChecker.
type fakeDirtyFileChecker struct {
	count     int
	err       error
	callCount int
}

func (f *fakeDirtyFileChecker) CountDirtyFiles(_ context.Context) (int, error) {
	f.callCount++
	return f.count, f.err
}

// fakeGitLockChecker is a test stub for GitLockChecker.
type fakeGitLockChecker struct {
	exists bool
}

func (f *fakeGitLockChecker) Exists() bool {
	return f.exists
}

var _ = Describe("DetermineBumpFromChangelog", func() {
	var (
		ctx     context.Context
		tempDir string
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		tempDir, err = os.MkdirTemp("", "determinebump-test-*")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	It("returns MinorBump for entry starting with '- feat:'", func() {
		err := os.WriteFile(
			filepath.Join(tempDir, "CHANGELOG.md"),
			[]byte("## Unreleased\n\n- feat: Add SpecWatcher\n"),
			0600,
		)
		Expect(err).NotTo(HaveOccurred())

		bump := git.DetermineBumpFromChangelog(ctx, tempDir)
		Expect(bump).To(Equal(git.MinorBump))
	})

	It("returns MinorBump for feat: entry with different description", func() {
		err := os.WriteFile(
			filepath.Join(tempDir, "CHANGELOG.md"),
			[]byte("## Unreleased\n\n- feat: Implement authentication\n"),
			0600,
		)
		Expect(err).NotTo(HaveOccurred())

		bump := git.DetermineBumpFromChangelog(ctx, tempDir)
		Expect(bump).To(Equal(git.MinorBump))
	})

	It("returns MinorBump when multiple entries include one feat: line", func() {
		err := os.WriteFile(
			filepath.Join(tempDir, "CHANGELOG.md"),
			[]byte(
				"## Unreleased\n\n- fix: Remove stale container\n- feat: Add SpecWatcher\n- chore: Update deps\n",
			),
			0600,
		)
		Expect(err).NotTo(HaveOccurred())

		bump := git.DetermineBumpFromChangelog(ctx, tempDir)
		Expect(bump).To(Equal(git.MinorBump))
	})

	It("returns PatchBump for '- fix:' entry", func() {
		err := os.WriteFile(
			filepath.Join(tempDir, "CHANGELOG.md"),
			[]byte("## Unreleased\n\n- fix: Remove stale container\n"),
			0600,
		)
		Expect(err).NotTo(HaveOccurred())

		bump := git.DetermineBumpFromChangelog(ctx, tempDir)
		Expect(bump).To(Equal(git.PatchBump))
	})

	It("returns PatchBump for '- refactor:' entry", func() {
		err := os.WriteFile(
			filepath.Join(tempDir, "CHANGELOG.md"),
			[]byte("## Unreleased\n\n- refactor: Extract worktree cleanup\n"),
			0600,
		)
		Expect(err).NotTo(HaveOccurred())

		bump := git.DetermineBumpFromChangelog(ctx, tempDir)
		Expect(bump).To(Equal(git.PatchBump))
	})

	It("returns PatchBump for '- chore:' entry", func() {
		err := os.WriteFile(
			filepath.Join(tempDir, "CHANGELOG.md"),
			[]byte("## Unreleased\n\n- chore: Update github.com/bborbe/errors to v1.5.2\n"),
			0600,
		)
		Expect(err).NotTo(HaveOccurred())

		bump := git.DetermineBumpFromChangelog(ctx, tempDir)
		Expect(bump).To(Equal(git.PatchBump))
	})

	It("returns PatchBump for '- test:' entry", func() {
		err := os.WriteFile(
			filepath.Join(tempDir, "CHANGELOG.md"),
			[]byte("## Unreleased\n\n- test: Add processor test suite\n"),
			0600,
		)
		Expect(err).NotTo(HaveOccurred())

		bump := git.DetermineBumpFromChangelog(ctx, tempDir)
		Expect(bump).To(Equal(git.PatchBump))
	})

	It("returns PatchBump for '- docs:' entry", func() {
		err := os.WriteFile(
			filepath.Join(tempDir, "CHANGELOG.md"),
			[]byte("## Unreleased\n\n- docs: Add changelog writing guide\n"),
			0600,
		)
		Expect(err).NotTo(HaveOccurred())

		bump := git.DetermineBumpFromChangelog(ctx, tempDir)
		Expect(bump).To(Equal(git.PatchBump))
	})

	It("returns PatchBump for '- perf:' entry", func() {
		err := os.WriteFile(
			filepath.Join(tempDir, "CHANGELOG.md"),
			[]byte("## Unreleased\n\n- perf: Improve startup latency\n"),
			0600,
		)
		Expect(err).NotTo(HaveOccurred())

		bump := git.DetermineBumpFromChangelog(ctx, tempDir)
		Expect(bump).To(Equal(git.PatchBump))
	})

	It("returns PatchBump for entry with no prefix (backward compat)", func() {
		err := os.WriteFile(
			filepath.Join(tempDir, "CHANGELOG.md"),
			[]byte("## Unreleased\n\n- Add container name tracking\n"),
			0600,
		)
		Expect(err).NotTo(HaveOccurred())

		bump := git.DetermineBumpFromChangelog(ctx, tempDir)
		Expect(bump).To(Equal(git.PatchBump))
	})

	It("returns PatchBump for '- feature:' entry (not exact prefix)", func() {
		err := os.WriteFile(
			filepath.Join(tempDir, "CHANGELOG.md"),
			[]byte("## Unreleased\n\n- feature: Flag system\n"),
			0600,
		)
		Expect(err).NotTo(HaveOccurred())

		bump := git.DetermineBumpFromChangelog(ctx, tempDir)
		Expect(bump).To(Equal(git.PatchBump))
	})

	It("returns PatchBump when feat: appears in middle of line", func() {
		err := os.WriteFile(
			filepath.Join(tempDir, "CHANGELOG.md"),
			[]byte("## Unreleased\n\n- fix: rename feat: related function\n"),
			0600,
		)
		Expect(err).NotTo(HaveOccurred())

		bump := git.DetermineBumpFromChangelog(ctx, tempDir)
		Expect(bump).To(Equal(git.PatchBump))
	})

	It("returns PatchBump when CHANGELOG.md does not exist", func() {
		bump := git.DetermineBumpFromChangelog(ctx, tempDir)
		Expect(bump).To(Equal(git.PatchBump))
	})

	It("returns PatchBump when no Unreleased section", func() {
		err := os.WriteFile(
			filepath.Join(tempDir, "CHANGELOG.md"),
			[]byte("# Changelog\n\n## v1.0.0\n\n- Initial release\n"),
			0600,
		)
		Expect(err).NotTo(HaveOccurred())

		bump := git.DetermineBumpFromChangelog(ctx, tempDir)
		Expect(bump).To(Equal(git.PatchBump))
	})

	It("returns PatchBump when Unreleased section is empty", func() {
		err := os.WriteFile(
			filepath.Join(tempDir, "CHANGELOG.md"),
			[]byte("# Changelog\n\n## Unreleased\n\n## v1.0.0\n\n- Initial release\n"),
			0600,
		)
		Expect(err).NotTo(HaveOccurred())

		bump := git.DetermineBumpFromChangelog(ctx, tempDir)
		Expect(bump).To(Equal(git.PatchBump))
	})
})

var _ = Describe("autoSetQueuedStatus", func() {
	var (
		ctx context.Context
		p   *processor
	)

	BeforeEach(func() {
		ctx = context.Background()
		p = &processor{
			skippedPrompts: make(map[string]libtime.DateTime),
		}
	})

	It("does not change status for cancelled prompt (no auto-promote)", func() {
		pr := &prompt.Prompt{
			Path:   "/queue/080-some-prompt.md",
			Status: prompt.CancelledPromptStatus,
		}
		err := p.autoSetQueuedStatus(ctx, pr)
		Expect(err).NotTo(HaveOccurred())
		Expect(pr.Status).To(Equal(prompt.CancelledPromptStatus))
	})

	It("does not change status for approved prompt", func() {
		pr := &prompt.Prompt{
			Path:   "/queue/080-some-prompt.md",
			Status: prompt.ApprovedPromptStatus,
		}
		err := p.autoSetQueuedStatus(ctx, pr)
		Expect(err).NotTo(HaveOccurred())
		Expect(pr.Status).To(Equal(prompt.ApprovedPromptStatus))
	})
})

// stubManager is a minimal prompt.Manager stub for internal cancel-watcher tests.
// It only implements Load; all other methods are no-ops.
type stubManager struct {
	loadFunc           func(ctx context.Context, path string) (*prompt.PromptFile, error)
	findCommittingFunc func(ctx context.Context) ([]string, error)
}

func (s *stubManager) Load(ctx context.Context, path string) (*prompt.PromptFile, error) {
	if s.loadFunc != nil {
		return s.loadFunc(ctx, path)
	}
	return nil, nil //nolint:nilnil
}

func (s *stubManager) ResetExecuting(_ context.Context) error { return nil }

func (s *stubManager) ResetFailed(_ context.Context) error { return nil }

func (s *stubManager) HasExecuting(_ context.Context) bool { return false }

func (s *stubManager) ListQueued(
	_ context.Context,
) ([]prompt.Prompt, error) {
	return nil, nil
} //nolint:nilnil
func (s *stubManager) ReadFrontmatter(_ context.Context, _ string) (*prompt.Frontmatter, error) {
	return nil, nil //nolint:nilnil
}

func (s *stubManager) SetStatus(_ context.Context, _ string, _ string) error { return nil }

func (s *stubManager) SetContainer(_ context.Context, _ string, _ string) error { return nil }

func (s *stubManager) SetVersion(_ context.Context, _ string, _ string) error { return nil }

func (s *stubManager) SetPRURL(_ context.Context, _ string, _ string) error { return nil }

func (s *stubManager) SetBranch(_ context.Context, _ string, _ string) error { return nil }

func (s *stubManager) IncrementRetryCount(_ context.Context, _ string) error { return nil }

func (s *stubManager) Content(_ context.Context, _ string) (string, error) { return "", nil }

func (s *stubManager) Title(_ context.Context, _ string) (string, error) { return "", nil }

func (s *stubManager) MoveToCompleted(_ context.Context, _ string) error { return nil }

func (s *stubManager) NormalizeFilenames(_ context.Context, _ string) ([]prompt.Rename, error) {
	return nil, nil //nolint:nilnil
}

func (s *stubManager) AllPreviousCompleted(_ context.Context, _ int) bool { return false }

func (s *stubManager) FindMissingCompleted(_ context.Context, _ int) []int { return nil }

func (s *stubManager) FindPromptStatusInProgress(_ context.Context, _ int) string { return "" }

func (s *stubManager) HasQueuedPromptsOnBranch(
	_ context.Context,
	_ string,
	_ string,
) (bool, error) {
	return false, nil
}

func (s *stubManager) FindCommitting(ctx context.Context) ([]string, error) {
	if s.findCommittingFunc != nil {
		return s.findCommittingFunc(ctx)
	}
	return nil, nil //nolint:nilnil
}

// stubExecutor is a minimal executor.Executor stub for internal cancel-watcher tests.
type stubExecutor struct {
	stopAndRemoveFunc func(ctx context.Context, containerName string)
	stopCallCount     int
	stopContainerArg  string
	reattachCallCount int
	reattachContainer string
	reattachErr       error
	reattachFunc      func(ctx context.Context, logFile string, containerName string) error
}

func (s *stubExecutor) Execute(_ context.Context, _ string, _ string, _ string) error { return nil }

func (s *stubExecutor) Reattach(
	ctx context.Context,
	logFile string,
	containerName string,
	_ time.Duration,
) error {
	s.reattachCallCount++
	s.reattachContainer = containerName
	if s.reattachFunc != nil {
		return s.reattachFunc(ctx, logFile, containerName)
	}
	return s.reattachErr
}

func (s *stubExecutor) StopAndRemoveContainer(ctx context.Context, containerName string) {
	s.stopCallCount++
	s.stopContainerArg = containerName
	if s.stopAndRemoveFunc != nil {
		s.stopAndRemoveFunc(ctx, containerName)
	}
}

var _ = Describe("watchForCancellation", func() {
	var (
		tempDir    string
		promptPath string
		mgr        *stubManager
		exec       *stubExecutor
		p          *processor
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "cancel-watcher-test-*")
		Expect(err).NotTo(HaveOccurred())

		promptPath = filepath.Join(tempDir, "080-test-prompt.md")
		err = os.WriteFile(promptPath, []byte("---\nstatus: executing\n---\n\n# Test\n"), 0600)
		Expect(err).NotTo(HaveOccurred())

		mgr = &stubManager{}
		exec = &stubExecutor{}

		p = &processor{
			promptManager:  mgr,
			executor:       exec,
			skippedPrompts: make(map[string]libtime.DateTime),
		}
	})

	AfterEach(func() {
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	It("exits immediately when context is already done", func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately

		execCancelCalled := false
		execCancel := func() { execCancelCalled = true }
		var cancelled bool

		done := make(chan struct{})
		go func() {
			defer close(done)
			p.watchForCancellation(ctx, execCancel, promptPath, "test-container", &cancelled)
		}()

		Eventually(done, 2*time.Second).Should(BeClosed())
		Expect(cancelled).To(BeFalse())
		Expect(execCancelCalled).To(BeFalse())
	})

	It("sets cancelledByUser and calls execCancel when status changes to cancelled", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		execCancelCalled := false
		execCancel := func() { execCancelCalled = true }
		var cancelled bool

		cancelledPF := prompt.NewPromptFile(
			promptPath,
			prompt.Frontmatter{Status: string(prompt.CancelledPromptStatus)},
			[]byte("# Test\n"),
			libtime.NewCurrentDateTime(),
		)
		mgr.loadFunc = func(_ context.Context, _ string) (*prompt.PromptFile, error) {
			return cancelledPF, nil
		}

		done := make(chan struct{})
		go func() {
			defer close(done)
			p.watchForCancellation(ctx, execCancel, promptPath, "test-container", &cancelled)
		}()

		// Trigger a write event by updating the file
		time.Sleep(100 * time.Millisecond)
		err := os.WriteFile(promptPath, []byte("---\nstatus: cancelled\n---\n\n# Test\n"), 0600)
		Expect(err).NotTo(HaveOccurred())

		Eventually(done, 2*time.Second).Should(BeClosed())
		Expect(cancelled).To(BeTrue())
		Expect(execCancelCalled).To(BeTrue())
		Expect(exec.stopCallCount).To(Equal(1))
		Expect(exec.stopContainerArg).To(Equal("test-container"))
	})
})

var _ = Describe("ResumeExecuting", func() {
	var (
		ctx      context.Context
		tempDir  string
		queueDir string
		logDir   string
		fakeExec *stubExecutor
		mgr      *stubManager
		proc     *processor
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		tempDir, err = os.MkdirTemp("", "resume-executing-test-*")
		Expect(err).NotTo(HaveOccurred())

		queueDir = filepath.Join(tempDir, "in-progress")
		logDir = filepath.Join(tempDir, "logs")

		Expect(os.MkdirAll(queueDir, 0750)).To(Succeed())
		Expect(os.MkdirAll(logDir, 0750)).To(Succeed())

		fakeExec = &stubExecutor{}
		mgr = &stubManager{}

		proc = &processor{
			dirs:             Dirs{Queue: queueDir, Log: logDir},
			projectName:      "test-project",
			executor:         fakeExec,
			promptManager:    mgr,
			workflowExecutor: NewDirectWorkflowExecutor(WorkflowDeps{PromptManager: mgr}),
			skippedPrompts:   make(map[string]libtime.DateTime),
		}
	})

	AfterEach(func() {
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	Context("when queueDir does not exist", func() {
		It("returns nil without error", func() {
			proc.dirs.Queue = filepath.Join(tempDir, "nonexistent")
			err := proc.ResumeExecuting(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeExec.reattachCallCount).To(Equal(0))
		})
	})

	Context("when no executing prompts exist", func() {
		BeforeEach(func() {
			promptPath := filepath.Join(queueDir, "001-approved.md")
			Expect(
				os.WriteFile(
					promptPath,
					[]byte("---\nstatus: approved\n---\n# Approved prompt\n"),
					0600,
				),
			).To(Succeed())

			pf := prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{Status: "approved"},
				[]byte("# Approved prompt\n"),
				libtime.NewCurrentDateTime(),
			)
			mgr.loadFunc = func(_ context.Context, _ string) (*prompt.PromptFile, error) {
				return pf, nil
			}
		})

		It("returns nil without calling Reattach", func() {
			err := proc.ResumeExecuting(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeExec.reattachCallCount).To(Equal(0))
		})
	})

	Context("when executing prompt has empty container name", func() {
		BeforeEach(func() {
			promptPath := filepath.Join(queueDir, "001-nocontainer.md")
			Expect(
				os.WriteFile(
					promptPath,
					[]byte("---\nstatus: executing\n---\n# No container\n"),
					0600,
				),
			).To(Succeed())

			pf := prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{Status: "executing"},
				[]byte("# No container\n"),
				libtime.NewCurrentDateTime(),
			)
			mgr.loadFunc = func(_ context.Context, _ string) (*prompt.PromptFile, error) {
				return pf, nil
			}
		})

		It("resets prompt to approved without calling Reattach", func() {
			err := proc.ResumeExecuting(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeExec.reattachCallCount).To(Equal(0))

			// Verify the file on disk was reset to approved
			content, readErr := os.ReadFile(filepath.Join(queueDir, "001-nocontainer.md"))
			Expect(readErr).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("status: approved"))
		})
	})

	Context("when worktree=true and clone dir is missing", func() {
		BeforeEach(func() {
			proc.workflowExecutor = NewCloneWorkflowExecutor(
				WorkflowDeps{ProjectName: "test-project", PromptManager: mgr},
			)
			promptPath := filepath.Join(queueDir, "001-test.md")
			Expect(
				os.WriteFile(
					promptPath,
					[]byte(
						"---\nstatus: executing\ncontainer: test-project-001-test\n---\n# Test\n",
					),
					0600,
				),
			).To(Succeed())

			pf := prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{Status: "executing", Container: "test-project-001-test"},
				[]byte("# Test\n"),
				libtime.NewCurrentDateTime(),
			)
			mgr.loadFunc = func(_ context.Context, _ string) (*prompt.PromptFile, error) {
				return pf, nil
			}
		})

		It("resets prompt to approved without calling Reattach", func() {
			err := proc.ResumeExecuting(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeExec.reattachCallCount).To(Equal(0))

			// Verify the file on disk was reset to approved
			content, readErr := os.ReadFile(filepath.Join(queueDir, "001-test.md"))
			Expect(readErr).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("status: approved"))
		})
	})

	Context("when executing prompt has container name (direct workflow)", func() {
		var promptPath string

		BeforeEach(func() {
			promptPath = filepath.Join(queueDir, "001-resume.md")
			Expect(
				os.WriteFile(
					promptPath,
					[]byte(
						"---\nstatus: executing\ncontainer: test-project-001-resume\n---\n# Resume test\n",
					),
					0600,
				),
			).To(Succeed())

			pf := prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{Status: "executing", Container: "test-project-001-resume"},
				[]byte("# Resume test\n"),
				libtime.NewCurrentDateTime(),
			)
			mgr.loadFunc = func(_ context.Context, _ string) (*prompt.PromptFile, error) {
				return pf, nil
			}
			// Return an error from Reattach so we stop before handlePostExecution
			fakeExec.reattachErr = &testReattachError{}
		})

		It("calls Reattach with the container name from frontmatter", func() {
			err := proc.ResumeExecuting(ctx)
			Expect(err).To(HaveOccurred())
			Expect(fakeExec.reattachCallCount).To(Equal(1))
			Expect(fakeExec.reattachContainer).To(Equal("test-project-001-resume"))
		})
	})

	Context("when Load returns an error", func() {
		BeforeEach(func() {
			promptPath := filepath.Join(queueDir, "001-loaderr.md")
			Expect(
				os.WriteFile(
					promptPath,
					[]byte("---\nstatus: executing\n---\n# Load error test\n"),
					0600,
				),
			).To(Succeed())

			mgr.loadFunc = func(_ context.Context, _ string) (*prompt.PromptFile, error) {
				return nil, &testReattachError{}
			}
		})

		It("returns an error wrapping load prompt for resume", func() {
			err := proc.ResumeExecuting(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("load prompt for resume"))
			Expect(fakeExec.reattachCallCount).To(Equal(0))
		})
	})

	Context("when Reattach succeeds but second Load returns error", func() {
		var promptPath string

		BeforeEach(func() {
			promptPath = filepath.Join(queueDir, "001-reloadfail.md")
			Expect(
				os.WriteFile(
					promptPath,
					[]byte(
						"---\nstatus: executing\ncontainer: test-project-reload\n---\n# Reload fail test\n",
					),
					0600,
				),
			).To(Succeed())

			callCount := 0
			mgr.loadFunc = func(_ context.Context, _ string) (*prompt.PromptFile, error) {
				callCount++
				if callCount == 1 {
					pf := prompt.NewPromptFile(
						promptPath,
						prompt.Frontmatter{Status: "executing", Container: "test-project-reload"},
						[]byte("# Reload fail test\n"),
						libtime.NewCurrentDateTime(),
					)
					return pf, nil
				}
				// Second load (after Reattach) fails
				return nil, &testReattachError{}
			}
		})

		It("returns error wrapping reload prompt after reattach", func() {
			err := proc.ResumeExecuting(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("reload prompt after reattach"))
			Expect(fakeExec.reattachCallCount).To(Equal(1))
		})
	})

	Context("when worktree=true and clone dir exists", func() {
		var clonePath string

		BeforeEach(func() {
			proc.workflowExecutor = NewCloneWorkflowExecutor(
				WorkflowDeps{ProjectName: "test-project", PromptManager: mgr},
			)
			promptPath := filepath.Join(queueDir, "001-cloneexists.md")
			clonePath = filepath.Join(os.TempDir(), "dark-factory", "test-project-001-cloneexists")
			Expect(os.MkdirAll(clonePath, 0750)).To(Succeed())

			Expect(
				os.WriteFile(
					promptPath,
					[]byte(
						"---\nstatus: executing\ncontainer: test-project-001-cloneexists\nbranch: dark-factory/001-cloneexists\n---\n# Clone exists test\n",
					),
					0600,
				),
			).To(Succeed())

			pf := prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{
					Status:    "executing",
					Container: "test-project-001-cloneexists",
				},
				[]byte("# Clone exists test\n"),
				libtime.NewCurrentDateTime(),
			)
			// Make Branch() return the expected branch
			pf.SetBranch("dark-factory/001-cloneexists")

			mgr.loadFunc = func(_ context.Context, _ string) (*prompt.PromptFile, error) {
				return pf, nil
			}
			// Return sentinel so Reattach fails and we don't hit handlePostExecution
			fakeExec.reattachErr = &testReattachError{}
		})

		AfterEach(func() {
			_ = os.RemoveAll(clonePath)
		})

		It("calls Reattach (clone dir found, proceeds to reattach)", func() {
			err := proc.ResumeExecuting(ctx)
			Expect(err).To(HaveOccurred())
			Expect(fakeExec.reattachCallCount).To(Equal(1))
		})
	})

	Context("when worktree=true and clone exists with empty branch name", func() {
		var clonePath string

		BeforeEach(func() {
			proc.workflowExecutor = NewCloneWorkflowExecutor(
				WorkflowDeps{ProjectName: "test-project", PromptManager: mgr},
			)
			promptPath := filepath.Join(queueDir, "001-nobranch.md")
			clonePath = filepath.Join(os.TempDir(), "dark-factory", "test-project-001-nobranch")
			Expect(os.MkdirAll(clonePath, 0750)).To(Succeed())

			Expect(
				os.WriteFile(
					promptPath,
					[]byte(
						"---\nstatus: executing\ncontainer: test-project-001-nobranch\n---\n# No branch test\n",
					),
					0600,
				),
			).To(Succeed())

			pf := prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{Status: "executing", Container: "test-project-001-nobranch"},
				[]byte("# No branch test\n"),
				libtime.NewCurrentDateTime(),
			)

			mgr.loadFunc = func(_ context.Context, _ string) (*prompt.PromptFile, error) {
				return pf, nil
			}
			// Return sentinel so Reattach fails and we don't hit handlePostExecution
			fakeExec.reattachErr = &testReattachError{}
		})

		AfterEach(func() {
			_ = os.RemoveAll(clonePath)
		})

		It("defaults branch to dark-factory/<baseName> and calls Reattach", func() {
			err := proc.ResumeExecuting(ctx)
			Expect(err).To(HaveOccurred())
			Expect(fakeExec.reattachCallCount).To(Equal(1))
		})
	})
})

// testReattachError is a sentinel error type used in ResumeExecuting tests.
type testReattachError struct{}

func (t *testReattachError) Error() string { return "reattach sentinel error" }

// stubContainerCounter is a minimal ContainerCounter for internal waitForContainerSlot tests.
type stubContainerCounter struct {
	callCount int32
	fn        func(n int) (int, error)
}

func (s *stubContainerCounter) CountRunning(_ context.Context) (int, error) {
	n := int(atomic.AddInt32(&s.callCount, 1))
	return s.fn(n)
}

func (s *stubContainerCounter) calls() int {
	return int(atomic.LoadInt32(&s.callCount))
}

// fakeContainerLock is a local test fake for containerlock.ContainerLock.
// It records Acquire/Release call counts and supports configurable stubs.
type fakeContainerLock struct {
	mu               sync.Mutex
	acquireCallCount int32
	releaseCallCount int32
	acquireErr       error
	acquireStub      func(context.Context) error
	releaseStub      func(context.Context) error
}

func (f *fakeContainerLock) Acquire(ctx context.Context) error {
	atomic.AddInt32(&f.acquireCallCount, 1)
	f.mu.Lock()
	stub := f.acquireStub
	err := f.acquireErr
	f.mu.Unlock()
	if stub != nil {
		return stub(ctx)
	}
	return err
}

func (f *fakeContainerLock) Release(ctx context.Context) error {
	atomic.AddInt32(&f.releaseCallCount, 1)
	f.mu.Lock()
	stub := f.releaseStub
	f.mu.Unlock()
	if stub != nil {
		return stub(ctx)
	}
	return nil
}

func (f *fakeContainerLock) AcquireCallCount() int {
	return int(atomic.LoadInt32(&f.acquireCallCount))
}

func (f *fakeContainerLock) ReleaseCallCount() int {
	return int(atomic.LoadInt32(&f.releaseCallCount))
}

var _ = Describe("waitForContainerSlot", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
		p      *processor
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		p = &processor{
			maxContainers:         3,
			containerPollInterval: 10 * time.Millisecond,
			skippedPrompts:        make(map[string]libtime.DateTime),
		}
	})

	AfterEach(func() {
		cancel()
	})

	It("returns nil immediately when maxContainers is 0", func() {
		counter := &stubContainerCounter{fn: func(_ int) (int, error) { return 0, nil }}
		p.maxContainers = 0
		p.containerCounter = counter
		err := p.waitForContainerSlot(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(counter.calls()).To(Equal(0))
	})

	It("returns nil immediately when count is below limit", func() {
		counter := &stubContainerCounter{fn: func(_ int) (int, error) { return 2, nil }}
		p.containerCounter = counter
		err := p.waitForContainerSlot(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(counter.calls()).To(Equal(1))
	})

	It("waits and returns nil when slot frees up", func() {
		counter := &stubContainerCounter{fn: func(n int) (int, error) {
			if n == 1 {
				return 3, nil // at limit on first call
			}
			return 2, nil // slot freed on second call
		}}
		p.containerCounter = counter
		err := p.waitForContainerSlot(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(counter.calls()).To(Equal(2))
	})

	It("returns ctx.Err() when context is cancelled while waiting", func() {
		counter := &stubContainerCounter{fn: func(_ int) (int, error) { return 3, nil }}
		p.containerCounter = counter
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()
		err := p.waitForContainerSlot(ctx)
		Expect(err).To(Equal(context.Canceled))
	})

	It("proceeds (returns nil) when counter returns error", func() {
		counter := &stubContainerCounter{fn: func(_ int) (int, error) {
			return 0, stderrors.New("docker error")
		}}
		p.containerCounter = counter
		err := p.waitForContainerSlot(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(counter.calls()).To(Equal(1))
	})
})

var _ = Describe("prepareContainerSlot", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
		p      *processor
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		p = &processor{
			maxContainers:         3,
			containerPollInterval: 10 * time.Millisecond,
			skippedPrompts:        make(map[string]libtime.DateTime),
		}
	})

	AfterEach(func() {
		cancel()
	})

	It("nil-lock fast path, slot free immediately", func() {
		counter := &stubContainerCounter{fn: func(_ int) (int, error) { return 1, nil }}
		p.containerLock = nil
		p.containerCounter = counter
		release, err := p.prepareContainerSlot(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(release).NotTo(BeNil())
		Expect(counter.calls()).To(Equal(1))
	})

	It("nil-lock fast path, slot-wait then free", func() {
		counter := &stubContainerCounter{fn: func(n int) (int, error) {
			if n == 1 {
				return 3, nil
			}
			return 2, nil
		}}
		p.containerLock = nil
		p.containerCounter = counter
		release, err := p.prepareContainerSlot(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(release).NotTo(BeNil())
		Expect(counter.calls()).To(Equal(2))
	})

	It("lock held, slot free immediately", func() {
		fakeLock := &fakeContainerLock{}
		counter := &stubContainerCounter{fn: func(_ int) (int, error) { return 2, nil }}
		p.containerLock = fakeLock
		p.containerCounter = counter
		release, err := p.prepareContainerSlot(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(release).NotTo(BeNil())
		Expect(fakeLock.AcquireCallCount()).To(Equal(1))
		Expect(fakeLock.ReleaseCallCount()).To(Equal(0))
		// Calling release once invokes Release exactly once.
		release()
		Expect(fakeLock.ReleaseCallCount()).To(Equal(1))
		// Calling release a second time is idempotent.
		release()
		Expect(fakeLock.ReleaseCallCount()).To(Equal(1))
	})

	It("lock held, slot full then free — key regression test", func() {
		counter := &stubContainerCounter{fn: func(n int) (int, error) {
			if n <= 2 {
				return 3, nil
			}
			return 2, nil
		}}
		p.containerCounter = counter
		p.containerPollInterval = 10 * time.Millisecond

		// Record event ordering: A=Acquire, R=Release.
		var events []string
		var eventsMu sync.Mutex
		appendEvent := func(e string) {
			eventsMu.Lock()
			events = append(events, e)
			eventsMu.Unlock()
		}
		fakeLock := &fakeContainerLock{
			acquireStub: func(_ context.Context) error {
				appendEvent("A")
				return nil
			},
			releaseStub: func(_ context.Context) error {
				appendEvent("R")
				return nil
			},
		}
		p.containerLock = fakeLock

		release, err := p.prepareContainerSlot(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(release).NotTo(BeNil())
		Expect(fakeLock.AcquireCallCount()).To(Equal(3))
		Expect(fakeLock.ReleaseCallCount()).To(Equal(2))
		Expect(counter.calls()).To(Equal(3))

		// Assert ordering before calling release: every slot-full Acquire is followed by
		// a Release before the next Acquire. The returned lock is still held.
		Expect(events).To(Equal([]string{"A", "R", "A", "R", "A"}))

		// The returned release has not been called yet.
		Expect(fakeLock.ReleaseCallCount()).To(Equal(2))
		release()
		Expect(fakeLock.ReleaseCallCount()).To(Equal(3))
	})

	It("ctx cancellation during slot-wait releases all acquired locks", func() {
		fakeLock := &fakeContainerLock{}
		counter := &stubContainerCounter{fn: func(_ int) (int, error) { return 3, nil }}
		p.containerLock = fakeLock
		p.containerCounter = counter
		p.containerPollInterval = 50 * time.Millisecond

		go func() {
			time.Sleep(80 * time.Millisecond)
			cancel()
		}()

		_, err := p.prepareContainerSlot(ctx)
		Expect(err).To(HaveOccurred())
		Expect(stderrors.Is(err, context.Canceled)).To(BeTrue())
		// Every acquired lock was released — no leaked lock.
		Expect(fakeLock.ReleaseCallCount()).To(Equal(fakeLock.AcquireCallCount()))
	})

	It("acquire error is propagated", func() {
		fakeLock := &fakeContainerLock{acquireErr: stderrors.New("flock denied")}
		p.containerLock = fakeLock
		// counter is not set — if it were called, test would panic.
		_, err := p.prepareContainerSlot(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("flock denied"))
		Expect(fakeLock.ReleaseCallCount()).To(Equal(0))
	})

	It("counter error is tolerated and lock stays held", func() {
		fakeLock := &fakeContainerLock{}
		counter := &stubContainerCounter{fn: func(_ int) (int, error) {
			return 0, stderrors.New("docker ls failed")
		}}
		p.containerLock = fakeLock
		p.containerCounter = counter
		release, err := p.prepareContainerSlot(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(release).NotTo(BeNil())
		Expect(fakeLock.AcquireCallCount()).To(Equal(1))
		Expect(fakeLock.ReleaseCallCount()).To(Equal(0))
	})
})

// stubReleaser is a minimal git.Releaser stub for handleDirectWorkflow tests.
type stubReleaser struct {
	hasChangelog       bool
	commitOnlyCalled   int
	commitAndRelCalled int
	nextVersion        string
}

func (s *stubReleaser) HasChangelog(_ context.Context) bool { return s.hasChangelog }

func (s *stubReleaser) CommitOnly(_ context.Context, _ string) error {
	s.commitOnlyCalled++
	return nil
}

func (s *stubReleaser) CommitAndRelease(_ context.Context, _ git.VersionBump) error {
	s.commitAndRelCalled++
	return nil
}

func (s *stubReleaser) GetNextVersion(_ context.Context, _ git.VersionBump) (string, error) {
	return s.nextVersion, nil
}

func (s *stubReleaser) CommitCompletedFile(_ context.Context, _ string) error { return nil }

func (s *stubReleaser) MoveFile(_ context.Context, _, _ string) error { return nil }

var _ = Describe("handleDirectWorkflow", func() {
	var (
		ctx    context.Context
		gitCtx context.Context
		rel    *stubReleaser
	)

	BeforeEach(func() {
		ctx = context.Background()
		gitCtx = context.Background()
		rel = &stubReleaser{nextVersion: "v1.1.0"}
	})

	newDeps := func(autoRelease bool) WorkflowDeps {
		return WorkflowDeps{
			Releaser:    rel,
			AutoRelease: autoRelease,
		}
	}

	Context("with CHANGELOG present and autoRelease disabled", func() {
		BeforeEach(func() {
			rel.hasChangelog = true
		})

		It("calls CommitOnly and does not call CommitAndRelease", func() {
			err := handleDirectWorkflow(gitCtx, ctx, newDeps(false), "test title", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(rel.commitOnlyCalled).To(Equal(1))
			Expect(rel.commitAndRelCalled).To(Equal(0))
		})
	})

	Context("with CHANGELOG present and autoRelease enabled", func() {
		BeforeEach(func() {
			rel.hasChangelog = true
		})

		It("calls CommitAndRelease and does not call CommitOnly", func() {
			err := handleDirectWorkflow(gitCtx, ctx, newDeps(true), "test title", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(rel.commitAndRelCalled).To(Equal(1))
			Expect(rel.commitOnlyCalled).To(Equal(0))
		})
	})

	Context("without CHANGELOG (autoRelease value irrelevant)", func() {
		BeforeEach(func() {
			rel.hasChangelog = false
		})

		It("calls CommitOnly regardless of autoRelease", func() {
			err := handleDirectWorkflow(gitCtx, ctx, newDeps(true), "test title", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(rel.commitOnlyCalled).To(Equal(1))
			Expect(rel.commitAndRelCalled).To(Equal(0))
		})
	})
})

var _ = Describe("checkDirtyFileThreshold", func() {
	var (
		ctx     context.Context
		p       *processor
		checker *fakeDirtyFileChecker
	)

	BeforeEach(func() {
		ctx = context.Background()
		checker = &fakeDirtyFileChecker{}
		p = &processor{}
	})

	It("returns (false, nil) when threshold is 0 (disabled) without calling checker", func() {
		p.dirtyFileThreshold = 0
		p.dirtyFileChecker = checker
		skip, err := p.checkDirtyFileThreshold(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(skip).To(BeFalse())
		Expect(checker.callCount).To(Equal(0))
	})

	It("returns (false, nil) when dirty count is under threshold", func() {
		p.dirtyFileThreshold = 10
		p.dirtyFileChecker = checker
		checker.count = 5
		skip, err := p.checkDirtyFileThreshold(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(skip).To(BeFalse())
	})

	It("returns (false, nil) when dirty count equals threshold", func() {
		p.dirtyFileThreshold = 10
		p.dirtyFileChecker = checker
		checker.count = 10
		skip, err := p.checkDirtyFileThreshold(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(skip).To(BeFalse())
	})

	It("returns (true, nil) when dirty count exceeds threshold", func() {
		p.dirtyFileThreshold = 10
		p.dirtyFileChecker = checker
		checker.count = 11
		skip, err := p.checkDirtyFileThreshold(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(skip).To(BeTrue())
	})

	It("returns (false, err) when checker returns an error", func() {
		p.dirtyFileThreshold = 10
		p.dirtyFileChecker = checker
		checker.err = stderrors.New("git error")
		skip, err := p.checkDirtyFileThreshold(ctx)
		Expect(err).To(HaveOccurred())
		Expect(skip).To(BeFalse())
	})
})

var _ = Describe("checkGitIndexLock", func() {
	var (
		p       *processor
		checker *fakeGitLockChecker
	)

	BeforeEach(func() {
		checker = &fakeGitLockChecker{}
		p = &processor{}
	})

	It("returns false when checker is nil", func() {
		p.gitLockChecker = nil
		Expect(p.checkGitIndexLock()).To(BeFalse())
	})

	It("returns false when lock file does not exist", func() {
		checker.exists = false
		p.gitLockChecker = checker
		Expect(p.checkGitIndexLock()).To(BeFalse())
	})

	It("returns true when lock file exists", func() {
		checker.exists = true
		p.gitLockChecker = checker
		Expect(p.checkGitIndexLock()).To(BeTrue())
	})
})

var _ = Describe("checkPreflightConditions — preflight checker", func() {
	var (
		ctx         context.Context
		proc        *processor
		fakeChecker *fakePreflightChecker
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeChecker = &fakePreflightChecker{}
		proc = &processor{skippedPrompts: make(map[string]libtime.DateTime)}
		proc.SetPreflightChecker(fakeChecker)
	})

	It("returns ErrPreflightSkip when preflight checker returns false", func() {
		fakeChecker.ok = false
		skip, err := proc.CheckPreflightConditions(ctx)
		Expect(err).To(HaveOccurred())
		Expect(stderrors.Is(err, ErrPreflightSkip)).To(BeTrue())
		Expect(skip).To(BeFalse())
		Expect(fakeChecker.callCount).To(Equal(1))
	})

	It("returns skip=false when preflight checker returns true", func() {
		fakeChecker.ok = true
		skip, err := proc.CheckPreflightConditions(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(skip).To(BeFalse())
	})

	It("returns ErrPreflightSkip when preflight checker returns an error", func() {
		fakeChecker.ok = false
		fakeChecker.err = stderrors.New("internal error")
		skip, err := proc.CheckPreflightConditions(ctx)
		Expect(err).To(HaveOccurred())
		Expect(stderrors.Is(err, ErrPreflightSkip)).To(BeTrue())
		Expect(skip).To(BeFalse())
	})

	It("returns skip=false when no preflight checker is set (nil)", func() {
		proc.SetPreflightChecker(nil)
		skip, err := proc.CheckPreflightConditions(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(skip).To(BeFalse())
	})
})

// --- Stub types for workflow routing tests ---

// stubWorktreer tracks Add/Remove calls.
type stubWorktreer struct {
	addStub      func(ctx context.Context, path, branch string) error
	removeStub   func(ctx context.Context, path string) error
	addCallCount int
	addPath      string
	addBranch    string
	removeCount  int
}

func (s *stubWorktreer) Add(ctx context.Context, path, branch string) error {
	s.addCallCount++
	s.addPath = path
	s.addBranch = branch
	if s.addStub != nil {
		return s.addStub(ctx, path, branch)
	}
	return nil
}

func (s *stubWorktreer) Remove(_ context.Context, _ string) error {
	s.removeCount++
	if s.removeStub != nil {
		return s.removeStub(context.Background(), "")
	}
	return nil
}

// stubCloner tracks Clone/Remove calls.
type stubCloner struct {
	cloneStub   func(ctx context.Context, srcDir, destDir, branch string) error
	removeStub  func(ctx context.Context, path string) error
	cloneCount  int
	removeCount int
}

func (s *stubCloner) Clone(ctx context.Context, srcDir, destDir, branch string) error {
	s.cloneCount++
	if s.cloneStub != nil {
		return s.cloneStub(ctx, srcDir, destDir, branch)
	}
	return nil
}

func (s *stubCloner) Remove(_ context.Context, _ string) error {
	s.removeCount++
	if s.removeStub != nil {
		return s.removeStub(context.Background(), "")
	}
	return nil
}

// stubBrancher tracks Push/CreateAndSwitch/FetchAndVerify/Switch/IsClean/DefaultBranch/MergeToDefault calls.
type stubBrancher struct {
	pushCount             int
	createAndSwitchCount  int
	fetchAndVerifyErr     error
	isClean               bool
	isCleanErr            error
	defaultBranch         string
	defaultBranchErr      error
	switchErr             error
	switchCount           int
	mergeToDefaultCount   int
	fetchErr              error
	mergeOriginDefaultErr error
	commitsAhead          int
	commitsAheadErr       error
}

func (s *stubBrancher) Push(_ context.Context, _ string) error {
	s.pushCount++
	return nil
}

func (s *stubBrancher) CreateAndSwitch(_ context.Context, _ string) error {
	s.createAndSwitchCount++
	return nil
}

func (s *stubBrancher) FetchAndVerifyBranch(_ context.Context, _ string) error {
	return s.fetchAndVerifyErr
}

func (s *stubBrancher) Switch(_ context.Context, _ string) error {
	s.switchCount++
	return s.switchErr
}

func (s *stubBrancher) IsClean(_ context.Context) (bool, error) {
	return s.isClean, s.isCleanErr
}

func (s *stubBrancher) DefaultBranch(_ context.Context) (string, error) {
	return s.defaultBranch, s.defaultBranchErr
}

func (s *stubBrancher) MergeToDefault(_ context.Context, _ string) error {
	s.mergeToDefaultCount++
	return nil
}

func (s *stubBrancher) CurrentBranch(_ context.Context) (string, error) { return "", nil }

func (s *stubBrancher) Fetch(_ context.Context) error { return s.fetchErr }

func (s *stubBrancher) Pull(_ context.Context) error { return nil }

func (s *stubBrancher) MergeOriginDefault(
	_ context.Context,
) error {
	return s.mergeOriginDefaultErr
}

func (s *stubBrancher) CommitsAhead(_ context.Context, _ string) (int, error) {
	return s.commitsAhead, s.commitsAheadErr
}

// stubPRCreator tracks FindOpenPR/Create calls.
type stubPRCreator struct {
	findOpenPRURL string
	findOpenPRErr error
	createURL     string
	createErr     error
	createCount   int
}

func (s *stubPRCreator) FindOpenPR(_ context.Context, _ string) (string, error) {
	return s.findOpenPRURL, s.findOpenPRErr
}

func (s *stubPRCreator) Create(_ context.Context, _, _ string) (string, error) {
	s.createCount++
	if s.createErr != nil {
		return "", s.createErr
	}
	return s.createURL, nil
}

// stubPRMergerImpl tracks WaitAndMerge calls.
type stubPRMergerImpl struct {
	waitAndMergeCount int
	waitAndMergeErr   error
}

func (s *stubPRMergerImpl) WaitAndMerge(_ context.Context, _ string) error {
	s.waitAndMergeCount++
	return s.waitAndMergeErr
}

// stubAutoCompleter is a minimal spec.AutoCompleter stub for workflow tests.
type stubAutoCompleter struct{}

func (s *stubAutoCompleter) CheckAndComplete(_ context.Context, _ string) error { return nil }

// stubWorkflowReleaser is a minimal git.Releaser for workflow routing tests.
type stubWorkflowReleaser struct {
	commitOnlyCount       int
	commitOnlyErr         error
	commitFileCount       int
	commitFileErr         error
	hasChangelog          bool
	commitAndReleaseCount int
}

func (s *stubWorkflowReleaser) CommitOnly(_ context.Context, _ string) error {
	s.commitOnlyCount++
	return s.commitOnlyErr
}

func (s *stubWorkflowReleaser) CommitCompletedFile(_ context.Context, _ string) error {
	s.commitFileCount++
	return s.commitFileErr
}

func (s *stubWorkflowReleaser) HasChangelog(_ context.Context) bool { return s.hasChangelog }

func (s *stubWorkflowReleaser) CommitAndRelease(_ context.Context, _ git.VersionBump) error {
	s.commitAndReleaseCount++
	return nil
}

func (s *stubWorkflowReleaser) GetNextVersion(
	_ context.Context,
	_ git.VersionBump,
) (string, error) {
	return "v1.0.0", nil
}

func (s *stubWorkflowReleaser) MoveFile(_ context.Context, _, _ string) error { return nil }

// stubWorkflowManager tracks MoveToCompleted and HasQueuedPromptsOnBranch.
type stubWorkflowManager struct {
	moveToCompletedCount    int
	moveToCompletedErr      error
	hasQueuedOnBranchResult bool
	hasQueuedOnBranchErr    error
	existingPRURL           string
	setPRURLCount           int
}

func (s *stubWorkflowManager) MoveToCompleted(_ context.Context, _ string) error {
	s.moveToCompletedCount++
	return s.moveToCompletedErr
}

func (s *stubWorkflowManager) HasQueuedPromptsOnBranch(
	_ context.Context,
	_, _ string,
) (bool, error) {
	return s.hasQueuedOnBranchResult, s.hasQueuedOnBranchErr
}

// Remaining prompt.Manager methods (no-ops).
func (s *stubWorkflowManager) ResetExecuting(_ context.Context) error { return nil }

func (s *stubWorkflowManager) ResetFailed(_ context.Context) error { return nil }

func (s *stubWorkflowManager) HasExecuting(_ context.Context) bool { return false }

func (s *stubWorkflowManager) ListQueued(_ context.Context) ([]prompt.Prompt, error) {
	return nil, nil //nolint:nilnil
}

func (s *stubWorkflowManager) Load(_ context.Context, path string) (*prompt.PromptFile, error) {
	if s.existingPRURL != "" {
		pf := prompt.NewPromptFile(
			path,
			prompt.Frontmatter{PRURL: s.existingPRURL},
			[]byte(""),
			libtime.NewCurrentDateTime(),
		)
		return pf, nil
	}
	return nil, nil //nolint:nilnil
}

func (s *stubWorkflowManager) ReadFrontmatter(
	_ context.Context,
	_ string,
) (*prompt.Frontmatter, error) {
	return nil, nil //nolint:nilnil
}

func (s *stubWorkflowManager) SetStatus(_ context.Context, _, _ string) error { return nil }

func (s *stubWorkflowManager) SetContainer(_ context.Context, _, _ string) error { return nil }

func (s *stubWorkflowManager) SetVersion(_ context.Context, _, _ string) error { return nil }

func (s *stubWorkflowManager) SetPRURL(_ context.Context, _, _ string) error {
	s.setPRURLCount++
	return nil
}

func (s *stubWorkflowManager) SetBranch(_ context.Context, _, _ string) error { return nil }

func (s *stubWorkflowManager) IncrementRetryCount(_ context.Context, _ string) error { return nil }

func (s *stubWorkflowManager) Content(
	_ context.Context,
	_ string,
) (string, error) {
	return "", nil
}

func (s *stubWorkflowManager) Title(_ context.Context, _ string) (string, error) { return "", nil }

func (s *stubWorkflowManager) NormalizeFilenames(
	_ context.Context,
	_ string,
) ([]prompt.Rename, error) {
	return nil, nil //nolint:nilnil
}

func (s *stubWorkflowManager) AllPreviousCompleted(_ context.Context, _ int) bool { return false }

func (s *stubWorkflowManager) FindMissingCompleted(_ context.Context, _ int) []int { return nil }

func (s *stubWorkflowManager) FindPromptStatusInProgress(_ context.Context, _ int) string {
	return ""
}

func (s *stubWorkflowManager) FindCommitting(
	_ context.Context,
) ([]string, error) {
	return nil, nil
}

var _ = Describe("processor workflow routing", func() {
	var (
		ctx           context.Context
		projectDir    string
		completedDir  string
		promptPath    string
		completedPath string

		stubWt       *stubWorktreer
		stubCl       *stubCloner
		stubBr       *stubBrancher
		stubPR       *stubPRCreator
		stubPRMerger *stubPRMergerImpl
		stubRel      *stubWorkflowReleaser
		stubMgr      *stubWorkflowManager
	)

	BeforeEach(func() {
		ctx = context.Background()

		projectDir = GinkgoT().TempDir()
		completedDir = filepath.Join(projectDir, "completed")
		Expect(os.MkdirAll(completedDir, 0750)).To(Succeed())

		promptPath = filepath.Join(projectDir, "001-test.md")
		completedPath = filepath.Join(completedDir, "001-test.md")

		stubWt = &stubWorktreer{}
		stubCl = &stubCloner{}
		stubBr = &stubBrancher{
			isClean:           true,
			defaultBranch:     "main",
			fetchAndVerifyErr: stderrors.New("branch not found"),
		}
		stubPR = &stubPRCreator{createURL: "https://github.com/user/repo/pull/1"}
		stubPRMerger = &stubPRMergerImpl{}
		stubRel = &stubWorkflowReleaser{}
		stubMgr = &stubWorkflowManager{}
	})

	makeDeps := func(pr bool) WorkflowDeps {
		return WorkflowDeps{
			ProjectName:   "test-project",
			PR:            pr,
			Brancher:      stubBr,
			PRCreator:     stubPR,
			PRMerger:      stubPRMerger,
			Cloner:        stubCl,
			Worktreer:     stubWt,
			Releaser:      stubRel,
			PromptManager: stubMgr,
			AutoCompleter: &stubAutoCompleter{},
		}
	}

	newPromptFile := func(branch string) *prompt.PromptFile {
		return prompt.NewPromptFile(
			promptPath,
			prompt.Frontmatter{
				Status: string(prompt.ApprovedPromptStatus),
				Branch: branch,
			},
			[]byte("# Test prompt\n"),
			libtime.NewCurrentDateTime(),
		)
	}

	// 11a: workflow: worktree, pr: true — Setup creates worktree, Complete removes it + pushes + creates PR
	Describe("11a: workflow worktree, pr true", func() {
		It("calls worktreer.Add, worktreer.Remove, brancher.Push, prCreator.Create", func() {
			stubBr.commitsAhead = 1
			deps := makeDeps(true)
			executor := NewWorktreeWorkflowExecutor(deps)
			pf := newPromptFile("feature/test-branch")

			// The Add stub creates the worktree directory so os.Chdir succeeds.
			expectedWorktreePath := filepath.Join(
				os.TempDir(),
				"dark-factory",
				"test-project-001-test",
			)
			stubWt.addStub = func(_ context.Context, path, _ string) error {
				return os.MkdirAll(path, 0750)
			}
			DeferCleanup(func() { _ = os.RemoveAll(expectedWorktreePath) })

			originalDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { _ = os.Chdir(originalDir) })

			err = executor.Setup(ctx, "001-test", pf)
			Expect(err).NotTo(HaveOccurred())
			Expect(stubWt.addCallCount).To(Equal(1))
			Expect(stubWt.addPath).To(Equal(expectedWorktreePath))
			Expect(stubWt.addBranch).To(Equal("feature/test-branch"))

			// Chdir back so Complete can chdir back to originalDir.
			Expect(os.Chdir(originalDir)).To(Succeed())
			Expect(os.Chdir(expectedWorktreePath)).To(Succeed())

			err = executor.Complete(ctx, ctx, pf, "test title", promptPath, completedPath)
			Expect(err).NotTo(HaveOccurred())

			Expect(stubWt.removeCount).To(Equal(1))
			Expect(stubBr.pushCount).To(Equal(1))
			Expect(stubPR.createCount).To(Equal(1))
		})
	})

	// 11b: workflow: worktree, pr: false — Complete removes worktree + pushes but NOT prCreator.Create
	Describe("11b: workflow worktree, pr false", func() {
		It("calls worktreer.Remove and brancher.Push but NOT prCreator.Create", func() {
			stubBr.commitsAhead = 1
			deps := makeDeps(false)
			rawExec, ok := NewWorktreeWorkflowExecutor(deps).(*worktreeWorkflowExecutor)
			Expect(ok).To(BeTrue())
			exec := rawExec
			pf := newPromptFile("feature/no-pr-branch")

			worktreeDir := GinkgoT().TempDir()
			originalDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { _ = os.Chdir(originalDir) })

			// Pre-populate executor state as if Setup had run.
			exec.branchName = "feature/no-pr-branch"
			exec.worktreePath = worktreeDir
			exec.originalDir = originalDir

			// Simulate being inside the worktree.
			Expect(os.Chdir(worktreeDir)).To(Succeed())

			err = exec.Complete(ctx, ctx, pf, "test title", promptPath, completedPath)
			Expect(err).NotTo(HaveOccurred())

			Expect(stubWt.removeCount).To(Equal(1))
			Expect(stubBr.pushCount).To(Equal(1))
			Expect(stubPR.createCount).To(Equal(0))
			Expect(stubMgr.moveToCompletedCount).To(Equal(1))
		})
	})

	// 11c: workflow: branch, pr: true — Setup switches branch, Complete pushes + creates PR
	Describe("11c: workflow branch, pr true", func() {
		It("sets up in-place branch then calls brancher.Push and prCreator.Create", func() {
			deps := makeDeps(true)
			rawExec, ok := NewBranchWorkflowExecutor(deps).(*branchWorkflowExecutor)
			Expect(ok).To(BeTrue())
			exec := rawExec
			pf := newPromptFile("feature/branch-pr")

			err := exec.Setup(ctx, "001-test", pf)
			Expect(err).NotTo(HaveOccurred())
			Expect(stubBr.createAndSwitchCount).To(Equal(1))
			Expect(exec.inPlaceBranch).To(Equal("feature/branch-pr"))
			Expect(exec.inPlaceDefaultBranch).To(Equal("main"))

			err = exec.Complete(ctx, ctx, pf, "test title", promptPath, completedPath)
			Expect(err).NotTo(HaveOccurred())

			Expect(stubBr.pushCount).To(Equal(1))
			Expect(stubPR.createCount).To(Equal(1))
		})
	})

	// 11d: workflow: branch, pr: false — handleBranchCompletion skips push when more prompts queued
	Describe("11d: workflow branch, pr false", func() {
		It("sets up in-place branch then runs handleBranchCompletion without Push", func() {
			deps := makeDeps(false)
			rawExec, ok := NewBranchWorkflowExecutor(deps).(*branchWorkflowExecutor)
			Expect(ok).To(BeTrue())
			exec := rawExec
			pf := newPromptFile("feature/branch-nopr")

			err := exec.Setup(ctx, "001-test", pf)
			Expect(err).NotTo(HaveOccurred())
			Expect(exec.inPlaceBranch).To(Equal("feature/branch-nopr"))

			// HasQueuedPromptsOnBranch returns false (default) → will try to merge
			err = exec.Complete(ctx, ctx, pf, "test title", promptPath, completedPath)
			Expect(err).NotTo(HaveOccurred())

			Expect(stubBr.pushCount).To(Equal(0))
			Expect(stubPR.createCount).To(Equal(0))
		})
	})

	// 11e: workflow: clone, pr: false — Complete removes clone + pushes but NOT prCreator.Create
	Describe("11e: workflow clone, pr false", func() {
		It("calls cloner.Remove, brancher.Push, but NOT prCreator.Create", func() {
			stubBr.commitsAhead = 1
			deps := makeDeps(false)
			rawExec, ok := NewCloneWorkflowExecutor(deps).(*cloneWorkflowExecutor)
			Expect(ok).To(BeTrue())
			exec := rawExec
			pf := newPromptFile("feature/clone-no-pr")

			cloneDir := GinkgoT().TempDir()
			originalDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { _ = os.Chdir(originalDir) })

			// Pre-populate executor state as if Setup had run.
			exec.branchName = "feature/clone-no-pr"
			exec.clonePath = cloneDir
			exec.originalDir = originalDir

			// Simulate being inside the clone.
			Expect(os.Chdir(cloneDir)).To(Succeed())

			err = exec.Complete(ctx, ctx, pf, "test title", promptPath, completedPath)
			Expect(err).NotTo(HaveOccurred())

			Expect(stubCl.removeCount).To(Equal(1))
			Expect(stubBr.pushCount).To(Equal(1))
			Expect(stubPR.createCount).To(Equal(0))
			Expect(stubMgr.moveToCompletedCount).To(Equal(1))
		})
	})

	// 11f: workflow: clone, pr: true — Complete removes clone + pushes + creates PR
	Describe("11f: workflow clone, pr true", func() {
		It("calls cloner.Remove, brancher.Push, and prCreator.Create", func() {
			stubBr.commitsAhead = 1
			deps := makeDeps(true)
			rawExec, ok := NewCloneWorkflowExecutor(deps).(*cloneWorkflowExecutor)
			Expect(ok).To(BeTrue())
			exec := rawExec
			pf := newPromptFile("feature/clone-with-pr")

			cloneDir := GinkgoT().TempDir()
			originalDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { _ = os.Chdir(originalDir) })

			// Pre-populate executor state as if Setup had run.
			exec.branchName = "feature/clone-with-pr"
			exec.clonePath = cloneDir
			exec.originalDir = originalDir

			Expect(os.Chdir(cloneDir)).To(Succeed())

			err = exec.Complete(ctx, ctx, pf, "test title", promptPath, completedPath)
			Expect(err).NotTo(HaveOccurred())

			Expect(stubCl.removeCount).To(Equal(1))
			Expect(stubBr.pushCount).To(Equal(1))
			Expect(stubPR.createCount).To(Equal(1))
		})
	})

	// 11g: handleAfterIsolatedCommit — pr false skips PR creation
	Describe("11g: handleAfterIsolatedCommit — pr false skips PR creation", func() {
		It("pushes branch and moves to completed without creating a PR", func() {
			stubBr.commitsAhead = 1
			deps := makeDeps(false)
			pf := newPromptFile("")

			err := handleAfterIsolatedCommit(
				ctx, ctx, deps, pf,
				"feature/isolated-no-pr",
				"test title",
				promptPath, completedPath,
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(stubBr.pushCount).To(Equal(1))
			Expect(stubPR.createCount).To(Equal(0))
			Expect(stubMgr.moveToCompletedCount).To(Equal(1))
			Expect(stubRel.commitFileCount).To(Equal(1))
		})
	})

	// 11g2: handleAfterIsolatedCommit — zero commits skips push
	Describe("11g2: handleAfterIsolatedCommit — zero commits skips push", func() {
		It("skips push and PR, moves directly to completed", func() {
			stubBr.commitsAhead = 0
			deps := makeDeps(true) // pr=true to verify it's also skipped
			pf := newPromptFile("")

			err := handleAfterIsolatedCommit(
				ctx, ctx, deps, pf,
				"feature/no-changes",
				"test title",
				promptPath, completedPath,
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(stubBr.pushCount).To(Equal(0))
			Expect(stubPR.createCount).To(Equal(0))
			Expect(stubMgr.moveToCompletedCount).To(Equal(1))
			Expect(stubRel.commitFileCount).To(Equal(1))
		})
	})

	// 11h: worktreeWorkflowExecutor.CleanupOnError — restores dir and removes worktree
	Describe("11h: worktreeWorkflowExecutor CleanupOnError", func() {
		It("chdirs back and removes worktree when worktreePath is set", func() {
			deps := makeDeps(false)
			rawExec, ok := NewWorktreeWorkflowExecutor(deps).(*worktreeWorkflowExecutor)
			Expect(ok).To(BeTrue())

			worktreeDir := GinkgoT().TempDir()
			originalDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { _ = os.Chdir(originalDir) })

			rawExec.worktreePath = worktreeDir
			rawExec.originalDir = originalDir

			// Simulate being inside worktree
			Expect(os.Chdir(worktreeDir)).To(Succeed())
			rawExec.CleanupOnError(ctx)

			// Should have called Remove on the worktree
			Expect(stubWt.removeCount).To(Equal(1))
		})

		It("is idempotent when cleanedUp is already true", func() {
			deps := makeDeps(false)
			rawExec, ok := NewWorktreeWorkflowExecutor(deps).(*worktreeWorkflowExecutor)
			Expect(ok).To(BeTrue())
			rawExec.cleanedUp = true
			rawExec.worktreePath = "/some/path"
			rawExec.CleanupOnError(ctx)
			// Remove should NOT have been called since cleanedUp=true
			Expect(stubWt.removeCount).To(Equal(0))
		})
	})

	// 11i: worktreeWorkflowExecutor.ReconstructState — returns false when path missing, true when present
	Describe("11i: worktreeWorkflowExecutor ReconstructState", func() {
		It("returns false when worktree path does not exist", func() {
			deps := makeDeps(false)
			exec := NewWorktreeWorkflowExecutor(deps)
			pf := newPromptFile("")

			canResume, err := exec.ReconstructState(ctx, "001-test", pf)
			Expect(err).NotTo(HaveOccurred())
			Expect(canResume).To(BeFalse())
		})

		It("returns true and sets internal state when worktree path exists", func() {
			deps := makeDeps(false)
			rawExec, ok := NewWorktreeWorkflowExecutor(deps).(*worktreeWorkflowExecutor)
			Expect(ok).To(BeTrue())
			pf := newPromptFile("feature/resume-branch")

			// Create the expected worktree directory
			worktreePath := filepath.Join(os.TempDir(), "dark-factory", "test-project-001-resume")
			Expect(os.MkdirAll(worktreePath, 0750)).To(Succeed())
			DeferCleanup(func() { _ = os.RemoveAll(worktreePath) })

			canResume, err := rawExec.ReconstructState(ctx, "001-resume", pf)
			Expect(err).NotTo(HaveOccurred())
			Expect(canResume).To(BeTrue())
			Expect(rawExec.worktreePath).To(Equal(worktreePath))
			Expect(rawExec.branchName).To(Equal("feature/resume-branch"))
		})
	})

	// 11j: branchWorkflowExecutor.ReconstructState — always returns true
	Describe("11j: branchWorkflowExecutor ReconstructState", func() {
		It("returns true without branch in frontmatter", func() {
			deps := makeDeps(false)
			exec := NewBranchWorkflowExecutor(deps)
			pf := newPromptFile("")

			canResume, err := exec.ReconstructState(ctx, "001-test", pf)
			Expect(err).NotTo(HaveOccurred())
			Expect(canResume).To(BeTrue())
		})

		It("returns true with branch in frontmatter and sets internal state", func() {
			deps := makeDeps(false)
			rawExec, ok := NewBranchWorkflowExecutor(deps).(*branchWorkflowExecutor)
			Expect(ok).To(BeTrue())
			pf := newPromptFile("feature/resume-branch")

			canResume, err := rawExec.ReconstructState(ctx, "001-test", pf)
			Expect(err).NotTo(HaveOccurred())
			Expect(canResume).To(BeTrue())
			Expect(rawExec.inPlaceBranch).To(Equal("feature/resume-branch"))
			Expect(rawExec.inPlaceDefaultBranch).To(Equal("main"))
		})
	})

	// 11k: cloneWorkflowExecutor.CleanupOnError — restores dir and removes clone
	Describe("11k: cloneWorkflowExecutor CleanupOnError", func() {
		It("chdirs back and removes clone when clonePath is set", func() {
			deps := makeDeps(false)
			rawExec, ok := NewCloneWorkflowExecutor(deps).(*cloneWorkflowExecutor)
			Expect(ok).To(BeTrue())

			cloneDir := GinkgoT().TempDir()
			originalDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { _ = os.Chdir(originalDir) })

			rawExec.clonePath = cloneDir
			rawExec.originalDir = originalDir

			Expect(os.Chdir(cloneDir)).To(Succeed())
			rawExec.CleanupOnError(ctx)

			Expect(stubCl.removeCount).To(Equal(1))
		})

		It("is idempotent when cleanedUp is already true", func() {
			deps := makeDeps(false)
			rawExec, ok := NewCloneWorkflowExecutor(deps).(*cloneWorkflowExecutor)
			Expect(ok).To(BeTrue())
			rawExec.cleanedUp = true
			rawExec.clonePath = "/some/path"
			rawExec.CleanupOnError(ctx)
			Expect(stubCl.removeCount).To(Equal(0))
		})
	})

	// 11l: branchWorkflowExecutor.handleBranchPRCompletion — autoMerge=false saves PR URL
	Describe("11l: branchWorkflowExecutor handleBranchPRCompletion autoMerge=false", func() {
		It("pushes branch, creates PR, saves PR URL to frontmatter", func() {
			deps := makeDeps(true) // PR=true
			deps.AutoMerge = false
			rawExec, ok := NewBranchWorkflowExecutor(deps).(*branchWorkflowExecutor)
			Expect(ok).To(BeTrue())
			pf := newPromptFile("feature/branch-pr-no-merge")

			err := rawExec.handleBranchPRCompletion(
				ctx,
				ctx,
				pf,
				"feature/branch-pr-no-merge",
				"test title",
				completedPath,
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(stubBr.pushCount).To(Equal(1))
			Expect(stubPR.createCount).To(Equal(1))
		})
	})

	// 11m: branchWorkflowExecutor.handleBranchCompletion — last prompt on branch merges
	Describe(
		"11m: branchWorkflowExecutor handleBranchCompletion last prompt triggers merge",
		func() {
			It("calls MergeToDefault when no more prompts queued", func() {
				deps := makeDeps(false)
				rawExec, ok := NewBranchWorkflowExecutor(deps).(*branchWorkflowExecutor)
				Expect(ok).To(BeTrue())
				// HasQueuedPromptsOnBranch returns false by default → last prompt → merge
				err := rawExec.handleBranchCompletion(
					ctx,
					ctx,
					promptPath,
					"test title",
					"feature/merge-branch",
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(stubBr.mergeToDefaultCount).To(Equal(1))
			})
		},
	)

	// 11l-hasMore: branchWorkflowExecutor.handleBranchCompletion — skips merge when more prompts
	Describe(
		"11l-hasMore: branchWorkflowExecutor handleBranchCompletion with hasMore=true",
		func() {
			It("skips merge when more prompts queued on branch", func() {
				deps := makeDeps(false)
				stubMgr.hasQueuedOnBranchResult = true // more prompts queued
				rawExec, ok := NewBranchWorkflowExecutor(deps).(*branchWorkflowExecutor)
				Expect(ok).To(BeTrue())

				err := rawExec.handleBranchCompletion(
					ctx,
					ctx,
					promptPath,
					"test title",
					"feature/more-prompts",
				)
				Expect(err).NotTo(HaveOccurred())
				// MergeToDefault should NOT be called when more prompts are queued
				Expect(stubBr.mergeToDefaultCount).To(Equal(0))
			})
		},
	)

	// 11l-autoMerge: branchWorkflowExecutor.handleBranchPRCompletion — autoMerge=true merges PR
	Describe(
		"11l-autoMerge: branchWorkflowExecutor handleBranchPRCompletion autoMerge=true",
		func() {
			It(
				"merges PR and runs postMergeActions when autoMerge=true and no more prompts",
				func() {
					deps := makeDeps(true) // PR=true
					deps.AutoMerge = true
					deps.AutoRelease = false
					stubMgr.hasQueuedOnBranchResult = false // last prompt
					rawExec, ok := NewBranchWorkflowExecutor(deps).(*branchWorkflowExecutor)
					Expect(ok).To(BeTrue())
					pf := newPromptFile("feature/branch-automerge")

					err := rawExec.handleBranchPRCompletion(
						ctx,
						ctx,
						pf,
						"feature/branch-automerge",
						"test title",
						completedPath,
					)
					Expect(err).NotTo(HaveOccurred())

					Expect(stubBr.pushCount).To(Equal(1))
					Expect(stubPR.createCount).To(Equal(1))
					// WaitAndMerge should have been called
					Expect(stubPRMerger.waitAndMergeCount).To(Equal(1))
				},
			)
		},
	)

	// 11n: worktreeWorkflowExecutor.Complete with pr=true — pushes and creates PR
	Describe("11n: worktreeWorkflowExecutor Complete with pr=true", func() {
		It("commits, removes worktree, pushes, and creates PR", func() {
			stubBr.commitsAhead = 1
			deps := makeDeps(true) // PR=true
			rawExec, ok := NewWorktreeWorkflowExecutor(deps).(*worktreeWorkflowExecutor)
			Expect(ok).To(BeTrue())
			pf := newPromptFile("feature/wt-pr")

			worktreeDir := GinkgoT().TempDir()
			originalDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { _ = os.Chdir(originalDir) })

			rawExec.branchName = "feature/wt-pr"
			rawExec.worktreePath = worktreeDir
			rawExec.originalDir = originalDir

			Expect(os.Chdir(worktreeDir)).To(Succeed())

			err = rawExec.Complete(ctx, ctx, pf, "test title", promptPath, completedPath)
			Expect(err).NotTo(HaveOccurred())

			Expect(stubWt.removeCount).To(Equal(1))
			Expect(stubBr.pushCount).To(Equal(1))
			Expect(stubPR.createCount).To(Equal(1))
		})
	})

	// 11o: branchWorkflowExecutor.setupInPlaceBranch — dirty working tree returns error
	Describe("11o: branchWorkflowExecutor setupInPlaceBranch dirty tree", func() {
		It("returns error when working tree is not clean", func() {
			deps := makeDeps(true)
			stubBr.isClean = false
			rawExec, ok := NewBranchWorkflowExecutor(deps).(*branchWorkflowExecutor)
			Expect(ok).To(BeTrue())

			err := rawExec.setupInPlaceBranch(ctx, "feature/dirty")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("working tree is not clean"))
		})
	})

	// 11p: branchWorkflowExecutor.handleBranchPRCompletion — autoMerge=true, hasMore=true defers merge
	Describe(
		"11p: branchWorkflowExecutor handleBranchPRCompletion autoMerge=true hasMore=true",
		func() {
			It("defers merge and saves PR URL when more prompts queued on branch", func() {
				deps := makeDeps(true)
				deps.AutoMerge = true
				stubMgr.hasQueuedOnBranchResult = true // more prompts remain
				rawExec, ok := NewBranchWorkflowExecutor(deps).(*branchWorkflowExecutor)
				Expect(ok).To(BeTrue())
				pf := newPromptFile("feature/deferred-merge")

				err := rawExec.handleBranchPRCompletion(
					ctx,
					ctx,
					pf,
					"feature/deferred-merge",
					"test title",
					completedPath,
				)
				Expect(err).NotTo(HaveOccurred())

				Expect(stubBr.pushCount).To(Equal(1))
				Expect(stubPR.createCount).To(Equal(1))
				// WaitAndMerge should NOT have been called
				Expect(stubPRMerger.waitAndMergeCount).To(Equal(0))
			})
		},
	)

	// 11q: branchWorkflowExecutor.Complete — error in moveToCompletedAndCommit restores default branch
	Describe("11q: branchWorkflowExecutor Complete — error in commit restores branch", func() {
		It("restores default branch when moveToCompletedAndCommit fails", func() {
			deps := makeDeps(false)
			stubRel.commitFileErr = stderrors.New("commit failed")
			rawExec, ok := NewBranchWorkflowExecutor(deps).(*branchWorkflowExecutor)
			Expect(ok).To(BeTrue())
			rawExec.inPlaceBranch = "feature/error-branch"
			rawExec.inPlaceDefaultBranch = "main"
			pf := newPromptFile("feature/error-branch")

			err := rawExec.Complete(ctx, ctx, pf, "test title", promptPath, completedPath)
			Expect(err).To(HaveOccurred())
			// restoreDefaultBranch should have been called (Switch to "main")
			Expect(stubBr.switchCount).To(BeNumerically(">=", 1))
		})
	})

	// 11r: savePRURLToFrontmatter — preserves existing PR URL
	Describe("11r: savePRURLToFrontmatter preserves existing URL", func() {
		It("skips SetPRURL when PR URL already set in frontmatter", func() {
			deps := makeDeps(true)
			// Make Load return a prompt file with an existing PR URL
			stubMgr.existingPRURL = "https://github.com/existing/pull/1"

			savePRURLToFrontmatter(ctx, deps, completedPath, "https://github.com/new/pull/2")

			// SetPRURL should NOT have been called since URL already exists
			Expect(stubMgr.setPRURLCount).To(Equal(0))
		})
	})

	// 11s: directWorkflowExecutor.CleanupOnError — no-op
	Describe("11s: directWorkflowExecutor CleanupOnError no-op", func() {
		It("does not panic and is a no-op", func() {
			deps := makeDeps(false)
			exec := NewDirectWorkflowExecutor(deps)
			exec.CleanupOnError(ctx) // should be a no-op
		})
	})

	// 11t: branchWorkflowExecutor.CleanupOnError — no-op
	Describe("11t: branchWorkflowExecutor CleanupOnError no-op", func() {
		It("does not panic and is a no-op", func() {
			deps := makeDeps(false)
			exec := NewBranchWorkflowExecutor(deps)
			exec.CleanupOnError(ctx) // should be a no-op
		})
	})

	// 11u: worktreeWorkflowExecutor.Setup — Add error is returned
	Describe("11u: worktreeWorkflowExecutor Setup — Add error", func() {
		It("returns error when Worktreer.Add fails", func() {
			deps := makeDeps(false)
			stubWt.addStub = func(_ context.Context, _, _ string) error {
				return stderrors.New("worktree add failed")
			}
			exec := NewWorktreeWorkflowExecutor(deps)
			pf := newPromptFile("feature/wt-add-fail")

			err := exec.Setup(ctx, "001-test", pf)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("add worktree"))
		})
	})

	// 11v: worktreeWorkflowExecutor.Complete — CommitOnly error is returned
	Describe("11v: worktreeWorkflowExecutor Complete — CommitOnly error", func() {
		It("returns error when CommitOnly fails", func() {
			deps := makeDeps(false)
			stubRel.commitOnlyErr = stderrors.New("commit only failed")
			rawExec, ok := NewWorktreeWorkflowExecutor(deps).(*worktreeWorkflowExecutor)
			Expect(ok).To(BeTrue())

			worktreeDir := GinkgoT().TempDir()
			originalDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { _ = os.Chdir(originalDir) })

			rawExec.branchName = "feature/wt-commit-fail"
			rawExec.worktreePath = worktreeDir
			rawExec.originalDir = originalDir

			Expect(os.Chdir(worktreeDir)).To(Succeed())

			pf := newPromptFile("feature/wt-commit-fail")
			err = rawExec.Complete(ctx, ctx, pf, "test title", promptPath, completedPath)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("commit changes"))
		})
	})

	// 11w: worktreeWorkflowExecutor.Setup — syncWithRemoteViaDeps error (fetch fails)
	Describe("11w: worktreeWorkflowExecutor Setup — syncWithRemote error", func() {
		It("returns error when Brancher.Fetch fails", func() {
			deps := makeDeps(false)
			stubBr.fetchErr = stderrors.New("fetch failed")
			exec := NewWorktreeWorkflowExecutor(deps)
			pf := newPromptFile("feature/wt-fetch-fail")

			err := exec.Setup(ctx, "001-test", pf)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("git fetch origin"))
		})
	})

	// 11x: cloneWorkflowExecutor.ReconstructState — returns false when clone path missing
	Describe("11x: cloneWorkflowExecutor ReconstructState — path missing", func() {
		It("returns false when clone path does not exist", func() {
			deps := makeDeps(false)
			exec := NewCloneWorkflowExecutor(deps)
			pf := newPromptFile("")

			canResume, err := exec.ReconstructState(ctx, "001-missing", pf)
			Expect(err).NotTo(HaveOccurred())
			Expect(canResume).To(BeFalse())
		})
	})

	// 11y: cloneWorkflowExecutor.Setup — syncWithRemote error (fetch fails)
	Describe("11y: cloneWorkflowExecutor Setup — syncWithRemote error", func() {
		It("returns error when Brancher.Fetch fails", func() {
			deps := makeDeps(false)
			stubBr.fetchErr = stderrors.New("fetch failed")
			exec := NewCloneWorkflowExecutor(deps)
			pf := newPromptFile("feature/clone-fetch-fail")

			err := exec.Setup(ctx, "001-test", pf)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("git fetch origin"))
		})
	})

	// 11z: cloneWorkflowExecutor.Setup — Clone error is returned
	Describe("11z: cloneWorkflowExecutor Setup — Clone error", func() {
		It("returns error when Cloner.Clone fails", func() {
			deps := makeDeps(false)
			stubCl.cloneStub = func(_ context.Context, _, _, _ string) error {
				return stderrors.New("clone failed")
			}
			exec := NewCloneWorkflowExecutor(deps)
			pf := newPromptFile("feature/clone-error")

			err := exec.Setup(ctx, "001-test", pf)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("clone repo"))
		})
	})

	// 11aa: branchWorkflowExecutor.handleBranchCompletion — HasQueuedPromptsOnBranch error is non-fatal
	Describe(
		"11aa: branchWorkflowExecutor handleBranchCompletion — error in HasQueued is non-fatal",
		func() {
			It("returns nil (non-fatal) when HasQueuedPromptsOnBranch returns error", func() {
				deps := makeDeps(false)
				stubMgr.hasQueuedOnBranchErr = stderrors.New("db error")
				rawExec, ok := NewBranchWorkflowExecutor(deps).(*branchWorkflowExecutor)
				Expect(ok).To(BeTrue())

				err := rawExec.handleBranchCompletion(
					ctx,
					ctx,
					promptPath,
					"test title",
					"feature/err-branch",
				)
				Expect(err).NotTo(HaveOccurred())
				// MergeToDefault should NOT have been called due to non-fatal bail-out
				Expect(stubBr.mergeToDefaultCount).To(Equal(0))
			})
		},
	)

	// 11ab: cloneWorkflowExecutor.Setup — default branch name when frontmatter branch is empty
	Describe("11ab: cloneWorkflowExecutor Setup — default branch from baseName", func() {
		It("uses dark-factory/<baseName> when prompt branch is empty", func() {
			deps := makeDeps(false)
			// Clone creates a real dir so chdir can succeed
			stubCl.cloneStub = func(_ context.Context, _, destDir, _ string) error {
				return os.MkdirAll(destDir, 0750)
			}
			rawExec, ok := NewCloneWorkflowExecutor(deps).(*cloneWorkflowExecutor)
			Expect(ok).To(BeTrue())
			pf := newPromptFile("") // empty branch

			originalDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { _ = os.Chdir(originalDir) })

			err = rawExec.Setup(ctx, "001-test", pf)
			Expect(err).NotTo(HaveOccurred())
			Expect(rawExec.branchName).To(Equal("dark-factory/001-test"))

			DeferCleanup(func() { _ = os.RemoveAll(rawExec.clonePath) })
		})
	})

	// 11ac: worktreeWorkflowExecutor.Setup — default branch from baseName
	Describe("11ac: worktreeWorkflowExecutor Setup — default branch from baseName", func() {
		It("uses dark-factory/<baseName> when prompt branch is empty", func() {
			deps := makeDeps(false)
			stubWt.addStub = func(_ context.Context, path, _ string) error {
				return os.MkdirAll(path, 0750)
			}
			rawExec, ok := NewWorktreeWorkflowExecutor(deps).(*worktreeWorkflowExecutor)
			Expect(ok).To(BeTrue())
			pf := newPromptFile("") // empty branch

			originalDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { _ = os.Chdir(originalDir) })

			err = rawExec.Setup(ctx, "001-test", pf)
			Expect(err).NotTo(HaveOccurred())
			Expect(rawExec.branchName).To(Equal("dark-factory/001-test"))

			DeferCleanup(func() { _ = os.RemoveAll(rawExec.worktreePath) })
		})
	})

	// 11ad: worktreeWorkflowExecutor.Complete — Remove fails (warn path, not error)
	Describe("11ad: worktreeWorkflowExecutor Complete — Remove fails (warn, not error)", func() {
		It("continues without error when Worktreer.Remove fails during Complete", func() {
			deps := makeDeps(false)
			stubWt.removeStub = func(_ context.Context, _ string) error {
				return stderrors.New("remove failed")
			}
			rawExec, ok := NewWorktreeWorkflowExecutor(deps).(*worktreeWorkflowExecutor)
			Expect(ok).To(BeTrue())

			worktreeDir := GinkgoT().TempDir()
			originalDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { _ = os.Chdir(originalDir) })

			rawExec.branchName = "feature/wt-remove-fail"
			rawExec.worktreePath = worktreeDir
			rawExec.originalDir = originalDir

			Expect(os.Chdir(worktreeDir)).To(Succeed())

			pf := newPromptFile("feature/wt-remove-fail")
			err = rawExec.Complete(ctx, ctx, pf, "test title", promptPath, completedPath)
			Expect(err).NotTo(HaveOccurred()) // warn only, not error
		})
	})

	// 11ae: worktreeWorkflowExecutor.CleanupOnError — Remove fails (warn path)
	Describe(
		"11ae: worktreeWorkflowExecutor CleanupOnError — Remove fails (warn, not error)",
		func() {
			It("does not panic when Remove fails during CleanupOnError", func() {
				deps := makeDeps(false)
				stubWt.removeStub = func(_ context.Context, _ string) error {
					return stderrors.New("remove failed")
				}
				rawExec, ok := NewWorktreeWorkflowExecutor(deps).(*worktreeWorkflowExecutor)
				Expect(ok).To(BeTrue())

				worktreeDir := GinkgoT().TempDir()
				originalDir, err := os.Getwd()
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(func() { _ = os.Chdir(originalDir) })

				rawExec.worktreePath = worktreeDir
				rawExec.originalDir = originalDir

				Expect(os.Chdir(worktreeDir)).To(Succeed())
				rawExec.CleanupOnError(ctx) // should be a no-panic even when Remove fails
			})
		},
	)

	// 11af: cloneWorkflowExecutor.CleanupOnError — Remove fails (warn path)
	Describe("11af: cloneWorkflowExecutor CleanupOnError — Remove fails (warn, not error)", func() {
		It("does not panic when Remove fails during CleanupOnError", func() {
			deps := makeDeps(false)
			stubCl.removeStub = func(_ context.Context, _ string) error {
				return stderrors.New("remove failed")
			}
			rawExec, ok := NewCloneWorkflowExecutor(deps).(*cloneWorkflowExecutor)
			Expect(ok).To(BeTrue())

			cloneDir := GinkgoT().TempDir()
			originalDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { _ = os.Chdir(originalDir) })

			rawExec.clonePath = cloneDir
			rawExec.originalDir = originalDir

			Expect(os.Chdir(cloneDir)).To(Succeed())
			rawExec.CleanupOnError(ctx)
		})
	})

	// 11ag: branchWorkflowExecutor.setupInPlaceBranch — IsClean returns error
	Describe("11ag: branchWorkflowExecutor setupInPlaceBranch — IsClean error", func() {
		It("returns error when IsClean fails", func() {
			deps := makeDeps(true)
			stubBr.isCleanErr = stderrors.New("is-clean error")
			rawExec, ok := NewBranchWorkflowExecutor(deps).(*branchWorkflowExecutor)
			Expect(ok).To(BeTrue())

			err := rawExec.setupInPlaceBranch(ctx, "feature/is-clean-err")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("check working tree"))
		})
	})

	// 11ah: branchWorkflowExecutor.setupInPlaceBranch — DefaultBranch returns error
	Describe("11ah: branchWorkflowExecutor setupInPlaceBranch — DefaultBranch error", func() {
		It("returns error when DefaultBranch fails", func() {
			deps := makeDeps(true)
			stubBr.isClean = true
			stubBr.defaultBranchErr = stderrors.New("default-branch error")
			rawExec, ok := NewBranchWorkflowExecutor(deps).(*branchWorkflowExecutor)
			Expect(ok).To(BeTrue())

			err := rawExec.setupInPlaceBranch(ctx, "feature/default-branch-err")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("get default branch"))
		})
	})

	// 11ai: branchWorkflowExecutor.handleBranchPRCompletion — WaitAndMerge error
	Describe("11ai: branchWorkflowExecutor handleBranchPRCompletion — WaitAndMerge error", func() {
		It("returns error when WaitAndMerge fails in autoMerge mode", func() {
			deps := makeDeps(true)
			deps.AutoMerge = true
			stubMgr.hasQueuedOnBranchResult = false // last prompt → proceed to merge
			stubPRMerger.waitAndMergeErr = stderrors.New("merge failed")
			rawExec, ok := NewBranchWorkflowExecutor(deps).(*branchWorkflowExecutor)
			Expect(ok).To(BeTrue())
			pf := newPromptFile("feature/automerge-fail")

			err := rawExec.handleBranchPRCompletion(
				ctx,
				ctx,
				pf,
				"feature/automerge-fail",
				"test title",
				completedPath,
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("wait and merge PR"))
		})
	})
})

var _ = Describe("ResumeCommitting", func() {
	var (
		ctx          context.Context
		tempDir      string
		queueDir     string
		completedDir string
		originalDir  string
		mgr          *stubManager
		rel          *stubWorkflowReleaser
		proc         *processor
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		originalDir, err = os.Getwd()
		Expect(err).NotTo(HaveOccurred())

		tempDir, err = os.MkdirTemp("", "resume-committing-test-*")
		Expect(err).NotTo(HaveOccurred())

		queueDir = filepath.Join(tempDir, "in-progress")
		completedDir = filepath.Join(tempDir, "completed")
		Expect(os.MkdirAll(queueDir, 0750)).To(Succeed())
		Expect(os.MkdirAll(completedDir, 0750)).To(Succeed())

		mgr = &stubManager{}
		rel = &stubWorkflowReleaser{}

		proc = &processor{
			dirs:           Dirs{Queue: queueDir, Completed: completedDir},
			promptManager:  mgr,
			releaser:       rel,
			autoCompleter:  &stubAutoCompleter{},
			skippedPrompts: make(map[string]libtime.DateTime),
		}
	})

	AfterEach(func() {
		_ = os.Chdir(originalDir)
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	Context("when there are no committing prompts", func() {
		It("returns nil without any git operations", func() {
			// findCommittingFunc not set → returns nil → no prompts processed
			err := proc.ResumeCommitting(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(rel.commitFileCount).To(Equal(0))
		})
	})

	Context("when FindCommitting returns an error", func() {
		BeforeEach(func() {
			mgr.findCommittingFunc = func(_ context.Context) ([]string, error) {
				return nil, stderrors.New("scan failed")
			}
		})

		It("returns nil (non-fatal, logs warn)", func() {
			err := proc.ResumeCommitting(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(rel.commitFileCount).To(Equal(0))
		})
	})

	Context("when committing prompt exists and git repo is clean (no dirty files)", func() {
		BeforeEach(func() {
			// Set up a real git repo so HasDirtyFiles succeeds.
			repoDir := filepath.Join(tempDir, "repo")
			Expect(os.MkdirAll(repoDir, 0750)).To(Succeed())

			for _, args := range [][]string{
				{"init"},
				{"config", "user.email", "test@example.com"},
				{"config", "user.name", "Test User"},
			} {
				cmd := exec.Command("git", args...)
				cmd.Dir = repoDir
				Expect(cmd.Run()).To(Succeed())
			}
			// Initial commit so the repo is not empty
			initFile := filepath.Join(repoDir, "README.md")
			Expect(os.WriteFile(initFile, []byte("# test\n"), 0600)).To(Succeed())
			for _, args := range [][]string{
				{"add", "-A"},
				{"commit", "-m", "initial"},
			} {
				cmd := exec.Command("git", args...)
				cmd.Dir = repoDir
				Expect(cmd.Run()).To(Succeed())
			}
			Expect(os.Chdir(repoDir)).To(Succeed())

			promptPath := filepath.Join(queueDir, "001-clean.md")
			Expect(
				os.WriteFile(
					promptPath,
					[]byte("---\nstatus: committing\n---\n# Clean prompt\n"),
					0600,
				),
			).To(Succeed())

			pf := prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{Status: "committing"},
				[]byte("# Clean prompt\n"),
				libtime.NewCurrentDateTime(),
			)
			mgr.findCommittingFunc = func(_ context.Context) ([]string, error) {
				return []string{promptPath}, nil
			}
			mgr.loadFunc = func(_ context.Context, _ string) (*prompt.PromptFile, error) {
				return pf, nil
			}
		})

		It("skips work commit, calls MoveToCompleted, calls CommitCompletedFile", func() {
			err := proc.ResumeCommitting(ctx)
			Expect(err).NotTo(HaveOccurred())
			// No dirty files means CommitAll was not called; CommitCompletedFile was called
			Expect(rel.commitFileCount).To(Equal(1))
		})
	})

	Context("when committing prompt exists and git repo has dirty files", func() {
		BeforeEach(func() {
			// Set up a real git repo with dirty files.
			repoDir := filepath.Join(tempDir, "repo2")
			Expect(os.MkdirAll(repoDir, 0750)).To(Succeed())

			for _, args := range [][]string{
				{"init"},
				{"config", "user.email", "test@example.com"},
				{"config", "user.name", "Test User"},
			} {
				cmd := exec.Command("git", args...)
				cmd.Dir = repoDir
				Expect(cmd.Run()).To(Succeed())
			}
			// Initial commit
			initFile := filepath.Join(repoDir, "README.md")
			Expect(os.WriteFile(initFile, []byte("# test\n"), 0600)).To(Succeed())
			for _, args := range [][]string{
				{"add", "-A"},
				{"commit", "-m", "initial"},
			} {
				cmd := exec.Command("git", args...)
				cmd.Dir = repoDir
				Expect(cmd.Run()).To(Succeed())
			}
			// Add dirty file
			Expect(
				os.WriteFile(filepath.Join(repoDir, "dirty.txt"), []byte("dirty\n"), 0600),
			).To(Succeed())
			Expect(os.Chdir(repoDir)).To(Succeed())

			promptPath := filepath.Join(queueDir, "001-dirty.md")
			Expect(
				os.WriteFile(
					promptPath,
					[]byte("---\nstatus: committing\n---\n# Dirty prompt\n"),
					0600,
				),
			).To(Succeed())

			pf := prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{Status: "committing"},
				[]byte("# Dirty prompt\n"),
				libtime.NewCurrentDateTime(),
			)
			mgr.findCommittingFunc = func(_ context.Context) ([]string, error) {
				return []string{promptPath}, nil
			}
			mgr.loadFunc = func(_ context.Context, _ string) (*prompt.PromptFile, error) {
				return pf, nil
			}
		})

		It("commits dirty work files, calls MoveToCompleted, calls CommitCompletedFile", func() {
			err := proc.ResumeCommitting(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(rel.commitFileCount).To(Equal(1))
		})
	})
})
