// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package committingrecoverer_test

import (
	"context"
	stderrors "errors"
	"os"
	"os/exec"
	"path/filepath"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/committingrecoverer"
	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/prompt"
)

// --- minimal stubs ---

type stubPromptManager struct {
	findCommittingFunc    func(ctx context.Context) ([]string, error)
	loadFunc              func(ctx context.Context, path string) (*prompt.PromptFile, error)
	moveToCompletedErr    error
	moveToCompletedCalled int
}

func (s *stubPromptManager) FindCommitting(ctx context.Context) ([]string, error) {
	if s.findCommittingFunc != nil {
		return s.findCommittingFunc(ctx)
	}
	return nil, nil //nolint:nilnil
}

func (s *stubPromptManager) Load(ctx context.Context, path string) (*prompt.PromptFile, error) {
	if s.loadFunc != nil {
		return s.loadFunc(ctx, path)
	}
	return nil, nil //nolint:nilnil
}

func (s *stubPromptManager) MoveToCompleted(_ context.Context, _ string) error {
	s.moveToCompletedCalled++
	return s.moveToCompletedErr
}

type stubReleaser struct {
	commitCompletedFileErr    error
	commitCompletedFileCalled int
}

func (s *stubReleaser) CommitCompletedFile(_ context.Context, _ string) error {
	s.commitCompletedFileCalled++
	return s.commitCompletedFileErr
}

func (s *stubReleaser) GetNextVersion(_ context.Context, _ git.VersionBump) (string, error) {
	return "v1.0.0", nil
}

func (s *stubReleaser) CommitAndRelease(_ context.Context, _ git.VersionBump) error {
	return nil
}

func (s *stubReleaser) CommitOnly(_ context.Context, _ string) error { return nil }

func (s *stubReleaser) HasChangelog(_ context.Context) bool { return false }

func (s *stubReleaser) MoveFile(_ context.Context, _, _ string) error { return nil }

type stubAutoCompleter struct {
	checkAndCompleteErr    error
	checkAndCompleteCalled int
}

func (s *stubAutoCompleter) CheckAndComplete(_ context.Context, _ string) error {
	s.checkAndCompleteCalled++
	return s.checkAndCompleteErr
}

// makePromptFile creates a PromptFile for test use.
func makePromptFile(path string) *prompt.PromptFile {
	return prompt.NewPromptFile(
		path,
		prompt.Frontmatter{Status: "committing"},
		[]byte("# Test prompt\n"),
		libtime.NewCurrentDateTime(),
	)
}

// initGitRepo sets up a minimal git repo with an initial commit and returns to originalDir on cleanup.
func initGitRepo(repoDir string) {
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test User"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		Expect(cmd.Run()).To(Succeed())
	}
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
}

