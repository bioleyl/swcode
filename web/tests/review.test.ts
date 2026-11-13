import { describe, expect, it } from 'vitest';

import { canCommit, reviewDiffs, reviewSummary } from '../src/lib/review';

import type { ToolDisplay } from '../../shared/protocol/session.schema';

describe('a review of a workspace', () => {
  it('says its branch and how many files changed, with how many are shown when there are more', () => {
    expect(reviewSummary('main', 1, 0)).toBe('On main · 1 file changed');
    expect(reviewSummary('fix/login', 3, 0)).toBe('On fix/login · 3 files changed');
    expect(reviewSummary('main', 50, 3)).toBe('On main · 53 files changed, the first 50 shown');
    expect(reviewSummary('main', 0, 0)).toBe('On main · nothing to commit');
    expect(reviewSummary('', 2, 0)).toBe('No branch · 2 files changed');
  });

  it('draws the diffs among what the daemon sent, and nothing else', () => {
    const files: ToolDisplay[] = [
      { added: 1, kind: 'diff', lines: [], path: 'app.ts', removed: 0 },
      { kind: 'text', text: 'not a diff' },
    ];
    expect(reviewDiffs(files).map((diff) => diff.path)).toEqual(['app.ts']);
  });

  it('commits only something, with a message, one commit at a time', () => {
    expect(canCommit('Fix the login', 2, false)).toBe(true);
    expect(canCommit('   ', 2, false)).toBe(false);
    expect(canCommit('Fix the login', 0, false)).toBe(false);
    expect(canCommit('Fix the login', 2, true)).toBe(false);
  });
});
