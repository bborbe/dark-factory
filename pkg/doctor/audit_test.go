// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package doctor_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/doctor"
)

var _ = Describe("WriteAuditEntry", func() {
	var (
		tempDir string
		ctx     context.Context
	)

	BeforeEach(func() {
		tempDir = GinkgoT().TempDir()
		ctx = context.Background()
	})

	It("creates the directory if it does not exist", func() {
		auditPath := filepath.Join(tempDir, "some", "nested", "dir", "audit.log")
		entry := doctor.AuditEntry{
			Timestamp:   time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
			Category:    doctor.CategoryDuplicateSpecNumbers,
			Action:      "applied",
			TargetPaths: []string{"/path/to/spec.md"},
			Before:      "001",
			After:       "002",
		}

		err := doctor.WriteAuditEntry(ctx, auditPath, entry)
		Expect(err).NotTo(HaveOccurred())

		stat, err := os.Stat(auditPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(stat.Mode().Perm()).To(Equal(os.FileMode(0644)))
	})

	It("writes a tab-separated line to the file", func() {
		auditPath := filepath.Join(tempDir, "audit.log")
		entry := doctor.AuditEntry{
			Timestamp:   time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
			Category:    doctor.CategoryOrphanPromptLink,
			Action:      "skipped",
			TargetPaths: []string{"/path/to/prompt.md"},
			Before:      "status=approved",
			After:       "",
		}

		err := doctor.WriteAuditEntry(ctx, auditPath, entry)
		Expect(err).NotTo(HaveOccurred())

		content, err := os.ReadFile(auditPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(
			string(content),
		).To(Equal("2026-06-01T12:00:00Z\torphan-prompt-link\tskipped\t/path/to/prompt.md\tstatus=approved\t\n"))
	})

	It("appends to existing file", func() {
		auditPath := filepath.Join(tempDir, "audit.log")
		existing := "2026-06-01T10:00:00Z\tsome-category\tsome-action\tsome-path\tbefore\tafter\n"
		err := os.WriteFile(auditPath, []byte(existing), 0644)
		Expect(err).NotTo(HaveOccurred())

		entry := doctor.AuditEntry{
			Timestamp:   time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
			Category:    doctor.CategoryStatusDirMismatch,
			Action:      "applied",
			TargetPaths: []string{"/path/to/spec.md", "/path/to/other.md"},
			Before:      "in-progress",
			After:       "completed",
		}

		err = doctor.WriteAuditEntry(ctx, auditPath, entry)
		Expect(err).NotTo(HaveOccurred())

		content, err := os.ReadFile(auditPath)
		Expect(err).NotTo(HaveOccurred())
		lines := splitLines(string(content))
		Expect(lines).To(HaveLen(2))
		Expect(
			lines[0],
		).To(Equal("2026-06-01T10:00:00Z\tsome-category\tsome-action\tsome-path\tbefore\tafter"))
		Expect(
			lines[1],
		).To(Equal("2026-06-01T12:00:00Z\tstatus-dir-mismatch\tapplied\t/path/to/spec.md /path/to/other.md\tin-progress\tcompleted"))
	})

	It("handles empty TargetPaths", func() {
		auditPath := filepath.Join(tempDir, "audit.log")
		entry := doctor.AuditEntry{
			Timestamp:   time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
			Category:    doctor.CategoryVerifyingStale,
			Action:      "skipped",
			TargetPaths: []string{},
			Before:      "",
			After:       "",
		}

		err := doctor.WriteAuditEntry(ctx, auditPath, entry)
		Expect(err).NotTo(HaveOccurred())

		content, err := os.ReadFile(auditPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(content)).To(Equal("2026-06-01T12:00:00Z\tverifying-stale\tskipped\t\t\t\n"))
	})

	It("returns error when directory creation fails", func() {
		// Use a path that ends with a file in a non-existent root
		auditPath := "/proc/0/audit.log"
		entry := doctor.AuditEntry{
			Timestamp:   time.Now(),
			Category:    doctor.CategoryDuplicateSpecNumbers,
			Action:      "applied",
			TargetPaths: []string{"/path"},
			Before:      "",
			After:       "",
		}

		err := doctor.WriteAuditEntry(ctx, auditPath, entry)
		Expect(err).To(HaveOccurred())
	})

	It("returns error when file write fails due to permission issue", func() {
		// Create a directory that is not writable
		auditPath := filepath.Join(tempDir, "subdir", "audit.log")
		err := os.MkdirAll(filepath.Dir(auditPath), 0555)
		Expect(err).NotTo(HaveOccurred())

		entry := doctor.AuditEntry{
			Timestamp:   time.Now(),
			Category:    doctor.CategoryDuplicateSpecNumbers,
			Action:      "applied",
			TargetPaths: []string{"/path"},
			Before:      "",
			After:       "",
		}

		err = doctor.WriteAuditEntry(ctx, auditPath, entry)
		Expect(err).To(HaveOccurred())
	})
})

func splitLines(s string) []string {
	var lines []string
	for _, line := range splitN(s, "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func splitN(s, sep string) []string {
	var result []string
	start := 0
	for {
		idx := find(s, sep, start)
		if idx == -1 {
			result = append(result, s[start:])
			break
		}
		result = append(result, s[start:idx])
		start = idx + len(sep)
	}
	return result
}

func find(s, substr string, start int) int {
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
