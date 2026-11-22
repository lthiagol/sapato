# 27 — Centered footer hints and the title band on every non-main screen

Spec: §9.1 item 4 hint row (now centered), §9.4 "Screens with a title band".

## Deliver
- Footer: hint row (and the wrapped second row) horizontally centered. Everything else about the bar unchanged.
- One shared helper that draws the title band (centered bold title row + full-width Overlay rule, exactly like the grid's column-header band) and centers the content block per §9.4 width rules. Use it for ADOPT, NEW (wizard and paste), IMPORT (source, picker, list, fix-up), EXPORT, THEME, LOG, HELP with the titles in §9.4. Remove each screen's own ad-hoc title/centering.
- Alignment pass: every one of those screens' content block is centered with the same left edge rule; fix inconsistencies you find (e.g. inputs, lists and hint lines starting at different columns).
- Confirms stay modal boxes (unchanged).

## Done when
- Tests: footer row centered (both rows when wrapped); for every listed screen at 120x40 and 60x20: the title band rows are present with the right title, centered, followed by a full-width rule; every content line of the block starts at the same column (or is centered as the block); nothing exceeds the terminal. Existing tests updated, none deleted without replacement. Gate passes.
