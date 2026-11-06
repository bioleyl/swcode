import { z } from 'zod';

/**
 * `auto`: edits and commands go through. `acceptEdits`: edits go through, commands wait for a person. `ask`: both
 * wait.
 */
export const PERMISSION_MODES = ['auto', 'acceptEdits', 'ask'] as const;
export const PermissionModeSchema = z.enum(PERMISSION_MODES);
export type PermissionMode = z.infer<typeof PermissionModeSchema>;

/**
 * How much the model thinks before it answers: as much as it does by itself (`full`), less (`medium`, `low`, the
 * levels Qwen 3.8 and gpt-oss templates know), or not at all (`off`, instruct).
 */
export const THINKING_MODES = ['full', 'medium', 'low', 'off'] as const;
export const ThinkingModeSchema = z.enum(THINKING_MODES);
export type ThinkingMode = z.infer<typeof ThinkingModeSchema>;

/**
 * What a new session of a project starts with, all four at once: its preset (the default preset when empty), its
 * permissions, how much its model thinks, token economy. The person can still pick otherwise for one session.
 */
export const SessionDefaultsSchema = z.object({
  concise: z.boolean(),
  mode: PermissionModeSchema,
  preset: z.string(),
  thinking: ThinkingModeSchema,
});
export type SessionDefaults = z.infer<typeof SessionDefaultsSchema>;
