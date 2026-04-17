// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/bborbe/errors"
	"github.com/bborbe/run"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/git"
)

var _ = Describe("Git", func() {
	Describe("Releaser interface", func() {
		var (
			ctx         context.Context
			tempDir     string
			originalDir string
			r           git.Releaser
		)

		BeforeEach(func() {
			ctx = context.Background()

			var err error
			originalDir, err = os.Getwd()
			Expect(err).NotTo(HaveOccurred())

			tempDir, err = os.MkdirTemp("", "git-interface-test-*")
			Expect(err).NotTo(HaveOccurred())

			// Initialize git repo
			cmd := exec.Command("git", "init")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Configure git
			cmd = exec.Command("git", "config", "user.email", "test@example.com")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("git", "config", "user.name", "Test User")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Change to temp directory
			err = os.Chdir(tempDir)
			Expect(err).NotTo(HaveOccurred())

			r = git.NewReleaser()
		})

		AfterEach(func() {
			if originalDir != "" {
				err := os.Chdir(originalDir)
				Expect(err).NotTo(HaveOccurred())
			}

			if tempDir != "" {
				_ = os.RemoveAll(tempDir)
			}
		})

		It("GetNextVersion returns v0.1.0 with no tags", func() {
			// Create initial commit
			err := os.WriteFile(filepath.Join(tempDir, "test.txt"), []byte("test"), 0600)
			Expect(err).NotTo(HaveOccurred())

			cmd := exec.Command("git", "add", ".")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("git", "commit", "-m", "initial")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			version, err := r.GetNextVersion(ctx, git.PatchBump)
			Expect(err).To(BeNil())
			Expect(version).To(Equal("v0.1.0"))
		})

		It("GetNextVersion falls back to CHANGELOG.md when no tags exist", func() {
			// Create CHANGELOG.md with version entries
			err := os.WriteFile(
				filepath.Join(tempDir, "CHANGELOG.md"),
				[]byte(
					"# Changelog\n\n## v1.3.4\n\n- Some feature\n\n## v1.2.0\n\n- Earlier feature\n",
				),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			cmd := exec.Command("git", "add", ".")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("git", "commit", "-m", "initial")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			version, err := r.GetNextVersion(ctx, git.PatchBump)
			Expect(err).To(BeNil())
			Expect(version).To(Equal("v1.3.5"))
		})

		It("GetNextVersion falls back to CHANGELOG.md minor bump when no tags exist", func() {
			err := os.WriteFile(
				filepath.Join(tempDir, "CHANGELOG.md"),
				[]byte("# Changelog\n\n## v2.5.1\n\n- Some feature\n"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			cmd := exec.Command("git", "add", ".")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("git", "commit", "-m", "initial")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			version, err := r.GetNextVersion(ctx, git.MinorBump)
			Expect(err).To(BeNil())
			Expect(version).To(Equal("v2.6.0"))
		})

		It(
			"GetNextVersion returns v0.1.0 when no tags and CHANGELOG has no version entries",
			func() {
				err := os.WriteFile(
					filepath.Join(tempDir, "CHANGELOG.md"),
					[]byte("# Changelog\n\nNo version entries here.\n"),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())

				cmd := exec.Command("git", "add", ".")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "commit", "-m", "initial")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				version, err := r.GetNextVersion(ctx, git.PatchBump)
				Expect(err).To(BeNil())
				Expect(version).To(Equal("v0.1.0"))
			},
		)

		It("GetNextVersion uses git tags when tags exist, ignoring CHANGELOG.md", func() {
			// Create CHANGELOG.md with higher version than tag
			err := os.WriteFile(
				filepath.Join(tempDir, "CHANGELOG.md"),
				[]byte("# Changelog\n\n## v9.9.9\n\n- Some feature\n"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			cmd := exec.Command("git", "add", ".")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("git", "commit", "-m", "initial")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Tag with lower version than CHANGELOG
			cmd = exec.Command("git", "tag", "v1.2.3")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			version, err := r.GetNextVersion(ctx, git.PatchBump)
			Expect(err).To(BeNil())
			Expect(version).To(Equal("v1.2.4"))
		})

		It("CommitCompletedFile commits a new file", func() {
			// Create initial commit
			err := os.WriteFile(filepath.Join(tempDir, "README.md"), []byte("# Test"), 0600)
			Expect(err).NotTo(HaveOccurred())

			cmd := exec.Command("git", "add", ".")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("git", "commit", "-m", "initial")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Create completed file
			completedPath := filepath.Join(tempDir, "completed.md")
			err = os.WriteFile(completedPath, []byte("completed"), 0600)
			Expect(err).NotTo(HaveOccurred())

			err = r.CommitCompletedFile(ctx, completedPath)
			Expect(err).To(BeNil())

			// Verify commit was created
			cmd = exec.Command("git", "log", "--oneline", "-n", "1")
			cmd.Dir = tempDir
			output, err := cmd.Output()
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("move prompt to completed"))
		})

		It("CommitAndRelease performs full workflow", func() {
			// Create CHANGELOG.md with entry in Unreleased
			err := os.WriteFile(
				filepath.Join(tempDir, "CHANGELOG.md"),
				[]byte("# Changelog\n\n## Unreleased\n\n- Add test feature\n"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			// Create initial commit
			cmd := exec.Command("git", "add", ".")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("git", "commit", "-m", "initial")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Create bare repo as remote
			bareDir := filepath.Join(filepath.Dir(tempDir), "bare-interface-test")
			cmd = exec.Command("git", "init", "--bare", bareDir)
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())
			defer func() {
				_ = os.RemoveAll(bareDir)
			}()

			cmd = exec.Command("git", "remote", "add", "origin", bareDir)
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("git", "push", "-u", "origin", "master")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Create a file to commit
			err = os.WriteFile(filepath.Join(tempDir, "test.txt"), []byte("test"), 0600)
			Expect(err).NotTo(HaveOccurred())

			err = r.CommitAndRelease(ctx, git.PatchBump)
			Expect(err).To(BeNil())

			// Verify tag was created
			cmd = exec.Command("git", "tag", "-l")
			cmd.Dir = tempDir
			output, err := cmd.Output()
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("v0.1.0"))
		})
	})

	Describe("GetNextVersion", func() {
		var (
			ctx         context.Context
			tempDir     string
			originalDir string
		)

		BeforeEach(func() {
			ctx = context.Background()

			var err error
			originalDir, err = os.Getwd()
			Expect(err).NotTo(HaveOccurred())

			tempDir, err = os.MkdirTemp("", "git-test-*")
			Expect(err).NotTo(HaveOccurred())

			// Initialize git repo
			cmd := exec.Command("git", "init")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Configure git
			cmd = exec.Command("git", "config", "user.email", "test@example.com")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("git", "config", "user.name", "Test User")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Change to temp directory
			err = os.Chdir(tempDir)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if originalDir != "" {
				err := os.Chdir(originalDir)
				Expect(err).NotTo(HaveOccurred())
			}

			if tempDir != "" {
				_ = os.RemoveAll(tempDir)
			}
		})

		Context("with no tags", func() {
			It("returns v0.1.0", func() {
				// Create initial commit
				err := os.WriteFile(filepath.Join(tempDir, "test.txt"), []byte("test"), 0600)
				Expect(err).NotTo(HaveOccurred())

				cmd := exec.Command("git", "add", ".")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "commit", "-m", "initial")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				version, err := git.GetNextVersion(ctx, git.PatchBump)
				Expect(err).To(BeNil())
				Expect(version).To(Equal("v0.1.0"))
			})
		})

		Context("with existing tag v0.1.0", func() {
			BeforeEach(func() {
				// Create initial commit and tag
				err := os.WriteFile(filepath.Join(tempDir, "test.txt"), []byte("test"), 0600)
				Expect(err).NotTo(HaveOccurred())

				cmd := exec.Command("git", "add", ".")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "commit", "-m", "initial")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "tag", "v0.1.0")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns v0.1.1 with PatchBump", func() {
				version, err := git.GetNextVersion(ctx, git.PatchBump)
				Expect(err).To(BeNil())
				Expect(version).To(Equal("v0.1.1"))
			})

			It("returns v0.2.0 with MinorBump", func() {
				version, err := git.GetNextVersion(ctx, git.MinorBump)
				Expect(err).To(BeNil())
				Expect(version).To(Equal("v0.2.0"))
			})
		})

		Context("with existing tag v1.2.3", func() {
			BeforeEach(func() {
				// Create initial commit and tag
				err := os.WriteFile(filepath.Join(tempDir, "test.txt"), []byte("test"), 0600)
				Expect(err).NotTo(HaveOccurred())

				cmd := exec.Command("git", "add", ".")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "commit", "-m", "initial")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "tag", "v1.2.3")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns v1.2.4 with PatchBump", func() {
				version, err := git.GetNextVersion(ctx, git.PatchBump)
				Expect(err).To(BeNil())
				Expect(version).To(Equal("v1.2.4"))
			})

			It("returns v1.3.0 with MinorBump", func() {
				version, err := git.GetNextVersion(ctx, git.MinorBump)
				Expect(err).To(BeNil())
				Expect(version).To(Equal("v1.3.0"))
			})
		})

		Context("with multiple tags", func() {
			BeforeEach(func() {
				// Create initial commit
				err := os.WriteFile(filepath.Join(tempDir, "test.txt"), []byte("test"), 0600)
				Expect(err).NotTo(HaveOccurred())

				cmd := exec.Command("git", "add", ".")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "commit", "-m", "initial")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				// Create multiple tags
				cmd = exec.Command("git", "tag", "v0.1.0")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				// Add another commit
				err = os.WriteFile(filepath.Join(tempDir, "test2.txt"), []byte("test2"), 0600)
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "add", ".")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "commit", "-m", "second")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "tag", "v0.2.0")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns bumped version of latest tag with PatchBump", func() {
				version, err := git.GetNextVersion(ctx, git.PatchBump)
				Expect(err).To(BeNil())
				Expect(version).To(Equal("v0.2.1"))
			})

			It("returns bumped version of latest tag with MinorBump", func() {
				version, err := git.GetNextVersion(ctx, git.MinorBump)
				Expect(err).To(BeNil())
				Expect(version).To(Equal("v0.3.0"))
			})
		})

		Context("regression: semver vs lexicographic sorting", func() {
			BeforeEach(func() {
				// Create initial commit
				err := os.WriteFile(filepath.Join(tempDir, "test.txt"), []byte("test"), 0600)
				Expect(err).NotTo(HaveOccurred())

				cmd := exec.Command("git", "add", ".")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "commit", "-m", "initial")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				// Create tags in an order that would be wrong with lexicographic sort
				// Lexicographically: "v0.10.0" < "v0.9.0" (wrong!)
				// Semver: v0.9.0 < v0.10.0 (correct!)
				tags := []string{"v0.1.9", "v0.2.25", "v0.10.0", "v0.9.0"}
				for _, tag := range tags {
					cmd = exec.Command("git", "tag", tag)
					cmd.Dir = tempDir
					err = cmd.Run()
					Expect(err).NotTo(HaveOccurred())
				}
			})

			It("correctly identifies v0.10.0 as the latest version", func() {
				version, err := git.GetNextVersion(ctx, git.PatchBump)
				Expect(err).To(BeNil())
				Expect(version).To(Equal("v0.10.1")) // Should bump from v0.10.0, not v0.9.0
			})

			It("v0.2.25 is greater than v0.1.9 (regression test)", func() {
				// This was the original bug: lexicographic sort would have picked v0.1.9 over v0.2.25
				// Create repo with only these two tags
				tempDir2, err := os.MkdirTemp("", "git-regression-test-*")
				Expect(err).NotTo(HaveOccurred())
				defer func() {
					_ = os.RemoveAll(tempDir2)
				}()

				cmd := exec.Command("git", "init")
				cmd.Dir = tempDir2
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "config", "user.email", "test@example.com")
				cmd.Dir = tempDir2
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "config", "user.name", "Test User")
				cmd.Dir = tempDir2
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				err = os.WriteFile(filepath.Join(tempDir2, "test.txt"), []byte("test"), 0600)
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "add", ".")
				cmd.Dir = tempDir2
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "commit", "-m", "initial")
				cmd.Dir = tempDir2
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				// Create tags
				for _, tag := range []string{"v0.1.9", "v0.2.25"} {
					cmd = exec.Command("git", "tag", tag)
					cmd.Dir = tempDir2
					err = cmd.Run()
					Expect(err).NotTo(HaveOccurred())
				}

				// Change to temp dir to test
				originalDir2, err := os.Getwd()
				Expect(err).NotTo(HaveOccurred())

				err = os.Chdir(tempDir2)
				Expect(err).NotTo(HaveOccurred())
				defer func() {
					_ = os.Chdir(originalDir2)
				}()

				version, err := git.GetNextVersion(ctx, git.PatchBump)
				Expect(err).To(BeNil())
				Expect(version).To(Equal("v0.2.26")) // Should bump from v0.2.25, not v0.1.9
			})
		})

		Context("with non-semver tags mixed in", func() {
			BeforeEach(func() {
				// Create initial commit
				err := os.WriteFile(filepath.Join(tempDir, "test.txt"), []byte("test"), 0600)
				Expect(err).NotTo(HaveOccurred())

				cmd := exec.Command("git", "add", ".")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "commit", "-m", "initial")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				// Create mix of valid and invalid tags
				tags := []string{"v0.1.0", "invalid-tag", "v0.2.0", "another-bad-tag", "v1"}
				for _, tag := range tags {
					cmd = exec.Command("git", "tag", tag)
					cmd.Dir = tempDir
					err = cmd.Run()
					Expect(err).NotTo(HaveOccurred())
				}
			})

			It("filters out non-semver tags and finds correct max", func() {
				version, err := git.GetNextVersion(ctx, git.PatchBump)
				Expect(err).To(BeNil())
				Expect(version).To(Equal("v0.2.1")) // Should use v0.2.0, ignoring invalid tags
			})
		})
	})

	Describe("CommitAndRelease", func() {
		var (
			ctx         context.Context
			tempDir     string
			originalDir string
		)

		BeforeEach(func() {
			ctx = context.Background()

			var err error
			originalDir, err = os.Getwd()
			Expect(err).NotTo(HaveOccurred())

			tempDir, err = os.MkdirTemp("", "git-commit-test-*")
			Expect(err).NotTo(HaveOccurred())

			// Initialize git repo
			cmd := exec.Command("git", "init")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Configure git
			cmd = exec.Command("git", "config", "user.email", "test@example.com")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("git", "config", "user.name", "Test User")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Create CHANGELOG.md
			changelogPath := filepath.Join(tempDir, "CHANGELOG.md")
			err = os.WriteFile(
				changelogPath,
				[]byte("# Changelog\n\n## Unreleased\n\n"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			// Initial commit
			cmd = exec.Command("git", "add", ".")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("git", "commit", "-m", "initial commit")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Create a bare repo as fake remote
			bareDir := filepath.Join(tempDir, "..", "bare-"+filepath.Base(tempDir))
			cmd = exec.Command("git", "init", "--bare", bareDir)
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Add remote
			cmd = exec.Command("git", "remote", "add", "origin", bareDir)
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Push initial commit
			cmd = exec.Command("git", "push", "-u", "origin", "master")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Change to temp directory
			err = os.Chdir(tempDir)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if originalDir != "" {
				err := os.Chdir(originalDir)
				Expect(err).NotTo(HaveOccurred())
			}

			if tempDir != "" {
				_ = os.RemoveAll(tempDir)
				// Also clean up bare repo
				bareDir := filepath.Join(filepath.Dir(tempDir), "bare-"+filepath.Base(tempDir))
				_ = os.RemoveAll(bareDir)
			}
		})

		Context("with changes to commit and ## Unreleased with entries", func() {
			BeforeEach(func() {
				// Update CHANGELOG to have entries in Unreleased
				changelogPath := filepath.Join(tempDir, "CHANGELOG.md")
				err := os.WriteFile(
					changelogPath,
					[]byte("# Changelog\n\n## Unreleased\n\n- Add test feature\n- Fix bug\n"),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())

				cmd := exec.Command("git", "add", ".")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "commit", "-m", "update changelog")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				// Create a new file to commit
				err = os.WriteFile(
					filepath.Join(tempDir, "test.txt"),
					[]byte("test content"),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())
			})

			It("renames ## Unreleased to version and preserves entries", func() {
				err := git.CommitAndRelease(ctx, git.PatchBump)
				Expect(err).To(BeNil())

				// Verify commit was created
				cmd := exec.Command("git", "log", "--oneline", "-n", "1")
				cmd.Dir = tempDir
				output, err := cmd.Output()
				Expect(err).NotTo(HaveOccurred())
				Expect(string(output)).To(ContainSubstring("release v0.1.0"))

				// Verify tag was created
				cmd = exec.Command("git", "tag", "-l")
				cmd.Dir = tempDir
				output, err = cmd.Output()
				Expect(err).NotTo(HaveOccurred())
				Expect(string(output)).To(ContainSubstring("v0.1.0"))

				// Verify CHANGELOG was updated - ## Unreleased renamed to ## v0.1.0
				changelogContent, err := os.ReadFile(filepath.Join(tempDir, "CHANGELOG.md"))
				Expect(err).NotTo(HaveOccurred())
				Expect(string(changelogContent)).To(ContainSubstring("## v0.1.0"))
				// Original entries should be preserved
				Expect(string(changelogContent)).To(ContainSubstring("- Add test feature"))
				Expect(string(changelogContent)).To(ContainSubstring("- Fix bug"))
				// Should NOT contain ## Unreleased anymore
				Expect(string(changelogContent)).NotTo(ContainSubstring("## Unreleased"))
			})
		})

		Context("with existing version tag", func() {
			BeforeEach(func() {
				// Update CHANGELOG to have v0.1.0 section
				changelogPath := filepath.Join(tempDir, "CHANGELOG.md")
				err := os.WriteFile(
					changelogPath,
					[]byte("# Changelog\n\n## v0.1.0\n\n- First feature\n"),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())

				// Create a file and commit it with v0.1.0 tag
				err = os.WriteFile(filepath.Join(tempDir, "first.txt"), []byte("first"), 0600)
				Expect(err).NotTo(HaveOccurred())

				cmd := exec.Command("git", "add", ".")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "commit", "-m", "first release")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "tag", "v0.1.0")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				// Add ## Unreleased section with new entries
				err = os.WriteFile(
					changelogPath,
					[]byte(
						"# Changelog\n\n## Unreleased\n\n- Add second feature\n\n## v0.1.0\n\n- First feature\n",
					),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "add", ".")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "commit", "-m", "add unreleased section")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				// Now create another file for next release
				err = os.WriteFile(filepath.Join(tempDir, "second.txt"), []byte("second"), 0600)
				Expect(err).NotTo(HaveOccurred())
			})

			It("bumps to next version with PatchBump", func() {
				err := git.CommitAndRelease(ctx, git.PatchBump)
				Expect(err).To(BeNil())

				// Verify new tag was created
				cmd := exec.Command("git", "tag", "-l")
				cmd.Dir = tempDir
				output, err := cmd.Output()
				Expect(err).NotTo(HaveOccurred())
				Expect(string(output)).To(ContainSubstring("v0.1.1"))

				// Verify CHANGELOG has both versions
				changelogContent, err := os.ReadFile(filepath.Join(tempDir, "CHANGELOG.md"))
				Expect(err).NotTo(HaveOccurred())
				Expect(string(changelogContent)).To(ContainSubstring("## v0.1.1"))
				Expect(string(changelogContent)).To(ContainSubstring("- Add second feature"))
				Expect(string(changelogContent)).To(ContainSubstring("## v0.1.0"))
			})
		})

		Context("with CHANGELOG without Unreleased section", func() {
			BeforeEach(func() {
				// Create a tag v0.1.0 first
				changelogPath := filepath.Join(tempDir, "CHANGELOG.md")
				err := os.WriteFile(
					changelogPath,
					[]byte("# Changelog\n\n## v0.1.0\n\n- Initial release\n"),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())

				cmd := exec.Command("git", "add", ".")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "commit", "-m", "initial release")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "tag", "v0.1.0")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				// Update CHANGELOG to not have Unreleased section (just version sections)
				err = os.WriteFile(
					changelogPath,
					[]byte(
						"# Changelog\n\nAll notable changes to this project will be documented in this file.\n\n## v0.1.0\n\n- Initial release\n",
					),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "add", ".")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "commit", "-m", "update changelog")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				// Create a new file to commit
				err = os.WriteFile(filepath.Join(tempDir, "test.txt"), []byte("test"), 0600)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns error when no Unreleased section exists", func() {
				err := git.CommitAndRelease(ctx, git.PatchBump)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("## Unreleased"))
			})
		})

		Context("with CHANGELOG with existing subsection in Unreleased", func() {
			BeforeEach(func() {
				// Update CHANGELOG to have subsection in Unreleased
				changelogPath := filepath.Join(tempDir, "CHANGELOG.md")
				err := os.WriteFile(
					changelogPath,
					[]byte(
						"# Changelog\n\n## Unreleased\n\n### Fixed\n- Some bug fix\n\n### Added\n- New feature\n",
					),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())

				cmd := exec.Command("git", "add", ".")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "commit", "-m", "update changelog")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				// Create a new file to commit
				err = os.WriteFile(filepath.Join(tempDir, "test.txt"), []byte("test"), 0600)
				Expect(err).NotTo(HaveOccurred())
			})

			It("renames Unreleased to version and preserves all subsections", func() {
				err := git.CommitAndRelease(ctx, git.PatchBump)
				Expect(err).To(BeNil())

				// Verify CHANGELOG has version WITH subsections preserved
				changelogContent, err := os.ReadFile(filepath.Join(tempDir, "CHANGELOG.md"))
				Expect(err).NotTo(HaveOccurred())
				Expect(string(changelogContent)).To(ContainSubstring("## v0.1.0"))
				// Subsections should be preserved
				Expect(string(changelogContent)).To(ContainSubstring("### Fixed"))
				Expect(string(changelogContent)).To(ContainSubstring("- Some bug fix"))
				Expect(string(changelogContent)).To(ContainSubstring("### Added"))
				Expect(string(changelogContent)).To(ContainSubstring("- New feature"))
				// Should NOT contain ## Unreleased anymore
				Expect(string(changelogContent)).NotTo(ContainSubstring("## Unreleased"))
			})
		})

		Context("with empty Unreleased section", func() {
			BeforeEach(func() {
				// Update CHANGELOG to have empty Unreleased section
				changelogPath := filepath.Join(tempDir, "CHANGELOG.md")
				err := os.WriteFile(
					changelogPath,
					[]byte("# Changelog\n\n## Unreleased\n\n## v0.0.1\n\n- Initial release\n"),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())

				cmd := exec.Command("git", "add", ".")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "commit", "-m", "update changelog")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				// Tag v0.0.1
				cmd = exec.Command("git", "tag", "v0.0.1")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				// Create a new file to commit
				err = os.WriteFile(filepath.Join(tempDir, "test.txt"), []byte("test"), 0600)
				Expect(err).NotTo(HaveOccurred())
			})

			It("renames empty Unreleased to version", func() {
				err := git.CommitAndRelease(ctx, git.PatchBump)
				Expect(err).To(BeNil())

				// Verify CHANGELOG renamed empty section
				changelogContent, err := os.ReadFile(filepath.Join(tempDir, "CHANGELOG.md"))
				Expect(err).NotTo(HaveOccurred())
				Expect(string(changelogContent)).To(ContainSubstring("## v0.0.2"))
				Expect(string(changelogContent)).NotTo(ContainSubstring("## Unreleased"))
				// Should still have old version
				Expect(string(changelogContent)).To(ContainSubstring("## v0.0.1"))
			})
		})

		Context("with multiple entries under Unreleased", func() {
			BeforeEach(func() {
				// Update CHANGELOG to have multiple entries
				changelogPath := filepath.Join(tempDir, "CHANGELOG.md")
				err := os.WriteFile(
					changelogPath,
					[]byte(
						"# Changelog\n\n## Unreleased\n\n- Add feature A\n- Add feature B\n- Fix bug C\n- Update documentation D\n",
					),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())

				cmd := exec.Command("git", "add", ".")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "commit", "-m", "update changelog")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				// Create a new file to commit
				err = os.WriteFile(filepath.Join(tempDir, "test.txt"), []byte("test"), 0600)
				Expect(err).NotTo(HaveOccurred())
			})

			It("preserves all entries after renaming", func() {
				err := git.CommitAndRelease(ctx, git.PatchBump)
				Expect(err).To(BeNil())

				// Verify all entries are preserved
				changelogContent, err := os.ReadFile(filepath.Join(tempDir, "CHANGELOG.md"))
				Expect(err).NotTo(HaveOccurred())
				Expect(string(changelogContent)).To(ContainSubstring("## v0.1.0"))
				Expect(string(changelogContent)).To(ContainSubstring("- Add feature A"))
				Expect(string(changelogContent)).To(ContainSubstring("- Add feature B"))
				Expect(string(changelogContent)).To(ContainSubstring("- Fix bug C"))
				Expect(string(changelogContent)).To(ContainSubstring("- Update documentation D"))
				Expect(string(changelogContent)).NotTo(ContainSubstring("## Unreleased"))
			})
		})
	})

	Describe("CommitCompletedFile", func() {
		var (
			ctx         context.Context
			tempDir     string
			originalDir string
		)

		BeforeEach(func() {
			ctx = context.Background()

			var err error
			originalDir, err = os.Getwd()
			Expect(err).NotTo(HaveOccurred())

			tempDir, err = os.MkdirTemp("", "git-completed-test-*")
			Expect(err).NotTo(HaveOccurred())

			// Initialize git repo
			cmd := exec.Command("git", "init")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Configure git
			cmd = exec.Command("git", "config", "user.email", "test@example.com")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("git", "config", "user.name", "Test User")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Initial commit
			err = os.WriteFile(filepath.Join(tempDir, "README.md"), []byte("# Test"), 0600)
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("git", "add", ".")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("git", "commit", "-m", "initial commit")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Change to temp directory
			err = os.Chdir(tempDir)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if originalDir != "" {
				err := os.Chdir(originalDir)
				Expect(err).NotTo(HaveOccurred())
			}

			if tempDir != "" {
				_ = os.RemoveAll(tempDir)
			}
		})

		Context("with a new completed file", func() {
			var completedFilePath string

			BeforeEach(func() {
				// Create completed directory
				err := os.MkdirAll(filepath.Join(tempDir, "prompts", "completed"), 0750)
				Expect(err).NotTo(HaveOccurred())

				// Create a completed file
				completedFilePath = filepath.Join(tempDir, "prompts", "completed", "001-test.md")
				err = os.WriteFile(
					completedFilePath,
					[]byte("# Test prompt\nstatus: completed"),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())
			})

			It("stages and commits the file", func() {
				err := git.CommitCompletedFile(ctx, completedFilePath)
				Expect(err).To(BeNil())

				// Verify commit was created
				cmd := exec.Command("git", "log", "--oneline", "-n", "1")
				cmd.Dir = tempDir
				output, err := cmd.Output()
				Expect(err).NotTo(HaveOccurred())
				Expect(string(output)).To(ContainSubstring("move prompt to completed"))

				// Verify file is in git
				cmd = exec.Command("git", "ls-files")
				cmd.Dir = tempDir
				output, err = cmd.Output()
				Expect(err).NotTo(HaveOccurred())
				Expect(string(output)).To(ContainSubstring("prompts/completed/001-test.md"))
			})
		})

		Context("with file already committed", func() {
			var completedFilePath string

			BeforeEach(func() {
				// Create and commit a completed file
				err := os.MkdirAll(filepath.Join(tempDir, "prompts", "completed"), 0750)
				Expect(err).NotTo(HaveOccurred())

				completedFilePath = filepath.Join(tempDir, "prompts", "completed", "001-test.md")
				err = os.WriteFile(
					completedFilePath,
					[]byte("# Test prompt\nstatus: completed"),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())

				cmd := exec.Command("git", "add", ".")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "commit", "-m", "add completed file")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())
			})

			It("does nothing when file is already committed", func() {
				// Get commit count before
				cmd := exec.Command("git", "rev-list", "--count", "HEAD")
				cmd.Dir = tempDir
				beforeOutput, err := cmd.Output()
				Expect(err).NotTo(HaveOccurred())

				err = git.CommitCompletedFile(ctx, completedFilePath)
				Expect(err).To(BeNil())

				// Get commit count after
				cmd = exec.Command("git", "rev-list", "--count", "HEAD")
				cmd.Dir = tempDir
				afterOutput, err := cmd.Output()
				Expect(err).NotTo(HaveOccurred())

				// Verify no new commit was created
				Expect(string(beforeOutput)).To(Equal(string(afterOutput)))
			})
		})

		Context("with modified but unstaged file", func() {
			var completedFilePath string

			BeforeEach(func() {
				// Create and commit a completed file first
				err := os.MkdirAll(filepath.Join(tempDir, "prompts", "completed"), 0750)
				Expect(err).NotTo(HaveOccurred())

				completedFilePath = filepath.Join(tempDir, "prompts", "completed", "001-test.md")
				err = os.WriteFile(
					completedFilePath,
					[]byte("# Test prompt\nstatus: completed"),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())

				cmd := exec.Command("git", "add", ".")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "commit", "-m", "add completed file")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				// Modify the file but don't stage it
				err = os.WriteFile(
					completedFilePath,
					[]byte("# Test prompt\nstatus: completed\n\nModified content"),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())
			})

			It("stages and commits the modification", func() {
				err := git.CommitCompletedFile(ctx, completedFilePath)
				Expect(err).To(BeNil())

				// Verify commit was created
				cmd := exec.Command("git", "log", "--oneline", "-n", "1")
				cmd.Dir = tempDir
				output, err := cmd.Output()
				Expect(err).NotTo(HaveOccurred())
				Expect(string(output)).To(ContainSubstring("move prompt to completed"))

				// Verify modification is committed
				cmd = exec.Command("git", "diff", "HEAD")
				cmd.Dir = tempDir
				output, err = cmd.Output()
				Expect(err).NotTo(HaveOccurred())
				Expect(string(output)).To(BeEmpty()) // No uncommitted changes
			})
		})
	})

	Describe("HasChangelog", func() {
		var (
			ctx         context.Context
			tempDir     string
			originalDir string
			r           git.Releaser
		)

		BeforeEach(func() {
			ctx = context.Background()

			var err error
			originalDir, err = os.Getwd()
			Expect(err).NotTo(HaveOccurred())

			tempDir, err = os.MkdirTemp("", "git-changelog-test-*")
			Expect(err).NotTo(HaveOccurred())

			// Change to temp directory
			err = os.Chdir(tempDir)
			Expect(err).NotTo(HaveOccurred())

			r = git.NewReleaser()
		})

		AfterEach(func() {
			if originalDir != "" {
				err := os.Chdir(originalDir)
				Expect(err).NotTo(HaveOccurred())
			}

			if tempDir != "" {
				_ = os.RemoveAll(tempDir)
			}
		})

		Context("with CHANGELOG.md present", func() {
			BeforeEach(func() {
				err := os.WriteFile(
					filepath.Join(tempDir, "CHANGELOG.md"),
					[]byte("# Changelog\n"),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns true", func() {
				result := r.HasChangelog(ctx)
				Expect(result).To(BeTrue())
			})
		})

		Context("without CHANGELOG.md", func() {
			It("returns false", func() {
				result := r.HasChangelog(ctx)
				Expect(result).To(BeFalse())
			})
		})
	})

	Describe("MoveFile", func() {
		var (
			ctx         context.Context
			tempDir     string
			originalDir string
			r           git.Releaser
		)

		BeforeEach(func() {
			ctx = context.Background()

			var err error
			originalDir, err = os.Getwd()
			Expect(err).NotTo(HaveOccurred())

			tempDir, err = os.MkdirTemp("", "git-movefile-test-*")
			Expect(err).NotTo(HaveOccurred())

			// Initialize git repo
			cmd := exec.Command("git", "init")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Configure git
			cmd = exec.Command("git", "config", "user.email", "test@example.com")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("git", "config", "user.name", "Test User")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Initial commit
			err = os.WriteFile(filepath.Join(tempDir, "README.md"), []byte("# Test"), 0600)
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("git", "add", ".")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("git", "commit", "-m", "initial commit")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Change to temp directory
			err = os.Chdir(tempDir)
			Expect(err).NotTo(HaveOccurred())

			r = git.NewReleaser()
		})

		AfterEach(func() {
			if originalDir != "" {
				err := os.Chdir(originalDir)
				Expect(err).NotTo(HaveOccurred())
			}

			if tempDir != "" {
				_ = os.RemoveAll(tempDir)
			}
		})

		Context("in a git repository", func() {
			It("moves file using git mv", func() {
				// Create and commit a file
				oldPath := filepath.Join(tempDir, "old-name.md")
				err := os.WriteFile(oldPath, []byte("test content"), 0600)
				Expect(err).NotTo(HaveOccurred())

				cmd := exec.Command("git", "add", ".")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "commit", "-m", "add file")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				// Move file
				newPath := filepath.Join(tempDir, "new-name.md")
				err = r.MoveFile(ctx, oldPath, newPath)
				Expect(err).To(BeNil())

				// Verify old file doesn't exist
				_, err = os.Stat(oldPath)
				Expect(os.IsNotExist(err)).To(BeTrue())

				// Verify new file exists
				_, err = os.Stat(newPath)
				Expect(err).NotTo(HaveOccurred())

				// Verify content is preserved
				content, err := os.ReadFile(newPath)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(content)).To(Equal("test content"))

				// Verify git status shows the file as renamed (staged)
				cmd = exec.Command("git", "status", "--short")
				cmd.Dir = tempDir
				output, err := cmd.Output()
				Expect(err).NotTo(HaveOccurred())
				Expect(string(output)).To(ContainSubstring("R"))
				Expect(string(output)).To(ContainSubstring("old-name.md"))
				Expect(string(output)).To(ContainSubstring("new-name.md"))
			})

			It("moves file to subdirectory", func() {
				// Create and commit a file
				oldPath := filepath.Join(tempDir, "file.md")
				err := os.WriteFile(oldPath, []byte("test"), 0600)
				Expect(err).NotTo(HaveOccurred())

				cmd := exec.Command("git", "add", ".")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "commit", "-m", "add file")
				cmd.Dir = tempDir
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				// Create subdirectory
				subDir := filepath.Join(tempDir, "completed")
				err = os.MkdirAll(subDir, 0750)
				Expect(err).NotTo(HaveOccurred())

				// Move file
				newPath := filepath.Join(subDir, "file.md")
				err = r.MoveFile(ctx, oldPath, newPath)
				Expect(err).To(BeNil())

				// Verify file was moved
				_, err = os.Stat(newPath)
				Expect(err).NotTo(HaveOccurred())

				_, err = os.Stat(oldPath)
				Expect(os.IsNotExist(err)).To(BeTrue())
			})
		})

		Context("outside a git repository", func() {
			BeforeEach(func() {
				// Create a new temp dir without git
				nonGitDir, err := os.MkdirTemp("", "non-git-test-*")
				Expect(err).NotTo(HaveOccurred())

				err = os.Chdir(nonGitDir)
				Expect(err).NotTo(HaveOccurred())

				tempDir = nonGitDir
			})

			It("falls back to os.Rename", func() {
				// Create a file
				oldPath := filepath.Join(tempDir, "old.md")
				err := os.WriteFile(oldPath, []byte("content"), 0600)
				Expect(err).NotTo(HaveOccurred())

				// Move file
				newPath := filepath.Join(tempDir, "new.md")
				err = r.MoveFile(ctx, oldPath, newPath)
				Expect(err).To(BeNil())

				// Verify file was moved
				_, err = os.Stat(newPath)
				Expect(err).NotTo(HaveOccurred())

				_, err = os.Stat(oldPath)
				Expect(os.IsNotExist(err)).To(BeTrue())

				// Verify content
				content, err := os.ReadFile(newPath)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(content)).To(Equal("content"))
			})
		})
	})

	Describe("CommitOnly", func() {
		var (
			ctx         context.Context
			tempDir     string
			originalDir string
			r           git.Releaser
		)

		BeforeEach(func() {
			ctx = context.Background()

			var err error
			originalDir, err = os.Getwd()
			Expect(err).NotTo(HaveOccurred())

			tempDir, err = os.MkdirTemp("", "git-commit-only-test-*")
			Expect(err).NotTo(HaveOccurred())

			// Initialize git repo
			cmd := exec.Command("git", "init")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Configure git
			cmd = exec.Command("git", "config", "user.email", "test@example.com")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("git", "config", "user.name", "Test User")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Initial commit
			err = os.WriteFile(filepath.Join(tempDir, "README.md"), []byte("# Test"), 0600)
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("git", "add", ".")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("git", "commit", "-m", "initial commit")
			cmd.Dir = tempDir
			err = cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Change to temp directory
			err = os.Chdir(tempDir)
			Expect(err).NotTo(HaveOccurred())

			r = git.NewReleaser()
		})

		AfterEach(func() {
			if originalDir != "" {
				err := os.Chdir(originalDir)
				Expect(err).NotTo(HaveOccurred())
			}

			if tempDir != "" {
				_ = os.RemoveAll(tempDir)
			}
		})

		Context("with changes to commit", func() {
			BeforeEach(func() {
				// Create a new file
				err := os.WriteFile(
					filepath.Join(tempDir, "test.txt"),
					[]byte("test content"),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())
			})

			It("commits without creating tag", func() {
				err := r.CommitOnly(ctx, "Add test feature")
				Expect(err).To(BeNil())

				// Verify commit was created
				cmd := exec.Command("git", "log", "--oneline", "-n", "1")
				cmd.Dir = tempDir
				output, err := cmd.Output()
				Expect(err).NotTo(HaveOccurred())
				Expect(string(output)).To(ContainSubstring("Add test feature"))

				// Verify NO tag was created
				cmd = exec.Command("git", "tag", "-l")
				cmd.Dir = tempDir
				output, err = cmd.Output()
				Expect(err).NotTo(HaveOccurred())
				Expect(string(output)).To(BeEmpty())
			})
		})

		Context("with special characters in message", func() {
			BeforeEach(func() {
				// Create a new file
				err := os.WriteFile(filepath.Join(tempDir, "special.txt"), []byte("content"), 0600)
				Expect(err).NotTo(HaveOccurred())
			})

			It("handles special characters in commit message", func() {
				err := r.CommitOnly(ctx, "Fix: handle \"quotes\" and 'apostrophes'")
				Expect(err).To(BeNil())

				// Verify commit was created with correct message
				cmd := exec.Command("git", "log", "--oneline", "-n", "1")
				cmd.Dir = tempDir
				output, err := cmd.Output()
				Expect(err).NotTo(HaveOccurred())
				Expect(string(output)).To(ContainSubstring("Fix:"))
			})
		})

		Context("with no changes to commit", func() {
			It("returns error when nothing to commit", func() {
				err := r.CommitOnly(ctx, "Empty commit")
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("git commit"))
			})
		})
	})
})

