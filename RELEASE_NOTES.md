# swcode 0.11.0

Released 2026-11-13.

swcode 0.11 makes "run everything" safe to leave on — the agent's commands run in a sandbox, and a folder speaks for itself only once you trust it — and gives the agent MCP servers, agents of your own, git worktrees and a review pane. On a phone it installs as an app and notifies you when a session waits.

## Highlights

- **A sandbox, and trust**: on Linux hosts with bubblewrap (this computer, WSL, a server over SSH) the agent's commands write only in the project, its scratch folder and a `/tmp` of their own, and your keys (`~/.ssh`, cloud and git credentials) are not there. A command that must leave the sandbox asks first, even in a session that runs everything. A folder you add as a project is not trusted until you say so: its `AGENTS.md` and its skills are kept from the agent until then.
- **MCP servers**: list them in `~/.swcode/mcp.json`, in the shape their documentation gives, and the agent has their tools beside its own. Settings → MCP servers shows where each stands.
- **Git**: a session can work in a worktree of its own, on a branch of its own; a turn's restore point now covers what its commands changed; and a session's Changes button shows what is uncommitted and commits it.
- **Permissions that last**: answer a request "in this project" and every session of it goes by that; the project's Config tab lists the rules and takes them back.
- **Agents of your own**: a file in `.agents/agents/` names an agent, its instructions and its tools — writing and running commands included.
- **Without the app**: `swcode run "<prompt>"` starts a session from the command line and waits for its turn; a project starts sessions by itself on a schedule; and `npm run eval` scores a dozen tasks against your own llm-server.
- **Phones**: swcode installs as an app, one press has Tailscale serve it over HTTPS, a device is notified when a session waits for you, and a browser adds other computers' swcode by scanning their code.
- **Faster, and easier to read**: answers stream in smoothly, long conversations stay light, reads run together, a summary no longer processes the whole conversation again, commands show their output as it comes, and code blocks are highlighted with a copy button.
- **Passwords in a terminal**: a command that asks for one (`sudo`, ssh) runs in a terminal of the app, where you type it while the agent waits.

The [changelog](https://github.com/bioleyl/swcode/blob/main/CHANGELOG.md) has the full list.

## Upgrading

Three things behave differently:

- **Commands run in a sandbox** where the project's host can make one (Linux with bubblewrap: `sudo apt install bubblewrap`). A `git push` over ssh or a `gh` call fails inside it, and the agent then asks to run it outside. A project's Config tab turns the sandbox off, and `SWCODE_SANDBOX=off` turns it off everywhere. Windows hosts have none.
- **swcode listens on its own computer.** An install that already has paired phones or computers keeps listening on every interface; a new one reaches a phone through your tailnet (Settings → Phone) or with `SWCODE_HOST=0.0.0.0`.
- **A folder you add from now on asks for your trust.** The projects you already have stay trusted.

Notifications on a phone need swcode over HTTPS (a tailnet gives it) and, on iOS, the app added to the Home Screen first. Scanning a QR code from a browser needs one that reads them (Chrome on Android does; Safari does not: type the code). swcode 0.10 offers this release itself. Otherwise, install or update with the command in the [README](https://github.com/bioleyl/swcode#install), or download the release for your platform. Checksums are published with the release.
