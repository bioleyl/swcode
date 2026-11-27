import type { GitLine, GitState } from '../types/git-state.types';

const NO_REPOSITORY: GitLine = { hint: 'This folder is not in a git repository.', text: 'no git' };
const SEPARATOR = ' · ';
const AHEAD_MARK = '↑';
const BEHIND_MARK = '↓';

const counted = (count: number, one: string, many: string): string => `${count} ${count === 1 ? one : many}`;

/** The branch, or the commit a workspace on no branch is at. */
const placeOf = (state: GitState): string => (state.detached ? `detached ${state.branch}`.trim() : state.branch);

const placeHint = (state: GitState): string =>
  state.detached ? `On no branch, at commit ${state.branch}`.trim() : `On branch ${state.branch}`;

const changedHint = (changed: number): string =>
  changed === 0 ? 'nothing uncommitted' : `${counted(changed, 'file', 'files')} uncommitted`;

/** How far the branch is from the one it follows, as git last fetched it. */
function followHint(state: GitState): string {
  if (!state.follows) {
    return 'follows no branch';
  }
  if (state.ahead === 0 && state.behind === 0) {
    return 'level with the branch it follows';
  }
  const sides = [
    state.ahead > 0 ? `${counted(state.ahead, 'commit', 'commits')} ahead of` : '',
    state.behind > 0 ? `${counted(state.behind, 'commit', 'commits')} behind` : '',
  ].filter((side) => side !== '');
  return `${sides.join(' and ')} the branch it follows`;
}

/**
 * What the header says of a workspace's git state: the branch, how many files are uncommitted and how many commits
 * it is ahead (↑) and behind (↓), each only when there is something to say; nothing when git could not answer.
 */
export function gitLine(state: GitState): GitLine | undefined {
  if (state.problem) {
    return undefined;
  }
  if (!state.repository) {
    return NO_REPOSITORY;
  }
  const parts = [
    placeOf(state),
    state.changed > 0 ? `${state.changed} changed` : '',
    state.follows && state.ahead > 0 ? `${AHEAD_MARK}${state.ahead}` : '',
    state.follows && state.behind > 0 ? `${BEHIND_MARK}${state.behind}` : '',
  ].filter((part) => part !== '');
  return {
    hint: `${[placeHint(state), changedHint(state.changed), followHint(state)].join(', ')}.`,
    text: parts.join(SEPARATOR),
  };
}
