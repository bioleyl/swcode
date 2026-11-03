import type { Lesson, LessonId, MotionKind, Toolset } from '../types/vim.types';

/** What each lesson brings: a few motions, on top of those of the lessons before it (`LESSON_ORDER`). */
export const LESSONS: Readonly<Record<LessonId, Lesson>> = {
  counts: {
    adds: [],
    counts: true,
    name: 'Counts: 3w 5j',
    numbers: 'relative',
    par: { max: 6, min: 2 },
    text: 'A number before a motion repeats it: 3w goes three words on, 5j five lines down, 2fa to the second a. The gutter counts lines from the cursor.',
  },
  find: {
    adds: ['f', 'F', 't', 'T', ';', ','],
    counts: false,
    name: 'Find: f t F T ; ,',
    numbers: 'relative',
    par: { max: 6, min: 2 },
    text: 'f and a character go to that character further on the line, t to just before it; F and T look back. ; finds again, , the other way.',
  },
  jumps: {
    adds: ['gg', 'G', '{', '}', '%'],
    counts: false,
    name: 'Far: gg G { } %',
    numbers: 'absolute',
    par: { max: 6, min: 2 },
    text: 'gg goes to the first line, G to the last and 12G to line 12; } and { to the next and the previous empty line; % to the matching bracket.',
  },
  line: {
    adds: ['0', '^', '$'],
    counts: false,
    name: 'Line ends: 0 ^ $',
    numbers: 'relative',
    par: { max: 6, min: 2 },
    text: '0 goes to the start of the line, ^ to its first character that is not a space, $ to its end.',
  },
  steps: {
    adds: ['h', 'j', 'k', 'l'],
    counts: false,
    name: 'Steps: h j k l',
    numbers: 'relative',
    par: { max: 8, min: 3 },
    text: 'h goes left, j down, k up and l right: a character or a line at a time.',
  },
  words: {
    adds: ['w', 'b', 'e'],
    counts: false,
    name: 'Words: w b e',
    numbers: 'relative',
    par: { max: 6, min: 2 },
    text: 'w goes to the next word, b back to the start of one, e to its end. A run of signs is a word too.',
  },
};

/** The lessons in the order they come. */
export const LESSON_ORDER = [
  'steps',
  'words',
  'line',
  'find',
  'counts',
  'jumps',
] as const satisfies readonly LessonId[];

/** The texts the cursor moves in: a few lines of code, with empty lines and brackets for the far motions. */
export const VIM_TEXTS: readonly (readonly string[])[] = [
  [
    "import { readFile } from 'node:fs/promises';",
    '',
    'const SEPARATOR = "=";',
    '',
    'export async function loadConfig(path: string) {',
    "  const text = await readFile(path, 'utf8');",
    '  const lines = text.split("\\n").filter(Boolean);',
    '  return Object.fromEntries(lines.map(parse));',
    '}',
    '',
    'function parse(line: string): [string, string] {',
    '  const [key, ...rest] = line.split(SEPARATOR);',
    '  return [key.trim(), rest.join(SEPARATOR).trim()];',
    '}',
  ],
  [
    "type Shape = { kind: 'circle'; radius: number }",
    "  | { kind: 'square'; side: number };",
    '',
    'const AREA = {',
    '  circle: (shape) => Math.PI * shape.radius ** 2,',
    '  square: (shape) => shape.side * shape.side,',
    '};',
    '',
    'export function totalArea(shapes: Shape[]) {',
    '  let sum = 0;',
    '  for (const shape of shapes) {',
    '    sum += AREA[shape.kind](shape);',
    '  }',
    '  return Math.round(sum * 100) / 100;',
    '}',
  ],
  [
    'export class Queue<Item> {',
    '  private items: Item[] = [];',
    '',
    '  push(item: Item): void {',
    '    this.items.push(item);',
    '  }',
    '',
    '  pop(): Item | undefined {',
    '    return this.items.shift();',
    '  }',
    '',
    '  get size(): number {',
    '    return this.items.length;',
    '  }',
    '}',
  ],
];

const indexOf = (id: LessonId): number => LESSON_ORDER.indexOf(id);

/** What the lessons up to the `index`th allow. */
const toolsUpTo = (index: number): Toolset => {
  const taken = LESSON_ORDER.slice(0, index + 1).map((id) => LESSONS[id]);
  return {
    counts: taken.some((lesson) => lesson.counts),
    motions: new Set<MotionKind>(taken.flatMap((lesson) => lesson.adds)),
  };
};

/** The motions a lesson allows: its own and those of the lessons before it. */
export const toolsOf = (id: LessonId): Toolset => toolsUpTo(indexOf(id));

/** What the lesson before allowed, which its targets are chosen to beat; nothing for the first. */
export const toolsBefore = (id: LessonId): Toolset | undefined =>
  indexOf(id) > 0 ? toolsUpTo(indexOf(id) - 1) : undefined;
