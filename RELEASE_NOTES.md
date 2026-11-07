# swcode 0.5.0

Released 2026-11-07.

swcode 0.5 comes with a skill of its own for writing code, starts each session from the project's notes, and has the agent keep those notes up to date as it works.

## Highlights

- **swcode-base**: a skill of coding rules that comes with swcode. Its last step has the agent check its own changes before it hands them over. A project's page installs it in the project, or in your home for every project, on the machine the project is on, and can take it out again. The app offers it once if your home doesn't have it.
- **The project's notes**: a session starts from the project's AGENTS.md files, from the repository's root down to the session's folder. They go to the model with the first message and again after a compaction, and the conversation shows them folded.
- **Notes the agent keeps**: a new notes tool adds what the agent found out to the project's AGENTS.md, each note at the end of its section, and creates the file if there is none. The agent uses it without being asked, and swcode-base also has it read the notes before exploring and write the first ones for a project that has none.
- **Saving the settings asks first**: saving the settings file restarts swcode, and the sessions open at that moment can break. Saving from the settings page or the SSH hosts now asks before it restarts. Run in a terminal, where you restart swcode yourself, it saves without asking.

## Fixes

- On a phone, the agent's question, permission and password sheets end above the tab bar, which used to cover their bottom.
- A session you saw on one device shows as seen on the others, and stays seen after an update. The daemon now keeps what you saw of each session, instead of each browser.

swcode is now under the MIT license. The [changelog](https://github.com/bioleyl/swcode/blob/main/CHANGELOG.md) has the full list.

## Known issue

Saving the settings file still restarts swcode, and the sessions open during the restart can break. For now, the new question only warns you before it happens.

## Upgrading

Nothing in your settings changes. swcode 0.4 offers this release itself. Otherwise, install or update with the command in the [README](https://github.com/bioleyl/swcode#install), or download the release for your platform. Checksums are published with the release.
