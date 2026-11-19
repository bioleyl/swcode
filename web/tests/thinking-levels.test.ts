// biome-ignore-all lint/style/useNamingConvention: the level fields are a model API's own names (reasoning_effort)
import { describe, expect, it } from 'vitest';

import {
  dictionaryOf,
  readThinking,
  THINKING_DICTIONARIES,
  thinkingChoiceOf,
  thinkingModesOf,
  thinkingOffered,
  thinkingText,
} from '../src/lib/thinking-levels';

describe('a model’s thinking levels', () => {
  it('reads the mapping typed in the dialog', () => {
    expect(readThinking('{"low":{"reasoning_effort":"max"},"off":{"thinking.type":"disabled"}}')).toEqual({
      kind: 'thinking',
      thinking: { low: { reasoning_effort: 'max' }, off: { 'thinking.type': 'disabled' } },
    });
  });

  it('says why a mapping cannot be read', () => {
    expect(readThinking('{no').kind).toBe('invalid');
    expect(readThinking('{"low":{"reasoning_effort":true}}').kind).toBe('invalid');
  });

  it('sends nothing when the dialog holds no mapping', () => {
    expect(readThinking('  ')).toEqual({ kind: 'thinking', thinking: {} });
  });

  it('names the dictionary a mapping is, and the person’s own otherwise', () => {
    expect(thinkingChoiceOf(THINKING_DICTIONARIES.deepseek.thinking, 'http://127.0.0.1:8080')).toBe('deepseek');
    expect(thinkingChoiceOf({}, 'https://api.deepseek.com')).toBe('none');
    expect(thinkingChoiceOf({ low: { reasoning_effort: 'min' } }, 'https://api.deepseek.com')).toBe('custom');
  });

  it('suggests the dictionary of a known host, the template’s otherwise', () => {
    expect(thinkingChoiceOf(undefined, 'https://api.deepseek.com')).toBe('deepseek');
    expect(thinkingChoiceOf(undefined, 'http://127.0.0.1:8080')).toBe('llama');
    expect(thinkingChoiceOf(undefined, 'not an address')).toBe('llama');
  });

  it('offers the modes a model names, all four when it names none', () => {
    expect(thinkingModesOf(undefined)).toEqual(['full', 'medium', 'low', 'off']);
    expect(thinkingModesOf({ full: {}, low: {} })).toEqual(['full', 'low']);
    expect(thinkingModesOf({})).toEqual([]);
  });

  it('takes the strongest offered mode when the chosen one is missing', () => {
    expect(thinkingOffered({ low: {} }, 'full')).toBe('low');
    expect(thinkingOffered(undefined, 'off')).toBe('off');
    expect(thinkingOffered({}, 'medium')).toBe('medium');
  });

  it('writes a mapping as it reads it back', () => {
    const thinking = dictionaryOf('effort') ?? {};
    expect(readThinking(thinkingText(thinking))).toEqual({ kind: 'thinking', thinking });
  });
});
