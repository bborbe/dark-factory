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

var _ = Describe("SpecApproveCommand", func() {
	var (
		tempDir        string
		specsDir       string
		inProgressDir  string
		completedDir   string
		specApproveCmd cmd.SpecApproveCommand
		ctx            context.Context
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "spec-approve-test-*")
		Expect(err).NotTo(HaveOccurred())

		specsDir = filepath.Join(tempDir, "specs")
		err = os.MkdirAll(specsDir, 0750)
		Expect(err).NotTo(HaveOccurred())

		inProgressDir = filepath.Join(tempDir, "specs", "in-progress")
		completedDir = filepath.Join(tempDir, "specs", "completed")

		specApproveCmd = cmd.NewSpecApproveCommand(specsDir, inProgressDir, completedDir)
		ctx = context.Background()
	})

	AfterEach(func() {
		_ = os.RemoveAll(tempDir)
	})

	Describe("Run", func() {
		It("returns error when no identifier given", func() {
			err := specApproveCmd.Run(ctx, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec identifier required"))
		})

		It("approves a spec by exact filename and moves it to inProgressDir", func() {
			specFile := filepath.Join(specsDir, "001-my-spec.md")
			err := os.WriteFile(specFile, []byte("---\nstatus: draft\n---\n# My Spec"), 0600)
			Expect(err).NotTo(HaveOccurred())

			err = specApproveCmd.Run(ctx, []string{"001-my-spec.md"})
			Expect(err).NotTo(HaveOccurred())

			// File should be gone from inboxDir
			_, statErr := os.Stat(specFile)
			Expect(os.IsNotExist(statErr)).To(BeTrue())

			// File should be present in inProgressDir with approved status
			dest := filepath.Join(inProgressDir, "001-my-spec.md")
			content, err := os.ReadFile(dest)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("status: approved"))
		})

		It("approves a spec by name without extension", func() {
			specFile := filepath.Join(specsDir, "001-my-spec.md")
			err := os.WriteFile(specFile, []byte("---\nstatus: draft\n---\n# My Spec"), 0600)
			Expect(err).NotTo(HaveOccurred())

			err = specApproveCmd.Run(ctx, []string{"001-my-spec"})
			Expect(err).NotTo(HaveOccurred())

			// File should be moved to inProgressDir
			dest := filepath.Join(inProgressDir, "001-my-spec.md")
			content, err := os.ReadFile(dest)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("status: approved"))
		})

		It("approves a spec by numeric prefix", func() {
			specFile := filepath.Join(specsDir, "001-my-spec.md")
			err := os.WriteFile(specFile, []byte("---\nstatus: draft\n---\n# My Spec"), 0600)
			Expect(err).NotTo(HaveOccurred())

			err = specApproveCmd.Run(ctx, []string{"001"})
			Expect(err).NotTo(HaveOccurred())

			// File should be moved to inProgressDir
			dest := filepath.Join(inProgressDir, "001-my-spec.md")
			content, err := os.ReadFile(dest)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("status: approved"))
		})

		It("returns error when spec not found", func() {
			err := specApproveCmd.Run(ctx, []string{"999-nonexistent.md"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec not found"))
		})

		It("returns error when spec is already approved", func() {
			specFile := filepath.Join(specsDir, "001-my-spec.md")
			err := os.WriteFile(specFile, []byte("---\nstatus: approved\n---\n# My Spec"), 0600)
			Expect(err).NotTo(HaveOccurred())

			err = specApproveCmd.Run(ctx, []string{"001-my-spec.md"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already approved"))
		})

		It("returns error when specs dir does not exist", func() {
			specApproveCmd = cmd.NewSpecApproveCommand(
				"/nonexistent/specs",
				"/nonexistent/specs/in-progress",
				"/nonexistent/specs/completed",
			)
			err := specApproveCmd.Run(ctx, []string{"001"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec not found"))
		})

		It("creates inProgressDir if it does not exist", func() {
			specFile := filepath.Join(specsDir, "002-new-spec.md")
			err := os.WriteFile(specFile, []byte("---\nstatus: draft\n---\n# New Spec"), 0600)
			Expect(err).NotTo(HaveOccurred())

			err = specApproveCmd.Run(ctx, []string{"002-new-spec.md"})
			Expect(err).NotTo(HaveOccurred())

			dest := filepath.Join(inProgressDir, "002-new-spec.md")
			_, statErr := os.Stat(dest)
			Expect(statErr).NotTo(HaveOccurred())
		})

		It("assigns a numeric prefix to unnumbered spec on approve", func() {
			// Existing numbered specs in in-progress and completed
			err := os.MkdirAll(inProgressDir, 0750)
			Expect(err).NotTo(HaveOccurred())
			err = os.MkdirAll(completedDir, 0750)
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(
				filepath.Join(inProgressDir, "003-existing.md"),
				[]byte("---\nstatus: approved\n---\n"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(
				filepath.Join(completedDir, "005-done.md"),
				[]byte("---\nstatus: completed\n---\n"),
				0600,
			)
			Expect(err).NotTo(HaveOccurred())

			// Unnumbered spec in inbox
			specFile := filepath.Join(specsDir, "my-new-spec.md")
			err = os.WriteFile(specFile, []byte("---\nstatus: draft\n---\n# My New Spec"), 0600)
			Expect(err).NotTo(HaveOccurred())

			err = specApproveCmd.Run(ctx, []string{"my-new-spec.md"})
			Expect(err).NotTo(HaveOccurred())

			// Should be placed in in-progress with next number after 5
			dest := filepath.Join(inProgressDir, "006-my-new-spec.md")
			content, readErr := os.ReadFile(dest)
			Expect(readErr).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("status: approved"))
		})

		It("preserves existing numeric prefix when approving already-numbered spec", func() {
			specFile := filepath.Join(specsDir, "010-numbered-spec.md")
			err := os.WriteFile(specFile, []byte("---\nstatus: draft\n---\n# Numbered Spec"), 0600)
			Expect(err).NotTo(HaveOccurred())

			err = specApproveCmd.Run(ctx, []string{"010-numbered-spec.md"})
			Expect(err).NotTo(HaveOccurred())

			dest := filepath.Join(inProgressDir, "010-numbered-spec.md")
			_, statErr := os.Stat(dest)
			Expect(statErr).NotTo(HaveOccurred())
		})
	})
})
