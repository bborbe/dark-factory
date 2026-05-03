// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package project_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/project"
)

var _ = Describe("FindRoot", func() {
	var ctx context.Context
	var origDir string

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		origDir, err = os.Getwd()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		// Always restore original working directory.
		Expect(os.Chdir(origDir)).To(Succeed())
	})

	It(
		"returns the directory containing .dark-factory.yaml when called from that directory",
		func() {
			projectDir, err := os.MkdirTemp("", "df-root-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(projectDir) }()

			// On macOS, os.MkdirTemp returns /var/folders/... but os.Getwd after
			// os.Chdir resolves the symlink to /private/var/folders/...
			// Resolve once for stable comparison.
			resolvedProjectDir, err := filepath.EvalSymlinks(projectDir)
			Expect(err).NotTo(HaveOccurred())

			Expect(
				os.WriteFile(
					filepath.Join(projectDir, ".dark-factory.yaml"),
					[]byte("workflow: direct\n"),
					0600,
				),
			).To(Succeed())
			Expect(os.Chdir(projectDir)).To(Succeed())

			result, err := project.FindRoot(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(resolvedProjectDir))
		},
	)

	It("walks up to find .dark-factory.yaml in a parent directory", func() {
		projectDir, err := os.MkdirTemp("", "df-root-test-*")
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = os.RemoveAll(projectDir) }()

		resolvedProjectDir, err := filepath.EvalSymlinks(projectDir)
		Expect(err).NotTo(HaveOccurred())

		subDir := filepath.Join(projectDir, "pkg", "config")
		Expect(os.MkdirAll(subDir, 0750)).To(Succeed())
		Expect(
			os.WriteFile(
				filepath.Join(projectDir, ".dark-factory.yaml"),
				[]byte("workflow: direct\n"),
				0600,
			),
		).To(Succeed())
		Expect(os.Chdir(subDir)).To(Succeed())

		result, err := project.FindRoot(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(resolvedProjectDir))
	})

	It("returns error when no .dark-factory.yaml is found anywhere up to $HOME", func() {
		// Use a temp dir under /tmp — no .dark-factory.yaml should exist there.
		tmpDir, err := os.MkdirTemp("", "df-no-project-*")
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = os.RemoveAll(tmpDir) }()

		// Ensure there is no .dark-factory.yaml in tmpDir or its parents up to $HOME.
		// (os.MkdirTemp creates under os.TempDir() which is typically /tmp — no .dark-factory.yaml)
		Expect(os.Chdir(tmpDir)).To(Succeed())

		// Verify assumption: $HOME does not contain .dark-factory.yaml
		home, homeErr := os.UserHomeDir()
		Expect(homeErr).NotTo(HaveOccurred())
		if _, statErr := os.Stat(filepath.Join(home, ".dark-factory.yaml")); statErr == nil {
			Skip("$HOME contains .dark-factory.yaml — test cannot run in this environment")
		}

		_, err = project.FindRoot(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not a dark-factory project"))
		Expect(err.Error()).To(ContainSubstring(".dark-factory.yaml"))
	})
})
