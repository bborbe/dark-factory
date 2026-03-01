---
status: executing
container: dark-factory-041-fix-security-issues
dark-factory-version: v0.10.2
---




# Fix security issues from code review

## Tasks

### 1. Replace hand-rolled HTTP server with `libhttp.NewServer`

Delete `pkg/server/server.go` `ListenAndServe` method and the hand-rolled `http.Server` setup. Instead use:

```go
import libhttp "github.com/bborbe/http"
```

`libhttp.NewServer(addr, mux)` returns a `run.Func` with sane defaults:
- `ReadHeaderTimeout: 10s`, `ReadTimeout: 30s`, `WriteTimeout: 30s`, `IdleTimeout: 60s`
- `MaxHeaderBytes: 1MB`
- Graceful shutdown via `context.WithoutCancel(ctx)`

The `Server` interface should change: instead of exposing `ListenAndServe(ctx) error`, store the `run.Func` returned by `libhttp.NewServer` and call it from the factory/runner.

Update `pkg/factory/factory.go` `CreateServer` to:
1. Build the `http.ServeMux` with all routes
2. Call `libhttp.NewServer(fmt.Sprintf("127.0.0.1:%d", port), mux)` — note `127.0.0.1` for localhost-only binding
3. Return the `run.Func` directly (or wrap in a thin Server interface)

This fixes: timeouts (#5), context.Background in shutdown (#6 server part), and localhost binding (#2) all at once.

### 2. Convert handlers to `libhttp.WithError`

All handlers in `pkg/server/` currently return `http.HandlerFunc` with manual error handling (`log.Printf` + `http.Error`). Convert to `libhttp.WithError` + `libhttp.NewErrorHandler` pattern:

**Before** (e.g. `status_handler.go`):
```go
func NewStatusHandler(checker status.Checker) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        st, err := checker.GetStatus(ctx)
        if err != nil {
            log.Printf("error: %v", err)
            http.Error(w, "Internal server error", 500)
            return
        }
        json.NewEncoder(w).Encode(st)
    }
}
```

**After**:
```go
func NewStatusHandler(checker status.Checker) libhttp.WithError {
    return libhttp.WithErrorFunc(func(ctx context.Context, resp http.ResponseWriter, req *http.Request) error {
        st, err := checker.GetStatus(ctx)
        if err != nil {
            return err
        }
        resp.Header().Set("Content-Type", "application/json")
        return json.NewEncoder(resp).Encode(st)
    })
}
```

Register with: `mux.Handle("/api/v1/status", libhttp.NewErrorHandler(NewStatusHandler(checker)))`

Apply to all handlers: `health_handler.go`, `status_handler.go`, `queue_handler.go`, `completed_handler.go`, `inbox_handler.go`, `queue_action_handler.go`.

For method-not-allowed errors, return `libhttp.WrapWithStatusCode(errors.New(ctx, "method not allowed"), http.StatusMethodNotAllowed)`.
For not-found errors, return `libhttp.WrapWithStatusCode(err, http.StatusNotFound)`.

### 3. Path traversal in `req.File`

`pkg/server/queue_helpers.go:26` — `filename` from JSON body passed unsanitized to `filepath.Join(inboxDir, filename)`.

Fix in `queueSingleFile`: add `filename = filepath.Base(filename)` before use. Reject if result is `.` or `..`.

### 4. Add request body size limit

`pkg/server/queue_action_handler.go` — add `r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)` before `json.NewDecoder`. (If using `libhttp.WithError`, use `req.Body` instead of `r.Body`.)

### 5. Cap `limit` parameter

`pkg/server/completed_handler.go` — add `const maxLimit = 1000` and cap parsed value.

### 6. Replace `context.Background()` in runner

`pkg/runner/runner.go:76` — change `context.Background()` to `context.WithoutCancel(ctx)` for lock release in defer.

(The server `context.Background()` is eliminated by switching to `libhttp.NewServer`.)

## Verification

Run `make precommit` — must pass.
