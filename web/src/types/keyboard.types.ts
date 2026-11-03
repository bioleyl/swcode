import type { GamePhase } from './game.types';

/** The keyboard game runs on without end: it is never over. */
export type KeyboardPhase = Exclude<GamePhase, 'over'>;

/** How fast one letter is typed: the time before it, averaged over its last samples, and how many it had. */
export interface LetterStats {
  samples: number;
  averageMs: number;
}

/** Where people are: how many letters they have (in `LETTER_ORDER`), the speed each must reach, and each one's stats. */
export interface KeyboardProgress {
  unlocked: number;
  targetWpm: number;
  letters: Partial<Record<string, LetterStats>>;
}

/** How words go: after each letter, and after a word's edge, how often each letter or the edge comes next. */
export interface WordModel {
  follows: ReadonlyMap<string, ReadonlyMap<string, number>>;
}

/** The line to type, the one after it, and how it goes. */
export interface KeyboardRun {
  line: string;
  next: string;
  /** The character to type next. */
  at: number;
  /** Whether a wrong key was typed for it: it stays marked until the right one. */
  missed: boolean;
  /** When the last right key was typed: none before the first, nor after a pause. */
  lastAt?: number;
  /** The time between right keys, and how many were timed: the line's speed. */
  timedMs: number;
  timedKeys: number;
  hits: number;
  misses: number;
}

/** A letter typed right, and the time since the key before it. */
export interface TypedLetter {
  letter: string;
  ms: number;
}

/** How a line went. */
export interface LineResult {
  wpm: number;
  accuracy: number;
}

/** What a key did: the run after it, the letter it typed right if it was timed, and the line if it finished one. */
export interface KeyOutcome {
  run: KeyboardRun;
  typed?: TypedLetter;
  finished?: LineResult;
}

/** Where a letter stands: not given yet, given but slower than the target, or at the target. */
export type LetterState = 'locked' | 'learning' | 'confident';

/** A letter as the row of letters shows it: its state, whether it is the one in focus, and its speed. */
export interface LetterView {
  letter: string;
  state: LetterState;
  focus: boolean;
  wpm?: number;
}
