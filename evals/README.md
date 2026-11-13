# swcode's evals

A dozen small tasks the agent does against your own llm-server, each scored on success, steps, tokens and time. Run
them before and after a change of prompt, tool or model, and compare the two tables.

## Run them

```bash
npm run eval                                  # every task, with the swcode on your PATH
SWCODE_BIN=./dist/swcode npm run eval         # with a binary you just built
npm run eval -- --only off-by-one --only fix-shell-script
```

The runner (`scripts/eval.ts`) does this for each task:

1. copies the task's `files/` into a fresh workspace, `evals/.work/<task>`, and commits them in a git repository of
   its own;
2. runs `swcode run --json --cwd <workspace> --timeout <the task's>s "<prompt>"`, and reads the run's last line,
   `{"kind":"runFinished","outcome":…,"steps":…,"toolCalls":…,"promptTokens":…,"completionTokens":…,"seconds":…}`;
3. runs the task's `check` in the workspace: exit 0 is a success, whatever the run said of itself.

It prints a table, a line a task and the totals, writes the same numbers to `evals/results/<date>.json`, and exits 1
when a task failed. The workspaces stay until the next run, to look at what the agent did (`git diff` in one).
Neither `.work/` nor `results/` is committed.

The model and its server are the ones your swcode is set up with (`~/.swcode/settings.env`, or the environment):
the scores say something of the model as much as of swcode, so compare runs made with the same one.

## A task

A folder of `evals/tasks`, named after what it asks:

```
evals/tasks/off-by-one/
  task.json     { "prompt": "…", "check": "…", "timeoutSeconds": 300 }
  files/        the workspace as the agent finds it
```

- `prompt` is the person's message, as they would write it.
- `check` is a shell command run in the workspace once the agent is done. It needs only `node` and POSIX tools: no
  install, no network. It looks at what the task asked for, not at how it was done, and it must fail on `files/` as
  they are (a test holds every task to that).
- `timeoutSeconds` is how long the agent has.

## Add one

Make the folder, write `task.json` and the starting files, then run `npx vitest run evals/tests` (the check fails on
the starting files) and `npm run eval -- --only <name>` (a model can do it). A good task is small, has one right
outcome a command can tell, and fails for one reason: the thing it is about.
