# Detailed planning and resumption

Recorded 2026-09-23. This describes a documentation workflow; it does not establish implemented platform behavior.

The project uses one canonical [detailed-design skill](../../.agents/skills/detailed-design/SKILL.md). Codex discovers it under `.agents/skills/detailed-design/`; Claude discovers the same folder through `.claude/skills/detailed-design`, a relative directory symlink. Edit the canonical skill and its linked references/assets rather than maintaining divergent copies. Keep the existing root `AGENTS.md -> CLAUDE.md` symlink intact.

The discovery paths and Claude's directory-symlink support were checked against the official [Codex skill documentation](https://learn.chatgpt.com/docs/build-skills) and [Claude skill documentation](https://code.claude.com/docs/en/skills). Installing these files does not establish that an already-running client has reloaded its skill catalog.

General guidance names responsibilities rather than model families. The skill owns model-selection preferences, while each detailed plan contains explicit assignments, alternatives, file ownership, dependencies, and acceptance evidence. Historical assignments in older reference plans are not current policy.

The active slice 1 design is [durable requests design](../plans/durable_requests/plan.md). Its companion `.state/1-durable-requests/plan_state.md` is local and ignored. Each future detailed plan gets its own `.state/<plan-slug>/plan_state.md`; each worker retains a separate work log. The lead appends snapshots covering all tasks and actual evidence so another session or tool can resume safely.

State files are not durable project documentation and are never committed. A new checkout may lack them; reconstruct state from the plan, actual diffs and durable evidence, marking unknown execution history. See [shared agent instructions](../../CLAUDE.md#working-state) for state ownership and cleanup. Save lasting findings in indexed knowledge entries before discarding finished work logs.

Planning produces substantial unexecuted code sketches and specified TDD suites. Implementation, when requested, begins with contract/skeleton setup and meaningful failing behavior tests, then bounded production assignments, independent review and lead acceptance. Read the skill for the detailed procedure rather than duplicating it here. Delivered slice status remains in [project direction](project-direction.md).
