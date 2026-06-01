// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd_test

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/cmd"
	"github.com/bborbe/dark-factory/pkg/doctor"
)

var _ = Describe("DoctorCommand", func() {
	var (
		tempDir     string
		ctx         context.Context
		fakeChecker *mocks.DoctorChecker
		fakeFixer   *mocks.DoctorFixer
	)

	BeforeEach(func() {
		tempDir = GinkgoT().TempDir()
		ctx = context.Background()
		fakeChecker = &mocks.DoctorChecker{}
		fakeFixer = &mocks.DoctorFixer{}
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
	})

	Describe("Run", func() {
		It("returns error for unknown flag", func() {
			doctorCmd := cmd.NewDoctorCommand(fakeChecker, fakeFixer, 24)
			err := doctorCmd.Run(ctx, []string{"--unknown-flag"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unknown flag"))
		})

		It("shows help for --help", func() {
			doctorCmd := cmd.NewDoctorCommand(fakeChecker, fakeFixer, 24)
			err := doctorCmd.Run(ctx, []string{"--help"})
			Expect(err).NotTo(HaveOccurred())
		})

		It("shows help for -h", func() {
			doctorCmd := cmd.NewDoctorCommand(fakeChecker, fakeFixer, 24)
			err := doctorCmd.Run(ctx, []string{"-h"})
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns no findings when checker returns empty", func() {
			fakeChecker.CheckReturns([]doctor.Finding{}, nil)
			doctorCmd := cmd.NewDoctorCommand(fakeChecker, fakeFixer, 24)
			err := doctorCmd.Run(ctx, []string{})
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns error when checker fails", func() {
			fakeChecker.CheckReturns(nil, context.DeadlineExceeded)
			doctorCmd := cmd.NewDoctorCommand(fakeChecker, fakeFixer, 24)
			err := doctorCmd.Run(ctx, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("doctor check failed"))
		})

		It("returns error without --fix when findings exist", func() {
			fakeChecker.CheckReturns([]doctor.Finding{
				{
					Category:    doctor.CategoryDuplicateSpecNumbers,
					TargetPaths: []string{"/path/to/spec.md"},
					FixCommand:  "dark-factory spec renumber 001 002",
					Detail:      "spec 001 has duplicate number",
				},
			}, nil)
			doctorCmd := cmd.NewDoctorCommand(fakeChecker, fakeFixer, 24)
			err := doctorCmd.Run(ctx, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("doctor found 1 finding"))
			Expect(err.Error()).To(ContainSubstring("re-run with --fix"))
		})

		It("applies fixes when --fix is given", func() {
			fakeChecker.CheckReturns([]doctor.Finding{
				{
					Category:    doctor.CategoryVerifyingStale,
					TargetPaths: []string{"/path/to/spec.md"},
					FixCommand:  "dark-factory spec verify 001",
					Detail:      "spec has been verifying too long",
				},
			}, nil)
			fakeFixer.ApplyReturns(doctor.ApplyResult{
				Applied: []doctor.AppliedFix{
					{
						Category:    doctor.CategoryVerifyingStale,
						TargetPaths: []string{"/path/to/spec.md"},
						FixCommand:  "dark-factory spec verify 001",
					},
				},
				Skipped: []doctor.SkippedFix{},
				Failed:  []doctor.FailedFix{},
			}, nil)

			doctorCmd := cmd.NewDoctorCommand(fakeChecker, fakeFixer, 24)
			err := doctorCmd.Run(ctx, []string{"--fix"})
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeFixer.ApplyCallCount()).To(Equal(1))
		})

		It("skips confirmation when --yes is given", func() {
			fakeChecker.CheckReturns([]doctor.Finding{
				{
					Category:    doctor.CategoryOrphanPromptLink,
					TargetPaths: []string{"/path/to/prompt.md"},
					FixCommand:  "dark-factory prompt unlink 001 spec001",
					Detail:      "spec spec001 not found",
				},
			}, nil)
			fakeFixer.ApplyReturns(doctor.ApplyResult{
				Applied: []doctor.AppliedFix{},
				Skipped: []doctor.SkippedFix{
					{
						Category:    doctor.CategoryOrphanPromptLink,
						TargetPaths: []string{"/path/to/prompt.md"},
						Detail:      "operator declined",
					},
				},
				Failed: []doctor.FailedFix{},
			}, nil)

			doctorCmd := cmd.NewDoctorCommand(fakeChecker, fakeFixer, 24)
			err := doctorCmd.Run(ctx, []string{"--fix", "--yes"})
			Expect(err).NotTo(HaveOccurred())

			// Verify ApplyOptions.Yes was true
			_, _, opts := fakeFixer.ApplyArgsForCall(0)
			Expect(opts.Yes).To(BeTrue())
		})

		It("returns error when fixer fails", func() {
			fakeChecker.CheckReturns([]doctor.Finding{
				{
					Category:    doctor.CategoryDuplicateSpecNumbers,
					TargetPaths: []string{"/path/to/spec.md"},
					FixCommand:  "dark-factory spec renumber 001 002",
					Detail:      "spec 001 has duplicate number",
				},
			}, nil)
			fakeFixer.ApplyReturns(doctor.ApplyResult{}, context.DeadlineExceeded)

			doctorCmd := cmd.NewDoctorCommand(fakeChecker, fakeFixer, 24)
			err := doctorCmd.Run(ctx, []string{"--fix"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("fixer apply failed"))
		})

		It("returns error when some fixes failed", func() {
			fakeChecker.CheckReturns([]doctor.Finding{
				{
					Category:    doctor.CategoryDuplicateSpecNumbers,
					TargetPaths: []string{"/path/to/spec.md"},
					FixCommand:  "dark-factory spec renumber 001 002",
					Detail:      "spec 001 has duplicate number",
				},
			}, nil)
			fakeFixer.ApplyReturns(doctor.ApplyResult{
				Applied: []doctor.AppliedFix{},
				Skipped: []doctor.SkippedFix{},
				Failed: []doctor.FailedFix{
					{
						Category:    doctor.CategoryDuplicateSpecNumbers,
						TargetPaths: []string{"/path/to/spec.md"},
						Detail:      "lock acquire failed",
					},
				},
			}, nil)

			doctorCmd := cmd.NewDoctorCommand(fakeChecker, fakeFixer, 24)
			err := doctorCmd.Run(ctx, []string{"--fix"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("fixer had 1 failure"))
		})

		It("prints applied count when fixes applied", func() {
			fakeChecker.CheckReturns([]doctor.Finding{
				{
					Category:    doctor.CategoryDuplicateSpecNumbers,
					TargetPaths: []string{"/path/to/spec.md"},
					FixCommand:  "dark-factory spec renumber 001 002",
					Detail:      "spec 001 has duplicate number",
				},
			}, nil)
			fakeFixer.ApplyReturns(doctor.ApplyResult{
				Applied: []doctor.AppliedFix{
					{
						Category:    doctor.CategoryDuplicateSpecNumbers,
						TargetPaths: []string{"/path/to/spec.md"},
					},
				},
				Skipped: []doctor.SkippedFix{},
				Failed:  []doctor.FailedFix{},
			}, nil)

			doctorCmd := cmd.NewDoctorCommand(fakeChecker, fakeFixer, 24)
			err := doctorCmd.Run(ctx, []string{"--fix"})
			Expect(err).NotTo(HaveOccurred())
		})

		It("prints skipped count when fixes skipped", func() {
			fakeChecker.CheckReturns([]doctor.Finding{
				{
					Category:    doctor.CategoryVerifyingStale,
					TargetPaths: []string{"/path/to/spec.md"},
					FixCommand:  "dark-factory spec verify 001",
					Detail:      "spec verifying too long",
				},
			}, nil)
			fakeFixer.ApplyReturns(doctor.ApplyResult{
				Applied: []doctor.AppliedFix{},
				Skipped: []doctor.SkippedFix{
					{
						Category:    doctor.CategoryVerifyingStale,
						TargetPaths: []string{"/path/to/spec.md"},
					},
				},
				Failed: []doctor.FailedFix{},
			}, nil)

			doctorCmd := cmd.NewDoctorCommand(fakeChecker, fakeFixer, 24)
			err := doctorCmd.Run(ctx, []string{"--fix"})
			Expect(err).NotTo(HaveOccurred())
		})

		It("groups findings by category for display", func() {
			fakeChecker.CheckReturns([]doctor.Finding{
				{
					Category:    doctor.CategoryDuplicateSpecNumbers,
					TargetPaths: []string{"/path/to/spec1.md"},
					FixCommand:  "dark-factory spec renumber 001 002",
					Detail:      "spec 001 has duplicate number",
				},
				{
					Category:    doctor.CategoryOrphanPromptLink,
					TargetPaths: []string{"/path/to/prompt1.md"},
					FixCommand:  "dark-factory prompt unlink prompt1 spec1",
					Detail:      "spec spec1 not found",
				},
			}, nil)

			doctorCmd := cmd.NewDoctorCommand(fakeChecker, fakeFixer, 24)
			err := doctorCmd.Run(ctx, []string{}) // no --fix
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("doctor found 2 finding"))
		})
	})

	Describe("DoctorHelp", func() {
		It("prints help to stdout without panicking", func() {
			cmd.DoctorHelp()
		})
	})
})
