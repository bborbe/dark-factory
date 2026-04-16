// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package formatter_test

import (
	"bytes"
	"context"
	"io"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/formatter"
)

func processLine(jsonLine string) (string, string) {
	f := formatter.NewFormatter()
	var rawBuf, fmtBuf bytes.Buffer
	ctx := context.Background()
	err := f.ProcessStream(ctx, strings.NewReader(jsonLine), &rawBuf, &fmtBuf)
	Expect(err).NotTo(HaveOccurred())
	return rawBuf.String(), fmtBuf.String()
}

func processLines(lines ...string) (string, string) {
	f := formatter.NewFormatter()
	var rawBuf, fmtBuf bytes.Buffer
	ctx := context.Background()
	input := strings.Join(lines, "\n") + "\n"
	err := f.ProcessStream(ctx, strings.NewReader(input), &rawBuf, &fmtBuf)
	Expect(err).NotTo(HaveOccurred())
	return rawBuf.String(), fmtBuf.String()
}

var _ = Describe("Formatter", func() {
	Describe("7a. System init event", func() {
		It("renders [init] with session, model, cwd, tools", func() {
			raw, formatted := processLine(
				`{"type":"system","subtype":"init","session_id":"abcdefghij","model":"claude-opus-4-6","cwd":"/workspace","tools":["Bash","Read"]}`,
			)
			Expect(
				raw,
			).To(Equal(`{"type":"system","subtype":"init","session_id":"abcdefghij","model":"claude-opus-4-6","cwd":"/workspace","tools":["Bash","Read"]}` + "\n"))
			Expect(formatted).To(ContainSubstring("[init] session=abcdefgh"))
			Expect(formatted).To(ContainSubstring("model=claude-opus-4-6"))
			Expect(formatted).To(ContainSubstring("cwd=/workspace"))
			Expect(formatted).To(ContainSubstring("tools=2"))
			Expect(formatted).To(HavePrefix("["))
		})
	})

	Describe("7b. Assistant text message", func() {
		It("renders text content", func() {
			raw, formatted := processLine(
				`{"type":"assistant","message":{"content":[{"type":"text","text":"Hello world"}]}}`,
			)
			Expect(formatted).To(ContainSubstring("Hello world"))
			Expect(raw).To(ContainSubstring(`"Hello world"`))
		})

		It("does not render blank text", func() {
			_, formatted := processLine(
				`{"type":"assistant","message":{"content":[{"type":"text","text":"   "}]}}`,
			)
			Expect(formatted).To(BeEmpty())
		})
	})

	Describe("7c. Assistant thinking message", func() {
		It("renders thinking with 💭 glyph", func() {
			_, formatted := processLine(
				`{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"Deep reasoning..."}]}}`,
			)
			Expect(formatted).To(ContainSubstring("💭"))
			Expect(formatted).To(ContainSubstring("Deep reasoning"))
		})
	})

	Describe("7d. tool_use: Bash", func() {
		It("renders as $ <command>", func() {
			_, formatted := processLine(
				`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls -la"}}]}}`,
			)
			Expect(formatted).To(ContainSubstring("$ ls -la"))
			Expect(formatted).NotTo(ContainSubstring("→"))
		})
	})

	Describe("7e. tool_use shapes", func() {
		It("Read renders [read]", func() {
			_, formatted := processLine(
				`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/foo/bar.go"}}]}}`,
			)
			Expect(formatted).To(ContainSubstring("[read] /foo/bar.go"))
		})

		It("Write renders [write]", func() {
			_, formatted := processLine(
				`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Write","input":{"file_path":"/foo/bar.go","content":"hello"}}]}}`,
			)
			Expect(formatted).To(ContainSubstring("[write] /foo/bar.go"))
		})

		It("Edit renders [edit]", func() {
			_, formatted := processLine(
				`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Edit","input":{"file_path":"/foo/bar.go","old_string":"old","new_string":"new"}}]}}`,
			)
			Expect(formatted).To(ContainSubstring("[edit] /foo/bar.go"))
		})

		It("Grep renders [grep]", func() {
			_, formatted := processLine(
				`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Grep","input":{"pattern":"func.*Error"}}]}}`,
			)
			Expect(formatted).To(ContainSubstring("[grep] func.*Error"))
		})

		It("Glob renders [glob]", func() {
			_, formatted := processLine(
				`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Glob","input":{"pattern":"**/*.go"}}]}}`,
			)
			Expect(formatted).To(ContainSubstring("[glob] **/*.go"))
		})
	})

	Describe("7f. tool_use: misc tools", func() {
		It("Task renders [agent:]", func() {
			_, formatted := processLine(
				`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Task","input":{"description":"do something","subagent_type":"go-quality","prompt":"review this"}}]}}`,
			)
			Expect(formatted).To(ContainSubstring("[agent:"))
		})

		It("ToolSearch renders [toolsearch]", func() {
			_, formatted := processLine(
				`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"ToolSearch","input":{"query":"select:Read"}}]}}`,
			)
			Expect(formatted).To(ContainSubstring("[toolsearch]"))
		})

		It("AskUserQuestion renders [ask]", func() {
			_, formatted := processLine(
				`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"AskUserQuestion","input":{"question":"What do you want?"}}]}}`,
			)
			Expect(formatted).To(ContainSubstring("[ask]"))
		})

		It("Skill renders [skill]", func() {
			_, formatted := processLine(
				`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Skill","input":{"skill":"commit","args":"-m foo"}}]}}`,
			)
			Expect(formatted).To(ContainSubstring("[skill]"))
		})

		It("WebFetch renders [webfetch]", func() {
			_, formatted := processLine(
				`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"WebFetch","input":{"url":"https://example.com"}}]}}`,
			)
			Expect(formatted).To(ContainSubstring("[webfetch]"))
		})

		It("WebSearch renders [websearch]", func() {
			_, formatted := processLine(
				`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"WebSearch","input":{"query":"go errors"}}]}}`,
			)
			Expect(formatted).To(ContainSubstring("[websearch]"))
		})

		It("TodoWrite renders [todo]", func() {
			_, formatted := processLine(
				`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"TodoWrite","input":{"todos":[{"status":"completed","content":"done task"},{"status":"in_progress","content":"current task"}]}}]}}`,
			)
			Expect(formatted).To(ContainSubstring("[todo]"))
		})

		It("mcp__ tool renders [mcp:]", func() {
			_, formatted := processLine(
				`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"mcp__myserver__mymethod","input":{}}]}}`,
			)
			Expect(formatted).To(ContainSubstring("[mcp:"))
		})
	})

	Describe("7g. tool_result success — correlated Read", func() {
		It("renders → N lines read", func() {
			useJSON := `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/foo.go"}}]}}`
			resultJSON := `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"line1\nline2\nline3\n"}]}}`
			_, formatted := processLines(useJSON, resultJSON)
			Expect(formatted).To(ContainSubstring("→"))
			Expect(formatted).To(ContainSubstring("lines read"))
		})
	})

	Describe("7h. tool_result error — correlated Bash", func() {
		It("renders ⚠ with bash failed output", func() {
			useJSON := `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t2","name":"Bash","input":{"command":"bad command"}}]}}`
			resultJSON := `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t2","is_error":true,"content":"error: command not found\nline2\nline3"}]}}`
			_, formatted := processLines(useJSON, resultJSON)
			Expect(formatted).To(ContainSubstring("⚠"))
		})
	})

	Describe("7h. tool_result Bash looks-failed (keywords in output)", func() {
		It("renders ⚠ [bash output] when text contains error keywords", func() {
			useJSON := `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t3","name":"Bash","input":{"command":"make test"}}]}}`
			resultJSON := `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t3","content":"exit code 1\nline2\nline3"}]}}`
			_, formatted := processLines(useJSON, resultJSON)
			Expect(formatted).To(ContainSubstring("⚠"))
			Expect(formatted).To(ContainSubstring("bash output"))
		})
	})

	Describe("7i. Orphan tool_result (unknown tool_use_id)", func() {
		It("does not panic and falls back gracefully", func() {
			resultJSON := `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"unknown-id","content":"some output"}]}}`
			_, formatted := processLine(resultJSON)
			// default handler is silent for unknown tool with no error; just no panic
			_ = formatted
		})
	})

	Describe("7i. Orphan tool_result with is_error", func() {
		It("renders error with (unknown tool) fallback", func() {
			resultJSON := `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"no-such-id","is_error":true,"content":"error text"}]}}`
			_, formatted := processLine(resultJSON)
			Expect(formatted).To(ContainSubstring("⚠"))
			Expect(formatted).To(ContainSubstring("(unknown tool)"))
		})
	})

	Describe("7j. Parse error fallback", func() {
		It("writes non-JSON line verbatim to both outputs", func() {
			f := formatter.NewFormatter()
			var rawBuf, fmtBuf bytes.Buffer
			ctx := context.Background()
			err := f.ProcessStream(ctx, strings.NewReader("this is not json\n"), &rawBuf, &fmtBuf)
			Expect(err).NotTo(HaveOccurred())
			Expect(rawBuf.String()).To(Equal("this is not json\n"))
			Expect(fmtBuf.String()).To(ContainSubstring("this is not json"))
		})
	})

	Describe("7k. Unknown message type", func() {
		It("renders unknown type without panic", func() {
			_, formatted := processLine(`{"type":"future_type","data":"x"}`)
			Expect(formatted).To(ContainSubstring("unknown type"))
		})
	})

	Describe("7l. Missing field resilience", func() {
		It("does not panic on empty tool_use fields", func() {
			_, _ = processLine(
				`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"","name":"","input":{}}]}}`,
			)
		})
	})

	Describe("7m. Long text truncation", func() {
		It("truncates thinking block and adds ellipsis", func() {
			longThinking := strings.Repeat("x", 1000)
			line := `{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"` + longThinking + `"}]}}`
			_, formatted := processLine(line)
			// Should not have 1000 x's (truncated to 500)
			Expect(len(formatted)).To(BeNumerically("<", 600))
			Expect(formatted).To(ContainSubstring("…"))
		})
	})

	Describe("7n. Final result event", func() {
		It("renders result, duration, cost, tokens", func() {
			_, formatted := processLine(
				`{"type":"result","result":"success","duration_ms":12345,"total_cost_usd":0.0123,"usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":8000}}`,
			)
			Expect(formatted).To(ContainSubstring("success"))
			Expect(formatted).To(ContainSubstring("12345"))
			Expect(formatted).To(ContainSubstring("0.0123"))
			Expect(formatted).To(ContainSubstring("100"))
			Expect(formatted).To(ContainSubstring("50"))
		})
	})

	Describe("7o. Multi-line stream", func() {
		It("processes 5 sequential events", func() {
			lines := []string{
				`{"type":"system","subtype":"init","session_id":"abc","model":"m","cwd":"/","tools":[]}`,
				`{"type":"assistant","message":{"content":[{"type":"text","text":"hello"}]}}`,
				`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}}]}}`,
				`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"file.go\n"}]}}`,
				`{"type":"result","result":"success","duration_ms":1000,"total_cost_usd":0.001,"usage":{"input_tokens":10,"output_tokens":5}}`,
			}
			f := formatter.NewFormatter()
			var rawBuf, fmtBuf bytes.Buffer
			ctx := context.Background()
			err := f.ProcessStream(
				ctx,
				strings.NewReader(strings.Join(lines, "\n")+"\n"),
				&rawBuf,
				&fmtBuf,
			)
			Expect(err).NotTo(HaveOccurred())
			rawLines := strings.Split(strings.TrimRight(rawBuf.String(), "\n"), "\n")
			Expect(len(rawLines)).To(Equal(5))
			// Bash success result is silent; expect at least 4 formatted lines
			fmtLines := strings.Split(strings.TrimRight(fmtBuf.String(), "\n"), "\n")
			Expect(len(fmtLines)).To(BeNumerically(">=", 4))
		})
	})

	Describe("7p. Context cancellation", func() {
		It("returns nil and does not hang on cancelled context", func() {
			pr, pw := io.Pipe()
			ctx, cancel := context.WithCancel(context.Background())

			f := formatter.NewFormatter()
			var rawBuf, fmtBuf bytes.Buffer
			done := make(chan error, 1)
			go func() {
				done <- f.ProcessStream(ctx, pr, &rawBuf, &fmtBuf)
			}()

			_, _ = pw.Write(
				[]byte(
					`{"type":"system","subtype":"init","session_id":"abc","model":"m","cwd":"/","tools":[]}` + "\n",
				),
			)
			cancel()
			_ = pw.Close()

			err := <-done
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("7q. Line exceeds 10 MB scanner buffer", func() {
		It("writes truncation marker to both outputs without panic", func() {
			// Build an 11 MB line (no newlines)
			bigLine := strings.Repeat("x", 11*1024*1024)
			f := formatter.NewFormatter()
			var rawBuf, fmtBuf bytes.Buffer
			ctx := context.Background()
			err := f.ProcessStream(ctx, strings.NewReader(bigLine), &rawBuf, &fmtBuf)
			Expect(err).NotTo(HaveOccurred())
			Expect(rawBuf.String()).To(ContainSubstring("scanner error"))
			Expect(fmtBuf.String()).To(ContainSubstring("scanner error"))
		})
	})

	Describe("Read tool_use with offset and limit", func() {
		It("includes offset and limit in render", func() {
			_, formatted := processLine(
				`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/foo.go","offset":10,"limit":50}}]}}`,
			)
			Expect(formatted).To(ContainSubstring("[read] /foo.go"))
			Expect(formatted).To(ContainSubstring("offset=10"))
			Expect(formatted).To(ContainSubstring("limit=50"))
		})
	})

	Describe("Grep with path", func() {
		It("includes path in render", func() {
			_, formatted := processLine(
				`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Grep","input":{"pattern":"foo","path":"/workspace"}}]}}`,
			)
			Expect(formatted).To(ContainSubstring("[grep] foo"))
			Expect(formatted).To(ContainSubstring("path=/workspace"))
		})
	})

	Describe("mcp__ tool name parsing", func() {
		It("extracts server and method", func() {
			_, formatted := processLine(
				`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"mcp__context7__get-library-docs","input":{}}]}}`,
			)
			Expect(formatted).To(ContainSubstring("[mcp:context7] get-library-docs"))
		})
	})

	Describe("system event non-init subtype", func() {
		It("renders [system: <subtype>]", func() {
			_, formatted := processLine(`{"type":"system","subtype":"other_event"}`)
			Expect(formatted).To(ContainSubstring("[system: other_event]"))
		})
	})

	Describe("empty session_id", func() {
		It("uses (unknown) fallback", func() {
			_, formatted := processLine(
				`{"type":"system","subtype":"init","session_id":"","model":"m","cwd":"/","tools":[]}`,
			)
			Expect(formatted).To(ContainSubstring("session=(unknown)"))
		})
	})

	Describe("Task/Agent tool_result", func() {
		It("renders ← agent reply:", func() {
			useJSON := `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t5","name":"Task","input":{"description":"do it","subagent_type":"general","prompt":"help"}}]}}`
			resultJSON := `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t5","content":"Here is the result"}]}}`
			_, formatted := processLines(useJSON, resultJSON)
			Expect(formatted).To(ContainSubstring("←"))
			Expect(formatted).To(ContainSubstring("agent reply"))
		})
	})

	Describe("tool_result with content as array of blocks", func() {
		It("extracts text from block array", func() {
			useJSON := `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t6","name":"Read","input":{"file_path":"/foo.go"}}]}}`
			resultJSON := `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t6","content":[{"type":"text","text":"line1\nline2\n"}]}]}}`
			_, formatted := processLines(useJSON, resultJSON)
			Expect(formatted).To(ContainSubstring("→"))
			Expect(formatted).To(ContainSubstring("lines read"))
		})
	})

	Describe("AskUserQuestion with questions array", func() {
		It("renders [ask] with question from questions array", func() {
			_, formatted := processLine(
				`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"AskUserQuestion","input":{"questions":[{"question":"Are you sure?"}]}}]}}`,
			)
			Expect(formatted).To(ContainSubstring("[ask]"))
			Expect(formatted).To(ContainSubstring("Are you sure?"))
		})
	})

	Describe("Glob tool_result", func() {
		It("renders → N files", func() {
			useJSON := `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t7","name":"Glob","input":{"pattern":"**/*.go"}}]}}`
			resultJSON := `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t7","content":"file1.go\nfile2.go\nfile3.go\n"}]}}`
			_, formatted := processLines(useJSON, resultJSON)
			Expect(formatted).To(ContainSubstring("→"))
			Expect(formatted).To(ContainSubstring("files"))
		})
	})

	Describe("Write tool_result", func() {
		It("renders → first line on success", func() {
			useJSON := `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t8","name":"Write","input":{"file_path":"/foo.go","content":"x"}}]}}`
			resultJSON := `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t8","content":"File written successfully"}]}}`
			_, formatted := processLines(useJSON, resultJSON)
			Expect(formatted).To(ContainSubstring("→"))
		})
	})

	Describe("rate_limit_event rendering", func() {
		It("renders a rate_limit_event with full rate_limit_info", func() {
			line := `{"type":"rate_limit_event","rate_limit_info":{"status":"allowed_warning","resetsAt":1800000000,"rateLimitType":"seven_day","utilization":0.85,"isUsingOverage":false,"surpassedThreshold":true}}`
			raw, formatted := processLine(line)
			Expect(raw).To(Equal(line + "\n"))
			Expect(formatted).To(ContainSubstring("⚠"))
			Expect(formatted).To(ContainSubstring("rate-limit"))
			Expect(formatted).To(ContainSubstring("seven_day"))
			Expect(formatted).To(ContainSubstring("85%"))
			Expect(formatted).To(ContainSubstring("resets="))
			Expect(formatted).To(ContainSubstring("status=allowed_warning"))
			Expect(formatted).NotTo(ContainSubstring("[unknown type: rate_limit_event]"))
		})

		It("renders a rate_limit_event with a different rateLimitType verbatim", func() {
			line := `{"type":"rate_limit_event","rate_limit_info":{"status":"blocked","resetsAt":1800003600,"rateLimitType":"five_hour","utilization":1.0}}`
			_, formatted := processLine(line)
			Expect(formatted).To(ContainSubstring("five_hour"))
			Expect(formatted).To(ContainSubstring("100%"))
			Expect(formatted).To(ContainSubstring("status=blocked"))
			Expect(formatted).NotTo(ContainSubstring("[unknown type: rate_limit_event]"))
		})

		It("renders a fallback line when rate_limit_info is absent", func() {
			line := `{"type":"rate_limit_event"}`
			_, formatted := processLine(line)
			Expect(formatted).To(ContainSubstring("⚠"))
			Expect(formatted).To(ContainSubstring("rate-limit"))
			Expect(formatted).NotTo(ContainSubstring("[unknown type: rate_limit_event]"))
			// Must not panic (Ginkgo recovers panics and marks the test failed)
		})

		It("omits the reset clause when resetsAt is zero", func() {
			line := `{"type":"rate_limit_event","rate_limit_info":{"status":"allowed_warning","resetsAt":0,"rateLimitType":"seven_day","utilization":0.50}}`
			_, formatted := processLine(line)
			Expect(formatted).To(ContainSubstring("seven_day"))
			Expect(formatted).NotTo(ContainSubstring("resets="))
		})

		It("renders utilization outside 0..1 verbatim without clamping", func() {
			line := `{"type":"rate_limit_event","rate_limit_info":{"status":"unknown","resetsAt":0,"rateLimitType":"seven_day","utilization":1.25}}`
			_, formatted := processLine(line)
			// int(1.25 * 100) = 125 — rendered as-is, not clamped to 100
			Expect(formatted).To(ContainSubstring("125%"))
		})
	})
})
