# 26 — Skill details: fixed-height agent section

Spec: §9.3 details item 4 (rewritten), §9.5 incompatible paragraph.

## Deliver
- The cursor agent's section is always exactly 2 lines: line 1 `<agent>  <icon> <status word as in the sidebar>` + ` · installed …` when managed + ` · ⊘ not compatible` (Yellow) when incompatible; line 2 indented: installed path, or the would-be install path + ` (not installed)` in Overlay when nothing is installed there.
- Remove the separate `not compatible with <agent>` line.
- The FILES block starts at the same row for every agent of a skill.

## Done when
- Tests: for one skill, the FILES title row index is identical with the cursor on an installed agent, a not-installed agent and an incompatible agent; line 2 shows the would-be path + `(not installed)` dimmed; compat suffix on line 1; status word matches the sidebar's. Gate passes.
