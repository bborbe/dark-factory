---
status: completed
summary: Reordered constructor before struct in seven files to match the dominant Interface → Constructor → Struct → Methods pattern.
container: dark-factory-282-review-dark-factory-fix-file-layout
dark-factory-version: v0.104.2-dirty
created: "2026-04-06T00:00:00Z"
queued: "2026-04-06T20:04:24Z"
started: "2026-04-06T20:20:43Z"
completed: "2026-04-06T20:28:28Z"
---

<summary>
- Seven files have their struct definition appearing before the constructor function
- The majority of files in the project follow Interface then Constructor then Struct then Methods order
- Struct-before-constructor is inconsistent with the dominant pattern and harder to navigate
- The fix reorders the declarations within each file without changing any logic
- No tests, imports, or behavior changes are needed
</summary>

<objective>
Reorder declarations in seven files so that each constructor function appears before the struct it constructs, matching the dominant project pattern used in the majority of existing files.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read and fix:
- `pkg/git/review_fetcher.go` — `reviewFetcher` struct (~line 49) appears before `NewReviewFetcher` constructor (~line 54)
- `pkg/git/pr_merger.go` — `prMerger` struct (~line 26) appears before `NewPRMerger` constructor (~line 34)
- `pkg/config/loader.go` — `fileLoader` struct (~line 24) appears before `NewLoader` constructor (~line 29)
- `pkg/spec/lister.go` — `lister` struct (~line 39) appears before `NewLister` constructor (~line 45)
- `pkg/lock/locker.go` — `locker` struct (~line 35) appears before `NewLocker` constructor (~line 41)
- `pkg/notifier/telegram.go` — `telegramNotifier` struct (~line 19) appears before `NewTelegramNotifier` constructor (~line 26)
- `pkg/notifier/discord.go` — `discordNotifier` struct (~line 17) appears before `NewDiscordNotifier` constructor (~line 22)
</context>

<requirements>
For each of the seven files listed above:

1. Read the complete file to understand the current layout.

2. Move the constructor function so it appears immediately after the interface (if any) and before the struct definition. The target order in each file is:
   ```
   // Interface (with counterfeiter:generate directive)
   type FooInterface interface { ... }

   // Constructor
   func NewFoo(...) FooInterface { return &foo{...} }

   // Struct
   type foo struct { ... }

   // Methods
   func (f *foo) Method1(...) { ... }
   func (f *foo) Method2(...) { ... }
   ```

3. Do not change any code logic, method implementations, import statements, or comments — only reorder the blocks.

4. After reordering each file, run `make test` to confirm compilation succeeds.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- No logic changes — only reorder struct and constructor declarations
- All paths are repo-relative
</constraints>

<verification>
make precommit
</verification>
