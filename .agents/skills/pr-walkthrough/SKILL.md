---
name: pr-walkthrough
description: Explain a pull request in an ignored Markdown walkthrough, following its Files changed list from bottom to top with regenerable terminal text diagrams. Use when someone wants to read a PR side by side with an explanation; not for posting a review.
---

# PR walkthrough

Create a local reading companion for a PR. Accept a PR URL, number, or “current branch.” Write the Markdown file to `.scratchpad/pr-walkthrough/<pr>-walkthrough.md` at the repository root; use `branch-<safe-name>` when no PR can be resolved. Put diagram sources beside it in `.scratchpad/pr-walkthrough/diagrams/`. The repository ignores `/.scratchpad/`. Do not stage, commit, comment on, review, or publish the walkthrough.

## Establish the source and reading order

Use an available read-only PR provider or `gh` to get the PR title, URL, description, base/head commit IDs, complete changed-file list in **Files changed display order**, and the complete patch. For “current branch,” resolve its open PR when possible. Fetch additional diff pages or individual files if output is truncated, and read relevant base/head source and tests around each change. Do not claim to have read a truncated patch.

If the provider exposes only patch order and the displayed order cannot be checked, reverse that patch order and label the substitution prominently. Never present it as verified Files changed order or silently rearrange files by dependency.

If PR access or authentication is unavailable, use the local base/head diff only when those revisions can be identified. Clearly label the result a **local diff walkthrough**, give the exact revisions and diff order used, and do not invent PR metadata or file anchors. If even that comparison is ambiguous, state the missing reference needed to produce an accurate walkthrough.

Record the source URL, base/head IDs, order source, and collection time in the document. Check that the head did not change while reading; refresh or mark the walkthrough stale if it did. Keep each original path, file position, and hunk locator (for example `@@ -a,b +c,d @@`), plus a direct Files changed anchor when verified. A general PR Files changed link is useful when a file-specific anchor is unavailable; never fabricate one.

Write file sections in the **reverse of the captured Files changed order**, starting with the last file shown in the PR and working upward. Preserve that navigation even when dependencies or execution flow suggest another teaching order. Say where the reader should look next above. For renames, deletions, binary files, and generated files, retain their positions and explain the observable change or the limit of the patch.

## Explain what the reader is seeing

Open with a brief orientation and the order convention. For each file, connect the changed lines to their purpose, the behavior before and after, the relevant owner or boundary, and any invariant, failure path, or test that makes the change understandable. Ground claims in the diff and surrounding code; distinguish observed code, PR claims, and your inference. Cover the whole PR, but spend detail where the mechanism is hard. Do not turn the walkthrough into an unsolicited review or assert that tests passed unless you have evidence.

Use the `on-writing-well` skill when drafting and revising the explanation: concrete actors and actions, one main point per paragraph, needed technical terms, and no stronger claims than the evidence supports. Preserve identifiers, conditions, and uncertainty. Keep paragraphs short enough to scan beside the PR.

Use the `draw-visual` skill for regenerable Unicode diagrams whenever a flow, state change, ownership boundary, or relationship is easier to see than to describe. Favor a useful extra diagram over leaving a complex section visually opaque, while giving each visual a distinct question to answer. Prefer Mermaid sources, render into Markdown embeds, and follow that skill’s width and visual-inspection checks. Aim for about 70 columns so the walkthrough fits beside the PR; adjust to the reader’s viewport. Hand-written text tables or cut-point maps are fine where the visual skill recommends them. Keep every source in the ignored diagrams folder and never hand-edit rendered blocks.

Finish by checking section order against the captured file list, links and hunk locators against the PR, diagram source/embed consistency, readable width, and the absence of unsupported claims. Report the ignored walkthrough path and any unavailable PR data or uninspected visual previews.
