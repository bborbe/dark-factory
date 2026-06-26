// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package prompt_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	libtime "github.com/bborbe/time"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

// TestLoadAcceptsLegacyContainerKey verifies that a prompt file with the legacy
// `container:` YAML key is loaded correctly, populating Frontmatter.Container.
func TestLoadAcceptsLegacyContainerKey(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "001-legacy.md")

	content := "---\nstatus: executing\ncontainer: legacy-name-123\n---\n\n# Title\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	pf, err := prompt.NewManager("", "", "", "", nil, libtime.NewCurrentDateTime()).Load(ctx, path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if pf.Frontmatter.Container != "legacy-name-123" {
		t.Fatalf("expected Container=%q, got %q", "legacy-name-123", pf.Frontmatter.Container)
	}
}

// TestSaveEmitsExecutionIDKey verifies that Save writes execution_id: (not container:)
// when the Container field is set.
func TestSaveEmitsExecutionIDKey(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "001-new.md")

	pf := prompt.NewPromptFile(
		path,
		prompt.Frontmatter{Status: "executing", Container: "exec-abc"},
		[]byte("# Title\n\nBody\n"),
		libtime.NewCurrentDateTime(),
	)
	if err := pf.Save(ctx); err != nil {
		t.Fatalf("Save: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	contents := string(raw)

	if !strings.Contains(contents, "execution_id: exec-abc") {
		t.Fatalf("expected file to contain %q, got:\n%s", "execution_id: exec-abc", contents)
	}
	if strings.Contains(contents, "\ncontainer:") {
		t.Fatalf("file must not contain a 'container:' line, got:\n%s", contents)
	}
}

// TestLegacyContainerKeyRoundTripsUnchanged verifies the semantic round-trip:
// loading a legacy file (container:) and saving it preserves the Container value.
// Legacy files canonicalize to execution_id on save by design (spec 102 writers emit
// execution_id only); we assert semantic equality, not byte-identity.
func TestLegacyContainerKeyRoundTripsUnchanged(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "001-legacy.md")

	fixture := "---\nstatus: executing\ncontainer: foo\n---\n\n# Title\n\nBody content.\n"
	if err := os.WriteFile(path, []byte(fixture), 0600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	mgr := prompt.NewManager("", "", "", "", nil, libtime.NewCurrentDateTime())
	pf, err := mgr.Load(ctx, path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := pf.Save(ctx); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Reload after save and assert semantic equality.
	pf2, err := mgr.Load(ctx, path)
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
	if pf2.Frontmatter.Container != "foo" {
		t.Fatalf(
			"after round-trip, expected Container=%q, got %q",
			"foo",
			pf2.Frontmatter.Container,
		)
	}

	// The saved file must use the canonical key.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile after Save: %v", err)
	}
	if !strings.Contains(string(raw), "execution_id: foo") {
		t.Fatalf("saved file must contain 'execution_id: foo', got:\n%s", string(raw))
	}
}
