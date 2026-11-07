import { confirmDialog } from '@syware/jlib';

import { ENDPOINTS } from '../../../shared/protocol/endpoints';
import {
  ModelsAnswerSchema,
  SettingsSavedSchema,
  SettingsViewSchema,
} from '../../../shared/protocol/settings.schema';
import { RESTART_QUESTION, restartsOnSave } from '../lib/settings-saved';
import { jsonOf, request } from './api';

import type {
  ModelsAnswer,
  Restart,
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

/**
 * Whether to go on with saving the settings file: where swcode restarts by itself on it, the sessions open now can
 * break, so the person confirms first.
 */
export async function confirmRestart(restart: Restart | undefined): Promise<boolean> {
  if (!restartsOnSave(restart)) {
    return true;
  }
  return confirmDialog(RESTART_QUESTION);
}

/** The models an llm-server serves, at an address not saved yet. */
export async function modelsAt(url: string, apiKey: string): Promise<ModelsAnswer> {
  const parsed = ModelsAnswerSchema.safeParse(
    await jsonOf(await request('POST', ENDPOINTS.settingsModels, { apiKey, url }))
  );
  return parsed.success ? parsed.data : { ok: false, problem: UNREACHABLE };
}
