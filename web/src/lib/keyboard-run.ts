import { wpmOf } from './keyboard-letters';

import type { KeyboardRun, KeyOutcome, LineResult } from '../types/keyboard.types';

const LETTER = /[a-z]/;
// A longer wait than this between two keys is a pause, not typing: it is not timed.
const MAX_KEY_MS = 2_000;
const PERCENT = 100;

export const newRun = (line: string, next: string): KeyboardRun => ({
  at: 0,
  hits: 0,
  line,
  missed: false,
  misses: 0,
  next,
  timedKeys: 0,
  timedMs: 0,
});

/** The line typed whole: its speed over the keys timed, and its share of right keys. */
const resultOf = (run: KeyboardRun): LineResult => ({
  accuracy: Math.round((PERCENT * run.hits) / Math.max(1, run.hits + run.misses)),
  wpm: run.timedKeys === 0 ? 0 : Math.round(wpmOf(run.timedMs / run.timedKeys)),
});

/** The time since the key before, unless there was none or it was a pause. */
const timeSince = (run: KeyboardRun, now: number): number | undefined => {
  const ms = run.lastAt === undefined ? undefined : now - run.lastAt;
  return ms !== undefined && ms <= MAX_KEY_MS ? ms : undefined;
};

/** A key typed: the right one moves on and is timed, a wrong one marks the character until the right one comes. */
export function typeKey(run: KeyboardRun, key: string, now: number): KeyOutcome {
  const expected = run.line[run.at];
  if (expected === undefined) {
    return { run };
  }
  if (key !== expected) {
    return { run: { ...run, missed: true, misses: run.misses + 1 } };
  }
  const ms = timeSince(run, now);
  const typed: KeyboardRun = {
    ...run,
    at: run.at + 1,
    hits: run.hits + 1,
    lastAt: now,
    missed: false,
    timedKeys: run.timedKeys + (ms === undefined ? 0 : 1),
    timedMs: run.timedMs + (ms ?? 0),
  };
  return {
    finished: typed.at >= typed.line.length ? resultOf(typed) : undefined,
    run: typed,
    typed: ms !== undefined && LETTER.test(expected) ? { letter: expected, ms } : undefined,
  };
}

/** The next line comes up, a new one after it; the time goes on from the last key. */
export const nextLine = (run: KeyboardRun, next: string): KeyboardRun => ({
  ...newRun(run.next, next),
  lastAt: run.lastAt,
});

/** The wait while paused is not typing: the next key is not timed. */
export const pauseRun = (run: KeyboardRun): KeyboardRun => ({ ...run, lastAt: undefined });
