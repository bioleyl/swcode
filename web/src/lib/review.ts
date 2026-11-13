import type { DiffDisplay, ToolDisplay } from '../../../shared/protocol/session.schema';

const ONE = 1;
const NO_BRANCH = 'No branch';
const CLEAN = 'nothing to commit';

const filesText = (count: number): string => (count === ONE ? '1 file changed' : `${count} files changed`);

/** The diffs among what the daemon sent: a review holds nothing else. */
export const reviewDiffs = (files: readonly ToolDisplay[]): DiffDisplay[] =>
  files.filter((file): file is DiffDisplay => file.kind === 'diff');

/**
 * What a review says under its heading: the branch, then how many files changed, with how many of them are shown
 * when there are more than the daemon sends.
 */
export const reviewSummary = (branch: string, shown: number, more: number): string => {
  const place = branch === '' ? NO_BRANCH : `On ${branch}`;
  if (shown === 0) {
    return `${place} · ${CLEAN}`;
  }
  const changed = filesText(shown + more);
  return more > 0 ? `${place} · ${changed}, the first ${shown} shown` : `${place} · ${changed}`;
};

/** Whether the Commit button can be pressed: there is something to commit, a message for it, and no commit under way. */
export const canCommit = (message: string, changed: number, committing: boolean): boolean =>
  !committing && changed > 0 && message.trim() !== '';
