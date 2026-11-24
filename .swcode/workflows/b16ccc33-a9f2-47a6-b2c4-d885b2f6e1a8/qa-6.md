# PO review

**Ticket 2 accepted.** The scaffold and the live session panel meet the goal and every acceptance criterion I could check on this machine.

**What I checked (not only the report).**
- `npm run build:vscode` runs both steps: vite → `dist/web/vscode.html` + `assets/vscode-*.js` sharing the app's `sc-app-shell` chunk (no component duplicated), esbuild → `vscode/out/extension.js`. `npm run package:vscode` produces `dist/swcode-0.1.0.vsix` (5 files, 5.1 KB) with a valid manifest.
- `npm run check` green end to end: biome, `tsc -p web|e2e|vscode` (the extension's TS is its own project, out of the web tsconfig), vitest 37 files / 319 tests (the 18 new `vscode/tests` included), `go test ./...`.
- The webview's page, rendered in a headless browser against a real daemon (port 4611, its own data dir): the compact shell draws the nav, the daemon status banner, the project picker and the composer at a 390 px side-bar width, dark theme applied, no layout break, no console errors. The Sessions route renders the list/empty state, and the app's hash routing keeps the iframe's `src` on `/vscode.html?daemon=…&token=…&theme=…` — so the dev's flag (the panel opens on `/`, the list is one nav tap away) costs nothing.
- The manifest has what the ticket asks (`name`, `publisher`, `engines.vscode`, `activationEvents`, `contributes.viewsContainers.activitybar` + `contributes.views` webview view, `main`); discovery reads `SWCODE_DATA_DIR` → `~/.swcode` with `os.homedir()`, the port from `settings.env` (default 4500, validated) and the token from `token`; the status bar probes at activation then every 15 s and the webview keeps working when the daemon is down. The extension adds no daemon call, as the ticket says.
- Layout, commands and the webview HTML are reported in full, which is what ticket 3 needs: the bundle comes from the daemon-served URL (no asset copy step), so ticket 3 reads nothing else.

**Notes for later tickets, not fixes (nothing here sends it back).**
1. The webview sets no `retainContextWhenHidden`, and a theme change rewrites the webview's html, so the iframe reloads: the conversation's scroll position and a composer draft are lost when the panel is hidden/shown or the IDE theme changes. Fine for ticket 2; worth a `retainContextWhenHidden` (or passing `theme` by postMessage) when ticket 3/4 touch the panel.
2. The .vsix carries no LICENSE (vsce warns) — noted for ticket 6's packaging, along with a real "Install from VSIX" smoke test, since this machine has no IDE.
3. The status bar's probe is a TCP connect, not `/api/status`; enough for connected / not running.

AGENTS.md: kept the dev's 3 notes (I merged a duplicated decision-map note and added the recipe for checking the webview without VS Code).
