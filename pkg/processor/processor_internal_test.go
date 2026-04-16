// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor

import (
	"context"
	stderrors "errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/notifier"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/report"
)

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

var _ = Describe("sanitizeContainerName", func() {
	It("keeps valid characters", func() {
		name := sanitizeContainerName("abc-123_XYZ")
		Expect(name).To(Equal("abc-123_XYZ"))
	})

	It("replaces special characters with hyphens", func() {
		name := sanitizeContainerName("test@file#name")
		Expect(name).To(Equal("test-file-name"))
	})

	It("handles spaces", func() {
		name := sanitizeContainerName("hello world")
		Expect(name).To(Equal("hello-world"))
	})

	It("handles multiple consecutive special characters", func() {
		name := sanitizeContainerName("test@@##name")
		Expect(name).To(Equal("test----name"))
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
	loadFunc func(ctx context.Context, path string) (*prompt.PromptFile, error)
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
			queueDir:       queueDir,
			logDir:         logDir,
			projectName:    "test-project",
			executor:       fakeExec,
			promptManager:  mgr,
			workflow:       config.WorkflowDirect,
			skippedPrompts: make(map[string]libtime.DateTime),
		}
	})

	AfterEach(func() {
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	Context("when queueDir does not exist", func() {
		It("returns nil without error", func() {
			proc.queueDir = filepath.Join(tempDir, "nonexistent")
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
			proc.workflow = config.WorkflowClone
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
			proc.workflow = config.WorkflowClone
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
			proc.workflow = config.WorkflowClone
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
		p      *processor
	)

	BeforeEach(func() {
		ctx = context.Background()
		gitCtx = context.Background()
		rel = &stubReleaser{nextVersion: "v1.1.0"}
		p = &processor{
			releaser:       rel,
			skippedPrompts: make(map[string]libtime.DateTime),
		}
	})

	Context("with CHANGELOG present and autoRelease disabled", func() {
		BeforeEach(func() {
			p.autoRelease = false
			rel.hasChangelog = true
		})

		It("calls CommitOnly and does not call CommitAndRelease", func() {
			err := p.handleDirectWorkflow(gitCtx, ctx, "test title", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(rel.commitOnlyCalled).To(Equal(1))
			Expect(rel.commitAndRelCalled).To(Equal(0))
		})
	})

	Context("with CHANGELOG present and autoRelease enabled", func() {
		BeforeEach(func() {
			p.autoRelease = true
			rel.hasChangelog = true
		})

		It("calls CommitAndRelease and does not call CommitOnly", func() {
			err := p.handleDirectWorkflow(gitCtx, ctx, "test title", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(rel.commitAndRelCalled).To(Equal(1))
			Expect(rel.commitOnlyCalled).To(Equal(0))
		})
	})

	Context("without CHANGELOG (autoRelease value irrelevant)", func() {
		BeforeEach(func() {
			p.autoRelease = true
			rel.hasChangelog = false
		})

		It("calls CommitOnly regardless of autoRelease", func() {
			err := p.handleDirectWorkflow(gitCtx, ctx, "test title", "")
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

var _ = Describe("validateCompletionReport", func() {
	var (
		ctx     context.Context
		tempDir string
		logFile string
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		tempDir, err = os.MkdirTemp("", "vcr-test-*")
		Expect(err).NotTo(HaveOccurred())
		logFile = filepath.Join(tempDir, "test.log")
	})

	AfterEach(func() {
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	writeLog := func(content string) {
		Expect(os.WriteFile(logFile, []byte(content), 0600)).To(Succeed())
	}

	It("returns non-nil report with summary for valid success report", func() {
		writeLog(`Starting session...

<!-- DARK-FACTORY-REPORT
{"status":"success","summary":"Added ScanForCriticalFailures","blockers":[],"verification":{"command":"make precommit","exitCode":0}}
DARK-FACTORY-REPORT -->
`)
		r, err := validateCompletionReport(ctx, logFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(r).NotTo(BeNil())
		Expect(r.Summary).To(Equal("Added ScanForCriticalFailures"))
		Expect(r.Status).To(Equal("success"))
	})

	It("returns error for partial status report with failing verification exit code", func() {
		writeLog(`Starting session...

<!-- DARK-FACTORY-REPORT
{"status":"success","summary":"half done","blockers":[],"verification":{"command":"make precommit","exitCode":1}}
DARK-FACTORY-REPORT -->
`)
		r, err := validateCompletionReport(ctx, logFile)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("completion report status: partial"))
		Expect(r).To(BeNil())
	})

	It("returns error for failed status report", func() {
		writeLog(`Starting session...

<!-- DARK-FACTORY-REPORT
{"status":"failed","summary":"could not complete","blockers":["tests failing"]}
DARK-FACTORY-REPORT -->
`)
		r, err := validateCompletionReport(ctx, logFile)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("completion report status: failed"))
		Expect(r).To(BeNil())
	})

	It("returns (nil, nil) when log has no report and no critical failure", func() {
		writeLog("Starting session...\nsome output\n--- DONE ---\n")
		r, err := validateCompletionReport(ctx, logFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(r).To(BeNil())
	})

	It("returns (nil, error) when log has no report but contains auth error pattern", func() {
		writeLog(`Starting headless session...
[18:31:29] Failed to authenticate. API Error: 401 {"type":"error","error":{"type":"authentication_error","message":"Invalid authentication credentials"}}

[18:31:29] --- DONE ---
`)
		r, err := validateCompletionReport(ctx, logFile)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("claude CLI critical failure"))
		Expect(r).To(BeNil())
	})

	It("returns (nil, error) when log has no report but contains API Error: 500", func() {
		writeLog(
			"Starting headless session...\nAPI Error: 500 Internal Server Error\n--- DONE ---\n",
		)
		r, err := validateCompletionReport(ctx, logFile)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("claude CLI critical failure"))
		Expect(r).To(BeNil())
	})

	It(
		"returns (nil, nil) for malformed JSON completion report (parse error downgraded to no-report)",
		func() {
			writeLog(`Starting session...

<!-- DARK-FACTORY-REPORT
{this is not valid json}
DARK-FACTORY-REPORT -->
`)
			r, err := validateCompletionReport(ctx, logFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(r).To(BeNil())
		},
	)

	It("returns (nil, error) when log is malformed JSON AND contains auth-error pattern", func() {
		writeLog(`Starting headless session...
Failed to authenticate. API Error: 401

<!-- DARK-FACTORY-REPORT
{this is not valid json}
DARK-FACTORY-REPORT -->
`)
		r, err := validateCompletionReport(ctx, logFile)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("claude CLI critical failure"))
		Expect(r).To(BeNil())
	})

	It("returns non-nil report using the report package types correctly", func() {
		writeLog(`Starting session...

<!-- DARK-FACTORY-REPORT
{"status":"success","summary":"Fixed auth detection","blockers":[],"verification":{"command":"make precommit","exitCode":0}}
DARK-FACTORY-REPORT -->
`)
		r, err := validateCompletionReport(ctx, logFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(r).To(BeAssignableToTypeOf(&report.CompletionReport{}))
		Expect(r.Verification).NotTo(BeNil())
		Expect(r.Verification.ExitCode).To(Equal(0))
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
	pushCount            int
	createAndSwitchCount int
	fetchAndVerifyErr    error
	isClean              bool
	isCleanErr           error
	defaultBranch        string
	defaultBranchErr     error
	switchErr            error
	mergeToDefaultCount  int
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

func (s *stubBrancher) Switch(_ context.Context, _ string) error { return s.switchErr }

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

func (s *stubBrancher) Fetch(_ context.Context) error { return nil }

func (s *stubBrancher) Pull(_ context.Context) error { return nil }

func (s *stubBrancher) MergeOriginDefault(_ context.Context) error { return nil }

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

// stubWorkflowReleaser is a minimal git.Releaser for workflow routing tests.
type stubWorkflowReleaser struct {
	commitOnlyCount       int
	commitFileCount       int
	hasChangelog          bool
	commitAndReleaseCount int
}

func (s *stubWorkflowReleaser) CommitOnly(_ context.Context, _ string) error {
	s.commitOnlyCount++
	return nil
}

func (s *stubWorkflowReleaser) CommitCompletedFile(_ context.Context, _ string) error {
	s.commitFileCount++
	return nil
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

func (s *stubWorkflowManager) Load(_ context.Context, _ string) (*prompt.PromptFile, error) {
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

func (s *stubWorkflowManager) SetPRURL(_ context.Context, _, _ string) error { return nil }

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

var _ = Describe("processor workflow routing", func() {
	var (
		ctx           context.Context
		projectDir    string
		completedDir  string
		promptPath    string
		completedPath string

		stubWt  *stubWorktreer
		stubCl  *stubCloner
		stubBr  *stubBrancher
		stubPR  *stubPRCreator
		stubRel *stubWorkflowReleaser
		stubMgr *stubWorkflowManager
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
		stubRel = &stubWorkflowReleaser{}
		stubMgr = &stubWorkflowManager{}
	})

	newProc := func(workflow config.Workflow, pr bool) *processor {
		return &processor{
			queueDir:       projectDir,
			completedDir:   completedDir,
			logDir:         filepath.Join(projectDir, "logs"),
			projectName:    "test-project",
			workflow:       workflow,
			pr:             pr,
			brancher:       stubBr,
			prCreator:      stubPR,
			cloner:         stubCl,
			worktreer:      stubWt,
			releaser:       stubRel,
			promptManager:  stubMgr,
			notifier:       notifier.NewMultiNotifier(),
			skippedPrompts: make(map[string]libtime.DateTime),
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

	// 11a: workflow: worktree, pr: true
	Describe("11a: workflow worktree, pr true", func() {
		It("calls worktreer.Add, worktreer.Remove, brancher.Push, prCreator.Create", func() {
			p := newProc(config.WorkflowWorktree, true)
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
			// Ensure the worktree directory is removed after the test to avoid polluting
			// other tests that check for the directory's non-existence.
			DeferCleanup(func() { _ = os.RemoveAll(expectedWorktreePath) })

			originalDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { _ = os.Chdir(originalDir) })

			state := &workflowState{}
			returnedState, err := p.setupWorktreeWorkflowState(ctx, "001-test", pf, state)
			Expect(err).NotTo(HaveOccurred())
			Expect(stubWt.addCallCount).To(Equal(1))
			Expect(stubWt.addPath).To(Equal(expectedWorktreePath))
			Expect(stubWt.addBranch).To(Equal("feature/test-branch"))

			// Override originalDir so handleWorktreeWorkflow can chdir back.
			returnedState.originalDir = originalDir

			err = p.handleWorktreeWorkflow(
				ctx,
				ctx,
				pf,
				"test title",
				promptPath,
				completedPath,
				returnedState,
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(stubWt.removeCount).To(Equal(1))
			Expect(stubBr.pushCount).To(Equal(1))
			Expect(stubPR.createCount).To(Equal(1))
		})
	})

	// 11b: workflow: worktree, pr: false
	Describe("11b: workflow worktree, pr false", func() {
		It("calls worktreer.Remove and brancher.Push but NOT prCreator.Create", func() {
			p := newProc(config.WorkflowWorktree, false)
			pf := newPromptFile("feature/no-pr-branch")

			worktreeDir := GinkgoT().TempDir()

			originalDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { _ = os.Chdir(originalDir) })

			state := &workflowState{
				branchName:   "feature/no-pr-branch",
				worktreePath: worktreeDir,
				originalDir:  originalDir,
			}

			// Simulate being inside the worktree
			Expect(os.Chdir(worktreeDir)).To(Succeed())

			err = p.handleWorktreeWorkflow(
				ctx,
				ctx,
				pf,
				"test title",
				promptPath,
				completedPath,
				state,
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(stubWt.removeCount).To(Equal(1))
			Expect(stubBr.pushCount).To(Equal(1))
			Expect(stubPR.createCount).To(Equal(0))
			Expect(stubMgr.moveToCompletedCount).To(Equal(1))
		})
	})

	// 11c: workflow: branch, pr: true
	Describe("11c: workflow branch, pr true", func() {
		It("sets up in-place branch then calls brancher.Push and prCreator.Create", func() {
			p := newProc(config.WorkflowBranch, true)

			state := &workflowState{}
			returnedState, err := p.setupInPlaceBranchState(ctx, "feature/branch-pr", state)
			Expect(err).NotTo(HaveOccurred())
			Expect(stubBr.createAndSwitchCount).To(Equal(1))
			Expect(returnedState.inPlaceBranch).To(Equal("feature/branch-pr"))
			Expect(returnedState.inPlaceDefaultBranch).To(Equal("main"))

			pf := newPromptFile("feature/branch-pr")
			err = p.handleBranchPRCompletion(
				ctx,
				ctx,
				pf,
				"feature/branch-pr",
				"test title",
				completedPath,
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(stubBr.pushCount).To(Equal(1))
			Expect(stubPR.createCount).To(Equal(1))
		})
	})

	// 11d: workflow: branch, pr: false
	Describe("11d: workflow branch, pr false", func() {
		It("sets up in-place branch then runs handleBranchCompletion without Push", func() {
			p := newProc(config.WorkflowBranch, false)

			state := &workflowState{}
			returnedState, err := p.setupInPlaceBranchState(ctx, "feature/branch-nopr", state)
			Expect(err).NotTo(HaveOccurred())
			Expect(returnedState.inPlaceBranch).To(Equal("feature/branch-nopr"))

			err = p.handleBranchCompletion(
				ctx,
				ctx,
				promptPath,
				"test title",
				"feature/branch-nopr",
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(stubBr.pushCount).To(Equal(0))
			Expect(stubPR.createCount).To(Equal(0))
		})
	})

	// 11e: workflow: clone, pr: false
	Describe("11e: workflow clone, pr false", func() {
		It("calls cloner.Remove, brancher.Push, but NOT prCreator.Create", func() {
			p := newProc(config.WorkflowClone, false)
			pf := newPromptFile("feature/clone-no-pr")

			cloneDir := GinkgoT().TempDir()

			originalDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { _ = os.Chdir(originalDir) })

			// Simulate being inside the clone
			Expect(os.Chdir(cloneDir)).To(Succeed())

			state := &workflowState{
				branchName:  "feature/clone-no-pr",
				clonePath:   cloneDir,
				originalDir: originalDir,
			}

			err = p.handleCloneWorkflow(
				ctx,
				ctx,
				pf,
				"test title",
				promptPath,
				completedPath,
				state,
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(stubCl.removeCount).To(Equal(1))
			Expect(stubBr.pushCount).To(Equal(1))
			Expect(stubPR.createCount).To(Equal(0))
			Expect(stubMgr.moveToCompletedCount).To(Equal(1))
		})
	})

	// 11f: workflow: clone, pr: true
	Describe("11f: workflow clone, pr true", func() {
		It("calls cloner.Remove, brancher.Push, and prCreator.Create", func() {
			p := newProc(config.WorkflowClone, true)
			pf := newPromptFile("feature/clone-with-pr")

			cloneDir := GinkgoT().TempDir()

			originalDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { _ = os.Chdir(originalDir) })

			Expect(os.Chdir(cloneDir)).To(Succeed())

			state := &workflowState{
				branchName:  "feature/clone-with-pr",
				clonePath:   cloneDir,
				originalDir: originalDir,
			}

			err = p.handleCloneWorkflow(
				ctx,
				ctx,
				pf,
				"test title",
				promptPath,
				completedPath,
				state,
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(stubCl.removeCount).To(Equal(1))
			Expect(stubBr.pushCount).To(Equal(1))
			Expect(stubPR.createCount).To(Equal(1))
		})
	})

	// 11g: handleAfterIsolatedCommit — pr false skips PR creation
	Describe("11g: handleAfterIsolatedCommit — pr false skips PR creation", func() {
		It("pushes branch and moves to completed without creating a PR", func() {
			p := newProc(config.WorkflowClone, false)
			pf := newPromptFile("")

			err := p.handleAfterIsolatedCommit(
				ctx, ctx, pf,
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
})
