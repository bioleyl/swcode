# Dev

Ticket 1 — Shared UI bundle entry: implemented, built and verified.

**Files added**
- `web/vscode.html` — the entry page a host ships (module script only, no app favicon/manifest).
- `web/src/vscode.tsx` — the entry: reads the host config, applies the theme, renders `ScAppShell` with a routes subset (session, sessions, new/home), connects the daemon socket and watches its status.
- `web/src/vscode.css` — own CSS entry: `@import './styles/global.css'` plus the `--jl-*` tokens taken from the host's `--vscode-*` variables (each with jlib's value as fallback; unlayered, so they win over jlib's `@layer jlib` tokens).
- `web/src/services/host.ts` + `web/src/types/host.types.ts` — `hostConfig()` parses the host's query (tolerates a missing `location` for server renders/tests).
- `web/tests/host.test.ts` — 6 cases for that parsing (empty, token, daemon+token, theme light/dark, unknown theme, a value with `=`).
- `web/vite.config.ts` — `build.rollupOptions.input = { app: index.html, vscode: vscode.html }`.

**Files changed (host props, no component forked)**
- `web/src/templates/sc-app-shell.tsx` — `routes` property (a `Record<string, RouteDefinition>`, defaults to the app's `ROUTES`, given to the `Router`) and `compact` (hides the phone tab bar, passes `compact` to the nav).
- `web/src/organisms/sc-nav.tsx` — `compact` property: drops the Teams section and the settings link.
- `web/src/services/access.ts` — takes the host's daemon base and token ahead of the paired computer's.
- `web/src/services/theme.ts` — `applyTheme(theme?)` takes the host's theme, the device's stored one by default.

**Components in the bundle** (all from `web/src`, none duplicated): `templates/sc-app-shell` (+ `atoms/sc-brand`, `organisms/sc-connect`, `organisms/sc-project-dialog`, `molecules/sc-computer-switch`), `organisms/sc-nav`, `pages/sessions-page` (+ `molecules/sc-session-row`), `pages/session-page` (+ `organisms/sc-conversation` with `molecules/sc-message`, `sc-thinking`, `sc-tool-row`, `sc-permission`, `sc-question`, `sc-password`, `sc-terminal-wait`, `organisms/sc-checkpoint-diff`, `molecules/sc-title`, `organisms/sc-side-panel`, `sc-review`, `sc-plan`, `sc-peek`, `molecules/sc-session-todos`), `pages/home-page` (the new-session form), `organisms/sc-composer` (+ `molecules/sc-command-help`, `sc-session-choices`). Services: `daemon`, `access`, `api`, `host`, `session-list`, `session-stream`, `navigation`, `presets`, `daemon-status`, `theme`, `work-mode`, `drafts`, `dictation`, `seen-sessions`, `project-list`, plus `shared/protocol`.

**Build output** (`npm run build` → `dist/web`, next to the app's, which is unchanged in behaviour): `vscode.html` (+ `.br`/`.gz`), `assets/vscode-<hash>.js` (437 B) and `assets/vscode-<hash>.css` (3.2 kB), sharing the app's `assets/sc-app-shell-<hash>.js` (574 kB, 162 kB gz) and `assets/shared-<hash>.js` chunks. The daemon serves it as `/vscode.html` (its static handler takes any file name; the `all:dist/web` embed picks it up) — checked with curl (200).

**Host contract for ticket 2**
- URL: `http://127.0.0.1:4500/vscode.html?daemon=<daemon base>&token=<token>&theme=light|dark`; `daemon` optional (empty = the origin that served the page), the socket built as `ws://<daemon>/api/socket?token=`. A webview that ships the bundle's assets passes `daemon`.
- Theme: `theme` sets `data-theme` on the root; the host's colours arrive as `--jl-*` custom properties on the root (`vscode.css` maps `--vscode-sideBar-background`, `-foreground`, `-descriptionForeground`, `-panel-border`, `-button-background/-foreground`, `-input-background`, `-textCodeBlock-background`, `-font-family`, `-editor-font-family` onto the jlib tokens).
- Shell props: `routes` (page subset) and `compact` (narrow pane).

**Verification**
- `npm run check` green (biome, `tsc -p web`, vitest 296 tests, go vet + `go test -race ./...`), exit 0.
- `npm run test:e2e` on the built bundle: 81 passed, 2 failed — the two `decision-map` failures are pre-existing (the same two fail with my changes stashed).
- A browser against a running daemon (`http://127.0.0.1:4611/vscode.html?token=…`): the shell with its navigation (no tab bar, no Teams), the sessions list, opening a session, its conversation and composer, the daemon-status alert; the app's own page renders identically.
- A playwright probe against the e2e rig (daemon + fake llm-server): prompt sent → session opened → turn running (stop offered) → permission answered (Allow) → tool rows drawn (read + edit) → streamed reply shown.

**Notes.** AGENTS.md gained the entry, its host contract and props, plus the probe command (`go build -o dist/e2e/swcode ./cmd/swcode`, then `npx tsx <file>.mts`). Flagged, not changed: the two e2e decision-map tests expect 4 decisions while the map now keeps one node per recorded `decision` call.

coding-style: 1 fixed (host.test.ts quote), 1 flagged
