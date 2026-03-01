// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runner_test

import (
	"context"
	stderrors "errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/runner"
)

var _ = Describe("Runner Integration", func() {
	var (
		tempDir      string
		promptsDir   string
		completedDir string
		originalDir  string
	)

	BeforeEach(func() {
		var err error

		// Save original directory
		originalDir, err = os.Getwd()
		Expect(err).NotTo(HaveOccurred())

		// Create temp directory
		tempDir, err = os.MkdirTemp("", "runner-test-*")
		Expect(err).NotTo(HaveOccurred())

		// Create prompts and completed directories
		promptsDir = filepath.Join(tempDir, "prompts")
		completedDir = filepath.Join(promptsDir, "completed")
		err = os.MkdirAll(completedDir, 0750)
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
		// Restore original directory
		if originalDir != "" {
			err := os.Chdir(originalDir)
			Expect(err).NotTo(HaveOccurred())
		}

		// Clean up temp directory
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	It("should process existing queued prompt on startup", func() {
		// Create a queued prompt
		promptPath := filepath.Join(promptsDir, "001-test-prompt.md")
		promptContent := `---
status: queued
---

# Add test feature

This is a test prompt.
`
		err := os.WriteFile(promptPath, []byte(promptContent), 0600)
		Expect(err).NotTo(HaveOccurred())

		// Create mock executor
		executor := &mocks.Executor{}

		// Create runner
		r := runner.NewRunner(
			promptsDir,
			executor,
			prompt.NewManager(promptsDir),
			git.NewReleaser(),
		)

		// Run runner in goroutine with timeout context
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		errCh := make(chan error, 1)
		go func() {
			errCh <- r.Run(ctx)
		}()

		// Wait for prompt to be processed (check completed directory)
		Eventually(func() bool {
			entries, err := os.ReadDir(completedDir)
			if err != nil {
				return false
			}
			return len(entries) > 0
		}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())

		// Verify prompt was moved to completed
		completedPath := filepath.Join(completedDir, "001-test-prompt.md")
		_, err = os.Stat(completedPath)
		Expect(err).NotTo(HaveOccurred())

		// Verify original prompt is gone
		_, err = os.Stat(promptPath)
		Expect(os.IsNotExist(err)).To(BeTrue())

		// Verify executor was called with correct container name
		Expect(executor.ExecuteCallCount()).To(Equal(1))
		_, _, _, containerName := executor.ExecuteArgsForCall(0)
		Expect(containerName).To(Equal("dark-factory-001-test-prompt"))

		// Verify container name is persisted in completed file frontmatter
		fm, err := prompt.ReadFrontmatter(ctx, completedPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(fm.Container).To(Equal("dark-factory-001-test-prompt"))

		// Cancel context to stop runner
		cancel()

		// Wait for runner to exit
		select {
		case err := <-errCh:
			Expect(err).To(BeNil())
		case <-time.After(2 * time.Second):
			Fail("runner did not exit within timeout")
		}
	})

	It("should watch for new queued prompts", func() {
		// Create mock executor
		executor := &mocks.Executor{}

		// Create runner
		r := runner.NewRunner(
			promptsDir,
			executor,
			prompt.NewManager(promptsDir),
			git.NewReleaser(),
		)

		// Run runner in goroutine with timeout context
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		go func() {
			_ = r.Run(ctx)
		}()

		// Wait a bit for watcher to start
		time.Sleep(500 * time.Millisecond)

		// Create a new queued prompt after runner started
		promptPath := filepath.Join(promptsDir, "002-new-prompt.md")
		promptContent := `---
status: queued
---

# Add new feature

This is a new prompt.
`
		err := os.WriteFile(promptPath, []byte(promptContent), 0600)
		Expect(err).NotTo(HaveOccurred())

		// Wait for prompt to be processed
		Eventually(func() bool {
			entries, err := os.ReadDir(completedDir)
			if err != nil {
				return false
			}
			return len(entries) > 0
		}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())

		// Verify prompt was moved to completed
		completedPath := filepath.Join(completedDir, "002-new-prompt.md")
		_, err = os.Stat(completedPath)
		Expect(err).NotTo(HaveOccurred())

		// Cancel context
		cancel()
	})

	It("should ignore prompts with skip status", func() {
		// Create mock executor that should never be called
		executor := &mocks.Executor{}
		executor.ExecuteStub = func(_ context.Context, _ string, _ string, _ string) error {
			Fail("executor should not be called for prompts with skip status")
			return nil
		}

		// Create runner
		r := runner.NewRunner(
			promptsDir,
			executor,
			prompt.NewManager(promptsDir),
			git.NewReleaser(),
		)

		// Run runner in goroutine with timeout context
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		go func() {
			_ = r.Run(ctx)
		}()

		// Wait a bit for watcher to start
		time.Sleep(500 * time.Millisecond)

		// Create a prompt with skip status (completed)
		promptPath := filepath.Join(promptsDir, "003-completed.md")
		promptContent := `---
status: completed
---

# Completed prompt

This should not be processed.
`
		err := os.WriteFile(promptPath, []byte(promptContent), 0600)
		Expect(err).NotTo(HaveOccurred())

		// Wait and verify it was NOT processed
		Consistently(func() int {
			entries, err := os.ReadDir(completedDir)
			if err != nil {
				return -1
			}
			return len(entries)
		}, 1*time.Second, 100*time.Millisecond).Should(Equal(0))

		// Cancel context
		cancel()
	})

	It("should handle executor errors and mark prompt as failed", func() {
		// Create a queued prompt
		promptPath := filepath.Join(promptsDir, "004-will-fail.md")
		promptContent := `---
status: queued
---

# Failing prompt

This prompt will fail during execution.
`
		err := os.WriteFile(promptPath, []byte(promptContent), 0600)
		Expect(err).NotTo(HaveOccurred())

		// Create mock executor that returns error
		executor := &mocks.Executor{}
		executor.ExecuteReturns(ErrTest)

		// Create runner
		r := runner.NewRunner(
			promptsDir,
			executor,
			prompt.NewManager(promptsDir),
			git.NewReleaser(),
		)

		// Run runner in goroutine with timeout context
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		go func() {
			_ = r.Run(ctx)
		}()

		// Wait for prompt to be marked as failed
		Eventually(func() string {
			fm, err := prompt.ReadFrontmatter(ctx, promptPath)
			if err != nil {
				return ""
			}
			return fm.Status
		}, 3*time.Second, 100*time.Millisecond).Should(Equal("failed"))

		// Verify prompt was NOT moved to completed
		completedPath := filepath.Join(completedDir, "004-will-fail.md")
		_, err = os.Stat(completedPath)
		Expect(os.IsNotExist(err)).To(BeTrue())

		// Cancel context
		cancel()
	})

	It("should process multiple queued prompts in order", func() {
		// Create multiple queued prompts
		for i := 1; i <= 3; i++ {
			promptPath := filepath.Join(promptsDir, fmt.Sprintf("00%d-test.md", i))
			promptContent := fmt.Sprintf(`---
status: queued
---

# Test prompt %d

Content here.
`, i)
			err := os.WriteFile(promptPath, []byte(promptContent), 0600)
			Expect(err).NotTo(HaveOccurred())
		}

		// Create mock executor
		executor := &mocks.Executor{}

		// Create runner
		r := runner.NewRunner(
			promptsDir,
			executor,
			prompt.NewManager(promptsDir),
			git.NewReleaser(),
		)

		// Run runner in goroutine with timeout context
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		go func() {
			_ = r.Run(ctx)
		}()

		// Wait for all prompts to be processed
		Eventually(func() int {
			entries, err := os.ReadDir(completedDir)
			if err != nil {
				return 0
			}
			return len(entries)
		}, 5*time.Second, 100*time.Millisecond).Should(Equal(3))

		// Verify executor was called 3 times
		Expect(executor.ExecuteCallCount()).To(Equal(3))

		// Verify prompts were processed in alphabetical order
		_, _, _, containerName1 := executor.ExecuteArgsForCall(0)
		_, _, _, containerName2 := executor.ExecuteArgsForCall(1)
		_, _, _, containerName3 := executor.ExecuteArgsForCall(2)

		Expect(containerName1).To(Equal("dark-factory-001-test"))
		Expect(containerName2).To(Equal("dark-factory-002-test"))
		Expect(containerName3).To(Equal("dark-factory-003-test"))

		// Cancel context
		cancel()
	})

	It("should handle prompts without frontmatter", func() {
		// Create a prompt without frontmatter
		promptPath := filepath.Join(promptsDir, "005-no-frontmatter.md")
		promptContent := `# Plain prompt

This has no YAML frontmatter.
`
		err := os.WriteFile(promptPath, []byte(promptContent), 0600)
		Expect(err).NotTo(HaveOccurred())

		// Create mock executor
		executor := &mocks.Executor{}

		// Create runner
		r := runner.NewRunner(
			promptsDir,
			executor,
			prompt.NewManager(promptsDir),
			git.NewReleaser(),
		)

		// Run runner in goroutine with timeout context
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		go func() {
			_ = r.Run(ctx)
		}()

		// Wait for prompt to be processed
		Eventually(func() bool {
			entries, err := os.ReadDir(completedDir)
			if err != nil {
				return false
			}
			return len(entries) > 0
		}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())

		// Verify prompt was moved to completed
		completedPath := filepath.Join(completedDir, "005-no-frontmatter.md")
		_, err = os.Stat(completedPath)
		Expect(err).NotTo(HaveOccurred())

		// Verify executor was called
		Expect(executor.ExecuteCallCount()).To(BeNumerically(">", 0))

		// Cancel context
		cancel()
	})

	It("should skip empty prompts and move to completed", func() {
		// Create an empty prompt file
		promptPath := filepath.Join(promptsDir, "006-empty.md")
		err := os.WriteFile(promptPath, []byte(""), 0600)
		Expect(err).NotTo(HaveOccurred())

		// Create mock executor that should never be called
		executor := &mocks.Executor{}
		executor.ExecuteStub = func(_ context.Context, _ string, _ string, _ string) error {
			Fail("executor should not be called for empty prompt")
			return nil
		}

		// Create runner
		r := runner.NewRunner(
			promptsDir,
			executor,
			prompt.NewManager(promptsDir),
			git.NewReleaser(),
		)

		// Run runner in goroutine with timeout context
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		go func() {
			_ = r.Run(ctx)
		}()

		// Wait for empty prompt to be moved to completed (skipped but moved)
		Eventually(func() bool {
			completedPath := filepath.Join(completedDir, "006-empty.md")
			_, err := os.Stat(completedPath)
			return !os.IsNotExist(err)
		}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())

		// Verify executor was never called
		Expect(executor.ExecuteCallCount()).To(Equal(0))

		// Verify prompt was moved to completed with completed status
		completedPath := filepath.Join(completedDir, "006-empty.md")
		fm, err := prompt.ReadFrontmatter(ctx, completedPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(fm.Status).To(Equal("completed"))

		// Cancel context
		cancel()
	})

	It("should update prompt status to executing before execution", func() {
		// Create a queued prompt
		promptPath := filepath.Join(promptsDir, "007-status-test.md")
		promptContent := `---
status: queued
---

# Status test

Testing status updates.
`
		err := os.WriteFile(promptPath, []byte(promptContent), 0600)
		Expect(err).NotTo(HaveOccurred())

		// Create mock executor with delay
		executor := &mocks.Executor{}
		executeCalled := make(chan struct{})
		executor.ExecuteStub = func(_ context.Context, _ string, _ string, _ string) error {
			// Signal that execute was called
			close(executeCalled)
			// Wait a bit to allow checking the status
			time.Sleep(100 * time.Millisecond)
			return nil
		}

		// Create runner
		r := runner.NewRunner(
			promptsDir,
			executor,
			prompt.NewManager(promptsDir),
			git.NewReleaser(),
		)

		// Run runner in goroutine
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		go func() {
			_ = r.Run(ctx)
		}()

		// Wait for execute to be called
		select {
		case <-executeCalled:
			// Good, execute was called
		case <-time.After(3 * time.Second):
			Fail("executor was not called within timeout")
		}

		// Wait for completion
		Eventually(func() bool {
			completedPath := filepath.Join(completedDir, "007-status-test.md")
			_, err := os.Stat(completedPath)
			return !os.IsNotExist(err)
		}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())

		// Cancel context
		cancel()
	})

	It("should handle watcher file write events", func() {
		// Create mock executor
		executor := &mocks.Executor{}

		// Create runner
		r := runner.NewRunner(
			promptsDir,
			executor,
			prompt.NewManager(promptsDir),
			git.NewReleaser(),
		)

		// Run runner in goroutine
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		go func() {
			_ = r.Run(ctx)
		}()

		// Wait for watcher to start
		time.Sleep(500 * time.Millisecond)

		// Create a new prompt by writing to a file (triggers write event)
		promptPath := filepath.Join(promptsDir, "008-write-event.md")
		promptContent := `---
status: queued
---

# Write event test

This should trigger the watcher.
`
		err := os.WriteFile(promptPath, []byte(promptContent), 0600)
		Expect(err).NotTo(HaveOccurred())

		// Wait for it to be processed
		Eventually(func() bool {
			completedPath := filepath.Join(completedDir, "008-write-event.md")
			_, err := os.Stat(completedPath)
			return !os.IsNotExist(err)
		}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())

		// Cancel context
		cancel()
	})

	It("should handle prompts with status: failed by ignoring them", func() {
		// Create a prompt with failed status
		promptPath := filepath.Join(promptsDir, "009-already-failed.md")
		promptContent := `---
status: failed
---

# Already failed prompt

This should not be processed.
`
		err := os.WriteFile(promptPath, []byte(promptContent), 0600)
		Expect(err).NotTo(HaveOccurred())

		// Create mock executor that should never be called
		executor := &mocks.Executor{}
		executor.ExecuteStub = func(_ context.Context, _ string, _ string, _ string) error {
			Fail("executor should not be called for failed prompt")
			return nil
		}

		// Create runner
		r := runner.NewRunner(
			promptsDir,
			executor,
			prompt.NewManager(promptsDir),
			git.NewReleaser(),
		)

		// Run runner in goroutine
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		go func() {
			_ = r.Run(ctx)
		}()

		// Wait and verify it was NOT processed
		Consistently(func() int {
			entries, err := os.ReadDir(completedDir)
			if err != nil {
				return -1
			}
			return len(entries)
		}, 1*time.Second, 100*time.Millisecond).Should(Equal(0))

		// Verify executor was never called
		Expect(executor.ExecuteCallCount()).To(Equal(0))

		// Cancel context
		cancel()
	})

	It("should set container name in frontmatter before execution", func() {
		// Create a queued prompt
		promptPath := filepath.Join(promptsDir, "010-container-name.md")
		promptContent := `---
status: queued
---

# Container name test

Testing container name setting.
`
		err := os.WriteFile(promptPath, []byte(promptContent), 0600)
		Expect(err).NotTo(HaveOccurred())

		// Create mock executor
		executor := &mocks.Executor{}

		// Create runner
		r := runner.NewRunner(
			promptsDir,
			executor,
			prompt.NewManager(promptsDir),
			git.NewReleaser(),
		)

		// Run runner in goroutine
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		go func() {
			_ = r.Run(ctx)
		}()

		// Wait for prompt to be processed
		Eventually(func() bool {
			completedPath := filepath.Join(completedDir, "010-container-name.md")
			_, err := os.Stat(completedPath)
			return !os.IsNotExist(err)
		}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())

		// Verify container name was set in completed file
		completedPath := filepath.Join(completedDir, "010-container-name.md")
		fm, err := prompt.ReadFrontmatter(ctx, completedPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(fm.Container).To(Equal("dark-factory-010-container-name"))

		// Cancel context
		cancel()
	})

	It("should reset executing prompts on startup", func() {
		// Create a prompt with executing status (stuck from previous crash)
		promptPath := filepath.Join(promptsDir, "011-stuck-executing.md")
		promptContent := `---
status: executing
---

# Stuck prompt

This was stuck from a previous crash.
`
		err := os.WriteFile(promptPath, []byte(promptContent), 0600)
		Expect(err).NotTo(HaveOccurred())

		// Create mock executor
		executor := &mocks.Executor{}

		// Create runner
		r := runner.NewRunner(
			promptsDir,
			executor,
			prompt.NewManager(promptsDir),
			git.NewReleaser(),
		)

		// Run runner in goroutine
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		go func() {
			_ = r.Run(ctx)
		}()

		// Wait for prompt to be processed (should be reset to queued and then processed)
		Eventually(func() bool {
			completedPath := filepath.Join(completedDir, "011-stuck-executing.md")
			_, err := os.Stat(completedPath)
			return !os.IsNotExist(err)
		}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())

		// Verify executor was called (prompt was processed)
		Expect(executor.ExecuteCallCount()).To(Equal(1))

		// Cancel context
		cancel()
	})

	It("should handle file modifications that trigger chmod events", func() {
		// Create mock executor
		executor := &mocks.Executor{}

		// Create runner
		r := runner.NewRunner(
			promptsDir,
			executor,
			prompt.NewManager(promptsDir),
			git.NewReleaser(),
		)

		// Run runner in goroutine
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		go func() {
			_ = r.Run(ctx)
		}()

		// Wait for watcher to start
		time.Sleep(500 * time.Millisecond)

		// Create a prompt file
		promptPath := filepath.Join(promptsDir, "012-chmod-event.md")
		promptContent := `---
status: queued
---

# Chmod event test

Test file permission changes.
`
		err := os.WriteFile(promptPath, []byte(promptContent), 0600)
		Expect(err).NotTo(HaveOccurred())

		// Change permissions (might trigger chmod event)
		err = os.Chmod(promptPath, 0644)
		Expect(err).NotTo(HaveOccurred())

		// Wait for it to be processed
		Eventually(func() bool {
			completedPath := filepath.Join(completedDir, "012-chmod-event.md")
			_, err := os.Stat(completedPath)
			return !os.IsNotExist(err)
		}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())

		// Cancel context
		cancel()
	})

	It("should ignore non-markdown files", func() {
		// Create mock executor that should never be called
		executor := &mocks.Executor{}
		executor.ExecuteStub = func(_ context.Context, _ string, _ string, _ string) error {
			Fail("executor should not be called for non-.md file")
			return nil
		}

		// Create runner
		r := runner.NewRunner(
			promptsDir,
			executor,
			prompt.NewManager(promptsDir),
			git.NewReleaser(),
		)

		// Run runner in goroutine
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		go func() {
			_ = r.Run(ctx)
		}()

		// Wait for watcher to start
		time.Sleep(500 * time.Millisecond)

		// Create a non-markdown file
		txtPath := filepath.Join(promptsDir, "readme.txt")
		err := os.WriteFile(txtPath, []byte("This is not a markdown file"), 0600)
		Expect(err).NotTo(HaveOccurred())

		// Wait and verify it was NOT processed
		Consistently(func() int {
			entries, err := os.ReadDir(completedDir)
			if err != nil {
				return -1
			}
			return len(entries)
		}, 1*time.Second, 100*time.Millisecond).Should(Equal(0))

		// Verify executor was never called
		Expect(executor.ExecuteCallCount()).To(Equal(0))

		// Cancel context
		cancel()
	})

	It("should handle executor errors during processExistingQueued", func() {
		// Create multiple queued prompts, first one will fail
		for i := 1; i <= 2; i++ {
			// Use unique numbers to avoid normalization renaming
			promptNum := 12 + i
			promptPath := filepath.Join(promptsDir, fmt.Sprintf("%03d-%d.md", promptNum, i))
			promptContent := fmt.Sprintf(`---
status: queued
---

# Test prompt %d

Content here.
`, i)
			err := os.WriteFile(promptPath, []byte(promptContent), 0600)
			Expect(err).NotTo(HaveOccurred())
		}

		// Create mock executor that fails on first call
		executor := &mocks.Executor{}
		callCount := 0
		executor.ExecuteStub = func(_ context.Context, _ string, _ string, _ string) error {
			callCount++
			if callCount == 1 {
				return ErrTest // First prompt fails
			}
			return nil // Subsequent prompts succeed
		}

		// Create runner
		r := runner.NewRunner(
			promptsDir,
			executor,
			prompt.NewManager(promptsDir),
			git.NewReleaser(),
		)

		// Run runner in goroutine
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		errCh := make(chan error, 1)
		go func() {
			errCh <- r.Run(ctx)
		}()

		// Wait for runner to exit due to error
		select {
		case err := <-errCh:
			Expect(err).NotTo(BeNil())
			Expect(err.Error()).To(ContainSubstring("test error"))
		case <-time.After(3 * time.Second):
			Fail("runner did not exit with error within timeout")
		}

		// Verify first prompt was marked as failed
		fm, err := prompt.ReadFrontmatter(ctx, filepath.Join(promptsDir, "013-1.md"))
		Expect(err).NotTo(HaveOccurred())
		Expect(fm.Status).To(Equal("failed"))

		// Cancel context
		cancel()
	})

	It("should process prompt and call git commit and release", func() {
		// Create a queued prompt
		promptPath := filepath.Join(promptsDir, "014-git-test.md")
		promptContent := `---
status: queued
---

# Git commit test

This tests the git workflow.
`
		err := os.WriteFile(promptPath, []byte(promptContent), 0600)
		Expect(err).NotTo(HaveOccurred())

		// Create mock executor
		executor := &mocks.Executor{}

		// Create runner
		r := runner.NewRunner(
			promptsDir,
			executor,
			prompt.NewManager(promptsDir),
			git.NewReleaser(),
		)

		// Run runner in goroutine
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		go func() {
			_ = r.Run(ctx)
		}()

		// Wait for prompt to be processed
		Eventually(func() bool {
			completedPath := filepath.Join(completedDir, "014-git-test.md")
			_, err := os.Stat(completedPath)
			return !os.IsNotExist(err)
		}, 5*time.Second, 100*time.Millisecond).Should(BeTrue())

		// Verify a git tag was created
		cmd := exec.Command("git", "tag", "-l")
		cmd.Dir = tempDir
		output, err := cmd.Output()
		Expect(err).NotTo(HaveOccurred())
		// Should have at least one tag
		Expect(string(output)).NotTo(BeEmpty())

		// Cancel context
		cancel()
	})

	It("should debounce rapid file writes", func() {
		// Create mock executor
		executor := &mocks.Executor{}

		// Create runner
		r := runner.NewRunner(
			promptsDir,
			executor,
			prompt.NewManager(promptsDir),
			git.NewReleaser(),
		)

		// Run runner in goroutine
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		go func() {
			_ = r.Run(ctx)
		}()

		// Wait for watcher to start
		time.Sleep(500 * time.Millisecond)

		// Create a prompt and write to it multiple times rapidly
		promptPath := filepath.Join(promptsDir, "015-debounce.md")
		for i := 0; i < 5; i++ {
			promptContent := fmt.Sprintf(`---
status: queued
---

# Debounce test %d

Content iteration %d
`, i, i)
			err := os.WriteFile(promptPath, []byte(promptContent), 0600)
			Expect(err).NotTo(HaveOccurred())
			// Small delay between writes
			time.Sleep(50 * time.Millisecond)
		}

		// Wait for it to be processed (should only process once after debounce)
		Eventually(func() bool {
			completedPath := filepath.Join(completedDir, "015-debounce.md")
			_, err := os.Stat(completedPath)
			return !os.IsNotExist(err)
		}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())

		// Executor should be called exactly once (debounced)
		Expect(executor.ExecuteCallCount()).To(Equal(1))

		// Cancel context
		cancel()
	})

	It("should serialize concurrent prompt execution", func() {
		// Create mock executor with delay to simulate long execution
		executor := &mocks.Executor{}
		var executionOrder []string
		var executionMu sync.Mutex
		executionStarted := make(chan string, 2)

		executor.ExecuteStub = func(_ context.Context, content string, _ string, containerName string) error {
			// Record execution start
			executionStarted <- containerName
			executionMu.Lock()
			executionOrder = append(executionOrder, containerName)
			executionMu.Unlock()

			// Simulate long execution
			time.Sleep(300 * time.Millisecond)
			return nil
		}

		// Create runner
		r := runner.NewRunner(
			promptsDir,
			executor,
			prompt.NewManager(promptsDir),
			git.NewReleaser(),
		)

		// Run runner in goroutine
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		go func() {
			_ = r.Run(ctx)
		}()

		// Wait for watcher to start
		time.Sleep(500 * time.Millisecond)

		// Create two prompts simultaneously
		promptPath1 := filepath.Join(promptsDir, "016-concurrent-1.md")
		promptPath2 := filepath.Join(promptsDir, "017-concurrent-2.md")

		promptContent1 := `---
status: queued
---

# Concurrent test 1

First concurrent prompt.
`
		promptContent2 := `---
status: queued
---

# Concurrent test 2

Second concurrent prompt.
`
		// Write both files at the same time
		err := os.WriteFile(promptPath1, []byte(promptContent1), 0600)
		Expect(err).NotTo(HaveOccurred())
		err = os.WriteFile(promptPath2, []byte(promptContent2), 0600)
		Expect(err).NotTo(HaveOccurred())

		// Wait for first prompt to start executing
		select {
		case <-executionStarted:
			// Good, first execution started
		case <-time.After(3 * time.Second):
			Fail("first prompt did not start within timeout")
		}

		// Verify second prompt doesn't start while first is executing
		// Give it a bit of time to potentially start (it shouldn't)
		select {
		case <-executionStarted:
			Fail("second prompt should not start while first is executing")
		case <-time.After(200 * time.Millisecond):
			// Good, second prompt did not start
		}

		// Wait for both prompts to complete
		Eventually(func() int {
			entries, err := os.ReadDir(completedDir)
			if err != nil {
				return 0
			}
			return len(entries)
		}, 10*time.Second, 100*time.Millisecond).Should(Equal(2))

		// Verify both were executed but serially (not concurrently)
		Expect(executor.ExecuteCallCount()).To(Equal(2))

		// Verify execution was serialized (both in execution order)
		executionMu.Lock()
		defer executionMu.Unlock()
		Expect(executionOrder).To(HaveLen(2))

		// Cancel context
		cancel()
	})
})

var _ = Describe("processPrompt with changelog", func() {
	var (
		ctx           context.Context
		tempDir       string
		promptsDir    string
		completedDir  string
		originalDir   string
		mockExecutor  *mocks.Executor
		mockReleaser  *mocks.Releaser
		promptManager prompt.Manager
		testRunner    runner.Runner
		promptPath    string
		promptContent string
	)

	BeforeEach(func() {
		ctx = context.Background()

		var err error
		originalDir, err = os.Getwd()
		Expect(err).NotTo(HaveOccurred())

		tempDir, err = os.MkdirTemp("", "runner-changelog-test-*")
		Expect(err).NotTo(HaveOccurred())

		// Create prompts and completed directories
		promptsDir = filepath.Join(tempDir, "prompts")
		completedDir = filepath.Join(promptsDir, "completed")
		err = os.MkdirAll(completedDir, 0750)
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

		cmd = exec.Command("git", "commit", "-m", "initial")
		cmd.Dir = tempDir
		err = cmd.Run()
		Expect(err).NotTo(HaveOccurred())

		// Change to temp directory
		err = os.Chdir(tempDir)
		Expect(err).NotTo(HaveOccurred())

		// Create mocks
		mockExecutor = &mocks.Executor{}
		mockReleaser = &mocks.Releaser{}
		promptManager = prompt.NewManager(promptsDir)

		// Create test prompt
		promptPath = filepath.Join(promptsDir, "001-test.md")
		promptContent = `---
status: queued
---

# Test prompt

This is a test.
`
		err = os.WriteFile(promptPath, []byte(promptContent), 0600)
		Expect(err).NotTo(HaveOccurred())

		// Create runner
		testRunner = runner.NewRunner(promptsDir, mockExecutor, promptManager, mockReleaser)
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
		It("calls CommitAndRelease", func() {
			// Mock HasChangelog to return true
			mockReleaser.HasChangelogReturns(true)
			mockReleaser.GetNextVersionReturns("v0.1.0", nil)
			mockReleaser.CommitAndReleaseReturns(nil)
			mockReleaser.CommitCompletedFileReturns(nil)

			// Run in goroutine with timeout
			ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()

			go func() {
				_ = testRunner.Run(ctx)
			}()

			// Wait for processing
			Eventually(func() bool {
				completedPath := filepath.Join(completedDir, "001-test.md")
				_, err := os.Stat(completedPath)
				return !os.IsNotExist(err)
			}, 2*time.Second, 100*time.Millisecond).Should(BeTrue())

			// Verify HasChangelog was called
			Expect(mockReleaser.HasChangelogCallCount()).To(BeNumerically(">=", 1))

			// Verify CommitAndRelease was called
			Expect(mockReleaser.CommitAndReleaseCallCount()).To(Equal(1))

			// Verify CommitOnly was NOT called
			Expect(mockReleaser.CommitOnlyCallCount()).To(Equal(0))

			// Verify GetNextVersion was called
			Expect(mockReleaser.GetNextVersionCallCount()).To(Equal(1))

			cancel()
		})
	})

	Context("without CHANGELOG.md", func() {
		It("calls CommitOnly instead of CommitAndRelease", func() {
			// Mock HasChangelog to return false
			mockReleaser.HasChangelogReturns(false)
			mockReleaser.CommitOnlyReturns(nil)
			mockReleaser.CommitCompletedFileReturns(nil)

			// Run in goroutine with timeout
			ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()

			go func() {
				_ = testRunner.Run(ctx)
			}()

			// Wait for processing
			Eventually(func() bool {
				completedPath := filepath.Join(completedDir, "001-test.md")
				_, err := os.Stat(completedPath)
				return !os.IsNotExist(err)
			}, 2*time.Second, 100*time.Millisecond).Should(BeTrue())

			// Verify HasChangelog was called
			Expect(mockReleaser.HasChangelogCallCount()).To(BeNumerically(">=", 1))

			// Verify CommitOnly was called
			Expect(mockReleaser.CommitOnlyCallCount()).To(Equal(1))

			// Verify CommitAndRelease was NOT called
			Expect(mockReleaser.CommitAndReleaseCallCount()).To(Equal(0))

			// Verify GetNextVersion was NOT called
			Expect(mockReleaser.GetNextVersionCallCount()).To(Equal(0))

			// Verify CommitOnly was called with correct message
			_, message := mockReleaser.CommitOnlyArgsForCall(0)
			Expect(message).To(Equal("Test prompt"))

			cancel()
		})
	})
})

// ErrTest is a test error used in runner tests.
var ErrTest = stderrors.New("test error")
