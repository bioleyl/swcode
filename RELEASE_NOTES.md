# swcode 0.12.0

Released 2026-11-16.

swcode 0.12 lets you name the machines you reach with ssh, grants ssh by the machine it reaches, and closes the terminals the agent opened once their command ended. The Phone topic now fits the desktop app.

## Highlights

- **Named SSH hosts**: write `buildbox=me@build-box` in Settings → SSH, and the new project dialog offers that machine by its name. Each host you add there is changed or removed from its row, and the Hosts of `~/.ssh/config` are listed beside them.
- **An ssh grant is for the machine**: "Always allow ssh marvin" lets `ssh marvin` go without asking, whatever it runs there, in this session or in the project — and not another machine. A separator inside quotes no longer makes a command unknown, so `ssh marvin 'cd x && ls'` gets that grant.
- **Fewer terminals left behind**: a terminal the agent opened for a command you answer closes itself once the command ends, unless you typed in it. One started without `wait` stays for the agent to read later.
- **The desktop app's Phone topic**: no "Install on this device" or "Notifications on this device" there — the app is installed, and the tray notifies through Windows — and it says when the Tailscale it finds is inside WSL, which the Windows app cannot use.

The [changelog](https://github.com/bioleyl/swcode/blob/main/CHANGELOG.md) has the full list.

## Upgrading

Nothing changes for an existing install: the settings, the projects and the sessions carry over.

Install or update with the command in the [README](https://github.com/bioleyl/swcode#install), or download the release for your platform. Checksums are published with the release. swcode 0.11 offers this release itself.
