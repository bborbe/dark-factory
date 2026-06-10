// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package committingrecoverer_test

import (
	"context"
	stderrors "errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
	pushBranchErr             error
	pushBranchCalled          int
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

func (s *stubReleaser) PushBranch(_ context.Context) error {
	s.pushBranchCalled++
	return s.pushBranchErr
}

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

// resolveGitToplevel returns the absolute path of the git toplevel of cwd.
// Read-only: runs `git rev-parse --show-toplevel` with no mutation of any repo.
func resolveGitToplevel() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// assertNotInRealRepo returns an error if cwd's git toplevel equals realRepoRoot.
// The error message names realRepoRoot so a developer reading the failure can
// locate the missing sandbox chdir. Read-only — does not mutate any repo.
//
// Returning an error (instead of calling Ginkgo's Fail directly) keeps the
// suite green for the negative-evidence spec: that spec wraps the call in
// InterceptGomegaFailure and asserts the error is non-nil, instead of letting
// the guard's failure cascade into a real spec failure.
func assertNotInRealRepo(realRepoRoot string) error {
	if realRepoRoot == "" {
		// No resolvable real repo at suite start (hideGit YOLO container
		// masks .git) — there is nothing to pollute, guard trivially holds.
		return nil
	}
	currentToplevel, err := resolveGitToplevel()
	if err != nil {
		// Cannot determine toplevel — likely not in a git repo at all. That's
		// fine for specs that have chdir'd into a sandbox repo: the sandbox
		// itself IS a git repo, so this branch is unreachable for well-behaved
		// specs. If we reach it, fail loud.
		return fmt.Errorf(
			"committingrecoverer spec could not resolve git toplevel from cwd (%s); "+
				"sandbox chdir may be missing — expected cwd inside a sandbox git repo, "+
				"got cwd whose git toplevel is the real repository (%s); "+
				"add the sandbox chdir before reaching the package-level dirty/commit helpers",
			realRepoRoot,
			realRepoRoot,
		)
	}
	if currentToplevel == realRepoRoot {
		return fmt.Errorf(
			"committingrecoverer spec ran with cwd inside the real repository (%s); "+
				"add the sandbox chdir before reaching the package-level dirty/commit helpers",
			realRepoRoot,
		)
	}
	return nil
}

var _ = Describe("Recoverer", func() {
	var (
		ctx          context.Context
		cancel       context.CancelFunc
		tempDir      string
		completedDir string
		repoDir      string
		realRepoRoot string
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

		// Resolve the real repo root BEFORE any chdir, so we can compare
		// current cwd's git toplevel against it from the guard. This is
		// read-only — runs `git rev-parse --show-toplevel` from the cwd at
		// suite start (the package source dir, inside the real repo).
		// In a hideGit YOLO container the repo's .git is masked (character
		// device) and resolution fails — there is then no reachable real
		// repo to pollute, so the guard is trivially satisfied: keep
		// realRepoRoot empty and assertNotInRealRepo passes through.
		realRepoRoot, err = git.ResolveGitRoot(ctx)
		if err != nil {
			realRepoRoot = ""
		}

		tempDir, err = os.MkdirTemp("", "committingrecoverer-test-*")
		Expect(err).NotTo(HaveOccurred())

		completedDir = filepath.Join(tempDir, "completed")
		Expect(os.MkdirAll(completedDir, 0750)).To(Succeed())

		// Sandbox every spec: create a temp git repo under tempDir and chdir
		// into it. This guarantees that any spec in this Describe block runs
		// with cwd inside an isolated git repo, never the real one. The
		// per-spec Recover specs that build their own repoDir will chdir
		// again into that sub-repo, which is still under tempDir and still
		// a sandbox.
		repoDir = filepath.Join(tempDir, "repo")
		Expect(os.MkdirAll(repoDir, 0750)).To(Succeed())
		initGitRepo(repoDir)
		Expect(os.Chdir(repoDir)).To(Succeed())

		mgr = &stubPromptManager{}
		rel = &stubReleaser{}
		ac = &stubAutoCompleter{}
		rec = committingrecoverer.NewRecoverer(mgr, rel, ac, completedDir, false)

		// Suite-level regression guard: every spec in this Describe block
		// reaches Recover() (directly or via RecoverAll). If cwd resolved
		// to the real repo at this point, the sandbox chdir above was
		// bypassed — fail loudly with a message naming the real-repo path
		// so the missing chdir is easy to locate.
		Expect(assertNotInRealRepo(realRepoRoot)).NotTo(
			HaveOccurred(),
			"sandbox chdir is missing — cwd must not be inside the real repository before reaching the package-level dirty/commit helpers",
		)
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

	// Negative-evidence spec: proves the suite-level guard fires with a
	// message naming the real repo when a Recover()-reaching spec runs with
	// cwd inside the real repository. This is the AC-5 evidence: a future
	// regression that removes the sandbox chdir will be caught loudly.
	// The test asserts the guard's error is non-nil (not a Gomega failure),
	// so the suite stays green — it asserts the guard FIRES, not that the
	// production code path mutates the real repo.
	It("guard fails when cwd is the real repo", func() {
		if realRepoRoot == "" {
			Skip("no resolvable real repo (hideGit YOLO container masks .git) — guard trivially satisfied, nothing to demonstrate")
		}
		// Temporarily chdir BACK into the real repo source dir, so cwd's
		// git toplevel equals realRepoRoot and the guard's equality check
		// matches.
		Expect(os.Chdir(originalDir)).To(Succeed())
		defer func() {
			// Restore cwd to the sandbox so the next BeforeEach (or
			// AfterEach) is unaffected. Use repoDir from the BeforeEach
			// if available, else fall back to originalDir.
			_ = os.Chdir(repoDir)
		}()

		// assertNotInRealRepo returns an error (instead of calling Ginkgo's
		// Fail directly) so the negative-evidence test can assert on the
		// error rather than failing the spec.
		err := assertNotInRealRepo(realRepoRoot)
		Expect(err).To(HaveOccurred(), "guard must fail when cwd is the real repo")
		Expect(err.Error()).To(
			ContainSubstring(realRepoRoot),
			"guard failure message must name the real-repo cwd path",
		)
	})
})

var _ = Describe("Recoverer autoRelease push matrix", func() {
	type matrixCase struct {
		autoRelease    bool
		wantPushBranch int
	}
	cases := []matrixCase{
		{autoRelease: false, wantPushBranch: 0},
		{autoRelease: true, wantPushBranch: 1},
	}

	for _, tc := range cases {
		desc := "autoRelease=false"
		if tc.autoRelease {
			desc = "autoRelease=true"
		}
		It(desc+" pushes or skips as expected", func() {
			ctx := context.Background()
			var err error

			originalDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())

			tempDir, err := os.MkdirTemp("", "recoverer-push-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tempDir) }()

			completedDir := filepath.Join(tempDir, "completed")
			Expect(os.MkdirAll(completedDir, 0750)).To(Succeed())

			// Set up a real git repo so HasDirtyFiles works
			repoDir := filepath.Join(tempDir, "repo")
			Expect(os.MkdirAll(repoDir, 0750)).To(Succeed())
			initGitRepo(repoDir)
			Expect(os.Chdir(repoDir)).To(Succeed())
			defer func() { _ = os.Chdir(originalDir) }()

			promptPath := filepath.Join(tempDir, "001-push-test.md")
			Expect(
				os.WriteFile(
					promptPath,
					[]byte("---\nstatus: committing\n---\n# Push test\n"),
					0600,
				),
			).To(Succeed())

			mgr := &stubPromptManager{}
			mgr.loadFunc = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return makePromptFile(path), nil
			}
			rel := &stubReleaser{}
			ac := &stubAutoCompleter{}

			rec := committingrecoverer.NewRecoverer(mgr, rel, ac, completedDir, tc.autoRelease)
			err = rec.Recover(ctx, promptPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(rel.pushBranchCalled).To(Equal(tc.wantPushBranch), "PushBranch call count")
		})
	}
})
