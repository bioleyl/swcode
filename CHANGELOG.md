# Changelog

All notable changes to swcode are documented here.

## [Unreleased]

### Maintenance

- The release workflow publishes `RELEASE_NOTES.md` as the release's notes, which the update offer shows, and stops when they are not the tagged version's.

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

[Unreleased]: https://github.com/bioleyl/swcode/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/bioleyl/swcode/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/bioleyl/swcode/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/bioleyl/swcode/compare/v0.1.0...v0.2.0
