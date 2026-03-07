// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/cmd"
)

var _ = Describe("ListCommand", func() {
	var (
		tempDir      string
		inboxDir     string
		queueDir     string
		completedDir string
		listCmd      cmd.ListCommand
		ctx          context.Context
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "list-test-*")
		Expect(err).NotTo(HaveOccurred())

		inboxDir = filepath.Join(tempDir, "inbox")
		queueDir = filepath.Join(tempDir, "queue")
		completedDir = filepath.Join(tempDir, "completed")

		err = os.MkdirAll(inboxDir, 0750)
		Expect(err).NotTo(HaveOccurred())
		err = os.MkdirAll(queueDir, 0750)
		Expect(err).NotTo(HaveOccurred())
		err = os.MkdirAll(completedDir, 0750)
		Expect(err).NotTo(HaveOccurred())

		listCmd = cmd.NewListCommand(inboxDir, queueDir, completedDir)
		ctx = context.Background()
	})

	AfterEach(func() {
		_ = os.RemoveAll(tempDir)
	})

	Describe("List all prompts", func() {
		It("hides completed prompts by default", func() {
			err := os.WriteFile(filepath.Join(inboxDir, "fix-something.md"), []byte("# Fix"), 0600)
			Expect(err).NotTo(HaveOccurred())

			err = os.WriteFile(
				filepath.Join(queueDir, "078-queued.md"),
				[]byte("---\nstatus: approved\n---\n# Queued"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			err = os.WriteFile(
				filepath.Join(completedDir, "077-done.md"),
				[]byte("---\nstatus: completed\n---\n# Done"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			err = listCmd.Run(ctx, []string{})
			Expect(err).NotTo(HaveOccurred())
		})

		It("shows completed prompts with --all flag", func() {
			err := os.WriteFile(
				filepath.Join(completedDir, "077-done.md"),
				[]byte("---\nstatus: completed\n---\n# Done"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			err = listCmd.Run(ctx, []string{"--all"})
			Expect(err).NotTo(HaveOccurred())
		})

		It("handles empty directories", func() {
			err := listCmd.Run(ctx, []string{})
			Expect(err).NotTo(HaveOccurred())
		})

		It("skips non-.md files", func() {
			err := os.WriteFile(filepath.Join(queueDir, "not-md.txt"), []byte("text"), 0600)
			Expect(err).NotTo(HaveOccurred())

			err = listCmd.Run(ctx, []string{})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("List with --queue flag", func() {
		It("shows only queue prompts", func() {
			err := os.WriteFile(filepath.Join(inboxDir, "inbox-file.md"), []byte("# Inbox"), 0600)
			Expect(err).NotTo(HaveOccurred())

			err = os.WriteFile(
				filepath.Join(queueDir, "080-queued.md"),
				[]byte("---\nstatus: approved\n---\n# Queued"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			err = listCmd.Run(ctx, []string{"--queue"})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("List with --failed flag", func() {
		It("shows only failed prompts", func() {
			err := os.WriteFile(
				filepath.Join(queueDir, "080-failed.md"),
				[]byte("---\nstatus: failed\n---\n# Failed"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			err = os.WriteFile(
				filepath.Join(queueDir, "081-queued.md"),
				[]byte("---\nstatus: approved\n---\n# Queued"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			err = listCmd.Run(ctx, []string{"--failed"})
			Expect(err).NotTo(HaveOccurred())
		})

		It("shows nothing when no failed prompts", func() {
			err := os.WriteFile(
				filepath.Join(queueDir, "081-queued.md"),
				[]byte("---\nstatus: approved\n---\n# Queued"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			err = listCmd.Run(ctx, []string{"--failed"})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("List with --json flag", func() {
		It("outputs JSON format", func() {
			err := os.WriteFile(
				filepath.Join(queueDir, "080-queued.md"),
				[]byte("---\nstatus: approved\n---\n# Queued"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			err = listCmd.Run(ctx, []string{"--json"})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Missing directories", func() {
		It("handles nonexistent inbox gracefully", func() {
			listCmd = cmd.NewListCommand("/nonexistent/inbox", queueDir, completedDir)
			err := listCmd.Run(ctx, []string{})
			Expect(err).NotTo(HaveOccurred())
		})

		It("handles nonexistent completed dir gracefully", func() {
			listCmd = cmd.NewListCommand(inboxDir, queueDir, "/nonexistent/completed")
			err := listCmd.Run(ctx, []string{})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
