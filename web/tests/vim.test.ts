import { describe, expect, it } from 'vitest';

import { LESSON_ORDER, LESSONS, toolsOf, VIM_TEXTS } from '../src/lib/vim-lessons';
import { applyMotion, feedKey, NO_KEYS } from '../src/lib/vim-motions';
import { placeKey, routesFrom } from '../src/lib/vim-par';
import { chooseTarget, efficiency, newRound, pressKey, TARGETS_PER_ROUND } from '../src/lib/vim-round';

import type { Random } from '../src/types/game.types';
import type { Cursor, Motion, VimRound } from '../src/types/vim.types';

const steady: Random = () => 0.5;
const START: Cursor = { col: 0, row: 0, want: 0 };
// c0 o1 n2 s3 t4 _5 t6 o7 t8 a9 l10 _11 =12 _13 a14 d15 d16 (17 o18 n19 e20 ,21 _22 t23 w24 o25 )26 ;27
const TEXT = ['const total = add(one, two);', '', '  return total;'];

const move = (motion: Motion, from: Cursor = START, lastFind?: Parameters<typeof applyMotion>[3]) =>
  applyMotion(TEXT, from, motion, lastFind);

const placeOf = (cursor: Cursor): [number, number] => [cursor.row, cursor.col];

describe('the keys', () => {
  it('make a motion of a key the lesson knows', () => {
    expect(feedKey(NO_KEYS, 'l', toolsOf('steps'))).toEqual({ kind: 'motion', motion: { count: 0, kind: 'l' } });
  });

  it('refuse a motion of a later lesson', () => {
    expect(feedKey(NO_KEYS, 'w', toolsOf('steps'))).toEqual({ kind: 'refused' });
  });

  it('take a count before a motion once counts are taught', () => {
    const counted = feedKey(NO_KEYS, '3', toolsOf('counts'));
    expect(counted).toEqual({ kind: 'pending', pending: { count: '3' } });
    expect(counted.kind === 'pending' && feedKey(counted.pending, 'w', toolsOf('counts'))).toEqual({
      kind: 'motion',
      motion: { count: 3, kind: 'w' },
    });
  });

  it('wait for the character of f, and for the second g of gg', () => {
    const find = feedKey(NO_KEYS, 'f', toolsOf('find'));
    expect(find.kind === 'pending' && feedKey(find.pending, '(', toolsOf('find'))).toEqual({
      kind: 'motion',
      motion: { char: '(', count: 0, kind: 'f' },
    });
    const g = feedKey(NO_KEYS, 'g', toolsOf('jumps'));
    expect(g.kind === 'pending' && feedKey(g.pending, 'g', toolsOf('jumps'))).toEqual({
      kind: 'motion',
      motion: { count: 0, kind: 'gg' },
    });
  });
});

