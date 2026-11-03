/** biome-ignore-all lint/style/useNamingConvention: the motions are keyed by their keys, and a capital (F, G) is a motion of its own */
import type {
  Cursor,
  FindKind,
  KeyResult,
  LastFind,
  Motion,
  MotionKind,
  PendingKeys,
  Place,
  PlainMotionKind,
  Toolset,
} from '../types/vim.types';

/** How vim sorts characters into words: letters, digits and _ make one, other signs another, blanks part them. */
type CharClass = 'blank' | 'word' | 'punct' | 'empty';

const WORD_CHAR = /[A-Za-z0-9_]/;
const BLANK = /\s/;

export const FIND_KINDS: readonly FindKind[] = ['f', 'F', 't', 'T'];
const PLAIN_KEYS: ReadonlySet<string> = new Set<PlainMotionKind>([
  'h',
  'j',
  'k',
  'l',
  'w',
  'b',
  'e',
  '0',
  '^',
  '$',
  'G',
  '{',
  '}',
  '%',
  ';',
  ',',
]);
const COUNT_START = /[1-9]/;
const DIGIT = /[0-9]/;
const G_KEY = 'g';

// The way , turns the last find around.
const REVERSED: Readonly<Record<FindKind, FindKind>> = { F: 'f', f: 'F', T: 't', t: 'T' };

const BRACKETS: Readonly<Record<string, { match: string; step: 1 | -1 }>> = {
  '(': { match: ')', step: 1 },
  ')': { match: '(', step: -1 },
  '[': { match: ']', step: 1 },
  ']': { match: '[', step: -1 },
  '{': { match: '}', step: 1 },
  '}': { match: '{', step: -1 },
};

export const NO_KEYS: PendingKeys = { count: '' };

const lineAt = (lines: readonly string[], row: number): string => lines[row] ?? '';
const lastRow = (lines: readonly string[]): number => Math.max(0, lines.length - 1);
export const lastCol = (lines: readonly string[], row: number): number =>
  Math.max(0, lineAt(lines, row).length - 1);
const clamp = (value: number, low: number, high: number): number => Math.min(high, Math.max(low, value));

const classOf = (char: string): CharClass => {
  if (BLANK.test(char)) {
    return 'blank';
  }
  return WORD_CHAR.test(char) ? 'word' : 'punct';
};

const classAt = (lines: readonly string[], place: Place): CharClass => {
  const line = lineAt(lines, place.row);
  return line === '' ? 'empty' : classOf(line[place.col] ?? ' ');
};

const isInWord = (kind: CharClass): boolean => kind === 'word' || kind === 'punct';

/** The next place in the text, over line ends; an empty line has one, its column 0. */
const forward = (lines: readonly string[], place: Place): Place | undefined => {
  if (place.col < lastCol(lines, place.row)) {
    return { col: place.col + 1, row: place.row };
  }
  return place.row < lastRow(lines) ? { col: 0, row: place.row + 1 } : undefined;
};

const backward = (lines: readonly string[], place: Place): Place | undefined => {
  if (place.col > 0) {
    return { col: place.col - 1, row: place.row };
  }
  return place.row > 0 ? { col: lastCol(lines, place.row - 1), row: place.row - 1 } : undefined;
};

/** How far along its line the run of one class goes from the place, one way (`step` 1) or the other (-1). */
const runEdge = (lines: readonly string[], place: Place, step: 1 | -1): Place => {
  const kind = classAt(lines, place);
  const line = lineAt(lines, place.row);
  let col = place.col;
  while (line[col + step] !== undefined && classOf(line[col + step] ?? '') === kind) {
    col += step;
  }
  return { col, row: place.row };
};

/** The last place of the run of one class the place is in, on its line. */
const endOfRun = (lines: readonly string[], place: Place): Place => runEdge(lines, place, 1);

const startOfRun = (lines: readonly string[], place: Place): Place => runEdge(lines, place, -1);

const lastPlace = (lines: readonly string[]): Place => ({
  col: lastCol(lines, lastRow(lines)),
  row: lastRow(lines),
});

/** w: the start of the next word, an empty line counting as one. */
const wordForward = (lines: readonly string[], from: Place): Place => {
  const left = isInWord(classAt(lines, from)) ? endOfRun(lines, from) : from;
  for (let next = forward(lines, left); next; next = forward(lines, next)) {
    if (classAt(lines, next) !== 'blank') {
      return next;
    }
  }
  return lastPlace(lines);
};

/** e: the end of the word, or of the next one when already there; empty lines are passed. */
const wordEnd = (lines: readonly string[], from: Place): Place => {
  for (let next = forward(lines, from); next; next = forward(lines, next)) {
    if (isInWord(classAt(lines, next))) {
      return endOfRun(lines, next);
    }
  }
  return lastPlace(lines);
};

