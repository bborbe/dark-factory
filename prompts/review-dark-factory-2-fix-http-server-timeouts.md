---
status: draft
created: "2026-05-24T00:00:00Z"
---

<summary>
- Added ReadTimeout, WriteTimeout, IdleTimeout, and MaxHeaderBytes to the HTTP server in factory.go
- Prevents slow-client resource exhaustion attacks where malicious clients hold connections indefinitely
- Set reasonable defaults: 15s read/write, 60s idle, 1MB max header
</summary>

<objective>
Add HTTP server timeouts to prevent unbounded resource consumption by slow clients.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes:
- `pkg/factory/factory.go` — line ~1046 where `&http.Server{Handler: mux}` is created, look for `server.Serve(listener)`
</context>

<requirements>
1. In `pkg/factory/factory.go`, find where the HTTP server is created (around line 1046).
2. Add timeouts to the server struct:
   ```go
   &http.Server{
       Handler:        mux,
       ReadTimeout:    15 * time.Second,
       WriteTimeout:   15 * time.Second,
       IdleTimeout:    60 * time.Second,
       MaxHeaderBytes: 1 << 20, // 1MB
   }
   ```
3. Ensure `time` is imported.
</requirements>

<constraints>
- Only change files in this repo
- Do NOT commit — dark-factory handles git
- Use `errors.Wrap`/`errors.Errorf` from `github.com/bborbe/errors` — never `fmt.Errorf` or bare `return err`
</constraints>

<verification>
make precommit
</verification>
