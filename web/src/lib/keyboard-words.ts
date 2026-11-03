import { pickWeighted } from './random';

import type { Random } from '../types/game.types';
import type { WordModel } from '../types/keyboard.types';

/** The space stands for a word's edge: what follows it starts a word, and a letter it follows ends one. */
const WORD_EDGE = ' ';
// The letters before the next one that the model looks at: two, or the last alone where two lead nowhere.
const CONTEXT_LENGTHS = [2, 1] as const;
const TOKEN = /[A-Za-z]+/g;
// Markdown's code and link targets: commands, identifiers and addresses, not prose.
const MARKDOWN_CODE = [/```[\s\S]*?```/g, /`[^`]*`/g, /\]\([^)]*\)/g] as const;
// A word of prose: in lower case, or capitalised at the start of a sentence; not an identifier nor an acronym.
const PROSE_WORD = /^[A-Z]?[a-z]+$/;
const WORD_LENGTH = { max: 8, min: 2 } as const;
// The words tried for each one given: enough that one of them has the letter in focus.
const WORD_TRIES = 30;
// How often a word is chosen for having the letter in focus.
const FOCUS_SHARE = 0.6;
export const LINE_CHARS = 44;

/** The prose of a Markdown text: its code blocks, inline code and link targets left out. */
export const proseOf = (markdown: string): string =>
  MARKDOWN_CODE.reduce<string>((text, pattern) => text.replace(pattern, ' '), markdown);

/** What comes before the next letter of a word, as the model is keyed: the last two characters, then the last. */
const contextsOf = (start: string): string[] => [
  ...new Set(CONTEXT_LENGTHS.map((length) => (WORD_EDGE + start).slice(-length))),
];

/** How often each letter follows each pair and each letter, and starts and ends a word, in an English text. */
export function wordModelOf(text: string): WordModel {
  const follows = new Map<string, Map<string, number>>();
  const count = (context: string, next: string): void => {
    const row = follows.get(context) ?? new Map<string, number>();
    row.set(next, (row.get(next) ?? 0) + 1);
    follows.set(context, row);
  };
  const words = (text.match(TOKEN) ?? [])
    .filter((token) => PROSE_WORD.test(token))
    .map((word) => word.toLowerCase());
  for (const word of words) {
    [...word, WORD_EDGE].forEach((next, index) => {
      for (const context of contextsOf(word.slice(0, index))) {
        count(context, next);
      }
    });
  }
  return { follows };
}

const hasChoice = (choices: Record<string, number>): boolean => Object.keys(choices).length > 0;

/** What may come after a context: the letters people have, each as often as the text has it there, and the edge. */
const choicesAt = (
  model: WordModel,
  context: string,
  letters: ReadonlySet<string>,
  canEnd: boolean
): Record<string, number> =>
  Object.fromEntries(
    [...(model.follows.get(context) ?? [])].filter(([next]) => letters.has(next) || (next === WORD_EDGE && canEnd))
  );

/** What may come after the start of a word: after its last two characters, or its last where those lead nowhere. */
const choicesAfter = (
  model: WordModel,
  start: string,
  letters: ReadonlySet<string>,
  canEnd: boolean
): Record<string, number> =>
  contextsOf(start)
    .map((context) => choicesAt(model, context, letters, canEnd))
    .find(hasChoice) ?? {};

/** A word grown a letter at a time from the model with the given letters only; none when it runs into a dead end. */
function growWord(model: WordModel, letters: ReadonlySet<string>, random: Random): string | undefined {
  let word = '';
  while (word.length < WORD_LENGTH.max) {
    const choices = choicesAfter(model, word, letters, word.length >= WORD_LENGTH.min);
    if (!hasChoice(choices)) {
      return undefined;
    }
    const next = pickWeighted(choices, random);
    if (next === WORD_EDGE) {
      return word;
    }
    word += next;
  }
  return word;
}

/** A word of the given letters that reads like English, often one with the letter in focus. */
export function makeWord(model: WordModel, letters: ReadonlySet<string>, focus: string, random: Random): string {
  const words = Array.from({ length: WORD_TRIES }, () => growWord(model, letters, random)).filter(
    (word) => word !== undefined
  );
  const focused = random() < FOCUS_SHARE ? words.find((word) => word.includes(focus)) : undefined;
  return focused ?? words[0] ?? focus.repeat(WORD_LENGTH.min);
}

/** A line of such words, as many as fit in `LINE_CHARS`, never the same word twice in a row. */
export function makeLine(model: WordModel, letters: ReadonlySet<string>, focus: string, random: Random): string {
  const words: string[] = [];
  for (let tries = 0; tries < LINE_CHARS; tries++) {
    const word = makeWord(model, letters, focus, random);
    if ([...words, word].join(WORD_EDGE).length > LINE_CHARS) {
      break;
    }
    if (word === words.at(-1)) {
      continue;
    }
    words.push(word);
  }
  return words.join(WORD_EDGE);
}
