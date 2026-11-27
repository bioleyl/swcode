import { describe, expect, it } from 'vitest';

import { gitLine } from '../src/lib/git-state';

import type { GitState } from '../src/types/git-state.types';

const CLEAN: GitState = {
  ahead: 0,
  behind: 0,
  branch: 'main',
  changed: 0,
  detached: false,
  follows: true,
  repository: true,
};

describe('what the header says of a workspace’s git state', () => {
  it('names the branch alone when nothing waits', () => {
    expect(gitLine(CLEAN)).toEqual({
      hint: 'On branch main, nothing uncommitted, level with the branch it follows.',
      text: 'main',
    });
  });

  it('counts what is uncommitted, ahead and behind', () => {
    expect(gitLine({ ...CLEAN, ahead: 2, behind: 1, branch: 'feature/git-info', changed: 3 })).toEqual({
      hint: 'On branch feature/git-info, 3 files uncommitted, 2 commits ahead of and 1 commit behind the branch it follows.',
      text: 'feature/git-info · 3 changed · ↑2 · ↓1',
    });
    expect(gitLine({ ...CLEAN, ahead: 1, changed: 1 })?.hint).toBe(
      'On branch main, 1 file uncommitted, 1 commit ahead of the branch it follows.'
    );
  });

  it('says a branch follows none, without counts', () => {
    expect(gitLine({ ...CLEAN, follows: false })).toEqual({
      hint: 'On branch main, nothing uncommitted, follows no branch.',
      text: 'main',
    });
  });

  it('names the commit of a workspace on no branch', () => {
    expect(gitLine({ ...CLEAN, branch: '4199495', detached: true, follows: false })).toEqual({
      hint: 'On no branch, at commit 4199495, nothing uncommitted, follows no branch.',
      text: 'detached 4199495',
    });
  });

  it('says a folder is in no repository', () => {
    expect(gitLine({ ...CLEAN, branch: '', follows: false, repository: false })).toEqual({
      hint: 'This folder is not in a git repository.',
      text: 'no git',
    });
  });

  it('says nothing when git could not answer', () => {
    expect(gitLine({ ...CLEAN, problem: 'git is not installed there.', repository: false })).toBeUndefined();
  });
});
