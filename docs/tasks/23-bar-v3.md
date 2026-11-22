# 23 — Bottom bar v3: summary in the rule, one left-aligned hint row

Spec: §9.1 item 4 (rewritten), §9.2 "Bar summary". Replaces the two-row bar of task 14.

## Deliver
- The bar rule carries the summary right-aligned (` … ` + `──` to the edge), with the color and dimming rules, and the narrow-terminal drop order (attention, then invalid, then plain rule).
- One hint row, left-aligned with a 1-col pad: action hints, ` │ `, global hints drawn one step dimmer (key Subtext, desc Overlay). FILTER and confirm replace the action part only.
- Fitting: globals wrap to a second row when the row is too wide; then drop `t theme`, then `L log`; action part truncated with `…` if alone too wide. Body height accounts for the bar being 2 or 3 lines; notifications stay directly above the bar.
- Every screen and mode that draws the bar (MAIN, SKILL, FILTER, dialogs, confirms, LOG, the import/wizard/adopt modes) uses the new layout; remove the old row-1/row-2 code.

## Done when
- Tests: summary in the rule at 120 cols (exact right edge `──`), colors per part and dimming at 0; drop order at narrow widths down to a plain rule; hint row left-aligned with `│` and dimmer globals; wrap to two rows at a width where one row doesn't fit; `t theme`/`L log` dropped in order; FILTER and confirm action parts; body height correct for 2- and 3-line bars (nothing exceeds the terminal at 60x20 and 120x40); existing bar tests updated, none deleted without replacement. Gate passes.
