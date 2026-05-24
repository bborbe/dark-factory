---
status: draft
created: "2026-05-24T00:00:00Z"
---

<summary>
- Extracted sequential concerns in dockerSpecGenerator.Generate() into named phase helpers
- The 64-line method now delegates to: isGitLocked, isGenerationRunning, markSpecGenerating, executeAndFinalize, resetSpecToApproved
</summary>

<objective>
Extract sequential concerns in pkg/generator/generator.go Generate method into named helpers.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes:
- `pkg/generator/generator.go` — lines 101-164, Generate method with 8 sequential concerns
</context>

<requirements>
1. In `pkg/generator/generator.go`, refactor `Generate` method to extract named helpers:

   ```go
   func (g *dockerSpecGenerator) Generate(ctx context.Context, specPath string) error {
       if g.isGitLocked(ctx) { return nil }
       if g.isGenerationRunning(ctx, specBasename, containerName) {
           return g.reattachAndFinalize(...)
       }
       if err := g.markSpecGenerating(ctx, specPath); err != nil { return err }
       prompted, err := g.executeAndFinalize(...)
       if !prompted && ctx.Err() == nil { g.resetSpecToApproved(ctx, specPath) }
       return nil
   }
   ```

2. Create private helper methods for each concern:
   - `isGitLocked(ctx)` — git index lock check
   - `isGenerationRunning(ctx, specBasename, containerName)` — container liveness check + reattachAndFinalize
   - `markSpecGenerating(ctx, specPath)` — state transition
   - `executeAndFinalize(...)` — execution
   - `resetSpecToApproved(ctx, specPath)` — error recovery
</requirements>

<constraints>
- Only change files in this repo
- Do NOT commit — dark-factory handles git
- Use `errors.Wrap`/`errors.Errorf` from `github.com/bborbe/errors` — never `fmt.Errorf` or bare `return err`
</constraints>

<verification>
make precommit
</verification>
