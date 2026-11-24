# PO

# PO — ticket 1 accepted, handing off ticket 2

**Brief, in my words.** A VS Code extension that is a second, smaller UI for the swcode daemon on the same machine: the developer gets swcode's agent inside the IDE (session list, conversation, composer) and the agent gets the IDE's intelligence (diagnostics, active file/selection, refactor prompts). The UI reuses the web app's `sc-*` components through one shared bundle, so nothing is drawn twice.

**Backlog (ordered).** 1 Shared UI bundle entry — *accepted*. 2 Extension scaffold + live session panel — *this ticket*. 3 IDE context in the prompt (diagnostics, selection, "Ask swcode"). 4 IDE feedback (open files/diffs in the editor at the right line). 5 Refactor commands. 6 Packaging (.vsix) and docs. Assumptions and defaults from po-1 still stand (side-bar webview, on-demand diagnostics, prompt-style refactorings, no daemon autostart, `vscode/` folder, `--jl-*` theming).

---

# TICKET 2 — Extension scaffold + live session panel

**Goal.** A VS Code extension in `vscode/` that a user installs and immediately sees swcode's session panel in the side bar, talking to the daemon on the same machine: list sessions, open one, watch it stream, send a prompt, stop the turn. It loads ticket 1's bundle — no component work in the extension.

**Scope.**
- **Scaffold**: new `vscode/` folder — `package.json` (manifest: `name`, `publisher`, `engines.vscode`, `activationEvents`, `contributes.views` + `contributes.viewsContainers.activitybar`, `main`), TypeScript sources under `vscode/src/`, built with the existing node toolchain (esbuild or tsc — pick one, keep it minimal, no new heavy dependency). A `.vsix` is produced by a script (`vscode:package` via `vsce` or `npx @vscode/vsce`); building it is enough, publishing is ticket 6.
- **Daemon discovery**: read the port from `~/.swcode/settings.env` (`SWCODE_PORT`, default 4500) and the token from `~/.swcode/token`. Works on Linux/macOS/Windows (use `os.homedir()`, not `$HOME`). No pairing flow.
- **Side-bar webview**: a `webview` view (secondary side bar allowed) whose HTML loads ticket 1's entry — `…/vscode.html?daemon=http://127.0.0.1:<port>&token=<token>&theme=<light|dark>` — using the host contract from ticket 1 (AGENTS.md, "shared UI bundle entry"). The webview's `--vscode-*` colours map to `--jl-*` in `web/src/vscode.css`; pass `theme` from VS Code (`onDidChangeTheme` → update the webview).
- **Status bar**: an item showing the daemon state (connected / not running). If the port is closed, show "swcode: daemon not running" and keep the webview usable (it shows its own status alert). No daemon autostart.
- **Live conversation**: the webview's existing socket (`ws://…/api/socket?token=`) drives list, open, send, stream, stop, question/permission answers — all from ticket 1's bundle; the extension adds no new daemon calls.
- **Project**: the extension opens the session list as the app does; picking the project that matches the workspace folder is ticket 3's concern — do not block on it here.

**Acceptance criteria.**
- `npm run build` (repo root) still produces `dist/web` with `vscode.html`; the web app is unchanged in behaviour.
- The extension builds (its own build script, e.g. `npm --prefix vscode run build` or a root script) with no errors, and `npm run check` stays green (biome, `tsc -p web`, vitest, go). Keep the extension's TS out of the `web` tsconfig.
- Installed in VS Code (Run Extension / `.vsix`) against a running daemon, the side-bar panel shows the session list, opening a session renders its conversation and tool rows, sending a prompt streams the reply, stop works, and a question/permission can be answered.
- The status bar shows the daemon state; with the daemon stopped, the panel degrades gracefully (status alert, no crash).
- Report the exact layout (`vscode/` file list), the build/package commands, and the webview HTML (how it references the bundle: daemon-served URL vs. copied assets) — ticket 3 builds on it.

**Notes for Dev.** Follow the swcode-base rules (read before editing, business rules in services, small functions, types in `types/`). The Windows desktop app (`desktop/webview.go`) is the precedent for a second host of this bundle. Ticket 1's QA note: the two failing e2e `decision-map` tests are pre-existing — not your scope.
