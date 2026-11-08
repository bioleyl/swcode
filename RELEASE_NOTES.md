# swcode 0.6.0

Released 2026-11-08.

swcode 0.6 lets you set how far a subagent may go before it reports, and takes swcode-base out of the program: swcode's repository publishes it, so other coding agents can use it too.

## Highlights

- **A step limit for subagents**: the settings page sets how many times a subagent may act before it stops and reports what it found (`SWCODE_SUBAGENT_MAX_STEPS`, 30 by default, up to 1000). It used to be a fixed 30 whatever the settings said. "Max steps per turn" (`SWCODE_MAX_STEPS`) limits the main agent only.
- **swcode-base from the repository**: the skill is no longer inside the program. A project's page installs it and takes it out with the [skills.sh](https://skills.sh) CLI, on the machine the project is on, and a toast says why when the CLI fails. Other coding agents can install it too: Claude Code as a plugin (`/plugin marketplace add bioleyl/swcode`, then `/plugin install swcode-base@swcode`), any other with `npx skills add bioleyl/swcode`.

The [changelog](https://github.com/bioleyl/swcode/blob/main/CHANGELOG.md) has the full list.

## Known issue

Saving the settings file still restarts swcode, and the sessions open during the restart can break. For now, the question before saving only warns you before it happens.

## Upgrading

Nothing in your settings changes: subagents keep their limit of 30 until you set another. To install or remove swcode-base from a project's page, the machine the project is on needs Node.js 22.20 or later. swcode 0.5 offers this release itself. Otherwise, install or update with the command in the [README](https://github.com/bioleyl/swcode#install), or download the release for your platform. Checksums are published with the release.
