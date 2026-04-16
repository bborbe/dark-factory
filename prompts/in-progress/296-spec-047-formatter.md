---
status: executing
spec: [047-go-stream-formatter]
container: dark-factory-296-spec-047-formatter
dark-factory-version: v0.111.2
created: "2026-04-16T17:22:00Z"
queued: "2026-04-16T17:27:40Z"
started: "2026-04-16T17:27:42Z"
branch: dark-factory/go-stream-formatter
---

<summary>
- A new `pkg/formatter/` package is created with a `Formatter` interface and a production implementation
- The formatter reads newline-delimited stream-json from an `io.Reader`, writes every raw line verbatim to a raw writer, and writes a human-readable rendering to a formatted writer
- All Claude Code message types are handled: system init, assistant (text, thinking, tool_use), user (tool_result), and the final result event
- All tool types are rendered with appropriate glyphs and field previews: Bash, Read, Write, Edit, Grep, Glob, Task/Agent, ToolSearch, AskUserQuestion, Skill, WebFetch, WebSearch, TodoWrite, and mcp__* tools
- tool_use IDs are stored in an in-memory map so tool_result messages can be correlated back to the originating tool name
- Parse errors (non-JSON lines) are written verbatim to both outputs — the pipeline never terminates on a bad line
- Unknown message types and tools render a generic "unknown type/tool" line rather than crashing
- Long tool inputs and stdout/stderr previews are truncated to the same limits used by the Python v2 formatter (prevent unbounded memory growth)
- A Counterfeiter mock is generated for the interface so the executor tests can use it
- Unit tests cover every message type, every named tool, parse-error fallback, orphan tool_result, and missing-field resilience — ≥80% statement coverage on the package
</summary>

<objective>
Create `pkg/formatter/` — the Go port of the Python v2 stream-json formatter from the claude-yolo `files/stream-formatter.py`. The formatter sits between the container's stdout pipe and the log files; it writes raw JSONL and formatted human-readable lines simultaneously. This prompt delivers the package in isolation with full unit tests; the executor integration is wired in the next prompt.
</objective>

<context>
Read `/workspace/CLAUDE.md` for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-security-linting.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-parse-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/`

The Python v2 formatter that this must be semantically equivalent to is at:
  https://raw.githubusercontent.com/bborbe/claude-yolo/refs/heads/master/files/stream-formatter.py
Fetch it and read it in full — it is the behavioral oracle for which message shapes are rendered and how.

Files to read before editing:
- `pkg/executor/executor.go` — understand how log files are currently opened; the formatter package will be wired in here in prompt 2
- `pkg/report/parse.go` — the formatted log is scanned for the DARK-FACTORY-REPORT block; the formatter must not swallow this block (`pkg/report/report.go` has the type definitions)
</context>

<requirements>

## 1. Package layout

Create these files:

```
pkg/formatter/
├── doc.go              — package godoc
├── formatter.go        — Formatter interface + NewFormatter constructor + ProcessStream implementation
├── message.go          — Go structs for parsing stream-json message shapes
├── render.go           — rendering functions (one per tool type / message type)
├── formatter_suite_test.go
└── formatter_test.go
```

## 2. `pkg/formatter/doc.go`

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package formatter implements a Go port of the claude-yolo Python v2 stream-json formatter.
// It reads newline-delimited Claude Code stream-json from an io.Reader, writes every raw line
// verbatim to a raw writer, and renders a human-readable formatted line to a formatted writer.
package formatter
```

## 3. `pkg/formatter/message.go` — JSON parsing structs

Define minimal structs sufficient to parse every event the Python v2 formatter handles.
Use `json.RawMessage` for fields that vary by subtype so partial parsing is safe.

