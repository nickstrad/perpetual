# Project guidance

This repository exists to teach understandable systems engineering through a useful VM platform.
Prefer clear, maintainable designs that readers can explain and change confidently.
Cleverness is not a goal; complexity must solve a demonstrated problem.

## Design for understanding

Make ownership, state changes, side effects, and failure handling visible.
Choose straightforward control flow and concrete names over compact tricks.
Keep responsibilities small without scattering one operation across unnecessary abstractions.
Introduce interfaces where they clarify ownership or enable meaningful failure testing.
Prefer established components over custom frameworks unless a concrete requirement demands otherwise.
Our focused custom simulator teaches coordination, fault handling, and reproducible debugging.
Keep its scope small; learning does not justify rebuilding Go's runtime.
Explain surprising decisions and tradeoffs near their implementation.
Define unfamiliar terms before relying on them in documentation.
Start with the [knowledge index](docs/knowledge/index.md) and preserve useful findings there.
Follow its [organization rules](docs/knowledge/README.md); update entries when their assumptions change.

Use the simplest design that preserves required behavior and recovery guarantees.
Do not simplify away uncertain outcomes, durability boundaries, or resource ownership.
An honest limitation teaches more than an unsupported reliability claim.

## Testing as engineering and learning

Treat tests as executable explanations of behavior, failures, and recovery.
Each technique must address a named risk and justify its maintenance cost.
Start with readable examples and real local components.
Add controlled faults, decision checks, or broader techniques when they expose concrete gaps.
Avoid frameworks, paradigms, and coverage targets chosen mainly for appearance.

[TESTING.md](TESTING.md) explains the adopted techniques, their purpose, and their limits.
Keep it accurate as the repository grows; commit updates alongside relevant testing changes.
Distinguish implemented checks, planned work, observed results, and unresolved assumptions.
Explain how to reproduce failures, not merely how to run successful examples.
The [simulation research](docs/TEST_RESEARCH.md) explains our chosen boundaries and alternative tools.
Follow [Go TigerStyle](docs/TIGERSTYLE.md) for concrete coding and invariant guidance.

## Ownership and review

The assigned test owner owns testing strategy, test implementation, fixtures, test infrastructure, and `TESTING.md`.
Production owners collaborate on testability and fix defects in their assigned components.
The lead assistant owns final review, validation, plan updates, and commits.
Assign concrete models and file ownership in the current plan using the [detailed-design skill](.agents/skills/detailed-design/SKILL.md).
Use a separate reviewer for independent review; the lead retains final acceptance responsibility.

Read the [MVP breakdown](docs/plans/README.md) before implementing platform changes.
[Project direction](docs/knowledge/project-direction.md) records delivered slices; assignments alone never imply completion.
Track in-progress work in untracked `.state/` files, as [CLAUDE.md](CLAUDE.md#working-state) describes.
Expand one slice at a time while keeping later plans mid-level.
Keep changes scoped, preserve unrelated work, and document material decisions.
Favor improvements a future reader can understand without reconstructing the entire conversation.
