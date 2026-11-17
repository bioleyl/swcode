# swcode 0.15.0

Released 2026-11-17.

**Workflows** are here: ask an agent for a team, or build one in the new Workflows page. Each role is a session of its own with its own preset (model pair), and passes its report to the next through a handoff file. Outputs carry the condition for taking them, so QA can send work back to Dev for a fix, Dev can return it to QA, and the PO can issue the next ticket — loops, with a Start that receives the brief and an End that finishes the run. A coding session starts one from the composer; its run session shows the graph, the step sessions and Stop. The project lists only the run, and deleting it removes its steps.

Settings now groups the models (llm-servers, image server, presets, thinking) and the network (MCP, SSH, config) in tabs, and a project’s config has Defaults, Permissions, Schedules and Notifications. The Android app gets its own Updates tab, keeps remotes only, and notifies through Android with a foreground service; notifications can be turned off per device and per project.
