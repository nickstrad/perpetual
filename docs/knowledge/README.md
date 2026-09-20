# Project knowledge

This folder preserves useful project knowledge for future humans and agents.
Start with [index.md](index.md), then read entries relevant to your work.
Follow [project guidance](../../GUIDANCE.md) and authoritative plans when making changes.

## What belongs here

Record durable discoveries, decision rationale, debugging lessons, and reproducible investigation results.
Explain why a finding matters and when someone should use it.
Distinguish verified observations, planned behavior, and unresolved questions.
Include the verification date, relevant versions, and evidence when they affect validity.
Link existing plans and guides rather than copying details that can drift.
Keep temporary progress in the plan tracker, not in permanent knowledge entries.
Never include credentials, private runtime data, or unexplained generated dumps.

## Organization

- Store text-only summaries as descriptive `*.md` files.
- Give entries containing supporting artifacts their own descriptively named folders.
- Artifacts include documents, scripts, datasets, and nested folders.
- Add `README.md` to every folder, including nested artifact folders.
- Explain each folder's purpose, contents, dependencies, and safe usage in its README.
- Document script effects and prerequisites; reading an entry never authorizes execution.
- Update [index.md](index.md) for every added, moved, renamed, or removed file and folder.
- Index nested artifacts and README files, not just top-level entries.
- Prefer small, curated evidence over copied repositories or large generated outputs.

## Maintenance

Update relevant entries whenever implementation invalidates their assumptions.
Mark superseded findings clearly and link their replacements when history remains useful.
Remove obsolete material when retaining it would mislead readers.
Review links and index completeness alongside documentation changes.
