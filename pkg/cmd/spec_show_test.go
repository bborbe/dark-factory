// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/cmd"
)

var _ = Describe("SpecShowCommand", func() {
	var (
		tempDir       string
		inboxDir      string
		inProgressDir string
		completedDir  string
		mockCounter   *mocks.PromptCounter
		specShowCmd   cmd.SpecShowCommand
		ctx           context.Context
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "spec-show-test-*")
		Expect(err).NotTo(HaveOccurred())

		inboxDir = filepath.Join(tempDir, "inbox")
		inProgressDir = filepath.Join(tempDir, "in-progress")
		completedDir = filepath.Join(tempDir, "completed")

		Expect(os.MkdirAll(inboxDir, 0750)).To(Succeed())
		Expect(os.MkdirAll(inProgressDir, 0750)).To(Succeed())
		Expect(os.MkdirAll(completedDir, 0750)).To(Succeed())

		mockCounter = &mocks.PromptCounter{}
		mockCounter.CountBySpecReturns(2, 5, nil)

		specShowCmd = cmd.NewSpecShowCommand(
			inboxDir,
			inProgressDir,
			completedDir,
			mockCounter,
			libtime.NewCurrentDateTime(),
		)
		ctx = context.Background()
	})

	AfterEach(func() {
		_ = os.RemoveAll(tempDir)
	})

	Describe("Run", func() {
		It("returns error when no identifier given", func() {
			err := specShowCmd.Run(ctx, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec identifier required"))
		})

		It("returns error when spec not found", func() {
			err := specShowCmd.Run(ctx, []string{"999-nonexistent.md"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec not found"))
		})

		It("shows spec details for a valid ID", func() {
			specFile := filepath.Join(inProgressDir, "001-my-spec.md")
			Expect(
				os.WriteFile(specFile, []byte("---\nstatus: approved\n---\n# My Spec"), 0600),
			).To(Succeed())

			err := specShowCmd.Run(ctx, []string{"001-my-spec.md"})
			Expect(err).NotTo(HaveOccurred())
			Expect(mockCounter.CountBySpecCallCount()).To(Equal(1))
		})

		It("shows spec from inboxDir", func() {
			specFile := filepath.Join(inboxDir, "002-inbox-spec.md")
			Expect(
				os.WriteFile(specFile, []byte("---\nstatus: draft\n---\n# Inbox Spec"), 0600),
			).To(Succeed())

			err := specShowCmd.Run(ctx, []string{"002"})
			Expect(err).NotTo(HaveOccurred())
		})

		It("outputs JSON when --json flag is given", func() {
			specFile := filepath.Join(completedDir, "003-done-spec.md")
			Expect(
				os.WriteFile(
					specFile,
					[]byte("---\nstatus: completed\ncompleted: 2026-01-01T00:00:00Z\n---\n# Done"),
					0600,
				),
			).To(Succeed())

			err := specShowCmd.Run(ctx, []string{"--json", "003"})
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns error when counter fails", func() {
			specFile := filepath.Join(inProgressDir, "004-my-spec.md")
			Expect(
				os.WriteFile(specFile, []byte("---\nstatus: verifying\n---\n# My Spec"), 0600),
			).To(Succeed())

			mockCounter.CountBySpecReturns(0, 0, errors.New("counter failure"))
			err := specShowCmd.Run(ctx, []string{"004"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("count prompts for spec"))
		})
	})
})
