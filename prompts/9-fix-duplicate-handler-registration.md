---
status: created
created: "2026-03-11T16:45:24Z"
---

<summary>
- The duplicate handler registration for queue action routes is resolved
- Each HTTP route maps to a distinct handler instance or the duplication is intentionally documented
- No unnecessary object allocations in the factory
</summary>

<objective>
Investigate and fix the duplicate `NewQueueActionHandler` registration in `CreateServer` where both `/api/v1/queue/action` and `/api/v1/queue/action/all` create separate identical handler instances with the same arguments. Either the `/all` route needs different behavior or the duplication should be eliminated by sharing a single handler instance.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/factory/factory.go` — find `CreateServer` (~line 358). Look at the two `mux.Handle` calls for `/api/v1/queue/action` and `/api/v1/queue/action/all`.
Read `pkg/server/queue_action_handler.go` — understand what `QueueActionHandler` does and whether it distinguishes between single-file and all-files operations based on the request path or body.
</context>

<requirements>
1. Read `pkg/server/queue_action_handler.go` to understand how the handler distinguishes single vs all operations.

2. If the handler uses the request body (e.g., a `file` field) to decide single vs all behavior:
   - Share a single handler instance between both routes instead of creating two:
     ```go
     queueActionHandler := libhttp.NewErrorHandler(
         server.NewQueueActionHandler(inboxDir, inProgressDir, promptManager),
     )
     mux.Handle("/api/v1/queue/action", queueActionHandler)
     mux.Handle("/api/v1/queue/action/all", queueActionHandler)
     ```

3. If the two routes are supposed to have different behavior (single file vs all files), create separate handler constructors or add a parameter to distinguish them.

4. Add a brief comment above the route registrations explaining why two routes map to the same handler (if sharing) or what each route does differently (if separated).
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
- Do not change handler behavior — only fix the unnecessary duplication in the factory.
</constraints>

<verification>
Run `make precommit` -- must pass.
</verification>
