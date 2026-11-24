# Dev

Ticket 5 (refactor commands) implemented, scoped to the ticket. No new daemon API, no new dependencies, no new UI.

**Message shape (for ticket 6's docs).** The host→bundle prompt message gains one optional field: `{ type: 'prompt', text: string, includeProblems?: boolean }`. `includeProblems: true` sends the selection with the diagnostics block; absent (or a non-boolean) keeps the composer's toggle state. The context message is unchanged (`{ type: 'context', context }`); the webview page's forward script passes `event.data` whole, so it needed no change.

**Files changed.**
- `vscode/package.json`: four commands contributed (`swcode.refactor`, `swcode.explain`, `swcode.fixProblems`, `swcode.addTests`, category `swcode`) in the palette and in `editor/context` groups `swcode@2..@5`, after `swcode.ask` (`swcode@1`).
- `vscode/src/swcode.constants.ts`: the four ids, titles and prompt texts as named constants; `ASK_PROMPT` moved here from `extension.ts`.
- `vscode/src/extension.ts`: registers the four commands beside `swcode.ask`, each through the panel's `ask`; `fixProblems` asks with the problems included, the other three leave the toggle to the person.
- `vscode/src/session-panel.ts`, `vscode/src/context.ts`: `ask(text, includeProblems?)` forwards the flag; `HostOutgoing`'s prompt carries it.
- `web/src/services/host-messages.ts`: `HostMessage`'s prompt carries `includeProblems?`, parsed by `flagOf` (boolean, anything else → absent).
- `web/src/organisms/sc-composer.tsx`: `takeHostMessage` applies the flag before filling the field and sending; the shared composer gains nothing new.
- Tests: `vscode/tests/extension.test.ts` (new — the five commands registered, each posts its own prompt text and flag, and shows the panel), `vscode/tests/daemon.test.ts` (manifest carries the four commands in the palette and `editor/context` with groups `swcode@2..@5`), `vscode/tests/context.test.ts` (prompt with the flag), `web/tests/host-messages.test.ts` (flag true, false, absent).

**Prompt texts, verbatim:** "Refactor the selected code, keeping its behaviour." · "Explain what this code does." · "Fix the problems reported for this file." · "Add tests for the selected code." (`swcode.ask` still sends an empty text.)

**Verified.** `npm run build`, `npm run build:vscode`, `npm run typecheck` (web, e2e, vscode), `npm run lint` and `npm run check` (vitest: 43 files, 364 tests; go tests) all green — lint's 4 warnings are pre-existing in `web/src/pages/workflows-page.tsx` and `web/tests/ide-context.test.ts`. Headless probe of the ticket 1/2/4 recipe (built daemon + generated webview page + http-server + chromium): after a context push, `{type:'prompt', text:'Fix the problems reported for this file.', includeProblems:true}` lands in the composer and the daemon records it as the fenced block (`File:` / `Selection: 1:1-3:9` / `Selected code (typescript):` / `Problems:` with the eslint diagnostic) followed by the prompt; a prompt without the flag lands with the selection only. Screenshots in scratch (`probe/with-problems.png`, `probe/without-flag.png`). The web app's entry is unchanged when no host gives a context.

**Notes.** AGENTS.md got two notes: the ticket-5 commands/channel convention with the prompt texts, and two traps (the fake `vscode` module needs `ColorThemeKind`/`StatusBarAlignment`/`Disposable.from` and a structural provider type; the probe reads the daemon's `userMessage` events because the composer that shows the sent prompt is the session page's fresh instance). Flagged, not changed: the `.vsix` packaging and README section stay ticket 6's.

coding-style: clean
