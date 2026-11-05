import { modelName } from './model-name';

import type { SelectOption } from '@syware/jlib';
import type { Status } from '../../../shared/protocol/messages.schema';
import type {
  ModelRef,
  Preset,
  PresetChoices,
  PresetStatus,
  SubagentLimit,
} from '../../../shared/protocol/models.schema';

/** Before the daemon says its presets. */
export const NO_PRESETS: PresetChoices = { defaultPreset: '', presets: [], sources: [] };

// A model a preset names none for: a single-model llm-server's, or a router's first loaded one.
const ITS_MODEL = 'its model';
const NO_SUBAGENTS = 'no subagents';
// jlib's select takes an empty value for nothing chosen: no limit, the default, has a value of its own.
const NO_LIMIT = 'any';
const LIMITS_OFFERED = [1, 2, 3, 4, 5, 10];

/** The choices of how many subagents a turn may start: none, a few, or no limit. */
export const SUBAGENT_LIMIT_OPTIONS: readonly SelectOption[] = [
  { label: 'None: the main agent works alone', value: '0' },
  ...LIMITS_OFFERED.map((count) => ({ label: String(count), value: String(count) })),
  { label: 'No limit', value: NO_LIMIT },
];

/** A limit as the select shows it. */
export const subagentLimitChoice = (limit: SubagentLimit | undefined): string =>
  limit === undefined ? NO_LIMIT : String(limit.perTurn);

/** The limit a select's choice stands for: none for no limit. */
export const subagentLimitOf = (choice: string): SubagentLimit | undefined =>
  choice === NO_LIMIT || choice === '' ? undefined : { perTurn: Number(choice) };

/** The preset a session works with, as the daemon picks it: its own, else the default one. */
export const presetOf = (choices: PresetChoices, id: string | undefined): Preset | undefined =>
  choices.presets.find((preset) => preset.id === id)
  ?? choices.presets.find((preset) => preset.id === choices.defaultPreset)
  ?? choices.presets[0];

/** A preset's main model, as `GET /api/status` says it now: its name, the context it holds and its vision. */
export const presetStatusOf = (status: Status | undefined, preset: Preset | undefined): PresetStatus | undefined =>
  status?.llama.presets.find((each) => each.id === preset?.id);

const sourceName = (choices: PresetChoices, id: string): string =>
  choices.sources.find((source) => source.id === id)?.name ?? id;

const modelText = (choices: PresetChoices, ref: ModelRef): string =>
  `${ref.model === '' ? ITS_MODEL : modelName(ref.model)} on ${sourceName(choices, ref.source)}`;

/** Subagents asking for no model of their own on the main agent's source get the main agent's. */
const subagentsShareMain = (preset: Preset): boolean =>
  preset.workers.source === preset.main.source
  && (preset.workers.model === '' || preset.workers.model === preset.main.model);

/** What a preset's subagents are in words, when it says more than the main agent: their model, how many a turn. */
const subagentText = (choices: PresetChoices, preset: Preset): string => {
  const limit = preset.subagentLimit?.perTurn;
  if (limit === 0) {
    return NO_SUBAGENTS;
  }
  const parts = [
    subagentsShareMain(preset) ? '' : modelText(choices, preset.workers),
    limit === undefined ? '' : `${limit} a turn`,
  ].filter((part) => part !== '');
  return parts.length === 0 ? '' : `subagents: ${parts.join(', ')}`;
};

/** A preset's models in words: "flash-next on garfield · subagents: qwen-small on gmktec, 2 a turn". */
export const presetDescription = (choices: PresetChoices, preset: Preset): string =>
  [modelText(choices, preset.main), subagentText(choices, preset)].filter((part) => part !== '').join(' · ');
