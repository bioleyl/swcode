# swcode 0.16.0

Released 2026-11-26.

The agent now has a browser. Six tools open a page, read its controls, click, type, take screenshots and read the console and network errors, in one Chromium per session. Reading pages is free. Clicking and typing ask unless the session runs everything, and password or payment fields, forms sent to another site and private-network addresses always ask. Settings has a new Browser topic: set **Agent browser** to **Visible window** to watch the agent's browser on the computer running swcode instead of running it headless.

In the session's side panel, a subagent's row now opens its details: the task it was given, its model, its state and every call it made. A terminal the agent opened leaves the panel once its command has finished and the turn ends, unless you typed in it or it still runs a server, and deleting a session closes its terminals.

Stopping a command, a job or a terminal now ends everything it started, Windows included. Workflow handoffs are no longer lost when events come in a burst, long streamed replies and busy terminals use far less CPU, and the daemon's job records stop growing over a long run.