describe('the motions', () => {
  it('go by words: w to the next one, e to its end, b back', () => {
    expect(placeOf(move({ count: 0, kind: 'w' }).cursor)).toEqual([0, 6]);
    expect(placeOf(move({ count: 0, kind: 'e' }).cursor)).toEqual([0, 4]);
    expect(placeOf(move({ count: 0, kind: 'b' }, { col: 6, row: 0, want: 6 }).cursor)).toEqual([0, 0]);
    expect(placeOf(move({ count: 3, kind: 'w' }).cursor)).toEqual([0, 14]);
  });

  it('go to the ends of the line', () => {
    expect(placeOf(move({ count: 0, kind: '$' }).cursor)).toEqual([0, 27]);
    expect(placeOf(move({ count: 0, kind: '^' }, { col: 9, row: 2, want: 9 }).cursor)).toEqual([2, 2]);
    expect(placeOf(move({ count: 0, kind: '0' }, { col: 9, row: 2, want: 9 }).cursor)).toEqual([2, 0]);
  });

  it('find a character on the line, again with ; and back with ,', () => {
    expect(placeOf(move({ char: '(', count: 0, kind: 'f' }).cursor)).toEqual([0, 17]);
    expect(placeOf(move({ char: '(', count: 0, kind: 't' }).cursor)).toEqual([0, 16]);
    const found = move({ char: 'o', count: 0, kind: 'f' });
    expect(placeOf(found.cursor)).toEqual([0, 1]);
    const again = move({ count: 0, kind: ';' }, found.cursor, found.lastFind);
    expect(placeOf(again.cursor)).toEqual([0, 7]);
    expect(placeOf(move({ count: 0, kind: ',' }, again.cursor, again.lastFind).cursor)).toEqual([0, 1]);
  });

  it('jump to the matching bracket, the next empty line and the last line', () => {
    expect(placeOf(move({ count: 0, kind: '%' }).cursor)).toEqual([0, 26]);
    expect(placeOf(move({ count: 0, kind: '}' }).cursor)).toEqual([1, 0]);
    expect(placeOf(move({ count: 0, kind: '}' }, { col: 0, row: 1, want: 0 }).cursor)).toEqual([2, 14]);
    expect(placeOf(move({ count: 0, kind: '{' }, { col: 4, row: 2, want: 4 }).cursor)).toEqual([1, 0]);
    expect(placeOf(move({ count: 0, kind: 'G' }).cursor)).toEqual([2, 2]);
  });

  it('keep the column j aims for across a short line', () => {
    const across = move(
      { count: 0, kind: 'j' },
      move({ count: 0, kind: 'j' }, { col: 10, row: 0, want: 10 }).cursor
    );
    expect(placeOf(across.cursor)).toEqual([2, 10]);
  });
});

describe('the par', () => {
  const costTo = (row: number, col: number, lesson: Parameters<typeof toolsOf>[0]): number | undefined =>
    routesFrom(TEXT, START, toolsOf(lesson), LESSONS[lesson].par.max).get(placeKey(row, col))?.cost;

  it('counts the fewest keys to a place with the lesson’s motions', () => {
    expect(costTo(0, 3, 'steps')).toBe(3);
    expect(costTo(0, 6, 'words')).toBe(1);
  });

  it('goes down as a lesson brings motions that get there sooner', () => {
    expect(costTo(0, 17, 'words')).toBe(4);
    expect(costTo(0, 17, 'find')).toBe(2);
  });

  it('gives each lesson a target on the text, within the lesson’s par', () => {
    for (const lesson of LESSON_ORDER) {
      const lines = VIM_TEXTS[0] ?? [];
      const target = chooseTarget(lines, START, lesson, steady);
      expect(target.cost).toBeGreaterThan(0);
      expect(target.cost).toBeLessThanOrEqual(LESSONS[lesson].par.max);
      expect(lines[target.place.row]?.[target.place.col]).toMatch(/\S/);
    }
  });
});

describe('a round', () => {
  const playing = (target: VimRound['target']): VimRound => ({
    ...newRound('steps', steady),
    cursor: START,
    lines: TEXT,
    phase: 'playing',
    target,
  });
  const twoRight: VimRound['target'] = { cost: 2, moves: ['l', 'l'], place: { col: 2, row: 0 } };
  const press = (round: VimRound, keys: string): VimRound =>
    [...keys].reduce((each, key) => pressKey(each, key, steady), round);

  it('counts a target reached, its keys and its par, and brings the next', () => {
    const next = press(playing(twoRight), 'll');
    expect(next.reached).toBe(1);
    expect(next.last).toEqual({ keys: 2, par: twoRight });
    expect(next.totalKeys).toBe(2);
    expect(next.keys).toBe(0);
    expect(next.target).not.toEqual(twoRight);
  });

  it('says so of a key the lesson does not know, which costs nothing', () => {
    const next = press(playing(twoRight), 'w');
    expect(next.refused).toBe('w');
    expect(next.keys).toBe(0);
  });

  it('ends with its last target, its efficiency the par over the keys', () => {
    const last = { ...playing(twoRight), reached: TARGETS_PER_ROUND - 1, totalKeys: 6, totalPar: 6 };
    const over = press(last, 'hll');
    expect(over.phase).toBe('over');
    expect(efficiency(over)).toBe(Math.round((100 * 8) / 9));
  });

  it('takes no key before it starts', () => {
    const ready = newRound('steps', steady);
    expect(pressKey(ready, 'l', steady)).toBe(ready);
  });
});
