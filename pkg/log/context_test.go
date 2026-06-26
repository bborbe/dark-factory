// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package log_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	log "github.com/bborbe/dark-factory/pkg/log"
)

func TestFromReturnsBoundLogger(t *testing.T) {
	var buf bytes.Buffer
	l := slog.New(slog.NewTextHandler(&buf, nil))
	ctx := log.NewContext(context.Background(), l)
	if got := log.From(ctx); got != l {
		t.Fatalf("From did not return the bound logger")
	}
}

func TestFromFallbackNeverNil(t *testing.T) {
	if got := log.From(context.Background()); got == nil {
		t.Fatal("From returned nil for an unbound context")
	}
}

func TestWithAttrsPropagate(t *testing.T) {
	var buf bytes.Buffer
	base := slog.New(slog.NewTextHandler(&buf, nil))
	bound := base.With("prompt_id", "042-foo")
	ctx := log.NewContext(context.Background(), bound)
	log.From(ctx).Info("hello")
	if !strings.Contains(buf.String(), "prompt_id=042-foo") {
		t.Fatalf("expected prompt_id attr in output, got: %s", buf.String())
	}
}

var _ = Describe("log context isolation", func() {
	It("two goroutines each see only their own prompt_id", func() {
		type result struct {
			promptID string
			output   string
		}

		results := make([]result, 2)
		var wg sync.WaitGroup

		ids := []string{"goroutine-A", "goroutine-B"}
		for i, id := range ids {
			wg.Add(1)
			go func(idx int, promptID string) {
				defer wg.Done()
				var buf bytes.Buffer
				l := slog.New(slog.NewTextHandler(&buf, nil)).With("prompt_id", promptID)
				ctx := log.NewContext(context.Background(), l)
				log.From(ctx).Info("working")
				results[idx] = result{promptID: promptID, output: buf.String()}
			}(i, id)
		}
		wg.Wait()

		for _, r := range results {
			Expect(r.output).To(ContainSubstring("prompt_id=" + r.promptID))
			for _, other := range ids {
				if other != r.promptID {
					Expect(r.output).NotTo(ContainSubstring("prompt_id=" + other))
				}
			}
		}
	})
})
