# Theme palettes

Source of truth for `internal/tui/theme`. Every theme fills every role. Hex values are from each project's official palette; do not invent others.

## Roles

| role | used for |
|---|---|
| `Base` | palette data (the screen background is the terminal's own) |
| `Mantle` | palette data (no longer painted; the bar blends with the screen) |
| `Surface` | selected row background, dialog background |
| `Overlay` | rules, muted text, ` · ` separators, `not-installed` icon, tree connectors, dimmed backdrop behind modal confirms |
| `Text` | normal text, skill count in the bar summary |
| `Subtext` | tagline, secondary text, hint descriptions |
| `Accent` | ASCII art, column headers, hint keys, focused pane border, plain confirm border, info notifications |
| `Green` | `synced`, success notifications |
| `Yellow` | `outdated`, "need attention" in the bar summary |
| `Orange` | `modified` |
| `Red` | `invalid`, errors, force confirm border, "invalid" in the bar summary |
| `Purple` | `orphaned` |
| `Cyan` | `unmanaged` |
| `Pink` | `missing` |

## catppuccin-mocha

Base `#1e1e2e` · Mantle `#181825` · Surface `#313244` · Overlay `#6c7086` · Text `#cdd6f4` · Subtext `#a6adc8` · Accent `#89b4fa` · Green `#a6e3a1` · Yellow `#f9e2af` · Orange `#fab387` · Red `#f38ba8` · Purple `#cba6f7` · Cyan `#94e2d5` · Pink `#f5c2e7`

## catppuccin-macchiato

Base `#24273a` · Mantle `#1e2030` · Surface `#363a4f` · Overlay `#6e738d` · Text `#cad3f5` · Subtext `#a5adcb` · Accent `#8aadf4` · Green `#a6da95` · Yellow `#eed49f` · Orange `#f5a97f` · Red `#ed8796` · Purple `#c6a0f6` · Cyan `#8bd5ca` · Pink `#f5bde6`

## catppuccin-frappe

Base `#303446` · Mantle `#292c3c` · Surface `#414559` · Overlay `#737994` · Text `#c6d0f5` · Subtext `#a5adce` · Accent `#8caaee` · Green `#a6d189` · Yellow `#e5c890` · Orange `#ef9f76` · Red `#e78284` · Purple `#ca9ee6` · Cyan `#81c8be` · Pink `#f4b8e4`

## catppuccin-latte

Base `#eff1f5` · Mantle `#e6e9ef` · Surface `#ccd0da` · Overlay `#9ca0b0` · Text `#4c4f69` · Subtext `#6c6f85` · Accent `#1e66f5` · Green `#40a02b` · Yellow `#df8e1d` · Orange `#fe640b` · Red `#d20f39` · Purple `#8839ef` · Cyan `#179299` · Pink `#ea76cb`

## dracula

Base `#282a36` · Mantle `#21222c` · Surface `#44475a` · Overlay `#6272a4` · Text `#f8f8f2` · Subtext `#bfbfbf` · Accent `#bd93f9` · Green `#50fa7b` · Yellow `#f1fa8c` · Orange `#ffb86c` · Red `#ff5555` · Purple `#bd93f9` · Cyan `#8be9fd` · Pink `#ff79c6`

## dracula-alucard

Base `#fffbeb` · Mantle `#efeddc` · Surface `#cfcfde` · Overlay `#6c664b` · Text `#1f1f1f` · Subtext `#6c664b` · Accent `#644ac9` · Green `#14710a` · Yellow `#846e15` · Orange `#a34d14` · Red `#cb3a2a` · Purple `#644ac9` · Cyan `#036a96` · Pink `#a3144d`

## nord

Base `#2e3440` · Mantle `#3b4252` · Surface `#434c5e` · Overlay `#4c566a` · Text `#eceff4` · Subtext `#d8dee9` · Accent `#88c0d0` · Green `#a3be8c` · Yellow `#ebcb8b` · Orange `#d08770` · Red `#bf616a` · Purple `#b48ead` · Cyan `#8fbcbb` · Pink `#b48ead`

## nord-light

Base `#eceff4` · Mantle `#e5e9f0` · Surface `#d8dee9` · Overlay `#4c566a` · Text `#2e3440` · Subtext `#4c566a` · Accent `#5e81ac` · Green `#a3be8c` · Yellow `#d08770` · Orange `#d08770` · Red `#bf616a` · Purple `#b48ead` · Cyan `#5e81ac` · Pink `#b48ead`

Note: nord has no pink; `Pink` reuses purple. nord-light reuses aurora colors for contrast on Snow Storm.
