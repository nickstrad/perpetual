# Diagram sources

This folder holds the sources for slices 2–7 and their shared decision loop.
Slice 1 sources live in [durable_requests/diagrams](../durable_requests/diagrams/README.md).
Mermaid sources end in `.mmd`; rendered text lives inside the Markdown files.
Each embed starts with a `<!-- draw-visual: diagrams/NAME.mmd -->` marker.
Never edit rendered text; change the source, then regenerate.

Run from `docs/plans/` using either global installation:

```bash
S="$HOME/.claude/skills/draw-visual/scripts"
# For Codex: S="$HOME/.codex/skills/draw-visual/scripts"
"$S/setup.sh"                      # install the renderer once per machine
"$S/render.py" diagrams/NAME.mmd   # preview one diagram (replace NAME)
"$S/render.py" --update FILE.md    # regenerate a document (replace FILE)
"$S/render.py" --check *.md        # fail on stale or over-wide embeds
```

Use the `draw-visual` skill to add or change a diagram.
Read the [rendering knowledge entry](../../knowledge/text-diagram-rendering.md) for tool choices and limits.

| Pattern | Used by |
| --- | --- |
| `N-name-walkthrough-K.mmd` | Walkthrough K in slice plan N |
| `N-name-simulator.mmd` | Simulator walkthrough in slice plan N |
| `architecture-topology-slice-*.mmd` | Connection diagram in the architecture files |
| `architecture-decision-loop.mmd` | Decision loop shared by every architecture file |
| `architecture-simulator-slice-N.mmd` | Simulator loop in architecture file N |

Timelines, cut-point maps, and state pictures stay hand-written in their documents.