Key top-level shape:
```go
// StreamMessage is the top-level envelope for every newline-delimited event.
// Field names MUST match the Python v2 oracle (stream-formatter.py lines ~211-219):
//   - `total_cost_usd` (NOT `cost_usd`)
//   - `usage.input_tokens` / `usage.output_tokens` / `usage.cache_read_input_tokens`
//     (NOT top-level input_tokens/output_tokens)
type StreamMessage struct {
    Type    string          `json:"type"`
    Subtype string          `json:"subtype"` // used by "system" events
    Message json.RawMessage `json:"message"` // present on "assistant" and "user"
    // Result event fields (type == "result"):
    DurationMs   *float64 `json:"duration_ms"`
    TotalCostUSD *float64 `json:"total_cost_usd"`
    Usage        *Usage   `json:"usage"`
    Result       *string  `json:"result"`    // "success" or error string
    IsError      *bool    `json:"is_error"`
    // System init fields (type == "system", subtype == "init"):
    SessionID string   `json:"session_id"`
    Model     string   `json:"model"`
    Cwd       string   `json:"cwd"`
    Tools     []string `json:"tools"`
}

// Usage captures the token counts from the final result event.
type Usage struct {
    InputTokens         *int `json:"input_tokens"`
    OutputTokens        *int `json:"output_tokens"`
    CacheReadInputTokens *int `json:"cache_read_input_tokens"`
}

// AssistantMessage is the decoded form of StreamMessage.Message for type == "assistant".
type AssistantMessage struct {
    Content []ContentBlock `json:"content"`
}

// UserMessage is the decoded form of StreamMessage.Message for type == "user".
type UserMessage struct {
    Content []ContentBlock `json:"content"`
}

// ContentBlock represents one element of a message's content array.
// IMPORTANT: field tags MUST match the Python v2 oracle. In particular, "thinking"
// blocks expose their text under the JSON key `thinking`, NOT `text` (see
// stream-formatter.py line ~62: `c.get("thinking", "")`).
type ContentBlock struct {
    Type      string          `json:"type"`     // "text", "thinking", "tool_use", "tool_result"
    Text      string          `json:"text"`     // for type == "text"
    Thinking  string          `json:"thinking"` // for type == "thinking"
    // tool_use fields:
    ID        string          `json:"id"`
    Name      string          `json:"name"`
    Input     json.RawMessage `json:"input"`
    // tool_result fields:
    ToolUseID string          `json:"tool_use_id"`
    IsError   *bool           `json:"is_error"`
    Content   json.RawMessage `json:"content"` // string | []ContentBlock
}
```

Do not add fields beyond what the Python v2 formatter actually uses. If in doubt, keep it minimal and extend later.

## 4. `pkg/formatter/formatter.go` — interface and implementation

### 4a. Interface and constructor

```go
//counterfeiter:generate -o ../../mocks/formatter.go --fake-name Formatter . Formatter

// Formatter processes a Claude Code stream-json stdout stream.
type Formatter interface {
    // ProcessStream reads newline-delimited stream-json from r until EOF or ctx cancellation.
    // For each line it writes the raw bytes (including the trailing newline) to rawWriter,
    // and a formatted human-readable line (with timestamp prefix, trailing newline) to formattedWriter.
    // Lines that are not valid JSON are written verbatim to both outputs.
    // ProcessStream returns nil on clean EOF. It returns a non-nil error only when rawWriter
    // or formattedWriter return a write error.
    ProcessStream(ctx context.Context, r io.Reader, rawWriter io.Writer, formattedWriter io.Writer) error
}

// NewFormatter creates a new Formatter.
func NewFormatter() Formatter {
    return &streamFormatter{}
}

type streamFormatter struct{}
```

### 4b. `ProcessStream` implementation

