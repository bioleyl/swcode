import type { ModelThinking } from '../../../shared/protocol/models.schema';

/** A known way a model's API takes thinking: what to call it, and the fields each mode sends. */
export interface ThinkingDictionary {
  label: string;
  thinking: ModelThinking;
}

export type ThinkingDictionaryName = 'llama' | 'deepseek' | 'effort' | 'halogen';

/** What the preset dialog offers: a known dictionary, the model's own (no fields), or the person's own. */
export type ThinkingChoice = ThinkingDictionaryName | 'none' | 'custom';

/** A thinking mapping as typed in the dialog: the mapping, or why it cannot be read. */
export type ThinkingInput = { kind: 'thinking'; thinking: ModelThinking } | { kind: 'invalid'; problem: string };
