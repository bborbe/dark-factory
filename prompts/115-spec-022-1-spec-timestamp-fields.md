---
spec: ["022"]
status: created
created: "2026-03-06T18:35:00Z"
---
<objective>
Add lifecycle timestamp fields to `spec.Frontmatter` and write them automatically on each status transition. When a spec moves to `approved`, `prompted`, `verifying`, or `completed`, the corresponding timestamp field is written once and never overwritten.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `/home/node/.claude/docs/go-patterns.md` and `go-testing.md` for patterns.
Read `pkg/spec/spec.go` — `Frontmatter` struct, `SpecFile`, `SetStatus()`, `MarkCompleted()`, `MarkVerifying()`, and `Load()`.
Read `pkg/cmd/spec_approve.go` — calls `sf.SetStatus(string(spec.StatusApproved))` then `sf.Save()`.
Read `pkg/cmd/spec_verify.go` — calls `sf.MarkCompleted()` then `sf.Save()`.
Read `pkg/generator/generator.go` — calls `sf.SetStatus(string(spec.StatusPrompted))` then `sf.Save()`.
Read `pkg/spec/spec.go` `autoCompleter.CheckAndComplete` — calls `sf.MarkVerifying()` then `sf.Save()`.
</context>

<requirements>
1. In `pkg/spec/spec.go`, extend `Frontmatter` with four optional timestamp fields:
   ```go
   type Frontmatter struct {
       Status    string   `yaml:"status"`
       Tags      []string `yaml:"tags,omitempty"`
       Approved  string   `yaml:"approved,omitempty"`
       Prompted  string   `yaml:"prompted,omitempty"`
       Verifying string   `yaml:"verifying,omitempty"`
       Completed string   `yaml:"completed,omitempty"`
   }
   ```

2. Add a `nowFunc` field to `SpecFile` for testability, a `now()` helper, and a `stampOnce` helper:
   ```go
   type SpecFile struct {
       Path        string
       Frontmatter Frontmatter
       Name        string
       Body        []byte
       nowFunc     func() time.Time
   }

   func (s *SpecFile) now() time.Time {
       if s.nowFunc == nil {
           return time.Now()
       }
       return s.nowFunc()
   }

   // stampOnce sets *field to the current UTC RFC3339 timestamp only if *field is empty.
   func (s *SpecFile) stampOnce(field *string) {
       if *field == "" {
           *field = s.now().UTC().Format(time.RFC3339)
       }
   }
   ```

3. Update `Load()` to initialise `nowFunc`:
   ```go
   return &SpecFile{
       Path:        path,
       Frontmatter: fm,
       Name:        name,
       Body:        body,
       nowFunc:     time.Now,
   }, nil
   ```
   Apply to both return sites in Load (the error-fallback path and the happy path).

4. Rewrite `SetStatus` to stamp the matching timestamp (only if the field is empty):
   ```go
   func (s *SpecFile) SetStatus(status string) {
       s.Frontmatter.Status = status
       switch Status(status) {
       case StatusApproved:
           s.stampOnce(&s.Frontmatter.Approved)
       case StatusPrompted:
           s.stampOnce(&s.Frontmatter.Prompted)
       case StatusVerifying:
           s.stampOnce(&s.Frontmatter.Verifying)
       case StatusCompleted:
           s.stampOnce(&s.Frontmatter.Completed)
       }
   }
   ```

5. Rewrite `MarkVerifying` to delegate to `SetStatus` so the timestamp is written:
   ```go
   func (s *SpecFile) MarkVerifying() {
       s.SetStatus(string(StatusVerifying))
   }
   ```

6. Rewrite `MarkCompleted` to delegate to `SetStatus` so the timestamp is written:
   ```go
   func (s *SpecFile) MarkCompleted() {
       s.SetStatus(string(StatusCompleted))
   }
   ```

7. Add or extend `pkg/spec/spec_test.go` to cover all new behaviour:
   - `SetStatus("approved")` sets `Frontmatter.Approved` if it was empty.
   - `SetStatus("approved")` does NOT overwrite an existing `Frontmatter.Approved`.
   - `SetStatus("prompted")` sets `Frontmatter.Prompted`.
   - `SetStatus("verifying")` sets `Frontmatter.Verifying`.
   - `SetStatus("completed")` sets `Frontmatter.Completed`.
   - `MarkVerifying()` sets `Frontmatter.Verifying`.
   - `MarkCompleted()` sets `Frontmatter.Completed`.
   - `Save()` / `Load()` round-trip: all four timestamp fields survive serialisation and deserialisation.
   Use `nowFunc` injection to control the returned time in tests (set a fixed `time.Time` on the `SpecFile` before calling the method under test).
</requirements>

<constraints>
- Spec frontmatter struct must be extended — not a new file format
- Timestamps are written once only — never overwrite a field that already has a value
- Existing `created`, `queued`, `started`, `completed` fields on *prompt* files are unchanged
- Do NOT commit — dark-factory handles git
- `make precommit` must pass
- Existing tests must still pass
</constraints>

<verification>
Run `make precommit` — must pass.
Run `go test ./pkg/spec/... -v` — all tests pass.
</verification>
