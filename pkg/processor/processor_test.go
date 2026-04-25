// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor_test

import (
	"context"
	stderrors "errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/cancellationwatcher"
	"github.com/bborbe/dark-factory/pkg/committingrecoverer"
	"github.com/bborbe/dark-factory/pkg/completionreport"
	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/containerlock"
	"github.com/bborbe/dark-factory/pkg/containerslot"
	"github.com/bborbe/dark-factory/pkg/executor"
	"github.com/bborbe/dark-factory/pkg/failurehandler"
	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/notifier"
	"github.com/bborbe/dark-factory/pkg/preflight"
	"github.com/bborbe/dark-factory/pkg/preflightconditions"
	"github.com/bborbe/dark-factory/pkg/processor"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/promptenricher"
	"github.com/bborbe/dark-factory/pkg/promptresumer"
	"github.com/bborbe/dark-factory/pkg/queuescanner"
	"github.com/bborbe/dark-factory/pkg/report"
	"github.com/bborbe/dark-factory/pkg/spec"
	"github.com/bborbe/dark-factory/pkg/specsweeper"
	"github.com/bborbe/dark-factory/pkg/validationprompt"
)

// newTestWorkflowExecutor builds a WorkflowExecutor from the given mocks and workflow type.
func newTestWorkflowExecutor(
	workflow config.Workflow, pr, autoMerge, autoRelease, autoReview bool,
	projectName string, mgr *mocks.ProcessorPromptManager, rel *mocks.Releaser,
	autoCompleter spec.AutoCompleter, brancher *mocks.Brancher, prCreator *mocks.PRCreator,
	cloner *mocks.Cloner, worktreer *mocks.Worktreer, prMerger *mocks.PRMerger,
) processor.WorkflowExecutor {
	deps := processor.WorkflowDeps{
		ProjectName: processor.ProjectName(projectName), PromptManager: mgr,
		AutoCompleter: autoCompleter, Releaser: rel, Brancher: brancher,
		PRCreator: prCreator, Cloner: cloner, Worktreer: worktreer, PRMerger: prMerger,
		PR: pr, AutoMerge: autoMerge, AutoReview: autoReview, AutoRelease: autoRelease,
	}
	switch workflow {
	case config.WorkflowClone:
		return processor.NewCloneWorkflowExecutor(deps)
	case config.WorkflowWorktree:
		return processor.NewWorktreeWorkflowExecutor(deps)
	case config.WorkflowBranch:
		return processor.NewBranchWorkflowExecutor(deps)
	default:
		return processor.NewDirectWorkflowExecutor(deps)
	}
}

// newTestProcessor creates a Processor using the legacy parameter style, building
// a real WorkflowExecutor from the supplied git mocks. This keeps existing tests
// working after the WorkflowExecutor refactoring without rewriting every call site.
func newTestProcessor(
	queueDir, completedDir, logDir, projectName string,
	exec *mocks.Executor, mgr *mocks.ProcessorPromptManager, rel *mocks.Releaser,
	vg *mocks.VersionGetter, wakeup <-chan struct{}, pr bool, workflow config.Workflow,
	brancher *mocks.Brancher, prCreator *mocks.PRCreator, cloner *mocks.Cloner,
	worktreer *mocks.Worktreer, prMerger *mocks.PRMerger,
	autoMerge, autoRelease, autoReview bool,
	autoCompleter spec.AutoCompleter, specLister spec.Lister,
	validationCommand, validationPrompt, testCommand string,
	verificationGate bool, n notifier.Notifier,
	containerCounter executor.ContainerCounter, maxContainers int,
	additionalInstructions string, containerLock containerlock.ContainerLock,
	containerChecker executor.ContainerChecker, dirtyFileThreshold int,
	dirtyFileChecker processor.DirtyFileChecker, gitLockChecker processor.GitLockChecker,
	autoRetryLimit int, maxPromptDuration time.Duration, preflightChecker preflight.Checker,
) processor.Processor {
	we := newTestWorkflowExecutor(
		workflow, pr, autoMerge, autoRelease, autoReview,
		projectName, mgr, rel, autoCompleter, brancher, prCreator, cloner, worktreer, prMerger,
	)
	fh := failurehandler.NewHandler(mgr, n, completedDir, projectName, autoRetryLimit)
	// Build a real resumer using a no-op workflow adapter so existing tests
	// that don't exercise ResumeExecuting are not affected.
	resumer := promptresumer.NewResumer(
		mgr,
		exec,
		&noOpWorkflowExecutorAdapter{},
		completionreport.NewValidator(),
		fh,
		queueDir,
		completedDir,
		logDir,
		projectName,
		maxPromptDuration,
	)
	proc := processor.NewProcessor(
		exec,
		mgr,
		rel,
		vg,
		we,
		autoCompleter,
		specsweeper.NewSweeper(specLister, autoCompleter),
		preflightconditions.NewConditions(
			preflightChecker,
			gitLockChecker,
			dirtyFileChecker,
			dirtyFileThreshold,
		),
		containerslot.NewManager(
			containerLock,
			containerCounter,
			containerChecker,
			maxContainers,
			10*time.Second,
		),
		cancellationwatcher.NewWatcher(exec, mgr),
		wakeup,
		processor.Dirs{Queue: queueDir, Completed: completedDir, Log: logDir},
		processor.ProjectName(projectName),
		fh,
		resumer,
		processor.VerificationGate(verificationGate),
		completionreport.NewValidator(),
		promptenricher.NewEnricher(
			rel,
			additionalInstructions,
			testCommand,
			validationCommand,
			validationPrompt,
			validationprompt.NewResolver(),
		),
		committingrecoverer.NewRecoverer(mgr, rel, autoCompleter, completedDir),
		0,
		0,   // queueInterval and sweepInterval: 0 → use defaults (5s, 60s)
		nil, // onIdle: no-op for tests
	)
	scanner := queuescanner.NewScanner(mgr, proc, fh, queueDir)
	proc.SetScanner(scanner)
	return proc
}

