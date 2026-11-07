import { describe, expect, it } from 'vitest';

import { isPhone } from '../src/lib/devices';
import { updateFacts, updateNote } from '../src/lib/update-view';

import type { UpdateView } from '../../shared/protocol/update.schema';

const NOW = new Date('2026-11-07T10:30:00.000Z');
const RELEASE = {
  notes: 'Updates itself.',
  page: 'https://github.com/bioleyl/swcode/releases/tag/v0.4.0',
  version: '0.4.0',
};
const view = (state: UpdateView['state'], restart: UpdateView['restart'] = 'self'): UpdateView => ({
  checks: true,
  restart,
  state,
  version: '0.3.0+7bd0cd6',
});

describe('isPhone', () => {
  it('tells a phone from a computer by what its browser says', () => {
    expect(isPhone('Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) Mobile/15E148 Safari/604.1')).toBe(
      true
    );
    expect(isPhone('Mozilla/5.0 (Linux; Android 15; Pixel 9) Chrome/131.0 Mobile Safari/537.36')).toBe(true);
    expect(isPhone('Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/131.0 Safari/537.36 Edg/131.0')).toBe(false);
    expect(isPhone('Mozilla/5.0 (Macintosh; Intel Mac OS X 15_1) Safari/605.1.15')).toBe(false);
  });
});

describe('updateFacts', () => {
  it('lists the version that runs, the newer one found, and when swcode looked', () => {
    expect(updateFacts(view({ at: '2026-11-07T10:26:00.000Z', kind: 'newer', release: RELEASE }), NOW)).toEqual([
      { code: true, label: 'This version', value: '0.3.0+7bd0cd6' },
      { label: 'Latest release', value: '0.4.0' },
      { label: 'Looked', value: '4 min ago' },
    ]);
  });

  it('says this one is the latest, and leaves out what a build from the sources cannot know', () => {
    expect(updateFacts(view({ at: '2026-11-07T10:30:00.000Z', kind: 'current' }), NOW)[1]?.value).toBe('This one');
    const fromSources = updateFacts(view({ kind: 'source' }), NOW);
    expect(fromSources.map((fact) => fact.value)).toEqual(['0.3.0+7bd0cd6', null, null]);
  });
});

describe('updateNote', () => {
  it('says why the last look failed, and nothing while a newer version waits to be installed', () => {
    expect(
      updateNote(view({ at: NOW.toISOString(), kind: 'failed', problem: 'GitHub answered 403 Forbidden' }))
    ).toEqual({
      text: 'The last look failed: GitHub answered 403 Forbidden',
      tone: 'warn',
    });
    expect(updateNote(view({ at: NOW.toISOString(), kind: 'newer', release: RELEASE }))).toBeUndefined();
  });

  it('tells the person to restart an installed version when swcode runs in a terminal', () => {
    expect(updateNote(view({ kind: 'installed', release: RELEASE }, 'person'))?.text).toBe(
      'swcode 0.4.0 is installed: restart swcode for it to run.'
    );
    expect(updateNote(view({ kind: 'source' }))?.tone).toBe('info');
  });
});
