---
status: completed
container: dark-factory-216-test-resume-v2
dark-factory-version: v0.67.1
created: "2026-03-21T20:19:41Z"
queued: "2026-03-21T20:19:41Z"
started: "2026-03-21T20:19:47Z"
completed: "2026-03-21T20:20:09Z"
---

<summary>
- Test prompt that sleeps for 5 minutes to test resume-on-restart
</summary>

<objective>
Sleep for 300 seconds then create `test-resume-marker.txt` with content "completed". This gives enough time to kill and restart the daemon mid-execution.
</objective>

<context>
Read nothing — this is a test prompt.
</context>

<requirements>
1. Run `sleep 300` to simulate a long-running task.
2. After sleeping, create `test-resume-marker.txt` with content "completed".
</requirements>

<constraints>
- Do NOT commit anything.
</constraints>

<verification>
`test-resume-marker.txt` exists with content "completed" after the prompt finishes.
</verification>
