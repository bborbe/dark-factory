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
)

var _ = Describe("CancelCommand", func() {
	var (
		tempDir   string
		queueDir  string
		cancelCmd cmd.CancelCommand
		ctx       context.Context
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "cancel-test-*")
		Expect(err).NotTo(HaveOccurred())

		queueDir = filepath.Join(tempDir, "queue")
		err = os.MkdirAll(queueDir, 0750)
		Expect(err).NotTo(HaveOccurred())

		cancelCmd = cmd.NewCancelCommand(queueDir, libtime.NewCurrentDateTime())
		ctx = context.Background()
	})

	AfterEach(func() {
		_ = os.RemoveAll(tempDir)
	})

	Describe("Cancel an approved prompt", func() {
		It("sets status to cancelled", func() {
			testFile := filepath.Join(queueDir, "080-approved.md")
			err := os.WriteFile(
				testFile,
				[]byte("---\nstatus: approved\n---\n# Approved prompt"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			err = cancelCmd.Run(ctx, []string{"080-approved.md"})
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(testFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("status: cancelled"))
		})
	})

	Describe("Cancel an executing prompt", func() {
		It("sets status to cancelled", func() {
			testFile := filepath.Join(queueDir, "081-executing.md")
			err := os.WriteFile(
				testFile,
				[]byte("---\nstatus: executing\n---\n# Executing prompt"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			err = cancelCmd.Run(ctx, []string{"081-executing.md"})
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(testFile)
			Expect(err).NotTo(HaveOccurred())
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

	Describe("Cancel an already cancelled prompt", func() {
		It("returns error", func() {
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
