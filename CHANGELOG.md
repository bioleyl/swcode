# Changelog

All notable changes to swcode are documented here.

## [0.11.0] - 2026-11-13

### Added

- MCP servers: the agent of a coding session has the tools of the servers listed in `~/.swcode/mcp.json` (a program over its standard streams, or an address over HTTP), and asks before it calls one. Settings → MCP servers lists them with where each stands.
- A sandbox for the agent's commands on Linux hosts (this computer, WSL, a server over SSH) that have bubblewrap: they write only in the project, its scratch folder and a `/tmp` of their own, and your keys are not there. A command that must leave it asks first, even in a session that runs everything; a project's Config tab turns it off, and `SWCODE_SANDBOX=off` turns it off everywhere.
- Trust: a folder added as a project speaks for itself only once you trust it. Until then its `AGENTS.md` and its own skills are kept from the agent, and its commands keep their sandbox.
- A permission request can be answered for the whole project: its Config tab lists the programs and MCP servers its sessions use without asking, each taken back with a press.
- A restore point also keeps what the turn's commands changed, in a git repository: its diff and its restore cover them as they cover the files the tools wrote.
- A session can work in a git worktree of its own, on a branch of its own beside the repository.
- A session's Changes button shows what its folder has uncommitted, file by file, and commits it with your message.
- Agents of your own: a file in `.agents/agents/` names an agent, its instructions and its tools, writing and running commands included. It works under the session's permissions.
- `swcode run "<prompt>"` starts a session from the command line and waits for its turn, with `--json` for scripts.
- A project starts a session on a prompt by itself, every hour, day or week, from its Config tab.
- An eval suite: a dozen small tasks in `evals/`, scored on success, steps, tokens and time against your own llm-server (`npm run eval`).
- A project runs a command after each file the agent writes or edits (a formatter, a linter, a type check), and the agent reads what it found.
- On a phone: swcode installs as an app, one press has Tailscale serve it over HTTPS on your tailnet, a device is notified when a session waits for you or ends its turn (Web Push), and a browser adds other computers' swcode by their pairing link or QR code, as the desktop app does.
- What a command prints shows while it runs, and the side panel says how fast the model answered and how much of the prompt llm-server had kept.
- Code blocks are highlighted and copied with a press, and Markdown draws italics, strikethrough, quotes, rules and nested lists.

### Changed

- A command that asks for a password (`sudo`, ssh) runs in a terminal of the app, where you type it while the agent waits for the result, instead of a card under the call.
- swcode listens on its own computer (`127.0.0.1`) unless told otherwise. An install that already has paired phones or computers keeps listening on every interface.
- `edit` finds its text by its words when only the indentation differs, and names the nearest lines when it finds nothing.
- The calls of one reply that only read run together.
- A summary is asked at the end of the conversation llm-server already holds, so compacting a long session no longer processes it all again.
- What you were typing in a composer is still there when you come back to the page.
- The web app is lighter: an answer streaming in is drawn once a frame, a long conversation lays out only the rows near the window, the terminal's code loads when a terminal opens, the files travel compressed and stay in the browser's cache, and a session's stream goes only to the pages that show it.

### Fixed

- A session opened again showed the wrong older pages above the end of a long conversation; a page that comes back online now goes on from its last event instead of drawing the conversation again.
- A call's row kept "running" when an older page brought its events after its result.
- Calls whose ids a server repeats from one reply to the next (call-0, call-1…) each keep their own result, for the model and on the page.
- Notes longer than a session reads lost the ones just written; they now lose their oldest.
- Forgetting a phone ends the sockets it had open.
- A project the daemon refuses says why in its dialog, which stayed waiting.
- A todo being edited keeps what was typed when the lists change.

### Security

