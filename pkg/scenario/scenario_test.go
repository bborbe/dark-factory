// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package scenario_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/scenario"
)

var _ = Describe("Load", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("loads a normal file with frontmatter and title", func() {
		dir := GinkgoT().TempDir()
		path := filepath.Join(dir, "plain-no-number.md")
		Expect(
			os.WriteFile(
				path,
				[]byte(
					"---\nstatus: active\n---\n\n# My scenario title\n\nValidates that something works.\n",
				),
				0600,
			),
		).To(Succeed())

		sf, err := scenario.Load(ctx, path)
		Expect(err).NotTo(HaveOccurred())
		Expect(sf.Frontmatter.Status).To(Equal("active"))
		Expect(sf.Title).To(Equal("My scenario title"))
		Expect(sf.Number).To(Equal(-1))
		Expect(sf.RawContent).NotTo(BeEmpty())
	})

	It("extracts numeric prefix from filename", func() {
		dir := GinkgoT().TempDir()
		path := filepath.Join(dir, "042-my-scenario.md")
		Expect(os.WriteFile(path, []byte("---\nstatus: draft\n---\n# Title\n"), 0600)).To(Succeed())

		sf, err := scenario.Load(ctx, path)
		Expect(err).NotTo(HaveOccurred())
		Expect(sf.Number).To(Equal(42))
	})

	It("handles malformed frontmatter gracefully", func() {
		dir := GinkgoT().TempDir()
		path := filepath.Join(dir, "no-frontmatter.md")
		Expect(os.WriteFile(path, []byte("# Just a title\n\nSome content.\n"), 0600)).To(Succeed())

		sf, err := scenario.Load(ctx, path)
		Expect(err).NotTo(HaveOccurred())
		Expect(sf.Frontmatter.Status).To(Equal(""))
		Expect(sf.Title).To(Equal("Just a title"))
	})

	It("returns empty title when no heading present", func() {
		dir := GinkgoT().TempDir()
		path := filepath.Join(dir, "no-title.md")
		Expect(
			os.WriteFile(path, []byte("---\nstatus: draft\n---\n\nNo heading here.\n"), 0600),
		).To(Succeed())

		sf, err := scenario.Load(ctx, path)
		Expect(err).NotTo(HaveOccurred())
		Expect(sf.Title).To(Equal(""))
	})

	It("returns error when file cannot be read", func() {
		_, err := scenario.Load(ctx, "/nonexistent/path/file.md")
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("IsKnown", func() {
	It("returns true for known statuses", func() {
		Expect(scenario.IsKnown(scenario.StatusActive)).To(BeTrue())
		Expect(scenario.IsKnown(scenario.StatusIdea)).To(BeTrue())
		Expect(scenario.IsKnown(scenario.StatusDraft)).To(BeTrue())
		Expect(scenario.IsKnown(scenario.StatusOutdated)).To(BeTrue())
	})

	It("returns false for unknown statuses", func() {
		Expect(scenario.IsKnown(scenario.Status("unknown"))).To(BeFalse())
		Expect(scenario.IsKnown(scenario.Status(""))).To(BeFalse())
	})
})
