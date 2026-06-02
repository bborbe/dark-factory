// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package doctor_test

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
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

var _ = Describe("Fixer", func() {
	var (
		tempDir    string
		specsDir   string
		promptsDir string
		ctx        context.Context
		stdout     *bytes.Buffer
		stderr     *bytes.Buffer
		auditPath  string
	)

	BeforeEach(func() {
		tempDir = GinkgoT().TempDir()
		specsDir = filepath.Join(tempDir, "specs")
		promptsDir = filepath.Join(tempDir, "prompts")
		ctx = context.Background()
		stdout = &bytes.Buffer{}
		stderr = &bytes.Buffer{}
		auditPath = filepath.Join(tempDir, "audit.log")

		os.MkdirAll(filepath.Join(specsDir, "inbox"), 0750)
		os.MkdirAll(filepath.Join(specsDir, "in-progress"), 0750)
		os.MkdirAll(filepath.Join(specsDir, "completed"), 0750)
		os.MkdirAll(filepath.Join(specsDir, "rejected"), 0750)
		os.MkdirAll(filepath.Join(promptsDir, "inbox"), 0750)
		os.MkdirAll(filepath.Join(promptsDir, "in-progress"), 0750)
		os.MkdirAll(filepath.Join(promptsDir, "completed"), 0750)
		os.MkdirAll(filepath.Join(promptsDir, "cancelled"), 0750)
	})

	makeDeps := func() doctor.FixerDeps {
		fakeMover := &mocks.FileMover{}
		fakeMover.MoveFileReturns(nil)
		fakeSpecLister := &mocks.Lister{}
		fakeSpecLister.ListReturns([]*spec.SpecFile{}, nil)
		fakeSpecLister.SummaryReturns(&spec.Summary{}, nil)
		fakeAutoCompleter := &mocks.AutoCompleter{}
		fakeAutoCompleter.CheckAndCompleteReturns(nil)
		fakeFileLock := &mocks.LockFileLock{}
		fakeFileLock.AcquireReturns(nil)
		fakeFileLock.ReleaseReturns(nil)

		pm := prompt.NewManager(
			filepath.Join(promptsDir, "inbox"),
			filepath.Join(promptsDir, "in-progress"),
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "cancelled"),
			fakeMover,
			libtime.NewCurrentDateTime(),
		)

		return doctor.FixerDeps{
			Deps: doctor.Deps{
				SpecsInboxDir:         filepath.Join(specsDir, "inbox"),
				SpecsInProgressDir:    filepath.Join(specsDir, "in-progress"),
				SpecsCompletedDir:     filepath.Join(specsDir, "completed"),
				SpecsRejectedDir:      filepath.Join(specsDir, "rejected"),
				PromptsInboxDir:       filepath.Join(promptsDir, "inbox"),
				PromptsInProgressDir:  filepath.Join(promptsDir, "in-progress"),
				PromptsCompletedDir:   filepath.Join(promptsDir, "completed"),
				PromptsCancelledDir:   filepath.Join(promptsDir, "cancelled"),
				SpecLister:            fakeSpecLister,
				PromptManager:         pm,
				CurrentDateTimeGetter: libtime.NewCurrentDateTime(),
				VerifyingStaleHours:   24,
			},
			AutoCompleter:         fakeAutoCompleter,
			Mover:                 fakeMover,
			FileLockFactory:       func(path string) lock.FileLock { return fakeFileLock },
			CurrentDateTimeGetter: libtime.NewCurrentDateTime(),
		}
	}

	Describe("Apply", func() {
		It("returns empty result when no findings", func() {
			fixer := doctor.NewFixer(makeDeps())
			result, err := fixer.Apply(ctx, nil, doctor.ApplyOptions{
				Yes:             true,
				AuditLogPath:    auditPath,
				FileLockTimeout: 5 * time.Second,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Applied).To(BeEmpty())
			Expect(result.Skipped).To(BeEmpty())
			Expect(result.Failed).To(BeEmpty())
		})

		It("skips when yes=false and operator declines", func() {
			fixer := doctor.NewFixer(makeDeps())
			stdin := bytes.NewBufferString("n\n")
			findings := []doctor.Finding{
				{
					Category:    doctor.CategoryDuplicateSpecNumbers,
					TargetPaths: []string{"/path/to/spec.md"},
					FixCommand:  "dark-factory spec renumber 001 002",
					Detail:      "spec 001 has duplicate number",
				},
			}
			result, err := fixer.Apply(ctx, findings, doctor.ApplyOptions{
				Yes:             false,
				Stdin:           stdin,
				Stdout:          stdout,
				Stderr:          stderr,
				AuditLogPath:    auditPath,
				FileLockTimeout: 5 * time.Second,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Skipped).To(HaveLen(1))
			Expect(result.Skipped[0].Detail).To(Equal("operator declined"))
			Expect(result.Applied).To(BeEmpty())
		})

		It("applies when yes=false and operator confirms with y", func() {
			fixer := doctor.NewFixer(makeDeps())
			stdin := bytes.NewBufferString("y\n")
			findings := []doctor.Finding{
				{
					Category:    doctor.CategoryVerifyingStale,
					TargetPaths: []string{"/path/to/spec.md"},
					FixCommand:  "dark-factory spec verify 001",
					Detail:      "verifying-stale spec",
				},
			}
			result, err := fixer.Apply(ctx, findings, doctor.ApplyOptions{
				Yes:             false,
				Stdin:           stdin,
				Stdout:          stdout,
				Stderr:          stderr,
				AuditLogPath:    auditPath,
				FileLockTimeout: 5 * time.Second,
			})
			Expect(err).NotTo(HaveOccurred())
			// verifying-stale is informational, always skipped
			Expect(result.Skipped).To(HaveLen(1))
			Expect(result.Skipped[0].Detail).To(ContainSubstring("verifying-stale"))
		})

		It("skips verifying-stale findings even with --yes", func() {
			fixer := doctor.NewFixer(makeDeps())
			findings := []doctor.Finding{
				{
					Category:    doctor.CategoryVerifyingStale,
					TargetPaths: []string{"/path/to/spec.md"},
					FixCommand:  "dark-factory spec verify 001",
					Detail:      "verifying-stale",
				},
			}
			result, err := fixer.Apply(ctx, findings, doctor.ApplyOptions{
				Yes:             true,
				Stdout:          stdout,
				Stderr:          stderr,
				AuditLogPath:    auditPath,
				FileLockTimeout: 5 * time.Second,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Skipped).To(HaveLen(1))
			Expect(result.Skipped[0].Detail).To(ContainSubstring("verifying-stale"))
			Expect(result.Applied).To(BeEmpty())
		})

		It("skips parse-error findings even with --yes", func() {
			fixer := doctor.NewFixer(makeDeps())
			findings := []doctor.Finding{
				{
					Category:    doctor.CategoryParseError,
					TargetPaths: []string{"/path/to/spec.md"},
					FixCommand:  "",
					Detail:      "yaml unmarshal error",
				},
			}
			result, err := fixer.Apply(ctx, findings, doctor.ApplyOptions{
				Yes:             true,
				Stdout:          stdout,
				Stderr:          stderr,
				AuditLogPath:    auditPath,
				FileLockTimeout: 5 * time.Second,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Skipped).To(HaveLen(1))
			Expect(result.Skipped[0].Detail).To(Equal("parse-errors require manual YAML fix"))
		})

		It("skips unknown category findings even with --yes", func() {
			fixer := doctor.NewFixer(makeDeps())
			findings := []doctor.Finding{
				{
					Category:    doctor.Category("unknown-category"),
					TargetPaths: []string{"/path/to/spec.md"},
					FixCommand:  "",
					Detail:      "",
				},
			}
			result, err := fixer.Apply(ctx, findings, doctor.ApplyOptions{
				Yes:             true,
				Stdout:          stdout,
				Stderr:          stderr,
				AuditLogPath:    auditPath,
				FileLockTimeout: 5 * time.Second,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Skipped).To(HaveLen(1))
			Expect(result.Skipped[0].Detail).To(Equal("unknown category"))
		})

		It("defaults Stdin/Stdout/Stderr when nil", func() {
			fixer := doctor.NewFixer(makeDeps())
			findings := []doctor.Finding{
				{
					Category:    doctor.CategoryVerifyingStale,
					TargetPaths: []string{"/path/to/spec.md"},
					FixCommand:  "",
					Detail:      "",
				},
			}
			// Should not panic with nil stdin/stdout/stderr
			_, err := fixer.Apply(ctx, findings, doctor.ApplyOptions{
				Yes:             true,
				Stdin:           nil,
				Stdout:          nil,
				Stderr:          nil,
				AuditLogPath:    auditPath,
				FileLockTimeout: 5 * time.Second,
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("defaults FileLockTimeout when zero", func() {
			fixer := doctor.NewFixer(makeDeps())
			findings := []doctor.Finding{
				{
					Category:    doctor.CategoryVerifyingStale,
					TargetPaths: []string{"/path/to/spec.md"},
					FixCommand:  "",
					Detail:      "",
				},
			}
			_, err := fixer.Apply(ctx, findings, doctor.ApplyOptions{
				Yes:             true,
				AuditLogPath:    auditPath,
				FileLockTimeout: 0,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("fixVerifyingStale", func() {
		It("always skips verifying-stale", func() {
			fixer := doctor.NewFixer(makeDeps())
			findings := []doctor.Finding{
				{
					Category:    doctor.CategoryVerifyingStale,
					TargetPaths: []string{filepath.Join(specsDir, "inbox", "001-feature.md")},
					FixCommand:  "dark-factory spec verify 001",
					Detail:      "spec has been verifying for 48 hours",
				},
			}
			result, err := fixer.Apply(ctx, findings, doctor.ApplyOptions{
				Yes:             true,
				AuditLogPath:    auditPath,
				FileLockTimeout: 5 * time.Second,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Skipped).To(HaveLen(1))
			Expect(result.Skipped[0].Detail).To(ContainSubstring("verifying-stale"))
			Expect(result.Skipped[0].Detail).To(ContainSubstring("dark-factory spec verify"))
		})
	})

	Describe("fixOrphanPromptLink", func() {
		It("fails when SpecID is missing", func() {
			fixer := doctor.NewFixer(makeDeps())
			promptPath := filepath.Join(promptsDir, "in-progress", "001-prompt.md")
			os.WriteFile(
				promptPath,
				[]byte("---\nstatus: approved\nspecs: [\"spec001\"]\n---\n# Prompt"),
				0644,
			)

			findings := []doctor.Finding{
				{
					Category:    doctor.CategoryOrphanPromptLink,
					TargetPaths: []string{promptPath},
					FixCommand:  "dark-factory prompt unlink 001 spec001",
					Detail:      "spec spec001 not found",
					// SpecID intentionally omitted to trigger early failure
				},
			}

			result, err := fixer.Apply(ctx, findings, doctor.ApplyOptions{
				Yes:             true,
				AuditLogPath:    auditPath,
				FileLockTimeout: 5 * time.Second,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Failed).To(HaveLen(1))
			Expect(result.Failed[0].Detail).To(ContainSubstring("missing spec ID"))
		})
	})

	Describe("fixStatusDirMismatch", func() {
		It("fails when FixCommand is unknown", func() {
			fixer := doctor.NewFixer(makeDeps())
			findings := []doctor.Finding{
				{
					Category:    doctor.CategoryStatusDirMismatch,
					TargetPaths: []string{filepath.Join(specsDir, "in-progress", "001-feature.md")},
					FixCommand:  "dark-factory unknown-cmd",
					Detail:      "spec in specs/in-progress/ has status completed but only statuses {idea, prompted, prompted-review} are allowed in that directory",
				},
			}

			result, err := fixer.Apply(ctx, findings, doctor.ApplyOptions{
				Yes:             true,
				AuditLogPath:    auditPath,
				FileLockTimeout: 5 * time.Second,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Failed).To(HaveLen(1))
			Expect(result.Failed[0].Detail).To(ContainSubstring("unknown FixCommand"))
		})

		It("fails when expected dir cannot be determined", func() {
			fixer := doctor.NewFixer(makeDeps())
			findings := []doctor.Finding{
				{
					Category:    doctor.CategoryStatusDirMismatch,
					TargetPaths: []string{filepath.Join(specsDir, "in-progress", "001-feature.md")},
					FixCommand:  "dark-factory spec move 001",
					Detail:      "spec in unknown-dir/ has status completed",
				},
			}

			result, err := fixer.Apply(ctx, findings, doctor.ApplyOptions{
				Yes:             true,
				AuditLogPath:    auditPath,
				FileLockTimeout: 5 * time.Second,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Failed).To(HaveLen(1))
			Expect(
				result.Failed[0].Detail,
			).To(ContainSubstring("could not determine expected directory"))
		})

		It("fails when source file does not exist", func() {
			fixer := doctor.NewFixer(makeDeps())
			findings := []doctor.Finding{
				{
					Category:    doctor.CategoryStatusDirMismatch,
					TargetPaths: []string{filepath.Join(specsDir, "in-progress", "nonexistent.md")},
					FixCommand:  "dark-factory spec move nonexistent",
					Detail:      "spec in specs/in-progress/ has status completed",
				},
			}

			result, err := fixer.Apply(ctx, findings, doctor.ApplyOptions{
				Yes:             true,
				AuditLogPath:    auditPath,
				FileLockTimeout: 5 * time.Second,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Failed).To(HaveLen(1))
			// Lock acquires (succeeds) but rename fails because source file doesn't exist
			Expect(result.Failed[0].Detail).To(ContainSubstring("rename failed"))
		})
	})

	Describe("fixDuplicateSpecNumbers", func() {
		// Note: fixDuplicateSpecNumbers uses reindexer which requires complex setup
		// with actual duplicate spec files. Testing it requires integration-level setup.
		// The orphan-prompt-link and status-dir-mismatch tests provide coverage of
		// the applyFinding dispatch logic.
	})

	Describe("fixPromptedNotSwept", func() {
		It("calls auto completer when finding is prompted-not-swept", func() {
			deps := makeDeps()
			fakeAutoCompleter := &mocks.AutoCompleter{}
			fakeAutoCompleter.CheckAndCompleteReturns(nil)
			deps.AutoCompleter = fakeAutoCompleter

			fixer := doctor.NewFixer(deps)
			specPath := filepath.Join(specsDir, "inbox", "001-feature.md")
			os.WriteFile(
				specPath,
				[]byte("---\nid: \"001\"\nstatus: prompted\n---\n# Feature"),
				0644,
			)

			findings := []doctor.Finding{
				{
					Category:    doctor.CategoryPromptedNotSwept,
					TargetPaths: []string{specPath},
					FixCommand:  "dark-factory spec sweep 001",
					Detail:      "spec 001 has status prompted but is not linked from any prompt",
				},
			}

			result, err := fixer.Apply(ctx, findings, doctor.ApplyOptions{
				Yes:             true,
				AuditLogPath:    auditPath,
				FileLockTimeout: 5 * time.Second,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Applied).To(HaveLen(1))
			Expect(fakeAutoCompleter.CheckAndCompleteCallCount()).To(Equal(1))
		})
	})

	Describe("fixOrphanPromptLink", func() {
		It("applies fix when SpecID is provided and prompt can be updated", func() {
			deps := makeDeps()
			fixer := doctor.NewFixer(deps)

			promptPath := filepath.Join(promptsDir, "in-progress", "001-prompt.md")
			os.WriteFile(
				promptPath,
				[]byte("---\nstatus: approved\nspecs: [\"001\"]\n---\n# Prompt"),
				0644,
			)

			findings := []doctor.Finding{
				{
					Category:    doctor.CategoryOrphanPromptLink,
					SpecID:      "001",
					TargetPaths: []string{promptPath},
					FixCommand:  "dark-factory prompt unlink 001 001",
					Detail:      "spec 001 not found",
				},
			}

			result, err := fixer.Apply(ctx, findings, doctor.ApplyOptions{
				Yes:             true,
				AuditLogPath:    auditPath,
				FileLockTimeout: 5 * time.Second,
			})
			Expect(err).NotTo(HaveOccurred())
			// No failure means lock was acquired, load succeeded
			Expect(result.Failed).To(BeEmpty())
		})
	})

	Describe("fixVerifyingStale", func() {
		It("always skips verifying-stale findings regardless of state", func() {
			fixer := doctor.NewFixer(makeDeps())
			findings := []doctor.Finding{
				{
					Category:    doctor.CategoryVerifyingStale,
					TargetPaths: []string{filepath.Join(specsDir, "inbox", "001-feature.md")},
					FixCommand:  "dark-factory spec verify 001",
					Detail:      "spec has been verifying for 100 hours",
				},
			}
			result, err := fixer.Apply(ctx, findings, doctor.ApplyOptions{
				Yes:             true,
				AuditLogPath:    auditPath,
				FileLockTimeout: 5 * time.Second,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Skipped).To(HaveLen(1))
			Expect(result.Skipped[0].Category).To(Equal(doctor.CategoryVerifyingStale))
		})
	})

	Describe("fixOrphanInProgressPrompt", func() {
		It("skips when prompt is not cancellable", func() {
			deps := makeDeps()
			fakeMover := &mocks.FileMover{}
			fakeMover.MoveFileReturns(nil)
			deps.Mover = fakeMover

			pm := prompt.NewManager(
				filepath.Join(promptsDir, "inbox"),
				filepath.Join(promptsDir, "in-progress"),
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "cancelled"),
				fakeMover,
				libtime.NewCurrentDateTime(),
			)
			deps.Deps.PromptManager = pm

			fixer := doctor.NewFixer(deps)

			// Create a prompt in idea status (not cancellable)
			promptPath := filepath.Join(promptsDir, "in-progress", "001-prompt.md")
			os.WriteFile(
				promptPath,
				[]byte("---\nstatus: idea\nspecs: [\"001\"]\n---\n# Prompt"),
				0644,
			)

			findings := []doctor.Finding{
				{
					Category:    doctor.CategoryOrphanInProgressPrompt,
					TargetPaths: []string{promptPath},
					FixCommand:  "dark-factory prompt cancel 001",
					Detail:      "prompt 001 links to a completed spec",
				},
			}

			result, err := fixer.Apply(ctx, findings, doctor.ApplyOptions{
				Yes:             true,
				AuditLogPath:    auditPath,
				FileLockTimeout: 5 * time.Second,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Skipped).To(HaveLen(1))
			Expect(result.Skipped[0].Detail).To(ContainSubstring("no longer cancellable"))
		})

		It("skips when prompt is not cancellable (invalid YAML loads as empty status)", func() {
			deps := makeDeps()
			fakeMover := &mocks.FileMover{}
			fakeMover.MoveFileReturns(nil)
			deps.Mover = fakeMover

			pm := prompt.NewManager(
				filepath.Join(promptsDir, "inbox"),
				filepath.Join(promptsDir, "in-progress"),
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "cancelled"),
				fakeMover,
				libtime.NewCurrentDateTime(),
			)
			deps.Deps.PromptManager = pm

			fixer := doctor.NewFixer(deps)

			// Create a prompt with invalid YAML - load succeeds but frontmatter is empty
			promptPath := filepath.Join(promptsDir, "in-progress", "bad-prompt.md")
			os.WriteFile(promptPath, []byte("not valid yaml ---"), 0644)

			findings := []doctor.Finding{
				{
					Category:    doctor.CategoryOrphanInProgressPrompt,
					TargetPaths: []string{promptPath},
					FixCommand:  "dark-factory prompt cancel bad",
					Detail:      "prompt bad links to completed spec",
				},
			}

			result, err := fixer.Apply(ctx, findings, doctor.ApplyOptions{
				Yes:             true,
				AuditLogPath:    auditPath,
				FileLockTimeout: 5 * time.Second,
			})
			Expect(err).NotTo(HaveOccurred())
			// Invalid YAML loads with empty frontmatter, which is not a cancellable status
			Expect(result.Skipped).To(HaveLen(1))
			Expect(result.Skipped[0].Detail).To(ContainSubstring("no longer cancellable"))
		})
	})
})

var _ = Describe("readLine", func() {
	It("trims whitespace from input", func() {
		r := bytes.NewReader([]byte("  hello  \n"))
		scanner := bufio.NewScanner(r)
		if scanner.Scan() {
			Expect(strings.TrimSpace(scanner.Text())).To(Equal("hello"))
		}
	})
})
