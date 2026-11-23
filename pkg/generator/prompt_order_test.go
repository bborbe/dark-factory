// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package generator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	libtime "github.com/bborbe/time"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

// renameMover is the production file move the prompt manager performs, without git.
type renameMover struct{}

func (renameMover) MoveFile(_ context.Context, oldPath string, newPath string) error {
	return os.Rename(oldPath, newPath)
}

// passingExecutor stands in for the audit subprocess: every audit passes.
type passingExecutor struct{}

func (passingExecutor) Execute(_ context.Context, _, _, _ string) error { return nil }

func (passingExecutor) Reattach(_ context.Context, _ string, _ string, _ time.Duration) error {
	return nil
}

func (passingExecutor) StopAndRemoveContainer(_ context.Context, _ string) {}

// TestDiffFilesOrdersByCreatorPrefix pins the order diffFiles hands to the auto-approve
// loop. Without the ordering, the map range returns these in a random order, so the test
// fails deterministically on a plain slice comparison rather than only sometimes.
//
// The 10-kilo entry is deliberate: lexicographic sorting of the paths would place
// "10-kilo.md" between "1-zulu.md" and "2-mike.md", so only a numeric prefix parse keeps
// the creator's order past nine prompts.
func TestDiffFilesOrdersByCreatorPrefix(t *testing.T) {
	after := map[string]struct{}{
		"/inbox/3-alpha.md":    {},
		"/inbox/10-kilo.md":    {},
		"/inbox/1-zulu.md":     {},
		"/inbox/2-mike.md":     {},
		"/inbox/unprefixed.md": {},
	}

	got := diffFiles(map[string]struct{}{}, after)

	want := []string{
		"/inbox/1-zulu.md",
		"/inbox/2-mike.md",
		"/inbox/3-alpha.md",
		"/inbox/10-kilo.md",
		"/inbox/unprefixed.md",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("diffFiles order = %v, want %v", got, want)
	}
}

// TestGeneratedPromptsAreNumberedInCreatorOrder drives the real approve path — diffFiles →
// autoApproveGeneratedPrompts → prompt.ApproveFromInbox → prompt.Manager.NormalizeFilenames
// — over a set whose slugs sort against the declared dependency order (zulu → mike →
// alpha), the shape that numbered spec-040's hardening prompt ahead of the service prompt
// it exercises.
//
// The loop repeats because the defect this guards against is random rather than
// deterministic: a single iteration could land in the right order by chance.
func TestGeneratedPromptsAreNumberedInCreatorOrder(t *testing.T) {
	const iterations = 20

	for i := 0; i < iterations; i++ {
		numbers := runApproveCycle(t, i)

		if numbers["zulu"] >= numbers["mike"] || numbers["mike"] >= numbers["alpha"] {
			t.Fatalf(
				"iteration %d: execution numbers = %v, want zulu < mike < alpha",
				i,
				numbers,
			)
		}
	}
}

// runApproveCycle writes the three prefixed prompts into a fresh inbox, approves them
// through the real generator path, and returns each slug's assigned execution number.
func runApproveCycle(t *testing.T, iteration int) map[string]int {
	t.Helper()

	ctx := context.Background()
	root := t.TempDir()
	inboxDir := filepath.Join(root, "inbox")
	queueDir := filepath.Join(root, "queue")
	completedDir := filepath.Join(root, "completed")
	logDir := filepath.Join(root, "logs")
	for _, dir := range []string{inboxDir, queueDir, completedDir, logDir} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatalf("iteration %d: mkdir %s: %v", iteration, dir, err)
		}
	}

	// Creator's declared order is zulu → mike → alpha; alphabetically it is the reverse.
	for _, name := range []string{"1-zulu.md", "2-mike.md", "3-alpha.md"} {
		body := "---\nstatus: draft\n---\n# Generated"
		if err := os.WriteFile(filepath.Join(inboxDir, name), []byte(body), 0o600); err != nil {
			t.Fatalf("iteration %d: write %s: %v", iteration, name, err)
		}
	}

	after, err := listMDFiles(inboxDir)
	if err != nil {
		t.Fatalf("iteration %d: list inbox: %v", iteration, err)
	}
	newFiles := diffFiles(map[string]struct{}{}, after)

	mgr := prompt.NewManager(
		inboxDir,
		queueDir,
		completedDir,
		"",
		renameMover{},
		libtime.NewCurrentDateTime(),
	)

	sg := &dockerSpecGenerator{
		executor:           passingExecutor{},
		inboxDir:           inboxDir,
		completedDir:       completedDir,
		logDir:             logDir,
		promptManager:      mgr,
		autoApprovePrompts: true,
		queueDir:           queueDir,
	}
	sg.autoApproveGeneratedPrompts(ctx, "spec-order-test", newFiles)

	return queuedNumbers(t, iteration, queueDir)
}

// queuedNumbers maps each approved prompt's slug to the execution number it was assigned.
func queuedNumbers(t *testing.T, iteration int, queueDir string) map[string]int {
	t.Helper()

	entries, err := os.ReadDir(queueDir)
	if err != nil {
		t.Fatalf("iteration %d: read queue: %v", iteration, err)
	}
	if len(entries) != 3 {
		t.Fatalf("iteration %d: queue holds %d prompts, want 3", iteration, len(entries))
	}

	numbers := make(map[string]int, len(entries))
	for _, entry := range entries {
		var num int
		var slug string
		if _, err := fmt.Sscanf(entry.Name(), "%d-%s", &num, &slug); err != nil {
			t.Fatalf("iteration %d: parse %q: %v", iteration, entry.Name(), err)
		}
		slug = slug[:len(slug)-len(".md")]
		numbers[slug] = num
	}
	return numbers
}
