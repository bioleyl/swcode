import { ENDPOINTS } from '../../../shared/protocol/endpoints';
import {
  ModelsAnswerSchema,
  SettingsSavedSchema,
  SettingsViewSchema,
} from '../../../shared/protocol/settings.schema';
import { jsonOf, request } from './api';

import type {
  ModelsAnswer,
  SettingsSaved,
  SettingsView,
  SettingValues,
} from '../../../shared/protocol/settings.schema';

const UNREACHABLE = 'The daemon did not answer.';

/** The settings in use, as the daemon reads them. */
export async function loadSettings(): Promise<SettingsView | undefined> {
  const parsed = SettingsViewSchema.safeParse(await jsonOf(await request('GET', ENDPOINTS.settings)));
  return parsed.success ? parsed.data : undefined;
}

/** Saves the settings; the daemon then restarts on them, or says why it would not. */
export async function saveSettings(values: SettingValues): Promise<SettingsSaved> {
  const parsed = SettingsSavedSchema.safeParse(await jsonOf(await request('PUT', ENDPOINTS.settings, values)));
  return parsed.success ? parsed.data : { ok: false, problem: UNREACHABLE };
}

/** The models a llama-server serves, at an address not saved yet. */
export async function modelsAt(url: string, apiKey: string): Promise<ModelsAnswer> {
  const parsed = ModelsAnswerSchema.safeParse(
    await jsonOf(await request('POST', ENDPOINTS.settingsModels, { apiKey, url }))
  );
  return parsed.success ? parsed.data : { ok: false, problem: UNREACHABLE };
}
