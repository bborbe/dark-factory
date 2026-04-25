// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package promptresumer_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/project"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/promptresumer"
	"github.com/bborbe/dark-factory/pkg/report"
)

// --- minimal stubs ---

type stubPromptManager struct {
	loadFunc func(ctx context.Context, path string) (*prompt.PromptFile, error)
}

func (s *stubPromptManager) Load(ctx context.Context, path string) (*prompt.PromptFile, error) {
	if s.loadFunc != nil {
		return s.loadFunc(ctx, path)
	}
	return nil, nil //nolint:nilnil
}

type stubWorkflowExecutor struct {
	reconstructStateFunc func(ctx context.Context, baseName prompt.BaseName, pf *prompt.PromptFile) (bool, error)
	completeFunc         func(gitCtx context.Context, ctx context.Context, pf *prompt.PromptFile, title, promptPath, completedPath string) error
	reconstructCallCount int
	completeCallCount    int
}

func (s *stubWorkflowExecutor) ReconstructState(
	ctx context.Context,
	baseName prompt.BaseName,
	pf *prompt.PromptFile,
) (bool, error) {
	s.reconstructCallCount++
	if s.reconstructStateFunc != nil {
		return s.reconstructStateFunc(ctx, baseName, pf)
	}
	return true, nil
}

func (s *stubWorkflowExecutor) Complete(
	gitCtx context.Context,
	ctx context.Context,
	pf *prompt.PromptFile,
	title, promptPath, completedPath string,
) error {
	s.completeCallCount++
	if s.completeFunc != nil {
		return s.completeFunc(gitCtx, ctx, pf, title, promptPath, completedPath)
	}
	return nil
}

type stubExecutor struct {
	reattachCallCount int
	reattachContainer string
	reattachErr       error
	reattachFunc      func(ctx context.Context, logFile string, containerName string, d time.Duration) error
	stopCallCount     int
	stopContainerArg  string
}

func (s *stubExecutor) Execute(_ context.Context, _ string, _ string, _ string) error { return nil }

func (s *stubExecutor) Reattach(
	ctx context.Context,
	logFile string,
	containerName string,
	d time.Duration,
) error {
	s.reattachCallCount++
	s.reattachContainer = containerName
	if s.reattachFunc != nil {
		return s.reattachFunc(ctx, logFile, containerName, d)
	}
	return s.reattachErr
}

func (s *stubExecutor) StopAndRemoveContainer(_ context.Context, containerName string) {
	s.stopCallCount++
	s.stopContainerArg = containerName
}

type stubFailureNotifier struct {
	notifyCallCount int
}

func (s *stubFailureNotifier) NotifyFromReport(_ context.Context, _ string, _ string) {
	s.notifyCallCount++
}

// noOpValidator always succeeds and returns nil report.
type noOpValidator struct{}

func (noOpValidator) Validate(_ context.Context, _ string) (*report.CompletionReport, error) {
	return nil, nil //nolint:nilnil
}

// errValidator always returns an error.
type errValidator struct{}

func (errValidator) Validate(_ context.Context, _ string) (*report.CompletionReport, error) {
	return nil, errSentinel
}

// --- helpers ---

var errSentinel = &sentinelError{"sentinel test error"}

type sentinelError struct{ msg string }

func (e *sentinelError) Error() string { return e.msg }

func newExecutingPromptFile(path, containerName string) *prompt.PromptFile {
	return prompt.NewPromptFile(
		path,
		prompt.Frontmatter{Status: string(prompt.ExecutingPromptStatus), Container: containerName},
		[]byte("# Test\n"),
		libtime.NewCurrentDateTime(),
	)
}

func newApprovedPromptFile(path string) *prompt.PromptFile {
	return prompt.NewPromptFile(
		path,
		prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
		[]byte("# Test\n"),
		libtime.NewCurrentDateTime(),
	)
}

