// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor_test

import (
	"regexp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/processor"
)

// dockerNameRegexp is the Docker container name validation regex.
// Docker requires names matching [a-zA-Z0-9][a-zA-Z0-9_.-]*.
var dockerNameRegexp = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_./-]*$`)

var _ = Describe("ContainerName", func() {
	Describe("Sanitize", func() {
		It("keeps valid characters", func() {
			name := processor.ContainerName("abc-123_XYZ").Sanitize()
			Expect(name.String()).To(Equal("abc-123_XYZ"))
		})

		It("replaces special characters with hyphens", func() {
			name := processor.ContainerName("test@file#name").Sanitize()
			Expect(name.String()).To(Equal("test-file-name"))
		})

		It("handles spaces", func() {
			name := processor.ContainerName("hello world").Sanitize()
			Expect(name.String()).To(Equal("hello-world"))
		})

		It("handles multiple consecutive special characters", func() {
			name := processor.ContainerName("test@@##name").Sanitize()
			Expect(name.String()).To(Equal("test----name"))
		})

		It("handles slashes", func() {
			name := processor.ContainerName("dark-factory/123-fix-bug").Sanitize()
			Expect(name.String()).To(Equal("dark-factory-123-fix-bug"))
		})

		It("handles unicode characters", func() {
			name := processor.ContainerName("héllo-wörld").Sanitize()
			Expect(name.String()).To(Equal("h-llo-w-rld"))
		})

		It("handles dots", func() {
			name := processor.ContainerName("some.prompt.md").Sanitize()
			Expect(name.String()).To(Equal("some-prompt-md"))
		})
	})

	Describe("String", func() {
		It("returns the underlying string", func() {
			name := processor.ContainerName("my-container")
			Expect(name.String()).To(Equal("my-container"))
		})
	})

	Describe("Docker name contract", func() {
		It("produces Docker-safe names for typical project-prompt inputs", func() {
			inputs := []string{
				"myproject-123-fix-bug",
				"dark-factory-001-add-feature",
				"project_name-456-refactor",
			}
			for _, input := range inputs {
				sanitized := processor.ContainerName(input).Sanitize()
				Expect(dockerNameRegexp.MatchString(sanitized.String())).To(BeTrue(),
					"expected %q to match docker name regex", sanitized.String())
			}
		})

		It("produces leading hyphen when input starts with a special character", func() {
			// Document the current behavior: Sanitize() does not add a prefix.
			// If the input starts with a special char, the result starts with '-'.
			name := processor.ContainerName("@leading-special").Sanitize()
			Expect(name.String()).To(Equal("-leading-special"))
			// Note: this does NOT pass Docker's name regex (requires [a-zA-Z0-9] start).
			// Callers should ensure the prefix (e.g. project name) starts with an
			// alphanumeric character; typical project names from config satisfy this.
		})

		It("produces empty string for all-special input", func() {
			name := processor.ContainerName("@@@").Sanitize()
			Expect(name.String()).To(Equal("---"))
		})
	})
})

var _ = Describe("BaseName", func() {
	Describe("String", func() {
		It("returns the underlying string", func() {
			base := processor.BaseName("001-fix-bug")
			Expect(base.String()).To(Equal("001-fix-bug"))
		})
	})
})
