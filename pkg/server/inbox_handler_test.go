// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server_test

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/server"
)

var _ = Describe("InboxHandler", func() {
	var (
		tempDir  string
		inboxDir string
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "inbox-handler-test-*")
		Expect(err).NotTo(HaveOccurred())

		inboxDir = filepath.Join(tempDir, "inbox")
		err = os.MkdirAll(inboxDir, 0750)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		_ = os.RemoveAll(tempDir)
	})

	Describe("GET /api/v1/inbox", func() {
		It("returns list of .md files", func() {
			// Create test files
			file1 := filepath.Join(inboxDir, "test1.md")
			file2 := filepath.Join(inboxDir, "test2.md")
			err := os.WriteFile(file1, []byte("# Test 1"), 0600)
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(file2, []byte("# Test 2"), 0600)
			Expect(err).NotTo(HaveOccurred())

			req := httptest.NewRequest("GET", "/api/v1/inbox", nil)
			w := httptest.NewRecorder()

			handler := server.NewInboxHandler(inboxDir)
			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(200))
			Expect(w.Header().Get("Content-Type")).To(Equal("application/json"))

			var response server.InboxListResponse
			err = json.NewDecoder(w.Body).Decode(&response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Files).To(HaveLen(2))
			Expect(response.Files[0].Name).To(Equal("test1.md"))
			Expect(response.Files[1].Name).To(Equal("test2.md"))
		})

		It("skips non-.md files", func() {
			// Create test files
			mdFile := filepath.Join(inboxDir, "test.md")
			txtFile := filepath.Join(inboxDir, "test.txt")
			err := os.WriteFile(mdFile, []byte("# Test"), 0600)
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(txtFile, []byte("text"), 0600)
			Expect(err).NotTo(HaveOccurred())

			req := httptest.NewRequest("GET", "/api/v1/inbox", nil)
			w := httptest.NewRecorder()

			handler := server.NewInboxHandler(inboxDir)
			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(200))

			var response server.InboxListResponse
			err = json.NewDecoder(w.Body).Decode(&response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Files).To(HaveLen(1))
			Expect(response.Files[0].Name).To(Equal("test.md"))
		})

		It("skips subdirectories", func() {
			// Create subdirectory
			subdir := filepath.Join(inboxDir, "subdir")
			err := os.MkdirAll(subdir, 0750)
			Expect(err).NotTo(HaveOccurred())

			req := httptest.NewRequest("GET", "/api/v1/inbox", nil)
			w := httptest.NewRecorder()

			handler := server.NewInboxHandler(inboxDir)
			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(200))

			var response server.InboxListResponse
			err = json.NewDecoder(w.Body).Decode(&response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Files).To(HaveLen(0))
		})

		It("returns empty list for empty inbox", func() {
			req := httptest.NewRequest("GET", "/api/v1/inbox", nil)
			w := httptest.NewRecorder()

			handler := server.NewInboxHandler(inboxDir)
			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(200))

			var response server.InboxListResponse
			err := json.NewDecoder(w.Body).Decode(&response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Files).To(HaveLen(0))
		})

		It("returns method not allowed for POST", func() {
			req := httptest.NewRequest("POST", "/api/v1/inbox", nil)
			w := httptest.NewRecorder()

			handler := server.NewInboxHandler(inboxDir)
			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(405))
		})
	})
})
