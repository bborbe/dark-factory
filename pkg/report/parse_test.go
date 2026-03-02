// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package report_test

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/report"
)

var _ = Describe("ParseFromLog", func() {
	var (
		tempDir string
		logFile string
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "parse-test-*")
		Expect(err).NotTo(HaveOccurred())
		logFile = filepath.Join(tempDir, "test.log")
	})

	AfterEach(func() {
		_ = os.RemoveAll(tempDir)
	})

	Context("success cases", func() {
		It("parses valid success report at end of log", func() {
			content := `dark-factory: executing prompt
some output
more output

<!-- DARK-FACTORY-REPORT
{"status":"success","summary":"Replaced splitFrontmatter with adrg/frontmatter library","blockers":[]}
DARK-FACTORY-REPORT -->
`
			Expect(os.WriteFile(logFile, []byte(content), 0600)).To(Succeed())

			result, err := report.ParseFromLog(logFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Status).To(Equal("success"))
			Expect(
				result.Summary,
			).To(Equal("Replaced splitFrontmatter with adrg/frontmatter library"))
			Expect(result.Blockers).To(BeEmpty())
		})

		It("parses partial status with blockers", func() {
			content := `dark-factory: executing prompt

<!-- DARK-FACTORY-REPORT
{"status":"partial","summary":"half done","blockers":["make precommit fails"]}
DARK-FACTORY-REPORT -->
`
			Expect(os.WriteFile(logFile, []byte(content), 0600)).To(Succeed())

			result, err := report.ParseFromLog(logFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Status).To(Equal("partial"))
			Expect(result.Summary).To(Equal("half done"))
			Expect(result.Blockers).To(ConsistOf("make precommit fails"))
		})

		It("parses failed status with multiple blockers", func() {
			content := `dark-factory: executing prompt

<!-- DARK-FACTORY-REPORT
{"status":"failed","summary":"could not complete","blockers":["tests failing","lint errors"]}
DARK-FACTORY-REPORT -->
`
			Expect(os.WriteFile(logFile, []byte(content), 0600)).To(Succeed())

			result, err := report.ParseFromLog(logFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Status).To(Equal("failed"))
			Expect(result.Summary).To(Equal("could not complete"))
			Expect(result.Blockers).To(ConsistOf("tests failing", "lint errors"))
		})

		It("finds report when followed by additional text", func() {
			content := `dark-factory: executing prompt

<!-- DARK-FACTORY-REPORT
{"status":"success","summary":"completed task","blockers":[]}
DARK-FACTORY-REPORT -->

Type /exit to close container
`
			Expect(os.WriteFile(logFile, []byte(content), 0600)).To(Succeed())

			result, err := report.ParseFromLog(logFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Status).To(Equal("success"))
			Expect(result.Summary).To(Equal("completed task"))
		})

		It("finds report in large log file (100KB)", func() {
			// Create 100KB of output before the report
			largeOutput := strings.Repeat("log line output\n", 6000) // ~100KB

			content := largeOutput + `

<!-- DARK-FACTORY-REPORT
{"status":"success","summary":"completed with large output","blockers":[]}
DARK-FACTORY-REPORT -->
`
			Expect(os.WriteFile(logFile, []byte(content), 0600)).To(Succeed())

			result, err := report.ParseFromLog(logFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Status).To(Equal("success"))
			Expect(result.Summary).To(Equal("completed with large output"))
		})
	})

	Context("no report found", func() {
		It("returns nil when log has no markers", func() {
			content := `dark-factory: executing prompt
some output
more output
`
			Expect(os.WriteFile(logFile, []byte(content), 0600)).To(Succeed())

			result, err := report.ParseFromLog(logFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
		})

		It("returns nil for empty log file", func() {
			Expect(os.WriteFile(logFile, []byte(""), 0600)).To(Succeed())

			result, err := report.ParseFromLog(logFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
		})
	})

	Context("error cases", func() {
		It("returns error when JSON is malformed", func() {
			content := `dark-factory: executing prompt

<!-- DARK-FACTORY-REPORT
{this is not valid json}
DARK-FACTORY-REPORT -->
`
			Expect(os.WriteFile(logFile, []byte(content), 0600)).To(Succeed())

			result, err := report.ParseFromLog(logFile)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unmarshal"))
			Expect(result).To(BeNil())
		})

		It("returns error when end marker is missing", func() {
			content := `dark-factory: executing prompt

<!-- DARK-FACTORY-REPORT
{"status":"success","summary":"test","blockers":[]}
`
			Expect(os.WriteFile(logFile, []byte(content), 0600)).To(Succeed())

			result, err := report.ParseFromLog(logFile)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no valid end marker"))
			Expect(result).To(BeNil())
		})

		It("returns error when log file does not exist", func() {
			result, err := report.ParseFromLog(filepath.Join(tempDir, "nonexistent.log"))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("open log file"))
			Expect(result).To(BeNil())
		})
	})
})
