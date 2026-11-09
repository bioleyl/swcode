# swcode: notes for working on the code

Read `README.md` first.

## Conventions

- The daemon is Go (`cmd/swcode`, `daemon/`, `desktop/`), the app TypeScript (`web/`).
- The protocol lives in `shared/protocol/` as zod schemas: the app parses with them, and `npm run protocol` writes
  the daemon's Go types from them (`daemon/protocol/protocol_gen.go`, never edited by hand; `check:go` fails while
  it is stale). A new session event or socket message needs a handler on both sides: the app's handler maps
  (`SessionEventHandlers`, `ServerHandlers`) and the daemon's generated mappers (`ClientMessageMapper`…, through
  `MapClientMessage`) do not compile until it has one.
- The daemon's model of a session is its event list. Do not keep conversation state elsewhere: the model's
  messages (`daemon/agent/conversation.go`) and the app's drawing (`sc-conversation`) are both rebuilt from events.
- Keep the system prompt and the start of the conversation stable: anything that changes them (a date, a counter)
  makes llm-server process the whole prompt again at every step.
- Frontend: built from jlib's components (`@syware/jlib`), written by their class in JSX, as SyWare's UX lays them
  out: `JlAppShell` and `JlSideNav` for the frame, a `JlPage` for each page, `JlPageOutlet` showing them. swcode's
  own components keep atomic design (`atoms/`, `molecules/`, `organisms/`, `templates/`, `pages/`), extend
  `ScElement` (jlib's `JlElement`), their CSS in a `.css` file beside it imported with `?inline`. Colours, radii,
  spaces and fonts are jlib's `--jl-*` tokens only. The logo blue (`--jl-brand`) never carries small text: use
  `--jl-accent`. An icon a jl-* component draws by name is registered (`registerIcons`, lucide's; the usual ones in
  `lib/icons.ts`).
- A session's history is paged from its end: `openSession` takes an `events` limit and `sessionHistory` answers with `keptBack` (events older than those shown); `olderEvents` asks for the page above. The conversation view (`sc-conversation`) keeps the scroll rules in `lib/history-window.ts` and asks for more at its top; the page never learns the whole list until it is all pulled.
- **A turn's restore point is captured by wrapping its `hosts.Host`** (`daemon/checkpoints`, `Recorder.Host()` in `runTurnIn`): writes through tool hosts are kept, bash's are not. The service makes the turn id (`runningTurn.id`, `agent.Setup.TurnID`), and `agent.RunTurn` records `checkpointSaved` before `turnFinished` through `TurnHost.TakeCheckpoint`, so the turn still ends on its own event.
- A paired computer's colour lives in `computers.json` and reaches the page as `Computer.color`; `sc-computer-switch` puts it on `<html>` as `data-computer-color`, and `global.css` turns that into the `--jl-accent*`, `--jl-on-accent` and `--jl-focus-ring` tokens (the light and dark values use `light-dark()`).
- A project's page is four tabs at `/projects/:id/<tab>` (`sessions`, `todos`, `config`, `skills`; the list in `web/src/lib/project-tabs.ts`): the address's `tab` is a route attribute like settings' `topic`, a picked tab only rewrites it with `history.replaceState`, and `/projects/:id` alone shows the sessions tab. A new tab needs its label/icon in `PROJECT_TAB_LABELS`, a `JlTabPanel`, and the e2e tests open it before touching its card.
- A project's todos live on `protocol.Project.todos` in `projects.json`, changed with `addProjectTodo` / `setProjectTodoText` / `setProjectTodoDone` / `removeProjectTodo` (the daemon sends the whole `projects` list back); they are not a file of their own. A new session's `createSession.pinnedTodos` (texts, not ids) are appended by `sessions.withPinnedTodos` after the first user message, and its `pinnedTodoIds` make `Projects.LinkTodos` set their `sessionId`, which the Todo tab links back to the session and `sc-session-todos` lists in it.
- The e2e tests are one file per user-facing area (`e2e/session.e2e.test.ts`, `composer`, `projects`, `settings`, `navigation`, `layout`); shared viewports, flows and page helpers (`start`, `settle`, `overflow`, `tab`, `navItem`…) live in `e2e/support.ts`, and every test file calls `afterEach(stopRigs)` or its rigs outlive the test.

## Gotchas met so far

- **`BASE_STYLES` defines `.label`** (the small capitals over a group): a component's own element named `label`
  gets that style. The button's text span is `.text` for that reason.
- **Inputs named like `HTMLElement` properties** (`role`, `title`, `hidden`…) clash with them: the message's
  author is `author`, not `role`.
- **Jadis JSX makes HTML elements only**: SVG is built with `createElementNS` (`lib/mark.ts`, lucide's
  `createElement` for icons).
- **`field-sizing: content` sizes the width too**: a textarea using it needs `width: 100%`, or its placeholder
  makes the page wider than a phone screen.
- **A new Playwright page is a new browser profile**: it has no access token. Open the link with `?token=` first,
  then navigate.
- **A subagent's own events never enter the session's list**: only `subagentStarted`, `subagentActivity` and
  `subagentFinished` do. Recording its `toolCall` or `assistantMessage` events would draw them as the main agent's
  and send them to the main model.
- **Biome's `useSortedKeys` reorders object keys**; the route table keeps its order with a `biome-ignore`.
- **jlib's page outlet keeps the page shown until the next one is ready**: both are in the document a moment, so
  the e2e `settle` waits for one page; Playwright's selectors reach into shadow roots, `jl-page-outlet > *` matches
  its insides too.
- To see the app without a window: `npm run test:e2e` (its global setup builds the Go daemon into `dist/e2e`, and
  `e2e/rig.ts` starts it, the fake llm-server and Chromium for each test). Send a no-op `page.evaluate('0')` after
  each click: headless clicks land on the next command. The e2e code is typed for Node only: run DOM code in the
  page as an expression string.
- **A jl-* component's events do not bubble** (`press`, `change`…): a listener on a list never hears the buttons or
  switches drawn in it. Make each with `createElement(JlButton, …)` and register its own event.
- **jlib's select takes an empty value for nothing chosen**: a real choice that means "the default" gets a value of
  its own on the page (`daemon-folder`, `default`), turned back into the empty setting when saved.
- **Class-form JSX takes no `attrs`** (`<JlField attrs={…}>` does not compile): make the element with
  `createElement(JlField, { attrs, props })` when it needs an attribute.
- **`jl-load-state`'s Try again runs the last `load()` again by itself**: a `retry` listener that loads again
  fetches twice.
- **A Windows process started from WSL ends with the WSL session that started it** (WSL keeps them in a job): the
  daemon's hidden `wsl.exe` is made through WMI (`Win32_Process.Create`) to outlive a shortcut's session. From the
  agent's shell, `Start-Process` on a shortcut or a new console does nothing: to test a double-click, have WMI
  start `explorer.exe "<the .lnk>"`.
- **The daemon stops when its terminal goes away** (a hangup, as when the SSH connection of `swcode remote`
  drops): whatever starts it detached from a terminal (`swcode daemon`) gives it a session of its own and sends its
  output to a file.
- **The tools never touch the disk themselves**: they go through the context's `Host` (`hosts.LocalHost`, or a
  `hosts.ShellHost` for WSL and SSH), and paths use the host's style (`Paths()`, POSIX or Windows).
