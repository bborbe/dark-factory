// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git_test

import (
	"context"
	"fmt"
	"os/exec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/git"
)

var _ = Describe("PRCreator", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("NewPRCreator", func() {
		It("creates PRCreator without token", func() {
			p := git.NewPRCreator("")
			Expect(p).NotTo(BeNil())
		})

		It("creates PRCreator with token", func() {
			p := git.NewPRCreator("test-token")
			Expect(p).NotTo(BeNil())
		})
	})

	Describe("Create", func() {
		It("returns error when command fails", func() {
			p := git.NewPRCreatorWithCommandOutput("", func(cmd *exec.Cmd) ([]byte, error) {
				return nil, fmt.Errorf("command failed")
			})
			_, err := p.Create(ctx, "Test PR", "Test body")
			Expect(err).To(HaveOccurred())
		})

		It("returns PR URL on success", func() {
			p := git.NewPRCreatorWithCommandOutput("", func(cmd *exec.Cmd) ([]byte, error) {
				return []byte("https://github.com/owner/repo/pull/1\n"), nil
			})
			url, err := p.Create(ctx, "Test PR", "Test body")
			Expect(err).NotTo(HaveOccurred())
			Expect(url).To(Equal("https://github.com/owner/repo/pull/1"))
		})

		It("returns error when title starts with a dash", func() {
			p := git.NewPRCreator("")
			_, err := p.Create(ctx, "--title-injection", "body")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid PR title"))
		})

		It("returns error when title starts with single dash", func() {
			p := git.NewPRCreator("")
			_, err := p.Create(ctx, "-bad-title", "body")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid PR title"))
		})

		It("allows title that does not start with a dash", func() {
			p := git.NewPRCreatorWithCommandOutput("", func(cmd *exec.Cmd) ([]byte, error) {
				return []byte("https://github.com/owner/repo/pull/1\n"), nil
			})
			_, err := p.Create(ctx, "Valid title", "body")
			Expect(err).NotTo(HaveOccurred())
		})

		It("sets GH_TOKEN env when token provided", func() {
			var capturedEnv []string
			p := git.NewPRCreatorWithCommandOutput("my-token", func(cmd *exec.Cmd) ([]byte, error) {
				capturedEnv = cmd.Env
				return []byte("https://github.com/owner/repo/pull/1\n"), nil
			})
			_, err := p.Create(ctx, "Test PR", "body")
			Expect(err).NotTo(HaveOccurred())
			Expect(capturedEnv).To(ContainElement("GH_TOKEN=my-token"))
		})
	})

	Describe("FindOpenPR", func() {
		It("returns empty string when no open PR exists", func() {
			p := git.NewPRCreatorWithCommandOutput("", func(cmd *exec.Cmd) ([]byte, error) {
				return []byte(""), nil
			})
			url, err := p.FindOpenPR(ctx, "feature/nonexistent-branch")
			Expect(err).NotTo(HaveOccurred())
			Expect(url).To(BeEmpty())
		})

		It("returns PR URL when open PR exists", func() {
			p := git.NewPRCreatorWithCommandOutput("", func(cmd *exec.Cmd) ([]byte, error) {
				return []byte("https://github.com/owner/repo/pull/42\n"), nil
			})
			url, err := p.FindOpenPR(ctx, "feature/my-branch")
			Expect(err).NotTo(HaveOccurred())
			Expect(url).To(Equal("https://github.com/owner/repo/pull/42"))
		})

		It("returns error when command fails", func() {
			p := git.NewPRCreatorWithCommandOutput(
				"test-token",
				func(cmd *exec.Cmd) ([]byte, error) {
					return nil, fmt.Errorf("gh auth required")
				},
			)
			_, err := p.FindOpenPR(ctx, "feature/test-branch")
			Expect(err).To(HaveOccurred())
		})

		It("sets GH_TOKEN env when token provided", func() {
			var capturedEnv []string
			p := git.NewPRCreatorWithCommandOutput("my-token", func(cmd *exec.Cmd) ([]byte, error) {
				capturedEnv = cmd.Env
				return []byte(""), nil
			})
			_, err := p.FindOpenPR(ctx, "feature/branch")
			Expect(err).NotTo(HaveOccurred())
			Expect(capturedEnv).To(ContainElement("GH_TOKEN=my-token"))
		})
	})
})
