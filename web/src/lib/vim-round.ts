import { pickOne } from './random';
import { LESSONS, toolsBefore, toolsOf, VIM_TEXTS } from './vim-lessons';
import { applyMotion, feedKey, NO_KEYS } from './vim-motions';
import { placeKey, routesFrom } from './vim-par';

import type { Random } from '../types/game.types';
import type { Cursor, LessonId, Route, VimRound } from '../types/vim.types';

export const TARGETS_PER_ROUND = 10;
const BLANK = /\s/;
const START: Cursor = { col: 0, row: 0, want: 0 };
const PERCENT = 100;

const isOnText = (lines: readonly string[], route: Route): boolean => {
  const char = lines[route.place.row]?.[route.place.col];
  return char !== undefined && !BLANK.test(char);
};

/**
 * A target on the text that the lesson's motions reach within its par, preferably one they reach in fewer keys
 * than the lesson before could: its new keys then pay off.
 */
export function chooseTarget(lines: readonly string[], cursor: Cursor, lesson: LessonId, random: Random): Route {
  const { par } = LESSONS[lesson];
  const now = routesFrom(lines, cursor, toolsOf(lesson), par.max);
  const before = toolsBefore(lesson);
  const old = before ? routesFrom(lines, cursor, before, par.max) : undefined;
  const fair = [...now.values()].filter((route) => route.cost >= par.min && isOnText(lines, route));
  const better = fair.filter(
    (route) => (old?.get(placeKey(route.place.row, route.place.col))?.cost ?? Infinity) > route.cost
  );
  const anywhere = [...now.values()].filter((route) => route.cost > 0 && isOnText(lines, route));
  return (
    pickOne(better, random)
    ?? pickOne(fair, random)
    ?? pickOne(anywhere, random) ?? { cost: 0, moves: [], place: { col: cursor.col, row: cursor.row } }
  );
}

/** A round of a lesson on one of the texts, the cursor at its start, on its title screen. */
export function newRound(lesson: LessonId, random: Random): VimRound {
  const lines = pickOne(VIM_TEXTS, random) ?? [];
  return {
    cursor: START,
    keys: 0,
    lesson,
    lines,
    pending: NO_KEYS,
    phase: 'ready',
    reached: 0,
    target: chooseTarget(lines, START, lesson, random),
    totalKeys: 0,
    totalPar: 0,
  };
}

/** The start button: the title screen's or a paused round goes on, a finished one starts again. */
export const startRound = (round: VimRound, random: Random): VimRound =>
  round.phase === 'over'
    ? { ...newRound(round.lesson, random), phase: 'playing' }
    : { ...round, phase: 'playing' };

export const pauseRound = (round: VimRound): VimRound =>
  round.phase === 'playing' ? { ...round, phase: 'paused' } : round;

/** Escape: what was typed toward a motion goes. */
export const clearPending = (round: VimRound): VimRound => ({ ...round, pending: NO_KEYS, refused: undefined });

export const hasPending = (round: VimRound): boolean =>
  round.pending.count !== '' || round.pending.awaiting !== undefined;

/** How close to par the round's keys came, in percent. */
export const efficiency = (round: Pick<VimRound, 'totalKeys' | 'totalPar'>): number =>
  round.totalKeys === 0 ? 0 : Math.min(PERCENT, Math.round((PERCENT * round.totalPar) / round.totalKeys));

const onTarget = (round: VimRound): boolean =>
  round.cursor.row === round.target.place.row && round.cursor.col === round.target.place.col;

/** The target reached: it counts, and the next one comes, or the round ends with its last. */
function reach(round: VimRound, random: Random): VimRound {
  const reached = round.reached + 1;
  const counted: VimRound = {
    ...round,
    keys: 0,
    last: { keys: round.keys, par: round.target },
    reached,
    totalKeys: round.totalKeys + round.keys,
    totalPar: round.totalPar + round.target.cost,
  };
  return reached >= TARGETS_PER_ROUND
    ? { ...counted, phase: 'over' }
    : { ...counted, target: chooseTarget(round.lines, round.cursor, round.lesson, random) };
}

/** A key typed in a running round: toward a motion, a motion that moves the cursor, or one the lesson lacks. */
export function pressKey(round: VimRound, key: string, random: Random): VimRound {
  if (round.phase !== 'playing') {
    return round;
  }
  const result = feedKey(round.pending, key, toolsOf(round.lesson));
  if (result.kind === 'refused') {
    return { ...round, pending: NO_KEYS, refused: key };
  }
  const typed = { ...round, keys: round.keys + 1, refused: undefined };
  if (result.kind === 'pending') {
    return { ...typed, pending: result.pending };
  }
  const moved = applyMotion(round.lines, round.cursor, result.motion, round.lastFind);
  const next = { ...typed, cursor: moved.cursor, lastFind: moved.lastFind, pending: NO_KEYS };
  return onTarget(next) ? reach(next, random) : next;
}
