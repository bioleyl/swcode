# swcode 0.3.0

Released 2026-11-06.

swcode 0.3 reads skills where other coding agents do, lets each project choose its skills, and adds slash commands to the composer.

## Highlights

- **Skills in `.agents`**: swcode reads your `~/.agents/skills` and the project's `.agents/skills`, on the machine the project is on, as other agents that share the convention do.
- **Skills per project**: a project's page lists its skills, each with a switch for its sessions.
- **Slash commands**: type `/` in the composer for `/new`, `/compact`, `/peek`, `/help` and your skills. `/<skill>` runs a skill with your message.
- **Projects**: delete sessions from a project's page, set what its new sessions start with, and start one from the `+` beside the project in the navigation.

## Upgrading

swcode no longer keeps skills of its own, and no longer reads Claude Code's skills, commands or plugins. Skills in `~/.swcode/skills` are not read anymore: copy the ones you want to keep to `~/.agents/skills`, on each machine your projects are on (for a project in WSL, in WSL):

```bash
mkdir -p ~/.agents/skills && cp -Rn ~/.swcode/skills/* ~/.agents/skills/
```

Install or update with the command in the [README](https://github.com/bioleyl/swcode#install), or download the release for your platform. Checksums are published with the release.