var _ = Describe("CommitWithRetry", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("returns nil when fn succeeds on first attempt", func() {
		callCount := 0
		fn := func(_ context.Context) error {
			callCount++
			return nil
		}
		testBackoff := run.Backoff{Delay: 0, Factor: 0, Retries: 3}
		err := git.CommitWithRetry(ctx, testBackoff, fn)
		Expect(err).NotTo(HaveOccurred())
		Expect(callCount).To(Equal(1))
	})

	It("retries and returns nil when fn fails once then succeeds", func() {
		callCount := 0
		fn := func(ctx context.Context) error {
			callCount++
			if callCount < 2 {
				return errors.Errorf(ctx, "simulated failure")
			}
			return nil
		}
		testBackoff := run.Backoff{Delay: 0, Factor: 0, Retries: 3}
		err := git.CommitWithRetry(ctx, testBackoff, fn)
		Expect(err).NotTo(HaveOccurred())
		Expect(callCount).To(Equal(2))
	})

	It("returns error when fn fails all retries", func() {
		callCount := 0
		fn := func(ctx context.Context) error {
			callCount++
			return errors.Errorf(ctx, "simulated failure")
		}
		testBackoff := run.Backoff{Delay: 0, Factor: 0, Retries: 3}
		err := git.CommitWithRetry(ctx, testBackoff, fn)
		Expect(err).To(HaveOccurred())
		Expect(callCount).To(Equal(4)) // 1 initial + 3 retries
	})

	It("returns error when context is already cancelled", func() {
		cancelledCtx, cancel := context.WithCancel(ctx)
		cancel()
		callCount := 0
		fn := func(_ context.Context) error {
			callCount++
			return nil
		}
		testBackoff := run.Backoff{Delay: 0, Factor: 0, Retries: 3}
		err := git.CommitWithRetry(cancelledCtx, testBackoff, fn)
		Expect(err).To(HaveOccurred())
		Expect(callCount).To(Equal(0))
	})
})
