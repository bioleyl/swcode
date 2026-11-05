import { modelName } from './model-name';

import type { Status } from '../../../shared/protocol/messages.schema';
import type { ModelRef, Preset, PresetChoices, PresetStatus } from '../../../shared/protocol/models.schema';

/** Before the daemon says its presets. */
export const NO_PRESETS: PresetChoices = { defaultPreset: '', presets: [], sources: [] };

// A model a preset names none for: a single-model llm-server's, or a router's first loaded one.
const ITS_MODEL = 'its model';

/** The preset a session works with, as the daemon picks it: its own, else the default one. */
export const presetOf = (choices: PresetChoices, id: string | undefined): Preset | undefined =>
  choices.presets.find((preset) => preset.id === id)
  ?? choices.presets.find((preset) => preset.id === choices.defaultPreset)
  ?? choices.presets[0];

/** A preset's main model, as `GET /api/status` says it now: its name, a slot's context and its vision. */
export const presetStatusOf = (status: Status | undefined, preset: Preset | undefined): PresetStatus | undefined =>
  status?.llama.presets.find((each) => each.id === preset?.id);

const sourceName = (choices: PresetChoices, id: string): string =>
  choices.sources.find((source) => source.id === id)?.name ?? id;

const modelText = (choices: PresetChoices, ref: ModelRef): string =>
  `${ref.model === '' ? ITS_MODEL : modelName(ref.model)} on ${sourceName(choices, ref.source)}`;

/** Workers asking for no model of their own on the main agent's source get the main agent's. */
const workersShareMain = (preset: Preset): boolean =>
  preset.workers.source === preset.main.source
  && (preset.workers.model === '' || preset.workers.model === preset.main.model);

/** A preset's models in words: "flash-next on garfield · workers: qwen-small on gmktec". */
export const presetDescription = (choices: PresetChoices, preset: Preset): string =>
  workersShareMain(preset)
    ? modelText(choices, preset.main)
    : `${modelText(choices, preset.main)} · workers: ${modelText(choices, preset.workers)}`;
