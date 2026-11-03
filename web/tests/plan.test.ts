import { describe, expect, it } from 'vitest';

import { planProgress, runningStep } from '../src/lib/plan';

describe('planProgress', () => {
  it('counts the completed steps', () => {
    expect(
      planProgress([
        { content: 'Profile', status: 'completed' },
        { content: 'Fix', status: 'in_progress' },
      ])
    ).toBe('1 of 2 done');
  });
});

describe('runningStep', () => {
  it('finds the step in progress', () => {
    expect(
      runningStep([
        { content: 'Profile', status: 'completed' },
        { content: 'Fix', status: 'in_progress' },
        { content: 'Test', status: 'pending' },
      ])?.content
    ).toBe('Fix');
  });

  it('finds none when no step is in progress', () => {
    expect(runningStep([{ content: 'Profile', status: 'completed' }])).toBeUndefined();
  });
});
