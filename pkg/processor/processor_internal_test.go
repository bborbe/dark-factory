// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor

import (
	"context"
	stderrors "errors"
	"os"
	"path/filepath"

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

var _ = Describe("ResumeExecuting delegates to Resumer", func() {
	It("calls resumer.ResumeAll and returns its result", func() {
		ctx := context.Background()
		fakeResumer := &stubResumer{err: stderrors.New("resumer error")}
		p := &processor{
			resumer: fakeResumer,
		}
		err := p.ResumeExecuting(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("resumer error"))
		Expect(fakeResumer.callCount).To(Equal(1))
	})
})

// stubResumer is a minimal promptresumer.Resumer stub for delegation tests.
type stubResumer struct {
	callCount int
	err       error
}

func (s *stubResumer) ResumeAll(_ context.Context) error {
	s.callCount++
	return s.err
}

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

var _ = Describe("ResumeCommitting delegates to CommittingRecoverer", func() {
	It("calls committingRecoverer.RecoverAll and always returns nil", func() {
		ctx := context.Background()
		fakeRecoverer := &stubCommittingRecoverer{}
		p := &processor{
			committingRecoverer: fakeRecoverer,
		}
		err := p.ResumeCommitting(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeRecoverer.recoverAllCalled).To(Equal(1))
	})
})

// stubCommittingRecoverer is a minimal committingrecoverer.Recoverer stub for delegation tests.
type stubCommittingRecoverer struct {
	recoverAllCalled int
}

func (s *stubCommittingRecoverer) RecoverAll(_ context.Context) {
	s.recoverAllCalled++
}

func (s *stubCommittingRecoverer) Recover(_ context.Context, _ string) error {
	return nil
}
