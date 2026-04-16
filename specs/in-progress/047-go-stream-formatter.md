---
status: prompted
tags:
    - dark-factory
    - spec
approved: "2026-04-16T17:10:37Z"
generating: "2026-04-16T17:10:37Z"
prompted: "2026-04-16T17:17:01Z"
branch: dark-factory/go-stream-formatter
---

## Summary

- Move Claude Code stream-json formatting from the claude-yolo Python formatter into dark-factory (Go).
- Every prompt run produces two persistent log files: a raw JSONL ground-truth log and a human-readable formatted log.
- Live terminal output stays formatted — operator UX is unchanged.
- Decouples presentation from the container runtime: formatter improvements no longer require a claude-yolo image rebuild.
- Raw JSONL ends `--rm` container data loss: every message Claude Code emitted is recoverable even if the formatter hides or misrenders it.

## Problem

Today dark-factory launches claude-yolo containers in stream-formatted mode. A Python formatter inside the container translates Claude Code's stream-json into human-readable lines that dark-factory captures through `MultiWriter(os.Stdout, logFileHandle)` into `prompts/log/NNN.log`. Two issues follow from this:

1. There is no raw capture. When the formatter hides, reorders, or misinterprets a message, the underlying JSONL is gone the moment the `--rm` container exits. Debugging a bad render is reconstruction-by-guesswork.
2. Every formatter change — new tool rendering, bug fix, richer stats — requires rebuilding and version-bumping the claude-yolo Docker image. Presentation is coupled to runtime release cadence.

Formatting belongs in the orchestrator, not the sandbox. Dark-factory already owns the log file lifecycle; it is the natural place to parse stream-json, emit both raw and formatted views, and iterate on presentation without touching the container.

## Goal

After this work, dark-factory runs every prompt with `YOLO_OUTPUT=json` so the container emits raw stream-json on stdout. Dark-factory consumes that stream, persists it verbatim as a raw JSONL log next to the formatted log, renders formatted lines to both the terminal and the formatted log, and behaves robustly when the stream contains malformed or unknown messages. The formatter lives in Go inside dark-factory with unit tests covering every message and tool type the current Python v2 formatter handles.

## Non-goals

- Retroactively reformatting historical `prompts/log/NNN.log` files — there is no historical raw JSONL to reformat from.
- A configuration flag to disable formatting or to toggle formatted/raw terminal output — formatting is always on.
- Deleting the Python formatter or any entrypoint mode in claude-yolo — that cleanup happens in a separate later prompt once the Go formatter is proven in production.
- Supporting non-stream-json container output modes (text, print) — dark-factory always requests stream-json.
- Byte-for-byte reproduction of the Python formatter's output. Semantic equivalence is the bar; minor whitespace or ordering differences are acceptable.

## Desired Behavior

1. Every prompt execution writes a raw JSONL log (`.jsonl` extension, same numbered prefix as the formatted log) capturing the container's stdout verbatim, one stream-json message per line, preserved even when parsing or formatting fails.
2. Every prompt execution writes a formatted log (`.log` extension, same numbered prefix as the raw log) containing a human-readable rendering of the same stream, including a timestamp prefix, tool invocations, tool results (correlated back to the originating tool via tool_use_id), assistant text and thinking, session init, and final result statistics (duration, cost, tokens).
3. The live terminal output matches the formatted log — operators see the same readable view they see today.
4. When a stdout line is not valid JSON, it is passed through verbatim to both the formatted log and the terminal (fallback), while also being preserved in the raw JSONL log. Parse failures never terminate the pipeline.
5. When a tool_result arrives whose tool_use_id was not previously seen (orphan), it is rendered without correlation rather than dropped or crashing the formatter.
6. Tool errors (is_error=true on a tool_result) and non-zero Bash exit codes are visually highlighted in the formatted output, including a tail of stderr and the last lines of stdout on Bash failure.
7. The formatter covers at minimum the tool set the current Python v2 formatter handles: Bash, Read, Write, Edit, Grep, Glob, Task/Agent, ToolSearch, AskUserQuestion, Skill, WebFetch, WebSearch, TodoWrite, and `mcp__*` tools; plus system init events, assistant text/thinking/tool_use, user tool_result, and the final result event.
8. Container stderr continues to be captured to the formatted log and the terminal so operator-visible container and Claude errors remain surfaced.

## Constraints

