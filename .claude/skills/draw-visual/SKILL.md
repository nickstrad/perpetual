---
name: draw-visual
description: Draw a diagram as terminal-readable text from a Mermaid (preferred) or PlantUML source, and embed it in a Markdown document so it can be regenerated. Use when a document in this repository needs an architecture, topology, flow, state, or sequence visual, or when an embedded diagram must be updated. Runs in the visual-drawer subagent.
when_to_use: Requests such as "draw", "diagram", "visualize", "sequence diagram", "architecture picture", "update the diagram", or any new or changed visual in plans/ or docs/.
argument-hint: "[what to draw, the Markdown file to embed it in, and where the source should live]"
context: fork
agent: visual-drawer
---

# Draw a visual

Request: $ARGUMENTS

This host is a VM without a display, so every diagram is text.
You write a diagram source, render it to Unicode text, and embed that text in Markdown.
The source is the truth; rendered text is never edited by hand.

If the request above is empty or unclear, report what is missing instead of guessing.

## Steps

1. Read the target Markdown file and any document the request names, so labels match its actor names.
2. Make sure the renderer exists: `${CLAUDE_SKILL_DIR}/scripts/setup.sh` (idempotent; add `--with-plantuml` only when you need PlantUML).
3. Choose the diagram type using the table below, then read `${CLAUDE_SKILL_DIR}/reference.md` for syntax that renders cleanly and for known failures.
4. Write the source beside the document, in a `diagrams/` folder unless the request says otherwise. Name it `<document-stem>-<purpose>.mmd`, or use a shared prefix when several documents embed one source.
5. Render it: `${CLAUDE_SKILL_DIR}/scripts/render.py path/to/source.mmd`. Look at the output. Check every edge lands on the right box, no label is cut, and no line is wider than 110 columns (100 preferred).
6. Iterate on the source until it is clean: shorten labels, change direction, tune padding options, or split one picture into two. A wrong-looking diagram is worse than none; if it cannot be made clean, say so and propose a table or list instead.
7. Embed it. Put this marker and an empty fenced block where the diagram belongs, then run `render.py --update FILE.md`:

   ````markdown
   <!-- draw-visual: diagrams/3-topology.mmd -->
   ```text
   ```
   ````

8. Run `render.py --check FILE.md` and report: source paths, embed locations, final width, and anything you could not draw well.

## Choosing a diagram type

| The visual shows | Use | Notes |
| --- | --- | --- |
| Actors exchanging messages over time | Mermaid `sequenceDiagram` | Best-looking output. Supports `autonumber`, notes, lost messages, `alt`, `loop`. |
| Components fanning out from a hub | Mermaid `graph LR` | At most about ten nodes. No subgraphs. Short edge labels or arrow numbers with a key below. |
| Anything containing a cycle or feedback loop | Mermaid `graph TD` | The back-edge returns up the side as a closed rectangle. `graph LR` cycles garble labels or crash the renderer. |
| Sequence needing PlantUML-only features | PlantUML sequence | Text mode supports sequences only, and crashes on `== divider ==`. |
| Timelines, tables of state, cut-point maps | Not this skill | Hand-written text or a Markdown table communicates better. |

## Rules for this repository

- Follow `CLAUDE.md`, `GUIDANCE.md`, and the [diagram knowledge entry](../../../docs/knowledge/text-diagram-rendering.md).
- Use the actor names the document already uses: Terminal, `perpetual` CLI, `agent-plane`, PostgreSQL, Firecracker, `vmagent`, Command. Never "user" or "human".
- Diagrams describe planned behavior unless the document says otherwise; never imply something is implemented.
- Keep render options in the source's first line (`%% draw-visual: -x 4 -y 1 -p 0`) so re-renders match.
- Do not commit, and do not change prose outside the embed unless the request asks.
