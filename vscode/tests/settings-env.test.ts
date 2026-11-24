import { describe, expect, it } from 'vitest';

import { parseSettingsEnv, portOf } from '../src/lib/settings-env';

describe('the settings file', () => {
  it('reads a plain value', () => {
    expect(parseSettingsEnv('SWCODE_PORT=4700')).toEqual({ SWCODE_PORT: '4700' });
  });

  it('reads the keys the settings page writes, with the export prefix and a comment', () => {
    const text = [
      '# swcode’s settings: the settings page writes them.',
      'export SWCODE_PORT=4711',
      'SWCODE_LLM_URL=http://127.0.0.1:8080',
      '',
      'SWCODE_TOKEN=abc=',
    ].join('\n');
    expect(parseSettingsEnv(text)).toEqual({
      SWCODE_PORT: '4711',
      SWCODE_LLM_URL: 'http://127.0.0.1:8080',
      SWCODE_TOKEN: 'abc=',
    });
  });

  it('reads a quoted value, and unquotes it', () => {
    const text = ["SWCODE_PORT='4712'", 'SWCODE_DATA_DIR="~/work"', 'SWCODE_TOKEN=`abc`'].join('\n');
    expect(parseSettingsEnv(text)).toEqual({
      SWCODE_PORT: '4712',
      SWCODE_DATA_DIR: '~/work',
      SWCODE_TOKEN: 'abc',
    });
  });

  it('keeps the last value saved for a key', () => {
    expect(parseSettingsEnv('SWCODE_PORT=4700\nSWCODE_PORT=4713')).toEqual({ SWCODE_PORT: '4713' });
  });

  it('reads nothing from an empty file', () => {
    expect(parseSettingsEnv('')).toEqual({});
  });
});

describe('the port', () => {
  it('takes the default when the settings name none', () => {
    expect(portOf({})).toBe(4500);
  });

  it('takes the saved port', () => {
    expect(portOf({ SWCODE_PORT: '4711' })).toBe(4711);
  });

  it('takes the default for a port that is not one', () => {
    expect(portOf({ SWCODE_PORT: 'port' })).toBe(4500);
    expect(portOf({ SWCODE_PORT: '0' })).toBe(4500);
    expect(portOf({ SWCODE_PORT: '65536' })).toBe(4500);
  });

  it('takes the edge ports', () => {
    expect(portOf({ SWCODE_PORT: '1' })).toBe(1);
    expect(portOf({ SWCODE_PORT: '65535' })).toBe(65535);
  });
});
