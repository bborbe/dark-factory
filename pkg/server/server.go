// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"

	"github.com/bborbe/run"
)

//counterfeiter:generate -o ../../mocks/server.go --fake-name Server . Server

// Server provides HTTP endpoints for status and monitoring.
type Server interface {
	ListenAndServe(ctx context.Context) error
}

// server implements Server by wrapping a run.Func from libhttp.NewServer.
type server struct {
	runFunc run.Func
}

// NewServer creates a new Server from a run.Func (typically from libhttp.NewServer).
func NewServer(runFunc run.Func) Server {
	return &server{
		runFunc: runFunc,
	}
}

// ListenAndServe executes the underlying run.Func.
func (s *server) ListenAndServe(ctx context.Context) error {
	return s.runFunc(ctx)
}
