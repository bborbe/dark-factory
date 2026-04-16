---
status: completed
spec: [053-rate-limit-event-formatter]
summary: Added rate_limit_event rendering branch to pkg/formatter/ — RateLimitInfo struct, renderRateLimitEvent function, switch case, 5 new tests, and CHANGELOG entry
container: dark-factory-310-spec-053-rate-limit-formatter
dark-factory-version: v0.111.2
created: "2026-04-16T20:10:00Z"
queued: "2026-04-16T21:01:50Z"
started: "2026-04-16T23:17:19Z"
completed: "2026-04-16T23:25:58Z"
---

<summary>
- A new `rate_limit_event` message type is recognized by the Go stream formatter — it no longer produces `[unknown type: rate_limit_event]` noise
- Well-formed events render as a single timestamp-prefixed warning line showing the rate-limit type, utilization percentage, reset time (local timezone), and status
- Events with a missing or nil `rate_limit_info` field still render a concise fallback line — the formatter never crashes on partial data
- Unknown `status` or `rateLimitType` string values are passed through verbatim, with no enumeration in the formatter
- A zero or absent `resetsAt` value omits the reset clause rather than printing a nonsensical epoch
- All existing formatter tests continue to pass
- New unit tests cover: full render with `seven_day`/`allowed_warning`, full render with a different type (e.g. `five_hour`), and nil `rate_limit_info` fallback
</summary>

<objective>
Add a `rate_limit_event` rendering branch to `pkg/formatter/` so that every such event in the container's stream-json produces a human-readable warning line in the formatted log instead of the generic `[unknown type: rate_limit_event]` marker. This is a pure presentation change inside the formatter package — no other packages are affected.
</objective>

<context>
Read `/workspace/CLAUDE.md` for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-parse-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-security-linting.md` in `~/.claude/plugins/marketplaces/coding/docs/`

Files to read in full before editing:
- `pkg/formatter/message.go` — existing `StreamMessage` struct; you will add a `RateLimitInfo` struct and a new field to `StreamMessage`
- `pkg/formatter/formatter.go` — `renderMessage` switch statement; you will add a `case "rate_limit_event":` branch
- `pkg/formatter/render.go` — existing rendering functions; you will add `renderRateLimitEvent` here
- `pkg/formatter/formatter_test.go` — existing test cases; understand conventions before adding new `It` blocks
- `pkg/formatter/formatter_suite_test.go` — test suite setup for context
</context>

<requirements>

## 1. Add `RateLimitInfo` struct to `pkg/formatter/message.go`

Define a new struct for the `rate_limit_info` sub-object:

```go
// RateLimitInfo contains the details of a rate_limit_event message.
type RateLimitInfo struct {
    // Status is the rate-limit status string, e.g. "allowed_warning" or "blocked".
    Status string `json:"status"`
    // ResetsAt is the unix epoch (seconds) when the bucket resets. Zero means unknown.
    ResetsAt int64 `json:"resetsAt"`
    // RateLimitType identifies the bucket, e.g. "seven_day" or "five_hour".
    RateLimitType string `json:"rateLimitType"`
    // Utilization is the fraction of the bucket consumed, 0.0–1.0.
    Utilization float64 `json:"utilization"`
    // IsUsingOverage indicates overage consumption.
    IsUsingOverage bool `json:"isUsingOverage"`
    // SurpassedThreshold indicates the threshold has been crossed.
    SurpassedThreshold bool `json:"surpassedThreshold"`
}
```

Then add a `RateLimitInfo` pointer field to `StreamMessage`, after the existing `IsError` field:

```go
// RateLimitInfo is populated when Type == "rate_limit_event".
RateLimitInfo *RateLimitInfo `json:"rate_limit_info"`
```

Do not change any existing fields.

## 2. Add `renderRateLimitEvent` to `pkg/formatter/render.go`

Add the following function at the end of `render.go`:

```go
// renderRateLimitEvent formats a rate_limit_event message as a single human-readable line.
//
// Full render (rate_limit_info is present):
//
//	[HH:MM:SS] ⚠ rate-limit: <rateLimitType> <utilization>% [resets=<local-time>] status=<status>
//
// The reset clause is omitted when ResetsAt is zero.
// Fallback (rate_limit_info is nil or missing):
//
//	[HH:MM:SS] ⚠ rate-limit event
func renderRateLimitEvent(msg StreamMessage) string {
    if msg.RateLimitInfo == nil {
        return formatTimestamp() + " ⚠ rate-limit event\n"
    }
    info := msg.RateLimitInfo
    utilPct := int(info.Utilization * 100)
    var sb strings.Builder
    sb.WriteString(formatTimestamp())
    sb.WriteString(" ⚠ rate-limit: ")
    sb.WriteString(info.RateLimitType)
    sb.WriteString(fmt.Sprintf(" %d%%", utilPct))
    if info.ResetsAt != 0 {
        resetTime := time.Unix(info.ResetsAt, 0).Local().Format("15:04:05")
        sb.WriteString(" resets=")
        sb.WriteString(resetTime)
    }
    sb.WriteString(" status=")
    sb.WriteString(info.Status)
    sb.WriteString("\n")
    return sb.String()
}
```

**Security note:** all field values (`RateLimitType`, `Status`) are written via `strings.Builder` string concatenation only — never executed, never passed to a template, never shell-escaped.

**Timezone note:** `time.Unix(info.ResetsAt, 0).Local()` uses the operator's local timezone, consistent with the rest of the formatter's `formatTimestamp()` which uses `time.Now().Format(...)` (also local).

## 3. Add `case "rate_limit_event":` to `renderMessage` in `pkg/formatter/formatter.go`

In the `renderMessage` switch, add a new case **before** the `default:` clause:

```go
case "rate_limit_event":
    return renderRateLimitEvent(msg)
