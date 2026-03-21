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
}

func (s *stubExecutor) Execute(_ context.Context, _ string, _ string, _ string) error { return nil }

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
