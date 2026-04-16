// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package report_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/report"
)

var _ = Describe("ScanForCriticalFailures", func() {
	var (
		tempDir string
		logFile string
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "failure-test-*")
		Expect(err).NotTo(HaveOccurred())
		logFile = filepath.Join(tempDir, "test.log")
	})

	AfterEach(func() {
		_ = os.RemoveAll(tempDir)
	})

	Context("no failure", func() {
		It("returns empty string when log file does not exist", func() {
			reason, err := report.ScanForCriticalFailures(
				context.Background(),
				filepath.Join(tempDir, "nonexistent.log"),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(reason).To(BeEmpty())
		})

		It("returns empty string for empty log file", func() {
			Expect(os.WriteFile(logFile, []byte(""), 0600)).To(Succeed())
			reason, err := report.ScanForCriticalFailures(context.Background(), logFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(reason).To(BeEmpty())
		})

		It("returns empty string when log contains only normal output", func() {
			content := "Starting headless session...\n[18:31:29] --- DONE ---\n"
			Expect(os.WriteFile(logFile, []byte(content), 0600)).To(Succeed())
			reason, err := report.ScanForCriticalFailures(context.Background(), logFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(reason).To(BeEmpty())
		})

		It("returns empty string when unrelated text contains the word authenticate", func() {
			content := "Starting headless session...\nrewriting authenticate handler\n"
			Expect(os.WriteFile(logFile, []byte(content), 0600)).To(Succeed())
			reason, err := report.ScanForCriticalFailures(context.Background(), logFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(reason).To(BeEmpty())
		})
	})

	Context("authentication failure", func() {
		It("detects Failed to authenticate with API Error 401", func() {
			content := `Starting headless session...
[18:31:29] Failed to authenticate. API Error: 401 {"type":"error","error":{"type":"authentication_error","message":"Invalid authentication credentials"},"request_id":"req_123"}

[18:31:29] --- DONE ---
Failed to authenticate. API Error: 401 {"type":"error","error":{"type":"authentication_error","message":"Invalid authentication credentials"},"request_id":"req_123"}
`
			Expect(os.WriteFile(logFile, []byte(content), 0600)).To(Succeed())
			reason, err := report.ScanForCriticalFailures(context.Background(), logFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(reason).NotTo(BeEmpty())
			Expect(reason).To(Equal("failed to authenticate"))
		})

		It("detects authentication_error JSON type", func() {
			content := `{"type":"error","error":{"type":"authentication_error","message":"Invalid credentials"}}`
			Expect(os.WriteFile(logFile, []byte(content), 0600)).To(Succeed())
			reason, err := report.ScanForCriticalFailures(context.Background(), logFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(reason).NotTo(BeEmpty())
		})

		It("detects FAILED TO AUTHENTICATE in uppercase (case-insensitive)", func() {
			content := "FAILED TO AUTHENTICATE. API Error: 401\n"
			Expect(os.WriteFile(logFile, []byte(content), 0600)).To(Succeed())
			reason, err := report.ScanForCriticalFailures(context.Background(), logFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(reason).NotTo(BeEmpty())
		})
	})

	Context("API error status codes", func() {
		It("detects API Error: 500", func() {
			content := "API Error: 500 Internal Server Error\n"
			Expect(os.WriteFile(logFile, []byte(content), 0600)).To(Succeed())
			reason, err := report.ScanForCriticalFailures(context.Background(), logFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(reason).To(Equal("api error: 500"))
		})

		It("detects API Error: 429", func() {
			content := "API Error: 429 Too Many Requests\n"
			Expect(os.WriteFile(logFile, []byte(content), 0600)).To(Succeed())
			reason, err := report.ScanForCriticalFailures(context.Background(), logFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(reason).To(Equal("api error: 429"))
		})

		It("detects API Error: 403", func() {
			content := "API Error: 403 Forbidden\n"
			Expect(os.WriteFile(logFile, []byte(content), 0600)).To(Succeed())
			reason, err := report.ScanForCriticalFailures(context.Background(), logFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(reason).To(Equal("api error: 403"))
		})

		It("detects API Error: 502", func() {
			content := "API Error: 502 Bad Gateway\n"
			Expect(os.WriteFile(logFile, []byte(content), 0600)).To(Succeed())
			reason, err := report.ScanForCriticalFailures(context.Background(), logFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(reason).To(Equal("api error: 502"))
		})

		It("detects API Error: 503", func() {
			content := "API Error: 503 Service Unavailable\n"
			Expect(os.WriteFile(logFile, []byte(content), 0600)).To(Succeed())
			reason, err := report.ScanForCriticalFailures(context.Background(), logFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(reason).To(Equal("api error: 503"))
		})

		It("detects API Error: 504", func() {
			content := "API Error: 504 Gateway Timeout\n"
			Expect(os.WriteFile(logFile, []byte(content), 0600)).To(Succeed())
			reason, err := report.ScanForCriticalFailures(context.Background(), logFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(reason).To(Equal("api error: 504"))
		})
	})

	Context("64 KiB boundary", func() {
		It("detects failure pattern in the first 64 KiB of a large log", func() {
			// Put auth error near the start, then pad to >64 KiB
			prefix := "Starting headless session...\nFailed to authenticate. API Error: 401\n"
			padding := strings.Repeat("x", 64*1024)
			content := prefix + padding
			Expect(os.WriteFile(logFile, []byte(content), 0600)).To(Succeed())
			reason, err := report.ScanForCriticalFailures(context.Background(), logFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(reason).NotTo(BeEmpty())
		})

		It("does not detect failure pattern located past the first 64 KiB", func() {
			// Fill first 64 KiB with clean content, then put auth error past the cap
			padding := strings.Repeat("x\n", 32*1024) // exactly 64 KiB
			suffix := "\nFailed to authenticate. API Error: 401\n"
			content := padding + suffix
			// Verify our content is larger than 64 KiB
			Expect(len(content)).To(BeNumerically(">", 64*1024))
			Expect(os.WriteFile(logFile, []byte(content), 0600)).To(Succeed())
			reason, err := report.ScanForCriticalFailures(context.Background(), logFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(reason).To(BeEmpty())
		})
	})
})