```

The switch should now look like:

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
    case "rate_limit_event":
        return renderRateLimitEvent(msg)
    default:
        return formatTimestamp() + " [unknown type: " + msg.Type + "]\n"
    }
}
```

Do not change anything else in `formatter.go`.

## 4. Unit tests in `pkg/formatter/formatter_test.go`

Add a new `Describe("rate_limit_event rendering", ...)` block inside the existing top-level `Describe` block in `formatter_test.go`. Place it after the last existing `Describe` / `It` block and before the closing `})` of the outer `Describe`.

Use the same `processLine` helper that already exists in the test file.

### 4a. Full render — seven_day / allowed_warning

```go
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
```

### 4b. Full render — different rate-limit type (five_hour)

```go
It("renders a rate_limit_event with a different rateLimitType verbatim", func() {
    line := `{"type":"rate_limit_event","rate_limit_info":{"status":"blocked","resetsAt":1800003600,"rateLimitType":"five_hour","utilization":1.0}}`
    _, formatted := processLine(line)
    Expect(formatted).To(ContainSubstring("five_hour"))
    Expect(formatted).To(ContainSubstring("100%"))
    Expect(formatted).To(ContainSubstring("status=blocked"))
    Expect(formatted).NotTo(ContainSubstring("[unknown type: rate_limit_event]"))
})
```

### 4c. Nil rate_limit_info — fallback line, no crash

```go
It("renders a fallback line when rate_limit_info is absent", func() {
    line := `{"type":"rate_limit_event"}`
    _, formatted := processLine(line)
    Expect(formatted).To(ContainSubstring("⚠"))
    Expect(formatted).To(ContainSubstring("rate-limit"))
    Expect(formatted).NotTo(ContainSubstring("[unknown type: rate_limit_event]"))
    // Must not panic (Ginkgo recovers panics and marks the test failed)
})
```

### 4d. Zero resetsAt — reset clause omitted

```go
It("omits the reset clause when resetsAt is zero", func() {
    line := `{"type":"rate_limit_event","rate_limit_info":{"status":"allowed_warning","resetsAt":0,"rateLimitType":"seven_day","utilization":0.50}}`
    _, formatted := processLine(line)
    Expect(formatted).To(ContainSubstring("seven_day"))
    Expect(formatted).NotTo(ContainSubstring("resets="))
})
```

### 4e. Utilization outside 0..1 — rendered as-is (no clamping)

```go
It("renders utilization outside 0..1 verbatim without clamping", func() {
    line := `{"type":"rate_limit_event","rate_limit_info":{"status":"unknown","resetsAt":0,"rateLimitType":"seven_day","utilization":1.25}}`
    _, formatted := processLine(line)
    // int(1.25 * 100) = 125 — rendered as-is, not clamped to 100
    Expect(formatted).To(ContainSubstring("125%"))
})
```

## 5. Write `## Unreleased` CHANGELOG entry

Check if `CHANGELOG.md` has an `## Unreleased` section. If it already exists, append to it. If not, add it immediately after the first `# Changelog` or `# CHANGELOG` heading. Add this bullet:

```
- feat: render rate_limit_event as a human-readable warning line in the stream formatter (previously emitted [unknown type: rate_limit_event] noise)
```

</requirements>

<constraints>
- Pure presentation change inside `pkg/formatter/` — do NOT touch any other package (`pkg/executor/`, `pkg/factory/`, `pkg/config/`, etc.)
- No new runtime dependencies — use only Go stdlib (`fmt`, `strings`, `time`) plus packages already imported in `render.go`
- All field values from the JSON event (`RateLimitType`, `Status`) are written via string concatenation or `fmt.Sprintf` only — never passed to `html/template`, `text/template`, or any subprocess
- Wrap all non-nil errors with `errors.Wrap` / `errors.Wrapf` from `github.com/bborbe/errors` if any new error paths are introduced; in practice `renderRateLimitEvent` returns a string (no errors)
- The formatter must never crash on malformed or partial `rate_limit_info` — if `RateLimitInfo` is nil, the fallback line is returned
- `[unknown type: rate_limit_event]` must not appear in any output for well-formed or partially-formed events
- Unknown `status` or `rateLimitType` strings pass through verbatim — no enumeration, no validation
- Reset time uses `time.Unix(resetsAt, 0).Local()` — local timezone, consistent with `formatTimestamp()`
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Do not touch `go.mod` / `go.sum`
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional spot checks:
1. `grep -n "rate_limit_event\|rate_limit_info\|RateLimitInfo" pkg/formatter/formatter.go pkg/formatter/message.go pkg/formatter/render.go` — all three files show relevant symbols
2. `grep -n "\[unknown type: rate_limit_event\]" pkg/formatter/` — must return zero matches
3. `go test -coverprofile=/tmp/cover.out ./pkg/formatter/... && go tool cover -func=/tmp/cover.out | grep -E "render\.go|formatter\.go"` — coverage ≥80% for the package
4. `go test -v -run "rate_limit" ./pkg/formatter/...` — all five new test cases pass
5. `grep -A1 "## Unreleased" CHANGELOG.md` — shows the new feat entry
</verification>
