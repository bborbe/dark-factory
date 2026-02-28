// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/factory"
)

// MockExecutor is a fake executor for testing.
type MockExecutor struct {
	ExecuteFunc func(ctx context.Context, promptContent string) error
}

func (m *MockExecutor) Execute(ctx context.Context, promptContent string) error {
	if m.ExecuteFunc != nil {
		return m.ExecuteFunc(ctx, promptContent)
	}
	return nil
}

var _ = Describe("Factory Integration", func() {
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
		tempDir, err = os.MkdirTemp("", "factory-test-*")
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
		mockExec := &MockExecutor{
			ExecuteFunc: func(ctx context.Context, content string) error {
				Expect(content).To(ContainSubstring("Add test feature"))
				return nil
			},
		}

		// Create factory
		f := factory.New(mockExec)

		// Run factory in goroutine with timeout context
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		errCh := make(chan error, 1)
		go func() {
			errCh <- f.Run(ctx)
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

		// Cancel context to stop factory
		cancel()

		// Wait for factory to exit
		select {
		case err := <-errCh:
			Expect(err).To(BeNil())
		case <-time.After(2 * time.Second):
			Fail("factory did not exit within timeout")
		}
	})

	It("should watch for new queued prompts", func() {
		// Create mock executor
		mockExec := &MockExecutor{
			ExecuteFunc: func(ctx context.Context, content string) error {
				return nil
			},
		}

		// Create factory
		f := factory.New(mockExec)

		// Run factory in goroutine with timeout context
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		go func() {
			_ = f.Run(ctx)
		}()

		// Wait a bit for watcher to start
		time.Sleep(500 * time.Millisecond)

		// Create a new queued prompt after factory started
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

	It("should ignore non-queued prompts", func() {
		// Create mock executor
		mockExec := &MockExecutor{
			ExecuteFunc: func(ctx context.Context, content string) error {
				Fail("executor should not be called for non-queued prompts")
				return nil
			},
		}

		// Create factory
		f := factory.New(mockExec)

		// Run factory in goroutine with timeout context
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		go func() {
			_ = f.Run(ctx)
		}()

		// Wait a bit for watcher to start
		time.Sleep(500 * time.Millisecond)

		// Create a non-queued prompt
		promptPath := filepath.Join(promptsDir, "003-not-queued.md")
		promptContent := `---
status: draft
---

# Draft prompt

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
})
