// biome-ignore-all lint/style/useNamingConvention: the level fields are a model API's own names (reasoning_effort)
import { THINKING_MODES } from '../../../shared/protocol/choices.schema';
import { ModelThinkingSchema } from '../../../shared/protocol/models.schema';

import type { SelectOption } from '@syware/jlib';
import type { ThinkingMode } from '../../../shared/protocol/choices.schema';
import type { ModelThinking } from '../../../shared/protocol/models.schema';
import type {
  ThinkingChoice,
  ThinkingDictionary,
  ThinkingDictionaryName,
  ThinkingInput,
} from '../types/thinking-levels.types';

const NO_THINKING: ThinkingChoice = 'none';
const OWN_THINKING: ThinkingChoice = 'custom';

const BAD_THINKING_JSON = 'The thinking levels are JSON, as in {"low": {"reasoning_effort": "low"}}.';
const BAD_THINKING_MAPPING =
  'Each level maps field names to values as text: {"low": {"reasoning_effort": "low"}}.';

/** What a level's fields mean: the request fields a model's API takes for it, by dotted path. */
export const THINKING_MAPPING_HINT =
  'For each mode, the fields its API takes, by dotted path: {"low": {"reasoning_effort": "low"}, "off": {"thinking.type": "disabled"}}. A value that reads as JSON keeps its type (false, 0.6); other text stays text (max, disabled).';

/** The modes a model offers: all four while its preset names no levels, else only those it names. */
export const thinkingModesOf = (thinking: ModelThinking | undefined): readonly ThinkingMode[] =>
  thinking === undefined ? THINKING_MODES : THINKING_MODES.filter((mode) => thinking[mode] !== undefined);

/** The mode to work in when the model does not offer the one chosen: its strongest. */
export function thinkingOffered(thinking: ModelThinking | undefined, mode: ThinkingMode): ThinkingMode {
  const modes = thinkingModesOf(thinking);
  return modes.includes(mode) ? mode : (modes[0] ?? mode);
}

/** The known ways models take thinking, by name: a preset starts from one and may change it. */
export const THINKING_DICTIONARIES: Readonly<Record<ThinkingDictionaryName, ThinkingDictionary>> = {
  deepseek: {
    label: 'DeepSeek',
    thinking: {
      full: { reasoning_effort: 'max' },
      low: { reasoning_effort: 'low' },
      medium: { reasoning_effort: 'high' },
      off: { 'thinking.type': 'disabled' },
    },
  },
  effort: {
    label: 'Reasoning effort (OpenAI-compatible)',
    thinking: {
      full: { reasoning_effort: 'high' },
      low: { reasoning_effort: 'low' },
      medium: { reasoning_effort: 'medium' },
      off: { reasoning_effort: 'none' },
    },
  },
  halogen: {
    label: 'Halogen (Qwen3.8)',
    thinking: {
      full: { reasoning_effort: 'xhigh' },
      low: { reasoning_effort: 'low' },
      medium: { reasoning_effort: 'medium' },
      off: { reasoning_effort: 'none' },
    },
  },
  llama: {
    label: 'llama.cpp chat template (default)',
    thinking: {
      full: {},
      low: { 'chat_template_kwargs.reasoning_effort': 'low' },
      medium: { 'chat_template_kwargs.reasoning_effort': 'medium' },
      off: { 'chat_template_kwargs.enable_thinking': 'false' },
    },
  },
};

const THINKING_ORDER: readonly ThinkingDictionaryName[] = ['llama', 'halogen', 'deepseek', 'effort'];

/** The thinking choices of a preset's dialog, the model's own and a person's own at the end. */
export const THINKING_OPTIONS: readonly SelectOption[] = [
  ...THINKING_ORDER.map((value) => ({ label: THINKING_DICTIONARIES[value].label, value })),
  { label: 'The model’s own', value: NO_THINKING },
  { label: 'Custom', value: OWN_THINKING },
];

/** A model's mapping as the dialog shows it: JSON, for people to read and change. */
export const thinkingText = (thinking: ModelThinking | undefined): string =>
  thinking === undefined ? '' : JSON.stringify(thinking, null, 2);

/** A dictionary's mapping, when the choice names one. */
export const dictionaryOf = (choice: ThinkingChoice): ModelThinking | undefined =>
  choice === NO_THINKING || choice === OWN_THINKING ? undefined : THINKING_DICTIONARIES[choice].thinking;

const KNOWN_HOSTS: Readonly<Record<string, ThinkingDictionaryName>> = { 'api.deepseek.com': 'deepseek' };

/** The dictionary a source's address suggests, when swcode knows it: DeepSeek's models, say. */
function dictionaryOfHost(url: string): ThinkingDictionaryName | undefined {
  try {
    return KNOWN_HOSTS[new URL(url).hostname];
  } catch {
    // An address that does not read is no known host: the model's own levels stay.
    return undefined;
  }
}

const sameFields = (
  one: Record<string, string> | undefined,
  other: Record<string, string> | undefined
): boolean =>
  one === undefined || other === undefined
    ? one === other
    : Object.keys(one).length === Object.keys(other).length
      && Object.keys(one).every((key) => other[key] === one[key]);

const sameThinking = (one: ModelThinking, other: ModelThinking): boolean =>
  THINKING_MODES.every((mode: ThinkingMode) => sameFields(one[mode], other[mode]));

const isNone = (thinking: ModelThinking): boolean =>
  THINKING_MODES.every((mode: ThinkingMode) => thinking[mode] === undefined);

/** Which choice a preset's mapping is now: a known dictionary, none at all, or the person's own. */
export function thinkingChoiceOf(thinking: ModelThinking | undefined, sourceUrl: string): ThinkingChoice {
  if (thinking === undefined) {
    return dictionaryOfHost(sourceUrl) ?? 'llama';
  }
  if (isNone(thinking)) {
    return NO_THINKING;
  }
  const known = THINKING_ORDER.find((name) => sameThinking(THINKING_DICTIONARIES[name].thinking, thinking));
  return known ?? OWN_THINKING;
}

/** The mapping typed in the dialog: nothing when empty, the mapping, or why it cannot be read. */
export function readThinking(text: string): ThinkingInput {
  if (text.trim() === '') {
    return { kind: 'thinking', thinking: {} };
  }
  let parsed: unknown;
  try {
    parsed = JSON.parse(text);
  } catch {
    return { kind: 'invalid', problem: BAD_THINKING_JSON };
  }
  const read = ModelThinkingSchema.safeParse(parsed);
  return read.success
    ? { kind: 'thinking', thinking: read.data }
    : { kind: 'invalid', problem: BAD_THINKING_MAPPING };
}
