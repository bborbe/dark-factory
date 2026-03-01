// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package status

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Format Functions", func() {
	Describe("formatDuration", func() {
		It("formats seconds", func() {
			d := 45 * time.Second
			Expect(formatDuration(d)).To(Equal("45s"))
		})

		It("formats minutes and seconds", func() {
			d := 2*time.Minute + 30*time.Second
			Expect(formatDuration(d)).To(Equal("2m30s"))
		})

		It("formats hours, minutes, and seconds", func() {
			d := 1*time.Hour + 15*time.Minute + 20*time.Second
			Expect(formatDuration(d)).To(Equal("1h15m20s"))
		})

		It("formats hours only", func() {
			d := 2 * time.Hour
			Expect(formatDuration(d)).To(Equal("2h"))
		})

		It("formats minutes only", func() {
			d := 5 * time.Minute
			Expect(formatDuration(d)).To(Equal("5m"))
		})
	})

	Describe("formatInt", func() {
		It("formats zero", func() {
			Expect(formatInt(0)).To(Equal("0"))
		})

		It("formats single digit", func() {
			Expect(formatInt(5)).To(Equal("5"))
		})

		It("formats multiple digits", func() {
			Expect(formatInt(123)).To(Equal("123"))
		})
	})

	Describe("formatBytes", func() {
		It("formats bytes", func() {
			Expect(formatBytes(500)).To(Equal("500 B"))
		})

		It("formats kilobytes", func() {
			Expect(formatBytes(1536)).To(Equal("1.5 KB"))
		})

		It("formats megabytes", func() {
			Expect(formatBytes(2 * 1024 * 1024)).To(Equal("2.0 MB"))
		})

		It("formats gigabytes", func() {
			Expect(formatBytes(3 * 1024 * 1024 * 1024)).To(Equal("3.0 GB"))
		})

		It("formats terabytes", func() {
			Expect(formatBytes(4 * 1024 * 1024 * 1024 * 1024)).To(Equal("4.0 TB"))
		})
	})
})
