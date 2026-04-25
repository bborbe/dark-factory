// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package completionreport_test

import (
	"context"
	"os"
	"path/filepath"

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
})
