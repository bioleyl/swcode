import { z } from 'zod';

import { SessionDefaultsSchema } from './choices.schema.js';

/** Where a folder is: this computer, a WSL distribution of it, or a machine reached over SSH. */
export const HostRefSchema = z.discriminatedUnion('kind', [
  z.object({ kind: z.literal('local') }),
  z.object({ distro: z.string().trim().min(1), kind: z.literal('wsl') }),
  z.object({ endpoint: z.string().trim().min(1), kind: z.literal('ssh') }),
]);
export type HostRef = z.infer<typeof HostRefSchema>;
export const LOCAL_HOST: HostRef = { kind: 'local' };

/** A host to offer for a new project, as people read it. */
export const HostOptionSchema = z.object({ label: z.string(), ref: HostRefSchema });
export type HostOption = z.infer<typeof HostOptionSchema>;

/** How much a project's todo may say, and how many a new session may pin. */
export const MAX_TODO_TEXT = 500;
export const MAX_PINNED_TODOS = 20;
export const TodoTextSchema = z.string().trim().min(1).max(MAX_TODO_TEXT);

/** A todo of a project: what it has to do, whether it is done, and the session it was pinned to. */
export const ProjectTodoSchema = z.object({
  createdAt: z.iso.datetime(),
  done: z.boolean().default(false),
  id: z.string(),
  // The session whose first message this todo was pinned to, once one started on it.
  sessionId: z.string().optional(),
  text: TodoTextSchema,
});
export type ProjectTodo = z.infer<typeof ProjectTodoSchema>;

/**
 * What a project's sessions do without asking, as people answered “in this project”: programs by name, in the
 * sandbox and outside it, and MCP servers by name.
 */
export const ALLOWED_RULES = ['program', 'outside', 'server'] as const;
export const AllowedRuleSchema = z.enum(ALLOWED_RULES);
export type AllowedRule = z.infer<typeof AllowedRuleSchema>;

export const ProjectAllowedSchema = z.object({
  outside: z.array(z.string()).default([]),
  programs: z.array(z.string()).default([]),
  servers: z.array(z.string()).default([]),
});
export type ProjectAllowed = z.infer<typeof ProjectAllowedSchema>;

/** How often a schedule starts a session: every hour, every day, or every week on a weekday. */
export const ScheduleEverySchema = z.enum(['hour', 'day', 'week']);
export type ScheduleEvery = z.infer<typeof ScheduleEverySchema>;

/** How much a schedule's prompt may say, and how many schedules a project keeps. */
export const MAX_SCHEDULE_PROMPT = 2000;
export const MAX_SCHEDULES = 50;
export const SchedulePromptSchema = z.string().trim().min(1).max(MAX_SCHEDULE_PROMPT);
/** A time of the day, in the daemon's own time: `08:00`. Every hour, its minute alone counts. */
export const ScheduleTimeSchema = z
  .string()
  .regex(/^([01]\d|2[0-3]):[0-5]\d$/, 'a time of the day, such as 08:00');
/** A day of the week, Sunday being 0: for a schedule that runs every week. */
export const ScheduleWeekdaySchema = z.number().int().min(0).max(6);

/**
 * A session a project starts by itself: on this prompt, every hour, day or week at that time, while it is on. The
 * daemon notes when it last ran it and the session it started.
 */
export const ProjectScheduleSchema = z.object({
  at: ScheduleTimeSchema,
  every: ScheduleEverySchema,
  id: z.string(),
  lastRunAt: z.iso.datetime().optional(),
  lastSessionId: z.string().optional(),
  on: z.boolean().default(true),
  prompt: SchedulePromptSchema,
  // From when its times count: when it was added, or last turned on. A time that passed before is not made up for.
  since: z.iso.datetime(),
  weekday: ScheduleWeekdaySchema.default(0),
});
export type ProjectSchedule = z.infer<typeof ProjectScheduleSchema>;

/** A folder sessions work in, on a host, with a name for people: the folder's, at first. */
// The most a project's check after an edit is: a command line.
export const MAX_AFTER_EDIT = 500;

export const ProjectSchema = z.object({
  // A command run in the project's folder after each file the agent writes or edits, `{file}` standing for the
  // file: a formatter, a linter, a type check. What it says goes back to the agent with the edit's result.
  afterEdit: z.string().optional(),
  // What its sessions do without asking.
  allowed: ProjectAllowedSchema.optional(),
  createdAt: z.iso.datetime(),
  // What its new sessions start with; the app's own defaults when it has none.
  defaults: SessionDefaultsSchema.optional(),
  // Projects from before hosts are on this computer.
  host: HostRefSchema.default(LOCAL_HOST),
  id: z.string(),
  name: z.string(),
  path: z.string(),
  // Its sessions' commands run in a sandbox where its host can make one, unless turned off here.
  sandboxOff: z.boolean().optional(),
  // The sessions it starts by itself, each on its prompt at its time.
  schedules: z.array(ProjectScheduleSchema).max(MAX_SCHEDULES).optional(),
  // The skills its sessions are neither told of nor can run, by name: every skill found is on until turned off here.
  skillsOff: z.array(z.string()).default([]),
  // What is left to do in the project, in the order it was added.
  todos: z.array(ProjectTodoSchema).default([]),
  // Added from a folder that was already there, and not trusted yet: its notes (AGENTS.md) and its own skills are
  // kept from the agent, which follows them as the person's words, and its commands keep their sandbox.
  untrusted: z.boolean().optional(),
});
export type Project = z.infer<typeof ProjectSchema>;

/**
 * `GET /api/sandbox?project=<id>`: whether the project's host can run commands in a sandbox (bubblewrap, on Linux),
 * and why not when it cannot.
 */
export const SandboxViewSchema = z.object({ available: z.boolean(), problem: z.string().optional() });
export type SandboxView = z.infer<typeof SandboxViewSchema>;

// A folder made for a new project: one name, not a path.
const FOLDER_NAME = /^[^/\\]+$/;
const MAX_FOLDER_NAME = 255;

export const NewFolderNameSchema = z
  .string()
  .trim()
  .min(1)
  .max(MAX_FOLDER_NAME)
  .regex(FOLDER_NAME, 'a folder name, without / or \\');
