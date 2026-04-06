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

var _ = Describe("RequeueCommand", func() {
	var (
		tempDir    string
		queueDir   string
		requeueCmd cmd.RequeueCommand
		ctx        context.Context
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "requeue-test-*")
		Expect(err).NotTo(HaveOccurred())

		queueDir = filepath.Join(tempDir, "queue")
		err = os.MkdirAll(queueDir, 0750)
		Expect(err).NotTo(HaveOccurred())

		requeueCmd = cmd.NewRequeueCommand(queueDir, libtime.NewCurrentDateTime())
		ctx = context.Background()
	})

	AfterEach(func() {
		_ = os.RemoveAll(tempDir)
	})

	Describe("Requeue specific file", func() {
		It("resets a failed file to queued status", func() {
			testFile := filepath.Join(queueDir, "080-failed.md")
			err := os.WriteFile(testFile, []byte("---\nstatus: failed\n---\n# Failed prompt"), 0600)
			Expect(err).NotTo(HaveOccurred())

			err = requeueCmd.Run(ctx, []string{"080-failed.md"})
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(testFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("status: approved"))
		})

		It("resets an executing file to queued status", func() {
			testFile := filepath.Join(queueDir, "081-executing.md")
			err := os.WriteFile(
				testFile,
				[]byte("---\nstatus: executing\n---\n# Executing prompt"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			err = requeueCmd.Run(ctx, []string{"081-executing.md"})
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(testFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("status: approved"))
		})

		It("returns error for nonexistent file", func() {
			err := requeueCmd.Run(ctx, []string{"nonexistent.md"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("file not found"))
		})
	})

	Describe("Requeue --failed", func() {
		It("requeues all failed prompts", func() {
			err := os.WriteFile(
				filepath.Join(queueDir, "080-failed.md"),
				[]byte("---\nstatus: failed\n---\n# Failed 1"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(
				filepath.Join(queueDir, "081-failed.md"),
				[]byte("---\nstatus: failed\n---\n# Failed 2"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(
				filepath.Join(queueDir, "082-queued.md"),
				[]byte("---\nstatus: approved\n---\n# Queued"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			err = requeueCmd.Run(ctx, []string{"--failed"})
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(filepath.Join(queueDir, "080-failed.md"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("status: approved"))

			content, err = os.ReadFile(filepath.Join(queueDir, "081-failed.md"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("status: approved"))

			// Non-failed file should remain queued (unchanged)
			content, err = os.ReadFile(filepath.Join(queueDir, "082-queued.md"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("status: approved"))
		})

		It("handles no failed prompts gracefully", func() {
			err := os.WriteFile(
				filepath.Join(queueDir, "080-queued.md"),
				[]byte("---\nstatus: approved\n---\n# Queued"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			err = requeueCmd.Run(ctx, []string{"--failed"})
			Expect(err).NotTo(HaveOccurred())
		})

		It("handles empty queue directory", func() {
			err := requeueCmd.Run(ctx, []string{"--failed"})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("No arguments", func() {
		It("returns usage error", func() {
			err := requeueCmd.Run(ctx, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("usage"))
		})
	})

	Describe("Retry (requeue --failed)", func() {
		It("requeues failed prompts same as --failed", func() {
			err := os.WriteFile(
				filepath.Join(queueDir, "080-failed.md"),
				[]byte("---\nstatus: failed\n---\n# Failed"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			// retry is just requeue with --failed
			err = requeueCmd.Run(ctx, []string{"--failed"})
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(filepath.Join(queueDir, "080-failed.md"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("status: approved"))
		})
	})

	Describe("retryCount reset on re-queue", func() {
		It("requeueFile resets retryCount to 0", func() {
			testFile := filepath.Join(queueDir, "080-retry.md")
			err := os.WriteFile(
				testFile,
				[]byte("---\nstatus: failed\nretryCount: 3\n---\n# Retry prompt"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			err = requeueCmd.Run(ctx, []string{"080-retry.md"})
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(testFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("status: approved"))
			Expect(string(content)).NotTo(ContainSubstring("retryCount: 3"))
		})

	})
})
