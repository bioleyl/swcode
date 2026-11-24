# PO

# PO — tickets 1 & 2 accepted, handing off ticket 3

**Brief, in my words.** A VS Code extension that is a *second, smaller UI* for the swcode daemon already running on the same machine. The developer gets swcode's agent inside the IDE (session list, conversation, composer) and the agent gets the IDE's intelligence (language-server diagnostics, active file + selection, refactor prompts). The UI reuses the web app's `sc-*` components through one shared bundle, so nothing is drawn twice, and the feature set stays deliberately smaller than the web app's.

**Backlog (ordered).**
1. Shared UI bundle entry — **accepted** (po-1 → dev-2 → qa-3).
2. Extension scaffold + live session panel — **accepted** (po-4 → dev-5 → qa-6).
3. **IDE context in the prompt** — *this ticket, full text below*.
4. IDE feedback (open a file/diff in the editor at the right line, follow the agent's edits).
5. Refactor commands (refactor / explain / fix problem / add tests as preset prompts).
6. Packaging and docs (`.vsix` wired into the repo's scripts, README section, shared-UI-entry tests).

po-1's assumptions and defaults still stand (side-bar webview, on-demand diagnostics, prompt-style refactorings, no daemon autostart, `vscode/` folder, `--jl-*` theming, no new daemon API). Nothing in qa-6 sends ticket 2 back; its notes (1 `retainContextWhenHidden`, 2 LICENSE in the .vsix, 3 TCP probe) are folded into tickets 3 and 6 below.

---

# TICKET 3 — IDE context in the prompt (diagnostics, selection, "Ask swcode")

**Goal.** A prompt sent from the panel carries the IDE's intelligence: the active file's path, its selection, and the language server's diagnostics — on demand, and from an editor command. The user never leaves the editor to give swcode the context of what they are looking at.

**Scope.**
- **Bundle side (web/, ticket 1's entry — reuse, do not fork):**
  - A host message channel: `web/src/services/host.ts` (or a small `host-messages.ts` beside it) listens for `window` `message` events and exposes the host's context to the pages. The webview's page (`vscode/src/lib/webview-html.ts`) gets the matching half: a tiny inline script that forwards `window.onmessage` into the iframe (`frame.contentWindow.postMessage`), so the extension's `webview.postMessage` reaches the bundle. Keep the frame's CSP as it is (the script is inline, `style-src 'unsafe-inline'` already there; add nothing new to the policy that breaks it).
  - `sc-composer` gets one host prop for it (a `context`-style property, like `compact`/`routes` on `sc-app-shell`): an "Include problems" affordance in its bar that, when on, prefixes the sent text with a context block. The app's own entry keeps working unchanged (the affordance stays hidden when the host gives no context).
  - The context block's shape (one fenced block, so `sc-message` renders it as code): the file path, the selection's line range, the selected code, then the diagnostics as `line:col severity message (source/code)`. Cap it (a sane max: e.g. 200 selection lines, 30 diagnostics) so a prompt stays light.
- **Extension side (vscode/):**
  - A `context.ts` service: reads `window.activeTextEditor` (document uri → path, `languageId`, selection range and text) and `languages.getDiagnostics(uri)`; refreshes on `window.onDidChangeTextEditorSelection` and `languages.onDidChangeDiagnostics` (debounced), and pushes the current context into the webview with `webview.postMessage` whenever it changes and when the panel is shown.
  - Commands: `swcode.ask` ("Ask swcode") in the editor context menu and the command palette, which opens/reveals the panel and sends a prompt with the selection + diagnostics (via the same context block). Register them in `contributes.commands` + `contributes.menus["editor/context"]`; keep the existing view contribution.
  - Fold qa-6 note 1 in here: set `retainContextWhenHidden` on the webview view so the context, the scroll position and a composer draft survive hiding the panel (the theme change may still redraw the frame; that is fine).
- **No new daemon calls**: `sendPrompt` (`{sessionId, text, pictures}`) is enough — the context rides in `text`.

**Acceptance criteria.**
- `npm run build` and `npm run build:vscode` succeed; `npm run check` stays green (biome, `tsc -p web|e2e|vscode`, vitest, go). Add vitest coverage for the new seams: the context block's formatting (a pure function, e.g. `web/src/lib/ide-context.ts`) and the host message parsing; the extension's context formatting/diagnostics mapping in `vscode/tests`.
- In VS Code against a running daemon: the panel shows "Include problems"; with it on, the next prompt arrives in the conversation as one message whose block shows the path, the selection and the diagnostics (verified in a headless browser against a real daemon, as ticket 2's recipe in AGENTS.md, plus the extension's own unit tests).
- "Ask swcode" from the editor context menu / palette reveals the panel and sends the prompt with the selection + diagnostics; with no file open it sends the prompt with no context block (no crash).
- The web app's entry is unchanged in behaviour (the affordance hidden without a host context).
- Report the exact message shape (extension → webview → bundle), the context block's format, the files added/changed in `web/` and `vscode/`, and the commands contributed — ticket 4 (open files/diffs in the editor) builds on it.

**Notes for Dev.** Follow the swcode-base rules (read before editing, business rules in services, small functions, types in `types/`). Keep the shared components shared: one new prop on `sc-composer`, no parallel component. The e2e suite runs the built bundle, so a web change needs `npm run build` before `npx vitest run --config vitest.e2e.config.ts`.