- The data folder and its files are the person's alone, and the page carries a content security policy.
- A session grant by program name no longer vouches for a command that sets a variable first, runs its words (`sudo`, `env`, `xargs`, `npx`…) or names a program for git or npm to run.
- The file tools stay out of `.git`, and the daemon's own git runs nothing a repository names.
- The web tools read the public web only, redirects included; a notes file that is a link is not read; a background job is its session's.
- Another swcode's page talks to this one only once it paired from there.

### Maintenance

- Pushes to main and pull requests run the check and the app end to end.
- The end-to-end tests give a busy machine the time it needs: a poll waits up to 15 seconds instead of one.

## [0.10.0] - 2026-11-11

### Changed

- A search result in a conversation opens the session on the message that says the words, scrolled to it and marked a moment, instead of at the end of the conversation. It finds a tool call's line and a summarized stretch of the conversation too, even when the message sits many pages up in a long session.

### Maintenance

- jlib 0.14.0: the side nav, the tables, the dialogs and the palette these views are built from keep their inputs, events and methods. Its marked table rows now tell a screen reader with `aria-selected`, and its skeleton rows are hidden from it.

## [0.9.0] - 2026-11-11

### Added

- Pins: hover a message of a session and pin it, and the side panel lists it by its first words. A pin opens the conversation back at that message, even when it sits many pages up, and the message's own pin takes it off again.
- The agent opens and closes the project's todos with `todo` and `closetodo`: ask it to plan the work and the project's Todo tab lists what it wrote, ticked as it goes.
- The command line runs swcode as a service: `swcode service install` registers the daemon to start when you log in — a systemd user unit on Linux, a logon task on Windows — and `swcode service uninstall` takes it out, keeping settings and sessions. `swcode start` and `swcode stop` run the daemon in the background without a service, and `swcode --update` installs the latest release and restarts the running daemon.

### Removed

- `swcode remote`, which ran the daemon on an SSH host for a while: projects on that host still open over SSH from the daemon on this computer.
- The macOS build, which is no longer published: `install.sh` installs on Linux and WSL.

### Fixed

- A project is its host and folder: a WSL or SSH project and a local one at the same path no longer list each other's sessions, in the project's page, the removal dialog or the new-session pick.
- A session opened while it works is never blank: the conversation measures what it drew and asks for older events until they fill the window.
- The desktop app asks a computer's swcode before switching to it, and stays on the one shown, saying which computer did not answer, instead of replacing the app with Edge's own error page.

### Maintenance

- The compaction tests' context sizes and file follow the longer prompt and tool list: the turn compacts after the read, as intended, instead of before it.

## [0.8.0] - 2026-11-10

### Added

- Terminals: a session page's side panel lists the project's terminals, and opens a shell in the project's folder on its host — this computer, a WSL distribution or an SSH endpoint — on a pseudo-terminal of its own, drawn in a dialog with xterm.js. Type in it, rename it in the panel, close it; the scrollback comes back when you open it again, and light and dark follow the page.
- The agent starts a terminal of its own with `terminal { command, title }`, which the person watches beside the session, and reads what any terminal of the workspace printed with `readterminal { terminal, lines }`: the last lines of a build's error, one the person ran included.
- A turn's restore point opens as diffs: "Changes", beside "Restore" on the line saying how many files were saved, shows each file as it was before the turn and as it is now, one dialog with a diff per file.

### Changed

- The system prompt sends a server or a watcher the person should watch to `terminal` with a short title, and a command whose output only the agent needs to `bash` with `background`.

### Maintenance

- Terminals run on `github.com/aymanbagabas/go-pty` (ConPTY on Windows), drawn by `@xterm/xterm`.
- The compaction tests' context sizes follow the longer prompt and tool list.

## [0.7.0] - 2026-11-09

### Added

- A project keeps its todos on its page's Todos tab: add what needs doing, tick it done, change its words or delete it, with a confirmation for the delete. A todo a session started on links back to that session.
- A new session's page lists the project's open todos, which you tick to pin them to its first message. The daemon appends the pinned todos to that message, so the agent reads the request first, and the session lists the todos it started on.
- The composer's `/todo` adds a todo to the folder the session works in: `/todo Write the tests`.

