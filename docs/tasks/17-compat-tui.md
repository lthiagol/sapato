# 17 — Compatibility in the TUI

Spec: §9.2 grid, §9.3 skill screen keys (`A`, `c`), §9.5 incompatible icon and skill-screen lines, §9.4 HELP legend. Depends on task 16 (ops API is in place).

## Deliver
- Icon sets: `incompatible` (nerd ``, unicode `⊘`, ascii `-`), Overlay color; grid cells for `incompatible` pairs.
- Skill screen: agent row suffix ` ⊘` for installed-but-incompatible; details line `not compatible with <agent>` (Yellow) for that agent; `compat  all agents|<list>` line under `vault`.
- `c` toggles compatibility for marked agents (or cursor agent) via `ops.SetCompat`; disabled with an error notification for skills not in the vault; notification on success; refresh.
- `A` skips incompatible agents and reports them in its notification.
- Help legend includes the new icon.

## Done when
- Tests: icon per set; grid renders the incompatible icon; sidebar suffix; details lines; `c` on cursor/marked agents writes the expected table (temp vault) and toggling back to all deletes it; `c` on a non-vault skill → error notification, no write; `A` skips incompatible. Gate passes.
