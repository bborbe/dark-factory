---
status: draft
created: "2026-05-24T00:00:00Z"
---

<summary>
- Added GoDoc comments to 38 exported items in pkg/factory/factory.go
- All Create* functions and EffectiveMaxContainers now have proper documentation
- Also fixed GoDoc for pkg/config/workflow.go:67, pkg/processor/processor.go:37, and pkg/processor/workflow_helpers.go:144
</summary>

<objective>
Add GoDoc comments to all exported types, functions, and methods missing them across 6 files.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-doc-best-practices.md` for GoDoc format.

Files to read before making changes:
- `pkg/factory/factory.go` — all exported functions (34 items), note the detached block comment at lines 71-81 is for EffectiveMaxContainers
- `pkg/config/workflow.go` — line 67, `Contains` method
- `pkg/processor/processor.go` — line 37, `ErrPreflightFailed`
- `pkg/processor/workflow_helpers.go` — line 144, `PostMergeActions`
- `pkg/formatter/message.go` — lines 10, 30, 46, 53, 58, 63 (StreamMessage, RateLimitInfo, Usage, AssistantMessage, UserMessage, ContentBlock)
- `pkg/server/queue_action_handler.go` — lines 20, 25, 31 (QueueRequest, QueuedFile, QueueResponse)
- `pkg/server/inbox_handler.go` — lines 19, 24 (InboxFile, InboxListResponse)
</context>

<requirements>
1. In `pkg/factory/factory.go`, for each exported function, add a GoDoc comment directly above the func declaration:
   - EffectiveMaxContainers: move the existing detached comment (lines 71-81) directly above the function
   - All other Create* functions: add descriptive comments (1-2 sentences, start with function name)

2. In `pkg/config/workflow.go`, add GoDoc to `Contains` method at line 67.

3. In `pkg/processor/processor.go`, add GoDoc to `ErrPreflightFailed` at line 37.

4. In `pkg/processor/workflow_helpers.go`, add GoDoc to `PostMergeActions` at line 144.

5. In `pkg/formatter/message.go`, add GoDoc to each exported struct: StreamMessage, RateLimitInfo, Usage, AssistantMessage, UserMessage, ContentBlock.

6. In `pkg/server/queue_action_handler.go`, add GoDoc to: QueueRequest, QueuedFile, QueueResponse.

7. In `pkg/server/inbox_handler.go`, add GoDoc to: InboxFile, InboxListResponse.
</requirements>

<constraints>
- Only change files in this repo
- Do NOT commit — dark-factory handles git
- GoDoc comments start with the symbol name, full sentences
</constraints>

<verification>
make precommit
</verification>
