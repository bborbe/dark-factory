// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package executor_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/executor"
	"github.com/bborbe/dark-factory/pkg/formatter"
)

var _ = Describe("LocalSubprocessExecutor", func() {
	var (
		ctx                       context.Context
		tempDir                   string
		logFile                   string
		fakeFormatter             *mocks.StreamFormatter
		fakeCurrentDateTimeGetter libtime.CurrentDateTimeGetter
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		tempDir, err = os.MkdirTemp("", "local-executor-test-*")
		Expect(err).NotTo(HaveOccurred())
		logFile = filepath.Join(tempDir, "test.log")
		fakeFormatter = &mocks.StreamFormatter{}
		fakeFormatter.ProcessStreamStub = func(_ context.Context, r io.Reader, _ io.Writer, _ io.Writer) error {
			_, _ = io.Copy(io.Discard, r)
			return nil
		}
		fakeCurrentDateTimeGetter = libtime.NewCurrentDateTime()
	})

	AfterEach(func() {
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	newExecutor := func(model string, maxDuration time.Duration) executor.Executor {
		return executor.NewLocalSubprocessExecutor(
			model,
			maxDuration,
			fakeCurrentDateTimeGetter,
			fakeFormatter,
		)
	}

	Describe("Execute", func() {
		Context("with claude stub on PATH", func() {
			var (
				stubDir  string
				argsFile string
			)

			BeforeEach(func() {
				var err error
				stubDir, err = os.MkdirTemp("", "claude-stub-*")
				Expect(err).NotTo(HaveOccurred())

				argsFile = filepath.Join(stubDir, "captured_args.txt")
				// Pure stub: writes args to a file, emits minimal stream-json, exits.
				// Does NOT call the real claude binary — it may not be in /usr/bin/claude.
				stubScript := "#!/bin/sh\n" +
					"printf '%s\n' \"$@\" > " + argsFile + "\n" +
					"printf '{\"type\":\"result\",\"result\":\"success\"}\\n'\n" +
					"exit 0\n"
				stubPath := filepath.Join(stubDir, "claude")
				err = os.WriteFile(stubPath, []byte(stubScript), 0755)
				Expect(err).NotTo(HaveOccurred())

				oldPath := os.Getenv("PATH")
				// Prepend stub dir so our fake claude is found first.
				Expect(os.Setenv("PATH", stubDir+":"+oldPath)).To(Succeed())
				DeferCleanup(func() { _ = os.Setenv("PATH", oldPath) })
			})

			It("creates both .log and .jsonl files", func() {
				e := newExecutor("claude-sonnet-4-6", 0)
				err := e.Execute(ctx, "# Test prompt\n\nHello.", logFile, "test-exec-id")
				Expect(err).NotTo(HaveOccurred())

				_, err = os.Stat(logFile)
				Expect(err).NotTo(HaveOccurred())

				rawLogFile := executor.RawLogPathForTest(logFile)
				_, err = os.Stat(rawLogFile)
				Expect(err).NotTo(HaveOccurred())
			})

			It("passes production argv flags to claude", func() {
				e := newExecutor("claude-opus-4-8", 0)
				err := e.Execute(ctx, "# Test\n\nHi.", logFile, "test-prod-argv")
				Expect(err).NotTo(HaveOccurred())

				argsContent, err := os.ReadFile(argsFile)
				Expect(err).NotTo(HaveOccurred())
				args := strings.Split(strings.TrimSpace(string(argsContent)), "\n")

				Expect(args).To(ContainElement("--dangerously-skip-permissions"))
				Expect(args).To(ContainElement("--model"))
				modelIdx := -1
				for i, a := range args {
					if a == "--model" && i+1 < len(args) {
						modelIdx = i + 1
						break
					}
				}
				Expect(modelIdx).To(BeNumerically(">=", 0), "--model must have a value")
				Expect(args[modelIdx]).To(Equal("claude-opus-4-8"))
				Expect(args).To(ContainElement("--output-format"))
				outputIdx := -1
				for i, a := range args {
					if a == "--output-format" && i+1 < len(args) {
						outputIdx = i + 1
						break
					}
				}
				Expect(outputIdx).To(BeNumerically(">=", 0))
				Expect(args[outputIdx]).To(Equal("stream-json"))
				Expect(args).To(ContainElement("--verbose"))
				Expect(args).To(ContainElement("--print"))
			})

			It("omits --model when model is empty", func() {
				e := newExecutor("", 0)
				err := e.Execute(ctx, "# Test\n\nHello.", logFile, "test-no-model")
				Expect(err).NotTo(HaveOccurred())

				argsContent, err := os.ReadFile(argsFile)
				Expect(err).NotTo(HaveOccurred())
				argsStr := string(argsContent)
				Expect(argsStr).NotTo(ContainSubstring("--model"))
			})

			It("creates formatted log file with content from formatter", func() {
				// Use the real formatter so the log file is actually written.
				e := executor.NewLocalSubprocessExecutor(
					"claude-sonnet-4-6",
					0,
					fakeCurrentDateTimeGetter,
					formatter.NewFormatter(fakeCurrentDateTimeGetter),
				)
				err := e.Execute(ctx, "# Prompt\n\nTest.", logFile, "test-formatted")
				Expect(err).NotTo(HaveOccurred())

				content, err := os.ReadFile(logFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(content).NotTo(BeEmpty())
			})
		})

		Context("with no claude on PATH", func() {
			BeforeEach(func() {
				// Set PATH to a directory that definitely does not contain claude.
				emptyDir, err := os.MkdirTemp("", "no-claude-*")
				Expect(err).NotTo(HaveOccurred())
				oldPath := os.Getenv("PATH")
				DeferCleanup(func() {
					_ = os.Setenv("PATH", oldPath)
					_ = os.RemoveAll(emptyDir)
				})
				Expect(os.Setenv("PATH", emptyDir)).To(Succeed())
			})

			It("returns an error containing 'claude not found on PATH'", func() {
				e := newExecutor("claude-sonnet-4-6", 0)
				err := e.Execute(ctx, "# Test\n\nHello.", logFile, "test-missing-claude")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("claude not found on PATH"))
			})

			It("returns ErrClaudeNotFound as the cause", func() {
				e := newExecutor("claude-sonnet-4-6", 0)
				err := e.Execute(ctx, "# Test\n\nHello.", logFile, "test-err-is-sentinel")
				Expect(err).To(HaveOccurred())
				Expect(
					executor.IsClaudeNotFound(err),
				).To(BeTrue(), "expected errors.Is(err, executor.ErrClaudeNotFound)")
			})

			It("does not create log file when claude is missing", func() {
				e := newExecutor("claude-sonnet-4-6", 0)
				_ = e.Execute(ctx, "# Test\n\nHello.", logFile, "test-no-log-on-missing-claude")
				_, statErr := os.Stat(logFile)
				Expect(
					os.IsNotExist(statErr),
				).To(BeTrue(), "log file should not be created when claude is absent")
			})
		})
	})

	Describe("Reattach", func() {
		It("returns an error wrapping ErrReattachUnsupported", func() {
			e := newExecutor("claude-sonnet-4-6", 0)
			err := e.Reattach(ctx, logFile, "any-id", 5*time.Minute)
			Expect(err).To(HaveOccurred())
			Expect(
				executor.IsReattachUnsupported(err),
			).To(BeTrue(), "expected errors.Is(err, executor.ErrReattachUnsupported)")
			Expect(err.Error()).To(ContainSubstring("local backend does not support reattach"))
		})

		It("does not create any log files", func() {
			e := newExecutor("claude-sonnet-4-6", 0)
			_ = e.Reattach(ctx, logFile, "any-id", 5*time.Minute)
			_, statErr := os.Stat(logFile)
			Expect(os.IsNotExist(statErr)).To(BeTrue())
		})
	})

	Describe("StopAndRemoveContainer", func() {
		Context("with no subprocess running", func() {
			It("is a no-op and does not panic", func() {
				e := newExecutor("claude-sonnet-4-6", 0)
				// Calling stop with no running subprocess must not panic.
				e.StopAndRemoveContainer(context.Background(), "test-stop-idle")
			})
		})

		Context("with a running subprocess", func() {
			var (
				stubDir       string
				startedFile   string
				completedFile string
			)

			BeforeEach(func() {
				var err error
				stubDir, err = os.MkdirTemp("", "claude-sleep-stub-*")
				Expect(err).NotTo(HaveOccurred())
				startedFile = filepath.Join(stubDir, "started.txt")
				completedFile = filepath.Join(stubDir, "completed.txt")
				// Stub signals start, sleeps long, and only writes the completed
				// sentinel if it runs to natural completion (i.e. was NOT killed).
				stubScript := "#!/bin/sh\n" +
					"echo started > " + startedFile + "\n" +
					"sleep 30\n" +
					"echo done > " + completedFile + "\n" +
					"exit 0\n"
				stubPath := filepath.Join(stubDir, "claude")
				Expect(os.WriteFile(stubPath, []byte(stubScript), 0755)).To(Succeed())
				oldPath := os.Getenv("PATH")
				DeferCleanup(func() {
					_ = os.Setenv("PATH", oldPath)
					_ = os.RemoveAll(stubDir)
				})
				Expect(os.Setenv("PATH", stubDir+":"+oldPath)).To(Succeed())
			})

			It("kills the child process group before its natural exit", func() {
				e := newExecutor("claude-sonnet-4-6", 0)
				done := make(chan error, 1)
				go func() {
					defer GinkgoRecover()
					done <- e.Execute(ctx, "# Test\n\nHello.", logFile, "test-stop-running")
				}()

				// Wait until the subprocess has actually started before stopping it.
				Eventually(func() bool {
					_, statErr := os.Stat(startedFile)
					return statErr == nil
				}, 5*time.Second, 50*time.Millisecond).
					Should(BeTrue(), "stub claude should have started")

				e.StopAndRemoveContainer(context.Background(), "test-stop-running")

				// Execute must return well before the stub's 30s natural sleep.
				Eventually(done, 15*time.Second, 100*time.Millisecond).Should(Receive())

				// The stub was killed, so it never wrote its completion sentinel.
				_, statErr := os.Stat(completedFile)
				Expect(
					os.IsNotExist(statErr),
				).To(BeTrue(), "killed stub must not have run to completion")
			})
		})
	})

	Describe("Execute with timeout", func() {
		Context("with maxPromptDuration set", func() {
			var stubDir string

			BeforeEach(func() {
				var err error
				stubDir, err = os.MkdirTemp("", "claude-timeout-stub-*")
				Expect(err).NotTo(HaveOccurred())

				// Stub that runs quickly and exits.
				stubScript := fmt.Sprintf("#!/bin/sh\n" +
					"printf '{\"type\":\"result\",\"result\":\"success\"}\\n'\n" +
					"exit 0\n")
				stubPath := filepath.Join(stubDir, "claude")
				err = os.WriteFile(stubPath, []byte(stubScript), 0755)
				Expect(err).NotTo(HaveOccurred())

				oldPath := os.Getenv("PATH")
				Expect(os.Setenv("PATH", stubDir+":"+oldPath)).To(Succeed())
				DeferCleanup(func() { _ = os.Setenv("PATH", oldPath) })
			})

			It("completes successfully when duration is sufficient", func() {
				// Use a generous timeout - the stub finishes quickly.
				e := executor.NewLocalSubprocessExecutor(
					"claude-sonnet-4-6",
					10*time.Second,
					fakeCurrentDateTimeGetter,
					fakeFormatter,
				)
				err := e.Execute(ctx, "# Test\n\nHello.", logFile, "test-timeout")
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when the subprocess exceeds the duration", func() {
			var stubDir string

			BeforeEach(func() {
				var err error
				stubDir, err = os.MkdirTemp("", "claude-slow-stub-*")
				Expect(err).NotTo(HaveOccurred())
				// Blocks well past the configured maxPromptDuration.
				stubScript := "#!/bin/sh\nsleep 30\nexit 0\n"
				stubPath := filepath.Join(stubDir, "claude")
				Expect(os.WriteFile(stubPath, []byte(stubScript), 0755)).To(Succeed())
				oldPath := os.Getenv("PATH")
				DeferCleanup(func() {
					_ = os.Setenv("PATH", oldPath)
					_ = os.RemoveAll(stubDir)
				})
				Expect(os.Setenv("PATH", stubDir+":"+oldPath)).To(Succeed())
			})

			It("aborts with an error instead of hanging for the full sleep", func() {
				// The stub sleeps 30s; maxPromptDuration is 300ms. If the timeout
				// path works, Execute returns an error in well under a second.
				// Note: on the deadline, stopProcessGroup SIGTERMs the child, so the
				// command func's "signal: terminated" error can win the
				// CancelOnFirstFinish race over the timeout func's "timed out"
				// string — hence we assert failure, not a specific message.
				e := newExecutor("claude-sonnet-4-6", 300*time.Millisecond)
				err := e.Execute(ctx, "# Test\n\nHello.", logFile, "test-timeout-fires")
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("LocalSubprocessExecutionChecker", func() {
		var checker executor.ExecutionChecker

		BeforeEach(func() {
			checker = executor.NewLocalSubprocessExecutionChecker(fakeCurrentDateTimeGetter)
		})

		Describe("IsRunning", func() {
			It("returns (false, nil) always", func() {
				running, err := checker.IsRunning(ctx, "any-execution-id")
				Expect(err).NotTo(HaveOccurred())
				Expect(running).To(BeFalse())
			})
		})

		Describe("WaitUntilRunning", func() {
			It("returns nil immediately", func() {
				err := checker.WaitUntilRunning(ctx, "any-execution-id", 10*time.Second)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns nil even with zero timeout", func() {
				err := checker.WaitUntilRunning(ctx, "any-execution-id", 0)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Describe("LocalSubprocessExecutionStopper", func() {
		var stopper executor.ExecutionStopper

		BeforeEach(func() {
			stopper = executor.NewLocalSubprocessExecutionStopper()
		})

		Describe("StopContainer", func() {
			It("returns nil (no-op)", func() {
				err := stopper.StopContainer(ctx, "any-execution-id")
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Describe("NoopContainerCounter", func() {
		It("CountRunning returns (0, nil) without invoking docker", func() {
			n, err := executor.NewNoopContainerCounter().CountRunning(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(n).To(Equal(0))
		})
	})
})
