---
status: approved
created: "2026-05-24T00:00:00Z"
queued: "2026-05-25T14:51:20Z"
---

<summary>
- Verify `libhttp.NewServer` defaults match desired security posture (30s read/write, 60s idle, 1MB header)
- If shorter timeouts wanted, pass `WithReadTimeout`/`WithWriteTimeout` option functions
- If defaults are acceptable, document the audit finding and close the prompt as no-op
- Prevents slow-client resource exhaustion attacks
</summary>

<objective>
Confirm HTTP server has sensible timeout configuration. The server is created via `libhttp.NewServer(addr, mux)` at `pkg/factory/factory.go:1064`, which already applies defaults (ReadTimeout 30s, WriteTimeout 30s, IdleTimeout 60s, MaxHeaderBytes 1MB). Verify these are sufficient; if not, override via option functions.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes:
- `pkg/factory/factory.go` — line ~1064 where `libhttp.NewServer(addr, mux)` is called
- The `github.com/bborbe/http` package source (in vendor or `$GOPATH/pkg/mod`) — `http_server.go`, look at `CreateServerOptions` default values
</context>

<requirements>
1. Read the `libhttp.NewServer` source to confirm default timeouts: ReadTimeout/WriteTimeout 30s, IdleTimeout 60s, MaxHeaderBytes 1MB.

2. Decide: are these defaults sufficient for dark-factory's threat model?
   - If YES: this prompt is a no-op. Add a comment near `libhttp.NewServer(addr, mux)` in `pkg/factory/factory.go` documenting that defaults are intentionally accepted, then exit.
   - If NO: override with option functions, e.g.:
     ```go
     runFunc := libhttp.NewServer(addr, mux,
         func(o *libhttp.ServerOptions) {
             o.ReadTimeout = 15 * time.Second
             o.WriteTimeout = 15 * time.Second
         },
     )
     ```

3. Do NOT replace `libhttp.NewServer` with a raw `&http.Server{}` literal — that bypasses the wrapper's shutdown handling.
</requirements>

<constraints>
- Only change files in this repo
- Do NOT commit — dark-factory handles git
- Use `errors.Wrap`/`errors.Errorf` from `github.com/bborbe/errors` — never `fmt.Errorf` or bare `return err`
</constraints>

<verification>
make precommit
</verification>
