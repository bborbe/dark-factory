// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package specwatcher_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/specwatcher"
)

var _ = Describe("SpecWatcher", func() {
	var (
		tempDir  string
		specsDir string
		ctx      context.Context
		cancel   context.CancelFunc
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "specwatcher-test-*")
		Expect(err).NotTo(HaveOccurred())

		specsDir = filepath.Join(tempDir, "specs")
		err = os.MkdirAll(specsDir, 0750)
		Expect(err).NotTo(HaveOccurred())

		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() {
		cancel()
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	It(
		"should call generator for approved spec present on startup before any fsnotify events",
		func() {
			gen := &mocks.SpecGenerator{}
			gen.GenerateReturns(nil)

			// Create approved spec BEFORE starting the watcher.
			specFile := filepath.Join(specsDir, "pre-existing-spec.md")
			content := "---\nstatus: approved\n---\n# Pre-existing Spec\n"
			err := os.WriteFile(specFile, []byte(content), 0600)
			Expect(err).NotTo(HaveOccurred())

			w := specwatcher.NewSpecWatcher(specsDir, gen, 200*time.Millisecond)

			go func() {
				_ = w.Watch(ctx)
			}()

			Eventually(func() int {
				return gen.GenerateCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

			_, passedPath := gen.GenerateArgsForCall(0)
			Expect(passedPath).To(Equal(specFile))

			cancel()
		},
	)

	It("should not call generator for draft spec present on startup", func() {
		gen := &mocks.SpecGenerator{}

		specFile := filepath.Join(specsDir, "draft-startup.md")
		content := "---\nstatus: draft\n---\n# Draft Spec\n"
		err := os.WriteFile(specFile, []byte(content), 0600)
		Expect(err).NotTo(HaveOccurred())

		w := specwatcher.NewSpecWatcher(specsDir, gen, 200*time.Millisecond)

		go func() {
			_ = w.Watch(ctx)
		}()

		Consistently(func() int {
			return gen.GenerateCallCount()
		}, 500*time.Millisecond, 50*time.Millisecond).Should(Equal(0))

		cancel()
	})

	It("should ignore non-markdown files in specsDir on startup", func() {
		gen := &mocks.SpecGenerator{}

		txtFile := filepath.Join(specsDir, "readme.txt")
		err := os.WriteFile(txtFile, []byte("hello"), 0600)
		Expect(err).NotTo(HaveOccurred())

		w := specwatcher.NewSpecWatcher(specsDir, gen, 200*time.Millisecond)

		go func() {
			_ = w.Watch(ctx)
		}()

		Consistently(func() int {
			return gen.GenerateCallCount()
		}, 500*time.Millisecond, 50*time.Millisecond).Should(Equal(0))

		cancel()
	})

	It("should start and stop cleanly", func() {
		gen := &mocks.SpecGenerator{}

		w := specwatcher.NewSpecWatcher(specsDir, gen, 500*time.Millisecond)

		errCh := make(chan error, 1)
		go func() {
			errCh <- w.Watch(ctx)
		}()

		time.Sleep(200 * time.Millisecond)
		cancel()

		select {
		case err := <-errCh:
			Expect(err).To(BeNil())
		case <-time.After(2 * time.Second):
			Fail("watcher did not stop within timeout")
		}
	})

	It("should call generator when approved spec is written", func() {
		gen := &mocks.SpecGenerator{}
		gen.GenerateReturns(nil)

		w := specwatcher.NewSpecWatcher(specsDir, gen, 200*time.Millisecond)

		go func() {
			_ = w.Watch(ctx)
		}()

		time.Sleep(100 * time.Millisecond)

		specFile := filepath.Join(specsDir, "my-spec.md")
		content := "---\nstatus: approved\n---\n# My Spec\n"
		err := os.WriteFile(specFile, []byte(content), 0600)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() int {
			return gen.GenerateCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

		_, passedPath := gen.GenerateArgsForCall(0)
		Expect(passedPath).To(Equal(specFile))

		cancel()
	})

	It("should NOT call generator when spec is in draft status", func() {
		gen := &mocks.SpecGenerator{}

		w := specwatcher.NewSpecWatcher(specsDir, gen, 200*time.Millisecond)

		go func() {
			_ = w.Watch(ctx)
		}()

		time.Sleep(100 * time.Millisecond)

		specFile := filepath.Join(specsDir, "draft-spec.md")
		content := "---\nstatus: draft\n---\n# Draft Spec\n"
		err := os.WriteFile(specFile, []byte(content), 0600)
		Expect(err).NotTo(HaveOccurred())

		Consistently(func() int {
			return gen.GenerateCallCount()
		}, 800*time.Millisecond, 50*time.Millisecond).Should(Equal(0))

		cancel()
	})

	It("should NOT call generator when spec is completed", func() {
		gen := &mocks.SpecGenerator{}

		w := specwatcher.NewSpecWatcher(specsDir, gen, 200*time.Millisecond)

		go func() {
			_ = w.Watch(ctx)
		}()

		time.Sleep(100 * time.Millisecond)

		specFile := filepath.Join(specsDir, "completed-spec.md")
		content := "---\nstatus: completed\n---\n# Completed Spec\n"
		err := os.WriteFile(specFile, []byte(content), 0600)
		Expect(err).NotTo(HaveOccurred())

		Consistently(func() int {
			return gen.GenerateCallCount()
		}, 800*time.Millisecond, 50*time.Millisecond).Should(Equal(0))

		cancel()
	})

	It("should log error and continue when generator fails", func() {
		gen := &mocks.SpecGenerator{}
		gen.GenerateReturns(os.ErrPermission)

		w := specwatcher.NewSpecWatcher(specsDir, gen, 200*time.Millisecond)

		errCh := make(chan error, 1)
		go func() {
			errCh <- w.Watch(ctx)
		}()

		time.Sleep(100 * time.Millisecond)

		specFile := filepath.Join(specsDir, "failing-spec.md")
		content := "---\nstatus: approved\n---\n# Failing Spec\n"
		err := os.WriteFile(specFile, []byte(content), 0600)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() int {
			return gen.GenerateCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

		// Watcher should still be running
		select {
		case <-errCh:
			Fail("watcher should not exit on generator error")
		default:
			// Good, still running
		}

		cancel()

		select {
		case err := <-errCh:
			Expect(err).To(BeNil())
		case <-time.After(2 * time.Second):
			Fail("watcher did not exit on context cancel")
		}
	})

	It("should ignore non-markdown files", func() {
		gen := &mocks.SpecGenerator{}

		w := specwatcher.NewSpecWatcher(specsDir, gen, 200*time.Millisecond)

		go func() {
			_ = w.Watch(ctx)
		}()

		time.Sleep(100 * time.Millisecond)

		testFile := filepath.Join(specsDir, "readme.txt")
		err := os.WriteFile(testFile, []byte("Hello"), 0600)
		Expect(err).NotTo(HaveOccurred())

		Consistently(func() int {
			return gen.GenerateCallCount()
		}, 800*time.Millisecond, 100*time.Millisecond).Should(Equal(0))

		cancel()
	})

	It("should debounce rapid writes and call generator once", func() {
		gen := &mocks.SpecGenerator{}
		gen.GenerateReturns(nil)

		w := specwatcher.NewSpecWatcher(specsDir, gen, 400*time.Millisecond)

		go func() {
			_ = w.Watch(ctx)
		}()

		time.Sleep(100 * time.Millisecond)

		specFile := filepath.Join(specsDir, "debounce-spec.md")
		content := "---\nstatus: approved\n---\n# Debounce Spec\n"

		for i := 0; i < 5; i++ {
			err := os.WriteFile(specFile, []byte(content), 0600)
			Expect(err).NotTo(HaveOccurred())
			time.Sleep(50 * time.Millisecond)
		}

		time.Sleep(700 * time.Millisecond)

		Expect(gen.GenerateCallCount()).To(Equal(1))

		cancel()
	})

	It("should work with relative paths", func() {
		origDir, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		defer func() {
			_ = os.Chdir(origDir)
		}()

		err = os.Chdir(tempDir)
		Expect(err).NotTo(HaveOccurred())

		relSpecsDir := "specs-rel"
		err = os.MkdirAll(relSpecsDir, 0750)
		Expect(err).NotTo(HaveOccurred())

		gen := &mocks.SpecGenerator{}
		gen.GenerateReturns(nil)

		w := specwatcher.NewSpecWatcher(relSpecsDir, gen, 200*time.Millisecond)

		go func() {
			_ = w.Watch(ctx)
		}()

		time.Sleep(100 * time.Millisecond)

		absSpecsRelDir := filepath.Join(tempDir, relSpecsDir)
		specFile := filepath.Join(absSpecsRelDir, "rel-spec.md")
		content := "---\nstatus: approved\n---\n# Rel Spec\n"
		err = os.WriteFile(specFile, []byte(content), 0600)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() int {
			return gen.GenerateCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

		cancel()
	})
})
