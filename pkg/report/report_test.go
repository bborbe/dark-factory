// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package report_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/report"
)

var _ = Describe("Suffix", func() {
	It("should return suffix containing DARK-FACTORY-REPORT marker", func() {
		suffix := report.Suffix()
		Expect(suffix).To(ContainSubstring("DARK-FACTORY-REPORT"))
	})

	It("should return suffix containing both opening and closing markers", func() {
		suffix := report.Suffix()
		Expect(suffix).To(ContainSubstring("<!-- DARK-FACTORY-REPORT"))
		Expect(suffix).To(ContainSubstring("DARK-FACTORY-REPORT -->"))
	})

	It("should return suffix containing example JSON", func() {
		suffix := report.Suffix()
		Expect(suffix).To(ContainSubstring(`{"status":"success"`))
		Expect(suffix).To(ContainSubstring(`"summary":`))
		Expect(suffix).To(ContainSubstring(`"blockers":`))
	})

	It("should return suffix containing field descriptions", func() {
		suffix := report.Suffix()
		Expect(suffix).To(ContainSubstring("status:"))
		Expect(suffix).To(ContainSubstring("summary:"))
		Expect(suffix).To(ContainSubstring("blockers:"))
	})
})

var _ = Describe("TestCommandSuffix", func() {
	It("should contain the command string", func() {
		suffix := report.TestCommandSuffix("make test")
		Expect(suffix).To(ContainSubstring("make test"))
	})

	It("should contain the Fast Feedback section header", func() {
		suffix := report.TestCommandSuffix("make test")
		Expect(suffix).To(ContainSubstring("Fast Feedback"))
	})

	It("should contain iteration wording", func() {
		suffix := report.TestCommandSuffix("make test")
		Expect(suffix).To(ContainSubstring("repeatedly"))
	})
})

var _ = Describe("ValidationSuffix", func() {
	It("should contain the command string", func() {
		suffix := report.ValidationSuffix("make precommit")
		Expect(suffix).To(ContainSubstring("make precommit"))
	})

	It("should contain the override instruction", func() {
		suffix := report.ValidationSuffix("make precommit")
		Expect(suffix).To(ContainSubstring("overrides"))
		Expect(suffix).To(ContainSubstring("verification"))
	})

	It("should contain the section header", func() {
		suffix := report.ValidationSuffix("make precommit")
		Expect(suffix).To(ContainSubstring("Project Validation Command"))
	})

	It("should contain once at the end wording", func() {
		suffix := report.ValidationSuffix("make precommit")
		Expect(suffix).To(ContainSubstring("ONCE at the end"))
	})
})

var _ = Describe("ValidationPromptSuffix", func() {
	It("should contain the criteria string", func() {
		suffix := report.ValidationPromptSuffix("readme.md is updated")
		Expect(suffix).To(ContainSubstring("readme.md is updated"))
	})

	It("should contain partial status instruction", func() {
		suffix := report.ValidationPromptSuffix("readme.md is updated")
		Expect(suffix).To(ContainSubstring("partial"))
	})

	It("should contain blockers reference", func() {
		suffix := report.ValidationPromptSuffix("readme.md is updated")
		Expect(suffix).To(ContainSubstring("blockers"))
	})
})

var _ = Describe("ChangelogSuffix", func() {
	It("should contain CHANGELOG.md reference", func() {
		suffix := report.ChangelogSuffix()
		Expect(suffix).To(ContainSubstring("CHANGELOG.md"))
	})

	It("should contain unreleased section instruction", func() {
		suffix := report.ChangelogSuffix()
		Expect(suffix).To(ContainSubstring("## Unreleased"))
	})

	It("should reference changelog guide", func() {
		suffix := report.ChangelogSuffix()
		Expect(suffix).To(ContainSubstring("changelog-guide.md"))
	})
})

