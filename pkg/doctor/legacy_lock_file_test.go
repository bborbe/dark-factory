// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package doctor_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/doctor"
	"github.com/bborbe/dark-factory/pkg/lock"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/spec"
)

var _ = Describe("LegacyLockFile", func() {
	var (
		tempDir    string
		specsDir   string
		promptsDir string
		ctx        context.Context
		fakeMover  *mocks.FileMover
		pm         *prompt.Manager
		auditPath  string
	)

	makeDeps := func() doctor.Deps {
		return doctor.Deps{
			SpecsInboxDir:        filepath.Join(specsDir, "inbox"),
			SpecsInProgressDir:   filepath.Join(specsDir, "in-progress"),
			SpecsCompletedDir:    filepath.Join(specsDir, "completed"),
			SpecsRejectedDir:     filepath.Join(specsDir, "rejected"),
			PromptsInboxDir:      filepath.Join(promptsDir, "inbox"),
			PromptsInProgressDir: filepath.Join(promptsDir, "in-progress"),
			PromptsCompletedDir:  filepath.Join(promptsDir, "completed"),
			PromptsCancelledDir:  filepath.Join(promptsDir, "cancelled"),
			SpecLister: spec.NewLister(
				libtime.NewCurrentDateTime(),
				filepath.Join(specsDir, "inbox"),
			),
			PromptManager:         pm,
			CurrentDateTimeGetter: libtime.NewCurrentDateTime(),
			VerifyingStaleHours:   24,
		}
	}

	BeforeEach(func() {
		tempDir = GinkgoT().TempDir()
		specsDir = filepath.Join(tempDir, "specs")
		promptsDir = filepath.Join(tempDir, "prompts")
		ctx = context.Background()
		auditPath = filepath.Join(tempDir, "audit.log")

		fakeMover = &mocks.FileMover{}
		fakeMover.MoveFileReturns(nil)
		pm = prompt.NewManager(
			filepath.Join(promptsDir, "inbox"),
			filepath.Join(promptsDir, "in-progress"),
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "cancelled"),
			fakeMover,
			libtime.NewCurrentDateTime(),
		)

		os.MkdirAll(filepath.Join(specsDir, "inbox"), 0750)
		os.MkdirAll(filepath.Join(specsDir, "in-progress"), 0750)
		os.MkdirAll(filepath.Join(specsDir, "completed"), 0750)
		os.MkdirAll(filepath.Join(specsDir, "rejected"), 0750)
		os.MkdirAll(filepath.Join(promptsDir, "inbox"), 0750)
		os.MkdirAll(filepath.Join(promptsDir, "in-progress"), 0750)
		os.MkdirAll(filepath.Join(promptsDir, "completed"), 0750)
		os.MkdirAll(filepath.Join(promptsDir, "cancelled"), 0750)
	})

	Describe("Detector", func() {
		It("detects a .lock file in a prompt directory", func() {
			lockPath := filepath.Join(promptsDir, "in-progress", "002-x.md.lock")
			Expect(os.WriteFile(lockPath, []byte{}, 0600)).To(Succeed())

			checker := doctor.NewChecker(makeDeps())
			findings, err := checker.Check(ctx)
			Expect(err).NotTo(HaveOccurred())

			var lockFindings []doctor.Finding
			for _, f := range findings {
				if f.Category == doctor.CategoryLegacyLockFile {
					lockFindings = append(lockFindings, f)
				}
			}
			Expect(lockFindings).To(HaveLen(1))
			Expect(lockFindings[0].TargetPaths).To(ConsistOf(lockPath))
			Expect(lockFindings[0].Detail).To(ContainSubstring("legacy lock-file sidecar"))
		})

		It("detects .lock files across multiple status dirs", func() {
			promptLock := filepath.Join(promptsDir, "in-progress", "001-foo.md.lock")
			specLock := filepath.Join(specsDir, "in-progress", "005-bar.md.lock")
			Expect(os.WriteFile(promptLock, []byte{}, 0600)).To(Succeed())
			Expect(os.WriteFile(specLock, []byte{}, 0600)).To(Succeed())

			checker := doctor.NewChecker(makeDeps())
			findings, err := checker.Check(ctx)
			Expect(err).NotTo(HaveOccurred())

			var lockFindings []doctor.Finding
			for _, f := range findings {
				if f.Category == doctor.CategoryLegacyLockFile {
					lockFindings = append(lockFindings, f)
				}
			}
			Expect(lockFindings).To(HaveLen(2))
		})

		It("returns no legacy-lock findings when the tree is clean", func() {
			checker := doctor.NewChecker(makeDeps())
			findings, err := checker.Check(ctx)
			Expect(err).NotTo(HaveOccurred())

			for _, f := range findings {
				Expect(f.Category).NotTo(Equal(doctor.CategoryLegacyLockFile))
			}
		})

		It("ignores .md files and does not flag them", func() {
			mdPath := filepath.Join(promptsDir, "in-progress", "001-normal.md")
			Expect(
				os.WriteFile(mdPath, []byte("---\nstatus: approved\n---\n# Test"), 0600),
			).To(Succeed())

			checker := doctor.NewChecker(makeDeps())
			findings, err := checker.Check(ctx)
			Expect(err).NotTo(HaveOccurred())

			for _, f := range findings {
				Expect(f.Category).NotTo(Equal(doctor.CategoryLegacyLockFile))
			}
		})
	})

	Describe("Fixer", func() {
		makeFixerDeps := func() doctor.FixerDeps {
			fakeDirLock := &mocks.LockDirLock{}
			fakeDirLock.AcquireReturns(nil)
			fakeDirLock.ReleaseReturns(nil)
			fakeAutoCompleter := &mocks.AutoCompleter{}
			fakeAutoCompleter.CheckAndCompleteReturns(nil)

			return doctor.FixerDeps{
				Deps:          makeDeps(),
				AutoCompleter: fakeAutoCompleter,
				Mover:         fakeMover,
				FileLockFactory: func(path string) lock.DirLock {
					return fakeDirLock
				},
			}
		}

		It("removes a legacy .lock file", func() {
			lockPath := filepath.Join(promptsDir, "in-progress", "003-foo.md.lock")
			Expect(os.WriteFile(lockPath, []byte{}, 0600)).To(Succeed())

			checker := doctor.NewChecker(makeFixerDeps().Deps)
			findings, err := checker.Check(ctx)
			Expect(err).NotTo(HaveOccurred())

			var lockFindings []doctor.Finding
			for _, f := range findings {
				if f.Category == doctor.CategoryLegacyLockFile {
					lockFindings = append(lockFindings, f)
				}
			}
			Expect(lockFindings).To(HaveLen(1))

			fixer := doctor.NewFixer(makeFixerDeps())
			result, err := fixer.Apply(ctx, lockFindings, doctor.ApplyOptions{
				Yes:             true,
				AuditLogPath:    auditPath,
				FileLockTimeout: 5 * time.Second,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Applied).To(HaveLen(1))
			Expect(result.Failed).To(BeEmpty())

			_, statErr := os.Stat(lockPath)
			Expect(os.IsNotExist(statErr)).To(BeTrue(), "lock file should have been removed")
		})

		It("is idempotent — re-running on an already-clean tree applies nothing", func() {
			lockPath := filepath.Join(promptsDir, "in-progress", "004-foo.md.lock")
			Expect(os.WriteFile(lockPath, []byte{}, 0600)).To(Succeed())

			deps := makeFixerDeps()

			// First run: detect and fix.
			checker := doctor.NewChecker(deps.Deps)
			findings, err := checker.Check(ctx)
			Expect(err).NotTo(HaveOccurred())

			var lockFindings []doctor.Finding
			for _, f := range findings {
				if f.Category == doctor.CategoryLegacyLockFile {
					lockFindings = append(lockFindings, f)
				}
			}
			Expect(lockFindings).To(HaveLen(1))

			fixer := doctor.NewFixer(deps)
			result, err := fixer.Apply(ctx, lockFindings, doctor.ApplyOptions{
				Yes:             true,
				AuditLogPath:    auditPath,
				FileLockTimeout: 5 * time.Second,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Applied).To(HaveLen(1))

			// Second run: clean tree produces zero findings.
			findings2, err := checker.Check(ctx)
			Expect(err).NotTo(HaveOccurred())
			for _, f := range findings2 {
				Expect(f.Category).NotTo(Equal(doctor.CategoryLegacyLockFile))
			}

			result2, err := fixer.Apply(ctx, nil, doctor.ApplyOptions{
				Yes:             true,
				AuditLogPath:    auditPath,
				FileLockTimeout: 5 * time.Second,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result2.Applied).To(BeEmpty())
			Expect(result2.Failed).To(BeEmpty())
		})

		It("treats already-gone file as success (os.IsNotExist branch)", func() {
			// Build a finding pointing at a non-existent path to test IsNotExist branch.
			nonExistentPath := filepath.Join(promptsDir, "in-progress", "ghost.md.lock")
			finding := doctor.Finding{
				Category:    doctor.CategoryLegacyLockFile,
				TargetPaths: []string{nonExistentPath},
				Detail:      "legacy lock-file sidecar from old per-file locking scheme",
				FixCommand:  "dark-factory doctor --fix",
			}

			fixer := doctor.NewFixer(makeFixerDeps())
			result, err := fixer.Apply(ctx, []doctor.Finding{finding}, doctor.ApplyOptions{
				Yes:             true,
				AuditLogPath:    auditPath,
				FileLockTimeout: 5 * time.Second,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Applied).To(HaveLen(1), "already-gone file should be treated as applied")
			Expect(result.Failed).To(BeEmpty())
		})
	})
})