// newProcessorTestPromptFile creates a PromptFile with approved status for use in processor tests.
// It is a package-level helper so it can be shared across split test files.
func newProcessorTestPromptFile(path string, body string) *prompt.PromptFile {
	return prompt.NewPromptFile(
		path,
		prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
		[]byte(body),
		libtime.NewCurrentDateTime(),
	)
}

// noOpWorkflowExecutorAdapter satisfies promptresumer.WorkflowExecutor with no-ops for tests
// that don't exercise the resume path.
type noOpWorkflowExecutorAdapter struct{}

func (noOpWorkflowExecutorAdapter) ReconstructState(
	_ context.Context,
	_ string,
	_ *prompt.PromptFile,
) (bool, error) {
	return true, nil
}

func (noOpWorkflowExecutorAdapter) Complete(
	_ context.Context,
	_ context.Context,
	_ *prompt.PromptFile,
	_, _, _ string,
) error {
	return nil
}

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

	// Set up default Load behavior to return a valid PromptFile
	BeforeEach(func() {
		manager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
			return newProcessorTestPromptFile(path, "# Test\n\nDefault test content"), nil
		}
	})

	It("should start and stop cleanly", func() {
		manager.ListQueuedReturns([]prompt.Prompt{}, nil)

		p := newTestProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
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
		errCh := make(chan error, 1)
		go func() {
			errCh <- p.Process(ctx)
		}()

		// Let it run briefly
		time.Sleep(200 * time.Millisecond)

		// Cancel and verify clean shutdown
		cancel()

		select {
		case err := <-errCh:
			Expect(err).To(BeNil())
		case <-time.After(2 * time.Second):
			Fail("processor did not stop within timeout")
		}
	})

	It("should process existing queued prompt on startup", func() {
		promptPath := filepath.Join(promptsDir, "001-test.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		// First call returns prompt, second call returns empty (processed)
		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		manager.SetStatusReturns(nil)
		manager.MoveToCompletedReturns(nil)
		manager.AllPreviousCompletedReturns(true)
		executor.ExecuteReturns(nil)
		releaser.CommitCompletedFileReturns(nil)
		releaser.HasChangelogReturns(false)
		releaser.CommitOnlyReturns(nil)

		p := newTestProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
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
		errCh := make(chan error, 1)
		go func() {
			errCh <- p.Process(ctx)
		}()

		// Wait for processing
		Eventually(func() int {
			return executor.ExecuteCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		// Verify executor was called with correct log path
		_, _, logFile, containerName := executor.ExecuteArgsForCall(0)
		Expect(logFile).To(Equal(filepath.Join(promptsDir, "log", "001-test.log")))
		Expect(containerName).To(Equal("test-project-001-test"))

		// Verify Load was called (processor uses Load/Save pattern now)
		Expect(manager.LoadCallCount()).To(BeNumerically(">=", 1))

		// Verify moved to completed
		Expect(manager.MoveToCompletedCallCount()).To(Equal(1))

		cancel()
	})

	It("should process prompts when wakeup signal received", func() {
		promptPath := filepath.Join(promptsDir, "001-signal.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		// Initially no prompts
		manager.ListQueuedReturns([]prompt.Prompt{}, nil)
		manager.SetStatusReturns(nil)
		manager.MoveToCompletedReturns(nil)
		manager.AllPreviousCompletedReturns(true)
		executor.ExecuteReturns(nil)
		releaser.CommitCompletedFileReturns(nil)
		releaser.HasChangelogReturns(false)
		releaser.CommitOnlyReturns(nil)

		p := newTestProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
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

		// Wait for initial scan
		time.Sleep(200 * time.Millisecond)

		// Now return a prompt and send wakeup signal
		manager.ListQueuedReturnsOnCall(1, queued, nil)
		manager.ListQueuedReturnsOnCall(2, []prompt.Prompt{}, nil)
		wakeup <- struct{}{}

		// Wait for processing
		Eventually(func() int {
			return executor.ExecuteCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		cancel()
	})

	It("should skip empty prompts", func() {
		promptPath := filepath.Join(promptsDir, "001-empty.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		// Override Load to return empty body
		manager.LoadReturns(
			prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
				[]byte(""),
				libtime.NewCurrentDateTime(),
			),
			nil,
		)
		manager.MoveToCompletedReturns(nil)
		manager.AllPreviousCompletedReturns(true)

		p := newTestProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
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

		// Verify executor was NOT called
		Expect(executor.ExecuteCallCount()).To(Equal(0))

		cancel()
	})

	It("should handle executor errors and mark prompt as failed", func() {
		promptPath := filepath.Join(promptsDir, "001-fail.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		// Return queued once, then empty (so loop exits after failure)
		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, nil, nil)
		manager.SetStatusReturns(nil)
		manager.AllPreviousCompletedReturns(true)
		executor.ExecuteReturns(stderrors.New("execution failed"))

		p := newTestProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
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

		// Run processor — marks failed and continues (no error returned)
		go func() {
			_ = p.Process(ctx)
		}()

		// Wait for Load to be called (marks prompt as failed)
		Eventually(func() int {
			return manager.LoadCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

		cancel()
	})

	It("should fire prompt_failed notification when executor returns error", func() {
		promptPath := filepath.Join(promptsDir, "001-fail-notify.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, nil, nil)
		manager.SetStatusReturns(nil)
		manager.AllPreviousCompletedReturns(true)
		executor.ExecuteReturns(stderrors.New("execution failed"))

		fakeNotifier := &mocks.Notifier{}

		p := newTestProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
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
		Expect(event.EventType).To(Equal("prompt_failed"))
		Expect(event.ProjectName).To(Equal("test-project"))
		Expect(event.PromptName).To(Equal("001-fail-notify.md"))
	})

	It("should call CommitOnly when no changelog", func() {
		promptPath := filepath.Join(promptsDir, "001-no-changelog.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		manager.SetStatusReturns(nil)
		manager.MoveToCompletedReturns(nil)
		manager.AllPreviousCompletedReturns(true)
		executor.ExecuteReturns(nil)
		releaser.CommitCompletedFileReturns(nil)
		releaser.HasChangelogReturns(false)
		releaser.CommitOnlyReturns(nil)

		p := newTestProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
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

		// Verify CommitAndRelease was NOT called
		Expect(releaser.CommitAndReleaseCallCount()).To(Equal(0))

		cancel()
	})

	It("should call CommitAndRelease with PatchBump when changelog exists", func() {
		promptPath := filepath.Join(promptsDir, "001-with-changelog.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		manager.SetStatusReturns(nil)
		manager.MoveToCompletedReturns(nil)
		manager.AllPreviousCompletedReturns(true)
		executor.ExecuteReturns(nil)
		releaser.CommitCompletedFileReturns(nil)
		releaser.HasChangelogReturns(true)
		releaser.GetNextVersionReturns("v0.1.1", nil)
		releaser.CommitAndReleaseReturns(nil)

		p := newTestProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
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
			true, // autoRelease=true
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
			return releaser.CommitAndReleaseCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		// Verify PatchBump was used
		_, bump := releaser.CommitAndReleaseArgsForCall(0)
		Expect(bump).To(Equal(git.PatchBump))

		// Verify CommitOnly was NOT called
		Expect(releaser.CommitOnlyCallCount()).To(Equal(0))

		cancel()
	})

	It("should call CommitAndRelease with MinorBump for feature title", func() {
		// Create CHANGELOG.md with "Add new feature" in Unreleased section
		// determineBump reads from current directory
		originalDir, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())

		err = os.Chdir(tempDir)
		Expect(err).NotTo(HaveOccurred())
		defer func() {
			_ = os.Chdir(originalDir)
		}()

		err = os.WriteFile(
			"CHANGELOG.md",
			[]byte("## Unreleased\n\n- feat: Add new feature\n"),
			0600,
		)
		Expect(err).NotTo(HaveOccurred())

		promptPath := filepath.Join(promptsDir, "001-feature.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		// Override Load to return PromptFile with title "Add new feature"
		manager.LoadReturns(
			prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
				[]byte("# Add new feature\n\nImplement new feature."),
				libtime.NewCurrentDateTime(),
			),
			nil,
		)
		manager.SetStatusReturns(nil)
		manager.MoveToCompletedReturns(nil)
		manager.AllPreviousCompletedReturns(true)
		executor.ExecuteReturns(nil)
		releaser.CommitCompletedFileReturns(nil)
		releaser.HasChangelogReturns(true)
		releaser.GetNextVersionReturns("v0.2.0", nil)
		releaser.CommitAndReleaseReturns(nil)

		p := newTestProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
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
			true, // autoRelease=true
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
			return releaser.CommitAndReleaseCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		// Verify MinorBump was used
		_, bump := releaser.CommitAndReleaseArgsForCall(0)
		Expect(bump).To(Equal(git.MinorBump))

		cancel()
	})

	It("should process multiple queued prompts sequentially", func() {
		promptPath1 := filepath.Join(promptsDir, "001-first.md")
		promptPath2 := filepath.Join(promptsDir, "002-second.md")

		// Return both prompts first, then just second, then none
		manager.ListQueuedReturnsOnCall(0, []prompt.Prompt{
			{Path: promptPath1, Status: prompt.ApprovedPromptStatus},
			{Path: promptPath2, Status: prompt.ApprovedPromptStatus},
		}, nil)
		manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{
			{Path: promptPath2, Status: prompt.ApprovedPromptStatus},
		}, nil)
		manager.ListQueuedReturnsOnCall(2, []prompt.Prompt{}, nil)

		manager.SetStatusReturns(nil)
		manager.MoveToCompletedReturns(nil)
		manager.AllPreviousCompletedReturns(true)
		executor.ExecuteReturns(nil)
		releaser.CommitCompletedFileReturns(nil)
		releaser.HasChangelogReturns(false)
		releaser.CommitOnlyReturns(nil)

		p := newTestProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
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

		// Wait for both to be processed
		Eventually(func() int {
			return executor.ExecuteCallCount()
		}, 3*time.Second, 50*time.Millisecond).Should(Equal(2))

		cancel()
	})

	It("should sanitize container name", func() {
		promptPath := filepath.Join(promptsDir, "001-test@file#name.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		manager.SetStatusReturns(nil)
		manager.MoveToCompletedReturns(nil)
		manager.AllPreviousCompletedReturns(true)
		executor.ExecuteReturns(nil)
		releaser.CommitCompletedFileReturns(nil)
		releaser.HasChangelogReturns(false)
		releaser.CommitOnlyReturns(nil)

		p := newTestProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
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
			return executor.ExecuteCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		// Verify container name was sanitized
		_, _, _, containerName := executor.ExecuteArgsForCall(0)
		Expect(containerName).To(Equal("test-project-001-test-file-name"))

		cancel()
	})

	It("should auto-set status to queued when prompt has non-standard status", func() {
		promptPath := filepath.Join(promptsDir, "001-auto-status.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: ""}, // empty status triggers auto-set
		}

		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		manager.SetStatusReturns(nil)
		manager.MoveToCompletedReturns(nil)
		manager.AllPreviousCompletedReturns(true)
		executor.ExecuteReturns(nil)
		releaser.CommitCompletedFileReturns(nil)
		releaser.HasChangelogReturns(false)
		releaser.CommitOnlyReturns(nil)

		p := newTestProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
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

		go func() {
			_ = p.Process(ctx)
		}()

		// Wait for SetStatus to be called (auto-set to queued)
		Eventually(func() int {
			return manager.SetStatusCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

		// Verify SetStatus was called with "approved"
		_, _, status := manager.SetStatusArgsForCall(0)
		Expect(status).To(Equal("approved"))

		cancel()
	})

	It("should skip prompt with invalid status", func() {
		promptPath := filepath.Join(promptsDir, "001-invalid-status.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ExecutingPromptStatus}, // Wrong status for execution
		}

		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		manager.AllPreviousCompletedReturns(true)

		p := newTestProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
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

		// Wait briefly to ensure processing completes
		time.Sleep(300 * time.Millisecond)

		// Verify executor was NOT called
		Expect(executor.ExecuteCallCount()).To(Equal(0))

		cancel()
	})

	It("should skip prompt when previous not completed", func() {
		promptPath := filepath.Join(promptsDir, "003-third.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		manager.AllPreviousCompletedReturns(false) // Previous prompts not completed

		p := newTestProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
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

		// Wait briefly to ensure processing completes
		time.Sleep(300 * time.Millisecond)

		// Verify executor was NOT called
		Expect(executor.ExecuteCallCount()).To(Equal(0))

		cancel()
	})

	It("should set version in frontmatter from version getter", func() {
		promptPath := filepath.Join(promptsDir, "001-version-test.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		manager.SetStatusReturns(nil)
		manager.MoveToCompletedReturns(nil)
		manager.AllPreviousCompletedReturns(true)
		executor.ExecuteReturns(nil)
		releaser.CommitCompletedFileReturns(nil)
		releaser.HasChangelogReturns(false)
		releaser.CommitOnlyReturns(nil)

		p := newTestProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
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

		// Wait for processing (processor uses Load/PrepareForExecution now)
		Eventually(func() int {
			return manager.LoadCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

		// Verify Load was called with correct path
		_, path := manager.LoadArgsForCall(0)
		Expect(path).To(Equal(promptPath))

		cancel()
	})

	It("should append completion report suffix to content before executor call", func() {
		promptPath := filepath.Join(promptsDir, "001-suffix-test.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		// Override Load to return PromptFile with specific content
		manager.LoadReturns(
			prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
				[]byte("# Test prompt content\n\nContent for testing suffix."),
				libtime.NewCurrentDateTime(),
			),
			nil,
		)
		manager.SetStatusReturns(nil)
		manager.MoveToCompletedReturns(nil)
		manager.AllPreviousCompletedReturns(true)
		executor.ExecuteReturns(nil)
		releaser.CommitCompletedFileReturns(nil)
		releaser.HasChangelogReturns(false)
		releaser.CommitOnlyReturns(nil)

		p := newTestProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
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
			return executor.ExecuteCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		// Verify executor was called with content including suffix
		_, promptContent, _, _ := executor.ExecuteArgsForCall(0)
		Expect(promptContent).To(ContainSubstring("# Test prompt content"))
		Expect(promptContent).To(ContainSubstring("DARK-FACTORY-REPORT"))
		Expect(promptContent).To(ContainSubstring("Completion Report (MANDATORY)"))
		Expect(promptContent).To(HaveSuffix(report.Suffix()))

		cancel()
	})

	It("should append validation suffix when validationCommand is set", func() {
		promptPath := filepath.Join(promptsDir, "001-validation-test.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		manager.LoadReturns(
			prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
				[]byte("# Validation test\n\nContent for validation suffix test."),
				libtime.NewCurrentDateTime(),
			),
			nil,
		)
		manager.MoveToCompletedReturns(nil)
		manager.AllPreviousCompletedReturns(true)
		executor.ExecuteReturns(nil)
		releaser.CommitCompletedFileReturns(nil)
		releaser.HasChangelogReturns(false)
		releaser.CommitOnlyReturns(nil)

		p := newTestProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
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
			"make precommit",
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

		go func() {
			_ = p.Process(ctx)
		}()

		Eventually(func() int {
			return executor.ExecuteCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		_, promptContent, _, _ := executor.ExecuteArgsForCall(0)
		Expect(promptContent).To(ContainSubstring(report.ValidationSuffix("make precommit")))

		cancel()
	})

	It("should append validation prompt suffix when validationPrompt is inline text", func() {
		promptPath := filepath.Join(promptsDir, "001-validation-prompt-test.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		manager.LoadReturns(
			prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
				[]byte("# Validation prompt test\n\nContent for validation prompt test."),
				libtime.NewCurrentDateTime(),
			),
			nil,
		)
		manager.MoveToCompletedReturns(nil)
		manager.AllPreviousCompletedReturns(true)
		executor.ExecuteReturns(nil)
		releaser.CommitCompletedFileReturns(nil)
		releaser.HasChangelogReturns(false)
		releaser.CommitOnlyReturns(nil)

		p := newTestProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
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
			"readme.md is updated",
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

		go func() {
			_ = p.Process(ctx)
		}()

		Eventually(func() int {
			return executor.ExecuteCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		_, promptContent, _, _ := executor.ExecuteArgsForCall(0)
		Expect(promptContent).To(ContainSubstring("readme.md is updated"))

		cancel()
	})

	It("should inject test command suffix when testCommand is set", func() {
		promptPath := filepath.Join(promptsDir, "001-test-command-inject.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		manager.LoadReturns(
			prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
				[]byte("# Test command inject test\n\nContent."),
				libtime.NewCurrentDateTime(),
			),
			nil,
		)
		manager.MoveToCompletedReturns(nil)
		manager.AllPreviousCompletedReturns(true)
		executor.ExecuteReturns(nil)
		releaser.CommitCompletedFileReturns(nil)
		releaser.HasChangelogReturns(false)
		releaser.CommitOnlyReturns(nil)

		p := newTestProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
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
			"make test",
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

		go func() {
			_ = p.Process(ctx)
		}()

		Eventually(func() int {
			return executor.ExecuteCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		_, promptContent, _, _ := executor.ExecuteArgsForCall(0)
		Expect(promptContent).To(ContainSubstring("make test"))
		Expect(promptContent).To(ContainSubstring("Fast Feedback"))

		cancel()
	})

	It("should not inject test command suffix when testCommand is empty", func() {
		promptPath := filepath.Join(promptsDir, "001-test-command-empty.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		manager.LoadReturns(
			prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
				[]byte("# Test command empty test\n\nContent."),
				libtime.NewCurrentDateTime(),
			),
			nil,
		)
		manager.MoveToCompletedReturns(nil)
		manager.AllPreviousCompletedReturns(true)
		executor.ExecuteReturns(nil)
		releaser.CommitCompletedFileReturns(nil)
		releaser.HasChangelogReturns(false)
		releaser.CommitOnlyReturns(nil)

		p := newTestProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
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

		go func() {
			_ = p.Process(ctx)
		}()

		Eventually(func() int {
			return executor.ExecuteCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		_, promptContent, _, _ := executor.ExecuteArgsForCall(0)
		Expect(promptContent).NotTo(ContainSubstring("Fast Feedback"))

		cancel()
	})

	It("should inject test command suffix before validation command suffix", func() {
		promptPath := filepath.Join(promptsDir, "001-test-command-order.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		manager.LoadReturns(
			prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
				[]byte("# Test command order test\n\nContent."),
				libtime.NewCurrentDateTime(),
			),
			nil,
		)
		manager.MoveToCompletedReturns(nil)
		manager.AllPreviousCompletedReturns(true)
		executor.ExecuteReturns(nil)
		releaser.CommitCompletedFileReturns(nil)
		releaser.HasChangelogReturns(false)
		releaser.CommitOnlyReturns(nil)

		p := newTestProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
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
			"make precommit",
			"",
			"make test",
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

		go func() {
			_ = p.Process(ctx)
		}()

		Eventually(func() int {
			return executor.ExecuteCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		_, promptContent, _, _ := executor.ExecuteArgsForCall(0)
		fastFeedbackIdx := strings.Index(promptContent, "Fast Feedback")
		validationIdx := strings.Index(promptContent, "Project Validation Command")
		Expect(fastFeedbackIdx).To(BeNumerically("<", validationIdx))

		cancel()
	})

	It(
		"should not append validation prompt suffix when validationPrompt is missing .md file",
		func() {
			promptPath := filepath.Join(promptsDir, "001-validation-prompt-missing-test.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			manager.LoadReturns(
				prompt.NewPromptFile(
					promptPath,
					prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
					[]byte("# Validation prompt missing test\n\nContent."),
					libtime.NewCurrentDateTime(),
				),
				nil,
			)
			manager.MoveToCompletedReturns(nil)
			manager.AllPreviousCompletedReturns(true)
			executor.ExecuteReturns(nil)
			releaser.CommitCompletedFileReturns(nil)
			releaser.HasChangelogReturns(false)
			releaser.CommitOnlyReturns(nil)

			p := newTestProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
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
				"nonexistent-file.md",
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

			go func() {
				_ = p.Process(ctx)
			}()

			Eventually(func() int {
				return executor.ExecuteCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			_, promptContent, _, _ := executor.ExecuteArgsForCall(0)
			Expect(promptContent).NotTo(ContainSubstring("Project Quality Criteria"))

			cancel()
		},
	)

})
