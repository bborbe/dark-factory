// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/git"
)

var _ = Describe("Git", func() {
	Describe("BumpPatchVersion", func() {
		Context("with valid semver tags", func() {
			It("bumps patch version from v0.1.0", func() {
				result, err := git.BumpPatchVersion("v0.1.0")
				Expect(err).To(BeNil())
				Expect(result).To(Equal("v0.1.1"))
			})

			It("bumps patch version from v1.2.3", func() {
				result, err := git.BumpPatchVersion("v1.2.3")
				Expect(err).To(BeNil())
				Expect(result).To(Equal("v1.2.4"))
			})

			It("bumps patch version from v10.20.99", func() {
				result, err := git.BumpPatchVersion("v10.20.99")
				Expect(err).To(BeNil())
				Expect(result).To(Equal("v10.20.100"))
			})
		})

		Context("with invalid tags", func() {
			It("returns error for non-semver tag", func() {
				_, err := git.BumpPatchVersion("invalid")
				Expect(err).NotTo(BeNil())
			})

			It("returns error for tag without v prefix", func() {
				_, err := git.BumpPatchVersion("1.2.3")
				Expect(err).NotTo(BeNil())
			})

			It("returns error for incomplete version", func() {
				_, err := git.BumpPatchVersion("v1.2")
				Expect(err).NotTo(BeNil())
			})
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

				version, err := git.GetNextVersion(ctx)
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

			It("returns v0.1.1", func() {
				version, err := git.GetNextVersion(ctx)
				Expect(err).To(BeNil())
				Expect(version).To(Equal("v0.1.1"))
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

			It("returns v1.2.4", func() {
				version, err := git.GetNextVersion(ctx)
				Expect(err).To(BeNil())
				Expect(version).To(Equal("v1.2.4"))
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

			It("returns bumped version of latest tag", func() {
				version, err := git.GetNextVersion(ctx)
				Expect(err).To(BeNil())
				Expect(version).To(Equal("v0.2.1"))
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
				[]byte("# Changelog\n\n## Unreleased\n\n### Added\n"),
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

		Context("with changes to commit", func() {
			BeforeEach(func() {
				// Create a new file to commit
				err := os.WriteFile(
					filepath.Join(tempDir, "test.txt"),
					[]byte("test content"),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())
			})

			It("creates commit, tag, and updates CHANGELOG", func() {
				err := git.CommitAndRelease(ctx, "Add test feature")
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

				// Verify CHANGELOG was updated
				changelogContent, err := os.ReadFile(filepath.Join(tempDir, "CHANGELOG.md"))
				Expect(err).NotTo(HaveOccurred())
				Expect(string(changelogContent)).To(ContainSubstring("## v0.1.0"))
				Expect(string(changelogContent)).To(ContainSubstring("- Add test feature"))
			})
		})

		Context("with existing version tag", func() {
			BeforeEach(func() {
				// Create a file and commit it with v0.1.0 tag
				err := os.WriteFile(filepath.Join(tempDir, "first.txt"), []byte("first"), 0600)
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

				// Now create another file for next release
				err = os.WriteFile(filepath.Join(tempDir, "second.txt"), []byte("second"), 0600)
				Expect(err).NotTo(HaveOccurred())
			})

			It("bumps to next version", func() {
				err := git.CommitAndRelease(ctx, "Add second feature")
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
			})
		})

		Context("with CHANGELOG without Unreleased section", func() {
			BeforeEach(func() {
				// Update CHANGELOG to not have Unreleased section
				changelogPath := filepath.Join(tempDir, "CHANGELOG.md")
				err := os.WriteFile(
					changelogPath,
					[]byte(
						"# Changelog\n\nAll notable changes to this project will be documented in this file.\n",
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

			It("inserts new version section", func() {
				err := git.CommitAndRelease(ctx, "Add feature without unreleased")
				Expect(err).To(BeNil())

				// Verify CHANGELOG has new version
				changelogContent, err := os.ReadFile(filepath.Join(tempDir, "CHANGELOG.md"))
				Expect(err).NotTo(HaveOccurred())
				Expect(string(changelogContent)).To(ContainSubstring("## v0.1.0"))
				Expect(
					string(changelogContent),
				).To(ContainSubstring("- Add feature without unreleased"))
			})
		})

		Context("with CHANGELOG with existing subsection in Unreleased", func() {
			BeforeEach(func() {
				// Update CHANGELOG to have subsection in Unreleased
				changelogPath := filepath.Join(tempDir, "CHANGELOG.md")
				err := os.WriteFile(
					changelogPath,
					[]byte("# Changelog\n\n## Unreleased\n### Fixed\n- Some bug fix\n"),
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

			It("adds entry under existing subsection", func() {
				err := git.CommitAndRelease(ctx, "Add new feature")
				Expect(err).To(BeNil())

				// Verify CHANGELOG has version with subsection preserved
				changelogContent, err := os.ReadFile(filepath.Join(tempDir, "CHANGELOG.md"))
				Expect(err).NotTo(HaveOccurred())
				Expect(string(changelogContent)).To(ContainSubstring("## v0.1.0"))
				Expect(string(changelogContent)).To(ContainSubstring("### Fixed"))
				Expect(string(changelogContent)).To(ContainSubstring("- Add new feature"))
			})
		})
	})
})
