import { describe, expect, it } from 'vitest';

import { isSwcodeBaseIn } from '../src/lib/swcode-base';

import type { SwcodeBaseStatus } from '../../shared/protocol/skill.schema';

const status = (inHome: boolean, inProject: boolean): SwcodeBaseStatus => ({
  carried: true,
  inHome,
  inProject,
  installed: inHome || inProject,
  names: ['swcode-base'],
});

describe('isSwcodeBaseIn', () => {
  it('reads each place on its own', () => {
    expect(isSwcodeBaseIn(status(true, false), 'home')).toBe(true);
    expect(isSwcodeBaseIn(status(true, false), 'project')).toBe(false);
    expect(isSwcodeBaseIn(status(false, true), 'project')).toBe(true);
    expect(isSwcodeBaseIn(status(false, true), 'home')).toBe(false);
  });
});