var _ = Describe("Recoverer", func() {
	var (
		ctx          context.Context
		cancel       context.CancelFunc
		tempDir      string
		completedDir string
		mgr          *stubPromptManager
		rel          *stubReleaser
		ac           *stubAutoCompleter
		rec          committingrecoverer.Recoverer
		originalDir  string
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		var err error
		originalDir, err = os.Getwd()
		Expect(err).NotTo(HaveOccurred())

		tempDir, err = os.MkdirTemp("", "committingrecoverer-test-*")
		Expect(err).NotTo(HaveOccurred())

		completedDir = filepath.Join(tempDir, "completed")
		Expect(os.MkdirAll(completedDir, 0750)).To(Succeed())

		mgr = &stubPromptManager{}
		rel = &stubReleaser{}
		ac = &stubAutoCompleter{}
		rec = committingrecoverer.NewRecoverer(mgr, rel, ac, completedDir)
	})

	AfterEach(func() {
		cancel()
		_ = os.Chdir(originalDir)
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	Describe("RecoverAll", func() {
		It("is a no-op when FindCommitting returns empty list", func() {
			mgr.findCommittingFunc = func(_ context.Context) ([]string, error) {
				return []string{}, nil
			}
			rec.RecoverAll(ctx)
			Expect(rel.commitCompletedFileCalled).To(Equal(0))
			Expect(mgr.moveToCompletedCalled).To(Equal(0))
		})

		It("logs and swallows when FindCommitting returns error", func() {
			mgr.findCommittingFunc = func(_ context.Context) ([]string, error) {
				return nil, stderrors.New("scan failed")
			}
			// Should not panic
			rec.RecoverAll(ctx)
			Expect(rel.commitCompletedFileCalled).To(Equal(0))
		})

		It("stops iteration when ctx is cancelled", func() {
			called := 0
			mgr.findCommittingFunc = func(_ context.Context) ([]string, error) {
				return []string{"/queue/001.md", "/queue/002.md"}, nil
			}
			mgr.loadFunc = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				called++
				cancel() // cancel on first call
				return makePromptFile(path), nil
			}
			rec.RecoverAll(ctx)
			// After cancel on first item, loop should exit before processing second
			Expect(called).To(BeNumerically("<=", 1))
		})

		It("logs error and continues when Recover fails for one prompt", func() {
			mgr.findCommittingFunc = func(_ context.Context) ([]string, error) {
				return []string{"/queue/001.md"}, nil
			}
			mgr.loadFunc = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return nil, stderrors.New("load failed")
			}
			// Should not panic or return error
			rec.RecoverAll(ctx)
		})
	})

	Describe("Recover", func() {
		It("returns error when Load fails", func() {
			mgr.loadFunc = func(_ context.Context, _ string) (*prompt.PromptFile, error) {
				return nil, stderrors.New("load error")
			}
			err := rec.Recover(ctx, "/queue/001-test.md")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("load committing prompt"))
		})

		It("returns error when MoveToCompleted fails", func() {
			// Need a clean git repo so HasDirtyFiles works.
			repoDir := filepath.Join(tempDir, "repo-mvfail")
			Expect(os.MkdirAll(repoDir, 0750)).To(Succeed())
			initGitRepo(repoDir)
			Expect(os.Chdir(repoDir)).To(Succeed())

			promptPath := filepath.Join(tempDir, "001-mv-fail.md")
			Expect(
				os.WriteFile(promptPath, []byte("---\nstatus: committing\n---\n# Test\n"), 0600),
			).To(Succeed())

			mgr.loadFunc = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return makePromptFile(path), nil
			}
			mgr.moveToCompletedErr = stderrors.New("move failed")

			err := rec.Recover(ctx, promptPath)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("move to completed during recovery"))
		})

		It("returns error when CommitCompletedFile fails", func() {
			repoDir := filepath.Join(tempDir, "repo-commitfail")
			Expect(os.MkdirAll(repoDir, 0750)).To(Succeed())
			initGitRepo(repoDir)
			Expect(os.Chdir(repoDir)).To(Succeed())

			promptPath := filepath.Join(tempDir, "001-commit-fail.md")
			Expect(
				os.WriteFile(promptPath, []byte("---\nstatus: committing\n---\n# Test\n"), 0600),
			).To(Succeed())

			mgr.loadFunc = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return makePromptFile(path), nil
			}
			rel.commitCompletedFileErr = stderrors.New("commit file failed")

			err := rec.Recover(ctx, promptPath)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("commit completed file during recovery"))
		})

		It("skips work commit and succeeds when no dirty files", func() {
			repoDir := filepath.Join(tempDir, "repo-clean")
			Expect(os.MkdirAll(repoDir, 0750)).To(Succeed())
			initGitRepo(repoDir)
			Expect(os.Chdir(repoDir)).To(Succeed())

			promptPath := filepath.Join(tempDir, "001-clean.md")
			Expect(
				os.WriteFile(promptPath, []byte("---\nstatus: committing\n---\n# Clean\n"), 0600),
			).To(Succeed())

			mgr.loadFunc = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return makePromptFile(path), nil
			}

			err := rec.Recover(ctx, promptPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(mgr.moveToCompletedCalled).To(Equal(1))
			Expect(rel.commitCompletedFileCalled).To(Equal(1))
		})

		It("commits dirty files then succeeds", func() {
			repoDir := filepath.Join(tempDir, "repo-dirty")
			Expect(os.MkdirAll(repoDir, 0750)).To(Succeed())
			initGitRepo(repoDir)
			// Add a dirty (untracked) file
			Expect(
				os.WriteFile(filepath.Join(repoDir, "dirty.txt"), []byte("dirty\n"), 0600),
			).To(Succeed())
			Expect(os.Chdir(repoDir)).To(Succeed())

			promptPath := filepath.Join(tempDir, "001-dirty.md")
			Expect(
				os.WriteFile(promptPath, []byte("---\nstatus: committing\n---\n# Dirty\n"), 0600),
			).To(Succeed())

			mgr.loadFunc = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return makePromptFile(path), nil
			}

			err := rec.Recover(ctx, promptPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(mgr.moveToCompletedCalled).To(Equal(1))
			Expect(rel.commitCompletedFileCalled).To(Equal(1))
		})

		It("logs warning and continues when spec auto-complete fails", func() {
			repoDir := filepath.Join(tempDir, "repo-specfail")
			Expect(os.MkdirAll(repoDir, 0750)).To(Succeed())
			initGitRepo(repoDir)
			Expect(os.Chdir(repoDir)).To(Succeed())

			promptPath := filepath.Join(tempDir, "001-spec-fail.md")
			Expect(
				os.WriteFile(
					promptPath,
					[]byte("---\nstatus: committing\n---\n# Spec fail\n"),
					0600,
				),
			).To(Succeed())

			pf := prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{Status: "committing", Specs: []string{"001-spec"}},
				[]byte("# Spec fail\n"),
				libtime.NewCurrentDateTime(),
			)
			mgr.loadFunc = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return pf, nil
			}
			ac.checkAndCompleteErr = stderrors.New("spec complete failed")

			// Should not return error — spec failure is logged and swallowed
			err := rec.Recover(ctx, promptPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(ac.checkAndCompleteCalled).To(Equal(1))
			Expect(mgr.moveToCompletedCalled).To(Equal(1))
		})
	})
})