/** b: the start of the word, or of the one before when already there; it stops on an empty line. */
const wordBack = (lines: readonly string[], from: Place): Place => {
  for (let previous = backward(lines, from); previous; previous = backward(lines, previous)) {
    const kind = classAt(lines, previous);
    if (kind === 'empty') {
      return previous;
    }
    if (kind !== 'blank') {
      return startOfRun(lines, previous);
    }
  }
  return { col: 0, row: 0 };
};

/** }: the next empty line past some text, the cursor's line included, or the end of the text. */
const paragraphForward = (lines: readonly string[], row: number): Place => {
  const text = lines.findIndex((line, index) => index >= row && line !== '');
  const empty = text === -1 ? -1 : lines.findIndex((line, index) => index > text && line === '');
  return empty === -1 ? lastPlace(lines) : { col: 0, row: empty };
};

const paragraphBack = (lines: readonly string[], row: number): Place => {
  const text = lines.findLastIndex((line, index) => index <= row && line !== '');
  const empty = text === -1 ? -1 : lines.findLastIndex((line, index) => index < text && line === '');
  return { col: 0, row: Math.max(0, empty) };
};

const firstNonBlank = (lines: readonly string[], row: number): number => {
  const found = [...lineAt(lines, row)].findIndex((char) => !BLANK.test(char));
  return found === -1 ? lastCol(lines, row) : found;
};

// The columns of each character of a line, asked many times by the search for a target's par: the texts are few.
const columnsCache = new Map<string, Map<string, number[]>>();

const columnsOf = (line: string, char: string): readonly number[] => {
  const known = columnsCache.get(line);
  if (known) {
    return known.get(char) ?? [];
  }
  const columns = new Map<string, number[]>();
  [...line].forEach((each, index) => {
    columns.set(each, [...(columns.get(each) ?? []), index]);
  });
  columnsCache.set(line, columns);
  return columns.get(char) ?? [];
};

/**
 * Where f, F, t or T lands: on (f, F) or next to (t, T) the `nth` occurrence of the character, after the cursor
 * (f, t) or before it (F, T). Repeated with ; (`again`), t and T look past the character they stand next to.
 */
const findColumn = (
  line: string,
  col: number,
  kind: FindKind,
  char: string,
  nth: number,
  again: boolean
): number | undefined => {
  const till = kind === 't' || kind === 'T';
  const skip = till && again ? 1 : 0;
  const ahead = kind === 'f' || kind === 't';
  const columns = columnsOf(line, char);
  const first = columns.findIndex((index) => index > col + skip);
  const last = columns.findLastIndex((index) => index < col - skip);
  const found = ahead ? (first === -1 ? undefined : columns[first + nth - 1]) : columns[last - nth + 1];
  if (found === undefined) {
    return undefined;
  }
  return till ? found - (ahead ? 1 : -1) : found;
};

/** The bracket that matches the first one at or after the cursor on its line. */
const matchingBracket = (lines: readonly string[], from: Place): Place | undefined => {
  const line = lineAt(lines, from.row);
  const offset = [...line.slice(from.col)].findIndex((char) => BRACKETS[char] !== undefined);
  if (offset === -1) {
    return undefined;
  }
  const start = { col: from.col + offset, row: from.row };
  const open = line[start.col] ?? '';
  const rule = BRACKETS[open];
  if (!rule) {
    return undefined;
  }
  const step = rule.step === 1 ? forward : backward;
  let depth = 0;
  for (let place = step(lines, start); place; place = step(lines, place)) {
    const char = lineAt(lines, place.row)[place.col];
    depth += char === open ? 1 : 0;
    if (char === rule.match && depth === 0) {
      return place;
    }
    depth -= char === rule.match ? 1 : 0;
  }
  return undefined;
};

const repeat = (times: number, move: (place: Place) => Place, from: Place): Place =>
  Array.from({ length: times }).reduce<Place>((place) => move(place), from);

const at = (place: Place): Cursor => ({ ...place, want: place.col });

const columnFor = (lines: readonly string[], row: number, want: Cursor['want']): number =>
  want === 'end' ? lastCol(lines, row) : Math.min(want, lastCol(lines, row));

const vertical = (lines: readonly string[], cursor: Cursor, rows: number): Cursor => {
  const row = clamp(cursor.row + rows, 0, lastRow(lines));
  return { col: columnFor(lines, row, cursor.want), row, want: cursor.want };
};

const lineJump = (lines: readonly string[], count: number, fallback: number): Cursor => {
  const row = clamp(count > 0 ? count - 1 : fallback, 0, lastRow(lines));
  return at({ col: firstNonBlank(lines, row), row });
};

type PlainMoves = Readonly<
  Record<
    Exclude<PlainMotionKind, ';' | ','>,
    (lines: readonly string[], cursor: Cursor, times: number, count: number) => Cursor
  >
>;

