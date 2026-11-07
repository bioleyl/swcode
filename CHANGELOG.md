# Changelog

All notable changes to swcode are documented here.

## [Unreleased]

### Added

- A command on this computer that stops to ask for a password (`sudo -A`, ssh, an ssh key's passphrase) shows a card where you type it, and waits; the password is never kept in the session.

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

[0.3.0]: https://github.com/bioleyl/swcode/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/bioleyl/swcode/compare/v0.1.0...v0.2.0
