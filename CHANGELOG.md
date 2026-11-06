# Changelog

All notable changes to swcode are documented here.

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

[0.2.0]: https://github.com/bioleyl/swcode/compare/v0.1.0...v0.2.0
