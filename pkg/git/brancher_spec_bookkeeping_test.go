// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/git"
)

// Regression: StashInPaths must ignore a prefix with no change, including one
// whose directory the repo never created. Splitting the empty
// `git status --porcelain` output yields [""], which is neither "??" nor "!!",
// so the prefix was added to the stash pathspec and `git stash push` failed
// with `pathspec ... did not match any file(s) known to git`.
var _ = Describe("Brancher StashInPaths", func() {
	const stashMessage = "dark-factory: spec bookkeeping for cross-branch switch"

	var (
		ctx         context.Context
		repoDir     string
		originalDir string
		brancher    git.Brancher
	)

	gitIn := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), string(out))
		return strings.TrimSpace(string(out))
	}

	writeIn := func(rel, content string) {
		full := filepath.Join(repoDir, rel)
		Expect(os.MkdirAll(filepath.Dir(full), 0o750)).To(Succeed())
		Expect(os.WriteFile(full, []byte(content), 0o600)).To(Succeed())
	}

	BeforeEach(func() {
		ctx = context.Background()

		var err error
		originalDir, err = os.Getwd()
		Expect(err).NotTo(HaveOccurred())

		repoDir = GinkgoT().TempDir()
		gitIn("init", "-b", "master", ".")
		gitIn("config", "user.email", "test@example.com")
		gitIn("config", "user.name", "Test User")
		writeIn("specs/in-progress/040-page-view.md", "---\nstatus: approved\n---\n")
		gitIn("add", ".")
		gitIn("commit", "-m", "init")

		Expect(os.Chdir(repoDir)).To(Succeed())
		brancher = git.NewBrancher()
	})

	AfterEach(func() {
		if originalDir != "" {
			Expect(os.Chdir(originalDir)).To(Succeed())
		}
	})

	It("stashes the dirty prefix and skips a prefix whose directory is absent", func() {
		writeIn("specs/in-progress/040-page-view.md", "---\nstatus: prompted\n---\n")

		stashed, err := brancher.StashInPaths(
			ctx,
			[]string{"specs/in-progress", "specs/rejected"},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(stashed).To(BeTrue())

		subjects := gitIn("stash", "list", "--format=%s")
		Expect(strings.Split(subjects, "\n")).To(HaveLen(1))
		Expect(subjects).To(ContainSubstring(stashMessage))

		Expect(
			strings.Split(gitIn("stash", "show", "--name-only"), "\n"),
		).To(ConsistOf("specs/in-progress/040-page-view.md"))
		Expect(gitIn("status", "--porcelain")).To(BeEmpty())
	})

	It("creates no stash when every prefix is clean or absent", func() {
		stashed, err := brancher.StashInPaths(
			ctx,
			[]string{"specs/in-progress", "specs/rejected"},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(stashed).To(BeFalse())
		Expect(gitIn("stash", "list")).To(BeEmpty())
	})

	It("creates no stash for an untracked-only prefix", func() {
		writeIn("specs/rejected/041-old.md", "---\nstatus: rejected\n---\n")

		stashed, err := brancher.StashInPaths(ctx, []string{"specs/rejected"})
		Expect(err).NotTo(HaveOccurred())
		Expect(stashed).To(BeFalse())
		Expect(gitIn("stash", "list")).To(BeEmpty())
	})
})

