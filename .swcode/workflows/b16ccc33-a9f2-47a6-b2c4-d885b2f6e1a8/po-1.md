# PO

# Brief understood

**In my words.** Add a VS Code extension that talks to the swcode daemon already running on the same machine, so a developer gets swcode's agent inside the IDE and, at the same time, the agent gets the IDE's intelligence: the language server's errors and hints, the file tree / active file / selection, and refactorings. The extension is a *second UI* for swcode, deliberately smaller than the web app (a subset of its features), and it must reuse the web app's components rather than redraw them, so the two UIs stay cheap to maintain together.

**What the project gives us** (checked): the daemon listens on `127.0.0.1:4500` (`SWCODE_PORT`/`SWCODE_HOST`), serves the web app plus `/api/*` and `/api/socket?token=` (token in `~/.swcode/token`, `daemon/app/network.go`), and pushes `projects`/`sessions`/`presets` at connect (`daemon/app/socket.go`). The UI is `@jadis/core` + `@syware/jlib` custom elements (`web/src/atoms|molecules|organisms|pages`), each self-contained (shadow DOM, `?inline` CSS, `--jl-*` tokens), bundled by `web/vite.config.ts` into `dist/web`. The Windows desktop app (`desktop/webview.go`) is the precedent for a second host rendering the same app. Nothing in the repo touches an editor today.

**Architecture decision** (recorded): a VS Code **webview** rendering the existing `sc-*` components, built from `web/src` by a second bundle entry — not native TreeView-only, not a new component set.

# Assumptions (Dev: take these as given, ask only if one blocks you)

1. **Scope of the "IDE powers"**: diagnostics (errors/hints from the language server), active file + selection + file tree, and refactor-style prompts. VS Code's own editor shows the agent's edits; the extension does not reimplement a file editor.
2. **Feature subset** for v1: session list + conversation + composer + stop + answering a question/permission. Dropped: games, settings page, workflows, Web Push, project page, side panel, presets dialog, xterm terminal.
3. **Connection**: read the port from `~/.swcode/settings.env` (default 4500) and the token from `~/.swcode/token`; connect straight to `ws://127.0.0.1:4500/api/socket?token=` from the webview. No pairing flow (same machine, token file is readable).
4. **Project**: the extension uses the project that matches the opened workspace folder, creating one if absent.
5. **Packaging**: TypeScript extension in a new `vscode/` folder of this repo, built with the existing node toolchain, shipped as a `.vsix`. No new runtime dependency on the daemon; the daemon stays as it is (extension-only changes, plus the shared-UI extraction).
6. **Theming**: the webview maps VS Code's light/dark onto the `--jl-*` tokens; no new theme system.
7. `npm run check` (biome + tsc + vitest + go) stays green; the existing web app must keep working unchanged.

# Open questions (answer with the default if nobody replies)

- Q1: sidebar panel (secondary side bar) as the primary surface, with the editor as the file/diff surface? → default: yes, a `webview` view in the side bar.
- Q2: are diagnostics attached to every prompt, or only on demand? → default: on demand, one "Include problems" affordance, attached to the active file.
- Q3: refactorings = prompts that ask the agent to refactor the selection, or real VS Code refactor providers backed by the model? → default: prompts (cheap, no new daemon API).
- Q4: does the extension start the daemon when none is running? → default: no, it reports "daemon not running" in the status bar.
- Q5: which editors' diagnostics matter first (TS/JS, Go, both)? → default: whatever VS Code reports, no per-language code.

# Backlog (ordered; nothing accepted yet)

1. **Shared UI bundle entry** — extract a reusable UI bundle (app shell + session list + conversation + composer) from `web/src`, built by a second vite entry, previewable in a plain browser page. ← *this ticket, full text below*
2. **Extension scaffold + live session panel** — `vscode/` extension: manifest, activation, daemon discovery (port + token), status bar, side-bar webview showing the bundle, real conversation against the daemon (list, open, send, stream, stop).
3. **IDE context in the prompt** — diagnostics (errors/hints) of the active file, selection and path attached to a prompt; "Ask swcode" editor command / code action.
4. **IDE feedback** — clicking a file in a tool row / diff opens it in VS Code at the right line, and the editor follows the agent's edits.
5. **Refactor commands** — a small set of IDE commands (refactor, explain, fix problem, add tests) that send a preset prompt with selection + diagnostics.
6. **Packaging and docs** — `.vsix` build script wired into the repo's scripts, README section, tests for the shared UI entry.

---

# TICKET 1 — Shared UI bundle entry (extract the reusable UI)

**Goal.** Make the swcode UI consumable by a host other than the web app: one bundle that renders the minimal set of screens (app shell, session list, conversation, composer) from the existing components, so the VS Code extension (ticket 2) only has to load a bundle and a token — no component work.

**Scope.**
- Pick and wire the minimal component set: `sc-app-shell` + `sc-nav` (simplified), `sc-session-row`/sessions list, `sc-conversation` with its molecules (`sc-message`, `sc-thinking`, `sc-tool-row`, `sc-permission`, `sc-question`, `sc-password`, `sc-diff`), `sc-composer` (+ `sc-command-help`, `sc-session-choices`). Keep them as they are — reuse, do not fork; add the few props/slots a host needs (e.g. hide parts, compact header) rather than new parallel components.
- Services needed: `services/daemon.ts`, `access.ts`, `api.ts`, `session-list.ts`, `session-stream.ts`, `navigation.ts`, `theme.ts`, plus `shared/protocol` (messages/session/endpoints). Leave out games, settings, workflows, push, xterm, project page, side panel.
- Add a **second vite entry** (e.g. `web/vscode.html` + `web/src/vscode.tsx`, own CSS entry) built by `npm run build` into `dist/web` alongside the app bundle, with a small host entry point that takes the daemon URL + token (query params, like the app's `?token=`) and renders the shell. Document the entry point and the props a host can pass.
- Theming through the existing `--jl-*` tokens (`styles/base.css`, `styles/global.css`), with the entry ready to accept host-provided token overrides (VS Code colours arrive in ticket 2; here, accept them via CSS custom properties on the root).
- Keep the existing app entry working exactly as today.

**Acceptance criteria.**
- `npm run build` produces the new bundle in `dist/web` next to the app's, and the app's own bundle is unchanged in behaviour.
- Opening the new entry in a browser against a running daemon (`http://127.0.0.1:4500/<entry>?token=…`) shows the session list, opens a session, streams the assistant's reply and tool rows, sends a prompt, stops a turn, and answers a question/permission.
- No component duplicated: the shared pieces come from `web/src/components/*` (state the file list in your report).
- `npm run check` green (biome, `tsc -p web`, vitest, go). Add/adjust a vitest test where the extraction introduces a seam (e.g. the entry's host-config parsing).
- Report the exact entry file, bundle output names, and the host contract (URL, token, CSS variables) — ticket 2 builds on it.

**Notes for Dev.** Follow the project's swcode-base rules (read before editing, business rules in services, small functions, types in `types/`). The e2e suite runs the built bundle, so a web change needs `npm run build` before `npx vitest run --config vitest.e2e.config.ts`.
