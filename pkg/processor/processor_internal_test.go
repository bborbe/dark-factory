// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor

import (
	"context"
	"os"
	"path/filepath"
	"time"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/prompt"
)

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
			skippedPrompts: make(map[string]time.Time),
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

func (s *stubExecutor) Reattach(ctx context.Context, logFile string, containerName string) error {
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
			skippedPrompts: make(map[string]time.Time),
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
			worktree:       false,
			skippedPrompts: make(map[string]time.Time),
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
			proc.worktree = true
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
			proc.worktree = true
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
			proc.worktree = true
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
