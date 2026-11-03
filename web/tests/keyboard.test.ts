import { describe, expect, it } from 'vitest';

import {
  FIRST_PROGRESS,
  focusOf,
  LETTER_ORDER,
  lettersOf,
  letterViews,
  newestOf,
  recordLetter,
  unlockWhenReady,
} from '../src/lib/keyboard-letters';
import { newRun, nextLine, pauseRun, typeKey } from '../src/lib/keyboard-run';
import { LINE_CHARS, makeLine, makeWord, proseOf, wordModelOf } from '../src/lib/keyboard-words';

import type { Random } from '../src/types/game.types';
import type { KeyboardProgress, KeyboardRun } from '../src/types/keyboard.types';

/** A fixed series of numbers in [0, 1), the same at each run. */
const seeded = (seed: number): Random => {
  let state = seed;
  return () => {
    state = (state * 1_103_515_245 + 12_345) % 2_147_483_648;
    return state / 2_147_483_648;
  };
};

const TEXT = 'The cat sat on the mat, then ate a tie. ScElement and WSL are code, not prose.';
const WORDS = 'tea ten net ant note tone into ion eat oat nine tie toe neat tent intent nation anoint';
const FIRST_SIX = new Set(lettersOf(FIRST_PROGRESS));

describe('the word model', () => {
  const model = wordModelOf(TEXT);

  it('counts how often each letter follows each, and starts and ends a word', () => {
    expect(model.follows.get('a')?.get('t')).toBe(4);
    expect(model.follows.get(' ')?.get('t')).toBe(4);
    expect(model.follows.get('e')?.get(' ')).toBe(7);
  });

  it('counts what follows each pair of letters too', () => {
    expect(model.follows.get('th')?.get('e')).toBe(3);
    expect(model.follows.get(' t')?.get('i')).toBe(1);
  });

  it('leaves out identifiers and acronyms', () => {
    expect(model.follows.get('s')?.get('c')).toBeUndefined();
    expect(model.follows.get('w')).toBeUndefined();
  });
});

describe('the prose of a Markdown text', () => {
  it('leaves out its code and its link targets', () => {
    const markdown = 'Run `npm test` [here](https://example.com).\n\n```bash\nls -la\n```\nDone.';
    expect(proseOf(markdown).match(/[a-z]+/gi)).toEqual(['Run', 'here', 'Done']);
  });
});

describe('the words', () => {
  const model = wordModelOf(WORDS);
  const random = seeded(7);

  it('are made of the letters people have only', () => {
    const words = Array.from({ length: 200 }, () => makeWord(model, FIRST_SIX, 'n', random));
    expect(words.every((word) => [...word].every((letter) => FIRST_SIX.has(letter)))).toBe(true);
    expect(words.every((word) => word.length >= 2 && word.length <= 8)).toBe(true);
  });

  it('bring the letter in focus often', () => {
    const words = Array.from({ length: 200 }, () => makeWord(model, FIRST_SIX, 'i', random));
    expect(words.filter((word) => word.includes('i')).length).toBeGreaterThan(100);
  });

  it('fill a line that fits, never the same word twice in a row', () => {
    const line = makeLine(model, FIRST_SIX, 'n', random);
    const words = line.split(' ');
    expect(line.length).toBeLessThanOrEqual(LINE_CHARS);
    expect(line.length).toBeGreaterThan(LINE_CHARS / 2);
    expect(words.every((word, index) => word !== words[index - 1])).toBe(true);
  });
});

describe('typing a line', () => {
  const typeAll = (run: KeyboardRun, keys: [string, number][]) =>
    keys.reduce((outcome, [key, now]) => typeKey(outcome.run, key, now), { run });

  it('times a letter from the key before, not the line’s first', () => {
    const first = typeKey(newRun('ab c', 'next'), 'a', 1_000);
    expect(first.typed).toBeUndefined();
    expect(typeKey(first.run, 'b', 1_300).typed).toEqual({ letter: 'b', ms: 300 });
  });

  it('stays on a character typed wrong, marked, until the right key', () => {
    const missed = typeAll(newRun('ab c', 'next'), [
      ['a', 1_000],
      ['x', 1_100],
    ]);
    expect(missed.run.at).toBe(1);
    expect(missed.run.missed).toBe(true);
    expect(typeKey(missed.run, 'b', 1_300).run.missed).toBe(false);
  });

  it('gives the line’s speed and accuracy once typed whole', () => {
    const done = typeAll(newRun('ab c', 'next'), [
      ['a', 1_000],
      ['x', 1_100],
      ['b', 1_300],
      [' ', 1_400],
    ]);
    // Three keys timed in 500 ms: 6 keys a second, 72 words a minute; 4 right keys out of 5.
    expect(typeKey(done.run, 'c', 1_500).finished).toEqual({ accuracy: 80, wpm: 72 });
  });

  it('times no key after a pause, nor after a long wait', () => {
    const started = typeKey(newRun('abc', 'next'), 'a', 1_000).run;
    expect(typeKey(pauseRun(started), 'b', 1_100).typed).toBeUndefined();
    expect(typeKey(started, 'b', 9_000).typed).toBeUndefined();
  });

  it('brings the next line up, the time going on from the last key', () => {
    const run = nextLine({ ...newRun('ab', 'cd'), lastAt: 1_000 }, 'ef');
    expect(run).toMatchObject({ at: 0, lastAt: 1_000, line: 'cd', next: 'ef' });
  });
});

describe('the letters', () => {
  const Fast = { averageMs: 100, samples: 10 };
  const allFast = (progress: KeyboardProgress): KeyboardProgress => ({
    ...progress,
    letters: Object.fromEntries(lettersOf(progress).map((letter) => [letter, Fast])),
  });

  it('start with six, the newest in focus before any is typed', () => {
    expect(lettersOf(FIRST_PROGRESS)).toEqual(LETTER_ORDER.slice(0, 6));
    expect(focusOf(FIRST_PROGRESS)).toBe(LETTER_ORDER[5]);
    expect(newestOf(FIRST_PROGRESS)).toBeUndefined();
  });

  it('average a letter’s times', () => {
    const once = recordLetter(FIRST_PROGRESS, { letter: 'e', ms: 300 });
    expect(recordLetter(once, { letter: 'e', ms: 500 }).letters.e).toEqual({ averageMs: 400, samples: 2 });
  });

  it('bring the next letter once each is at the target speed', () => {
    const next = unlockWhenReady(allFast(FIRST_PROGRESS));
    expect(next.unlocked).toBe(7);
    expect(newestOf(next)).toBe(LETTER_ORDER[6]);
  });

  it('wait while a letter is slow or has too few samples', () => {
    const fast = allFast(FIRST_PROGRESS);
    const slow = { ...fast, letters: { ...fast.letters, a: { averageMs: 900, samples: 10 } } };
    const few = { ...fast, letters: { ...fast.letters, a: { averageMs: 300, samples: 3 } } };
    expect(unlockWhenReady(slow).unlocked).toBe(6);
    expect(unlockWhenReady(few).unlocked).toBe(6);
    expect(focusOf(slow)).toBe('a');
  });

  it('show each letter, those to come locked', () => {
    const views = letterViews(allFast(FIRST_PROGRESS));
    expect(views).toHaveLength(LETTER_ORDER.length);
    expect(views[0]).toMatchObject({ state: 'confident', wpm: 120 });
    expect(views[6]).toMatchObject({ state: 'locked', wpm: undefined });
  });
});
