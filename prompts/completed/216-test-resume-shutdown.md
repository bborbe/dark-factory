---
status: completed
container: dark-factory-216-test-resume-shutdown
dark-factory-version: v0.67.1
created: "2026-03-21T20:54:39Z"
queued: "2026-03-21T20:54:39Z"
started: "2026-03-21T20:54:41Z"
completed: "2026-03-21T20:56:01Z"
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
