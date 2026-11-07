# swcode: notes for working on the code

Before writing code, look at the skills in `~/.agents/skills` and in the project's `.agents/skills` (a `SKILL.md` in
each folder) and load those that fit the task: Claude Code does not find them there by itself. Of two skills of one
name, the one in `~/.agents/skills` wins. The app needs `jadis-official` (the frontend library), `jlib-official` and
`syware-ux` (the components the app is built from, and how SyWare apps behave), and `swcode-base` (named constants,
guard clauses, exhaustive mappers, types in `types/` directories, a check of the diff). `swcode-base` is written in
this repository (`plugins/swcode-base/skills/swcode-base`); a missing other installs with
`npx skills add bioleyl/jadis --agent universal --yes` or `npx skills add bioleyl/jlib --agent universal --yes`.

The project's notes are in `AGENTS.md`, which swcode's own agent reads too: add to them there, not here.

@AGENTS.md
