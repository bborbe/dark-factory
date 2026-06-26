// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package project_test

import (
	"context"
	"time"

	"github.com/bborbe/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/project"
	"github.com/bborbe/dark-factory/pkg/subproc"
)

var _ = Describe("Resolve", func() {
	var runner subproc.Runner

	BeforeEach(func() {
		runner = subproc.NewRunner()
	})

	Context("with config override", func() {
		It("returns the override value without running git", func() {
			ctx := context.Background()
			result, err := project.Resolve(ctx, runner, "my-custom-project")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(project.Name("my-custom-project")))
		})
	})

	Context("without config override", func() {
		It("returns a non-empty string", func() {
			ctx := context.Background()
			result, err := project.Resolve(ctx, runner, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeEmpty())
		})

		It("returns a valid container name prefix", func() {
			ctx := context.Background()
			result, err := project.Resolve(ctx, runner, "")
			Expect(err).NotTo(HaveOccurred())
			// Should not contain invalid Docker name characters
			Expect(result.String()).To(MatchRegexp(`^[a-zA-Z0-9._-]+$`))
		})
	})

	Context("edge cases", func() {
		It("handles empty override by auto-detecting", func() {
			ctx := context.Background()
			result, err := project.Resolve(ctx, runner, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeEmpty())
			Expect(result).NotTo(Equal(project.Name("")))
		})
	})

	Context("with a mock runner", func() {
		It("uses git remote URL when git root returns empty", func() {
			ctx := context.Background()
			fake := &mocks.SubprocRunner{}
			fake.RunWithWarnAndTimeoutStub = func(_ context.Context, op, name string, args ...string) ([]byte, error) {
				if op == "git rev-parse --show-toplevel" {
					return []byte(""), nil
				}
				if op == "git remote get-url origin" {
					return []byte("https://github.com/user/my-repo.git\n"), nil
				}
				return nil, nil
			}
			result, err := project.Resolve(ctx, fake, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(project.Name("my-repo")))
		})

		It("falls back to working directory when both git probes return empty", func() {
			ctx := context.Background()
			fake := &mocks.SubprocRunner{}
			fake.RunWithWarnAndTimeoutStub = func(_ context.Context, op, name string, args ...string) ([]byte, error) {
				return []byte(""), nil
			}
			result, err := project.Resolve(ctx, fake, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeEmpty())
		})

		It("falls through benign git errors to the next fallback", func() {
			ctx := context.Background()
			fake := &mocks.SubprocRunner{}
			callCount := 0
			fake.RunWithWarnAndTimeoutStub = func(innerCtx context.Context, op, name string, args ...string) ([]byte, error) {
				callCount++
				// Return a non-context error — should fall through, not propagate
				return nil, errors.New(innerCtx, "git not found")
			}
			result, err := project.Resolve(ctx, fake, "")
			// Benign git errors: fall through to working-directory fallback
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeEmpty())
			Expect(callCount).To(Equal(2)) // both git probes attempted
		})

		It("strips .git suffix from remote URLs", func() {
			ctx := context.Background()
			fake := &mocks.SubprocRunner{}
			fake.RunWithWarnAndTimeoutStub = func(_ context.Context, op, name string, args ...string) ([]byte, error) {
				if op == "git rev-parse --show-toplevel" {
					return []byte(""), nil
				}
				return []byte("git@github.com:user/dark-factory.git\n"), nil
			}
			result, err := project.Resolve(ctx, fake, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(project.Name("dark-factory")))
		})
	})

	Context("CancelledCtx", func() {
		It("returns quickly with a cancellation error when ctx is already cancelled", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			start := time.Now()
			result, err := project.Resolve(ctx, subproc.NewRunner(), "")
			elapsed := time.Since(start)

			Expect(elapsed).To(BeNumerically("<", 500*time.Millisecond))
			Expect(err).To(HaveOccurred())
			Expect(result).To(Equal(project.Name("")))
			Expect(
				errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded),
			).To(BeTrue())
		})
	})
})