var _ = Describe("Resumer", func() {
	var (
		ctx          context.Context
		tempDir      string
		queueDir     string
		completedDir string
		logDir       string
		fakeExec     *stubExecutor
		mgr          *stubPromptManager
		we           *stubWorkflowExecutor
		notifier     *stubFailureNotifier
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		tempDir, err = os.MkdirTemp("", "promptresumer-test-*")
		Expect(err).NotTo(HaveOccurred())

		queueDir = filepath.Join(tempDir, "in-progress")
		completedDir = filepath.Join(tempDir, "completed")
		logDir = filepath.Join(tempDir, "logs")

		Expect(os.MkdirAll(queueDir, 0750)).To(Succeed())
		Expect(os.MkdirAll(completedDir, 0750)).To(Succeed())
		Expect(os.MkdirAll(logDir, 0750)).To(Succeed())

		fakeExec = &stubExecutor{}
		mgr = &stubPromptManager{}
		we = &stubWorkflowExecutor{}
		notifier = &stubFailureNotifier{}
	})

	AfterEach(func() {
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	newResumer := func(maxDur time.Duration) promptresumer.Resumer {
		return promptresumer.NewResumer(
			mgr, fakeExec, we, noOpValidator{}, notifier,
			queueDir, completedDir, logDir, project.Name("test-project"), maxDur,
		)
	}

	Context("when queueDir does not exist", func() {
		It("returns nil without error", func() {
			r := promptresumer.NewResumer(
				mgr, fakeExec, we, noOpValidator{}, notifier,
				filepath.Join(tempDir, "nonexistent"),
				completedDir, logDir, project.Name("test-project"), 0,
			)
			Expect(r.ResumeAll(ctx)).To(Succeed())
			Expect(fakeExec.reattachCallCount).To(Equal(0))
		})
	})

	Context("when queue is empty", func() {
		It("returns nil without calling Reattach", func() {
			r := newResumer(0)
			Expect(r.ResumeAll(ctx)).To(Succeed())
			Expect(fakeExec.reattachCallCount).To(Equal(0))
		})
	})

	Context("when queue has a non-executing prompt", func() {
		BeforeEach(func() {
			promptPath := filepath.Join(queueDir, "001-approved.md")
			Expect(
				os.WriteFile(promptPath, []byte("---\nstatus: approved\n---\n# Approved\n"), 0600),
			).To(Succeed())
			mgr.loadFunc = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return newApprovedPromptFile(path), nil
			}
		})

		It("skips the prompt without calling Reattach", func() {
			r := newResumer(0)
			Expect(r.ResumeAll(ctx)).To(Succeed())
			Expect(fakeExec.reattachCallCount).To(Equal(0))
		})
	})

	Context("when executing prompt has empty container name", func() {
		BeforeEach(func() {
			promptPath := filepath.Join(queueDir, "001-nocontainer.md")
			Expect(
				os.WriteFile(
					promptPath,
					[]byte("---\nstatus: executing\n---\n# No container\n"),
					0600,
				),
			).To(Succeed())
			mgr.loadFunc = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return prompt.NewPromptFile(
					path,
					prompt.Frontmatter{Status: string(prompt.ExecutingPromptStatus)},
					[]byte("# No container\n"),
					libtime.NewCurrentDateTime(),
				), nil
			}
		})

		It("resets prompt to approved without calling Reattach", func() {
			r := newResumer(0)
			Expect(r.ResumeAll(ctx)).To(Succeed())
			Expect(fakeExec.reattachCallCount).To(Equal(0))
			content, err := os.ReadFile(filepath.Join(queueDir, "001-nocontainer.md"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("status: approved"))
		})
	})

	Context("when Load returns an error", func() {
		BeforeEach(func() {
			promptPath := filepath.Join(queueDir, "001-loaderr.md")
			Expect(
				os.WriteFile(
					promptPath,
					[]byte("---\nstatus: executing\n---\n# Load error\n"),
					0600,
				),
			).To(Succeed())
			mgr.loadFunc = func(_ context.Context, _ string) (*prompt.PromptFile, error) {
				return nil, errSentinel
			}
		})

		It("returns an error wrapping load prompt for resume", func() {
			r := newResumer(0)
			err := r.ResumeAll(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("load prompt for resume"))
		})
	})

	Context("when workflow state cannot be reconstructed", func() {
		BeforeEach(func() {
			promptPath := filepath.Join(queueDir, "001-norecon.md")
			Expect(os.WriteFile(
				promptPath,
				[]byte(
					"---\nstatus: executing\ncontainer: test-project-001-norecon\n---\n# No recon\n",
				),
				0600,
			)).To(Succeed())
			mgr.loadFunc = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return newExecutingPromptFile(path, "test-project-001-norecon"), nil
			}
			we.reconstructStateFunc = func(_ context.Context, _ prompt.BaseName, _ *prompt.PromptFile) (bool, error) {
				return false, nil
			}
		})

		It("resets prompt to approved without calling Reattach", func() {
			r := newResumer(0)
			Expect(r.ResumeAll(ctx)).To(Succeed())
			Expect(fakeExec.reattachCallCount).To(Equal(0))
			content, err := os.ReadFile(filepath.Join(queueDir, "001-norecon.md"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("status: approved"))
		})
	})

	Context("when executing prompt has a container name", func() {
		BeforeEach(func() {
			promptPath := filepath.Join(queueDir, "001-resume.md")
			Expect(os.WriteFile(
				promptPath,
				[]byte(
					"---\nstatus: executing\ncontainer: test-project-001-resume\n---\n# Resume test\n",
				),
				0600,
			)).To(Succeed())
			mgr.loadFunc = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return newExecutingPromptFile(path, "test-project-001-resume"), nil
			}
			fakeExec.reattachErr = errSentinel
		})

		It("calls Reattach with the container name from frontmatter", func() {
			r := newResumer(0)
			err := r.ResumeAll(ctx)
			Expect(err).To(HaveOccurred())
			Expect(fakeExec.reattachCallCount).To(Equal(1))
			Expect(fakeExec.reattachContainer).To(Equal("test-project-001-resume"))
		})
	})

	Context("when container has exceeded maxPromptDuration on reattach", func() {
		var promptPath string

		BeforeEach(func() {
			promptPath = filepath.Join(queueDir, "001-timeout.md")
			started := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
			Expect(os.WriteFile(promptPath, []byte(
				"---\nstatus: executing\ncontainer: test-project-001-timeout\nstarted: "+started+"\n---\n# Timeout\n",
			), 0600)).To(Succeed())
			mgr.loadFunc = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return prompt.NewPromptFile(
					path,
					prompt.Frontmatter{
						Status:    string(prompt.ExecutingPromptStatus),
						Container: "test-project-001-timeout",
						Started:   started,
					},
					[]byte("# Timeout\n"),
					libtime.NewCurrentDateTime(),
				), nil
			}
		})

		It("kills the container and marks the prompt failed without reattaching", func() {
			r := newResumer(time.Hour) // maxDur = 1h, but started 2h ago
			Expect(r.ResumeAll(ctx)).To(Succeed())
			Expect(fakeExec.reattachCallCount).To(Equal(0))
			Expect(fakeExec.stopCallCount).To(Equal(1))
			Expect(fakeExec.stopContainerArg).To(Equal("test-project-001-timeout"))
			content, err := os.ReadFile(promptPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("status: failed"))
		})
	})

	Context("when Reattach succeeds but second Load returns error", func() {
		BeforeEach(func() {
			promptPath := filepath.Join(queueDir, "001-reloadfail.md")
			Expect(os.WriteFile(
				promptPath,
				[]byte(
					"---\nstatus: executing\ncontainer: test-project-reload\n---\n# Reload fail\n",
				),
				0600,
			)).To(Succeed())
			callCount := 0
			mgr.loadFunc = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				callCount++
				if callCount == 1 {
					return newExecutingPromptFile(path, "test-project-reload"), nil
				}
				return nil, errSentinel
			}
		})

		It("returns error wrapping reload prompt after reattach", func() {
			r := newResumer(0)
			err := r.ResumeAll(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("reload prompt after reattach"))
			Expect(fakeExec.reattachCallCount).To(Equal(1))
		})
	})

	Context("when Reattach and second Load succeed but validator fails", func() {
		BeforeEach(func() {
			promptPath := filepath.Join(queueDir, "001-valfail.md")
			Expect(os.WriteFile(
				promptPath,
				[]byte(
					"---\nstatus: executing\ncontainer: test-project-valfail\n---\n# Val fail\n",
				),
				0600,
			)).To(Succeed())
			mgr.loadFunc = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return newExecutingPromptFile(path, "test-project-valfail"), nil
			}
		})

		It("notifies failure and returns error wrapping validate completion report", func() {
			r := promptresumer.NewResumer(
				mgr, fakeExec, we, errValidator{}, notifier,
				queueDir, completedDir, logDir, project.Name("test-project"), 0,
			)
			err := r.ResumeAll(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("validate completion report"))
			Expect(notifier.notifyCallCount).To(Equal(1))
		})
	})

	Context("when full resume succeeds (reattach + completion)", func() {
		BeforeEach(func() {
			promptPath := filepath.Join(queueDir, "001-success.md")
			Expect(os.WriteFile(
				promptPath,
				[]byte(
					"---\nstatus: executing\ncontainer: test-project-001-success\n---\n# Success\n",
				),
				0600,
			)).To(Succeed())
			mgr.loadFunc = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return newExecutingPromptFile(path, "test-project-001-success"), nil
			}
		})

		It("calls Reattach and Complete", func() {
			r := newResumer(0)
			Expect(r.ResumeAll(ctx)).To(Succeed())
			Expect(fakeExec.reattachCallCount).To(Equal(1))
			Expect(we.completeCallCount).To(Equal(1))
		})
	})

	Context("log dir path computation", func() {
		It("derives log file path from logDir and base name", func() {
			var capturedLogFile string
			fakeExec.reattachFunc = func(_ context.Context, logFile string, _ string, _ time.Duration) error {
				capturedLogFile = logFile
				return errSentinel
			}
			promptPath := filepath.Join(queueDir, "042-my-prompt.md")
			Expect(os.WriteFile(
				promptPath,
				[]byte(
					"---\nstatus: executing\ncontainer: test-project-042-my-prompt\n---\n# My prompt\n",
				),
				0600,
			)).To(Succeed())
			mgr.loadFunc = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return newExecutingPromptFile(path, "test-project-042-my-prompt"), nil
			}
			r := newResumer(0)
			_ = r.ResumeAll(ctx)
			Expect(capturedLogFile).To(HaveSuffix("042-my-prompt.log"))
		})
	})
})