### Changed

- A project inside a WSL distribution shows the folder icon, as a project on this computer does: it is a folder too. Its status light fills that same shape.

### Maintenance

- jlib 0.13.0 keeps the side nav's header and foot in place with the sections scrolling between them, so `sc-nav` no longer patches the side nav's shadow root for its layout.
- The end-to-end tests split by area (`session`, `composer`, `projects`, `settings`, `navigation`, `layout`), with the viewports, flows and page helpers they share in `e2e/support.ts`.

## [0.6.0] - 2026-11-08

### Added

- Subagents have a step limit of their own on the settings page: how many times a subagent may act before it stops and reports what it found (`SWCODE_SUBAGENT_MAX_STEPS`, 30 by default, up to 1000). It used to be a fixed 30 whatever the settings said; `SWCODE_MAX_STEPS` limits the main agent only.

### Changed

- swcode-base is no longer inside the program: swcode's repository publishes it, as a Claude Code plugin (`/plugin marketplace add bioleyl/swcode`, then `/plugin install swcode-base@swcode`) and for skills.sh (`npx skills add bioleyl/swcode`). A project's page installs it and takes it out with the skills CLI, on the project's host, which then needs Node.js 22.20 or later; why the CLI failed shows in a toast.

### Maintenance

- The keyboard help's list drops its workaround: jlib 0.12's dialog body holds the keycaps' spoken words. An e2e test checks that only the dialog's body scrolls in a short window.
- The skills for working on swcode come from skills.sh, locked in `skills-lock.json`.

## [0.5.0] - 2026-11-07

### Added

- swcode comes with a skill of its own, swcode-base: coding rules, with a check of the agent's own changes before it hands them over. A project's page installs it in the project or in your home for all projects, on the project's own host, and takes it out again; the app offers it once when your home has none.
- A session starts from the project's notes: the AGENTS.md files from the repository's root down to the session's folder go to the model with its first message, and again after a compaction. The conversation shows them folded.
- swcode-base has the agent keep the project's AGENTS.md without being asked: it goes by the notes before exploring, writes the first ones of a project that has none, and adds what each task found out.
- A notes tool: the agent adds what it found out to the project's AGENTS.md in one call, each note at the end of its section, and makes the file when there is none. Its description asks for it unasked, so the agent keeps the notes even without swcode-base.

### Changed

- Saving the settings file, from the settings page or the SSH hosts, asks first where swcode restarts by itself on it: the sessions open at that moment can break with the restart. Run in a terminal, where you restart it yourself, swcode saves without asking.

### Fixed

- On a phone, the agent's question, permission and password sheets end above the tab bar, which used to cover their bottom (the question's own answer field among it).
- A session seen on one device shows seen on the others, and stays seen after an update: the daemon keeps what you saw of each session, instead of each browser, where a phone never knew what the computer saw and a lost or empty record turned every session green again.

### Maintenance

- The release workflow publishes `RELEASE_NOTES.md` as the release's notes, which the update offer shows, and stops when they are not the tagged version's.
- The README is a shorter tour of swcode, with screenshots of the app, and swcode is under the MIT license.
- The notes for coding agents working on swcode are in AGENTS.md, which swcode's own agent reads too; CLAUDE.md imports them.

## [0.4.0] - 2026-11-07

### Added

