# Dev

## Dev — ticket 2 done: extension scaffold + live session panel

**Layout (`vscode/`, 17 files).**
- `package.json` — manifest: `name`/`publisher` (syware), `displayName`, `description`, `version` 0.1.0, `license`, `main: ./out/extension.js`, `engines.vscode ^1.90.0`, `activationEvents: ["onView:swcode.sessionPanel"]`, `contributes.viewsContainers.activitybar` (container `swcode`, `assets/icon.svg`) + `contributes.views.swcode` (webview view `swcode.sessionPanel`, "Sessions"). Scripts: `build` (esbuild), `watch`, `package` (vsce).
- `tsconfig.json` (extends the repo base, own project — added to `npm run typecheck`), `.vscodeignore`, `assets/icon.svg` (the app's mark, `currentColor`).
- `src/extension.ts` — activate: registers the view provider, the theme listener and the status bar.
- `src/session-panel.ts` — `WebviewViewProvider` (`enableScripts`), redraws on theme change.
- `src/status-bar.ts` — item with the daemon state, probed at activation then every 15 s; `Disposable.from(item, timer)`.
- `src/daemon.ts` — discovery gateway: `SWCODE_DATA_DIR` else `~/.swcode` (via `os.homedir()`), port from `settings.env`, token from `token`, TCP probe.
- `src/lib/settings-env.ts` (pure parser: plain/`export`/quoted values, comments, last value wins, port validation), `src/lib/theme.ts` (`ColorThemeKind` → `light|dark` mapper), `src/lib/webview-html.ts` (pure page builder).
- `src/types/daemon.types.ts`, `src/types/webview-html.types.ts`, `src/swcode.constants.ts`; tests in `vscode/tests/{daemon,settings-env,webview-html}.test.ts` (18 tests, added to vitest's include).
- Root `package.json`: `build:vscode`, `package:vscode`, `typecheck` gains `-p vscode`; devDependencies gain `@types/vscode`, `@vscode/vsce` (esbuild and typescript were already there, so `vscode/` needs no node_modules of its own). `.gitignore` gains `vscode/out/`.

**How the webview gets the bundle: daemon-served URL.** The webview's html is a page with one iframe: `http://127.0.0.1:<port>/vscode.html?daemon=http%3A%2F%2F127.0.0.1%3A<port>&token=<token>&theme=<light|dark>`, with a CSP of `default-src 'none'; frame-src <daemon origin>; style-src 'unsafe-inline'`. Chosen over copied assets because the bundle reads the host contract from the address's query (a webview's own document has no query), the daemon's `--vscode-*` colours inherit into the frame, and the .vsix stays 5 KB with no asset copy step. The extension adds no daemon call: the bundle's socket drives list/open/send/stream/stop/questions.

**Commands.** `npm run build:vscode` (vite → `dist/web/vscode.html`, then esbuild → `vscode/out/extension.js`), `npm run package:vscode` (→ `dist/swcode-0.1.0.vsix`, 5 files). Both ran.

**Verification.** `npm run check` green: biome (3 pre-existing warnings, none in `vscode/`), `tsc -p web|e2e|vscode`, vitest 37 files / 319 tests, `go test ./...`. The bundle in the frame was checked in a headless browser against a daemon on port 4611 (its own data dir): the compact shell renders — nav bar, "llm-server" status banner, project picker, composer — at a 390 px side-bar width, dark theme applied, no layout break. The extension itself is not run inside VS Code here (no IDE on this machine); the .vsix installs via "Install from VSIX".

**Flags (not changed).** The panel opens on the bundle's `/` route (the new-session form); the session list is one nav tap away — the daemon serves `vscode.html` only at that path, so the iframe's path cannot be `/sessions`. The .vsix carries no LICENSE file (vsce warns); ticket 6's packaging can include it.

AGENTS.md: 3 notes added (extension layout + commands, webview/bundle contract + discovery, probe/`@types/vscode` gotchas).

coding-style: 4 fixed, 2 flagged
