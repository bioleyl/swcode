import { ENDPOINTS } from '../../../shared/protocol/endpoints';
import { ModelSettingsSchema, ModelsSavedSchema } from '../../../shared/protocol/models.schema';
import { jsonOf, request } from './api';

import type { ModelSettings, ModelsSaved } from '../../../shared/protocol/models.schema';

const UNREACHABLE = 'The daemon did not answer.';

/** The llama-servers and the presets, with the servers' keys. */
export async function loadModels(): Promise<ModelSettings | undefined> {
  const parsed = ModelSettingsSchema.safeParse(await jsonOf(await request('GET', ENDPOINTS.models)));
  return parsed.success ? parsed.data : undefined;
}

/** Saves them all: the daemon uses them at once, or says why it would not. */
export async function saveModels(settings: ModelSettings): Promise<ModelsSaved> {
  const parsed = ModelsSavedSchema.safeParse(await jsonOf(await request('PUT', ENDPOINTS.models, settings)));
  return parsed.success ? parsed.data : { ok: false, problem: UNREACHABLE };
}