```go
func (f *streamFormatter) ProcessStream(
    ctx context.Context,
    r io.Reader,
    rawWriter io.Writer,
    formattedWriter io.Writer,
) error {
    // Per-stream state: maps tool_use ID → tool name for result correlation.
    toolNames := make(map[string]string)

    scanner := bufio.NewScanner(r)
    // Allow lines up to 10 MB (handles large tool inputs without OOM; beyond this the line is
    // truncated at the scanner level — the raw line is still written verbatim up to the buffer limit).
    const maxScannerBuf = 10 * 1024 * 1024
    buf := make([]byte, 64*1024)
    scanner.Buffer(buf, maxScannerBuf)

    for scanner.Scan() {
        // Check context cancellation without blocking (non-blocking select).
        select {
        case <-ctx.Done():
            return nil
        default:
        }

        line := scanner.Bytes()

        // 1. Always write raw line + newline to rawWriter.
        // NOTE: scanner.Bytes() returns a slice into the scanner's internal buffer that is
        // reused on the next Scan; append(line, '\n') can alias into that buffer. Write the
        // line and newline as two separate calls to avoid the aliasing bug.
        if _, err := rawWriter.Write(line); err != nil {
            return errors.Wrap(ctx, err, "write raw log")
        }
        if _, err := rawWriter.Write([]byte{'\n'}); err != nil {
            return errors.Wrap(ctx, err, "write raw log newline")
        }

        // 2. Parse and format; on any parse error write the line verbatim to formattedWriter.
        var msg StreamMessage
        if err := json.Unmarshal(line, &msg); err != nil {
            formatted := formatTimestamp() + " " + string(line) + "\n"
            if _, err2 := formattedWriter.Write([]byte(formatted)); err2 != nil {
                return errors.Wrap(ctx, err2, "write formatted fallback")
            }
            continue
        }

        rendered := f.renderMessage(msg, toolNames)
        if rendered == "" {
            continue
        }
        if _, err := formattedWriter.Write([]byte(rendered)); err != nil {
            return errors.Wrap(ctx, err, "write formatted output")
        }
    }

    if err := scanner.Err(); err != nil {
        // Scanner error (e.g. line too long > 10 MB). Spec Desired Behavior 4 requires the
        // overflowing content be preserved. Write a truncation marker to BOTH outputs so the
        // pipeline does not silently drop data.
        marker := fmt.Sprintf("[formatter: scanner error, line truncated or dropped: %v]\n", err)
        if _, werr := rawWriter.Write([]byte(marker)); werr != nil {
            return errors.Wrap(ctx, werr, "write truncation marker to raw log")
        }
        if _, werr := formattedWriter.Write([]byte(formatTimestamp() + " " + marker)); werr != nil {
            return errors.Wrap(ctx, werr, "write truncation marker to formatted log")
        }
        slog.Warn("formatter scanner error", "error", err)
    }
    return nil
}
```

### 4c. `formatTimestamp`

```go
// formatTimestamp returns the current local time formatted as [HH:MM:SS].
func formatTimestamp() string {
    return time.Now().Format("[15:04:05]")
}
```

### 4d. `renderMessage` dispatch

```go
func (f *streamFormatter) renderMessage(msg StreamMessage, toolNames map[string]string) string {
    switch msg.Type {
    case "system":
        return renderSystemInit(msg)
    case "assistant":
        return renderAssistant(msg, toolNames)
    case "user":
        return renderUser(msg, toolNames)
    case "result":
        return renderResult(msg)
    default:
        return formatTimestamp() + " [unknown type: " + msg.Type + "]\n"
    }
}
```

## 5. `pkg/formatter/render.go` — rendering functions

### 5a. System init

Match the Python v2 oracle (line ~49):
`[init] session=<first-8-chars> model=<m> cwd=<c> tools=<N>`

```go
func renderSystemInit(msg StreamMessage) string {
    if msg.Subtype != "init" {
        return formatTimestamp() + " [system: " + msg.Subtype + "]\n"
    }
    session := msg.SessionID
    if len(session) > 8 {
        session = session[:8]
    }
    if session == "" {
        session = "(unknown)"
    }
    return fmt.Sprintf("%s [init] session=%s model=%s cwd=%s tools=%d\n",
        formatTimestamp(), session, msg.Model, msg.Cwd, len(msg.Tools))
}
```

### 5b. Final result

```go
func renderResult(msg StreamMessage) string {
    var parts []string
    if msg.Result != nil {
        parts = append(parts, "result: "+*msg.Result)
    }
    if msg.DurationMs != nil {
        parts = append(parts, fmt.Sprintf("duration: %.0fms", *msg.DurationMs))
    }
    if msg.TotalCostUSD != nil {
        parts = append(parts, fmt.Sprintf("cost: $%.4f", *msg.TotalCostUSD))
    }
    if msg.Usage != nil && msg.Usage.InputTokens != nil && msg.Usage.OutputTokens != nil {
        parts = append(parts, fmt.Sprintf("tokens: %d in / %d out",
            *msg.Usage.InputTokens, *msg.Usage.OutputTokens))
    }
    return formatTimestamp() + " " + strings.Join(parts, " | ") + "\n"
}
```

### 5c. Assistant messages

```go
func renderAssistant(msg StreamMessage, toolNames map[string]string) string {
    var am AssistantMessage
    if err := json.Unmarshal(msg.Message, &am); err != nil {
        return formatTimestamp() + " [assistant: parse error]\n"
    }

    var sb strings.Builder
    for _, block := range am.Content {
        switch block.Type {
        case "text":
            if strings.TrimSpace(block.Text) != "" {
                sb.WriteString(formatTimestamp() + " " + block.Text + "\n")
            }
        case "thinking":
            preview := truncate(block.Thinking, 500)
            sb.WriteString(formatTimestamp() + " 💭 " + preview + "\n")
        case "tool_use":
            toolNames[block.ID] = block.Name
            sb.WriteString(renderToolUse(block))
        default:
            sb.WriteString(formatTimestamp() + " [assistant content: " + block.Type + "]\n")
        }
    }
    return sb.String()
}
```

