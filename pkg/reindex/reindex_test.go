// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reindex_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/reindex"
)

type osFileMover struct{}

func (m *osFileMover) MoveFile(_ context.Context, oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

type errorFileMover struct{ err error }

func (m *errorFileMover) MoveFile(_ context.Context, _, _ string) error {
	return m.err
}

func writeFile(path string, created string) {
	content := "---\n"
	if created != "" {
		content += fmt.Sprintf("created: %q\n", created)
	}
	content += "---\n\nbody\n"
	Expect(os.WriteFile(path, []byte(content), 0600)).To(Succeed())
}

var _ = Describe("Reindexer", func() {
	var ctx context.Context
	var mover reindex.FileMover

	BeforeEach(func() {
		ctx = context.Background()
		mover = &osFileMover{}
	})

	Context("no duplicates", func() {
		It("returns empty slice and files are unchanged", func() {
			dir := GinkgoT().TempDir()
			writeFile(filepath.Join(dir, "001-alpha.md"), "2026-01-01")
			writeFile(filepath.Join(dir, "002-beta.md"), "2026-01-02")

			r := reindex.NewReindexer([]string{dir}, mover)
			renames, err := r.Reindex(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(renames).To(BeEmpty())

			Expect(filepath.Join(dir, "001-alpha.md")).To(BeAnExistingFile())
			Expect(filepath.Join(dir, "002-beta.md")).To(BeAnExistingFile())
		})
	})

	Context("duplicate in same dir, earlier created keeps number", func() {
		It("renames the file with later created date", func() {
			dir := GinkgoT().TempDir()
			writeFile(filepath.Join(dir, "035-foo.md"), "2026-01-01T00:00:00Z")
			writeFile(filepath.Join(dir, "035-bar.md"), "2026-02-01T00:00:00Z")

			r := reindex.NewReindexer([]string{dir}, mover)
			renames, err := r.Reindex(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(renames).To(HaveLen(1))

			// 035-foo.md (earlier) should keep its number
			Expect(filepath.Join(dir, "035-foo.md")).To(BeAnExistingFile())
			// 035-bar.md should be renamed
			Expect(filepath.Join(dir, "035-bar.md")).NotTo(BeAnExistingFile())
			// New file should exist
			Expect(renames[0].NewPath).To(BeAnExistingFile())
		})
	})

	Context("duplicate across dirs, earlier created keeps number", func() {
		It("renames the file in the dir with later created date", func() {
			dir1 := GinkgoT().TempDir()
			dir2 := GinkgoT().TempDir()
			writeFile(filepath.Join(dir1, "035-foo.md"), "2026-01-01")
			writeFile(filepath.Join(dir2, "035-bar.md"), "2026-02-01")

			r := reindex.NewReindexer([]string{dir1, dir2}, mover)
			renames, err := r.Reindex(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(renames).To(HaveLen(1))

			// dir1/035-foo.md (earlier) should remain unchanged
			Expect(filepath.Join(dir1, "035-foo.md")).To(BeAnExistingFile())
			// dir2/035-bar.md should be renamed
			Expect(filepath.Join(dir2, "035-bar.md")).NotTo(BeAnExistingFile())
		})
	})

	Context("alphabetical tiebreak when created dates are equal", func() {
		It("file with earlier alphabetical name keeps the number", func() {
			dir := GinkgoT().TempDir()
			writeFile(filepath.Join(dir, "010-zoo.md"), "2026-01-01T00:00:00Z")
			writeFile(filepath.Join(dir, "010-aaa.md"), "2026-01-01T00:00:00Z")

			r := reindex.NewReindexer([]string{dir}, mover)
			renames, err := r.Reindex(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(renames).To(HaveLen(1))

			// 010-aaa.md (alphabetically earlier) should keep 010
			Expect(filepath.Join(dir, "010-aaa.md")).To(BeAnExistingFile())
			// 010-zoo.md should be renamed
			Expect(filepath.Join(dir, "010-zoo.md")).NotTo(BeAnExistingFile())
		})
	})

	Context("alphabetical tiebreak when created is missing", func() {
		It("earlier alphabetical name keeps the number", func() {
			dir := GinkgoT().TempDir()
			writeFile(filepath.Join(dir, "020-zzz.md"), "")
			writeFile(filepath.Join(dir, "020-aaa.md"), "")

			r := reindex.NewReindexer([]string{dir}, mover)
			renames, err := r.Reindex(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(renames).To(HaveLen(1))

			Expect(filepath.Join(dir, "020-aaa.md")).To(BeAnExistingFile())
			Expect(filepath.Join(dir, "020-zzz.md")).NotTo(BeAnExistingFile())
		})
	})

	Context("one has created, one does not", func() {
		It("file with valid created keeps the number", func() {
			dir := GinkgoT().TempDir()
			writeFile(filepath.Join(dir, "015-nocreated.md"), "")
			writeFile(filepath.Join(dir, "015-hascreated.md"), "2026-01-01T00:00:00Z")

			r := reindex.NewReindexer([]string{dir}, mover)
			renames, err := r.Reindex(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(renames).To(HaveLen(1))

			// File with valid created keeps the number
			Expect(filepath.Join(dir, "015-hascreated.md")).To(BeAnExistingFile())
			Expect(filepath.Join(dir, "015-nocreated.md")).NotTo(BeAnExistingFile())
		})
	})

	Context("three-way conflict", func() {
		It("oldest keeps number, next two get sequential new numbers", func() {
			dir := GinkgoT().TempDir()
			writeFile(filepath.Join(dir, "005-alpha.md"), "2026-01-01T00:00:00Z")
			writeFile(filepath.Join(dir, "005-beta.md"), "2026-02-01T00:00:00Z")
			writeFile(filepath.Join(dir, "005-gamma.md"), "2026-03-01T00:00:00Z")

			r := reindex.NewReindexer([]string{dir}, mover)
			renames, err := r.Reindex(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(renames).To(HaveLen(2))

			// 005-alpha.md (oldest) keeps its number
			Expect(filepath.Join(dir, "005-alpha.md")).To(BeAnExistingFile())
			// beta and gamma get new numbers
			Expect(filepath.Join(dir, "005-beta.md")).NotTo(BeAnExistingFile())
			Expect(filepath.Join(dir, "005-gamma.md")).NotTo(BeAnExistingFile())
			// Two new files should exist
			Expect(renames[0].NewPath).To(BeAnExistingFile())
			Expect(renames[1].NewPath).To(BeAnExistingFile())
		})
	})

	Context("idempotent", func() {
		It("second call returns empty slice", func() {
			dir := GinkgoT().TempDir()
			writeFile(filepath.Join(dir, "007-foo.md"), "2026-01-01T00:00:00Z")
			writeFile(filepath.Join(dir, "007-bar.md"), "2026-02-01T00:00:00Z")

			r := reindex.NewReindexer([]string{dir}, mover)
			_, err := r.Reindex(ctx)
			Expect(err).NotTo(HaveOccurred())

			renames2, err := r.Reindex(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(renames2).To(BeEmpty())
		})
	})

	Context("no-op when dirs do not exist", func() {
		It("returns empty slice without error", func() {
			r := reindex.NewReindexer([]string{"/nonexistent/dir/abc"}, mover)
			renames, err := r.Reindex(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(renames).To(BeEmpty())
		})
	})

	Context("gap-filling", func() {
		It("assigns next available number when 1-5 are used", func() {
			dir := GinkgoT().TempDir()
			writeFile(filepath.Join(dir, "001-a.md"), "")
			writeFile(filepath.Join(dir, "002-b.md"), "")
			writeFile(filepath.Join(dir, "003-c.md"), "")
			writeFile(filepath.Join(dir, "004-d.md"), "")
			writeFile(filepath.Join(dir, "005-e.md"), "")
			writeFile(filepath.Join(dir, "005-f.md"), "2026-06-01T00:00:00Z")

			r := reindex.NewReindexer([]string{dir}, mover)
			renames, err := r.Reindex(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(renames).To(HaveLen(1))

			// New file should be 006-*.md
			Expect(renames[0].NewPath).To(ContainSubstring("006-"))
		})
	})

	Context("rename error is returned", func() {
		It("propagates the error", func() {
			dir := GinkgoT().TempDir()
			writeFile(filepath.Join(dir, "009-foo.md"), "2026-01-01T00:00:00Z")
			writeFile(filepath.Join(dir, "009-bar.md"), "2026-02-01T00:00:00Z")

			errMover := &errorFileMover{err: fmt.Errorf("move failed")}
			r := reindex.NewReindexer([]string{dir}, errMover)
			_, err := r.Reindex(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("move failed"))
		})
	})
})
