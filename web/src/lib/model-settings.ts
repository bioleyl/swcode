import type { LlmSource, ModelSettings, Preset } from '../../../shared/protocol/models.schema';

const NOT_WORD = /[^a-z0-9]+/g;
const EDGES = /^-|-$/g;
const FALLBACK_ID = 'item';
const FIRST = 1;

const unique = (base: string, taken: readonly string[], count = FIRST): string => {
  const id = count === FIRST ? base : `${base}-${count}`;
  return taken.includes(id) ? unique(base, taken, count + 1) : id;
};

/** An id for a new source or preset, from its name, that no other has: "gmktec", then "gmktec-2". */
export const newId = (name: string, taken: readonly string[]): string =>
  unique(name.toLowerCase().replace(NOT_WORD, '-').replace(EDGES, '') || FALLBACK_ID, taken);

const replacing = <Item extends { id: string }>(items: readonly Item[], item: Item): Item[] =>
  items.some((each) => each.id === item.id)
    ? items.map((each) => (each.id === item.id ? item : each))
    : [...items, item];

/** The models with a source added, or changed in place. */
export const withSource = (settings: ModelSettings, source: LlmSource): ModelSettings => ({
  ...settings,
  sources: replacing(settings.sources, source),
});

/** The models with a preset added, or changed in place. */
export const withPreset = (settings: ModelSettings, preset: Preset): ModelSettings => ({
  ...settings,
  presets: replacing(settings.presets, preset),
});

export const withoutSource = (settings: ModelSettings, id: string): ModelSettings => ({
  ...settings,
  sources: settings.sources.filter((source) => source.id !== id),
});

export const withoutPreset = (settings: ModelSettings, id: string): ModelSettings => ({
  ...settings,
  presets: settings.presets.filter((preset) => preset.id !== id),
});

/** The first preset that asks a source for one of its models: a source in use cannot go. */
export const presetUsing = (settings: ModelSettings, source: string): Preset | undefined =>
  settings.presets.find((preset) => preset.main.source === source || preset.workers.source === source);

/** A source in a few words: "http://gmktec:8080 · share 2/2 · with a key". */
export const sourceDescription = (source: LlmSource): string =>
  [source.url, source.slotShare ? `share ${source.slotShare}` : '', source.apiKey ? 'with a key' : '']
    .filter((part) => part !== '')
    .join(' · ');
