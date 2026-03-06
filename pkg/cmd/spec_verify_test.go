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
		inboxDir      string
		inProgressDir string
		completedDir  string
		specVerifyCmd cmd.SpecVerifyCommand
		ctx           context.Context
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "spec-verify-test-*")
		Expect(err).NotTo(HaveOccurred())

		inboxDir = filepath.Join(tempDir, "inbox")
		inProgressDir = filepath.Join(tempDir, "in-progress")
		completedDir = filepath.Join(tempDir, "completed")

		err = os.MkdirAll(inboxDir, 0750)
		Expect(err).NotTo(HaveOccurred())
		err = os.MkdirAll(inProgressDir, 0750)
		Expect(err).NotTo(HaveOccurred())
		// completedDir created on demand by the command

		specVerifyCmd = cmd.NewSpecVerifyCommand(inboxDir, inProgressDir, completedDir)
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

		It("verifies a spec in verifying state and moves it to completedDir", func() {
			specFile := filepath.Join(inProgressDir, "001-my-spec.md")
			err := os.WriteFile(specFile, []byte("---\nstatus: verifying\n---\n# My Spec"), 0600)
			Expect(err).NotTo(HaveOccurred())

			err = specVerifyCmd.Run(ctx, []string{"001-my-spec.md"})
			Expect(err).NotTo(HaveOccurred())

			// File should now be in completedDir
			dest := filepath.Join(completedDir, "001-my-spec.md")
			content, err := os.ReadFile(dest)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("status: completed"))
		})

		It("removes file from inProgressDir after verify", func() {
			specFile := filepath.Join(inProgressDir, "001-my-spec.md")
			err := os.WriteFile(specFile, []byte("---\nstatus: verifying\n---\n# My Spec"), 0600)
			Expect(err).NotTo(HaveOccurred())

			err = specVerifyCmd.Run(ctx, []string{"001-my-spec.md"})
			Expect(err).NotTo(HaveOccurred())

			// File should no longer exist in inProgressDir
			_, statErr := os.Stat(specFile)
			Expect(os.IsNotExist(statErr)).To(BeTrue())
		})

		It("returns error when spec is in draft state", func() {
			specFile := filepath.Join(inProgressDir, "001-my-spec.md")
			err := os.WriteFile(specFile, []byte("---\nstatus: draft\n---\n# My Spec"), 0600)
			Expect(err).NotTo(HaveOccurred())

			err = specVerifyCmd.Run(ctx, []string{"001-my-spec.md"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not in verifying state"))
		})

		It("returns error when spec is in approved state", func() {
			specFile := filepath.Join(inProgressDir, "001-my-spec.md")
			err := os.WriteFile(specFile, []byte("---\nstatus: approved\n---\n# My Spec"), 0600)
			Expect(err).NotTo(HaveOccurred())

			err = specVerifyCmd.Run(ctx, []string{"001-my-spec.md"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not in verifying state"))
		})

		It("returns error when spec is in completed state", func() {
			specFile := filepath.Join(inProgressDir, "001-my-spec.md")
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

		It("returns error when all spec dirs do not exist", func() {
			specVerifyCmd = cmd.NewSpecVerifyCommand(
				"/nonexistent/inbox",
				"/nonexistent/in-progress",
				"/nonexistent/completed",
			)
			err := specVerifyCmd.Run(ctx, []string{"001"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec not found"))
		})

		It("finds spec in inboxDir if not in inProgressDir", func() {
			specFile := filepath.Join(inboxDir, "002-inbox-spec.md")
			err := os.WriteFile(specFile, []byte("---\nstatus: verifying\n---\n# Inbox Spec"), 0600)
			Expect(err).NotTo(HaveOccurred())

			err = specVerifyCmd.Run(ctx, []string{"002-inbox-spec.md"})
			Expect(err).NotTo(HaveOccurred())

			dest := filepath.Join(completedDir, "002-inbox-spec.md")
			content, err := os.ReadFile(dest)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("status: completed"))
		})
	})
})
