// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/server"
	"github.com/bborbe/dark-factory/pkg/status"
)

var _ = Describe("Server", func() {
	var (
		mockStatusChecker *mocks.Checker
		srv               server.Server
		ctx               context.Context
		cancel            context.CancelFunc
	)

	BeforeEach(func() {
		mockStatusChecker = &mocks.Checker{}
		srv = server.NewServer(":18080", mockStatusChecker)
		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() {
		cancel()
	})

	Describe("Server lifecycle", func() {
		It("starts and stops gracefully", func() {
			done := make(chan error)
			go func() {
				done <- srv.ListenAndServe(ctx)
			}()

			// Give server time to start
			time.Sleep(100 * time.Millisecond)

			// Cancel context
			cancel()

			// Wait for server to stop
			err := <-done
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Health endpoint", func() {
		It("returns 200 OK with status ok", func() {
			req := httptest.NewRequest("GET", "/health", nil)
			w := httptest.NewRecorder()

			handler := server.NewHealthHandler()
			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(200))
			Expect(w.Header().Get("Content-Type")).To(Equal("application/json"))
			Expect(w.Body.String()).To(Equal(`{"status":"ok"}`))
		})

		It("returns method not allowed for POST", func() {
			req := httptest.NewRequest("POST", "/health", nil)
			w := httptest.NewRecorder()

			handler := server.NewHealthHandler()
			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(405))
		})
	})

	Describe("Status endpoint", func() {
		It("returns status from StatusChecker", func() {
			expectedStatus := &status.Status{
				Daemon:         "running",
				CurrentPrompt:  "test-prompt.md",
				ExecutingSince: "2m30s",
				Container:      "dark-factory-test",
				QueueCount:     3,
				QueuedPrompts:  []string{"a.md", "b.md", "c.md"},
				CompletedCount: 10,
				IdeasCount:     5,
			}
			mockStatusChecker.GetStatusReturns(expectedStatus, nil)

			req := httptest.NewRequest("GET", "/api/v1/status", nil)
			w := httptest.NewRecorder()

			handler := server.NewStatusHandler(mockStatusChecker)
			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(200))
			Expect(w.Header().Get("Content-Type")).To(Equal("application/json"))

			var result status.Status
			err := json.NewDecoder(w.Body).Decode(&result)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(*expectedStatus))
		})

		It("returns method not allowed for POST", func() {
			req := httptest.NewRequest("POST", "/api/v1/status", nil)
			w := httptest.NewRecorder()

			handler := server.NewStatusHandler(mockStatusChecker)
			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(405))
		})
	})

	Describe("Status endpoint error handling", func() {
		It("returns 500 when StatusChecker fails", func() {
			mockStatusChecker.GetStatusReturns(nil, context.DeadlineExceeded)

			req := httptest.NewRequest("GET", "/api/v1/status", nil)
			w := httptest.NewRecorder()

			handler := server.NewStatusHandler(mockStatusChecker)
			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(500))
		})
	})

	Describe("Queue endpoint", func() {
		It("returns queued prompts from StatusChecker", func() {
			expectedQueue := []status.QueuedPrompt{
				{Name: "test1.md", Title: "Test 1", Size: 1234},
				{Name: "test2.md", Title: "Test 2", Size: 5678},
			}
			mockStatusChecker.GetQueuedPromptsReturns(expectedQueue, nil)

			req := httptest.NewRequest("GET", "/api/v1/queue", nil)
			w := httptest.NewRecorder()

			handler := server.NewQueueHandler(mockStatusChecker)
			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(200))
			Expect(w.Header().Get("Content-Type")).To(Equal("application/json"))

			var result []status.QueuedPrompt
			err := json.NewDecoder(w.Body).Decode(&result)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(expectedQueue))
		})
	})

	Describe("Queue endpoint error handling", func() {
		It("returns 500 when StatusChecker fails", func() {
			mockStatusChecker.GetQueuedPromptsReturns(nil, context.DeadlineExceeded)

			req := httptest.NewRequest("GET", "/api/v1/queue", nil)
			w := httptest.NewRecorder()

			handler := server.NewQueueHandler(mockStatusChecker)
			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(500))
		})

		It("returns method not allowed for POST", func() {
			req := httptest.NewRequest("POST", "/api/v1/queue", nil)
			w := httptest.NewRecorder()

			handler := server.NewQueueHandler(mockStatusChecker)
			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(405))
		})
	})

	Describe("Completed endpoint", func() {
		It("returns completed prompts with default limit", func() {
			completedTime := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
			expectedCompleted := []status.CompletedPrompt{
				{Name: "test1.md", CompletedAt: completedTime},
			}
			mockStatusChecker.GetCompletedPromptsReturns(expectedCompleted, nil)

			req := httptest.NewRequest("GET", "/api/v1/completed", nil)
			w := httptest.NewRecorder()

			handler := server.NewCompletedHandler(mockStatusChecker)
			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(200))

			var result []status.CompletedPrompt
			err := json.NewDecoder(w.Body).Decode(&result)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(expectedCompleted))

			// Verify default limit of 10 was used
			Expect(mockStatusChecker.GetCompletedPromptsCallCount()).To(Equal(1))
			_, limit := mockStatusChecker.GetCompletedPromptsArgsForCall(0)
			Expect(limit).To(Equal(10))
		})

		It("respects limit query parameter", func() {
			mockStatusChecker.GetCompletedPromptsReturns([]status.CompletedPrompt{}, nil)

			req := httptest.NewRequest("GET", "/api/v1/completed?limit=5", nil)
			w := httptest.NewRecorder()

			handler := server.NewCompletedHandler(mockStatusChecker)
			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(200))

			// Verify limit of 5 was used
			Expect(mockStatusChecker.GetCompletedPromptsCallCount()).To(Equal(1))
			_, limit := mockStatusChecker.GetCompletedPromptsArgsForCall(0)
			Expect(limit).To(Equal(5))
		})

		It("ignores invalid limit parameter", func() {
			mockStatusChecker.GetCompletedPromptsReturns([]status.CompletedPrompt{}, nil)

			req := httptest.NewRequest("GET", "/api/v1/completed?limit=invalid", nil)
			w := httptest.NewRecorder()

			handler := server.NewCompletedHandler(mockStatusChecker)
			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(200))

			// Verify default limit was used
			Expect(mockStatusChecker.GetCompletedPromptsCallCount()).To(Equal(1))
			_, limit := mockStatusChecker.GetCompletedPromptsArgsForCall(0)
			Expect(limit).To(Equal(10))
		})

		It("returns 500 when StatusChecker fails", func() {
			mockStatusChecker.GetCompletedPromptsReturns(nil, context.DeadlineExceeded)

			req := httptest.NewRequest("GET", "/api/v1/completed", nil)
			w := httptest.NewRecorder()

			handler := server.NewCompletedHandler(mockStatusChecker)
			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(500))
		})

		It("returns method not allowed for POST", func() {
			req := httptest.NewRequest("POST", "/api/v1/completed", nil)
			w := httptest.NewRecorder()

			handler := server.NewCompletedHandler(mockStatusChecker)
			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(405))
		})
	})
})