var _ = Describe("computeReattachDuration (via ResumeAll timeout path)", func() {
	var (
		ctx      context.Context
		tempDir  string
		queueDir string
		logDir   string
		fakeExec *stubExecutor
		mgr      *stubPromptManager
		we       *stubWorkflowExecutor
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		tempDir, err = os.MkdirTemp("", "reattach-dur-test-*")
		Expect(err).NotTo(HaveOccurred())
		queueDir = filepath.Join(tempDir, "in-progress")
		logDir = filepath.Join(tempDir, "logs")
		Expect(os.MkdirAll(queueDir, 0750)).To(Succeed())
		Expect(os.MkdirAll(logDir, 0750)).To(Succeed())
		fakeExec = &stubExecutor{}
		mgr = &stubPromptManager{}
		we = &stubWorkflowExecutor{}
	})

	AfterEach(func() { _ = os.RemoveAll(tempDir) })

	newResumerWithDur := func(maxDur time.Duration) promptresumer.Resumer {
		return promptresumer.NewResumer(
			mgr, fakeExec, we, noOpValidator{}, &stubFailureNotifier{},
			queueDir, filepath.Join(tempDir, "completed"), logDir,
			project.Name("proj"), maxDur,
		)
	}

	writeExecutingPrompt := func(name, started string) string {
		path := filepath.Join(queueDir, name+".md")
		content := "---\nstatus: executing\ncontainer: proj-" + name
		if started != "" {
			content += "\nstarted: " + started
		}
		content += "\n---\n# Test\n"
		Expect(os.WriteFile(path, []byte(content), 0600)).To(Succeed())
		return path
	}

	Context("when maxPromptDuration is zero", func() {
		It("does not timeout and calls Reattach", func() {
			started := time.Now().Add(-100 * time.Hour).UTC().Format(time.RFC3339)
			name := "001-zero-dur"
			writeExecutingPrompt(name, started)
			mgr.loadFunc = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return prompt.NewPromptFile(
					path,
					prompt.Frontmatter{
						Status:    string(prompt.ExecutingPromptStatus),
						Container: "proj-" + name,
						Started:   started,
					},
					nil,
					libtime.NewCurrentDateTime(),
				), nil
			}
			fakeExec.reattachErr = errSentinel
			r := newResumerWithDur(0) // 0 = unlimited
			_ = r.ResumeAll(ctx)
			Expect(fakeExec.reattachCallCount).To(Equal(1))
			Expect(fakeExec.stopCallCount).To(Equal(0))
		})
	})

	Context("when started is empty", func() {
		It("does not timeout and calls Reattach", func() {
			name := "001-no-started"
			writeExecutingPrompt(name, "")
			mgr.loadFunc = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return prompt.NewPromptFile(
					path,
					prompt.Frontmatter{
						Status:    string(prompt.ExecutingPromptStatus),
						Container: "proj-" + name,
					},
					nil,
					libtime.NewCurrentDateTime(),
				), nil
			}
			fakeExec.reattachErr = errSentinel
			r := newResumerWithDur(time.Hour)
			_ = r.ResumeAll(ctx)
			Expect(fakeExec.reattachCallCount).To(Equal(1))
			Expect(fakeExec.stopCallCount).To(Equal(0))
		})
	})

	Context("when started timestamp is malformed", func() {
		It("uses full timeout and calls Reattach", func() {
			name := "001-bad-ts"
			writeExecutingPrompt(name, "not-a-timestamp")
			mgr.loadFunc = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return prompt.NewPromptFile(
					path,
					prompt.Frontmatter{
						Status:    string(prompt.ExecutingPromptStatus),
						Container: "proj-" + name,
						Started:   "not-a-timestamp",
					},
					nil,
					libtime.NewCurrentDateTime(),
				), nil
			}
			fakeExec.reattachErr = errSentinel
			r := newResumerWithDur(time.Hour)
			_ = r.ResumeAll(ctx)
			Expect(fakeExec.reattachCallCount).To(Equal(1))
			Expect(fakeExec.stopCallCount).To(Equal(0))
		})
	})

	Context("when still within window", func() {
		It("calls Reattach without killing container", func() {
			started := time.Now().Add(-30 * time.Minute).UTC().Format(time.RFC3339)
			name := "001-in-window"
			writeExecutingPrompt(name, started)
			mgr.loadFunc = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return prompt.NewPromptFile(
					path,
					prompt.Frontmatter{
						Status:    string(prompt.ExecutingPromptStatus),
						Container: "proj-" + name,
						Started:   started,
					},
					nil,
					libtime.NewCurrentDateTime(),
				), nil
			}
			fakeExec.reattachErr = errSentinel
			r := newResumerWithDur(time.Hour)
			_ = r.ResumeAll(ctx)
			Expect(fakeExec.reattachCallCount).To(Equal(1))
			Expect(fakeExec.stopCallCount).To(Equal(0))
		})
	})

	Context("when exceeded maxPromptDuration", func() {
		It("kills container without reattaching", func() {
			started := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
			name := "001-exceeded"
			path := writeExecutingPrompt(name, started)
			mgr.loadFunc = func(_ context.Context, p string) (*prompt.PromptFile, error) {
				return prompt.NewPromptFile(
					p,
					prompt.Frontmatter{
						Status:    string(prompt.ExecutingPromptStatus),
						Container: "proj-" + name,
						Started:   started,
					},
					nil,
					libtime.NewCurrentDateTime(),
				), nil
			}
			r := newResumerWithDur(time.Hour)
			Expect(r.ResumeAll(ctx)).To(Succeed())
			Expect(fakeExec.reattachCallCount).To(Equal(0))
			Expect(fakeExec.stopCallCount).To(Equal(1))
			content, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("status: failed"))
		})
	})
})
