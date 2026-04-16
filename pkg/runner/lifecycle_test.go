// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runner_test

import (
	"context"
	stderrors "errors"
	"os"
	"path/filepath"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/runner"
)

var _ = Describe("startupSequence", func() {
	var (
		ctx          context.Context
		cancel       context.CancelFunc
		tempDir      string
		deps         runner.StartupDepsForTest
		mgr          *mocks.RunnerPromptManager
		slugMigrator *mocks.SpecSlugMigrator
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		var err error
		tempDir, err = os.MkdirTemp("", "lifecycle-test-*")
		Expect(err).NotTo(HaveOccurred())

		promptsBase := filepath.Join(tempDir, "prompts")
		specsBase := filepath.Join(tempDir, "specs")

		mgr = &mocks.RunnerPromptManager{}
		mgr.NormalizeFilenamesReturns(nil, nil)

		containerChecker := &mocks.ContainerChecker{}

		slugMigrator = &mocks.SpecSlugMigrator{}
		slugMigrator.MigrateDirsReturns(nil)

		mover := &mocks.FileMover{}

		deps = runner.StartupDepsForTest{
			InboxDir:              filepath.Join(promptsBase, "inbox"),
			InProgressDir:         filepath.Join(promptsBase, "in-progress"),
			CompletedDir:          filepath.Join(promptsBase, "completed"),
			LogDir:                filepath.Join(promptsBase, "logs"),
			SpecsInboxDir:         filepath.Join(specsBase, "inbox"),
			SpecsInProgressDir:    filepath.Join(specsBase, "in-progress"),
			SpecsCompletedDir:     filepath.Join(specsBase, "completed"),
			SpecsLogDir:           filepath.Join(specsBase, "logs"),
			PromptManager:         mgr,
			ContainerChecker:      containerChecker,
			SlugMigrator:          slugMigrator,
			Mover:                 mover,
			CurrentDateTimeGetter: libtime.NewCurrentDateTime(),
		}
	})

	AfterEach(func() {
		cancel()
		_ = os.RemoveAll(tempDir)
	})

	It("creates all lifecycle directories", func() {
		Expect(runner.RunStartupSequenceForTest(ctx, deps)).To(Succeed())
		for _, dir := range []string{
			deps.InboxDir, deps.InProgressDir, deps.CompletedDir, deps.LogDir,
			deps.SpecsInboxDir, deps.SpecsInProgressDir, deps.SpecsCompletedDir, deps.SpecsLogDir,
		} {
			_, err := os.Stat(dir)
			Expect(err).NotTo(HaveOccurred(), "directory should exist: %s", dir)
		}
	})

	It("calls NormalizeFilenames on the in-progress dir", func() {
		Expect(runner.RunStartupSequenceForTest(ctx, deps)).To(Succeed())
		Expect(mgr.NormalizeFilenamesCallCount()).To(Equal(1))
		_, dir := mgr.NormalizeFilenamesArgsForCall(0)
		Expect(dir).To(Equal(deps.InProgressDir))
	})

	It("calls SlugMigrator.MigrateDirs with the four prompt dirs", func() {
		Expect(runner.RunStartupSequenceForTest(ctx, deps)).To(Succeed())
		Expect(slugMigrator.MigrateDirsCallCount()).To(Equal(1))
		_, dirs := slugMigrator.MigrateDirsArgsForCall(0)
		Expect(
			dirs,
		).To(ConsistOf(deps.InboxDir, deps.InProgressDir, deps.CompletedDir, deps.LogDir))
	})

	It("returns an error if NormalizeFilenames fails", func() {
		mgr.NormalizeFilenamesReturns(nil, stderrors.New("normalize error"))
		err := runner.RunStartupSequenceForTest(ctx, deps)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("normalize filenames"))
	})

	It("returns an error if MigrateDirs fails", func() {
		slugMigrator.MigrateDirsReturns(stderrors.New("migrate error"))
		err := runner.RunStartupSequenceForTest(ctx, deps)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("migrate spec slugs"))
	})
})
