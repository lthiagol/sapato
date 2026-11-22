# 25 — Incompatible skills in the mock data

Spec: §3 Compatibility, §6 `incompatible`, §7 doctor compat findings.

## Deliver
- `testdata/skill-db/sapato.toml`: append `[skills.mocked-skill-00004] compatible = ["claude"]` and `[skills.mocked-skill-00005] compatible = ["claude", "cursor"]` after the existing comments (keep them).
- `testdata/targets/agents/`: make `mocked-skill-00005` an installed, managed copy there (folder copied from the vault + lock entry with the vault hash), so it is `synced` but `compatible: false` and `doctor` warns. If the agents target already has it, just make sure the state is as described.
- Update every test whose expectations change because of this (status rows, doctor findings, grid renders, counts), and `TestFixturesConsistent` (or its expectations) so it stays green and asserts the new compat state. Don't delete tests; adjust expectations.
- If testdata has a README describing the fixture, add the two skills' compat state to it.

## Done when
- `sapato status` on the fixture shows `incompatible` for mocked-skill-00004 in every agent but claude, and mocked-skill-00005 in `agents` as synced with compatible=false; `doctor` reports the warn. A test pins each. Gate passes.