### 5d. User messages (tool results)

```go
func renderUser(msg StreamMessage, toolNames map[string]string) string {
    var um UserMessage
    if err := json.Unmarshal(msg.Message, &um); err != nil {
        return formatTimestamp() + " [user: parse error]\n"
    }

    var sb strings.Builder
    for _, block := range um.Content {
        if block.Type != "tool_result" {
            continue
        }
        toolName := toolNames[block.ToolUseID]
        if toolName == "" {
            toolName = "(unknown tool)"
        }
        isError := block.IsError != nil && *block.IsError
        sb.WriteString(renderToolResult(block, toolName, isError))
    }
    return sb.String()
}
```

### 5e. Tool use rendering — `renderToolUse`

**The Python v2 oracle (`files/stream-formatter.py`, function `format_tool_use`) is authoritative for output shape — glyphs, prefixes, and per-tool input formatting.** The table below is a summary, not a replacement. Before implementing, read `format_tool_use` and reproduce each branch; where this prompt and the oracle disagree, the oracle wins.

Dispatch on `block.Name`. Parse `block.Input` as `map[string]json.RawMessage` to safely access fields. For unknown tools, render a generic fallback line.

Declare these constants in `render.go` (match the oracle names and values):

```go
const (
    MAX_LINE         = 200 // per-line truncation for rendered input previews
    MAX_PROMPT_CHARS = 160 // Task/Agent prompt preview
    MAX_TAIL_LINES   = 20  // Bash failure tail length (see 5f)
)
```

Key rendering per the Python oracle:

| Tool | Oracle output shape (see `format_tool_use`) |
|------|----------------|
| `Bash` | `$ <command>` (no `→`, no brackets); command truncated to MAX_LINE |
| `Read` | `[read] <file_path>` with optional `offset=N limit=N` |
| `Write` | `[write] <file_path> (<N> chars)` |
| `Edit` | `[edit] <file_path>` + indented diff-style `- old` / `+ new` preview |
| `Grep` | `[grep] <pattern>` + optional `path=<path>` |
| `Glob` | `[glob] <pattern>` + optional `path=<path>` |
| `Task` / `Agent` | `[agent] subagent=<type>`; prompt preview truncated to MAX_PROMPT_CHARS |
| `ToolSearch` | `[tool-search] <query>` |
| `AskUserQuestion` | `[ask] <question>` |
| `Skill` | `[skill] <skill>` |
| `WebFetch` | `[web-fetch] <url>` |
| `WebSearch` | `[web-search] <query>` |
| `TodoWrite` | `[todo] <N items: N completed / N in_progress / N pending>` |
| `mcp__*` | `[mcp] <tool>` |
| unknown | `[<tool>] (unknown tool)` |

Every rendered line MUST be prefixed with `formatTimestamp() + " "` and suffixed with `"\n"`.

**Important:** The `→` and `←` glyphs are used by the oracle ONLY on tool RESULT lines (see 5f), not on tool_use lines. Do not emit `→` in `renderToolUse`.

For security: all field values extracted from JSON are passed through `fmt.Sprintf("%s", ...)` or simple string concatenation only — never executed, never passed to `html/template` or `text/template`.

### 5f. Tool result rendering — `renderToolResult`

**The Python v2 oracle (`format_tool_result`, lines ~155-200) is authoritative.** Per-tool result shape summary (read `format_tool_result` in full and reproduce every branch):

| Tool name (from correlated tool_use) | Oracle result shape |
|------|------|
| `Read` | `→ N lines read` (count newlines in preview) |
| `Write` | silent (no line emitted on success) |
| `Edit` | `→ updated` or silent on success |
| `Grep` | `→ N match lines` |
| `Glob` | `→ N files` |
| `Bash` (success) | silent |
| `Bash` (error / `is_error=true` / non-zero exit in text) | `⚠ bash failed:` followed by last MAX_TAIL_LINES (20) lines of output, each indented |
| `Task` / `Agent` | `← agent reply: <preview>` (preview truncated to MAX_LINE) |
| any tool with `is_error=true` | `⚠ <tool>: <preview>` (error glyph overrides success shape) |
| unknown tool | `← <tool>: <preview>` |

