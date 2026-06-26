// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/cancellationwatcher"
	"github.com/bborbe/dark-factory/pkg/committingrecoverer"
	"github.com/bborbe/dark-factory/pkg/completionreport"
	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/executionslot"
	"github.com/bborbe/dark-factory/pkg/failurehandler"
	"github.com/bborbe/dark-factory/pkg/notifier"
	"github.com/bborbe/dark-factory/pkg/preflightconditions"
	"github.com/bborbe/dark-factory/pkg/processor"
	"github.com/bborbe/dark-factory/pkg/project"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/promptenricher"
	"github.com/bborbe/dark-factory/pkg/promptresumer"
	"github.com/bborbe/dark-factory/pkg/queuescanner"
	"github.com/bborbe/dark-factory/pkg/specsweeper"
	"github.com/bborbe/dark-factory/pkg/validationprompt"
)

// processorPromptProcesser is a local interface matching *processor's ProcessPrompt method,
// used to call it from the external test package without naming the unexported type.
type processorPromptProcesser interface {
	ProcessPrompt(ctx context.Context, pr prompt.Prompt) error
}

// newProcessorWithMockWatcher creates a processor with a mock CancellationWatcher
// for testing the cancellation path without going through the full Process loop.
func newProcessorWithMockWatcher(
	logDir string,
	exec *mocks.Executor,
	mgr *mocks.ProcessorPromptManager,
	vg *mocks.VersionGetter,
	cancellationWatcher cancellationwatcher.Watcher,
	workflowExec *mocks.WorkflowExecutor,
) processorPromptProcesser {
	enricherReleaser := &mocks.Releaser{}
	enricherReleaser.CommitWithRetryStub = func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }
	enricherReleaser.HasChangelogReturns(false)

	fh := failurehandler.NewHandler(mgr, notifier.NewMultiNotifier(), "", project.Name("test"), 0)
	resumer := promptresumer.NewResumer(
		mgr,
		exec,
		&noOpWorkflowExecutorAdapter{},
		completionreport.NewValidator(),
		fh,
		"",
		"",
		logDir,
		project.Name("test"),
		0,
	)
	ppForwarder := &lazyProcessorForwarder{}
	scanner := queuescanner.NewScanner(mgr, ppForwarder, fh, "", nil, 0)

	proc := processor.NewProcessor(
		exec,
		mgr,
		nil,
		vg,
		workflowExec,
		nil,
		specsweeper.NewSweeper(nil, nil),
		preflightconditions.NewConditions(nil, nil, nil, 0),
		executionslot.NewManager(nil, nil, nil, 0, 0),
		cancellationWatcher,
		make(chan struct{}),
		processor.Dirs{Log: logDir},
		project.Name("test"),
		fh,
		resumer,
		config.WorkflowDirect,
		false,
		completionreport.NewValidator(),
		promptenricher.NewEnricher(
			enricherReleaser,
			"",
			"",
			"",
			"",
			validationprompt.NewResolver(),
			false,
		),
		committingrecoverer.NewRecoverer(mgr, nil, nil, "", false),
		scanner,
		0,
		0,
		nil,
	)
	ppForwarder.inner = proc
	return proc
}

