// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package report_test

import (
	"context"
	stderrors "errors"
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

			result, err := report.ParseFromLog(context.Background(), logFile)
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

			result, err := report.ParseFromLog(context.Background(), logFile)
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

			result, err := report.ParseFromLog(context.Background(), logFile)
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

			result, err := report.ParseFromLog(context.Background(), logFile)
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

			result, err := report.ParseFromLog(context.Background(), logFile)
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

			result, err := report.ParseFromLog(context.Background(), logFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
		})

		It("returns nil for empty log file", func() {
			Expect(os.WriteFile(logFile, []byte(""), 0600)).To(Succeed())

			result, err := report.ParseFromLog(context.Background(), logFile)
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

			result, err := report.ParseFromLog(context.Background(), logFile)
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

			result, err := report.ParseFromLog(context.Background(), logFile)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no valid end marker"))
			Expect(result).To(BeNil())
		})

		It("returns error when log file does not exist", func() {
			result, err := report.ParseFromLog(
				context.Background(),
				filepath.Join(tempDir, "nonexistent.log"),
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("open log file"))
			Expect(result).To(BeNil())
		})
	})

	Context("last-block-wins and tail-boundary cases", func() {
		It("returns last block when log contains two complete report blocks", func() {
			content := `output line 1
output line 2

` + report.MarkerStart + `
{"status":"success","summary":"first attempt","blockers":[]}
` + report.MarkerEnd + `

More output after first report.

` + report.MarkerStart + `
{"status":"failed","summary":"second attempt","blockers":["compile error"]}
` + report.MarkerEnd + `
`
			Expect(os.WriteFile(logFile, []byte(content), 0600)).To(Succeed())

			result, err := report.ParseFromLog(context.Background(), logFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Status).To(Equal("failed"))
			Expect(result.Summary).To(Equal("second attempt"))
		})

		It(
			"production reproduction: 9235-byte log with orphaned end marker in tail window",
			func() {
				// Production failure layout (dark-factory v0.154.0, prompt 005-update-build-golang-1.26.3.md):
				// Total file size: 9235 bytes; tail window starts at byte 5139 (9235 - 4096)
				// Block1 MarkerStart at byte 5122 — 17 bytes BEFORE the tail window (outside tail)
				// Block1 MarkerEnd   at byte 5693 — inside the tail (orphaned end marker)
				// Block2 MarkerStart at byte 7371 — inside the tail
				// Block2 MarkerEnd   at byte 7942 — inside the tail
				// Old parser: strings.Index found orphaned end (offset 554) before real start (offset 2232) → error
				// New parser: strings.LastIndex finds real start, then searches forward for end → correct
				makeFailedJSON := func(wantLen int) string {
					base := `{"status":"failed","blockers":[],"summary":"make precommit failed","verification":{"command":"make precommit","exitCode":1}}`
					padLen := wantLen - len(base) - 8 // 8 = len(`,"_p":""}`)
					Expect(padLen).To(BeNumerically(">=", 0),
						"base JSON is already %d bytes, cannot pad to %d", len(base), wantLen)
					return base[:len(base)-1] + `,"_p":"` + strings.Repeat("x", padLen) + `"}`
				}

				const (
					wantTotal     = 9235
					tailBytesSize = 4096
					block1Start   = 5122
					block1End     = 5693
					block2Start   = 7371
				)
				tailWindowStart := wantTotal - tailBytesSize // = 5139
				Expect(block1Start).To(BeNumerically("<", tailWindowStart),
					"block1 start must be before the tail window")
				Expect(block1End).To(BeNumerically(">=", tailWindowStart),
					"block1 end must be inside the tail window (orphaned)")

				// json length = block1End - block1Start - len(MarkerStart) - 2 newlines
				jsonLen := block1End - block1Start - len(report.MarkerStart) - 2
				Expect(jsonLen).To(Equal(545))

				json1 := makeFailedJSON(jsonLen)
				Expect(len(json1)).To(Equal(jsonLen))
				json2 := makeFailedJSON(jsonLen)

				block1 := report.MarkerStart + "\n" + json1 + "\n" + report.MarkerEnd + "\n"
				block2 := report.MarkerStart + "\n" + json2 + "\n" + report.MarkerEnd + "\n"
				Expect(len(block1)).To(Equal(595))

				filler := strings.Repeat("f", block2Start-(block1Start+len(block1)))
				trailing := strings.Repeat("t", wantTotal-block2Start-len(block2))
				preamble := strings.Repeat("a", block1Start)

				content := preamble + block1 + filler + block2 + trailing
				Expect(len(content)).To(Equal(wantTotal))

				Expect(os.WriteFile(logFile, []byte(content), 0600)).To(Succeed())

				result, err := report.ParseFromLog(context.Background(), logFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Status).To(Equal("failed"))
			},
		)

		It("ignores orphaned end marker when a complete pair follows it in the tail", func() {
			content := report.MarkerEnd + `

Some output between markers.

` + report.MarkerStart + `
{"status":"success","summary":"recovered","blockers":[]}
` + report.MarkerEnd + `
`
			Expect(os.WriteFile(logFile, []byte(content), 0600)).To(Succeed())

			result, err := report.ParseFromLog(context.Background(), logFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Status).To(Equal("success"))
			Expect(result.Summary).To(Equal("recovered"))
		})

		It(
			"returns ErrStartWithoutEnd when start marker is present but end marker is missing",
			func() {
				content := `starting session...

` + report.MarkerStart + `
{"status":"failed","summary":"truncated","blockers":[]}
`
				// NOTE: no closing MarkerEnd
				Expect(os.WriteFile(logFile, []byte(content), 0600)).To(Succeed())

				result, err := report.ParseFromLog(context.Background(), logFile)
				Expect(err).To(HaveOccurred())
				Expect(stderrors.Is(err, report.ErrStartWithoutEnd)).To(BeTrue())
				Expect(result).To(BeNil())
			},
		)

		It("returns (nil, nil) when only an end marker is present (no start marker)", func() {
			content := `some output
` + report.MarkerEnd + `
`
			Expect(os.WriteFile(logFile, []byte(content), 0600)).To(Succeed())

			result, err := report.ParseFromLog(context.Background(), logFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
		})
	})
})
