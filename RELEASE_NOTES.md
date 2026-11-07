# swcode 0.4.0

Released 2026-11-07.

swcode 0.4 updates itself, asks you for the passwords a command needs, lets you dictate your messages, and holds on when llm-server is out of reach for a moment.

## Highlights

- **Updates**: swcode looks for a newer release every 10 minutes and, on a computer, asks once whether to install it. It checks the download against the release's checksums, puts it in place and restarts. Settings → Updates shows the version that runs and what the newer one brings, looks now, or stops looking.
- **Passwords**: a command that asks for a password (`sudo -A`, ssh, an ssh key's passphrase) shows a card in the session where you type it. The password goes to the command and is never kept.
- **Dictation**: the microphone in the composer writes what you say where the caret was, in your browser's language. It needs a browser that recognizes speech (Chrome, Edge, Safari) on a secure page: localhost, or https. Your browser's speech service hears what you say (Google's, in Chrome).
- **Steps per turn**: Settings sets how many times the main agent may act in one turn before it stops and reports what it did (`SWCODE_MAX_STEPS`, 150 by default, up to 1000).

## Fixes

- A turn no longer fails on a passing `502 Bad Gateway` from the proxy before llm-server, or a `503` while it loads its model: swcode sends the request again for about 15 seconds first.
- A daemon that is up but refuses a request says what it answered, instead of "The daemon does not answer"; a refused pairing code no longer says the daemon is not running.
- A session's delete button waits until the session and its subagents are done, since the daemon deletes none at work.
- The game button and the arcade keep going while subagents work after the turn has ended.

The [changelog](https://github.com/bioleyl/swcode/blob/main/CHANGELOG.md) lists the smaller fixes.

## Upgrading

Nothing in your settings changes. Install or update with the command in the [README](https://github.com/bioleyl/swcode#install), or download the release for your platform. Checksums are published with the release. From 0.4 on, swcode offers the next releases itself.
