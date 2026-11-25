// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"context"
	stderrors "errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/subproc"
)

// scriptedRunner returns a canned result per call, in order. It exists so this
// package's internal tests can drive a Brancher without importing mocks, which
// imports this package and would close an import cycle.
type scriptedRunner struct {
	results []scriptedResult
	calls   int
}

type scriptedResult struct {
	out []byte
	err error
}

func (r *scriptedRunner) next() ([]byte, error) {
	if r.calls >= len(r.results) {
		return nil, stderrors.New("scriptedRunner: unexpected call beyond the script")
	}
	res := r.results[r.calls]
	r.calls++
	return res.out, res.err
}

func (r *scriptedRunner) RunWithWarnAndTimeout(
	_ context.Context,
	_ string,
	_ string,
	_ ...string,
) ([]byte, error) {
	return r.next()
}

func (r *scriptedRunner) RunWithWarnAndTimeoutDir(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ ...string,
) ([]byte, error) {
	return r.next()
}

func (r *scriptedRunner) RunWithWarnAndTimeoutEnv(
	_ context.Context,
	_ string,
	_ string,
	_ []string,
	_ string,
	_ ...string,
) ([]byte, error) {
	return r.next()
}

var _ subproc.Runner = (*scriptedRunner)(nil)

var _ = Describe("brancher StashPop failure paths", func() {
	It("returns the pop error when the failure leaves no unmerged paths", func() {
		runner := &scriptedRunner{
			results: []scriptedResult{
				{out: []byte("On master: " + specBookkeepingStashMessage)},
				{err: stderrors.New("pop failed")},
				{out: []byte("")},
			},
		}

		b := NewBrancher(withBrancherRunner(runner))

		err := b.StashPop(context.Background())
		Expect(err).To(MatchError(ContainSubstring("restore stashed bookkeeping")))
	})
})
