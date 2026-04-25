// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/notifier"
	"github.com/bborbe/dark-factory/pkg/prompt"
)

var _ = Describe("Processor", func() {
	var (
		tempDir       string
		promptsDir    string
		wakeup        chan struct{}
		ctx           context.Context
		cancel        context.CancelFunc
		executor      *mocks.Executor
		manager       *mocks.ProcessorPromptManager
		releaser      *mocks.Releaser
		versionGet    *mocks.VersionGetter
		brancher      *mocks.Brancher
		prCreator     *mocks.PRCreator
		cloner        *mocks.Cloner
		worktreer     *mocks.Worktreer
		prMerger      *mocks.PRMerger
		autoCompleter *mocks.AutoCompleter
		specLister    *mocks.Lister
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "processor-test-*")
		Expect(err).NotTo(HaveOccurred())

		promptsDir = filepath.Join(tempDir, "prompts")
		err = os.MkdirAll(promptsDir, 0750)
		Expect(err).NotTo(HaveOccurred())

		wakeup = make(chan struct{}, 10)
		ctx, cancel = context.WithCancel(context.Background())

		executor = &mocks.Executor{}
		manager = &mocks.ProcessorPromptManager{}
		releaser = &mocks.Releaser{}
		versionGet = &mocks.VersionGetter{}
		brancher = &mocks.Brancher{}
		brancher.CommitsAheadReturns(1, nil)
		prCreator = &mocks.PRCreator{}
		cloner = &mocks.Cloner{}
		worktreer = &mocks.Worktreer{}
		prMerger = &mocks.PRMerger{}
		autoCompleter = &mocks.AutoCompleter{}
		specLister = &mocks.Lister{}
		specLister.ListReturns(nil, nil)
		versionGet.GetReturns("v0.0.1-test")
	})

	AfterEach(func() {
		cancel()
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	BeforeEach(func() {
		manager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
			return newProcessorTestPromptFile(path, "# Test\n\nDefault test content"), nil
		}
	})

	Describe("Completion Report Parsing", func() {
		It("should store summary in frontmatter when report has summary", func() {
			promptPath := filepath.Join(promptsDir, "001-summary-test.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}
			logDir := filepath.Join(tempDir, "log")
			completedDir := filepath.Join(promptsDir, "completed")
			err := os.MkdirAll(logDir, 0750)
			Expect(err).NotTo(HaveOccurred())
			err = os.MkdirAll(completedDir, 0750)
			Expect(err).NotTo(HaveOccurred())

			// Track the PromptFile that gets loaded/saved
			var savedPromptFile *prompt.PromptFile

			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			manager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				// Create a real PromptFile with a temporary backing file
				pf := prompt.NewPromptFile(
					path,
					prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
					[]byte("# Summary test\n\nTest content"),
					libtime.NewCurrentDateTime(),
				)
				savedPromptFile = pf
				return pf, nil
			}
			manager.AllPreviousCompletedReturns(true)
			releaser.CommitCompletedFileReturns(nil)
			releaser.HasChangelogReturns(false)
			releaser.CommitOnlyReturns(nil)

			// Mock executor writes log with success report containing summary
			executor.ExecuteStub = func(_ context.Context, _ string, logFile string, _ string) error {
				logContent := `dark-factory: executing prompt
some output

<!-- DARK-FACTORY-REPORT
{"status":"success","summary":"Added feature successfully","blockers":[]}
DARK-FACTORY-REPORT -->
`
				return os.WriteFile(logFile, []byte(logContent), 0600)
			}

			// Mock MoveToCompleted to verify frontmatter has summary
			manager.MoveToCompletedStub = func(ctx context.Context, path string) error {
				// Verify summary was set before moving
				Expect(savedPromptFile.Frontmatter.Summary).To(Equal("Added feature successfully"))
				return nil
			}

			p := newTestProcessor(
				promptsDir,
				completedDir,
				logDir,
				"test-project",
				executor,
				manager,
				releaser,
				versionGet,
				wakeup,
				false,
				config.WorkflowDirect,
				brancher,
				prCreator,
				cloner,
				worktreer,
				prMerger,
				false,
				false,
				false,
				autoCompleter,
				specLister,
				"",
				"",
				"",
				false,
				notifier.NewMultiNotifier(),
				nil,
				0,
				"",
				nil,
				nil,
				0,
				nil,
				nil,
				0,
				0,
				nil,
			)

			// Run processor in goroutine
			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for processing
			Eventually(func() int {
				return manager.MoveToCompletedCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// Verify summary was stored
			Expect(savedPromptFile.Frontmatter.Summary).To(Equal("Added feature successfully"))

			cancel()
		})

		It("should continue to commit when report status is success", func() {
			promptPath := filepath.Join(promptsDir, "001-report-success.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}
			logDir := filepath.Join(tempDir, "log")
			err := os.MkdirAll(logDir, 0750)
			Expect(err).NotTo(HaveOccurred())

			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			manager.SetStatusReturns(nil)
			manager.MoveToCompletedReturns(nil)
			manager.AllPreviousCompletedReturns(true)
			releaser.CommitCompletedFileReturns(nil)
			releaser.HasChangelogReturns(false)
			releaser.CommitOnlyReturns(nil)

			// Mock executor writes log with success report
			executor.ExecuteStub = func(_ context.Context, _ string, logFile string, _ string) error {
				logContent := `dark-factory: executing prompt
some output

<!-- DARK-FACTORY-REPORT
{"status":"success","summary":"Completed successfully","blockers":[]}
DARK-FACTORY-REPORT -->
`
				return os.WriteFile(logFile, []byte(logContent), 0600)
			}

			p := newTestProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				logDir,
				"test-project",
				executor,
				manager,
				releaser,
				versionGet,
				wakeup,
				false,
				config.WorkflowDirect,
				brancher,
				prCreator,
				cloner,
				worktreer,
				prMerger,
				false,
				false,
				false,
				autoCompleter,
				specLister,
				"",
				"",
				"",
				false,
				notifier.NewMultiNotifier(),
				nil,
				0,
				"",
				nil,
				nil,
				0,
				nil,
				nil,
				0,
				0,
				nil,
			)

			// Run processor in goroutine
			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for processing
			Eventually(func() int {
				return releaser.CommitOnlyCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// Verify moved to completed
			Expect(manager.MoveToCompletedCallCount()).To(Equal(1))

			cancel()
		})

		It("should mark failed and continue when report status is failed", func() {
			promptPath := filepath.Join(promptsDir, "001-report-failed.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}
			logDir := filepath.Join(tempDir, "log")
			err := os.MkdirAll(logDir, 0750)
			Expect(err).NotTo(HaveOccurred())

			// Return queued once, then empty (so loop exits)
			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, nil, nil)
			manager.SetStatusReturns(nil)
			manager.AllPreviousCompletedReturns(true)

			// Mock executor writes log with failed report
			executor.ExecuteStub = func(_ context.Context, _ string, logFile string, _ string) error {
				logContent := `dark-factory: executing prompt
some output

<!-- DARK-FACTORY-REPORT
{"status":"failed","summary":"Could not complete","blockers":["tests failed","lint errors"]}
DARK-FACTORY-REPORT -->
`
				return os.WriteFile(logFile, []byte(logContent), 0600)
			}

			p := newTestProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				logDir,
				"test-project",
				executor,
				manager,
				releaser,
				versionGet,
				wakeup,
				false,
				config.WorkflowDirect,
				brancher,
				prCreator,
				cloner,
				worktreer,
				prMerger,
				false,
				false,
				false,
				autoCompleter,
				specLister,
				"",
				"",
				"",
				false,
				notifier.NewMultiNotifier(),
				nil,
				0,
				"",
				nil,
				nil,
				0,
				nil,
				nil,
				0,
				0,
				nil,
			)

			// Run processor — should not return error (continues after failure)
			errCh := make(chan error, 1)
			go func() {
				errCh <- p.Process(ctx)
			}()

			// Wait for processing to complete
			Eventually(func() int {
				return manager.LoadCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

			// Verify moved to completed was NOT called
			Expect(manager.MoveToCompletedCallCount()).To(Equal(0))

			cancel()
		})

		It("should mark failed and continue when report status is partial", func() {
			promptPath := filepath.Join(promptsDir, "001-report-partial.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}
			logDir := filepath.Join(tempDir, "log")
			err := os.MkdirAll(logDir, 0750)
			Expect(err).NotTo(HaveOccurred())

			// Return queued once, then empty (so loop exits)
			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, nil, nil)
			manager.SetStatusReturns(nil)
			manager.AllPreviousCompletedReturns(true)

			// Mock executor writes log with partial report
			executor.ExecuteStub = func(_ context.Context, _ string, logFile string, _ string) error {
				logContent := `dark-factory: executing prompt
some output

<!-- DARK-FACTORY-REPORT
{"status":"partial","summary":"Half done","blockers":["make precommit fails"]}
DARK-FACTORY-REPORT -->
`
				return os.WriteFile(logFile, []byte(logContent), 0600)
			}

			p := newTestProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				logDir,
				"test-project",
				executor,
				manager,
				releaser,
				versionGet,
				wakeup,
				false,
				config.WorkflowDirect,
				brancher,
				prCreator,
				cloner,
				worktreer,
				prMerger,
				false,
				false,
				false,
				autoCompleter,
				specLister,
				"",
				"",
				"",
				false,
				notifier.NewMultiNotifier(),
				nil,
				0,
				"",
				nil,
				nil,
				0,
				nil,
				nil,
				0,
				0,
				nil,
			)

			// Run processor — should not return error (continues after failure)
			errCh := make(chan error, 1)
			go func() {
				errCh <- p.Process(ctx)
			}()

			// Wait for processing to complete
			Eventually(func() int {
				return manager.LoadCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

			// Verify moved to completed was NOT called
			Expect(manager.MoveToCompletedCallCount()).To(Equal(0))

			cancel()
		})

		It("should fire prompt_partial notification when report status is partial", func() {
			promptPath := filepath.Join(promptsDir, "001-report-partial-notify.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}
			logDir := filepath.Join(tempDir, "log-notify")
			err := os.MkdirAll(logDir, 0750)
			Expect(err).NotTo(HaveOccurred())

			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, nil, nil)
			manager.SetStatusReturns(nil)
			manager.AllPreviousCompletedReturns(true)

			executor.ExecuteStub = func(_ context.Context, _ string, logFile string, _ string) error {
				logContent := `dark-factory: executing prompt
some output

<!-- DARK-FACTORY-REPORT
{"status":"partial","summary":"Half done","blockers":["lint fails"]}
DARK-FACTORY-REPORT -->
`
				return os.WriteFile(logFile, []byte(logContent), 0600)
			}

			fakeNotifier := &mocks.Notifier{}

			p := newTestProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				logDir,
				"test-project",
				executor,
				manager,
				releaser,
				versionGet,
				wakeup,
				false,
				config.WorkflowDirect,
				brancher,
				prCreator,
				cloner,
				worktreer,
				prMerger,
				false,
				false,
				false,
				autoCompleter,
				specLister,
				"",
				"",
				"",
				false,
				fakeNotifier,
				nil,
				0,
				"",
				nil,
				nil,
				0,
				nil,
				nil,
				0,
				0,
				nil,
			)

			go func() {
				_ = p.Process(ctx)
			}()

			Eventually(func() int {
				return fakeNotifier.NotifyCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))
			cancel()

			_, event := fakeNotifier.NotifyArgsForCall(0)
			Expect(event.EventType).To(Equal("prompt_partial"))
			Expect(event.ProjectName).To(Equal("test-project"))
			Expect(event.PromptName).To(Equal("001-report-partial-notify.md"))
		})

		It("should continue when no report found (backwards compatible)", func() {
			promptPath := filepath.Join(promptsDir, "001-no-report.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}
			logDir := filepath.Join(tempDir, "log")
			err := os.MkdirAll(logDir, 0750)
			Expect(err).NotTo(HaveOccurred())

			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			manager.SetStatusReturns(nil)
			manager.MoveToCompletedReturns(nil)
			manager.AllPreviousCompletedReturns(true)
			releaser.CommitCompletedFileReturns(nil)
			releaser.HasChangelogReturns(false)
			releaser.CommitOnlyReturns(nil)

			// Mock executor writes log WITHOUT report (old-style prompt)
			executor.ExecuteStub = func(_ context.Context, _ string, logFile string, _ string) error {
				logContent := `dark-factory: executing prompt
some output
more output
`
				return os.WriteFile(logFile, []byte(logContent), 0600)
			}

			p := newTestProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				logDir,
				"test-project",
				executor,
				manager,
				releaser,
				versionGet,
				wakeup,
				false,
				config.WorkflowDirect,
				brancher,
				prCreator,
				cloner,
				worktreer,
				prMerger,
				false,
				false,
				false,
				autoCompleter,
				specLister,
				"",
				"",
				"",
				false,
				notifier.NewMultiNotifier(),
				nil,
				0,
				"",
				nil,
				nil,
				0,
				nil,
				nil,
				0,
				0,
				nil,
			)

			// Run processor in goroutine
			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for processing
			Eventually(func() int {
				return releaser.CommitOnlyCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// Verify moved to completed
			Expect(manager.MoveToCompletedCallCount()).To(Equal(1))

			cancel()
		})

		It("should override success to partial when verification exitCode is non-zero", func() {
			promptPath := filepath.Join(promptsDir, "001-verification-override.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}
			logDir := filepath.Join(tempDir, "log")
			err := os.MkdirAll(logDir, 0750)
			Expect(err).NotTo(HaveOccurred())

			// Return queued once, then empty (so loop exits)
			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, nil, nil)
			manager.SetStatusReturns(nil)
			manager.AllPreviousCompletedReturns(true)

			// Mock executor writes log with success report but non-zero verification exit code
			executor.ExecuteStub = func(_ context.Context, _ string, logFile string, _ string) error {
				logContent := `dark-factory: executing prompt
some output

<!-- DARK-FACTORY-REPORT
{"status":"success","summary":"Task completed","blockers":[],"verification":{"command":"make precommit","exitCode":1}}
DARK-FACTORY-REPORT -->
`
				return os.WriteFile(logFile, []byte(logContent), 0600)
			}

			p := newTestProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				logDir,
				"test-project",
				executor,
				manager,
				releaser,
				versionGet,
				wakeup,
				false,
				config.WorkflowDirect,
				brancher,
				prCreator,
				cloner,
				worktreer,
				prMerger,
				false,
				false,
				false,
				autoCompleter,
				specLister,
				"",
				"",
				"",
				false,
				notifier.NewMultiNotifier(),
				nil,
				0,
				"",
				nil,
				nil,
				0,
				nil,
				nil,
				0,
				0,
				nil,
			)

			// Run processor — should not return error (continues after failure)
			errCh := make(chan error, 1)
			go func() {
				errCh <- p.Process(ctx)
			}()

			// Wait for processing to complete
			Eventually(func() int {
				return manager.LoadCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

			// Verify moved to completed was NOT called (status was overridden to partial)
			Expect(manager.MoveToCompletedCallCount()).To(Equal(0))

			cancel()
		})

		It("should continue when report parsing fails (graceful degradation)", func() {
			promptPath := filepath.Join(promptsDir, "001-malformed-report.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}
			logDir := filepath.Join(tempDir, "log")
			err := os.MkdirAll(logDir, 0750)
			Expect(err).NotTo(HaveOccurred())

			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			manager.SetStatusReturns(nil)
			manager.MoveToCompletedReturns(nil)
			manager.AllPreviousCompletedReturns(true)
			releaser.CommitCompletedFileReturns(nil)
			releaser.HasChangelogReturns(false)
			releaser.CommitOnlyReturns(nil)

			// Mock executor writes log with malformed JSON
			executor.ExecuteStub = func(_ context.Context, _ string, logFile string, _ string) error {
				logContent := `dark-factory: executing prompt
some output

<!-- DARK-FACTORY-REPORT
{this is not valid json}
DARK-FACTORY-REPORT -->
`
				return os.WriteFile(logFile, []byte(logContent), 0600)
			}

			p := newTestProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				logDir,
				"test-project",
				executor,
				manager,
				releaser,
				versionGet,
				wakeup,
				false,
				config.WorkflowDirect,
				brancher,
				prCreator,
				cloner,
				worktreer,
				prMerger,
				false,
				false,
				false,
				autoCompleter,
				specLister,
				"",
				"",
				"",
				false,
				notifier.NewMultiNotifier(),
				nil,
				0,
				"",
				nil,
				nil,
				0,
				nil,
				nil,
				0,
				0,
				nil,
			)

			// Run processor in goroutine
			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for processing - should continue despite malformed JSON
			Eventually(func() int {
				return releaser.CommitOnlyCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// Verify moved to completed
			Expect(manager.MoveToCompletedCallCount()).To(Equal(1))

			cancel()
		})
	})

})
