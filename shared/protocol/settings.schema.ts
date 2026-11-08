import { z } from 'zod';

/**
 * The settings the app's settings page changes: the names of `~/.swcode/settings.env`. The llm-servers and the
 * presets are `~/.swcode/models.json` (`models.schema.ts`).
 */
export const EDITABLE_SETTINGS = [
  'SWCODE_THINKING',
  'SWCODE_SUBAGENT_THINKING',
  'SWCODE_WORKSPACE',
  'SWCODE_SSH_ENDPOINTS',
  'SWCODE_IMAGE_URL',
  'SWCODE_HOST',
  'SWCODE_PORT',
  'SWCODE_MAX_STEPS',
  'SWCODE_SUBAGENT_MAX_STEPS',
] as const;
export const SettingNameSchema = z.enum(EDITABLE_SETTINGS);
export type SettingName = z.infer<typeof SettingNameSchema>;

/** Values by setting name; a missing one takes its default. */
export const SettingValuesSchema = z.partialRecord(SettingNameSchema, z.string());
export type SettingValues = z.infer<typeof SettingValuesSchema>;

/**
 * How a daemon takes new settings: it restarts itself in the background, or, run in a terminal, the person
 * restarts it.
 */
export const RESTARTS = ['self', 'person'] as const;
export type Restart = (typeof RESTARTS)[number];

/** What `GET /api/settings` answers: the settings in use, which the environment sets, and how they apply. */
export const SettingsViewSchema = z.object({
  // Set in the environment, which comes on top of the file: the page cannot change them.
  environment: z.array(SettingNameSchema),
  path: z.string(),
  restart: z.enum(RESTARTS),
  values: SettingValuesSchema,
});
export type SettingsView = z.infer<typeof SettingsViewSchema>;

/** What `PUT /api/settings` answers: saved, and how it applies; or why not. */
export const SettingsSavedSchema = z.discriminatedUnion('ok', [
  z.object({ ok: z.literal(true), restart: z.enum(RESTARTS) }),
  z.object({ ok: z.literal(false), problem: z.string() }),
]);
export type SettingsSaved = z.infer<typeof SettingsSavedSchema>;

/** `POST /api/settings/models`: the models an llm-server serves, to pick from before saving its address. */
export const ModelsQuerySchema = z.object({ apiKey: z.string().optional(), url: z.string() });
export const ModelsAnswerSchema = z.discriminatedUnion('ok', [
  z.object({ models: z.array(z.string()), ok: z.literal(true) }),
  z.object({ ok: z.literal(false), problem: z.string() }),
]);
export type ModelsAnswer = z.infer<typeof ModelsAnswerSchema>;
