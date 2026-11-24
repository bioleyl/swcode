# Dev

## Dev — ticket 3 (IDE context in the prompt) implemented

**Message shape (extension → webview → bundle).** The extension calls `webview.postMessage(message)`; the webview's page has an inline script that sets `window.onmessage` and forwards `event.data` to the iframe with `frame.contentWindow.postMessage(event.data, '*')` (CSP unchanged). The bundle's `services/host-messages.ts` listens on `window` `message` and parses it (`hostMessageFrom`) into:
- `{ type: 'context', context: { path, language, selection: { startLine, startColumn, endLine, endColumn, text }, diagnostics: [{ line, column, severity, message, source, code }] } }`
- `{ type: 'prompt', text: string }`

Both are published on `hostMessageBus` (`{kind:'context'|'prompt', …}`); `vscode.tsx` starts the listener with `watchHostMessages()`. Lines/columns are 1-based; severity is the word `error|warning|info|hint`; absent `source`/`code` are empty strings. Ticket 4 can post the same two messages.

**Context block** (`web/src/lib/ide-context.ts`, `contextBlock`) — one fenced block, the fence grown past the longest backtick run in the code, capped at 200 selection lines and 30 diagnostics:
```
```
File: /home/dev/app/src/main.ts
Selection: 12:1-14:9
Selected code (typescript):
const a = 1;
…
Problems:
12:7 warning 'a' is never read. (eslint/no-unused-vars)
```
```
A caret renders as `Selection: 12:1`. The block is sent as one message: `sc-composer` prefixes it to the typed text (`{sessionId, text, pictures}`, no new daemon call).

**Files added.** `web/src/lib/ide-context.ts`, `web/src/types/ide-context.types.ts`, `web/src/services/host-messages.ts`, `web/tests/ide-context.test.ts` (7 tests), `web/tests/host-messages.test.ts` (4); `vscode/src/context.ts` (`currentContext`, `startContext` with a 150 ms debounce on `onDidChangeTextEditorSelection` + `languages.onDidChangeDiagnostics`, push on panel shown, `ask`), `vscode/tests/context.test.ts` (7).
**Files changed.** `web/src/organisms/sc-composer.tsx`/`.css` — one new host prop `hostContext` (+ internal `includeProblems`): a `.problems` jl-button (icon `warning`) appears only when the host gave a context, its label is the toggle ("Include problems" / "Problems included"); the app's entry keeps it hidden. `web/src/vscode.tsx` calls `watchHostMessages()`. `vscode/src/lib/webview-html.ts` — the forwarding script. `vscode/src/session-panel.ts` — `ask(text)` (posts the prompt, `view.show(true)`) and `retainContextWhenHidden: true` on the view contribution. `vscode/src/extension.ts` — registers `swcode.ask` via `commands.registerCommand`. `vscode/src/swcode.constants.ts` — `ASK_COMMAND`/`ASK_TITLE`, `CONTEXT_MESSAGE`, `PROMPT_MESSAGE`, `CONTEXT_REFRESH_MS`. `vscode/package.json` — `contributes.commands` `swcode.ask` ("Ask swcode", category swcode) + `contributes.menus["editor/context"]`, and `retainContextWhenHidden` on the view; the view contribution is kept. `package.json` — `jsdom` devDependency (the host-messages test needs a `window`).

**Verified.** `npm run build`, `npm run build:vscode` and `npm run check` (biome, `tsc -p web|e2e|vscode`, 40 vitest files, go) all green — the 4 biome warnings are pre-existing in `web/src/pages/workflows-page.tsx`. Headless browser against a real daemon (ticket 2's recipe: rig + `frameHtml` page + iframe): the composer's affordance is hidden, a posted `context` message makes it appear, clicking it toggles the label, and the prompt reaches the daemon as one message whose block carries the path, the selection, the code and the diagnostics. "Ask swcode" posts the same prompt with the current context and reveals the panel; with no file open `currentContext()` gives `undefined` and the prompt goes with no block.

**Notes.** AGENTS.md got the message channel, the block format and the `@types/vscode` gotchas (`WebviewOptions` has no `retainContextWhenHidden` → set it in the manifest; `commands.registerCommand`; `view.onDidChangeVisibility`). Flagged: the extension's `out/` is rebuilt by `npm run build:vscode`; nothing in ticket 3 needs a daemon change.

coding-style: 3 fixed, 1 flagged
