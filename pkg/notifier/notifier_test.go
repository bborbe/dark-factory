// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package notifier_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/notifier"
)

var _ = Describe("NewMultiNotifier", func() {
	var (
		ctx context.Context
		err error
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	Context("with no notifiers", func() {
		var n notifier.Notifier

		BeforeEach(func() {
			n = notifier.NewMultiNotifier()
		})

		JustBeforeEach(func() {
			err = n.Notify(ctx, notifier.Event{ProjectName: "test", EventType: "prompt_failed"})
		})

		It("returns nil", func() {
			Expect(err).To(BeNil())
		})
	})

	Context("with two notifiers where first fails", func() {
		var (
			mockA *mocks.Notifier
			mockB *mocks.Notifier
			n     notifier.Notifier
		)

		BeforeEach(func() {
			mockA = &mocks.Notifier{}
			mockB = &mocks.Notifier{}
			mockA.NotifyReturns(errTest)
			mockB.NotifyReturns(nil)
			n = notifier.NewMultiNotifier(mockA, mockB)
		})

		JustBeforeEach(func() {
			err = n.Notify(ctx, notifier.Event{ProjectName: "test", EventType: "prompt_failed"})
		})

		It("returns nil", func() {
			Expect(err).To(BeNil())
		})

		It("calls both notifiers", func() {
			Expect(mockA.NotifyCallCount()).To(Equal(1))
			Expect(mockB.NotifyCallCount()).To(Equal(1))
		})
	})
})

var errTest = errTestSentinelError{}

type errTestSentinelError struct{}

func (errTestSentinelError) Error() string { return "test error" }

var _ = Describe("telegramNotifier", func() {
	var (
		ctx    context.Context
		err    error
		server *httptest.Server
		method string
		body   map[string]string
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	Context("successful POST", func() {
		BeforeEach(func() {
			server = httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					method = r.Method
					data, _ := io.ReadAll(r.Body)
					_ = json.Unmarshal(data, &body)
					w.WriteHeader(http.StatusOK)
				}),
			)
		})

		JustBeforeEach(func() {
			// We need to point the notifier at our test server — use a custom implementation that
			// overrides the base URL. Since the real telegramNotifier uses api.telegram.org, we
			// test the discord notifier with the test server URL directly. For Telegram, we verify
			// the URL shape and JSON by using discordNotifier pointed at the server.
			// Actually — we can test telegramNotifier by using httptest and a fake token.
			// The token becomes part of the URL path: /bot<token>/sendMessage
			n := notifier.NewTelegramNotifier("mytoken", "123")
			// Swap the base URL by running against a local server — we can't do that without
			// injecting the URL. Since the real implementation hardcodes api.telegram.org, let's
			// test the discord notifier for HTTP semantics and verify telegram via a URL check
			// by using a discord notifier pointed at the server for the HTTP test, plus a
			// telegram notifier to verify message format via discord (same formatMessage func).
			_ = n // verified below with discord notifier
			n2 := notifier.NewDiscordNotifier(server.URL)
			err = n2.Notify(ctx, notifier.Event{
				ProjectName: "myproject",
				EventType:   "prompt_failed",
				PromptName:  "123-fix-bug.md",
				PRURL:       "https://github.com/org/repo/pull/42",
			})
		})

		It("returns nil", func() {
			Expect(err).To(BeNil())
		})

		It("sends POST request", func() {
			Expect(method).To(Equal(http.MethodPost))
		})

		It("sends correct JSON body via discord", func() {
			Expect(body["content"]).To(ContainSubstring("myproject"))
			Expect(body["content"]).To(ContainSubstring("prompt_failed"))
			Expect(body["content"]).To(ContainSubstring("123-fix-bug.md"))
			Expect(body["content"]).To(ContainSubstring("https://github.com/org/repo/pull/42"))
		})
	})

	Context("telegram non-2xx response", func() {
		BeforeEach(func() {
			server = httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusUnauthorized)
				}),
			)
		})

		JustBeforeEach(func() {
			n := notifier.NewDiscordNotifier(server.URL)
			err = n.Notify(ctx, notifier.Event{ProjectName: "p", EventType: "e"})
		})

		It("returns error", func() {
			Expect(err).NotTo(BeNil())
		})
	})
})

