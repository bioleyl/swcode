# The project's notes: AGENTS.md

A project's AGENTS.md is what the agent knows of it before reading any code. swcode gives it to the model with the first message of each session, and again after a compaction; other coding agents read it too. What is written there is not found out again. What is not is found out again by every session, at the cost of its time and its context.

Keeping it is part of every task, done without being asked: the next session should start where this one ended.

## Use it

- At the start of a task, go by the notes before exploring: the commands, the layout and the traps they give are already checked. In swcode they come with the first message ("The project's notes (AGENTS.md)…"); elsewhere, read the AGENTS.md at the project's root.
- No notes came, and the root has no AGENTS.md: the project has none yet. Write the first ones at the end of this task (below).
- A note turns out wrong while you work: fix it then, before you forget why.

## Where

- The AGENTS.md at the project's root, beside its README. A part of a large repository with rules of its own may have one in its folder: swcode reads each one from the repository's root down to the folder the session works in, the root's first.

## First notes

At the end of the first task in a project without notes, create its AGENTS.md, unasked. Write it from what the task already taught you, and from a quick read of what defines how the project is worked on:

- **What it is and where its parts are**, in a line or two, when the folder names don't say it (`daemon/` is the Go server, `web/` the app).
- **Commands**: from `package.json`'s scripts, a `Makefile`, `justfile` or `Taskfile`, the CI workflow, the README: how to install, build, test, lint and run it, and which one to run before a commit. A command you ran and saw work is worth more than one you only read.
- **Conventions** the configuration shows: the linter and formatter, what is generated and never edited by hand, where the tests go.
- **What this task found out**: whatever you had to dig for.

A first version is short, 20 to 40 lines: a start the next sessions add to, not a tour of the code. Say in your report that you created it, so the user can read it.

## What goes in

What you had to dig for, and the next session would have to dig for too:

- **Commands**: how to build, test, lint and run the app, with what matters about them (`npm run check` before a commit, it stops at the first failing step), not "run the tests".
- **Traps**: what surprised you, why, and what to do instead (`/tmp` is in RAM: keep large outputs on the disk).
- **Conventions** the code doesn't show at a glance: where a kind of file goes, what is generated and never edited by hand, how something is named.
- **Where to look**: the module that owns a concern, when its name doesn't say so.
- **How the user wants things done**, when they said so and it will come up again.

## What stays out

- What the code, the README or the history already say: a list of files, a summary of a module, the last changes.
- What only mattered to this task: the bug you fixed, the steps you took, what is left to do.
- Guesses: write what you checked, not what you suppose.
- Secrets, tokens, passwords, and anything personal.

## How to write it

- One fact per bullet, in a line or two: the fact, then the reason or the way around it.
- A few headings (Commands, Conventions, Traps…): add to one before making another.
- Short: the whole file goes to the model in every session, and each line costs context there. Past a couple of hundred lines, merge and prune.
- A note that turned out wrong: correct it or delete it at once. A stale note costs more than none.
- Edit it as you edit code: read it first, change only what you learned, and keep its style.
- Say in your report what you added or changed in it, so the user can check it.

## When

- At the end of every task, before the report, unasked: create the notes if there are none, add what this task found out that they lack, fix what misled you. Most tasks in a project that has notes add nothing; that is fine.
- When the user corrects how you did something and it will recur: that is a convention, note it.
- When the user asks for the project's notes: write or refresh them whole, as for first notes.
