---
status: completed
container: dark-factory-217-test-resume-v3
dark-factory-version: v0.67.1
created: "2026-03-21T20:21:04Z"
queued: "2026-03-21T20:21:04Z"
started: "2026-03-21T20:21:08Z"
completed: "2026-03-21T20:21:38Z"
---

<summary>
- Test prompt that sleeps for 5 minutes to verify resume-on-restart
</summary>

<objective>
Sleep for 300 seconds then create `test-resume-marker.txt` with content "completed".
</objective>

<context>
Read nothing — this is a test prompt.
</context>

<requirements>
1. Run `sleep 300`.
2. After sleeping, create `test-resume-marker.txt` with content "completed".
</requirements>

<constraints>
- Do NOT commit anything.
</constraints>

<verification>
`test-resume-marker.txt` exists with content "completed".
</verification>
