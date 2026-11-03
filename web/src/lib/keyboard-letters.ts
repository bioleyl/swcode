import type { KeyboardProgress, LetterState, LetterStats, LetterView, TypedLetter } from '../types/keyboard.types';

/** The letters in the order they come, the most used first: the first few already make many words. */
export const LETTER_ORDER: readonly string[] = [...'etaoinsrhldcumfpgwybvkxjqz'];
const FIRST_LETTERS = 6;
/** The speeds people can aim for, in words a minute. */
export const TARGET_WPMS = [25, 35, 50, 70] as const;
const DEFAULT_WPM = 35;
const CHARS_PER_WORD = 5;
const MS_PER_MINUTE = 60_000;
// A letter's speed is the average of about its last this many samples, and it needs as many to count.
const LETTER_WINDOW = 10;

export const FIRST_PROGRESS: KeyboardProgress = { letters: {}, targetWpm: DEFAULT_WPM, unlocked: FIRST_LETTERS };

export const wpmOf = (msPerKey: number): number => MS_PER_MINUTE / (msPerKey * CHARS_PER_WORD);

const msPerKeyAt = (wpm: number): number => MS_PER_MINUTE / (wpm * CHARS_PER_WORD);

/** The letters people have. */
export const lettersOf = (progress: KeyboardProgress): readonly string[] =>
  LETTER_ORDER.slice(0, progress.unlocked);

/** How near the target a letter is: its speed over the target's, scaled down while it has few samples. */
const confidenceOf = (stats: LetterStats | undefined, targetWpm: number): number =>
  stats ? (msPerKeyAt(targetWpm) / stats.averageMs) * Math.min(1, stats.samples / LETTER_WINDOW) : 0;

const isConfident = (progress: KeyboardProgress, letter: string): boolean =>
  confidenceOf(progress.letters[letter], progress.targetWpm) >= 1;

/** The letter to practise: the furthest from the target, the newest of those as far. */
export const focusOf = (progress: KeyboardProgress): string => {
  const confidence = (letter: string): number => confidenceOf(progress.letters[letter], progress.targetWpm);
  return lettersOf(progress).reduce((focus, letter) => (confidence(letter) <= confidence(focus) ? letter : focus));
};

/** One more sample of a letter: its average moves toward it, the more so while it has few. */
export function recordLetter(progress: KeyboardProgress, typed: TypedLetter): KeyboardProgress {
  const known = progress.letters[typed.letter];
  const samples = (known?.samples ?? 0) + 1;
  const previous = known?.averageMs ?? typed.ms;
  const averageMs = previous + (typed.ms - previous) / Math.min(samples, LETTER_WINDOW);
  return { ...progress, letters: { ...progress.letters, [typed.letter]: { averageMs, samples } } };
}

/** The next letter, once each of those people have is typed at the target speed. */
export function unlockWhenReady(progress: KeyboardProgress): KeyboardProgress {
  const ready =
    progress.unlocked < LETTER_ORDER.length
    && lettersOf(progress).every((letter) => isConfident(progress, letter));
  return ready ? { ...progress, unlocked: progress.unlocked + 1 } : progress;
}

/** The letter people got last, when they have more than they started with. */
export const newestOf = (progress: KeyboardProgress): string | undefined =>
  progress.unlocked > FIRST_LETTERS ? LETTER_ORDER[progress.unlocked - 1] : undefined;

const stateOf = (progress: KeyboardProgress, letter: string, index: number): LetterState => {
  if (index >= progress.unlocked) {
    return 'locked';
  }
  return isConfident(progress, letter) ? 'confident' : 'learning';
};

/** Every letter as the row shows it. */
export function letterViews(progress: KeyboardProgress): LetterView[] {
  const focus = focusOf(progress);
  return LETTER_ORDER.map((letter, index) => {
    const stats = progress.letters[letter];
    return {
      focus: letter === focus,
      letter,
      state: stateOf(progress, letter, index),
      wpm: stats ? Math.round(wpmOf(stats.averageMs)) : undefined,
    };
  });
}
