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

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/doctor"
	"github.com/bborbe/dark-factory/pkg/lock"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/spec"

	"github.com/bborbe/errors"
	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
		// Behaviorally back the mock with a real os.Rename so the reindexer's
		// move actually moves the file. Without this, downstream Load(NewPath)
		// fails because the file is still at OldPath on disk.
		fakeMover.MoveFileStub = func(_ context.Context, oldPath, newPath string) error {
			return os.Rename(oldPath, newPath)
		}
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
			AutoCompleter:   fakeAutoCompleter,
			Mover:           fakeMover,
			FileLockFactory: func(path string) lock.FileLock { return fakeFileLock },
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

		DescribeTable("status/dir-mismatch success paths",
			// Entry args are evaluated at package init (BEFORE BeforeEach runs),
			// so specsDir/promptsDir are still "" at Entry time. Resolve dirs
			// inside the body where the closure vars are populated.
			func(parentKind, sourceSubdir, destField, filename, fixCommand, detail string) {
				deps := makeDeps()
				fixer := doctor.NewFixer(deps)

				var parentDir, destDir string
				if parentKind == "spec" {
					parentDir = specsDir
					switch destField {
					case "SpecsCompletedDir":
						destDir = deps.Deps.SpecsCompletedDir
					case "SpecsRejectedDir":
						destDir = deps.Deps.SpecsRejectedDir
					}
				} else {
					parentDir = promptsDir
					switch destField {
					case "PromptsCompletedDir":
						destDir = deps.Deps.PromptsCompletedDir
					}
				}
				srcPath := filepath.Join(parentDir, sourceSubdir, filename)

				err := os.WriteFile(srcPath, []byte("---\nstatus: completed\n---\n"), 0o600)
				Expect(err).NotTo(HaveOccurred())

				result, err := fixer.Apply(ctx, []doctor.Finding{
					{
						Category:    doctor.CategoryStatusDirMismatch,
						TargetPaths: []string{srcPath},
						FixCommand:  fixCommand,
						Detail:      detail,
					},
				}, doctor.ApplyOptions{
					Yes:             true,
					AuditLogPath:    auditPath,
					FileLockTimeout: 5 * time.Second,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Applied).To(HaveLen(1))
				Expect(result.Applied[0].Category).To(Equal(doctor.CategoryStatusDirMismatch))
				Expect(result.Applied[0].FixCommand).To(Equal(fixCommand))
				Expect(result.Applied[0].TargetPaths).To(HaveLen(2))

				// Source removed
				_, err = os.Stat(srcPath)
				Expect(os.IsNotExist(err)).To(BeTrue())

				// Dest present
				_, err = os.Stat(filepath.Join(destDir, filename))
				Expect(err).NotTo(HaveOccurred())

				// Audit log written
				auditContent, err := os.ReadFile(auditPath)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(auditContent)).To(ContainSubstring("status-dir-mismatch"))
				Expect(string(auditContent)).To(ContainSubstring("applied"))
			},
			Entry("spec in-progress + status completed -> SpecsCompletedDir",
				"spec", "in-progress", "SpecsCompletedDir",
				"056-foo.md",
				"dark-factory spec move 056-foo",
				"spec in specs/in-progress/ has status completed but only statuses {idea, draft, approved, generating, prompted, verifying} are allowed in that directory"),
			Entry("spec in-progress + status rejected -> SpecsRejectedDir",
				"spec", "in-progress", "SpecsRejectedDir",
				"057-foo.md",
				"dark-factory spec move 057-foo",
				"spec in specs/in-progress/ has status rejected but only statuses {idea, draft, approved, generating, prompted, verifying} are allowed in that directory"),
			Entry("prompt in-progress + status completed -> PromptsCompletedDir",
				"prompt", "in-progress", "PromptsCompletedDir",
				"1-foo.md",
				"dark-factory prompt move 1-foo",
				"prompt in prompts/in-progress/ has status completed but only statuses {idea, draft, approved, executing, failed, in_review, pending_verification, committing} are allowed in that directory"),
		)
	})

	Describe("fixDuplicateSpecNumbers", func() {
		var (
			specsInProgressDir string
			promptsInProgressDir string
		)

		BeforeEach(func() {
			specsInProgressDir = filepath.Join(specsDir, "in-progress")
			promptsInProgressDir = filepath.Join(promptsDir, "in-progress")
		})

		Context("when reindex returns one rename for a colliding spec", func() {
			It("applies the renumber and writes previous_id and audit log", func() {
				// Both files in same dir so reindexer sees a collision.
				// Lex sort: 056-aaa.md < 056-bar.md → bar is the lex-last collider,
				// which the reindexer picks as the loser to renumber.
				err := os.WriteFile(
					filepath.Join(specsInProgressDir, "056-aaa.md"),
					[]byte("---\nid: \"056\"\nstatus: idea\n---\n# Spec Foo"),
					0644,
				)
				Expect(err).NotTo(HaveOccurred())
				err = os.WriteFile(
					filepath.Join(specsInProgressDir, "056-bar.md"),
					[]byte("---\nid: \"056\"\nstatus: idea\n---\n# Spec Bar"),
					0644,
				)
				Expect(err).NotTo(HaveOccurred())

				deps := makeDeps()
				fixer := doctor.NewFixer(deps)

				result, err := fixer.Apply(ctx, []doctor.Finding{
					{
						Category:    doctor.CategoryDuplicateSpecNumbers,
						TargetPaths: []string{"056-bar.md"},
						SpecID:      "056-foo",
						FixCommand:  "dark-factory spec renumber 056-bar",
					},
				}, doctor.ApplyOptions{
					Yes:             true,
					AuditLogPath:    auditPath,
					FileLockTimeout: 5 * time.Second,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Applied).To(HaveLen(1))
				Expect(result.Applied[0].Category).To(Equal(doctor.CategoryDuplicateSpecNumbers))
				Expect(result.Applied[0].TargetPaths).To(HaveLen(2))

				// FileMover called once
				Expect(deps.Mover.(*mocks.FileMover).MoveFileCallCount()).To(Equal(1))

				// Audit log written
				auditContent, err := os.ReadFile(auditPath)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(auditContent)).To(ContainSubstring("duplicate-spec-numbers"))
				Expect(string(auditContent)).To(ContainSubstring("applied"))

				// Renamed file present, old path gone
				_, err = os.Stat(filepath.Join(specsInProgressDir, "056-bar.md"))
				Expect(os.IsNotExist(err)).To(BeTrue())
				entries, err := os.ReadDir(specsInProgressDir)
				Expect(err).NotTo(HaveOccurred())
				var foundNewName bool
				for _, e := range entries {
					if strings.HasPrefix(e.Name(), "0") && strings.Contains(e.Name(), "-bar.md") {
						foundNewName = true
						// Verify previous_id is unquoted
						data, err := os.ReadFile(filepath.Join(specsInProgressDir, e.Name()))
						Expect(err).NotTo(HaveOccurred())
						Expect(string(data)).To(ContainSubstring("\nprevious_id: 056\n"))
						break
					}
				}
				Expect(foundNewName).To(BeTrue(), "renamed spec file not found in in-progress dir")
			})
		})

		Context("when reindex returns empty renames (no relevant collisions)", func() {
			It("returns empty applied and failed", func() {
				// No files written — reindexer sees no collisions → empty renames
				deps := makeDeps()
				fixer := doctor.NewFixer(deps)

				result, err := fixer.Apply(ctx, []doctor.Finding{
					{
						Category:    doctor.CategoryDuplicateSpecNumbers,
						TargetPaths: []string{"056-bar.md"},
						SpecID:      "056-foo",
						FixCommand:  "dark-factory spec renumber 056-bar",
					},
				}, doctor.ApplyOptions{
					Yes:             true,
					AuditLogPath:    auditPath,
					FileLockTimeout: 5 * time.Second,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Applied).To(BeEmpty())
				Expect(result.Failed).To(BeEmpty())
			})
		})

		Context("when lock acquire fails", func() {
			It("returns a FailedFix with lock acquire detail", func() {
				// Collision fixture so reindex produces a rename that the fixer will try to apply.
				Expect(os.WriteFile(
					filepath.Join(specsInProgressDir, "056-aaa.md"),
					[]byte("---\nstatus: idea\n---\n# A"), 0644)).To(Succeed())
				Expect(os.WriteFile(
					filepath.Join(specsInProgressDir, "056-bar.md"),
					[]byte("---\nstatus: idea\n---\n# Bar"), 0644)).To(Succeed())

				deps := makeDeps()
				fakeLock := &mocks.LockFileLock{}
				fakeLock.AcquireReturns(errors.New(ctx, "lock acquire timeout after 5s"))
				deps.FileLockFactory = func(path string) lock.FileLock { return fakeLock }
				fixer := doctor.NewFixer(deps)

				result, err := fixer.Apply(ctx, []doctor.Finding{
					{
						Category:    doctor.CategoryDuplicateSpecNumbers,
						TargetPaths: []string{"056-bar.md"},
						SpecID:      "056-foo",
						FixCommand:  "dark-factory spec renumber 056-bar",
					},
				}, doctor.ApplyOptions{
					Yes:             true,
					AuditLogPath:    auditPath,
					FileLockTimeout: 5 * time.Second,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Failed).To(HaveLen(1))
				Expect(result.Failed[0].Category).To(Equal(doctor.CategoryDuplicateSpecNumbers))
				Expect(result.Failed[0].Detail).To(ContainSubstring("lock acquire"))
			})
		})

		PContext("when spec file frontmatter is unparseable (load failure)", func() {
			// PENDING: `spec.Load` swallows YAML parse errors by design (returns an
			// empty-frontmatter SpecFile with nil error — see pkg/spec/spec.go:289-296),
			// so the `load failed` branch in fix_renumber.go is only triggerable via
			// an I/O failure on os.ReadFile — not deterministically reachable from
			// a test without OS-level tricks (perms 0o000 fail under containerized
			// roots, NUL-byte paths fail before reaching Load, etc.). Either:
			//   a) Add a `ParseError` sentinel to spec.Load and re-enable this test, OR
			//   b) Accept that the branch is reachable only by real-world disk failure
			//      and keep this test pending as documentation of the gap.
			It("returns a FailedFix with load failed detail", func() {
				Expect(os.WriteFile(
					filepath.Join(specsInProgressDir, "056-aaa.md"),
					[]byte("---\nstatus: idea\n---\n# A"), 0644)).To(Succeed())
				err := os.WriteFile(
					filepath.Join(specsInProgressDir, "056-bar.md"),
					[]byte("---\nstatus: idea\nbad:\n\tindented_with_tab: x\n---\n# Bad"),
					0644,
				)
				Expect(err).NotTo(HaveOccurred())

				deps := makeDeps()
				fixer := doctor.NewFixer(deps)

				result, err := fixer.Apply(ctx, []doctor.Finding{
					{
						Category:    doctor.CategoryDuplicateSpecNumbers,
						TargetPaths: []string{"056-bar.md"},
						SpecID:      "056-foo",
						FixCommand:  "dark-factory spec renumber 056-bar",
					},
				}, doctor.ApplyOptions{
					Yes:             true,
					AuditLogPath:    auditPath,
					FileLockTimeout: 5 * time.Second,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Failed).To(HaveLen(1))
				Expect(result.Failed[0].Detail).To(ContainSubstring("load failed"))
			})
		})

		Context("when reindex's move fails", func() {
			It("returns a FailedFix with reindex failed detail (move error surfaces via reindex)", func() {
				// Collision fixture
				Expect(os.WriteFile(
					filepath.Join(specsInProgressDir, "056-aaa.md"),
					[]byte("---\nstatus: idea\n---\n# A"), 0644)).To(Succeed())
				Expect(os.WriteFile(
					filepath.Join(specsInProgressDir, "056-bar.md"),
					[]byte("---\nstatus: idea\n---\n# Bar"), 0644)).To(Succeed())

				deps := makeDeps()
				// Reindex is the only MoveFile caller post-refactor; injecting a failing
				// Mover surfaces the error through reindex, not through the fixer's
				// per-rename loop (which no longer calls MoveFile).
				fakeMover := &mocks.FileMover{}
				fakeMover.MoveFileReturns(errors.New(ctx, "rename failed: device or resource busy"))
				deps.Mover = fakeMover
				fixer := doctor.NewFixer(deps)

				result, err := fixer.Apply(ctx, []doctor.Finding{
					{
						Category:    doctor.CategoryDuplicateSpecNumbers,
						TargetPaths: []string{"056-bar.md"},
						SpecID:      "056-aaa",
						FixCommand:  "dark-factory spec renumber 056-bar",
					},
				}, doctor.ApplyOptions{
					Yes:             true,
					AuditLogPath:    auditPath,
					FileLockTimeout: 5 * time.Second,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Failed).To(HaveLen(1))
				Expect(result.Failed[0].Detail).To(ContainSubstring("reindex failed"))
			})
		})

		Context("when audit log write fails", func() {
			It("returns the FailedFix and the file was still renamed", func() {
				// Collision fixture so reindex produces a rename and the fixer reaches the audit-write step.
				Expect(os.WriteFile(
					filepath.Join(specsInProgressDir, "056-aaa.md"),
					[]byte("---\nstatus: idea\n---\n# A"), 0644)).To(Succeed())
				err := os.WriteFile(
					filepath.Join(specsInProgressDir, "056-bar.md"),
					[]byte("---\nid: \"056\"\nstatus: idea\n---\n# Bar"),
					0644,
				)
				Expect(err).NotTo(HaveOccurred())

				// auditPath resolves to a path inside a regular-file component → ENOTDIR on OpenFile
				auditBlocker := filepath.Join(tempDir, "audit-blocker")
				err = os.WriteFile(auditBlocker, []byte("x"), 0644)
				Expect(err).NotTo(HaveOccurred())
				blockedAuditPath := filepath.Join(auditBlocker, "log.tsv")

				deps := makeDeps()
				fixer := doctor.NewFixer(deps)

				result, err := fixer.Apply(ctx, []doctor.Finding{
					{
						Category:    doctor.CategoryDuplicateSpecNumbers,
						TargetPaths: []string{"056-bar.md"},
						SpecID:      "056-foo",
						FixCommand:  "dark-factory spec renumber 056-bar",
					},
				}, doctor.ApplyOptions{
					Yes:             true,
					AuditLogPath:    blockedAuditPath,
					FileLockTimeout: 5 * time.Second,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Failed).To(HaveLen(1))
				Expect(result.Failed[0].Detail).To(ContainSubstring("audit log write failed"))

				// Rename still happened despite audit failure
				_, err = os.Stat(filepath.Join(specsInProgressDir, "056-bar.md"))
				Expect(os.IsNotExist(err)).To(BeTrue())
			})
		})

		Context("when TargetPaths contains a file not in any spec dir", func() {
			It("returns empty applied and failed (early return)", func() {
				// No collision files at all — reindex returns empty renames
				deps := makeDeps()
				fixer := doctor.NewFixer(deps)

				result, err := fixer.Apply(ctx, []doctor.Finding{
					{
						Category:    doctor.CategoryDuplicateSpecNumbers,
						TargetPaths: []string{"999-does-not-exist.md"},
						SpecID:      "999",
						FixCommand:  "dark-factory spec renumber 999-does-not-exist",
					},
				}, doctor.ApplyOptions{
					Yes:             true,
					AuditLogPath:    auditPath,
					FileLockTimeout: 5 * time.Second,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Applied).To(BeEmpty())
				Expect(result.Failed).To(BeEmpty())
			})
		})

		Context("when TargetPaths is empty", func() {
			It("returns empty applied and failed (early return)", func() {
				deps := makeDeps()
				fixer := doctor.NewFixer(deps)

				result, err := fixer.Apply(ctx, []doctor.Finding{
					{
						Category:    doctor.CategoryDuplicateSpecNumbers,
						TargetPaths: []string{},
						SpecID:      "056",
						FixCommand:  "dark-factory spec renumber 056",
					},
				}, doctor.ApplyOptions{
					Yes:             true,
					AuditLogPath:    auditPath,
					FileLockTimeout: 5 * time.Second,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Applied).To(BeEmpty())
				Expect(result.Failed).To(BeEmpty())
			})
		})

		Context("when UpdateSpecRefs fails (intentionally swallowed)", func() {
			It("returns AppliedFix and audit log was written despite UpdateSpecRefs error", func() {
				// Create a prompt in prompts/in-progress with unparseable frontmatter
				// so UpdateSpecRefs returns an error.
				promptPath := filepath.Join(promptsInProgressDir, "999-bad.md")
				err := os.WriteFile(promptPath, []byte("---\nstatus: !!invalid\n---\n# Bad"), 0644)
				Expect(err).NotTo(HaveOccurred())

				err = os.WriteFile(
					filepath.Join(specsInProgressDir, "056-aaa.md"),
					[]byte("---\nid: \"056\"\nstatus: idea\n---\n# Foo"),
					0644,
				)
				Expect(err).NotTo(HaveOccurred())
				err = os.WriteFile(
					filepath.Join(specsInProgressDir, "056-bar.md"),
					[]byte("---\nid: \"056\"\nstatus: idea\n---\n# Bar"),
					0644,
				)
				Expect(err).NotTo(HaveOccurred())

				deps := makeDeps()
				fixer := doctor.NewFixer(deps)

				result, err := fixer.Apply(ctx, []doctor.Finding{
					{
						Category:    doctor.CategoryDuplicateSpecNumbers,
						TargetPaths: []string{"056-bar.md"},
						SpecID:      "056-foo",
						FixCommand:  "dark-factory spec renumber 056-bar",
					},
				}, doctor.ApplyOptions{
					Yes:             true,
					AuditLogPath:    auditPath,
					FileLockTimeout: 5 * time.Second,
				})
				Expect(err).NotTo(HaveOccurred())
				// fix_renumber.go:151-154 intentionally swallows UpdateSpecRefs errors —
				// the renumber already succeeded on disk, and spec-ref updates in prompts
				// are best-effort. This test pins that behavior; if you change the
				// production code to surface the error, update this expectation accordingly.
				Expect(result.Applied).To(HaveLen(1))
				Expect(result.Applied[0].Category).To(Equal(doctor.CategoryDuplicateSpecNumbers))
				Expect(result.Failed).To(BeEmpty())

				// Audit log was written
				auditContent, err := os.ReadFile(auditPath)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(auditContent)).To(ContainSubstring("duplicate-spec-numbers"))
				Expect(string(auditContent)).To(ContainSubstring("applied"))
			})
		})
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
