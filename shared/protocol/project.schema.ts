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
