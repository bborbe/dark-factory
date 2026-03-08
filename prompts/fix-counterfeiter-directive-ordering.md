---
status: created
created: "2026-03-08T21:12:08Z"
---

<objective>
Fix counterfeiter directive placement on ~22 interfaces. The `//counterfeiter:generate` directive must be placed ABOVE the GoDoc comment with a blank line between them. Currently most interfaces have the directive BELOW the GoDoc (reversed order).
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `/home/node/.claude/docs/go-patterns.md` — counterfeiter conventions.

The correct pattern is:
```go
//counterfeiter:generate -o ../../mocks/foo.go --fake-name Foo . Foo

// Foo does something.
type Foo interface {
```

The WRONG pattern (currently in codebase) is:
```go
// Foo does something.
//
//counterfeiter:generate -o ../../mocks/foo.go --fake-name Foo . Foo
type Foo interface {
```

Files to fix (check each — the directive may be above or below GoDoc):
- `pkg/config/loader.go` — `Loader`
- `pkg/git/git.go` — `Releaser`
- `pkg/git/brancher.go` — `Brancher`
- `pkg/git/pr_creator.go` — `PRCreator`
- `pkg/git/pr_merger.go` — `PRMerger`
- `pkg/git/review_fetcher.go` — `ReviewFetcher`
- `pkg/git/worktree.go` — `Worktree`
- `pkg/executor/executor.go` — `Executor`
- `pkg/processor/processor.go` — `Processor`
- `pkg/prompt/counter.go` — `Counter`
- `pkg/prompt/prompt.go` — `FileMover`, `Manager`
- `pkg/spec/lister.go` — `Lister`
- `pkg/spec/spec.go` — `AutoCompleter`
- `pkg/specwatcher/watcher.go` — `SpecWatcher`
- `pkg/status/status.go` — `Checker`
- `pkg/status/formatter.go` — `Formatter`
- `pkg/version/version.go` — `Getter`
- `pkg/watcher/watcher.go` — `Watcher`
- `pkg/generator/generator.go` — `SpecGenerator`
- `pkg/review/fix_prompt_generator.go` — `FixPromptGenerator`
- `pkg/review/poller.go` — `ReviewPoller`
- `pkg/lock/locker.go` — `Locker`
- `pkg/server/server.go` — `Server`
</context>

<requirements>
For each interface listed above:

1. Read the current placement of the `//counterfeiter:generate` directive relative to the GoDoc comment.

2. If the directive is BELOW the GoDoc, move it ABOVE with a blank line:
   ```go
   // Before:
   // Foo does something.
   //
   //counterfeiter:generate -o ../../mocks/foo.go --fake-name Foo . Foo
   type Foo interface {

   // After:
   //counterfeiter:generate -o ../../mocks/foo.go --fake-name Foo . Foo

   // Foo does something.
   type Foo interface {
   ```

3. If the directive is ABOVE but has no blank line, add a blank line between directive and GoDoc:
   ```go
   // Before:
   //counterfeiter:generate -o ../../mocks/foo.go --fake-name Foo . Foo
   // Foo does something.
   type Foo interface {

   // After:
   //counterfeiter:generate -o ../../mocks/foo.go --fake-name Foo . Foo

   // Foo does something.
   type Foo interface {
   ```

4. If already correct (directive above, blank line, then GoDoc), skip.

5. Handle special cases:
   - `pkg/review/poller.go` — `ReviewPoller` has `//nolint` between directive and GoDoc. Keep `//nolint` with the GoDoc, not the directive:
     ```go
     //counterfeiter:generate ...

     //nolint:revive
     // ReviewPoller ...
     type ReviewPoller interface {
     ```
</requirements>

<constraints>
- Do NOT change any directive content — only move its position
- Do NOT change any GoDoc content — only move its position
- Do NOT change any code
- Preserve all `//nolint` annotations with the type they belong to
- Do NOT commit — dark-factory handles git
- `make precommit` must pass
- `make generate` must still produce identical mocks
</constraints>

<verification>
Run `make precommit` — must pass.

Verify correct ordering (directive before GoDoc with blank line):
```bash
# Check that no GoDoc immediately precedes a counterfeiter directive
grep -B1 "counterfeiter:generate" pkg/**/*.go | grep "^.*// [A-Z]"
# Expected: no output (GoDoc should not be directly before directive)
```

Verify mocks unchanged:
```bash
make generate
git diff mocks/
# Expected: no changes
```
</verification>

<success_criteria>
- All ~22 interfaces have directive ABOVE GoDoc with blank line separator
- No GoDoc content changed
- No directive content changed
- `make generate` produces identical mocks
- `make precommit` passes
</success_criteria>
