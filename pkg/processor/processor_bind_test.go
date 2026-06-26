// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	. "github.com/onsi/gomega"

	pkglog "github.com/bborbe/dark-factory/pkg/log"
)

func TestBindPromptLogger(t *testing.T) {
	g := NewGomegaWithT(t)

	var buf bytes.Buffer
	orig := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(orig)

	ctx := bindPromptLogger(context.Background(), "042-foo", "099", "", "direct")
	pkglog.From(ctx).Info("x")

	out := buf.String()
	g.Expect(out).To(ContainSubstring("prompt_id=042-foo"))
	g.Expect(out).To(ContainSubstring("spec_id=099"))
	g.Expect(out).To(ContainSubstring(`container=""`))
	g.Expect(out).To(ContainSubstring("workflow_type=direct"))
}
