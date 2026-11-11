# swcode 0.10.0

Released 2026-11-11.

swcode 0.10 runs as a service that starts with your session, opens a search result on the message it found, lets you pin the messages you come back to, and has the agent keep the project's todos. It is the first release since 0.7, so everything below is new.

## Highlights

- **A service, and an updater**: `swcode service install` registers the daemon to start when you log in, as a systemd user unit on Linux or a logon task on Windows; `swcode service uninstall` takes it out and keeps your settings and sessions. `swcode start` and `swcode stop` run the daemon in the background without a service, and `swcode --update` installs the latest release and restarts the daemon that runs.
- **A search lands on its message**: choosing a conversation result of the command palette opens the session on the message that says the words, scrolled to it and marked a moment — even when it sits many pages up in a long conversation. A tool call's line and a summarized stretch of the conversation are found the same way.
- **Pins**: hover a message and pin it. The side panel lists your pins by their first words, and a click goes back to that message — even one many pages up in a long session. The message's pin takes it off.
- **Todos the agent keeps**: ask the agent to plan the work and it opens todos on the project's Todo tab with `todo`, ticking them done with `closetodo` as it goes. They are the project's todos, so the next session starts from what is left.
- **Terminals**: a session page's side panel opens a shell in the project's folder on its host, and the agent starts and reads terminals of its own. A turn's restore point opens as diffs of each file it changed.

The [changelog](https://github.com/bioleyl/swcode/blob/main/CHANGELOG.md) has the full list.

## Upgrading

Nothing in your settings changes. If you started swcode by hand until now, `swcode service install` makes it start with your session. `swcode remote` is gone: projects on that host still open over SSH from the daemon on this computer. The macOS build is no longer published, and `install.sh` installs on Linux and WSL. swcode 0.7 offers this release itself. Otherwise, install or update with the command in the [README](https://github.com/bioleyl/swcode#install), or download the release for your platform. Checksums are published with the release.
