// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package report_test

import (
	"encoding/json"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/report"
)

func TestReport(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Report Suite")
}

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
})
