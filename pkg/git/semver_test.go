// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/git"
)

var _ = Describe("SemanticVersionNumber", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("ParseSemanticVersionNumber", func() {
		Context("with valid semver tags", func() {
			It("parses v0.2.25", func() {
				version, err := git.ParseSemanticVersionNumber(ctx, "v0.2.25")
				Expect(err).To(BeNil())
				Expect(version.Major).To(Equal(0))
				Expect(version.Minor).To(Equal(2))
				Expect(version.Patch).To(Equal(25))
			})

			It("parses v1.0.0", func() {
				version, err := git.ParseSemanticVersionNumber(ctx, "v1.0.0")
				Expect(err).To(BeNil())
				Expect(version.Major).To(Equal(1))
				Expect(version.Minor).To(Equal(0))
				Expect(version.Patch).To(Equal(0))
			})

			It("parses v10.20.30", func() {
				version, err := git.ParseSemanticVersionNumber(ctx, "v10.20.30")
				Expect(err).To(BeNil())
				Expect(version.Major).To(Equal(10))
				Expect(version.Minor).To(Equal(20))
				Expect(version.Patch).To(Equal(30))
			})
		})

		Context("with invalid tags", func() {
			It("returns error for non-semver tag", func() {
				_, err := git.ParseSemanticVersionNumber(ctx, "invalid")
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("invalid version tag"))
			})

			It("returns error for incomplete version v1", func() {
				_, err := git.ParseSemanticVersionNumber(ctx, "v1")
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("invalid version tag"))
			})

			It("returns error for incomplete version v1.2", func() {
				_, err := git.ParseSemanticVersionNumber(ctx, "v1.2")
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("invalid version tag"))
			})

			It("returns error for tag without v prefix", func() {
				_, err := git.ParseSemanticVersionNumber(ctx, "1.2.3")
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("invalid version tag"))
			})

			It("returns error for empty string", func() {
				_, err := git.ParseSemanticVersionNumber(ctx, "")
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("String", func() {
		It("converts {0, 2, 25} to v0.2.25", func() {
			version := git.SemanticVersionNumber{Major: 0, Minor: 2, Patch: 25}
			Expect(version.String()).To(Equal("v0.2.25"))
		})

		It("converts {1, 0, 0} to v1.0.0", func() {
			version := git.SemanticVersionNumber{Major: 1, Minor: 0, Patch: 0}
			Expect(version.String()).To(Equal("v1.0.0"))
		})

		It("converts {10, 20, 30} to v10.20.30", func() {
			version := git.SemanticVersionNumber{Major: 10, Minor: 20, Patch: 30}
			Expect(version.String()).To(Equal("v10.20.30"))
		})
	})

	Describe("BumpPatch", func() {
		It("bumps v0.2.25 to v0.2.26", func() {
			version := git.SemanticVersionNumber{Major: 0, Minor: 2, Patch: 25}
			bumped := version.BumpPatch()
			Expect(bumped.String()).To(Equal("v0.2.26"))
		})

		It("bumps v1.0.0 to v1.0.1", func() {
			version := git.SemanticVersionNumber{Major: 1, Minor: 0, Patch: 0}
			bumped := version.BumpPatch()
			Expect(bumped.String()).To(Equal("v1.0.1"))
		})

		It("bumps v0.0.99 to v0.0.100", func() {
			version := git.SemanticVersionNumber{Major: 0, Minor: 0, Patch: 99}
			bumped := version.BumpPatch()
			Expect(bumped.String()).To(Equal("v0.0.100"))
		})
	})

	Describe("BumpMinor", func() {
		It("bumps v0.2.25 to v0.3.0", func() {
			version := git.SemanticVersionNumber{Major: 0, Minor: 2, Patch: 25}
			bumped := version.BumpMinor()
			Expect(bumped.String()).To(Equal("v0.3.0"))
		})

		It("bumps v1.0.0 to v1.1.0", func() {
			version := git.SemanticVersionNumber{Major: 1, Minor: 0, Patch: 0}
			bumped := version.BumpMinor()
			Expect(bumped.String()).To(Equal("v1.1.0"))
		})

		It("resets patch to 0 when bumping minor", func() {
			version := git.SemanticVersionNumber{Major: 1, Minor: 5, Patch: 99}
			bumped := version.BumpMinor()
			Expect(bumped.Major).To(Equal(1))
			Expect(bumped.Minor).To(Equal(6))
			Expect(bumped.Patch).To(Equal(0))
		})
	})

	Describe("Less", func() {
		Context("comparing major versions", func() {
			It("returns true when v0.2.25 < v1.0.0", func() {
				v1 := git.SemanticVersionNumber{Major: 0, Minor: 2, Patch: 25}
				v2 := git.SemanticVersionNumber{Major: 1, Minor: 0, Patch: 0}
				Expect(v1.Less(v2)).To(BeTrue())
			})

			It("returns false when v1.0.0 < v0.99.99", func() {
				v1 := git.SemanticVersionNumber{Major: 1, Minor: 0, Patch: 0}
				v2 := git.SemanticVersionNumber{Major: 0, Minor: 99, Patch: 99}
				Expect(v1.Less(v2)).To(BeFalse())
			})
		})

		Context("comparing minor versions", func() {
			It("returns true when v0.1.9 < v0.2.25", func() {
				v1 := git.SemanticVersionNumber{Major: 0, Minor: 1, Patch: 9}
				v2 := git.SemanticVersionNumber{Major: 0, Minor: 2, Patch: 25}
				Expect(v1.Less(v2)).To(BeTrue())
			})

			It("returns true when v0.9.0 < v0.10.0", func() {
				v1 := git.SemanticVersionNumber{Major: 0, Minor: 9, Patch: 0}
				v2 := git.SemanticVersionNumber{Major: 0, Minor: 10, Patch: 0}
				Expect(v1.Less(v2)).To(BeTrue())
			})

			It("returns false when v0.10.0 < v0.9.0", func() {
				v1 := git.SemanticVersionNumber{Major: 0, Minor: 10, Patch: 0}
				v2 := git.SemanticVersionNumber{Major: 0, Minor: 9, Patch: 0}
				Expect(v1.Less(v2)).To(BeFalse())
			})
		})

		Context("comparing patch versions", func() {
			It("returns true when v0.2.24 < v0.2.25", func() {
				v1 := git.SemanticVersionNumber{Major: 0, Minor: 2, Patch: 24}
				v2 := git.SemanticVersionNumber{Major: 0, Minor: 2, Patch: 25}
				Expect(v1.Less(v2)).To(BeTrue())
			})

			It("returns true when v0.2.9 < v0.2.10", func() {
				v1 := git.SemanticVersionNumber{Major: 0, Minor: 2, Patch: 9}
				v2 := git.SemanticVersionNumber{Major: 0, Minor: 2, Patch: 10}
				Expect(v1.Less(v2)).To(BeTrue())
			})

			It("returns false when v0.2.10 < v0.2.9", func() {
				v1 := git.SemanticVersionNumber{Major: 0, Minor: 2, Patch: 10}
				v2 := git.SemanticVersionNumber{Major: 0, Minor: 2, Patch: 9}
				Expect(v1.Less(v2)).To(BeFalse())
			})
		})

		Context("comparing equal versions", func() {
			It("returns false when v0.2.25 == v0.2.25", func() {
				v1 := git.SemanticVersionNumber{Major: 0, Minor: 2, Patch: 25}
				v2 := git.SemanticVersionNumber{Major: 0, Minor: 2, Patch: 25}
				Expect(v1.Less(v2)).To(BeFalse())
			})
		})

		Context("regression test cases", func() {
			It("v0.2.25 is greater than v0.1.9", func() {
				v1 := git.SemanticVersionNumber{Major: 0, Minor: 2, Patch: 25}
				v2 := git.SemanticVersionNumber{Major: 0, Minor: 1, Patch: 9}
				Expect(v2.Less(v1)).To(BeTrue())
				Expect(v1.Less(v2)).To(BeFalse())
			})
		})
	})
})
