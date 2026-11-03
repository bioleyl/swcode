# swcode

A coding agent for local models. A daemon runs the agent and its tools next to your code and talks to
[llama-server](https://github.com/ggml-org/llama.cpp/tree/master/tools/server); a web app, on a computer or a
phone, follows what it does and answers its questions.

```
 web app (computer, phone) ──socket──▶ swcode daemon ──HTTP──▶ llama-server
                                       sessions, agent loop,      the model(s), on this
                                       tools, permissions         machine or another
```

The daemon is a Go program, the web app is in Jadis. It works on projects on this computer, in WSL or over SSH,
with one llama-server or several (several models, shared between people), and builds as one program: a command line
for Linux, macOS and WSL, and a desktop app for Windows with the daemon inside.

## Install

swcode needs a llama-server that does tool calls, on this machine or another one ([llama-server](#llama-server)
below). swcode itself is one program, downloaded from the [latest release](https://github.com/bioleyl/swcode/releases/latest).

### Windows: the desktop app

In PowerShell:

```powershell
irm https://raw.githubusercontent.com/bioleyl/swcode/main/install.ps1 | iex
```

or download `swcode-desktop.exe` from the latest release and open it. Either way it installs itself in
`%LOCALAPPDATA%\Programs\swcode`, makes its Start menu and desktop shortcuts and starts, then lives in the
notification area. To update it, quit it (Quit swcode, in the icon's menu) and do the same again. The program is not
signed: the first time, Windows may say it protected your PC (More info, then Run anyway). More in
[On Windows](#on-windows).

### Linux, WSL and macOS: the command line

```bash
curl -fsSL https://raw.githubusercontent.com/bioleyl/swcode/main/install.sh | sh
```

downloads the program built for this machine (Linux or macOS, x86-64 or ARM), checks it against the release's
checksums and installs it as `~/.local/bin/swcode`, keeping the `LLAMA_*` and `SWCODE_*` settings set in the
environment. In WSL it also adds a shortcut to Windows' Start menu, and asks (UAC) for the firewall rule phones need.
The same command updates it. Then:

```bash
swcode
```

starts the daemon in the background when it does not run, and opens the app in the browser.

### Built from the sources

On Linux or in WSL, with Go 1.27 (an older one, from 1.21, fetches the version `go.mod` names and runs it) and
Node 24:

```bash
git clone https://github.com/bioleyl/swcode.git && cd swcode
npm install
npm run build:desktop && npm run install:desktop   # Windows' desktop app, from WSL
npm run install:binary                             # the command line, for this machine
```

`install:desktop` refuses while the app is open: quit it first. [From the sources](#from-the-sources) below runs it
for development, and [The command line](#the-command-line) builds it for other machines.

### First start

swcode prints a link with its access token, which the desktop app opens by itself: open it once, the app keeps the
token. Then, in Settings → Models, add your llama-server (its address, its key when it has one) and the presets of
its models; [Settings](#settings) has the rest. A phone pairs from Phone, in the navigation
([On a phone](#on-a-phone)).

## A remote daemon

The daemon can run on another machine than the one you work from: a computer that stays on and works while yours
is off, or a machine you reach over SSH for a while. The tools run there, and the sessions are kept there.

### A computer that stays on

Install swcode there, then start its daemon:

- **Linux**: install it there with the install command above (`curl … | sh`, an ARM board too), then run `swcode`.
  It starts the daemon in the background and prints its link (on a machine without a desktop, no browser opens),
  and the daemon goes on once the SSH connection closes. To start it at boot, give it a systemd user service,
  `~/.config/systemd/user/swcode.service`:

  ```ini
  [Unit]
  Description=swcode
  After=network-online.target

  [Service]
  ExecStart=%h/.local/bin/swcode serve
  Restart=on-failure

  [Install]
  WantedBy=default.target
  ```

  then `systemctl --user enable --now swcode`, and `loginctl enable-linger` so that it starts before anyone logs
  in. New settings then apply with `systemctl --user restart swcode`.
- **Windows**: the desktop app, as above, and in its settings' Startup topic, **Without an icon** (`--daemon`): the
  daemon alone starts when you log in, without window or icon. The app opened while it runs shows it, and quitting
  the app leaves it running.

Its daemon listens on the network (`SWCODE_HOST`, every interface by default). Pair your computer with it: there,
`swcode pair` prints a code (`swcode-desktop.exe --pair` on Windows; its phone dialog shows one too). Then, in the
desktop app's settings, Computers: its address (`gmktec`, `192.168.1.20:4600`, `https://gmktec.tailnet.ts.net`) and
the code, which works once, for a minute; a link with that swcode's token needs no code. The app trades the code for
a token of its own, which that swcode lists with its paired phones, and keeps it in
`%APPDATA%\swcode\computers.json`; no page ever sees it. From a browser, open that swcode's own link instead.

The top of the side navigation says where swcode works: this computer, or the other one, tinted and marked Remote,
and the window's title and the icon's tip name it. It switches between them, and its last choice is where the window
opens next time. The app follows every computer's sessions for its notifications. In a browser, that place names the
machine the daemon runs on.

### Over SSH, for a while

```bash
swcode remote me@build-box -w ~/projects/app
```

copies swcode to `~/.swcode/bin` on that machine when it lacks it or has another build (for another platform, the
one built for it beside this one: `swcode-linux-arm64`…), and starts it there over SSH, with the folder to work in.
Its port comes back here (`-p`, 4510 by default) and llama-server goes there through the same connection, so the
other machine does not need to reach llama-server (`--llama-url` when it does). The tools run there, the sessions
are kept there, and Ctrl+C here stops it. `~/.ssh/config`, keys and the agent work as usual: it is plain `ssh`. The
llama-server key, when there is one, is on that machine's command line.

For a project on another machine without moving the daemon, the project dialog's **Where** offers the SSH endpoints
too ([Where projects are](#where-projects-are)): the daemon stays here and runs the tools there.

## llama-server

A llama-server that does tool calls (`--jinja`, on by default in recent builds). With one model, for example:

```bash
llama-server -m ~/models/gpt-oss-120b-mxfp4.gguf -c 131072 --host 0.0.0.0 --port 8080 --api-key "$LLAMA_API_KEY"
```

Without `--parallel`, llama-server opens 4 slots that share the `-c` tokens, each able to use all of them; with
`--parallel 4`, each slot gets a quarter. With `--host 0.0.0.0` llama-server is reachable from the network, hence
`--api-key`.

### A smaller model for the subagents

In router mode, llama-server serves several models, each with its own settings from a presets file (the keys are
its options without the dashes, the paths absolute):

```ini
version = 1

[qwen3.8-flash-next]
model = /home/syware/models/qwen38-flash-next/Qwen3.8-Flash-Next-GSQ-RCO-abliterated-IQ3_S-00001-of-00002.gguf
ctx-size = 131072
load-on-startup = true

[qwen3.5-9b-defiant]
model = /home/syware/models/qwen35-9b-defiant/Qwen3.5-9B-The-Defiant-Fable-Uncnr-Heretic-plusIQ-TOOLS-NEO-MAX-MTP-Q6_K.gguf
ctx-size = 131072
spec-type = draft-mtp
load-on-startup = true
```

```bash
llama-server --models-preset ~/models/presets.ini --models-max 2 --host 0.0.0.0 --port 8080
```

Then name both in a preset (below): the main agent on `qwen3.8-flash-next`, the workers on `qwen3.5-9b-defiant`. A
router wants each request to name its model and loads the one it is asked for; a preset naming no main model takes the
first loaded one that its workers do not use.

### Several llama-servers, and presets

Settings → Models lists the llama-servers swcode asks (a name, an address, a key, a share of the slots) and the
presets made of their models: each preset a name, the main agent's model and the workers' (subagents, and the reader
of web pages), on the same llama-server or on two. Each session picks its preset in its composer, beside its
permissions and its thinking, from its next request on; one preset is the default of new sessions. They are kept in
`~/.swcode/models.json` and in use as soon as they change, without a restart. Until they are saved once, they are the
`LLAMA_*` settings below: one llama-server, and a preset of its models.

What the small model buys, measured on the GMKtec: it reads prompts 2.7 times faster (957 against 353 tokens/s),
and reading files is most of an explore subagent's work. It does not write faster: on a unified-memory machine a
token costs a read of the weights it uses, all of them for a dense 9B, the active experts only for a mixture of
experts, so both write about 26 tokens/s. Its MTP heads (`spec-type = draft-mtp`) bring it to 34–42.

### Two people, one llama-server

Each person runs their own swcode, with a share of each model's slots: the first half (`1/2`) on one computer, the
second (`2/2`) on the other, set with the llama-server in Settings → Models (`LLAMA_SLOT_SHARE` until the models are
saved once). A daemon picks the slot of every request so that a conversation finds its prompt
cache again; two daemons on the same slots would undo each other's cache, and processing a long conversation again
takes minutes. With shares, one person's sessions never take the other's slots, even when those are idle.

Give each model fixed slots for that, `parallel` of them sharing `ctx-size` equally (without `parallel`, the slots
share one pool, and one long conversation can fill it for everyone). The GMKtec's file, for two people:

```ini
; 2 conversations each, of 196,608 tokens; an 8-bit KV cache: half the memory, the same speed here.
; In RAM: 4 snapshots of the recurrent state per slot (default 32), a 2 GiB prompt cache (default 8).
[qwen3.8-flash-next]
parallel = 4
ctx-size = 786432
cache-type-k = q8_0
cache-type-v = q8_0
ctx-checkpoints = 4
cache-ram = 2048

; 3 subagents each, of 65,536 tokens.
[qwen3.5-9b-defiant]
parallel = 6
ctx-size = 393216
cache-type-k = q8_0
cache-type-v = q8_0
ctx-checkpoints = 4
cache-ram = 1024
```

Both models keep a recurrent state beside the KV cache, and llama-server keeps snapshots of it for each slot in RAM
as a conversation grows (112 MiB each for the main model): with 32 of them per slot and 8 GiB of prompt cache per
model, the defaults no longer fit beside the models. swcode's prompts only grow at their end, so a few snapshots are
enough. Both models were trained on 262,144 tokens, the most a slot should hold.

### At boot

On the GMKtec, llama-server is a systemd user service, `~/.config/systemd/user/llama-server.service`, which runs the
router (`systemctl --user restart llama-server`, `journalctl --user -u llama-server -f`). `loginctl enable-linger`
starts it at boot, before anyone logs in. A user manager keeps the groups it started with: after adding the user to
`render` and `video`, restart it (`systemctl --user exit`, or reboot), or llama-server finds no GPU and runs on
the CPU, at half the speed.

## From the sources

```bash
npm install
npm run build
LLAMA_URL=http://gmktec:8080 LLAMA_API_KEY=… SWCODE_WORKSPACE=~/projects npm start
```

`npm start` runs the daemon from the sources (`go run ./cmd/swcode serve`), serving the app built in `dist/web`. It
prints a link with its access token: open it once, the app keeps the token.

For development, `npm run dev` starts the daemon (built again and restarted when a Go file changes) and Vite (hot
reload); the link then points at Vite, which forwards the API and the socket to the daemon.

### A release

Raise `version` in `package.json`, commit, then tag that commit with it and push the tag:

```bash
git tag v0.2.0 && git push origin v0.2.0
```

The Release workflow (`.github/workflows/release.yml`) checks that the tag is `package.json`'s version, runs
`npm run check`, builds the command line for Linux, macOS and Windows (x86-64 and ARM) and the desktop app, and
publishes them as a GitHub release with their `SHA256SUMS`: the install commands take the latest one. A copy of the
desktop app opened anywhere but `%LOCALAPPDATA%\Programs\swcode` installs itself there; `--here` runs a test copy
where it is.

## The command line

```bash
npm run build:binary
```

builds `dist/swcode-<os>-<arch>` (`swcode-linux-amd64`, as Go names platforms): one program with the web app and
the skills inside, nothing to install beside it. Go builds it for other platforms from here, named as arguments:
`npm run build:binary -- linux-arm64 darwin-arm64 windows-amd64`.

```bash
swcode            # opens the app in the browser, starting the daemon in the background when it does not run
swcode stop       # stops it
swcode pair       # a code to pair another computer's swcode, or a phone, with this one
swcode serve      # runs it in this terminal instead, until Ctrl+C
swcode install    # installs this binary: the swcode command, its settings, its shortcuts (Windows, WSL)
swcode remote me@box -w ~/app   # runs it on that machine over SSH (A remote daemon, above)
```

`swcode` again only opens the browser, and the daemon keeps running with no browser open. In the background it
writes its log to `~/.swcode/swcode.log`.

## On Windows

On Windows, swcode is one desktop app, one program: a window of its own with swcode's server inside, nothing else
running. It works on projects on Windows itself, in its WSL distributions, or on other machines over SSH. It
installs itself ([Install](#install)), or is built in WSL, in this repository:

```bash
npm run build:desktop && npm run install:desktop
```

`build:desktop` builds the app, `dist/swcode-desktop.exe`: its window (`desktop/`) on Edge's WebView2, which
Windows 11 has, with the daemon and the web app inside. Go builds it for Windows from WSL, and go-winres writes its
icon and manifest into it: no Windows needed. `install:desktop` puts it in `%LOCALAPPDATA%\Programs\swcode`, in
place of what was there (an older swcode daemon is stopped first). Opened anywhere else, the program copies itself
there first, then starts from there (`--here` runs a copy where it is, to try a build). Opened there, it makes its
Start menu and desktop shortcuts. The settings stay in `%USERPROFILE%\.swcode\settings.env`, with the
sessions; the settings page restarts the server on new ones, and the window opens it again.

The server writes its log to `~/.swcode/swcode.log`. The app runs once (opened again, its window comes forward),
keeps its window's size and place, and opens links that leave it in the browser. It lives in the notification
area, like a torrent client: closing the window hides it, the sessions working on, and Windows' notifications say
when a session needs you (an approval, a question), finished its turn, or failed, unless the window is in front on
that page; a click on one opens its session. The icon's menu opens the window, switches computers, and quits:
quitting stops the server, asking first while a session works or a command runs in the background
(`GET /api/closing`). The app is in a Windows job of its own, which ends every command the agent started when it
quits, even on a crash; a browser a link opens is started outside it, by WMI, and stays. Phones reach it while it
runs. Windows asks about the firewall the first time it listens on the network: allow private networks for phones.
Windows 11 puts a new icon behind the taskbar's ^ arrow: drag it next to the clock to keep it in sight.

The settings' Startup topic starts swcode when you log in (the `swcode` value of HKCU's `Run` key): in the
notification area (`--minimized`), or the daemon alone, without window or icon (`--daemon`), for a computer that
works for others. The app opened while the daemon alone runs shows it, and quitting the app leaves it running;
`swcode-desktop.exe --pair` shows a code to pair another computer with it.

On a Windows project, commands run with Git Bash when Git for Windows is installed, else Windows PowerShell 5.1,
and the system prompt says which: Git for Windows brings git and a bash the models know best.

The command line program also builds for Windows (`npm run build:binary -- windows-amd64`), for those who want the
daemon without a window: `swcode-windows-amd64.exe install` puts it on the PATH, and `swcode` opens it in an Edge
window.

### Where projects are

The project dialog's **Where** offers this computer, the WSL distributions Windows has (`wsl.exe -l`), and the SSH
endpoints: those of the settings (`SWCODE_SSH_ENDPOINTS`, by commas) and the `Host` aliases of `~/.ssh/config`. On
a WSL distribution or an SSH machine, the tools run there and the sessions stay with the daemon, which needs
nothing installed there but bash and the usual tools (stat, base64, find, grep with `-P`, setsid; git when the
project is a repository). It reaches that bash through `wsl.exe -d <distro>` (which wakes the distribution) or
`ssh` (batch mode: a key or the agent, no password), and keeps one open for the quick requests (a file, a listing,
a search), each in a subshell between two markers; a command runs in a process of its own, in a process group the
open shell can stop, and a background command keeps its output in `~/.swcode/jobs` there. Searches there use
grep's Perl expressions, close to JavaScript's. Linux machines only, for now.

### On another computer

Install it there as above, and add in Settings → Models that person's llama-servers, presets and share of the
slots. To have it work
for you while yours is off, see [A remote daemon](#a-remote-daemon).

### Inside WSL instead

swcode also runs in WSL, as `~/.local/bin/swcode` (`npm run install:binary`, which builds it and runs
`swcode install`), with a Windows shortcut. WSL stops a distribution once no `wsl.exe` uses it, so the daemon runs
in a hidden `wsl.exe` of its own, which Windows makes itself (WMI's `Win32_Process.Create`): one started from WSL
would end with the session that started it. WSL's own firewall (Hyper-V's) never asks: `swcode install` asks
instead, with a UAC prompt, for the rule `swcode (phones on the network)` on port 4500 from private networks.

## On a phone

On the same network, open Phone in the navigation: its QR code opens swcode on the phone, at
this computer's address on the network, with a pairing code. The code works once, for a minute, and the dialog
shows a new one every minute; the phone trades it for a token of its own (`POST /api/pairing`), kept in its
browser, which works like the access token until the dialog forgets that phone. The daemon keeps the paired phones
in `devices.json` of the data folder, their tokens as hashes only. Whoever sees the QR code, or types the address,
gets nothing once the minute is over or the code used. The daemon listens on every interface
(`SWCODE_HOST=127.0.0.1` keeps it to this computer).

To reach it from anywhere, put the daemon behind
[Tailscale Serve](https://tailscale.com/kb/1312/serve), which gives it an HTTPS address on your tailnet:

```bash
tailscale serve --bg 4500
```

Start the daemon with that address, `SWCODE_PUBLIC_URL=https://gmktec.your-tailnet.ts.net`, then open Phone in the
navigation: the QR code pairs the phone, as above.

## Settings

The settings page (Settings in the navigation) has a tab for each topic: the models, thinking, projects, pictures,
the network, the skills, and Claude Code's plugins. Thinking, projects, pictures and the network are kept in
`~/.swcode/settings.env` (`KEY=value` lines), which the page writes: saving restarts the daemon on them when it runs in the background (`swcode`) or in
the desktop app, and waits while a session works. The environment's own come on top, for one command; the page shows
those as set in the environment.

| Variable | Default | What it sets |
| --- | --- | --- |
| `LLAMA_URL` | `http://127.0.0.1:8080` | Where llama-server answers, until `models.json` is saved once (then the Models tab). |
| `LLAMA_MODEL` | none | The main agent's model. In router mode, without it: the first loaded one the subagents do not use. |
| `LLAMA_SUBAGENT_MODEL` | `LLAMA_MODEL` | The model explore subagents ask for. |
| `LLAMA_SLOT_SHARE` | `1/1` | This daemon's part of each model's slots, such as `2/2` for the second half, when people share a llama-server. |
| `LLAMA_API_KEY` | none | llama-server's `--api-key`. |
| `SWCODE_THINKING` | `full` | How much the main agent of a new session thinks: `full` (the model's default), `medium`, `low`, `off` (instruct). Each session changes it in its composer. |
| `SWCODE_SUBAGENT_THINKING` | `low` | How much subagents think. |
| `SWCODE_WORKSPACE` | the current folder | Where projects usually go: the project dialog starts there, and a session without a project works there. |
| `SWCODE_SSH_ENDPOINTS` | none | Machines projects can be on, reached over SSH: ssh destinations by commas (`me@box, gmktec`). The `Host` aliases of `~/.ssh/config` come too. |
| `SWCODE_IMAGE_URL` | none | A stable-diffusion.cpp `sd-server`, such as `http://garfield.home:8090`: the main agent makes pictures with it (`image`). Without one the tool is not offered. |
| `SWCODE_HOST`, `SWCODE_PORT` | `0.0.0.0`, `4500` | Where the daemon listens: every interface, so phones on the network reach it; `127.0.0.1` for this computer only. |
| `SWCODE_DATA_DIR` | `~/.swcode` | Where the sessions, the projects, the skills, the access token and `settings.env` are kept. |
| `SWCODE_TOKEN` | created once, kept in the data folder | The access token every API call and the socket need. |
| `SWCODE_PUBLIC_URL` | `http://host:port` | The address the printed link uses. |
| `SWCODE_WEB_DIR` | the app inside the program, else `dist/web` | The built web app. |

## How it works

- **The app** is built from jlib's components and works as SyWare's apps do: the navigation down the side (where
  swcode works, a new session, the projects with their sessions, what needs you counted beside each, the phone, the
  skills, the settings), folding into a rail; on a phone a top bar, the navigation in a drawer and tabs (New,
  Sessions, Settings). A project has its page, with its sessions; Sessions lists them all, what needs you first.
  Ctrl K (⌘K) searches the sessions, projects, settings and actions. A problem of the daemon shows over every page.
- **Sessions** are kept as two files in the data folder: `<id>.json` (title, workspace, permission mode) and
  `<id>.jsonl`, one event a line. The model's messages are rebuilt from the events at every step, appended only,
  and the system prompt never changes, so llama-server's prompt cache (`cache_prompt`) is reused from step to step.
  The model knows the date from the messages: each one goes to it with the local time it was sent
  (`[Sat 2026-10-24 23:05]`), which never changes afterwards.
- **Plan**: for a task of several steps, the main agent writes a plan with the `plan` tool (the steps, each
  pending, in progress or completed, as in Claude Code's task list) and writes it again as steps start and end.
  The app pins the latest plan above the composer, and the system prompt has the agent answer and stop once every
  step is completed, rather than look for more to do. When the model ends its turn with steps left (some announce
  their next step, then stop), the daemon reminds it of them and the turn goes on: only when it did something since
  the last reminder, three times a turn at most. A turn has 150 steps (a subagent 30): at the limit, the model is
  asked for its report (what it did, what worked, what to undo) instead of stopping in silence.
- **Questions**: the main agent's `question` tool asks you 1 to 3 questions, each with 2 to 4 answers to pick (one
  or several) or your own words, for decisions that are yours to make. The turn waits as for an approval: the
  session shows "Needs you", and the answers come back to the model as the result.
- **Skills**: the user's instructions for kinds of tasks, a folder each in `skills/` of the data folder, as Agent
  Skills (and Claude Code's) have them: a `SKILL.md` with a name and a description in its frontmatter, then the
  instructions, and the files they name. A new skills folder starts with the coding style and Jadis (the repository's
  `skills/`, inside the binary and the desktop app too); one that exists stays as it is. The model is told each
  skill's name and description in the conversation, not the system prompt: after the person's message when the skills
  changed since it last saw them, and again after a compaction, so the prompt llama-server cached stays the same. The
  main agent loads a skill with `skill` (its instructions, or a file of its folder) and saves one the user asks for
  with `saveskill`, which is an edit for the permissions; Skills in the navigation opens the settings' Skills, to read,
  write, rename and delete them. The daemon keeps them, so they are the same for projects on every host.
- **Claude Code's skills and plugins** come too, read where they are at each turn, on the machine the project is on
  (this computer, WSL, SSH), as Claude Code run there would find them; nothing is copied. The person's
  `~/.claude/skills` and `~/.claude/commands`; the project's `.claude/skills` and `.claude/commands`, in its folder
  and its parents up to the repository's root; and the plugins Claude Code installed
  (`~/.claude/plugins/installed_plugins.json`), each enabled or not as Claude Code's settings say (`enabledPlugins`
  of `~/.claude/settings.json`, then the project's `.claude/settings.json` and `settings.local.json`), their skills
  and commands named after them (`jadis:jadis`, `my-plugin:db:migrate`). The settings' Plugins topic turns each
  source and each plugin on or off for swcode (`claude.json` in the data folder), Claude Code's own settings left
  as they are, and shows what it finds on each machine. swcode's own skills come first: a Claude Code skill of the
  same name is left out. A loaded skill starts with its base directory, where its scripts are run from, and gets
  Claude Code's substitutions: the arguments the agent passes (`$ARGUMENTS`, `$0`, `$ARGUMENTS[1]`, the names of
  its `arguments`, or an `ARGUMENTS:` line at the end), `${CLAUDE_SKILL_DIR}`, `${CLAUDE_PLUGIN_ROOT}`,
  `${CLAUDE_PLUGIN_DATA}`, `${CLAUDE_PROJECT_DIR}` and `${CLAUDE_SESSION_ID}`. A skill with
  `disable-model-invocation` is a command for the person in Claude Code: the agent is not offered it. swcode runs
  none of a skill's `` !`command` `` lines before the model reads it, and says to run them with bash instead; a
  skill's `allowed-tools`, `model`, `context: fork` and `hooks`, and a plugin's agents, hooks and MCP servers, are
  left out.
- **Tools**: `read`, `write`, `edit` (exact text replacement), `bash`, `grep`, `glob`. An edit or a write needs a
  read of the file as it is now, and is refused, without asking anyone, when it cannot apply. The file tools stay
  in the workspace and the session's scratch folder (`scratch/<id>` in the data folder, deleted with the session,
  where the system prompt tells the agent to keep its experiments), symbolic links followed; in a git repository,
  `grep` and `glob` see what git shows, so the `.gitignore` files apply.
- **Background commands**: `bash` with `background` starts a command that does not end by itself (a dev server, a
  watcher) in its own process group (its own tree on Windows), on the session's host, and answers after its first
  seconds with what it printed and a job id. `output` reads what it printed since, `kill` stops it and what it
  started. The side panel lists the running ones, each with a stop button; they outlive the turn and stop with
  their session or the daemon (on WSL and SSH machines too, when the daemon goes). A plain `bash` call ends a second after
  its shell exits, so `server &` no longer holds it until the timeout.
- **The web**: `websearch` asks DuckDuckGo from the daemon, with nothing to set up and from any network, and lists
  its first results (title, address, snippet). It reads its pages without scripts: html.duckduckgo.com, or
  lite.duckduckgo.com when the first takes swcode for a robot (it turns away a few searches in a row) or has no
  results. The daemon makes one search at a time, 5 s apart, whichever session asks: spaced so, they all pass.
  `webfetch` downloads a page from the daemon, turns its HTML into markdown (the `main` or `article` part when the
  page marks one, without menus, scripts and pictures; the first 60,000 characters) and has the subagents' model
  answer the agent's question from it, with no tools: the page's text never enters the agent's context, and what the
  page says to do cannot be done. Explore subagents have both tools too. The system prompts call web content data,
  never instructions.
- **Pictures**: with `SWCODE_IMAGE_URL`, the main agent's `image` tool asks the picture server for a picture
  (`/v1/images/generations`), or for a change to pictures of the workspace (`/v1/images/edits`, the references in
  order), and saves it as a PNG where the agent says, through the host like any file. The daemon keeps a copy in
  `images/<session>` of the data folder, deleted with the session, and the conversation shows it in the tool's row
  (`/api/images`, behind the access token); a click opens it whole. The model itself never sees the picture.
- **Pasted pictures**: a picture pasted in the composer, or dropped on it, goes with the message (4 at most). The app
  makes it small first, 1280 px on its longest side, as a JPEG: a Qwen vision encoder takes a token for each 32 × 32
  pixels, so a screenshot costs about 900 tokens where a 4K one would take 8,000. The daemon keeps it in
  `images/<session>` beside the agent's pictures, and the session's events only name it, so its file stays small. The
  model sees it when llama-server loaded a vision projector for it (`mmproj` in the model's preset, `--mmproj`;
  `/props` then says `vision`): the picture goes after the message's text, read from the daemon's copy at each
  request, and the prompt cache keeps it processed from step to step, since the start of the conversation never
  changes. A model without one is told a picture is there, and the composer says so. A compaction's summary is text:
  the pictures go with the rest of what it replaces.
- **Games while it works**: while a turn runs, the composer's gamepad button opens three games in a dialog over the
  session, a tab each. **Drift** is a top-down space shooter (arrows or WASD, or a finger on a phone; the ship fires
  on its own). **Vim** teaches vim's motions a few at a time (h j k l, then words, line ends, f and t, counts, and
  gg G { } %): the cursor goes to each target in as few keys as the best way there, which shows once it is reached.
  **Keyboard** trains touch typing: words that read like English, their letter pairs learnt from this README's prose,
  made of six letters at first and one more once each is typed at the target speed, the slowest coming most often.
  Each game loads the first time its tab opens, in a chunk of its own, follows the theme through jlib's tokens, and
  keeps its scores and progress on the device, as it does the tab chosen last. The one shown pauses when the turn
  ends or asks you something, and says so, with the way back to the session.
- **Screenshots**: the main agent's `screenshot` tool draws a web page (a dev server it started in the background,
  or any address) at a desktop size, 1440 × 900, or a phone's, 390 × 844, the window or the whole page (4,000 px at
  most), after an optional text the page shows once ready. With a question, a request of its own shows the picture
  to the main model, which answers from it: only the answer enters the conversation. With `see`, the picture itself
  comes into the agent's context, after the results of its calls (about 1,300 tokens for the desktop size). The
  errors the page logged come with the result either way, and the conversation shows the picture, kept in
  `images/<session>` like the others. The daemon draws with `SWCODE_CHROMIUM`, else Playwright's headless
  shell (`npm run e2e:install`), else the Chrome, Chromium or Edge the computer has (Windows always has Edge). It is
  offered when there is a browser and the main model sees pictures.
- **Permissions**: new sessions run everything (`auto`): edits and commands go through without asking. In
  `acceptEdits` ("Ask before commands"), edits go through and commands ask; in `ask`, both ask. A command asks until
  a person allows its programs for the session; one with a substitution, a redirection or a background job always
  asks.
- **Context**: before each request the daemon estimates how full the context is, from the last reply's token
  count (llama-server's `usage`) plus what came since. Past 80% of a slot (`n_ctx` in `/props`), the model
  summarizes the conversation and goes on from the summary; when llama-server still finds the prompt too long, it
  compacts and tries once more. The app shows where that happened, with the summary.
- **Thinking**: what a reasoning model (gpt-oss, Qwen3) thinks before it answers streams into the conversation and
  folds away once the answer comes. It is kept with the reply and sent back with it (`reasoning_content`): Qwen's
  templates show past thoughts (preserved thinking), so the model goes on from its plan instead of working it out
  again at every step, and the prompt matches what llama-server cached while it wrote. How much it thinks goes
  to the chat template with each request (llama-server's `chat_template_kwargs`): `reasoning_effort` `medium` or
  `low`, which Qwen 3.8 and gpt-oss templates know, or `enable_thinking: false`, instruct for Qwen templates; `full`
  sends nothing. A template that adds an instruction for it at the start (DavidAU's do) makes the next step
  process the conversation again when a session changes it.
- **Subagents**: the main agent's `agent` tool starts an explore subagent with the read-only tools (read, grep,
  glob, websearch, webfetch) on `LLAMA_SUBAGENT_MODEL`. Several agent calls run one after another: on one GPU, subagents at the same
  time share its speed, and more of them than slots wipe each other's prompt cache. A subagent's conversation
  stays in memory; the session keeps a line for each of its calls, and its last answer is the call's result.
- **Slots**: llama-server answers as many requests at once as a model has slots (`total_slots` in `/props`). The
  daemon holds one slot per request, gives an agent back the slot it used last, where its conversation is still
  cached, and makes a request wait when every slot of its model is busy. The side panel shows how busy they are,
  and with `LLAMA_SLOT_SHARE`, which of them are this daemon's.
- **Projects** are folders on the daemon's computer, kept in `projects.json` in the data folder. "New project"
  browses the daemon's folders: use the one shown, or name a new folder to make inside it. Every folder a session
  works in has a project; one with sessions stays, and removing one never touches its folder.
- **Sessions** start from the home page: a project, and the first message. A message sent while a turn runs waits
  for the next turn; stopping the turn drops it. **Send now** hands the waiting messages to the model at the turn's
  next step instead, after the results of its current calls, without stopping the turn (as Claude Code steers). Sessions are renamed from their title and deleted from their
  header.
- **The socket** (`/api/socket`) carries the app's commands one way and every session event the other; the app
  draws the conversation from the same events the session file holds.

## Code

| Folder | What is there |
| --- | --- |
| `shared/` | The protocol's schemas (zod), which the app parses with and the daemon's Go types are generated from (`npm run protocol`, into `daemon/protocol/protocol_gen.go`); the command parser for permissions. |
| `skills/` | The skills a new skills folder starts with: the user's coding style and Jadis. |
| `cmd/swcode/` | The command line: `open`, `serve`, `daemon`, `stop`, `install`, `remote`. |
| `daemon/` | The daemon, in Go: `agent/` (the loop), `llm/` (llama-server client, slot scheduler), `tools/`, `hosts/` (this computer, WSL, SSH), `web/` (search, pages, the reader), `skills/`, `claude/` (Claude Code's skills and plugins), `permissions/`, `sessions/`, `projects/`, `app/` (the API and the socket), `serve/`, `local/` and `remote/` (what the command line does). Tests beside the code; `fakellama/` plays a scripted llama-server, `testapp/` runs a daemon for them. |
| `desktop/` | The Windows app: its window on WebView2 with the daemon inside, its icon and notifications, the daemon alone (`--daemon`); `watch/` follows a daemon's sessions for the notifications, `computers/` keeps the other computers and pairs with them. |
| `web/src/` | The app, built from jlib's components (`@syware/jlib`, on Jadis) as SyWare's apps are, and swcode's own: `atoms/`, `molecules/`, `organisms/`, `templates/`, `pages/`, `services/`, `lib/`. |
| `web/tests/`, `shared/tests/` | `npm test`. |
| `e2e/` | `npm run test:e2e`: the built app in a browser, against the Go daemon; `fake-llama.ts` plays llama-server. |

`npm run check` runs the lint, the type checks, the tests, and the Go checks (the generated protocol up to date,
gofmt, go vet for Linux and Windows, go test). `npm run test:e2e` builds the app and drives it in
headless Chromium, on a computer's and a phone's screen, against the daemon and the fake llama-server: install
the browser once with `npm run e2e:install`, or point `SWCODE_CHROMIUM` at a Chromium already there.
