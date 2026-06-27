// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package prompt_test

import (
	"context"
	"os"
	"path/filepath"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

var _ = Describe("ApproveFromInbox", func() {
	var (
		ctx      context.Context
		tempDir  string
		inboxDir string
		queueDir string
		mgr      *prompt.Manager
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		tempDir, err = os.MkdirTemp("", "approve-test-*")
		Expect(err).To(BeNil())

		inboxDir = filepath.Join(tempDir, "inbox")
		queueDir = filepath.Join(tempDir, "queue")
		Expect(os.Mkdir(inboxDir, 0o755)).To(Succeed())
		Expect(os.Mkdir(queueDir, 0o755)).To(Succeed())

		mgr = prompt.NewManager(
			inboxDir, queueDir, "", "",
			&simpleMover{},
			libtime.NewCurrentDateTime(),
		)
	})

	AfterEach(func() {
		_ = os.RemoveAll(tempDir)
	})

	It("moves prompt from inbox to queue, strips numeric prefix, and marks approved", func() {
		inboxPath := createPromptFile(inboxDir, "017-do-thing.md", "draft")

		newPath, err := prompt.ApproveFromInbox(ctx, inboxPath, queueDir, mgr)
		Expect(err).NotTo(HaveOccurred())

		// File moved: gone from inbox, present in queue WITHOUT the numeric prefix
		// (NormalizeFilenames is the caller's responsibility — it would re-number).
		_, statErr := os.Stat(inboxPath)
		Expect(os.IsNotExist(statErr)).To(BeTrue())
		Expect(newPath).To(Equal(filepath.Join(queueDir, "do-thing.md")))
		_, statErr = os.Stat(newPath)
		Expect(statErr).NotTo(HaveOccurred())

		// Frontmatter status set to approved
		pf, loadErr := mgr.Load(ctx, newPath)
		Expect(loadErr).NotTo(HaveOccurred())
		Expect(pf.Frontmatter.Status).To(Equal("approved"))
	})

	It("returns wrapped error when the source file doesn't exist", func() {
		_, err := prompt.ApproveFromInbox(
			ctx,
			filepath.Join(inboxDir, "does-not-exist.md"),
			queueDir,
			mgr,
		)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("move file to queue"))
	})

	It("does NOT call NormalizeFilenames (caller's responsibility)", func() {
		// Two files in the queue with overlapping numeric prefixes — if the
		// primitive were calling NormalizeFilenames it would renumber them.
		// Asserting it doesn't pins the divergence between cmd (returns
		// normalize error) and generator (warns + tolerates) at the call sites.
		inboxPath := createPromptFile(inboxDir, "005-new.md", "draft")
		_ = createPromptFile(queueDir, "002-existing.md", "approved")

		newPath, err := prompt.ApproveFromInbox(ctx, inboxPath, queueDir, mgr)
		Expect(err).NotTo(HaveOccurred())

		// The new file landed at "new.md" (prefix stripped) — NOT
		// "001-new.md" / "002-new.md" (those would be NormalizeFilenames's job).
		Expect(filepath.Base(newPath)).To(Equal("new.md"))
		// The existing queue file is undisturbed.
		_, statErr := os.Stat(filepath.Join(queueDir, "002-existing.md"))
		Expect(statErr).NotTo(HaveOccurred())
	})
})
