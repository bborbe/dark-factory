// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/cmd"
)

var _ = Describe("SpecVerifyCommand", func() {
	var (
		tempDir       string
		specsDir      string
		specVerifyCmd cmd.SpecVerifyCommand
		ctx           context.Context
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "spec-verify-test-*")
		Expect(err).NotTo(HaveOccurred())

		specsDir = filepath.Join(tempDir, "specs")
		err = os.MkdirAll(specsDir, 0750)
		Expect(err).NotTo(HaveOccurred())

		specVerifyCmd = cmd.NewSpecVerifyCommand(specsDir)
		ctx = context.Background()
	})

	AfterEach(func() {
		_ = os.RemoveAll(tempDir)
	})

	Describe("Run", func() {
		It("returns error when no identifier given", func() {
			err := specVerifyCmd.Run(ctx, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec identifier required"))
		})

		It("verifies a spec in verifying state", func() {
			specFile := filepath.Join(specsDir, "001-my-spec.md")
			err := os.WriteFile(specFile, []byte("---\nstatus: verifying\n---\n# My Spec"), 0600)
			Expect(err).NotTo(HaveOccurred())

			err = specVerifyCmd.Run(ctx, []string{"001-my-spec.md"})
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(specFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("status: completed"))
		})

		It("returns error when spec is in draft state", func() {
			specFile := filepath.Join(specsDir, "001-my-spec.md")
			err := os.WriteFile(specFile, []byte("---\nstatus: draft\n---\n# My Spec"), 0600)
			Expect(err).NotTo(HaveOccurred())

			err = specVerifyCmd.Run(ctx, []string{"001-my-spec.md"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not in verifying state"))
		})

		It("returns error when spec is in approved state", func() {
			specFile := filepath.Join(specsDir, "001-my-spec.md")
			err := os.WriteFile(specFile, []byte("---\nstatus: approved\n---\n# My Spec"), 0600)
			Expect(err).NotTo(HaveOccurred())

			err = specVerifyCmd.Run(ctx, []string{"001-my-spec.md"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not in verifying state"))
		})

		It("returns error when spec is in completed state", func() {
			specFile := filepath.Join(specsDir, "001-my-spec.md")
			err := os.WriteFile(specFile, []byte("---\nstatus: completed\n---\n# My Spec"), 0600)
			Expect(err).NotTo(HaveOccurred())

			err = specVerifyCmd.Run(ctx, []string{"001-my-spec.md"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not in verifying state"))
		})

		It("returns error when spec file not found", func() {
			err := specVerifyCmd.Run(ctx, []string{"999-nonexistent.md"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec not found"))
		})

		It("returns error when specs dir does not exist", func() {
			specVerifyCmd = cmd.NewSpecVerifyCommand("/nonexistent/specs")
			err := specVerifyCmd.Run(ctx, []string{"001"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec not found"))
		})
	})
})