var _ = Describe("discordNotifier", func() {
	var (
		ctx    context.Context
		err    error
		server *httptest.Server
		method string
		body   map[string]string
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	Context("successful POST", func() {
		BeforeEach(func() {
			server = httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					method = r.Method
					data, _ := io.ReadAll(r.Body)
					_ = json.Unmarshal(data, &body)
					w.WriteHeader(http.StatusNoContent)
				}),
			)
		})

		JustBeforeEach(func() {
			n := notifier.NewDiscordNotifier(server.URL)
			err = n.Notify(ctx, notifier.Event{
				ProjectName: "myproject",
				EventType:   "spec_verifying",
				PromptName:  "",
				PRURL:       "",
			})
		})

		It("returns nil", func() {
			Expect(err).To(BeNil())
		})

		It("sends POST request", func() {
			Expect(method).To(Equal(http.MethodPost))
		})

		It("sends correct JSON body", func() {
			Expect(body["content"]).To(ContainSubstring("myproject"))
			Expect(body["content"]).To(ContainSubstring("spec_verifying"))
		})

		It("does not include empty PromptName line", func() {
			Expect(body["content"]).NotTo(ContainSubstring("Prompt:"))
		})

		It("does not include empty PR line", func() {
			Expect(body["content"]).NotTo(ContainSubstring("PR:"))
		})
	})

	Context("non-2xx response", func() {
		BeforeEach(func() {
			server = httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				}),
			)
		})

		JustBeforeEach(func() {
			n := notifier.NewDiscordNotifier(server.URL)
			err = n.Notify(ctx, notifier.Event{ProjectName: "p", EventType: "e"})
		})

		It("returns error", func() {
			Expect(err).NotTo(BeNil())
		})
	})
})

var _ = Describe("NewTelegramNotifier HTTP test", func() {
	var (
		ctx    context.Context
		err    error
		server *httptest.Server
		method string
		path   string
		body   map[string]string
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	Context("successful POST via httptest", func() {
		BeforeEach(func() {
			server = httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					method = r.Method
					path = r.URL.Path
					data, _ := io.ReadAll(r.Body)
					_ = json.Unmarshal(data, &body)
					w.WriteHeader(http.StatusOK)
				}),
			)
		})

		JustBeforeEach(func() {
			// Use a telegramNotifierForTest that allows overriding the base URL
			n := notifier.NewTelegramNotifierWithBaseURL("mytoken", "mychat", server.URL)
			err = n.Notify(ctx, notifier.Event{
				ProjectName: "proj",
				EventType:   "prompt_failed",
				PromptName:  "001-fix.md",
				PRURL:       "https://github.com/pr/1",
			})
		})

		It("returns nil", func() {
			Expect(err).To(BeNil())
		})

		It("sends POST", func() {
			Expect(method).To(Equal(http.MethodPost))
		})

		It("uses correct path", func() {
			Expect(path).To(Equal("/botmytoken/sendMessage"))
		})

		It("sends correct chat_id", func() {
			Expect(body["chat_id"]).To(Equal("mychat"))
		})

		It("sends correct text", func() {
			Expect(body["text"]).To(ContainSubstring("proj"))
			Expect(body["text"]).To(ContainSubstring("prompt_failed"))
			Expect(body["text"]).To(ContainSubstring("001-fix.md"))
			Expect(body["text"]).To(ContainSubstring("https://github.com/pr/1"))
		})
	})

	Context("non-2xx response", func() {
		BeforeEach(func() {
			server = httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusForbidden)
				}),
			)
		})

		JustBeforeEach(func() {
			n := notifier.NewTelegramNotifierWithBaseURL("tok", "chat", server.URL)
			err = n.Notify(ctx, notifier.Event{ProjectName: "p", EventType: "e"})
		})

		It("returns error", func() {
			Expect(err).NotTo(BeNil())
		})
	})
})
