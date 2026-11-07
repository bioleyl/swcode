import { confirmDialog } from '@syware/jlib';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { RESTART_QUESTION, restartsOnSave } from '../src/lib/settings-saved';
import { confirmRestart } from '../src/services/settings';

// The tests run without a DOM, which jlib's components need: the dialog alone stands in for it.
vi.mock('@syware/jlib', () => ({ confirmDialog: vi.fn() }));

const asked = vi.mocked(confirmDialog);

afterEach(() => asked.mockReset());

describe('restartsOnSave', () => {
  it('restarts under the sessions unless the person restarts swcode', () => {
    expect(restartsOnSave('self')).toBe(true);
    expect(restartsOnSave('person')).toBe(false);
  });

  it('counts on a restart while the settings are not loaded', () => {
    expect(restartsOnSave(undefined)).toBe(true);
  });
});

describe('confirmRestart', () => {
  it('asks before swcode restarts by itself, and goes on as the person answers', async () => {
    asked.mockResolvedValueOnce(false);
    expect(await confirmRestart('self')).toBe(false);
    asked.mockResolvedValueOnce(true);
    expect(await confirmRestart('self')).toBe(true);
    expect(asked).toHaveBeenCalledWith(RESTART_QUESTION);
  });

  it('goes on without asking when the person restarts swcode', async () => {
    expect(await confirmRestart('person')).toBe(true);
    expect(asked).not.toHaveBeenCalled();
  });
});
