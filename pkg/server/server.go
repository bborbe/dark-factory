// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/status"
)

// Server provides HTTP endpoints for status and monitoring.
//
//counterfeiter:generate -o ../../mocks/server.go --fake-name Server . Server
type Server interface {
	ListenAndServe(ctx context.Context) error
}

// server implements Server.
type server struct {
	addr          string
	statusChecker status.Checker
	inboxDir      string
	queueDir      string
	promptManager prompt.Manager
}

// NewServer creates a new Server.
func NewServer(
	addr string,
	statusChecker status.Checker,
	inboxDir string,
	queueDir string,
	promptManager prompt.Manager,
) Server {
	return &server{
		addr:          addr,
		statusChecker: statusChecker,
		inboxDir:      inboxDir,
		queueDir:      queueDir,
		promptManager: promptManager,
	}
}

// ListenAndServe starts the HTTP server and blocks until context is cancelled.
func (s *server) ListenAndServe(ctx context.Context) error {
	mux := http.NewServeMux()

	// Register handlers
	mux.HandleFunc("/health", NewHealthHandler())
	mux.HandleFunc("/api/v1/status", NewStatusHandler(s.statusChecker))
	mux.HandleFunc("/api/v1/queue", NewQueueHandler(s.statusChecker))
	mux.HandleFunc(
		"/api/v1/queue/action",
		NewQueueActionHandler(s.inboxDir, s.queueDir, s.promptManager),
	)
	mux.HandleFunc(
		"/api/v1/queue/action/all",
		NewQueueActionHandler(s.inboxDir, s.queueDir, s.promptManager),
	)
	mux.HandleFunc("/api/v1/inbox", NewInboxHandler(s.inboxDir))
	mux.HandleFunc("/api/v1/completed", NewCompletedHandler(s.statusChecker))

	httpServer := &http.Server{
		Addr:              s.addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		BaseContext: func(l net.Listener) context.Context {
			return ctx
		},
	}

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		log.Printf("dark-factory: HTTP server listening on %s", s.addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		log.Printf("dark-factory: shutting down HTTP server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return errors.Wrap(ctx, err, "shutdown HTTP server")
		}
		return nil
	case err := <-errChan:
		return errors.Wrap(ctx, err, "HTTP server error")
	}
}