const PLAIN_MOVES: PlainMoves = {
  '{': (lines, cursor, times) => at(repeat(times, (place) => paragraphBack(lines, place.row), cursor)),
  '}': (lines, cursor, times) => at(repeat(times, (place) => paragraphForward(lines, place.row), cursor)),
  '%': (lines, cursor) => {
    const place = matchingBracket(lines, cursor);
    return place ? at(place) : cursor;
  },
  '^': (lines, cursor) => at({ col: firstNonBlank(lines, cursor.row), row: cursor.row }),
  $: (lines, cursor, times) => {
    const row = clamp(cursor.row + times - 1, 0, lastRow(lines));
    return { col: lastCol(lines, row), row, want: 'end' };
  },
  '0': (_lines, cursor) => at({ col: 0, row: cursor.row }),
  b: (lines, cursor, times) => at(repeat(times, (place) => wordBack(lines, place), cursor)),
  e: (lines, cursor, times) => at(repeat(times, (place) => wordEnd(lines, place), cursor)),
  G: (lines, _cursor, _times, count) => lineJump(lines, count, lastRow(lines)),
  gg: (lines, _cursor, _times, count) => lineJump(lines, count, 0),
  h: (_lines, cursor, times) => at({ col: Math.max(0, cursor.col - times), row: cursor.row }),
  j: (lines, cursor, times) => vertical(lines, cursor, times),
  k: (lines, cursor, times) => vertical(lines, cursor, -times),
  l: (lines, cursor, times) =>
    at({ col: Math.min(lastCol(lines, cursor.row), cursor.col + times), row: cursor.row }),
  w: (lines, cursor, times) => at(repeat(times, (place) => wordForward(lines, place), cursor)),
};

const find = (lines: readonly string[], cursor: Cursor, last: LastFind, times: number, again: boolean): Cursor => {
  const col = findColumn(lineAt(lines, cursor.row), cursor.col, last.kind, last.char, times, again);
  return col === undefined ? cursor : at({ col, row: cursor.row });
};

/** The repeat of the last find, the same way (;) or the other (,); nothing before any find. */
const findAgain = (
  lines: readonly string[],
  cursor: Cursor,
  last: LastFind | undefined,
  times: number,
  reverse: boolean
) => {
  if (!last) {
    return { cursor };
  }
  const kind = reverse ? REVERSED[last.kind] : last.kind;
  return { cursor: find(lines, cursor, { char: last.char, kind }, times, true), lastFind: last };
};

/** Where a motion takes the cursor, and the find that ; and , repeat after it. */
export function applyMotion(
  lines: readonly string[],
  cursor: Cursor,
  motion: Motion,
  lastFind?: LastFind
): { cursor: Cursor; lastFind?: LastFind } {
  const times = Math.max(1, motion.count);
  if (motion.kind === 'f' || motion.kind === 'F' || motion.kind === 't' || motion.kind === 'T') {
    const last = { char: motion.char, kind: motion.kind };
    return { cursor: find(lines, cursor, last, times, false), lastFind: last };
  }
  if (motion.kind === ';' || motion.kind === ',') {
    return findAgain(lines, cursor, lastFind, times, motion.kind === ',');
  }
  return { cursor: PLAIN_MOVES[motion.kind](lines, cursor, times, motion.count), lastFind };
}

const countOf = (pending: PendingKeys): number => (pending.count === '' ? 0 : Number(pending.count));

const isFindKind = (key: string): key is FindKind => FIND_KINDS.some((kind) => kind === key);
const isPlainKey = (key: string): key is PlainMotionKind => PLAIN_KEYS.has(key);

/** Whether a key goes on a count: 1 to 9 start one, 0 only goes on one (alone, it is a motion). */
const extendsCount = (pending: PendingKeys, key: string, tools: Toolset): boolean =>
  tools.counts && (COUNT_START.test(key) || (DIGIT.test(key) && pending.count !== ''));

const allows = (tools: Toolset, kind: MotionKind): boolean => tools.motions.has(kind);

/** Reads one more key toward a motion, as vim does in normal mode, with the lesson's motions only. */
export function feedKey(pending: PendingKeys, key: string, tools: Toolset): KeyResult {
  const count = countOf(pending);
  if (pending.awaiting === 'g') {
    return key === G_KEY ? { kind: 'motion', motion: { count, kind: 'gg' } } : { kind: 'refused' };
  }
  if (pending.awaiting) {
    return { kind: 'motion', motion: { char: key, count, kind: pending.awaiting } };
  }
  if (extendsCount(pending, key, tools)) {
    return { kind: 'pending', pending: { count: `${pending.count}${key}` } };
  }
  if (key === G_KEY && allows(tools, 'gg')) {
    return { kind: 'pending', pending: { awaiting: 'g', count: pending.count } };
  }
  if (isFindKind(key) && allows(tools, key)) {
    return { kind: 'pending', pending: { awaiting: key, count: pending.count } };
  }
  return isPlainKey(key) && allows(tools, key)
    ? { kind: 'motion', motion: { count, kind: key } }
    : { kind: 'refused' };
}
