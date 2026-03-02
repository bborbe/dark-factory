---
status: completed
container: dark-factory-051-test-success-completion-report
dark-factory-version: dev
created: "2026-03-02T21:03:55Z"
queued: "2026-03-02T21:03:55Z"
started: "2026-03-02T21:03:55Z"
completed: "2026-03-02T21:04:19Z"
---
<objective>
Verify that dark-factory correctly parses a successful completion report.
This is a trivial test prompt that should always succeed.
</objective>

<context>
Read CLAUDE.md for project conventions.
</context>

<requirements>
1. Create a file `pkg/testdata/hello.txt` with the content "hello from dark-factory test prompt"
2. Verify the file exists by reading it back
</requirements>

<constraints>
- Do NOT modify any existing files
- Do NOT run tests or make precommit (nothing changed that needs validation)
</constraints>

<verification>
Read `pkg/testdata/hello.txt` and confirm it contains the expected content.
</verification>

<success_criteria>
- File `pkg/testdata/hello.txt` exists with correct content
</success_criteria>
