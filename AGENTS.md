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

## Commands

- `npm run check` needs both toolchains on PATH: nvm's node (`~/.nvm/versions/node/v26.7.0/bin`) and the go.mod toolchain (`~/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.27.3.linux-amd64/bin`) — the machine's `go` (1.18) can't parse go.mod, and `GOTOOLCHAIN` with a path doesn't help.