Glyphs:
- `←` on agent/generic success previews
- `→` on summary-style result lines (Read/Grep/Glob/Edit counts)
- `⚠` on errors (`is_error=true` OR Bash non-zero exit)

`extractResultPreview` tries to unmarshal `block.Content` as:
1. A `string` — use directly
2. A `[]ContentBlock` — join the `Text` fields of any `"text"` type blocks

On failure, return `"(result)"`.

For Bash failure tails: split the preview on `\n`, take the last min(len(lines), MAX_TAIL_LINES) lines, prefix each with two spaces for indentation, and join with newlines.

### 5g. `truncate` helper

```go
// truncate returns s truncated to maxLen runes, appending "…" if truncated.
func truncate(s string, maxLen int) string {
    runes := []rune(s)
    if len(runes) <= maxLen {
        return s
    }
    return string(runes[:maxLen]) + "…"
}
```

## 6. Generate Counterfeiter mock

After creating the interface, run:
```
cd /workspace && go generate ./pkg/formatter/...
```

This produces `mocks/formatter.go` with `FakeFormatter`.

## 7. Unit tests — `pkg/formatter/formatter_test.go`

Use Ginkgo v2 / Gomega, external test package (`package formatter_test`).

### Test suite file

`formatter_suite_test.go`:
```go
package formatter_test

//go:generate go run -mod=mod github.com/maxbrunsfeld/counterfeiter/v6 -generate

import (
    "testing"
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

func TestFormatter(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "Formatter Suite")
}
```

**Note:** The `//go:generate` directive is REQUIRED. Match the convention in `pkg/cmd/cmd_suite_test.go:5`. Without it, `go generate ./...` will not produce `mocks/formatter.go` and verification step 2 (`ls mocks/formatter.go`) fails.

### Required test cases

Create a helper `processLine(jsonLine string) (raw string, formatted string)` that constructs a `streamFormatter`, calls `ProcessStream` with an `io.Reader` wrapping the single line, and returns the two outputs as strings.

**7a. System init event**
Input: `{"type":"system","subtype":"init","session_id":"abcdefghij","model":"claude-opus-4-6","cwd":"/workspace","tools":["Bash","Read"]}`
Assertions:
- raw output == input + "\n"
- formatted output contains `[init] session=abcdefgh` (8-char truncation)
- formatted output contains `model=claude-opus-4-6`
- formatted output contains `cwd=/workspace`
- formatted output contains `tools=2`
- formatted output starts with `[` (timestamp prefix)

**7b. Assistant text message**
Input: `{"type":"assistant","message":{"content":[{"type":"text","text":"Hello world"}]}}`
Assertions:
- formatted output contains "Hello world"
- raw output preserved verbatim

**7c. Assistant thinking message**
Input: `{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"Deep reasoning..."}]}}`
Assertions:
- formatted output contains "💭"
- formatted output contains "Deep reasoning"

**NOTE:** the thinking text is under the JSON key `thinking`, not `text`. This test asserts the `ContentBlock.Thinking` field binding works.

**7d. tool_use: Bash**
Input: `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls -la"}}]}}`
Assertions (match Python oracle: Bash renders as `$ <command>`):
- formatted output contains "$ ls -la"
- formatted output does NOT contain "→" on this line (→ is for results only)

**7e. tool_use: Read, Write, Edit, Grep, Glob**
For each, assert formatted output matches the oracle shape:
- Read → `[read] <file_path>`
- Write → `[write] <file_path>`
- Edit → `[edit] <file_path>`
- Grep → `[grep] <pattern>`
- Glob → `[glob] <pattern>`

**7f. tool_use: Task, ToolSearch, AskUserQuestion, Skill, WebFetch, WebSearch, TodoWrite, mcp__custom**
For each, assert formatted output contains the bracketed prefix matching the oracle (e.g. `[agent]`, `[tool-search]`, `[ask]`, `[skill]`, `[web-fetch]`, `[web-search]`, `[todo]`, `[mcp]`).

**7g. tool_result success — correlated (Read)**
First send a tool_use with `id: "t1"`, `name: "Read"`, then a tool_result with `tool_use_id: "t1"` and content `"line1\nline2\nline3\n"`.
Assertions (match Python: Read results render as `→ N lines read`):
- formatted result output contains "→"
- formatted result output contains "3 lines" OR "lines read" (oracle exact form)

