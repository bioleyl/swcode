import { z } from 'zod';

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

/** A folder sessions work in, on a host, with a name for people: the folder's, at first. */
export const ProjectSchema = z.object({
  createdAt: z.iso.datetime(),
  // Projects from before hosts are on this computer.
  host: HostRefSchema.default(LOCAL_HOST),
  id: z.string(),
  name: z.string(),
  path: z.string(),
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
