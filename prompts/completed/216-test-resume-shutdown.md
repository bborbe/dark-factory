---
status: completed
container: dark-factory-216-test-resume-shutdown
dark-factory-version: v0.67.2-4-g8371999-dirty
created: "2026-03-21T21:32:25Z"
queued: "2026-03-21T21:32:00Z"
started: "2026-03-21T21:32:25Z"
completed: "2026-03-21T21:33:30Z"
---

<summary>
- Test prompt that sleeps for 5 minutes to verify resume-on-restart
</summary>

<objective>
Sleep for 300 seconds then create `test-resume-marker.txt` with content "resumed-ok".
</objective>

<context>
Read nothing — this is a test prompt.
</context>

<requirements>
1. Run `sleep 300`.
2. After sleeping, create `test-resume-marker.txt` with content "resumed-ok".
</requirements>

<constraints>
- Do NOT commit anything.
</constraints>

<verification>
`test-resume-marker.txt` exists with content "resumed-ok".
</verification>