- A command on this computer that stops to ask for a password (`sudo -A`, ssh, an ssh key's passphrase) shows a card where you type it, and waits; the password is never kept in the session.
- The settings page sets how many times the main agent may act in one turn before it stops and reports what it did (`SWCODE_MAX_STEPS`, 150 by default, up to 1000).
- swcode updates itself: it looks for a newer release on GitHub every 10 minutes, asks once on a computer (never on a phone) whether to install it, then puts the checked download in place of the program and restarts on it. Settings → Updates shows the version that runs and what the newer one brings, looks now, and turns the looking off.
- A microphone in the composer dictates the message, through the browser's speech recognition and in the browser's language: the words go where the caret was as they are heard, until you press it again or stop speaking. It shows where the browser recognizes speech (Chrome, Edge, Safari) on a secure page (localhost or https), not on a phone reaching swcode over the network's plain http.

### Fixed

- The game button and the arcade keep going while subagents are still at work after the main agent's turn has ended.
- A turn no longer fails at once when llm-server is out of reach for a moment (502 Bad Gateway from the proxy before it, 503 while it loads its model): the request goes again for about 15 seconds first.
- A daemon that is up but refuses a request, or answers something other than its status, no longer shows as one that does not answer: the page says what it answered and points to its log.
- A pairing code the daemon refuses no longer says the daemon is not running: the phone topic shows what the daemon said.
- A pasted picture the browser cannot draw says which step failed.
- Two pages asking `/peek` at the same moment no longer risk getting each other's answer: each peek has an id of its own.
- A session's own page holds its delete button while the session works, its subagents included, as the project page's list did: the daemon refuses to delete a session at work, and the page used to leave as if it had gone.

### Maintenance

- Permissions and the agent's questions share how a turn waits for you, and sessions have tests of their own.
- `npm run check` runs the Go tests with the race detector.
- The project dialog, the SSH hosts and the phone topic get the daemon's answers through one service instead of each reading its broadcasts, and the side nav's fold is kept by a service like the other preferences.

## [0.3.0] - 2026-11-06

### Added

- Slash commands in the composer: `/new` starts a session with the same settings, `/compact` summarizes the conversation now, `/help` lists the commands, skills and keys, and `/peek` joins the same menu.
- Skills as slash commands: `/<skill>` loads the skill with your message, in a session or a new session's composer.
- A project's page lists the skills its sessions find, each with a switch to turn it off for them.
- A project's page deletes its sessions and sets what its new sessions start with: preset, permissions, thinking and token economy.
- A `+` beside each project in the navigation starts a session there.

### Changed

- Skills are read from the shared `.agents` folders, on the machine the project is on: `~/.agents/skills`, then the project's `.agents/skills` up to the repository's root.
- The agent writes a skill you ask for in the project's `.agents/skills`.

### Removed

- swcode's own skills: the `skills/` folder of the data folder, the Skills settings tab, the `saveskill` tool, and the coding-style and Jadis skills new installs started with.
- Claude Code compatibility: skills, commands and plugins from `~/.claude` and the project's `.claude`, the Plugins settings tab, and Claude Code's skill extras (`$ARGUMENTS`, `${CLAUDE_*}`, `disable-model-invocation`, `` !`command` `` lines).

### Maintenance

- Updated jlib to 0.11.1.

## [0.2.0] - 2026-11-06

### Added

- Chat mode for conversations that can read the web and draw images.
- OpenAI Responses API support, alongside other compatible llm-servers.
- Background subagents with configurable concurrency and models.
- `/peek` for asking an agent a question without changing its conversation.
- Ctrl/Cmd+K search across sessions, projects, settings and conversation text.
- SSH project hosts in their own settings tab.
- Per-model context sizes and token-economy defaults for new sessions.
- Session and project status indicators for working, waiting and unseen completion states.
- Side-panel llm-server status information.

### Changed

- Presets now configure subagent models, context sizes and concurrent subagents.
- The app consistently calls compatible backends llm-servers.
- Navigation keeps projects accessible while moving its actions into a compact footer.
- Phone pairing is available from Settings.

### Maintenance

- Updated release workflow actions for Node 24.
- Added protocol validation for numeric bounds and optional numbers.

[0.7.0]: https://github.com/bioleyl/swcode/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/bioleyl/swcode/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/bioleyl/swcode/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/bioleyl/swcode/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/bioleyl/swcode/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/bioleyl/swcode/compare/v0.1.0...v0.2.0
