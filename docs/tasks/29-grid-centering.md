# 29 — Grid centering by text extent

Spec: §9.2 Grid (amended: center the text extent).

Measured at HEAD (5 agents, unicode): header text 46–94 (center 70.0 at 141 cols), rows 46–91 (68.5): the grid is centered as a padded block whose last column has ~4 blank cells after its icon, so rows sit ~1.5 cols left. Labels and icons already share centers (no change there). A balancing right gutter makes it worse (tried).

## Deliver
- Compute the grid's left offset so the span first SKILL char → last agent icon is centered on the terminal (±0.5). The marker gutter stays left of that span; trailing blank of the last column is not counted. Keep widths, truncation, cursor style, scrolling. Clamp so nothing exceeds the terminal (marker gutter never goes below col 0).

## Done when
- Tests (5 built-in agents, widths 141, 100, 80, unicode icons, a skill with a cell in every agent): row text midpoint (first name char to last icon) within 0.5 of the screen center; header text midpoint within 1.5 of the row midpoint; each icon column == its label's center; nothing exceeds the terminal; existing grid tests (e.g. TestGridIsCentered) updated, none deleted without replacement. Gate passes.