- **The web tools run in the daemon, not on the project's host**: the context's `Web` (`daemon/web`). A page's text
  only ever goes to the reader model, which has no tools (`daemon/web/reader.go`): never hand page text to an agent
  that can run commands.
- **A `ShellHost` script never takes a value as it is**: paths go through `ShellWord`, anything else (a command, a
  pattern, bytes) in base64 decoded there. Each script runs in a subshell of the open bash, so its `exit` and `cd`
  stay there.
- **Ubuntu 26.04 has uutils coreutils, bfs and ugrep** in place of GNU's: try a `ShellHost` script on WSL before
  relying on a flag.
- **Windows has no process groups**: a command is stopped with `taskkill /T` (`daemon/hosts/run_windows.go`).
- **The settings file reads `\n` in double quotes as a line break** (`config.ParseEnv`, as Node's `parseEnv` did):
  it single-quotes values, so Windows paths keep their backslashes.
- **Go is `go.mod`'s `toolchain` (1.27.3)**: this machine's go (1.24.5, `/usr/local/go`) fetches it and runs it, so
  never set `GOTOOLCHAIN=local`, which then refuses to build. `gofmt` is not a go command and does not switch:
  run the toolchain's (`$(go env GOROOT)/bin/gofmt`, as `check:go` does). The latest of a dependency can want a
  newer Go still, and `go get` then raises `go.mod`'s lines: pin versions, and raise Go on purpose.
- **swcode-base is not in the program**: the repository publishes it (`plugins/swcode-base`, listed in
  `.claude-plugin/marketplace.json` for Claude Code's plugins and skills.sh), and the daemon has the skills CLI install
  and remove it on the project's host (`daemon/swcodebase`): `npx --yes skills@<pinned> add bioleyl/swcode --skill
  swcode-base --agent universal --yes`, `--global` for the home. `universal` is the CLI's agent whose folder is
  `.agents/skills` itself; any other also copies it into its own. A change to the skill reaches people from GitHub's
  main. A release raises `plugin.json`'s version with `package.json`'s (a test compares them): Claude Code updates a
  plugin only when its version changes. The tests put a fake `npx` first on the PATH, with a HOME of their own.
- **The desktop app runs the daemon inside it** (`desktop/`, Windows only, built from WSL): Win32 hands it structures
  as `uintptr`, hence `-unsafeptr=false` when `check:go` vets for Windows. Its page may only `postMessage` strings:
  go-webview2 ends the program on any other.
- **The page and the desktop app talk through WebView2's messages, JSON strings** (`shared/protocol/desktop.schema.ts`):
  go-webview2 sends each of the page's messages back to it, so the page tells the app's answers by their type. No
  token ever goes to the page. The e2e tests play the app with an init script (`fakeDesktopApp`).
- **A test copy of the desktop app rewrites the Start menu's shortcut to itself** (`keepShortcuts`), and Windows
  remembers its tray icon (`HKCU\Control Panel\NotifyIconSettings`): point the shortcut back after trying one.
- **The desktop app is in a Windows job that ends everything in it with the window**: a program meant to outlive it
  (the browser a link opens) is made by WMI (`startOutside`). Never let the job allow breakaway: Git Bash's runtime
  then starts every process out of it, and the agent's commands outlive a crash.
- To try the desktop app from WSL: build it (`npm run build:desktop`), copy `dist/swcode-desktop.exe` under
  `/mnt/c`, run it there with `--here`, settings passed with `WSLENV`: without `--here`, a copy outside
  `%LOCALAPPDATA%\Programs\swcode` installs itself over the installed app and starts that one.
- `/tmp` is a 16 GB tmpfs in RAM: a benchmark's workspaces and outputs go on the disk.
- **A new tool or system prompt line lengthens every request**: the compaction tests size their context to what
  the prompt and the tools take, and compact too early when those grow.
- **The skills are read on the project's host**, from `.agents/skills` in its home and in the project
  (`daemon/catalog/find.go`): the test app and the e2e rig give the daemon a home of their own, or the person's
  `~/.agents` enters the tests.
- **The skills and the project's notes (these `AGENTS.md` files) are given in the conversation, never in the system
  prompt** (`daemon/agent/skills.go`, `daemon/agent/notes.go`): two user messages in a row are joined into one
  (`ConversationOf`), since some chat templates want the roles to alternate.
- Commit only after `npm run check` passed: it stops at the first failing step.
- **A settings panel removed in a browser takes its ref with it**: `useRefs` resolves lazily and `getElement` throws once the element is gone, so `SettingsPage.renderTopic` touches `_refs.computers` only under `inDesktopApp()` (the panel is removed otherwise).
- **The e2e rig serves `dist/web`**: running `npx vitest --config vitest.e2e.config.ts` alone tests the last `npm run build`, so build after a web change (or use `npm run test:e2e`, which builds first).
- A new request from the page to the desktop app (`shared/protocol/desktop.schema.ts`) needs `npm run protocol`, then a method on `bridge` in `desktop/bridge.go`. That file is Windows-only, so plain `go vet ./...` on Linux skips it: use `GOOS=windows go vet -unsafeptr=false ./...` to compile it.
- **Restore points live in `<data>/checkpoints/<session>/<turn>`** (manifest plus blob copies), keep the newest 10 turns of a session, and go with the session. A restore writes the pre-images back and deletes files the turn created, checking each path through `Host.RealPath` so a link cannot lead outside the workspace.
- **jlib 0.13.0 keeps `jl-side-nav`'s `header` and `footer` slots in place** with `.sections` scrolling between them, so `sc-nav` no longer patches the side nav's shadow root for its layout (it still does for the + of a project row).
- **jlib 0.13.0 draws nothing when a list is given again as the same array object** (`sideNav.sections`, `select.options`, `segmented.options`, `menu.items`, `chipGroup.options`…): build a new array each time, as swcode's setters all do.
- **WSL appends Windows' PATH to the host's bash**, so `npx` there is `/mnt/c/Program Files/nodejs/npx` (Windows'), whose home is `C:\Users\<name>`: the skills CLI then installs swcode-base where the distribution never reads it, and a project install dies on `EPERM ... C:\Windows\.agents` while still exiting 0. `daemon/swcodebase` wraps the command for bash hosts in `cmd.exe` (`pushd` the share, `cd` to the folder, `set USERPROFILE=$HOME`) when `command -v npx` is under `/mnt/`; cmd takes a quoted UNC folder as invalid, so `pushd` gets the share root unquoted and `cd` the rest quoted.
- **The skills CLI 1.7.6 on Windows reports `Successfully removed` but leaves the skill's folder and lock in place, then dies with `Assertion failed: !(handle->flags & UV_HANDLE_CLOSING), file src\win\async.c`** (exit not 0, so swcode shows the failure). It does remove correctly on Linux. Reproduced on a plain Windows folder, so it is the CLI's, not the daemon's wrapper.
- **A `jl-switch`'s clickable track is at its label's left**: clicking the host's centre works in a narrow card but misses once the card is wide (a tab panel), so e2e clicks `getByRole('switch')` inside it.
- **jlib's page header drops the `actions` slot under the title until they fit**: `.main` wraps and `.titles` has a flex basis (275px since jlib 0.13.0), so at 390px two icon buttons still wrap (275 + 24 gap + 80 buttons > 328 of content); they share the row from ~380px of content width.
- **`useRefs` finds the first element for a class**: `ref(JlEmpty)` in the project page is the sessions' empty state, so the todos' empty state needs its own selector (`ref<JlEmpty>('.todos-empty')`). The same trap made the projects' skills page hold two `JlLoadState`s, each found by a class.
- **The same folder is `local` to one daemon and `wsl` to another**: a daemon running inside WSL (dev, `npm run dev`) and the Windows app's own daemon keep separate data, and the Windows one records a project inside a distribution as `{kind:'wsl', distro}`. So a side nav icon or a path that differs between dev and prod is usually the project's host, not a stale build: check it in the project's Config tab (`WSL · <distro> · /home/…`). A project's row icon comes from `HOST_ICONS` — a folder on this computer and in a WSL distribution, a server for SSH; `PROJECT_SHAPES` in `lib/session-lights.ts` keeps one lit shape per icon name, since two host kinds can share one.

## Commands

- `npm run check` needs both toolchains on PATH: nvm's node (`~/.nvm/versions/node/v26.12.0/bin`; the v26.7.0 path is gone) and the go.mod toolchain (`~/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.27.3.linux-amd64/bin`) — the machine's `go` (1.18) can't parse go.mod, and `GOTOOLCHAIN` with a path doesn't help. Without node's folder `npx` is not found.
- `npm run test:e2e`'s global setup runs plain `go build`: without the go.mod toolchain on PATH (`~/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.27.3.linux-amd64/bin`) it fails parsing go.mod (`invalid go version '1.27.0'`).
- A release is one commit titled `Release vX.Y.Z` that bumps `package.json`, `package-lock.json` and `plugins/swcode-base/.claude-plugin/plugin.json` together and writes `CHANGELOG.md` and `RELEASE_NOTES.md`; the tag is lightweight, and pushing it runs `.github/workflows/release.yml`, which stops unless `RELEASE_NOTES.md`'s first line is `# swcode X.Y.Z` and the tag is package.json's version.
