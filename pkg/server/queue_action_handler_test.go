// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server_test

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"

	libhttp "github.com/bborbe/http"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/server"
)

var _ = Describe("QueueActionHandler", func() {
	var (
		tempDir           string
		inboxDir          string
		queueDir          string
		mockPromptManager *mocks.Manager
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "queue-action-handler-test-*")
		Expect(err).NotTo(HaveOccurred())

		inboxDir = filepath.Join(tempDir, "inbox")
		queueDir = filepath.Join(tempDir, "queue")
		err = os.MkdirAll(inboxDir, 0750)
		Expect(err).NotTo(HaveOccurred())
		err = os.MkdirAll(queueDir, 0750)
		Expect(err).NotTo(HaveOccurred())

		mockPromptManager = &mocks.Manager{}
		mockPromptManager.NormalizeFilenamesReturns([]prompt.Rename{}, nil)
	})

	AfterEach(func() {
		_ = os.RemoveAll(tempDir)
	})

	Describe("POST /api/v1/queue/action", func() {
		It("queues a single file", func() {
			// Create test file in inbox
			testFile := filepath.Join(inboxDir, "test.md")
			err := os.WriteFile(testFile, []byte("# Test Prompt"), 0600)
			Expect(err).NotTo(HaveOccurred())

			reqBody := server.QueueRequest{File: "test.md"}
			body, err := json.Marshal(reqBody)
			Expect(err).NotTo(HaveOccurred())

			req := httptest.NewRequest("POST", "/api/v1/queue/action", bytes.NewReader(body))
			w := httptest.NewRecorder()

			handler := libhttp.NewErrorHandler(
				server.NewQueueActionHandler(inboxDir, queueDir, mockPromptManager),
			)
			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(200))
			Expect(w.Header().Get("Content-Type")).To(Equal("application/json"))

			var response server.QueueResponse
			err = json.NewDecoder(w.Body).Decode(&response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Queued).To(HaveLen(1))
			Expect(response.Queued[0].Old).To(Equal("test.md"))
			Expect(response.Queued[0].New).To(Equal("test.md"))

			// Verify file moved to queue
			_, err = os.Stat(testFile)
			Expect(os.IsNotExist(err)).To(BeTrue())

			queuedFile := filepath.Join(queueDir, "test.md")
			_, err = os.Stat(queuedFile)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns 404 for nonexistent file", func() {
			reqBody := server.QueueRequest{File: "nonexistent.md"}
			body, err := json.Marshal(reqBody)
			Expect(err).NotTo(HaveOccurred())

			req := httptest.NewRequest("POST", "/api/v1/queue/action", bytes.NewReader(body))
			w := httptest.NewRecorder()

			handler := libhttp.NewErrorHandler(
				server.NewQueueActionHandler(inboxDir, queueDir, mockPromptManager),
			)
			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(404))
		})

		It("returns 400 for missing file parameter", func() {
			reqBody := server.QueueRequest{File: ""}
			body, err := json.Marshal(reqBody)
			Expect(err).NotTo(HaveOccurred())

			req := httptest.NewRequest("POST", "/api/v1/queue/action", bytes.NewReader(body))
			w := httptest.NewRecorder()

			handler := libhttp.NewErrorHandler(
				server.NewQueueActionHandler(inboxDir, queueDir, mockPromptManager),
			)
			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(400))
		})

		It("returns 400 for invalid JSON", func() {
			req := httptest.NewRequest(
				"POST",
				"/api/v1/queue/action",
				bytes.NewReader([]byte("invalid json")),
			)
			w := httptest.NewRecorder()

			handler := libhttp.NewErrorHandler(
				server.NewQueueActionHandler(inboxDir, queueDir, mockPromptManager),
			)
			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(400))
		})

		It("returns method not allowed for GET", func() {
			req := httptest.NewRequest("GET", "/api/v1/queue/action", nil)
			w := httptest.NewRecorder()

			handler := libhttp.NewErrorHandler(
				server.NewQueueActionHandler(inboxDir, queueDir, mockPromptManager),
			)
			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(405))
		})

		It("handles normalized filename", func() {
			// Create test file in inbox
			testFile := filepath.Join(inboxDir, "test.md")
			err := os.WriteFile(testFile, []byte("# Test Prompt"), 0600)
			Expect(err).NotTo(HaveOccurred())

			// Mock normalization
			mockPromptManager.NormalizeFilenamesReturns([]prompt.Rename{
				{
					OldPath: filepath.Join(queueDir, "test.md"),
					NewPath: filepath.Join(queueDir, "001-test.md"),
				},
			}, nil)

			reqBody := server.QueueRequest{File: "test.md"}
			body, err := json.Marshal(reqBody)
			Expect(err).NotTo(HaveOccurred())

			req := httptest.NewRequest("POST", "/api/v1/queue/action", bytes.NewReader(body))
			w := httptest.NewRecorder()

			handler := libhttp.NewErrorHandler(
				server.NewQueueActionHandler(inboxDir, queueDir, mockPromptManager),
			)
			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(200))

			var response server.QueueResponse
			err = json.NewDecoder(w.Body).Decode(&response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Queued).To(HaveLen(1))
			Expect(response.Queued[0].Old).To(Equal("test.md"))
			Expect(response.Queued[0].New).To(Equal("001-test.md"))
		})
	})

	Describe("POST /api/v1/queue/action/all", func() {
		It("queues all .md files", func() {
			// Create test files in inbox
			testFile1 := filepath.Join(inboxDir, "test1.md")
			testFile2 := filepath.Join(inboxDir, "test2.md")
			err := os.WriteFile(testFile1, []byte("# Test 1"), 0600)
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(testFile2, []byte("# Test 2"), 0600)
			Expect(err).NotTo(HaveOccurred())

			req := httptest.NewRequest("POST", "/api/v1/queue/action/all", nil)
			w := httptest.NewRecorder()

			handler := libhttp.NewErrorHandler(
				server.NewQueueActionHandler(inboxDir, queueDir, mockPromptManager),
			)
			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(200))
			Expect(w.Header().Get("Content-Type")).To(Equal("application/json"))

			var response server.QueueResponse
			err = json.NewDecoder(w.Body).Decode(&response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Queued).To(HaveLen(2))

			// Verify files moved to queue
			_, err = os.Stat(testFile1)
			Expect(os.IsNotExist(err)).To(BeTrue())
			_, err = os.Stat(testFile2)
			Expect(os.IsNotExist(err)).To(BeTrue())
		})

		It("skips non-.md files", func() {
			// Create test files
			mdFile := filepath.Join(inboxDir, "test.md")
			txtFile := filepath.Join(inboxDir, "test.txt")
			err := os.WriteFile(mdFile, []byte("# Test"), 0600)
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(txtFile, []byte("text"), 0600)
			Expect(err).NotTo(HaveOccurred())

			req := httptest.NewRequest("POST", "/api/v1/queue/action/all", nil)
			w := httptest.NewRecorder()

			handler := libhttp.NewErrorHandler(
				server.NewQueueActionHandler(inboxDir, queueDir, mockPromptManager),
			)
			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(200))

			var response server.QueueResponse
			err = json.NewDecoder(w.Body).Decode(&response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Queued).To(HaveLen(1))
			Expect(response.Queued[0].Old).To(Equal("test.md"))

			// Verify txt file not moved
			_, err = os.Stat(txtFile)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns empty list for empty inbox", func() {
			req := httptest.NewRequest("POST", "/api/v1/queue/action/all", nil)
			w := httptest.NewRecorder()

			handler := libhttp.NewErrorHandler(
				server.NewQueueActionHandler(inboxDir, queueDir, mockPromptManager),
			)
			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(200))

			var response server.QueueResponse
			err := json.NewDecoder(w.Body).Decode(&response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Queued).To(HaveLen(0))
		})

		It("returns method not allowed for GET", func() {
			req := httptest.NewRequest("GET", "/api/v1/queue/action/all", nil)
			w := httptest.NewRecorder()

			handler := libhttp.NewErrorHandler(
				server.NewQueueActionHandler(inboxDir, queueDir, mockPromptManager),
			)
			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(405))
		})
	})
})