var _ = Describe("ProcessPrompt — cancellation", func() {
	It("calls MoveToCancelled when container is cancelled", func() {
		tempDir, err := os.MkdirTemp("", "processor-cancel-*")
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = os.RemoveAll(tempDir) }()

		logDir := filepath.Join(tempDir, "log")
		err = os.MkdirAll(logDir, 0750)
		Expect(err).NotTo(HaveOccurred())

		promptPath := filepath.Join(tempDir, "001-cancel-test.md")
		err = os.WriteFile(
			promptPath,
			[]byte("---\nstatus: approved\n---\n# Cancel test\n\nTest content"),
			0600,
		)
		Expect(err).NotTo(HaveOccurred())

		ctx := context.Background()

		mgr := &mocks.ProcessorPromptManager{}
		mgr.LoadReturns(
			prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
				[]byte("# Cancel test\n\nTest content"),
				libtime.NewCurrentDateTime(),
			),
			nil,
		)
		mgr.MoveToCancelledReturns(nil)

		// Executor blocks until its context is cancelled.
		exec := &mocks.Executor{}
		exec.ExecuteStub = func(execCtx context.Context, _, _, _ string) error {
			<-execCtx.Done()
			return execCtx.Err()
		}

		// CancellationWatcher returns a pre-closed channel so cancellation fires immediately.
		fakeCancellationWatcher := &mocks.CancellationWatcher{}
		cancelledCh := make(chan struct{})
		close(cancelledCh)
		fakeCancellationWatcher.WatchReturns(cancelledCh)

		workflowExec := &mocks.WorkflowExecutor{}
		workflowExec.SetupReturns(nil)

		vg := &mocks.VersionGetter{}
		vg.GetReturns("v0.0.1-test")

		pp := newProcessorWithMockWatcher(
			logDir,
			exec,
			mgr,
			vg,
			fakeCancellationWatcher,
			workflowExec,
		)

		pr := prompt.Prompt{Path: promptPath, Status: prompt.ApprovedPromptStatus}

		// Use a timeout so the test does not hang if the implementation is wrong.
		testCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		err = pp.ProcessPrompt(testCtx, pr)
		Expect(err).NotTo(HaveOccurred())

		Expect(mgr.MoveToCancelledCallCount()).To(Equal(1))
		_, cancelledPath := mgr.MoveToCancelledArgsForCall(0)
		Expect(cancelledPath).To(Equal(promptPath))
	})

	It("detects cancellation from file when Execute returns before cancel channel closes", func() {
		tempDir, err := os.MkdirTemp("", "processor-cancel-fallback-*")
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = os.RemoveAll(tempDir) }()

		logDir := filepath.Join(tempDir, "log")
		err = os.MkdirAll(logDir, 0750)
		Expect(err).NotTo(HaveOccurred())

		promptPath := filepath.Join(tempDir, "002-cancel-fallback-test.md")
		err = os.WriteFile(
			promptPath,
			[]byte("---\nstatus: approved\n---\n# Cancel fallback test\n\nTest content"),
			0600,
		)
		Expect(err).NotTo(HaveOccurred())

		ctx := context.Background()

		mgr := &mocks.ProcessorPromptManager{}
		// First call (ProcessPrompt initial load): return approved status
		mgr.LoadReturnsOnCall(0,
			prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
				[]byte("# Cancel fallback test\n\nTest content"),
				libtime.NewCurrentDateTime(),
			),
			nil,
		)
		// Second call (runContainer fallback re-read): return cancelled status
		mgr.LoadReturnsOnCall(1,
			prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{Status: string(prompt.CancelledPromptStatus)},
				[]byte("# Cancel fallback test\n\nTest content"),
				libtime.NewCurrentDateTime(),
			),
			nil,
		)
		mgr.MoveToCancelledReturns(nil)

		// Execute returns immediately with an error (simulates SIGTERM from container stop)
		exec := &mocks.Executor{}
		exec.ExecuteReturns(fmt.Errorf("exit status 143"))

		// CancellationWatcher returns a channel that NEVER closes during the test.
		// This simulates the race: Execute returns before the cancel channel fires.
		fakeCancellationWatcher := &mocks.CancellationWatcher{}
		neverClosingCh := make(chan struct{}) // never closed — simulates the race
		fakeCancellationWatcher.WatchReturns(neverClosingCh)

		workflowExec := &mocks.WorkflowExecutor{}
		workflowExec.SetupReturns(nil)

		vg := &mocks.VersionGetter{}
		vg.GetReturns("v0.0.1-test")

		pp := newProcessorWithMockWatcher(
			logDir,
			exec,
			mgr,
			vg,
			fakeCancellationWatcher,
			workflowExec,
		)

		pr := prompt.Prompt{Path: promptPath, Status: prompt.ApprovedPromptStatus}

		testCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		err = pp.ProcessPrompt(testCtx, pr)
		Expect(err).NotTo(HaveOccurred())

		// Processor must call MoveToCancelled (not MarkFailed / return error)
		Expect(mgr.MoveToCancelledCallCount()).To(Equal(1))
		_, cancelledPath := mgr.MoveToCancelledArgsForCall(0)
		Expect(cancelledPath).To(Equal(promptPath))
	})
})
