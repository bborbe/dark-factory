// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd_test

import (
	"context"
	"os"
	"path/filepath"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/cmd"
	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/prompt"
)

var _ = Describe("CancelCommand", func() {
	var (
		tempDir      string
		queueDir     string
		cancelledDir string
		cancelCmd    cmd.CancelCommand
		ctx          context.Context
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "cancel-test-*")
		Expect(err).NotTo(HaveOccurred())

		queueDir = filepath.Join(tempDir, "queue")
		err = os.MkdirAll(queueDir, 0750)
		Expect(err).NotTo(HaveOccurred())

		cancelledDir = filepath.Join(tempDir, "cancelled")
		// Do NOT pre-create cancelledDir — test that MoveToCancelled creates it on demand.

		cancelCmd = cmd.NewCancelCommand(
			queueDir,
			cancelledDir,
			prompt.NewManager(
				"",
				queueDir,
				"",
				cancelledDir,
				git.NewReleaser(),
				libtime.NewCurrentDateTime(),
			),
		)
		ctx = context.Background()
	})

	AfterEach(func() {
		_ = os.RemoveAll(tempDir)
	})

	Describe("Cancel an approved prompt", func() {
		It("moves the file to cancelled dir", func() {
			testFile := filepath.Join(queueDir, "080-approved.md")
			err := os.WriteFile(
				testFile,
				[]byte("---\nstatus: approved\n---\n# Approved prompt"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			err = cancelCmd.Run(ctx, []string{"080-approved.md"})
			Expect(err).NotTo(HaveOccurred())

			// File must be moved out of the queue dir.
			_, statErr := os.Stat(testFile)
			Expect(statErr).To(HaveOccurred())
			Expect(os.IsNotExist(statErr)).To(BeTrue())

			// File must appear in cancelled dir with status: cancelled.
			cancelledFile := filepath.Join(cancelledDir, "080-approved.md")
			content, readErr := os.ReadFile(cancelledFile)
			Expect(readErr).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("status: cancelled"))
			Expect(string(content)).To(ContainSubstring("cancelled:"))
		})
	})

	Describe("Cancel an executing prompt", func() {
		It("marks cancelled but leaves file in queue dir (processor moves it)", func() {
			testFile := filepath.Join(queueDir, "081-executing.md")
			err := os.WriteFile(
				testFile,
				[]byte("---\nstatus: executing\n---\n# Executing prompt"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			err = cancelCmd.Run(ctx, []string{"081-executing.md"})
			Expect(err).NotTo(HaveOccurred())

			// File must still exist in queue dir (processor is responsible for moving it).
			content, readErr := os.ReadFile(testFile)
			Expect(readErr).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("status: cancelled"))
		})
	})

	Describe("Cancel a completed prompt", func() {
		It("returns error with current status", func() {
			testFile := filepath.Join(queueDir, "082-completed.md")
			err := os.WriteFile(
				testFile,
				[]byte("---\nstatus: completed\n---\n# Completed prompt"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			err = cancelCmd.Run(ctx, []string{"082-completed.md"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot cancel prompt with status"))
			Expect(err.Error()).To(ContainSubstring("completed"))
		})
	})

	Describe("Cancel a failed prompt", func() {
		It("returns error", func() {
			testFile := filepath.Join(queueDir, "083-failed.md")
			err := os.WriteFile(testFile, []byte("---\nstatus: failed\n---\n# Failed prompt"), 0600)
			Expect(err).NotTo(HaveOccurred())

			err = cancelCmd.Run(ctx, []string{"083-failed.md"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot cancel prompt with status"))
			Expect(err.Error()).To(ContainSubstring("failed"))
		})
	})

	Describe("Cancel an already cancelled prompt still in queue dir", func() {
		It("returns error (only files moved to cancelled/ are idempotent)", func() {
			testFile := filepath.Join(queueDir, "084-cancelled.md")
			err := os.WriteFile(
				testFile,
				[]byte("---\nstatus: cancelled\n---\n# Cancelled prompt"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			err = cancelCmd.Run(ctx, []string{"084-cancelled.md"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot cancel prompt with status"))
			Expect(err.Error()).To(ContainSubstring("cancelled"))
		})
	})

	Describe("Cancel an already-cancelled prompt (already in cancelled dir)", func() {
		It("is idempotent: returns nil", func() {
			err := os.MkdirAll(cancelledDir, 0750)
			Expect(err).NotTo(HaveOccurred())

			testFile := filepath.Join(cancelledDir, "084-cancelled.md")
			err = os.WriteFile(
				testFile,
				[]byte("---\nstatus: cancelled\n---\n# Cancelled prompt"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			err = cancelCmd.Run(ctx, []string{"084-cancelled.md"})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Cancel with no args", func() {
		It("returns usage error", func() {
			err := cancelCmd.Run(ctx, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("usage: dark-factory prompt cancel <id>"))
		})
	})

	Describe("Cancel with too many args", func() {
		It("returns usage error", func() {
			err := cancelCmd.Run(ctx, []string{"one", "two"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("usage: dark-factory prompt cancel <id>"))
		})
	})

	Describe("Cancel with unknown ID", func() {
		It("returns prompt not found error", func() {
			err := cancelCmd.Run(ctx, []string{"999-nonexistent.md"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("prompt not found"))
		})
	})
})
