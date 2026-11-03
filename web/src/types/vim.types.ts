import type { GamePhase } from './game.types';

/** A place in the text: its line and its column, from 0. */
export interface Place {
  row: number;
  col: number;
}

/** Where the cursor is, and the column j and k aim for: vim keeps it across short lines, and `end` after $. */
export interface Cursor extends Place {
  want: number | 'end';
}

export type FindKind = 'f' | 'F' | 't' | 'T';

/** A motion without a character to find. */
export type PlainMotionKind =
  | 'h'
  | 'j'
  | 'k'
  | 'l'
  | 'w'
  | 'b'
  | 'e'
  | '0'
  | '^'
  | '$'
  | 'gg'
  | 'G'
  | '{'
  | '}'
  | '%'
  | ';'
  | ',';

export type MotionKind = PlainMotionKind | FindKind;

/** A motion as typed: its count (0 when none was typed), and for f, F, t and T the character it looks for. */
export type Motion = { kind: PlainMotionKind; count: number } | { kind: FindKind; count: number; char: string };

/** The last f, F, t or T, which ; repeats and , repeats the other way. */
export interface LastFind {
  kind: FindKind;
  char: string;
}

/** What has been typed toward a motion: a count, and a key waiting for the next one (f's character, g's g). */
export interface PendingKeys {
  count: string;
  awaiting?: FindKind | 'g';
}

/** A key read toward a motion: still more to come, a whole motion, or a key the lesson does not know. */
export type KeyResult =
  | { kind: 'pending'; pending: PendingKeys }
  | { kind: 'motion'; motion: Motion }
  | { kind: 'refused' };

/** The motions a lesson allows, and whether counts come before them. */
export interface Toolset {
  motions: ReadonlySet<MotionKind>;
  counts: boolean;
}

/** The fewest keys to a place: their count, and the keys of each motion on the way (`3w`, `fa;`). */
export interface Route {
  place: Place;
  cost: number;
  moves: string[];
}

export type LessonId = 'steps' | 'words' | 'line' | 'find' | 'counts' | 'jumps';

/** How the gutter numbers the lines: from the cursor's line (for counts), or from the top (for G). */
export type LineNumbers = 'relative' | 'absolute';

export interface Lesson {
  name: string;
  text: string;
  /** The motions it brings, on top of the lessons before it. */
  adds: readonly MotionKind[];
  /** Whether it brings counts. */
  counts: boolean;
  numbers: LineNumbers;
  /** How many keys its targets take at best. */
  par: { min: number; max: number };
}

/** How the last target went: the best way there, and the keys it took. */
export interface TargetResult {
  par: Route;
  keys: number;
}

/** A round of a lesson: the text, the cursor, the target and its par, and the targets reached so far. */
export interface VimRound {
  phase: GamePhase;
  lesson: LessonId;
  lines: readonly string[];
  cursor: Cursor;
  lastFind?: LastFind;
  pending: PendingKeys;
  target: Route;
  /** Keys typed toward the current target. */
  keys: number;
  reached: number;
  totalKeys: number;
  totalPar: number;
  last?: TargetResult;
  /** A key the lesson does not know, just typed. */
  refused?: string;
}
