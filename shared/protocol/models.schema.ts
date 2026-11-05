import { z } from 'zod';

const NAME_LENGTH = 40;

/** An llm-server the agents ask: its name in the app, its address, and its key when it wants one. */
export const LlmSourceSchema = z.object({
  // Only when llm-server was started with --api-key.
  apiKey: z.string().trim().optional(),
  id: z.string().min(1),
  name: z.string().trim().min(1).max(NAME_LENGTH),
  url: z.string().trim().min(1),
});
export type LlmSource = z.infer<typeof LlmSourceSchema>;

/**
 * A model of a source, by the name the source serves it as. Empty for a single-model llm-server's, or, on a
 * router, its first loaded model.
 */
export const ModelRefSchema = z.object({ model: z.string().trim(), source: z.string().min(1) });
export type ModelRef = z.infer<typeof ModelRefSchema>;

/** The models a session works with, picked together by name: the main agent's, and the workers' (subagents, the web page reader). */
export const PresetSchema = z.object({
  id: z.string().min(1),
  main: ModelRefSchema,
  name: z.string().trim().min(1).max(NAME_LENGTH),
  workers: ModelRefSchema,
});
export type Preset = z.infer<typeof PresetSchema>;

/** The sources and presets: `~/.swcode/models.json`, `GET` and `PUT /api/models`. */
export const ModelSettingsSchema = z.object({
  // The preset of a new session, and of one whose preset was removed.
  defaultPreset: z.string().min(1),
  presets: z.array(PresetSchema).min(1),
  sources: z.array(LlmSourceSchema).min(1),
});
export type ModelSettings = z.infer<typeof ModelSettingsSchema>;

/** What `PUT /api/models` answers: saved and in use at once; or why not. */
export const ModelsSavedSchema = z.discriminatedUnion('ok', [
  z.object({ ok: z.literal(true) }),
  z.object({ ok: z.literal(false), problem: z.string() }),
]);
export type ModelsSaved = z.infer<typeof ModelsSavedSchema>;

/** The presets as every page picks them, and the sources' names they show: no address, no key. */
export const PresetChoicesSchema = z.object({
  defaultPreset: z.string(),
  presets: z.array(PresetSchema),
  sources: z.array(z.object({ id: z.string(), name: z.string() })),
});
export type PresetChoices = z.infer<typeof PresetChoicesSchema>;

/** A source as `GET /api/status` sees it now: whether it answers, and the models it serves. */
export const SourceStatusSchema = z.object({
  id: z.string(),
  models: z.array(z.string()),
  name: z.string(),
  reachable: z.boolean(),
  url: z.string(),
});
export type SourceStatus = z.infer<typeof SourceStatusSchema>;

/** A preset's main model as its source says now: its name, the context it holds, and whether it sees pictures. */
export const PresetStatusSchema = z.object({
  // The context a conversation has, when llm-server says it.
  contextTokens: z.number().int().optional(),
  id: z.string(),
  // The model asked for: a router's first loaded one when the preset names none.
  model: z.string().optional(),
  // Whether it sees pictures: llm-server loaded a vision projector (--mmproj) for it.
  vision: z.boolean().optional(),
});
export type PresetStatus = z.infer<typeof PresetStatusSchema>;
