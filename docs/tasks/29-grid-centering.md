# 29 — Grid centering

Spec: §9.2 Grid (amended).

Measured before this task (5 agents, 141 cols): header text spans cols 38–102 (centered), but rows span 36–99 — agent labels are left-aligned in their columns while icons are centered, and the left marker gutter is not balanced on the right, so rows sit ~2 cols left of center.

## Deliver
- Agent column labels centered in their columns with the same rounding as the icons.
- 2-col right gutter balancing the marker gutter; center the whole block.
- Keep everything else of the grid (widths, truncation, cursor style, scrolling).

## Done when
- Tests (fixture with 5 agents, widths 141, 100, 80; unicode icons): for every agent column the icon's column index equals the label's center (label start + (len-1)/2, same rounding); the midpoint of the header row's text and the midpoint of a full row's text (first name char to last icon) are within 1 col of each other and of the screen center; nothing exceeds the terminal; existing grid tests updated. Gate passes.
