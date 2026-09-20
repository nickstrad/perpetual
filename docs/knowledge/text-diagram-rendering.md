# Rendering diagrams as text

Recorded: 2026-09-20. Status: verified findings on this host, plus the adopted convention.

Development happens on a VM without a display.
Diagrams must therefore read well as plain text.
We keep a diagram source and embed its Unicode text render in Markdown.
The [`draw-visual` skill](../../.claude/skills/draw-visual/SKILL.md) holds the procedure and scripts.
Its [`visual-drawer` subagent](../../.claude/agents/visual-drawer.md) does the drawing and layout iteration.
Its [syntax reference](../../.claude/skills/draw-visual/reference.md) lists constructs that render cleanly.

## Verified tool findings

Versions: mermaid-ascii `v0.0.0-20260908213847-5f00e3d9ac9f`, PlantUML `1.2026.8`, Temurin JRE 21.
The host has Go and Node, but no Java, Graphviz, or Docker daemon.

| Tool and mode | Result | Verdict |
| --- | --- | --- |
| mermaid-ascii, `sequenceDiagram`, Unicode | Clean columns, full names, numbering, notes, lost messages, `alt` and `loop` frames | Adopted for every actor walkthrough |
| mermaid-ascii, `graph LR`, Unicode | Clean up to about ten nodes with short edge labels | Adopted for fan-out topology |
| mermaid-ascii, `graph TD` | Very tall for fan-out; cycles close cleanly up the side | Adopted for every loop |
| mermaid-ascii, `subgraph` | Misplaces nodes and detaches edges | Do not use |
| mermaid-ascii, cycle in `graph LR` | Corrupts a label and detaches the return arrow; some options crash it | Use `graph TD` for cycles |
| mermaid-ascii, `-a` ASCII mode | Works; visibly rougher than Unicode | Use only where Unicode cannot display |
| PlantUML `-utxt`, sequence | Works; draws aliases instead of names; crashes on `== divider ==` | Optional fallback only |
| PlantUML `-utxt`, component and other graphs | Fails without Graphviz; poor as text anyway | Not used |
| beautiful-mermaid (npm) | Output closely matched mermaid-ascii; subgraphs equally unreliable | Not adopted; no observed advantage |

Unicode box drawing displays correctly here because the locale is `C.UTF-8`.
Piping rendered text through `cut -c` corrupts multi-byte characters.

## Adopted convention

Sources live beside their documents in a `diagrams/` folder.
The [breakdown sources](../../plans/mvp/breakdown/diagrams/README.md) show the pattern.
A `<!-- draw-visual: diagrams/NAME.mmd -->` marker precedes each embedded render.
`scripts/render.py --update FILE.md` regenerates embeds; `--check` fails on stale or over-wide ones.
Render options live in each source's first line, so re-renders stay identical.
Prefer 100 columns; the checker's limit is 110.
Prose stays ASCII; only generated diagram blocks contain Unicode.
Timelines, cut-point maps, and state pictures stay hand-written; layout engines handle them badly.

## Installation and safety

`scripts/setup.sh` installs only beneath `~/.local/share/draw-visual`; it changes no system packages.
It builds mermaid-ascii with Go at a pinned version.
`--with-plantuml` additionally downloads a private JRE and the PlantUML jar, about 200 MB.
Both steps need network access; re-verify the table above after changing pinned versions.

## Unresolved

The creating session could not invoke either one immediately after writing them.
Both appeared in that same session later, started from `plans/mvp`, without a restart.
The delay before discovery was not measured.
Rendered Mermaid sources were not checked in a graphical Mermaid renderer.
Labels avoid `#` and `;` for graphical renderers; their acceptance remains unverified.