**7h. tool_result error — correlated (Bash)**
Send tool_use with `name: "Bash"` then tool_result with `is_error: true` and content containing multiple lines.
Assertions (match Python: Bash error renders as `⚠ bash failed:` + indented tail):
- formatted output contains "⚠"
- formatted output contains "bash failed" (or oracle form)
- the last lines of the error content appear, each indented

**7i. Orphan tool_result (unknown tool_use_id)**
Send tool_result without a preceding tool_use.
Assertions:
- formatted output contains "←" or "⚠"
- does NOT panic or return error
- output does NOT contain an empty tool name where a fallback is expected (check for "(unknown tool)" or similar)

**7j. Parse error fallback**
Input: `this is not json`
Assertions:
- raw output == "this is not json\n"
- formatted output == "this is not json\n" OR contains timestamp + "this is not json"
- `ProcessStream` returns nil (not an error)

**7k. Unknown message type**
Input: `{"type":"future_type","data":"x"}`
Assertions:
- formatted output contains "unknown type"
- does NOT panic

**7l. Missing field resilience**
Input: `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"","name":"","input":{}}]}}`
Assertions:
- does NOT panic
- `ProcessStream` returns nil

**7m. Long text truncation**
Construct a thinking block with 1000 rune text in the `thinking` JSON key.
Assertions:
- formatted output does NOT exceed the truncation limit in its thinking line
- formatted output contains "…" (ellipsis)

**7n. Final result event**
Input: `{"type":"result","result":"success","duration_ms":12345,"total_cost_usd":0.0123,"usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":8000}}`
Assertions (match Python oracle field names: `total_cost_usd` + `usage.{input,output}_tokens`):
- formatted output contains "success"
- formatted output contains "12345ms" or "12345"
- formatted output contains "$0.0123" or "0.0123"
- formatted output contains "100" and "50"

**7o. Multi-line stream**
Build an `io.Reader` with 5 sequential events (system init, assistant text, tool_use, tool_result, result).
Assert `ProcessStream` returns nil, raw output has 5 lines, formatted output has ≥5 lines.

**7p. Context cancellation**
Build a reader that blocks after the first line (use `io.Pipe`, close writer after 1 line).
Cancel context immediately after writing line 1.
Assert `ProcessStream` returns nil (not an error) and does not hang.

**7q. Line exceeds 10 MB scanner buffer**
Build a reader that emits a single line of 11 MB (no newlines), followed by EOF.
Assertions:
- `ProcessStream` returns nil (no panic, no non-nil error)
- rawWriter contains a truncation marker line mentioning "scanner error" or "truncated"
- formattedWriter contains the same truncation marker
- pipeline does NOT silently drop the event (Desired Behavior 4)

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- No new runtime dependencies — use only Go stdlib plus packages already in `go.mod`. The repo is non-vendored (`make ensure` runs `rm -rf vendor`), so there is no `vendor/` to check.
- Wrap all non-nil errors with `errors.Wrap` / `errors.Wrapf` / `errors.Errorf` from `github.com/bborbe/errors`. Never `fmt.Errorf`, never bare `return err`.
- All container-stdout content (JSON field values) is untrusted. Never execute, eval, template, or shell-escape any field value — string concatenation and `fmt.Sprintf` with `%s`/`%d`/`%f` verbs only.
- Never open or stat files that appear in tool inputs (Read, Write, Edit paths are display-only).
- No network access, no subprocess calls, no filesystem writes other than via the `rawWriter` and `formattedWriter` parameters.
- The formatter must never buffer an entire run in memory — it reads and writes line by line.
- Unicode glyphs `⚠`, `→`, `←`, `💭` must appear in rendered output exactly as specified; do not substitute ASCII equivalents.
- `ProcessStream` must return nil on clean EOF and on context cancellation — only write errors cause a non-nil return.
- Existing tests must still pass.
- Do not touch `go.mod` / `go.sum`.
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional checks:
1. `ls pkg/formatter/` — shows formatter.go, message.go, render.go, doc.go, *_test.go files
2. `ls mocks/formatter.go` — Counterfeiter fake exists
3. `go test -coverprofile=/tmp/cover.out ./pkg/formatter/... && go tool cover -func=/tmp/cover.out | grep -E 'formatter|render'` — statement coverage ≥80%
4. `grep -rn "template\|exec\.Command\|os\.Open\|http\." pkg/formatter/` — must return zero matches (no filesystem, network, or subprocess access)
</verification>