var _ = Describe("CompletionReport", func() {
	It("should marshal to JSON with all fields", func() {
		cr := report.CompletionReport{
			Status:   "success",
			Summary:  "Completed task successfully",
			Blockers: []string{},
		}

		data, err := json.Marshal(cr)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring(`"status":"success"`))
		Expect(string(data)).To(ContainSubstring(`"summary":"Completed task successfully"`))
		Expect(string(data)).To(ContainSubstring(`"blockers":[]`))
	})

	It("should marshal with blockers array", func() {
		cr := report.CompletionReport{
			Status:   "partial",
			Summary:  "Partial completion",
			Blockers: []string{"test failed", "dependency missing"},
		}

		data, err := json.Marshal(cr)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring(`"status":"partial"`))
		Expect(string(data)).To(ContainSubstring(`"blockers":["test failed","dependency missing"]`))
	})

	It("should unmarshal from JSON correctly", func() {
		jsonData := `{"status":"failed","summary":"Task failed","blockers":["error occurred"]}`

		var cr report.CompletionReport
		err := json.Unmarshal([]byte(jsonData), &cr)
		Expect(err).NotTo(HaveOccurred())
		Expect(cr.Status).To(Equal("failed"))
		Expect(cr.Summary).To(Equal("Task failed"))
		Expect(cr.Blockers).To(HaveLen(1))
		Expect(cr.Blockers[0]).To(Equal("error occurred"))
	})

	It("should round-trip marshal and unmarshal all fields", func() {
		original := report.CompletionReport{
			Status:   "success",
			Summary:  "Round trip test",
			Blockers: []string{},
		}

		// Marshal
		data, err := json.Marshal(original)
		Expect(err).NotTo(HaveOccurred())

		// Unmarshal
		var restored report.CompletionReport
		err = json.Unmarshal(data, &restored)
		Expect(err).NotTo(HaveOccurred())

		// Verify all fields survived
		Expect(restored.Status).To(Equal(original.Status))
		Expect(restored.Summary).To(Equal(original.Summary))
		Expect(restored.Blockers).To(Equal(original.Blockers))
	})

	It("should round-trip with multiple blockers", func() {
		original := report.CompletionReport{
			Status:   "partial",
			Summary:  "Multiple blockers test",
			Blockers: []string{"blocker 1", "blocker 2", "blocker 3"},
		}

		// Marshal
		data, err := json.Marshal(original)
		Expect(err).NotTo(HaveOccurred())

		// Unmarshal
		var restored report.CompletionReport
		err = json.Unmarshal(data, &restored)
		Expect(err).NotTo(HaveOccurred())

		// Verify all fields survived
		Expect(restored.Status).To(Equal(original.Status))
		Expect(restored.Summary).To(Equal(original.Summary))
		Expect(restored.Blockers).To(HaveLen(3))
		Expect(restored.Blockers).To(Equal(original.Blockers))
	})

	It("should marshal with verification field", func() {
		cr := report.CompletionReport{
			Status:   "success",
			Summary:  "Task completed",
			Blockers: []string{},
			Verification: &report.Verification{
				Command:  "make precommit",
				ExitCode: 0,
			},
		}

		data, err := json.Marshal(cr)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring(`"verification":`))
		Expect(string(data)).To(ContainSubstring(`"command":"make precommit"`))
		Expect(string(data)).To(ContainSubstring(`"exitCode":0`))
	})

	It("should omit verification field when nil", func() {
		cr := report.CompletionReport{
			Status:       "success",
			Summary:      "Task completed",
			Blockers:     []string{},
			Verification: nil,
		}

		data, err := json.Marshal(cr)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).NotTo(ContainSubstring(`"verification"`))
	})

	It("should unmarshal from JSON with verification field", func() {
		jsonData := `{"status":"success","summary":"Done","blockers":[],"verification":{"command":"make test","exitCode":0}}`

		var cr report.CompletionReport
		err := json.Unmarshal([]byte(jsonData), &cr)
		Expect(err).NotTo(HaveOccurred())
		Expect(cr.Verification).NotTo(BeNil())
		Expect(cr.Verification.Command).To(Equal("make test"))
		Expect(cr.Verification.ExitCode).To(Equal(0))
	})

	It("should unmarshal from JSON without verification field (backwards compatible)", func() {
		jsonData := `{"status":"success","summary":"Done","blockers":[]}`

		var cr report.CompletionReport
		err := json.Unmarshal([]byte(jsonData), &cr)
		Expect(err).NotTo(HaveOccurred())
		Expect(cr.Status).To(Equal("success"))
		Expect(cr.Summary).To(Equal("Done"))
		Expect(cr.Verification).To(BeNil())
	})
})

var _ = Describe("ValidateConsistency", func() {
	It("should return success unchanged when verification is nil", func() {
		cr := report.CompletionReport{
			Status:       "success",
			Summary:      "Task completed",
			Blockers:     []string{},
			Verification: nil,
		}

		correctedStatus, overridden := cr.ValidateConsistency()
		Expect(correctedStatus).To(Equal("success"))
		Expect(overridden).To(BeFalse())
	})

	It("should return success unchanged when verification exitCode is 0", func() {
		cr := report.CompletionReport{
			Status:   "success",
			Summary:  "Task completed",
			Blockers: []string{},
			Verification: &report.Verification{
				Command:  "make precommit",
				ExitCode: 0,
			},
		}

		correctedStatus, overridden := cr.ValidateConsistency()
		Expect(correctedStatus).To(Equal("success"))
		Expect(overridden).To(BeFalse())
	})

	It("should override success to partial when verification exitCode is non-zero", func() {
		cr := report.CompletionReport{
			Status:   "success",
			Summary:  "Task completed",
			Blockers: []string{},
			Verification: &report.Verification{
				Command:  "make precommit",
				ExitCode: 1,
			},
		}

		correctedStatus, overridden := cr.ValidateConsistency()
		Expect(correctedStatus).To(Equal("partial"))
		Expect(overridden).To(BeTrue())
	})

	It("should not override failed status even with non-zero exitCode", func() {
		cr := report.CompletionReport{
			Status:   "failed",
			Summary:  "Task failed",
			Blockers: []string{"build error"},
			Verification: &report.Verification{
				Command:  "make test",
				ExitCode: 1,
			},
		}

		correctedStatus, overridden := cr.ValidateConsistency()
		Expect(correctedStatus).To(Equal("failed"))
		Expect(overridden).To(BeFalse())
	})

	It("should not override partial status even with non-zero exitCode", func() {
		cr := report.CompletionReport{
			Status:   "partial",
			Summary:  "Partially completed",
			Blockers: []string{"some issues"},
			Verification: &report.Verification{
				Command:  "make precommit",
				ExitCode: 1,
			},
		}

		correctedStatus, overridden := cr.ValidateConsistency()
		Expect(correctedStatus).To(Equal("partial"))
		Expect(overridden).To(BeFalse())
	})
})