// Regression: StashPop must not fail the workflow when the branch it switches
// onto carries a different committed version of the same bookkeeping file. The
// stash's diff is taken against the source branch, so popping it onto the
// target branch has no common base and `git stash pop` conflicts — observed in
// production as `restore stashed bookkeeping: : git stash pop -- failed: exit
// status 1` on a vault-ui run whose spec declared a branch with divergent
// `specs/`.
var _ = Describe("Brancher StashPop across a branch switch", func() {
	const specPath = "specs/in-progress/040-page-view.md"

	var (
		ctx         context.Context
		repoDir     string
		originalDir string
		brancher    git.Brancher
	)

	gitIn := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), string(out))
		return strings.TrimSpace(string(out))
	}

	writeIn := func(rel, content string) {
		full := filepath.Join(repoDir, rel)
		Expect(os.MkdirAll(filepath.Dir(full), 0o750)).To(Succeed())
		Expect(os.WriteFile(full, []byte(content), 0o600)).To(Succeed())
	}

	readIn := func(rel string) string {
		content, err := os.ReadFile(filepath.Join(repoDir, rel))
		Expect(err).NotTo(HaveOccurred())
		return string(content)
	}

	BeforeEach(func() {
		ctx = context.Background()

		var err error
		originalDir, err = os.Getwd()
		Expect(err).NotTo(HaveOccurred())

		repoDir = GinkgoT().TempDir()
		gitIn("init", "-b", "master", ".")
		gitIn("config", "user.email", "test@example.com")
		gitIn("config", "user.name", "Test User")
		writeIn(specPath, "---\nstatus: approved\n---\n")
		gitIn("add", ".")
		gitIn("commit", "-m", "init")

		// The target branch carries a divergent committed version of the same
		// spec, so a stash taken on master cannot be applied onto it cleanly.
		gitIn("checkout", "-b", "test/spec-040")
		writeIn(specPath, "---\nstatus: verifying\n---\n")
		gitIn("commit", "-am", "branch edit")
		gitIn("checkout", "master")

		Expect(os.Chdir(repoDir)).To(Succeed())
		brancher = git.NewBrancher()
	})

	AfterEach(func() {
		if originalDir != "" {
			Expect(os.Chdir(originalDir)).To(Succeed())
		}
	})

	It("keeps the checked-out branch's version and drops the stash instead of failing", func() {
		writeIn(specPath, "---\nstatus: prompted\n---\n")

		stashed, err := brancher.StashInPaths(ctx, []string{"specs/in-progress"})
		Expect(err).NotTo(HaveOccurred())
		Expect(stashed).To(BeTrue())

		Expect(brancher.Switch(ctx, "test/spec-040")).To(Succeed())

		// Unfixed code returns `restore stashed bookkeeping: ... exit status 1`.
		Expect(brancher.StashPop(ctx)).To(Succeed())

		Expect(gitIn("stash", "list")).To(BeEmpty())
		Expect(readIn(specPath)).To(Equal("---\nstatus: verifying\n---\n"))
		Expect(gitIn("status", "--porcelain")).To(BeEmpty())
	})

	It("still restores a stash that applies cleanly", func() {
		writeIn(specPath, "---\nstatus: prompted\n---\n")

		stashed, err := brancher.StashInPaths(ctx, []string{"specs/in-progress"})
		Expect(err).NotTo(HaveOccurred())
		Expect(stashed).To(BeTrue())

		Expect(brancher.StashPop(ctx)).To(Succeed())

		Expect(gitIn("stash", "list")).To(BeEmpty())
		Expect(readIn(specPath)).To(Equal("---\nstatus: prompted\n---\n"))
		Expect(gitIn("status", "--porcelain")).To(ContainSubstring(specPath))
	})

	It("keeps the branch's deletion when the branch deleted the spec", func() {
		// The checked-out branch has no version of the file at all, so there is
		// nothing to check out — the branch's deletion must win.
		gitIn("checkout", "-b", "test/spec-040-deleted", "master")
		Expect(os.Remove(filepath.Join(repoDir, specPath))).To(Succeed())
		gitIn("commit", "-am", "delete spec")
		gitIn("checkout", "master")

		writeIn(specPath, "---\nstatus: prompted\n---\n")

		stashed, err := brancher.StashInPaths(ctx, []string{"specs/in-progress"})
		Expect(err).NotTo(HaveOccurred())
		Expect(stashed).To(BeTrue())

		Expect(brancher.Switch(ctx, "test/spec-040-deleted")).To(Succeed())

		Expect(brancher.StashPop(ctx)).To(Succeed())

		Expect(gitIn("stash", "list")).To(BeEmpty())
		Expect(filepath.Join(repoDir, specPath)).NotTo(BeAnExistingFile())
		Expect(gitIn("status", "--porcelain")).To(BeEmpty())
	})
})
