# PO review

Ticket 1 — Shared UI bundle entry: **accepted**. Every acceptance criterion checked against the delivered work, not just the report.

**Verified**
- `npm run build` puts `vscode.html` (+ `.br`/`.gz`) and `assets/vscode-<hash>.js`/`.css` in `dist/web` beside the app's `index.html`; the vscode chunk is a thin entry that shares the app's `sc-app-shell-*.js` (574 kB) and `shared-*.js` — no component duplicated.
- The four changed files are props/services only, no component forked: `ScAppShell.routes` (defaults to the app's `ROUTES`, passed to the `Router`) + `compact` (hides the phone tab bar, forwarded to `ScNav`, which drops the Teams section and the settings link); `access.ts` takes the host's `daemon`/`token` ahead of the paired computer's; `applyTheme(theme?)` takes the host's theme.
- `npm run check` green here (exit 0, biome + tsc + vitest 296 + go vet + `go test -race`); `web/tests/host.test.ts` runs 6/6 for the host-config seam.
- The report's browser run against a live daemon covers the full flow (list, open, stream + tool rows, send, stop, answer a permission), and the playwright probe against the e2e rig confirms the socket path.
- The host contract (URL `…/vscode.html?daemon=&token=&theme=`, the `--vscode-*` → `--jl-*` mapping in `vscode.css`, the shell props) is stated in the report and in AGENTS.md, so ticket 2 (extension scaffold + live session panel) can build on it as-is.

**Notes for the next tickets (no rework asked)**
- The two failing e2e `decision-map` tests are pre-existing (they expect 4 decisions, the map now keeps one node per recorded `decision` call); they need a small fix at some point, not in ticket 2's scope.
- The bundle ships `sc-checkpoint-diff` for the ticket's `sc-diff`; fine.

Take the `ack` exit: back to PO for ticket 2 (extension scaffold + live session panel).
