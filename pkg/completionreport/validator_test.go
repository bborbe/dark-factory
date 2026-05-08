// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package completionreport_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/completionreport"
	"github.com/bborbe/dark-factory/pkg/report"
)

var _ = Describe("Validator", func() {
	var (
		ctx       context.Context
		tempDir   string
		logFile   string
		validator completionreport.Validator
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		tempDir, err = os.MkdirTemp("", "vcr-test-*")
		Expect(err).NotTo(HaveOccurred())
		logFile = filepath.Join(tempDir, "test.log")
		validator = completionreport.NewValidator()
	})

	AfterEach(func() {
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	writeLog := func(content string) {
		Expect(os.WriteFile(logFile, []byte(content), 0600)).To(Succeed())
	}

	It("returns non-nil report with summary for valid success report", func() {
		writeLog(`Starting session...

<!-- DARK-FACTORY-REPORT
{"status":"success","summary":"Added ScanForCriticalFailures","blockers":[],"verification":{"command":"make precommit","exitCode":0}}
DARK-FACTORY-REPORT -->
`)
		r, err := validator.Validate(ctx, logFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(r).NotTo(BeNil())
		Expect(r.Summary).To(Equal("Added ScanForCriticalFailures"))
		Expect(r.Status).To(Equal("success"))
	})

	It("returns error for partial status report with failing verification exit code", func() {
		writeLog(`Starting session...

<!-- DARK-FACTORY-REPORT
{"status":"success","summary":"half done","blockers":[],"verification":{"command":"make precommit","exitCode":1}}
DARK-FACTORY-REPORT -->
`)
		r, err := validator.Validate(ctx, logFile)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("completion report status: partial"))
		Expect(r).To(BeNil())
	})

	It("returns error for failed status report", func() {
		writeLog(`Starting session...

<!-- DARK-FACTORY-REPORT
{"status":"failed","summary":"could not complete","blockers":["tests failing"]}
DARK-FACTORY-REPORT -->
`)
		r, err := validator.Validate(ctx, logFile)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("completion report status: failed"))
		Expect(r).To(BeNil())
	})

	It("returns (nil, nil) when log has no report and no critical failure", func() {
		writeLog("Starting session...\nsome output\n--- DONE ---\n")
		r, err := validator.Validate(ctx, logFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(r).To(BeNil())
	})

	It("returns (nil, error) when log has no report but contains auth error pattern", func() {
		writeLog(`Starting headless session...
[18:31:29] Failed to authenticate. API Error: 401 {"type":"error","error":{"type":"authentication_error","message":"Invalid authentication credentials"}}

[18:31:29] --- DONE ---
`)
		r, err := validator.Validate(ctx, logFile)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("claude CLI critical failure"))
		Expect(r).To(BeNil())
	})

	It("returns (nil, error) when log has no report but contains API Error: 500", func() {
		writeLog(
			"Starting headless session...\nAPI Error: 500 Internal Server Error\n--- DONE ---\n",
		)
		r, err := validator.Validate(ctx, logFile)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("claude CLI critical failure"))
		Expect(r).To(BeNil())
	})

	It(
		"returns (nil, nil) for malformed JSON completion report (parse error downgraded to no-report)",
		func() {
			writeLog(`Starting session...

<!-- DARK-FACTORY-REPORT
{this is not valid json}
DARK-FACTORY-REPORT -->
`)
			r, err := validator.Validate(ctx, logFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(r).To(BeNil())
		},
	)

	It("returns (nil, error) when log is malformed JSON AND contains auth-error pattern", func() {
		writeLog(`Starting headless session...
Failed to authenticate. API Error: 401

<!-- DARK-FACTORY-REPORT
{this is not valid json}
DARK-FACTORY-REPORT -->
`)
		r, err := validator.Validate(ctx, logFile)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("claude CLI critical failure"))
		Expect(r).To(BeNil())
	})

	It("returns non-nil report using the report package types correctly", func() {
		writeLog(`Starting session...

<!-- DARK-FACTORY-REPORT
{"status":"success","summary":"Fixed auth detection","blockers":[],"verification":{"command":"make precommit","exitCode":0}}
DARK-FACTORY-REPORT -->
`)
		r, err := validator.Validate(ctx, logFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(r).To(BeAssignableToTypeOf(&report.CompletionReport{}))
		Expect(r.Verification).NotTo(BeNil())
		Expect(r.Verification.ExitCode).To(Equal(0))
	})

	It("propagates error when log has start marker but no end marker", func() {
		writeLog(`Starting session...

` + report.MarkerStart + `
{"status":"failed","summary":"truncated","blockers":[]}
`)
		// NOTE: no closing MarkerEnd
		r, err := validator.Validate(ctx, logFile)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("parse report tail boundary"))
		Expect(r).To(BeNil())
	})

	It("propagates failure when production-shaped log has orphaned end marker in tail", func() {
		// Reuse the 9235-byte construction from pkg/report/parse_test.go test 4b.
		// Both report blocks contain "status":"failed".
		makeFailedJSON := func(wantLen int) string {
			base := `{"status":"failed","blockers":[],"summary":"make precommit failed","verification":{"command":"make precommit","exitCode":1}}`
			padLen := wantLen - len(base) - 8
			Expect(padLen).To(BeNumerically(">=", 0))
			return base[:len(base)-1] + `,"_p":"` + strings.Repeat("x", padLen) + `"}`
		}

		const (
			wantTotal     = 9235
			tailBytesSize = 4096
			block1Start   = 5122
			block1End     = 5693
			block2Start   = 7371
		)

		jsonLen := block1End - block1Start - len(report.MarkerStart) - 2
		json1 := makeFailedJSON(jsonLen)
		json2 := makeFailedJSON(jsonLen)

		block1 := report.MarkerStart + "\n" + json1 + "\n" + report.MarkerEnd + "\n"
		block2 := report.MarkerStart + "\n" + json2 + "\n" + report.MarkerEnd + "\n"

		filler := strings.Repeat("f", block2Start-(block1Start+len(block1)))
		trailing := strings.Repeat("t", wantTotal-block2Start-len(block2))
		preamble := strings.Repeat("a", block1Start)

		content := preamble + block1 + filler + block2 + trailing
		Expect(len(content)).To(Equal(wantTotal))

		writeLog(content)

		r, err := validator.Validate(ctx, logFile)
		Expect(err).To(HaveOccurred())
		Expect(r).To(BeNil())
		// The validator returns a non-nil error reflecting the parsed status:failed,
		// NOT a downgraded "no report" path.
	})
})
