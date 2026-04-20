// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package scenario_test

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/scenario"
)

func writeScenarioFile(dir, name, content string) {
	GinkgoHelper()
	Expect(os.WriteFile(filepath.Join(dir, name), []byte(content), 0600)).To(Succeed())
}

var _ = Describe("Lister", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("List", func() {
		It("returns empty slice when directory does not exist", func() {
			l := scenario.NewLister(fmt.Sprintf("/tmp/does-not-exist-%d", rand.Int()))
			files, err := l.List(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(files).To(BeEmpty())
		})

		It("skips non-NNN-*.md files", func() {
			dir := GinkgoT().TempDir()
			writeScenarioFile(dir, "README.md", "# readme\n")
			writeScenarioFile(dir, "001-first.md", "---\nstatus: active\n---\n# First\n")

			l := scenario.NewLister(dir)
			files, err := l.List(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(files).To(HaveLen(1))
			Expect(files[0].Name).To(Equal("001-first"))
		})

		It("sorts files by number ascending", func() {
			dir := GinkgoT().TempDir()
			writeScenarioFile(dir, "003-third.md", "---\nstatus: active\n---\n# Third\n")
			writeScenarioFile(dir, "001-first.md", "---\nstatus: active\n---\n# First\n")
			writeScenarioFile(dir, "002-second.md", "---\nstatus: active\n---\n# Second\n")

			l := scenario.NewLister(dir)
			files, err := l.List(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(files).To(HaveLen(3))
			Expect(files[0].Number).To(Equal(1))
			Expect(files[1].Number).To(Equal(2))
			Expect(files[2].Number).To(Equal(3))
		})

		It("includes file with malformed frontmatter with empty status", func() {
			dir := GinkgoT().TempDir()
			writeScenarioFile(dir, "001-bad.md", "no frontmatter delimiters\n# Title\n")

			l := scenario.NewLister(dir)
			files, err := l.List(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(files).To(HaveLen(1))
			Expect(files[0].Frontmatter.Status).To(Equal(""))
		})
	})

	Describe("Summary", func() {
		It("counts all known statuses and unknown", func() {
			dir := GinkgoT().TempDir()
			writeScenarioFile(dir, "001-a.md", "---\nstatus: idea\n---\n# A\n")
			writeScenarioFile(dir, "002-b.md", "---\nstatus: active\n---\n# B\n")
			writeScenarioFile(dir, "003-c.md", "---\nstatus: outdated\n---\n# C\n")
			writeScenarioFile(dir, "004-d.md", "no frontmatter\n")
			writeScenarioFile(dir, "005-e.md", "---\nstatus: bogus\n---\n# E\n")

			l := scenario.NewLister(dir)
			s, err := l.Summary(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(s.Idea).To(Equal(1))
			Expect(s.Active).To(Equal(1))
			Expect(s.Outdated).To(Equal(1))
			Expect(s.Unknown).To(Equal(2))
			Expect(s.Total).To(Equal(5))
		})
	})

	Describe("Find", func() {
		It("finds by numeric prefix", func() {
			dir := GinkgoT().TempDir()
			writeScenarioFile(dir, "001-workflow-direct.md", "---\nstatus: active\n---\n# Direct\n")

			l := scenario.NewLister(dir)
			matches, err := l.Find(ctx, "001")
			Expect(err).NotTo(HaveOccurred())
			Expect(matches).To(HaveLen(1))
			Expect(matches[0].Number).To(Equal(1))
		})

		It("finds by name fragment", func() {
			dir := GinkgoT().TempDir()
			writeScenarioFile(dir, "001-workflow-direct.md", "---\nstatus: active\n---\n# Direct\n")

			l := scenario.NewLister(dir)
			matches, err := l.Find(ctx, "workflow-direct")
			Expect(err).NotTo(HaveOccurred())
			Expect(matches).To(HaveLen(1))
			Expect(matches[0].Name).To(Equal("001-workflow-direct"))
		})

		It("returns empty slice when no match", func() {
			dir := GinkgoT().TempDir()
			writeScenarioFile(dir, "001-workflow-direct.md", "---\nstatus: active\n---\n# Direct\n")

			l := scenario.NewLister(dir)
			matches, err := l.Find(ctx, "nonexistent")
			Expect(err).NotTo(HaveOccurred())
			Expect(matches).To(BeEmpty())
		})

		It("returns multiple matches for same number", func() {
			dir := GinkgoT().TempDir()
			writeScenarioFile(dir, "001-first.md", "---\nstatus: active\n---\n# First\n")
			writeScenarioFile(dir, "001-second.md", "---\nstatus: draft\n---\n# Second\n")

			l := scenario.NewLister(dir)
			matches, err := l.Find(ctx, "001")
			Expect(err).NotTo(HaveOccurred())
			Expect(matches).To(HaveLen(2))
		})

		It("returns multiple matches for fragment", func() {
			dir := GinkgoT().TempDir()
			writeScenarioFile(dir, "001-workflow-direct.md", "---\nstatus: active\n---\n# Direct\n")
			writeScenarioFile(dir, "002-workflow-pr.md", "---\nstatus: active\n---\n# PR\n")

			l := scenario.NewLister(dir)
			matches, err := l.Find(ctx, "workflow")
			Expect(err).NotTo(HaveOccurred())
			Expect(matches).To(HaveLen(2))
		})
	})
})
