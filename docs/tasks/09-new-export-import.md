# 09 — `new`, tarball export/import (ops + CLI)

Spec: §7 (`new`, `export`, `import`), §11.

## Deliver
- `ops.Create(vault, content []byte, opts{Force, DryRun})`: parse frontmatter name, validate exactly like vault skills (§3) by staging into a temp dir, then place via the existing set-aside/restore placement. Refuse existing name unless Force (wrap `ErrNeedsForce`).
- `ops.Export(vault, skills []string, w io.Writer)` and `ops.Import` support for `*.tar.gz`/`*.tgz` per §11 (manifest, determinism, all safety limits, all-or-nothing, `--name` only for single-skill archives).
- CLI: `sapato new [file]` (stdin if no file or `-`), `sapato export <skill…> [-o file]` (default names per §7), `import` accepts tarballs.

## Done when
- Tests: create valid/invalid/existing/force/dry-run; export→import round trip preserves hashes for every valid fixture skill (00001–00006) incl. exec bits; export is byte-identical across two runs; each import safety rule rejected with a crafted archive (abs path, `..`, symlink, hardlink, top-level file, size cap, entry cap); multi-skill archive with one invalid skill imports nothing; `--name` rejected for multi-skill archive.
- `gofmt -l .` empty, `go vet ./...`, `go test -count=1 ./...` pass.