- The dark-factory CLI surface and configuration schema must not change. No new flags are required for this feature.
- No new runtime dependencies — Go stdlib plus packages already vendored in the project.
- The raw JSONL log and the formatted log share the existing numbered-prefix naming used by the current log file; they differ only by extension. Both live in the existing log directory.
- Claude-yolo's entrypoint is untouched. Its three output modes remain available for standalone users; dark-factory simply always selects the JSON mode via environment variable.
- This feature requires a claude-yolo image version that supports the `YOLO_OUTPUT=json` raw-passthrough mode (first released in claude-yolo v0.6.0). The dark-factory default container image tag and the example config must be updated to a version with this support.
- Existing executor behavior for container lifecycle, completion-report watching, timeout handling, and reattach must continue to work unchanged for both the primary execute path and the sidechain execute path.
- The formatter must handle unbounded input without buffering an entire run in memory; it streams line-by-line.
- Unicode glyphs used by the current Python formatter (⚠, →, ←, 💭) are preserved in the Go port's output.
- Follows dark-factory's own contribution rules (see CLAUDE.md): no direct edits — all code lands via the dark-factory pipeline.

## Assumptions

- Claude-yolo's existing JSON output mode emits newline-delimited stream-json on stdout and is already stable enough to be the default for dark-factory runs.
- The formatted output does not need to be machine-parseable; the raw JSONL is the machine-readable artifact.
- The reference Python formatter (claude-yolo `files/stream-formatter.py`, v2) is the behavioral oracle for which messages and tools are rendered and how.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| A stdout line is not valid JSON | Write the raw line to the raw log, pass it through verbatim to the formatted log and terminal | No action; parsing continues on next line |
| A known message type has an unexpected shape (missing field) | Render whatever fields are present; never panic | No action; next message resumes normal rendering |
| An unknown message type or tool name | Render a generic "unknown type/tool" line including the type string | No action |
| tool_result references an unknown tool_use_id | Render without correlation (no tool name prefix) | No action |
| Raw log file cannot be created or written | Fail fast before the container starts, with a clear error naming the raw log path | Operator fixes the log directory and retries |
| Container produces no stdout at all | Raw log exists and is empty; formatted log contains only stderr and session framing | Normal completion flow (completion-report watcher handles this) |
| Container emits a very long single line (no newlines) | Line is eventually flushed when a newline arrives or the stream ends; memory stays bounded by reasonable per-line buffer limits | No action |

## Security / Abuse Cases

- Container stdout is untrusted input. The formatter must never execute, interpret as a shell command, or pass to a template any field from the stream. Rendering is string-concatenation only.
- Long or adversarial payloads (huge tool inputs, massive Bash stdout) must not cause unbounded memory growth; previews are truncated using the same limits the Python formatter already applies.
- File paths that appear in tool inputs (Read, Write, Edit) are displayed only; the formatter never opens or touches them.
- No network, no subprocess, no filesystem writes other than the two log files.

## Acceptance Criteria

- [ ] Every prompt run produces a raw JSONL log file alongside the formatted log, both persisted after the container exits.
- [ ] The raw log contains the container's stdout verbatim, byte-preserved, one stream-json message per line.
- [ ] The formatted log is human-readable, with a timestamp prefix on each line, and renders session init, assistant text/thinking, tool invocations, tool results, and final stats.
- [ ] Live terminal output remains formatted and matches the formatted log.
- [ ] A parse-error line in the stream does not terminate the run and is visible in both logs.
- [ ] Tool errors and non-zero Bash exits are clearly marked in the formatted output.
- [ ] tool_result lines are correlated to their originating tool via tool_use_id; orphan tool_results render without crashing.
- [ ] Container stderr continues to reach both the terminal and the formatted log.
- [ ] Both executor code paths (primary and sidechain) produce identical logging behavior.
- [ ] Unit tests cover every message and tool type the Python v2 formatter handles, plus the parse-error fallback and the orphan-tool_result path.
- [ ] No new runtime dependencies introduced.

## Verification

```
make precommit
```

Additionally, an operator check against a real run:

```
# After running a prompt through dark-factory:
ls prompts/log/                            # shows NNN.log and NNN.jsonl
head prompts/log/NNN.jsonl | jq .          # raw messages parse as JSON
head prompts/log/NNN.log                   # formatted, human-readable
```

## Do-Nothing Option

Stay on the Python formatter inside claude-yolo. Dark-factory keeps capturing only formatted lines, loses the underlying JSONL when the `--rm` container exits, and every formatting improvement continues to require a claude-yolo image rebuild and version bump. Acceptable only if we believe the formatter is already good enough forever and nobody will ever need to debug a bad render from the raw stream. Neither of those has held true recently — the Python formatter was substantially reworked after bugs and missing tool coverage were discovered — so doing nothing just defers the same cost and accepts ongoing data loss.
