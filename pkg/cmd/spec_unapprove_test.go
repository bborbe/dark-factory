// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd_test

import (
	"context"
	"os"
	"path/filepath"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/cmd"
)

var _ = Describe("SpecUnapproveCommand", func() {
	var (
		tempDir              string
		specsInboxDir        string
		specsInProgressDir   string
		promptsInboxDir      string
		promptsInProgressDir string
		specUnapproveCmd     cmd.SpecUnapproveCommand
		ctx                  context.Context
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "spec-unapprove-test-*")
		Expect(err).NotTo(HaveOccurred())

		specsInboxDir = filepath.Join(tempDir, "specs")
		err = os.MkdirAll(specsInboxDir, 0750)
		Expect(err).NotTo(HaveOccurred())

		specsInProgressDir = filepath.Join(tempDir, "specs", "in-progress")
		err = os.MkdirAll(specsInProgressDir, 0750)
		Expect(err).NotTo(HaveOccurred())

		promptsInboxDir = filepath.Join(tempDir, "prompts")
		err = os.MkdirAll(promptsInboxDir, 0750)
		Expect(err).NotTo(HaveOccurred())

		promptsInProgressDir = filepath.Join(tempDir, "prompts", "in-progress")
		err = os.MkdirAll(promptsInProgressDir, 0750)
		Expect(err).NotTo(HaveOccurred())

		specUnapproveCmd = cmd.NewSpecUnapproveCommand(
			specsInboxDir,
			specsInProgressDir,
			promptsInboxDir,
			promptsInProgressDir,
			libtime.NewCurrentDateTime(),
		)
		ctx = context.Background()
	})

	AfterEach(func() {
		_ = os.RemoveAll(tempDir)
	})

	Describe("Run", func() {
		It("returns error when no identifier given", func() {
			err := specUnapproveCmd.Run(ctx, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec identifier required"))
		})

		It("returns error when spec not found in in-progress dir", func() {
			err := specUnapproveCmd.Run(ctx, []string{"999-nonexistent.md"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec not found"))
		})

		It("unapproves an approved spec and moves it back to inbox", func() {
			specFile := filepath.Join(specsInProgressDir, "001-my-spec.md")
			err := os.WriteFile(
				specFile,
				[]byte(
					"---\nstatus: approved\napproved: \"2026-01-01T00:00:00Z\"\nbranch: dark-factory/spec-001\n---\n# My Spec",
				),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			err = specUnapproveCmd.Run(ctx, []string{"001-my-spec.md"})
			Expect(err).NotTo(HaveOccurred())

			// File should be gone from in-progress
			_, statErr := os.Stat(specFile)
			Expect(os.IsNotExist(statErr)).To(BeTrue())

			// File should be present in inbox with stripped name
			dest := filepath.Join(specsInboxDir, "my-spec.md")
			content, readErr := os.ReadFile(dest)
			Expect(readErr).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("status: draft"))
		})

		It("clears approved and branch fields", func() {
			specFile := filepath.Join(specsInProgressDir, "002-feature.md")
			err := os.WriteFile(
				specFile,
				[]byte(
					"---\nstatus: approved\napproved: \"2026-01-01T00:00:00Z\"\nbranch: dark-factory/spec-002\n---\n# Feature",
				),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			err = specUnapproveCmd.Run(ctx, []string{"002-feature.md"})
			Expect(err).NotTo(HaveOccurred())

			dest := filepath.Join(specsInboxDir, "feature.md")
			content, readErr := os.ReadFile(dest)
			Expect(readErr).NotTo(HaveOccurred())
			Expect(string(content)).NotTo(ContainSubstring("approved:"))
			Expect(string(content)).NotTo(ContainSubstring("branch:"))
		})

		It("returns error when spec has status prompted", func() {
			specFile := filepath.Join(specsInProgressDir, "003-prompted.md")
			err := os.WriteFile(
				specFile,
				[]byte("---\nstatus: prompted\n---\n# Prompted Spec"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			err = specUnapproveCmd.Run(ctx, []string{"003-prompted.md"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("only approved specs can be unapproved"))
		})

		It("returns error when spec has status verifying", func() {
			specFile := filepath.Join(specsInProgressDir, "004-verifying.md")
			err := os.WriteFile(
				specFile,
				[]byte("---\nstatus: verifying\n---\n# Verifying Spec"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			err = specUnapproveCmd.Run(ctx, []string{"004-verifying.md"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("only approved specs can be unapproved"))
		})

		It("unapproves by numeric prefix", func() {
			specFile := filepath.Join(specsInProgressDir, "005-my-spec.md")
			err := os.WriteFile(
				specFile,
				[]byte("---\nstatus: approved\n---\n# My Spec"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			err = specUnapproveCmd.Run(ctx, []string{"005"})
			Expect(err).NotTo(HaveOccurred())

			dest := filepath.Join(specsInboxDir, "my-spec.md")
			_, statErr := os.Stat(dest)
			Expect(statErr).NotTo(HaveOccurred())
		})

		It("renumbers higher-numbered specs after unapprove", func() {
			// Create specs 001, 002, 003 in in-progress
			for _, name := range []string{"001-first.md", "002-second.md", "003-third.md"} {
				err := os.WriteFile(
					filepath.Join(specsInProgressDir, name),
					[]byte("---\nstatus: approved\n---\n# Spec"),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())
			}

			// Unapprove 002
			err := specUnapproveCmd.Run(ctx, []string{"002-second.md"})
			Expect(err).NotTo(HaveOccurred())

			// 002 should be gone
			_, statErr := os.Stat(filepath.Join(specsInProgressDir, "002-second.md"))
			Expect(os.IsNotExist(statErr)).To(BeTrue())

			// 001 should remain unchanged
			_, statErr = os.Stat(filepath.Join(specsInProgressDir, "001-first.md"))
			Expect(statErr).NotTo(HaveOccurred())

			// 003 should be renumbered to 002
			_, statErr = os.Stat(filepath.Join(specsInProgressDir, "002-third.md"))
			Expect(statErr).NotTo(HaveOccurred())

			// Old 003 should be gone
			_, statErr = os.Stat(filepath.Join(specsInProgressDir, "003-third.md"))
			Expect(os.IsNotExist(statErr)).To(BeTrue())
		})

		It("does not renumber specs with lower numbers", func() {
			// Create specs 001, 003 in in-progress
			for _, name := range []string{"001-first.md", "003-third.md"} {
				err := os.WriteFile(
					filepath.Join(specsInProgressDir, name),
					[]byte("---\nstatus: approved\n---\n# Spec"),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())
			}

			// Unapprove 003
			err := specUnapproveCmd.Run(ctx, []string{"003-third.md"})
			Expect(err).NotTo(HaveOccurred())

			// 001 should remain unchanged
			_, statErr := os.Stat(filepath.Join(specsInProgressDir, "001-first.md"))
			Expect(statErr).NotTo(HaveOccurred())

			// 003 should be in inbox (no higher specs to renumber)
			dest := filepath.Join(specsInboxDir, "third.md")
			_, statErr = os.Stat(dest)
			Expect(statErr).NotTo(HaveOccurred())
		})

		It("returns error when linked prompt exists in prompts inbox", func() {
			specFile := filepath.Join(specsInProgressDir, "010-my-spec.md")
			err := os.WriteFile(
				specFile,
				[]byte("---\nstatus: approved\n---\n# My Spec"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			// Create a prompt linking to spec 010
			promptFile := filepath.Join(promptsInboxDir, "050-do-something.md")
			err = os.WriteFile(
				promptFile,
				[]byte("---\nstatus: draft\nspec: [\"010\"]\n---\n# Do Something"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			err = specUnapproveCmd.Run(ctx, []string{"010-my-spec.md"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("linked prompts"))
		})

		It("returns error when linked prompt exists in prompts in-progress", func() {
			specFile := filepath.Join(specsInProgressDir, "011-my-spec.md")
			err := os.WriteFile(
				specFile,
				[]byte("---\nstatus: approved\n---\n# My Spec"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			// Create a prompt linking to spec 011
			promptFile := filepath.Join(promptsInProgressDir, "060-executing.md")
			err = os.WriteFile(
				promptFile,
				[]byte("---\nstatus: executing\nspec: [\"011\"]\n---\n# Executing Prompt"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			err = specUnapproveCmd.Run(ctx, []string{"011-my-spec.md"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("linked prompts"))
		})

		It("succeeds when no linked prompts exist", func() {
			specFile := filepath.Join(specsInProgressDir, "020-clean-spec.md")
			err := os.WriteFile(
				specFile,
				[]byte("---\nstatus: approved\n---\n# Clean Spec"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			// Unrelated prompt that links to a different spec
			promptFile := filepath.Join(promptsInboxDir, "070-other.md")
			err = os.WriteFile(
				promptFile,
				[]byte("---\nstatus: draft\nspec: [\"999\"]\n---\n# Other Prompt"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			err = specUnapproveCmd.Run(ctx, []string{"020-clean-spec.md"})
			Expect(err).NotTo(HaveOccurred())

			dest := filepath.Join(specsInboxDir, "clean-spec.md")
			_, statErr := os.Stat(dest)
			Expect(statErr).NotTo(HaveOccurred())
		})

		It("unapproves by name without extension", func() {
			specFile := filepath.Join(specsInProgressDir, "030-named-spec.md")
			err := os.WriteFile(
				specFile,
				[]byte("---\nstatus: approved\n---\n# Named Spec"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			err = specUnapproveCmd.Run(ctx, []string{"030-named-spec"})
			Expect(err).NotTo(HaveOccurred())

			dest := filepath.Join(specsInboxDir, "named-spec.md")
			_, statErr := os.Stat(dest)
			Expect(statErr).NotTo(HaveOccurred())
		})
	})
})
