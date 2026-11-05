import { describe, expect, it } from 'vitest';

import {
  newId,
  presetUsing,
  sourceDescription,
  withoutPreset,
  withPreset,
  withSource,
} from '../src/lib/model-settings';
import { presetDescription, presetOf } from '../src/lib/presets';

import type { ModelSettings, Preset } from '../../shared/protocol/models.schema';

const big: Preset = {
  id: 'big',
  main: { model: 'qwen3.8-flash-next', source: 'garfield' },
  name: 'Big',
  workers: { model: 'qwen3.5-9b', source: 'gmktec' },
};
const local: Preset = {
  id: 'local',
  main: { model: '', source: 'gmktec' },
  name: 'Local',
  workers: { model: '', source: 'gmktec' },
};
const garfield = {
  apiKey: 'secret',
  id: 'garfield',
  name: 'garfield',
  url: 'https://garfield.example',
};
const settings: ModelSettings = {
  defaultPreset: 'local',
  presets: [local, big],
  sources: [{ id: 'gmktec', name: 'gmktec', url: 'http://gmktec:8080' }, garfield],
};
const choices = {
  defaultPreset: settings.defaultPreset,
  presets: settings.presets,
  sources: settings.sources.map(({ id, name }) => ({ id, name })),
};

describe('presets', () => {
  it('are the session’s own, else the default one, as the daemon picks them', () => {
    expect(presetOf(choices, 'big')?.id).toBe('big');
    expect(presetOf(choices, 'removed')?.id).toBe('local');
    expect(presetOf(choices, undefined)?.id).toBe('local');
  });

  it('say their models, and the workers’ only when they are others', () => {
    expect(presetDescription(choices, big)).toBe('qwen3.8-flash-next on garfield · workers: qwen3.5-9b on gmktec');
    expect(presetDescription(choices, local)).toBe('its model on gmktec');
  });
});

describe('model settings', () => {
  it('take a new id from a name, another one when it is taken', () => {
    expect(newId('My GMKtec!', [])).toBe('my-gmktec');
    expect(newId('gmktec', ['gmktec', 'gmktec-2'])).toBe('gmktec-3');
    expect(newId('***', [])).toBe('item');
  });

  it('change an item in place, add a new one at the end, and remove one', () => {
    const renamed = withPreset(settings, { ...big, name: 'Bigger' });
    expect(renamed.presets.map((preset) => preset.name)).toEqual(['Local', 'Bigger']);
    const added = withSource(settings, { id: 'third', name: 'third', url: 'http://third:8080' });
    expect(added.sources.map((source) => source.id)).toEqual(['gmktec', 'garfield', 'third']);
    expect(withoutPreset(settings, 'big').presets).toEqual([local]);
  });

  it('know which preset uses a source, and say a source in a few words', () => {
    expect(presetUsing(settings, 'garfield')?.id).toBe('big');
    expect(presetUsing(withoutPreset(settings, 'big'), 'garfield')).toBeUndefined();
    expect(sourceDescription(garfield)).toBe('https://garfield.example · with a key');
  });
});
