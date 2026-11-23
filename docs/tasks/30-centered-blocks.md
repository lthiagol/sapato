# 30 — Titled screens: block as wide as its content, centered

Spec: §9.4 "Screens with a title band" (width rule amended).

Before: every titled screen used a fixed two-thirds-wide block with left-aligned content, so short content (ADD's rows, the NEW wizard, ADOPT) hugs the left: at 141 cols content starts at col 23 while the title is centered at ~70.

## Deliver
- screenBody width = clamp(widest content line, min(60, W), max(60, 2W/3)); block centered; content left-aligned; inputs fill the block.
- Steps of one screen share one width (max over their widths): IMPORT (source, picker, list, fix-up) and NEW (wizard, paste). Compute it so it doesn't depend on which step is open (e.g. a per-screen width function over the content each step would render, or a stable floor such as the input width).
- Check ADD's rows share one left edge whether or not the cursor marker is on them.

## Done when
- Tests at 141x40, 100x30, 60x20 for every titled screen: block left margin == right margin ±1 (centered), measured as (W − block width)/2 where block width is the widest content line; ADD, NEW wizard and ADOPT at 141 are visibly centered (left margin > 30); IMPORT steps all have the same left edge; NEW wizard and paste the same left edge; ADD rows share one left edge with the cursor on each row; nothing exceeds the terminal. Existing band/width tests updated. Gate passes.
