// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git_test

import (
	"context"
	"os"
	"os/exec"

	berrors "github.com/bborbe/errors"
	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/subproc"
)

var _ = Describe("Runner injection and stderr/exit-code preservation", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("RunnerInjected — cloner.Clone routes all spawns through the injected runner", func() {
		fakeRunner := &mocks.SubprocRunner{}
		// All Run* methods default-return (nil, nil) — sufficient for Clone's happy path.
		c := git.NewClonerWithRunnerForTest(fakeRunner)

		// destDir must not exist so removeStale is a no-op.
		err := c.Clone(
			ctx,
			"/fake/src",
			"/tmp/nonexistent-clone-dest-999888",
			"dark-factory/my-branch",
		)
		Expect(err).NotTo(HaveOccurred())

		// gitClone + setRealRemote(get-url + set-url) + fetch + rev-parse + checkoutTrack/New
		total := fakeRunner.RunWithWarnAndTimeoutCallCount() + fakeRunner.RunWithWarnAndTimeoutDirCallCount()
		Expect(total).To(BeNumerically(">=", 1))
	})

	It(
		"RunnerInjected — brancher.CreateAndSwitch routes the spawn through the injected runner",
		func() {
			fakeRunner := &mocks.SubprocRunner{}
			b := git.NewBrancherWithRunnerForTest(fakeRunner)

			err := b.CreateAndSwitch(ctx, "dark-factory/my-branch")
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeRunner.RunWithWarnAndTimeoutCallCount()).To(Equal(1))
		},
	)

	It(
		"RunnerInjected — worktreer.Add routes at least one spawn through the injected runner",
		func() {
			fakeRunner := &mocks.SubprocRunner{}
			w := git.NewWorktreerWithRunnerForTest(fakeRunner)

			err := w.Add(ctx, "/tmp/fake-worktree", "dark-factory/my-branch")
			Expect(err).NotTo(HaveOccurred())

			// rev-parse check + worktree add = 2 calls
			Expect(fakeRunner.RunWithWarnAndTimeoutCallCount()).To(BeNumerically(">=", 1))
		},
	)

	It(
		"RunnerInjected — releaser.CommitOnly routes at least one spawn through the injected runner",
		func() {
			fakeRunner := &mocks.SubprocRunner{}
			r := git.NewReleaserWithRunnerForTest(fakeRunner)

			// stageAllAndCheck runs git status --porcelain; empty output → no staged files → early return.
			_ = r.CommitOnly(ctx, "test commit")

			Expect(fakeRunner.RunWithWarnAndTimeoutCallCount()).To(BeNumerically(">=", 1))
		},
	)

	It(
		"RunnerInjected — ghRepoNameFetcher.Fetch routes the spawn through the injected runner",
		func() {
			fakeRunner := &mocks.SubprocRunner{}
			fakeRunner.RunWithWarnAndTimeoutEnvReturns([]byte("owner/repo\n"), nil)
			f := git.NewGHRepoNameFetcherWithRunnerForTest("", fakeRunner)

			name, err := f.Fetch(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(name).To(Equal("owner/repo"))
			Expect(fakeRunner.RunWithWarnAndTimeoutEnvCallCount()).To(Equal(1))
		},
	)

	It(
		"RunnerInjected — ghCollaboratorLister.List routes the spawn through the injected runner",
		func() {
			fakeRunner := &mocks.SubprocRunner{}
			fakeRunner.RunWithWarnAndTimeoutEnvReturns([]byte("alice\nbob\n"), nil)
			l := git.NewGHCollaboratorListerWithRunnerForTest("", fakeRunner)

			names, err := l.List(ctx, "owner/repo")
			Expect(err).NotTo(HaveOccurred())
			Expect(names).To(ConsistOf("alice", "bob"))
			Expect(fakeRunner.RunWithWarnAndTimeoutEnvCallCount()).To(Equal(1))
		},
	)

	It(
		"RunnerInjected — prMerger is constructable with an injected runner",
		func() {
			fakeRunner := &mocks.SubprocRunner{}
			merger := git.NewPRMergerWithRunnerForTest("", libtime.NewCurrentDateTime(), fakeRunner)
			Expect(merger).NotTo(BeNil())
		},
	)

	It(
		"TruncateStderr — StderrFromError caps oversized child stderr at 8 KiB and appends (truncated)",
		func() {
			// Run a real command that writes 16 KiB to stderr; capture via cmd.Output() so
			// ExitError.Stderr is populated (same mechanism used inside subproc.Runner).
			cmd := exec.Command(
				"sh",
				"-c",
				`head -c 16384 /dev/zero | tr '\0' 'X' >&2; exit 1`,
			) //nolint:gosec
			_, runErr := cmd.Output()
			Expect(runErr).To(HaveOccurred())

			// Wrap as the runner does: bborbe/errors.Wrapf adds a layer around the *exec.ExitError.
			wrapped := berrors.Wrapf(ctx, runErr, "subprocess test command")
			result := git.StderrFromError(wrapped)

			Expect(result).To(HaveSuffix(" (truncated)"))
			Expect(len(result)).To(BeNumerically("<=", 8192+len(" (truncated)")))
		},
	)

	It(
		"ExitCodePropagated — *exec.ExitError exit code survives bborbe/errors wrapping end-to-end",
		func() {
			// Run git status in a directory that is NOT a git repo — git exits with code 128.
			tmpDir, err := os.MkdirTemp("", "exit-code-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			r := subproc.NewRunner()
			_, runErr := r.RunWithWarnAndTimeoutDir(ctx, "git status", tmpDir, "git", "status")
			Expect(runErr).To(HaveOccurred())

			var exitErr *exec.ExitError
			Expect(
				berrors.As(runErr, &exitErr),
			).To(BeTrue(), "error should unwrap to *exec.ExitError")
			Expect(exitErr.ExitCode()).To(BeNumerically(">", 0))
		},
	)
})
