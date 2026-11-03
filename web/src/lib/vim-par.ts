import { applyMotion, FIND_KINDS } from './vim-motions';

import type { Cursor, Motion, PlainMotionKind, Route, Toolset } from '../types/vim.types';

/** A motion to try, and the keys it takes. */
interface Move {
  motion: Motion;
  keys: string;
}

/** The cheapest way found so far to a state of the cursor. */
interface Reached {
  cost: number;
  moves: string[];
}

// With a count, these go once more from where the count before left the cursor: 3w is w from where 2w landed.
const CHAINED_KINDS: readonly PlainMotionKind[] = ['h', 'j', 'k', 'l', 'w', 'b', 'e', '{', '}'];
// These go once from the cursor: 0 takes no count, $ and G take one their own way.
const DIRECT_KINDS: readonly PlainMotionKind[] = ['0', '^', '$', 'gg', 'G', '%'];
const LINE_KINDS: ReadonlySet<PlainMotionKind> = new Set(['j', 'k']);
const COUNTS = { first: 2, last: 9 } as const;
const REPEAT_KEY = ';';
const BLANK = /\s/;

const range = (first: number, last: number): number[] =>
  Array.from({ length: Math.max(0, last - first + 1) }, (_, index) => first + index);

const stateKey = (cursor: Cursor): string => `${cursor.row},${cursor.col},${cursor.want}`;

/** A place as the routes are keyed by. */
export const placeKey = (row: number, col: number): string => `${row},${col}`;

const keysOf = (kind: PlainMotionKind, count: number): string => (count === 1 ? kind : `${count}${kind}`);

/** The highest count worth trying: none without counts, 9, or past it for j and k, as many as the text has lines. */
const highestCount = (kind: PlainMotionKind, lines: readonly string[], tools: Toolset): number => {
  if (!tools.counts) {
    return 1;
  }
  return LINE_KINDS.has(kind) ? Math.max(COUNTS.last, lines.length) : COUNTS.last;
};

/** The motions tried once from the cursor, with their counts: n$ goes n - 1 lines down, nG to line n. */
function directMoves(lines: readonly string[], tools: Toolset): Move[] {
  const allowed = (kind: PlainMotionKind) => tools.motions.has(kind);
  const simple = DIRECT_KINDS.filter(allowed).map((kind) => ({ keys: kind, motion: { count: 0, kind } }));
  if (!tools.counts) {
    return simple;
  }
  const ends = allowed('$')
    ? range(COUNTS.first, COUNTS.last).map((count) => ({
        keys: `${count}$`,
        motion: { count, kind: '$' as const },
      }))
    : [];
  const jumps = allowed('G')
    ? range(1, lines.length).map((count) => ({ keys: `${count}G`, motion: { count, kind: 'G' as const } }))
    : [];
  return [...simple, ...ends, ...jumps];
}

/** f, F, t and T to each character of a line, the nth one with ; repeated or with a count. */
function findMoves(line: string, tools: Toolset): Move[] {
  const chars = [...new Set(line)].filter((char) => !BLANK.test(char));
  return FIND_KINDS.filter((kind) => tools.motions.has(kind)).flatMap((kind) =>
    chars.flatMap((char) => {
      const occurrences = [...line].filter((each) => each === char).length;
      return range(1, Math.min(occurrences, COUNTS.last)).flatMap((count) => {
        const motion = { char, count, kind };
        const repeated = tools.motions.has(REPEAT_KEY) && count > 1;
        const counted = tools.counts && count > 1;
        return [
          ...(count === 1 ? [{ keys: `${kind}${char}`, motion }] : []),
          ...(repeated ? [{ keys: `${kind}${char}${REPEAT_KEY.repeat(count - 1)}`, motion }] : []),
          ...(counted ? [{ keys: `${count}${kind}${char}`, motion }] : []),
        ];
      });
    })
  );
}

/**
 * The fewest keys from the cursor to each place of the text, with the lesson's motions, up to `maxCost`: a
 * search over the cursor's states (its place and the column j and k aim for), a key costing one.
 */
export function routesFrom(
  lines: readonly string[],
  from: Cursor,
  tools: Toolset,
  maxCost: number
): Map<string, Route> {
  const chained = CHAINED_KINDS.filter((kind) => tools.motions.has(kind));
  const direct = directMoves(lines, tools);
  const finds = lines.map((line) => findMoves(line, tools));
  const best = new Map<string, Reached>([[stateKey(from), { cost: 0, moves: [] }]]);
  const buckets: Cursor[][] = [[from]];
  const routes = new Map<string, Route>();

  /** Queues a state reached in fewer keys than before; false when the keys cost too much to try. */
  const offer = (next: Cursor, reached: Reached, keys: string): boolean => {
    const cost = reached.cost + keys.length;
    if (cost > maxCost) {
      return false;
    }
    const key = stateKey(next);
    const known = best.get(key);
    if (known && known.cost <= cost) {
      return true;
    }
    best.set(key, { cost, moves: [...reached.moves, keys] });
    const bucket = buckets[cost] ?? [];
    bucket.push(next);
    buckets[cost] = bucket;
    return true;
  };

  /** A chained motion with each count in turn, until it no longer moves or costs too much. */
  const offerChain = (cursor: Cursor, reached: Reached, kind: PlainMotionKind): void => {
    let previous = cursor;
    for (let count = 1; count <= highestCount(kind, lines, tools); count++) {
      const next = applyMotion(lines, previous, { count: 0, kind }).cursor;
      if (stateKey(next) === stateKey(previous) || !offer(next, reached, keysOf(kind, count))) {
        return;
      }
      previous = next;
    }
  };

  for (let cost = 0; cost <= maxCost && cost < buckets.length; cost++) {
    for (const cursor of buckets[cost] ?? []) {
      const reached = best.get(stateKey(cursor));
      // Reached again more cheaply since it was queued.
      if (!reached || reached.cost !== cost) {
        continue;
      }
      const place = placeKey(cursor.row, cursor.col);
      if (!routes.has(place)) {
        routes.set(place, { cost, moves: reached.moves, place: { col: cursor.col, row: cursor.row } });
      }
      for (const kind of chained) {
        offerChain(cursor, reached, kind);
      }
      for (const moves of [direct, finds[cursor.row] ?? []]) {
        for (const move of moves) {
          offer(applyMotion(lines, cursor, move.motion).cursor, reached, move.keys);
        }
      }
    }
  }
  return routes;
}
