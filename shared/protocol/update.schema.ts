import { z } from 'zod';

import { RESTARTS } from './settings.schema';

/** A version of swcode published on GitHub: its number (0.4.0), what it brings, and its page. */
export const ReleaseSchema = z.object({ notes: z.string(), page: z.string(), version: z.string() });
export type Release = z.infer<typeof ReleaseSchema>;

/**
 * What swcode knows of its next version: nothing for a build from the sources, which never looks; not looked yet;
 * this one is the latest, or a newer one is out (since `at`); the last look failed; or the newer one is installed
 * and runs once swcode starts again.
 */
export const UpdateStateSchema = z.discriminatedUnion('kind', [
  z.object({ kind: z.literal('source') }),
  z.object({ kind: z.literal('unchecked') }),
  z.object({ at: z.string(), kind: z.literal('current') }),
  z.object({ at: z.string(), kind: z.literal('newer'), release: ReleaseSchema }),
  z.object({ at: z.string(), kind: z.literal('failed'), problem: z.string() }),
  z.object({ kind: z.literal('installed'), release: ReleaseSchema }),
]);
export type UpdateState = z.infer<typeof UpdateStateSchema>;

/**
 * `GET /api/update`: the build that runs (0.3.0+7bd0cd6), whether it looks for a newer one every 10 minutes, what
 * it found, and how an update applies: swcode restarts by itself, or, run in a terminal, the person restarts it.
 */
export const UpdateViewSchema = z.object({
  checks: z.boolean(),
  restart: z.enum(RESTARTS),
  state: UpdateStateSchema,
  version: z.string(),
});
export type UpdateView = z.infer<typeof UpdateViewSchema>;

/** `PUT /api/update`: whether to look every 10 minutes from now on. */
export const UpdateChangeSchema = z.object({ checks: z.boolean() });
export type UpdateChange = z.infer<typeof UpdateChangeSchema>;

/** What `POST /api/update/install` answers: the version installed and how it applies; or why not. */
export const UpdateInstalledSchema = z.discriminatedUnion('ok', [
  z.object({ ok: z.literal(true), restart: z.enum(RESTARTS), version: z.string() }),
  z.object({ ok: z.literal(false), problem: z.string() }),
]);
export type UpdateInstalled = z.infer<typeof UpdateInstalledSchema>;
