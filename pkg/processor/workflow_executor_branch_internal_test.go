// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor

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

// Regression: the generator's own spec-status write must not trip the
// branch-switch cleanliness gate.
var _ = Describe("branchWorkflowExecutor spec bookkeeping", func() {
	var (
		ctx         context.Context
		repoDir     string
		originalDir string
		executor    *branchWorkflowExecutor
	)

	gitIn := func(dir string, args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), string(out))
		return strings.TrimSpace(string(out))
	}

	writeIn := func(dir, rel, content string) {
		full := filepath.Join(dir, rel)
		Expect(os.MkdirAll(filepath.Dir(full), 0o750)).To(Succeed())
		Expect(os.WriteFile(full, []byte(content), 0o600)).To(Succeed())
	}

	BeforeEach(func() {
		ctx = context.Background()

		var err error
		originalDir, err = os.Getwd()
		Expect(err).NotTo(HaveOccurred())

		remoteDir := GinkgoT().TempDir()
		gitIn(remoteDir, "init", "--bare", "-b", "master", ".")

		repoDir = GinkgoT().TempDir()
		gitIn(repoDir, "init", "-b", "master", ".")
		gitIn(repoDir, "config", "user.email", "test@example.com")
		gitIn(repoDir, "config", "user.name", "Test User")
		gitIn(repoDir, "remote", "add", "origin", remoteDir)

		writeIn(repoDir, "README.md", "base\n")
		writeIn(repoDir, "specs/in-progress/040-page-view.md", "---\nstatus: approved\n---\n")
		gitIn(repoDir, "add", ".")
		gitIn(repoDir, "commit", "-m", "init")
		gitIn(repoDir, "push", "origin", "master")

		gitIn(repoDir, "checkout", "-b", "feat/page-view")
		gitIn(repoDir, "push", "origin", "feat/page-view")

		// The generator's own write, same file and transition as
		// pkg/generator/generator.go:289.
		writeIn(repoDir, "specs/in-progress/040-page-view.md", "---\nstatus: prompted\n---\n")

		Expect(os.Chdir(repoDir)).To(Succeed())

		executor = &branchWorkflowExecutor{deps: WorkflowDeps{
			Brancher:                git.NewBrancher(git.WithDefaultBranch("master")),
			IgnorePathPrefixes:      []string{"prompts"},
			SpecBookkeepingPrefixes: []string{"specs/in-progress"},
		}}
	})

	AfterEach(func() {
		if originalDir != "" {
			Expect(os.Chdir(originalDir)).To(Succeed())
		}
	})

	It("commits the spec-status write into the branch history instead of refusing", func() {
		Expect(executor.setupInPlaceBranch(ctx, "feat/page-view")).To(Succeed())

		// SC2: the write is in the branch's own history, carrying the
		// post-transition value.
		Expect(gitIn(repoDir,
			"log", "--oneline", "feat/page-view", "--", "specs/in-progress/040-page-view.md",
		)).To(ContainSubstring("chore: persist spec status transition"))
		Expect(gitIn(repoDir, "show", "feat/page-view:specs/in-progress/040-page-view.md")).
			To(ContainSubstring("status: prompted"))

		// The commit landed on the current branch, which is the target branch.
		Expect(gitIn(repoDir, "rev-parse", "--abbrev-ref", "HEAD")).To(Equal("feat/page-view"))
	})

	It("leaves an untracked new spec file uncommitted, so the gate still refuses it", func() {
		writeIn(repoDir, "specs/in-progress/041-new-draft.md", "---\nstatus: draft\n---\n")

		err := executor.setupInPlaceBranch(ctx, "feat/page-view")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("041-new-draft.md"))

		// The untracked file was not swept into the bookkeeping commit.
		Expect(gitIn(repoDir, "status", "--porcelain", "--", "specs/in-progress/041-new-draft.md")).
			To(ContainSubstring("??"))
		Expect(gitIn(repoDir,
			"log", "--oneline", "feat/page-view", "--", "specs/in-progress/041-new-draft.md",
		)).To(BeEmpty())

		// The tracked spec-status write was still committed.
		Expect(gitIn(repoDir,
			"log", "--oneline", "feat/page-view", "--", "specs/in-progress/040-page-view.md",
		)).To(ContainSubstring("chore: persist spec status transition"))
	})

	It(
		"lands the write on the target branch when switching onto it, not on the current branch",
		func() {
			// HEAD moves off the target branch while the spec write stays dirty.
			gitIn(repoDir, "checkout", "master")

			Expect(executor.setupInPlaceBranch(ctx, "feat/page-view")).To(Succeed())

			// The write landed on the target branch's history, carrying the
			// post-transition value.
			Expect(gitIn(repoDir,
				"log", "--oneline", "feat/page-view", "--", "specs/in-progress/040-page-view.md",
			)).To(ContainSubstring("chore: persist spec status transition"))
			Expect(gitIn(repoDir, "show", "feat/page-view:specs/in-progress/040-page-view.md")).
				To(ContainSubstring("status: prompted"))

			// The write must not be stranded on master.
			Expect(gitIn(repoDir,
				"log", "--oneline", "master", "--", "specs/in-progress/040-page-view.md",
			)).NotTo(ContainSubstring("chore: persist spec status transition"))

			// We end on the target branch.
			Expect(gitIn(repoDir, "rev-parse", "--abbrev-ref", "HEAD")).To(Equal("feat/page-view"))
		},
	)

	It("does not consume a pre-existing unrelated stash", func() {
		gitIn(repoDir, "checkout", "master")
		// Stash an unrelated change on a different file, leaving the spec write dirty.
		writeIn(repoDir, "README.md", "unrelated\n")
		gitIn(repoDir, "stash", "push", "-m", "unrelated-user-stash", "--", "README.md")

		Expect(executor.setupInPlaceBranch(ctx, "feat/page-view")).To(Succeed())

		// Our bookkeeping stash was popped; the user's unrelated stash survives.
		Expect(gitIn(repoDir, "stash", "list", "--format=%s")).
			To(ContainSubstring("unrelated-user-stash"))
		// The spec write still landed on the target branch.
		Expect(gitIn(repoDir,
			"log", "--oneline", "feat/page-view", "--", "specs/in-progress/040-page-view.md",
		)).To(ContainSubstring("chore: persist spec status transition"))
	})

	It("still refuses a genuinely dirty tree and names the offending file", func() {
		writeIn(repoDir, "README.md", "user edit\n")

		err := executor.setupInPlaceBranch(ctx, "feat/page-view")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("working tree is not clean"))
		Expect(err.Error()).To(ContainSubstring("README.md"))
		Expect(err.Error()).NotTo(ContainSubstring("040-page-view.md"))
	})
})
