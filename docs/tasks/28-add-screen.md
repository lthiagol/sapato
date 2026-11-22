# 28 — ADD screen on `n`

Spec: §9.2 keys (`n` add; `a`/`I` hidden shortcuts listed in HELP), §9.4 `ADD`. Depends on task 27's title-band helper.

## Deliver
- `n` on the main screen opens ADD (title `Add a skill to the vault`): Create / Import / Adopt rows per §9.4, live unmanaged count, Adopt dimmed and not selectable at 0 (`no unmanaged skills`). `↑↓`+`enter` or `c`/`i`/`a` open NEW, IMPORT, ADOPT; `esc` back to main; `esc` in those screens returns to main.
- Main bar hint `n add`; `a` and `I` removed from the bar but still work; HELP lists them.

## Done when
- Tests: `n` opens ADD with the three rows and the right count; letters and enter open the right screen; Adopt disabled at 0 (letter and enter do nothing / error notification, cursor skips it); esc paths; bar shows `n add` and no `a adopt`/`I import`; `a` and `I` still open their screens; HELP lists them. Gate passes.
