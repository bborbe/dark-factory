---
status: completed
summary: Updated github.com/modelcontextprotocol/go-sdk to v1.4.1 and google.golang.org/grpc to v1.79.3 to resolve three CVEs flagged by osv-scanner
container: dark-factory-205-fix-tests-and-dod
dark-factory-version: v0.59.5-dirty
created: "2026-03-20T12:58:05Z"
queued: "2026-03-20T12:58:05Z"
started: "2026-03-20T13:49:25Z"
completed: "2026-03-20T14:14:37Z"
---

<summary>
- All existing tests pass without failures
- Code compiles cleanly with no errors
- Linting and formatting pass
- The full precommit check succeeds end-to-end
- Definition of Done criteria are met for existing code
</summary>

<objective>
Ensure the project is in a healthy state: all tests pass, code compiles, linting succeeds, and the Definition of Done is satisfied. Fix any issues found.
</objective>

<context>
Read CLAUDE.md for project conventions and build commands.
Read `docs/dod.md` for the Definition of Done criteria.
Run `make precommit` to identify any current failures.
</context>

<requirements>
1. Run `make precommit` and capture all failures
2. Fix any compilation errors
3. Fix any failing tests
4. Fix any linting or formatting issues
5. Review code against `docs/dod.md` criteria — fix any violations in files you touched
6. Run `make precommit` again to confirm all issues are resolved
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do NOT refactor code unrelated to fixing failures
- Do NOT add new features — only fix what is broken
- Minimize changes — fix the root cause, not symptoms
</constraints>

<verification>
Run `make precommit` — must pass with exit code 0.
</verification>
