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

/** A folder sessions work in, on a host, with a name for people: the folder's, at first. */
export const ProjectSchema = z.object({
  createdAt: z.iso.datetime(),
  // What its new sessions start with; the app's own defaults when it has none.
  defaults: SessionDefaultsSchema.optional(),
  // Projects from before hosts are on this computer.
  host: HostRefSchema.default(LOCAL_HOST),
  id: z.string(),
  name: z.string(),
  path: z.string(),
  // The skills its sessions are neither told of nor can run, by name: every skill found is on until turned off here.
  skillsOff: z.array(z.string()).default([]),
  // What is left to do in the project, in the order it was added.
  todos: z.array(ProjectTodoSchema).default([]),
});
export type Project = z.infer<typeof ProjectSchema>;

// A folder made for a new project: one name, not a path.
const FOLDER_NAME = /^[^/\\]+$/;
const MAX_FOLDER_NAME = 255;

export const NewFolderNameSchema = z
  .string()
  .trim()
  .min(1)
  .max(MAX_FOLDER_NAME)
  .regex(FOLDER_NAME, 'a folder name, without / or \\');
