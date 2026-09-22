# Rendering diagrams as text

Recorded: 2026-09-20. Status: verified findings on this host, plus the adopted convention.

Development happens on a VM without a display.
Diagrams must therefore read well as plain text.
We keep a diagram source and embed its Unicode text render in Markdown.
The global `draw-visual` skill holds the procedure, scripts, and `reference.md` syntax guide.
Claude's copy lives at `~/.claude/skills/draw-visual/` and delegates drawing and layout
iteration to `~/.claude/agents/visual-drawer.md`.
Codex's copy lives at `~/.codex/skills/draw-visual/` and runs the workflow directly.
These are user-level installations; this repository does not bundle the skill.

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
Use the document's platform actor names: Terminal, `perpetual` CLI, `agent-plane`,
PostgreSQL, Firecracker, `vmagent`, and Command; do not substitute "user" or "human".
Diagrams describe planned behavior unless the document says otherwise.

## Installation and safety

`scripts/setup.sh` installs only beneath `~/.local/share/draw-visual`; it changes no system packages.
It builds mermaid-ascii with Go at a pinned version.
`--with-plantuml` additionally downloads a private JRE and the PlantUML jar, about 200 MB.
Both steps need network access; re-verify the table above after changing pinned versions.
The global Claude and Codex copies share this renderer installation.

On 2026-09-20, the skill and Claude agent moved out of this repository to the global
locations above. Both skill variants rendered Mermaid and PlantUML examples, regenerated
Markdown embeds without changing surrounding prose, and rejected stale embeds.
All 47 existing embeds across 14 breakdown documents passed the renderer's check.

## Unresolved

Global skill discovery in a fresh Claude or Codex session has not been verified;
validation invoked the installed scripts directly.
Rendered Mermaid sources were not checked in a graphical Mermaid renderer.
Labels avoid `#` and `;` for graphical renderers; their acceptance remains unverified.
