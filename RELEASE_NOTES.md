# swcode 0.8.0

Released 2026-11-10.

swcode 0.8 opens terminals in the app, which the agent can start and read, and shows what a turn changed before you restore it.

## Highlights

- **Terminals**: a session's side panel lists the project's terminals. Open one and a shell runs in the project's folder, on its host — this computer, a WSL distribution or an SSH endpoint — in a dialog you can type in, rename from the panel and close. It keeps its scrollback for the next visit, and follows the page's light or dark theme.
- **The agent and the terminals**: ask it to run the dev server and it opens a terminal titled `dev server`; tell it a build failed in a terminal you ran and it reads that terminal's last lines with `readterminal`. Anything it starts still asks for approval like any command.
- **The changes of a turn**: the line saying "3 files saved before this turn" has a Changes button. It shows each file as it was before the turn and as it is now, a diff per file, before you decide to restore.

The [changelog](https://github.com/bioleyl/swcode/blob/main/CHANGELOG.md) has the full list.

## Upgrading

Nothing in your settings changes. A terminal runs while the daemon does: restarting swcode closes the shells it opened, and the restore points of the last turns are kept as before. swcode 0.7 offers this release itself. Otherwise, install or update with the command in the [README](https://github.com/bioleyl/swcode#install), or download the release for your platform. Checksums are published with the release.
